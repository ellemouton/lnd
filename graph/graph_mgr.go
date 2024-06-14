package graph

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/chainview"
	"github.com/lightningnetwork/lnd/routing/route"
)

const (
	// DefaultFirstTimePruneDelay is the time we'll wait after startup
	// before attempting to prune the graph for zombie channels. We don't
	// do it immediately after startup to allow lnd to start up without
	// getting blocked by this job.
	DefaultFirstTimePruneDelay = 30 * time.Second

	// DefaultChannelPruneExpiry is the default duration used to determine
	// if a channel should be pruned or not.
	DefaultChannelPruneExpiry = time.Duration(time.Hour * 24 * 14)
)

var (
	// ErrGraphManagerShuttingDown is returned if the graph manager is in
	// the process of shutting down.
	ErrGraphManagerShuttingDown = fmt.Errorf("graph manager shutting down")
)

type Config struct {
	Graph GraphDB

	// Chain is the router's source to the most up-to-date blockchain data.
	// All incoming advertised channels will be checked against the chain
	// to ensure that the channels advertised are still open.
	Chain lnwallet.BlockChainIO

	// ChainView is an instance of a FilteredChainView which is used to
	// watch the sub-set of the UTXO set (the set of active channels) that
	// we need in order to properly maintain the channel graph.
	ChainView chainview.FilteredChainView

	// ChannelPruneExpiry is the duration used to determine if a channel
	// should be pruned or not. If the delta between now and when the
	// channel was last updated is greater than ChannelPruneExpiry, then
	// the channel is marked as a zombie channel eligible for pruning.
	ChannelPruneExpiry time.Duration

	// GraphPruneInterval is used as an interval to determine how often we
	// should examine the channel graph to garbage collect zombie channels.
	GraphPruneInterval time.Duration

	// AssumeChannelValid toggles whether the manager will check for
	// spentness of channel outpoints. For neutrino, this saves long rescans
	// from blocking initial usage of the daemon.
	AssumeChannelValid bool

	// FirstTimePruneDelay is the time we'll wait after startup before
	// attempting to prune the graph for zombie channels. We don't do it
	// immediately after startup to allow lnd to start up without getting
	// blocked by this job.
	FirstTimePruneDelay time.Duration

	// StrictZombiePruning determines if we attempt to prune zombie
	// channels according to a stricter criteria. If true, then we'll prune
	// a channel if only *one* of the edges is considered a zombie.
	// Otherwise, we'll only prune the channel when both edges have a very
	// dated last update.
	StrictZombiePruning bool
}

// Manager maintains a view of the Lightning Network graph.
type Manager struct {
	started uint32 // To be used atomically.
	stopped uint32 // To be used atomically.

	bestHeight uint32 // To be used atomically.

	cfg *Config

	// selfNode is this node's public key. It is used to know which edges
	// and vertices not to prune.
	selfNode route.Vertex

	quit chan struct{}
	wg   sync.WaitGroup
}

func NewManager(cfg *Config) (*Manager, error) {
	selfNode, err := cfg.Graph.SourceNode()
	if err != nil {
		return nil, err
	}

	return &Manager{
		cfg:      cfg,
		selfNode: selfNode.PubKeyBytes,
		quit:     make(chan struct{}),
	}, nil
}

func (m *Manager) Start() error {
	if !atomic.CompareAndSwapUint32(&m.started, 0, 1) {
		return nil
	}

	log.Info("GraphDB Manager starting")

	bestHash, bestHeight, err := m.cfg.Chain.GetBestBlock()
	if err != nil {
		return err
	}

	// If the graph has never been pruned, or hasn't fully been created yet,
	// then we don't treat this as an explicit error.
	if _, _, err := m.cfg.Graph.PruneTip(); err != nil {
		switch {
		case err == channeldb.ErrGraphNeverPruned:
			fallthrough
		case err == channeldb.ErrGraphNotFound:
			// If the graph has never been pruned, then we'll set
			// the prune height to the current best height of the
			// chain backend.
			_, err = m.cfg.Graph.PruneGraph(
				nil, bestHash, uint32(bestHeight),
			)
			if err != nil {
				return err
			}
		default:
			return err
		}
	}

	// If AssumeChannelValid is present, then we won't rely on pruning
	// channels from the graph based on their spentness, but whether they
	// are considered zombies or not. We will start zombie pruning after a
	// small delay, to avoid slowing down startup of lnd.
	if m.cfg.AssumeChannelValid {
		time.AfterFunc(m.cfg.FirstTimePruneDelay, func() {
			select {
			case <-m.quit:
				return
			default:
			}

			log.Info("Initial zombie prune starting")
			if err := m.pruneZombieChans(); err != nil {
				log.Errorf("Unable to prune zombies: %v", err)
			}
		})
	} else {
		// Otherwise, we'll use our filtered chain view to prune
		// channels as soon as they are detected as spent on-chain.
		if err := m.cfg.ChainView.Start(); err != nil {
			return err
		}

		// Before we perform our manual block pruning, we'll construct
		// and apply a fresh chain filter to the active
		// FilteredChainView instance.  We do this before, as otherwise
		// we may miss on-chain events as the filter hasn't properly
		// been applied.
		channelView, err := m.cfg.Graph.ChannelView()
		if err != nil && err != channeldb.ErrGraphNoEdgesFound {
			return err
		}

		log.Infof("Filtering chain using %v channels active",
			len(channelView))

		if len(channelView) != 0 {
			err = m.cfg.ChainView.UpdateFilter(
				channelView, uint32(bestHeight),
			)
			if err != nil {
				return err
			}
		}

		// The graph pruning might have taken a while and there could be
		// new blocks available.
		_, bestHeight, err = m.cfg.Chain.GetBestBlock()
		if err != nil {
			return err
		}
		m.bestHeight = uint32(bestHeight)

		// Before we begin normal operation of the router, we first need
		// to synchronize the channel graph to the latest state of the
		// UTXO set.
		if err := m.syncGraphWithChain(); err != nil {
			return err
		}

		// Finally, before we proceed, we'll prune any unconnected nodes
		// from the graph in order to ensure we maintain a tight graph
		// of "useful" nodes.
		err = m.cfg.Graph.PruneGraphNodes()
		if err != nil && err != channeldb.ErrGraphNodesNotFound {
			return err
		}
	}

	m.wg.Add(1)
	go m.pruneHandler()

	return nil
}

// Stop signals the ChannelRouter to gracefully halt all routines. This method
// will *block* until all goroutines have excited. If the channel router has
// already stopped then this method will return immediately.
func (m *Manager) Stop() error {
	if !atomic.CompareAndSwapUint32(&m.stopped, 0, 1) {
		return nil
	}

	log.Info("GraphDB Manager shutting down...")
	defer log.Debug("GraphDB Manager shutdown complete")

	// Our filtered chain view could've only been started if
	// AssumeChannelValid isn't present.
	if !m.cfg.AssumeChannelValid {
		if err := m.cfg.ChainView.Stop(); err != nil {
			return err
		}
	}

	close(m.quit)
	m.wg.Wait()

	return nil
}

func (m *Manager) pruneHandler() {
	defer m.wg.Done()

	pruneTicker := time.NewTicker(m.cfg.GraphPruneInterval)
	defer pruneTicker.Stop()

	for {
		select {
		// The graph prune ticker has ticked, so we'll examine the
		// state of the known graph to filter out any zombie channels
		// for pruning.
		case <-pruneTicker.C:
			if err := m.pruneZombieChans(); err != nil {
				log.Errorf("Unable to prune zombies: %v", err)
			}

		// The router has been signalled to exit, to we exit our main
		// loop so the wait group can be decremented.
		case <-m.quit:
			return
		}
	}
}

// pruneZombieChans is a method that will be called periodically to prune out
// any "zombie" channels. We consider channels zombies if *both* edges haven't
// been updated since our zombie horizon. If AssumeChannelValid is present,
// we'll also consider channels zombies if *both* edges are disabled. This
// usually signals that a channel has been closed on-chain. We do this
// periodically to keep a healthy, lively routing table.
func (m *Manager) pruneZombieChans() error {
	chansToPrune := make(map[uint64]struct{})
	chanExpiry := m.cfg.ChannelPruneExpiry

	log.Infof("Examining channel graph for zombie channels")

	// A helper method to detect if the channel belongs to this node
	isSelfChannelEdge := func(info *models.ChannelEdgeInfo) bool {
		return info.NodeKey1Bytes == m.selfNode ||
			info.NodeKey2Bytes == m.selfNode
	}

	// First, we'll collect all the channels which are eligible for garbage
	// collection due to being zombies.
	filterPruneChans := func(info *models.ChannelEdgeInfo,
		e1, e2 *models.ChannelEdgePolicy) error {

		// Exit early in case this channel is already marked to be
		// pruned
		_, markedToPrune := chansToPrune[info.ChannelID]
		if markedToPrune {
			return nil
		}

		// We'll ensure that we don't attempt to prune our *own*
		// channels from the graph, as in any case this should be
		// re-advertised by the sub-system above us.
		if isSelfChannelEdge(info) {
			return nil
		}

		e1Zombie, e2Zombie, isZombieChan := m.isZombieChannel(e1, e2)

		if e1Zombie {
			log.Tracef("Node1 pubkey=%x of chan_id=%v is zombie",
				info.NodeKey1Bytes, info.ChannelID)
		}

		if e2Zombie {
			log.Tracef("Node2 pubkey=%x of chan_id=%v is zombie",
				info.NodeKey2Bytes, info.ChannelID)
		}

		// If either edge hasn't been updated for a period of
		// chanExpiry, then we'll mark the channel itself as eligible
		// for graph pruning.
		if !isZombieChan {
			return nil
		}

		log.Debugf("ChannelID(%v) is a zombie, collecting to prune",
			info.ChannelID)

		// TODO(roasbeef): add ability to delete single directional edge
		chansToPrune[info.ChannelID] = struct{}{}

		return nil
	}

	// If AssumeChannelValid is present we'll look at the disabled bit for
	// both edges. If they're both disabled, then we can interpret this as
	// the channel being closed and can prune it from our graph.
	if m.cfg.AssumeChannelValid {
		disabledChanIDs, err := m.cfg.Graph.DisabledChannelIDs()
		if err != nil {
			return fmt.Errorf("unable to get disabled channels "+
				"ids chans: %v", err)
		}

		disabledEdges, err := m.cfg.Graph.FetchChanInfos(
			disabledChanIDs,
		)
		if err != nil {
			return fmt.Errorf("unable to fetch disabled channels "+
				"edges chans: %v", err)
		}

		// Ensuring we won't prune our own channel from the graph.
		for _, disabledEdge := range disabledEdges {
			if !isSelfChannelEdge(disabledEdge.Info) {
				chansToPrune[disabledEdge.Info.ChannelID] =
					struct{}{}
			}
		}
	}

	startTime := time.Unix(0, 0)
	endTime := time.Now().Add(-1 * chanExpiry)
	oldEdges, err := m.cfg.Graph.ChanUpdatesInHorizon(startTime, endTime)
	if err != nil {
		return fmt.Errorf("unable to fetch expired channel updates "+
			"chans: %v", err)
	}

	for _, u := range oldEdges {
		filterPruneChans(u.Info, u.Policy1, u.Policy2)
	}

	log.Infof("Pruning %v zombie channels", len(chansToPrune))
	if len(chansToPrune) == 0 {
		return nil
	}

	// With the set of zombie-like channels obtained, we'll do another pass
	// to delete them from the channel graph.
	toPrune := make([]uint64, 0, len(chansToPrune))
	for chanID := range chansToPrune {
		toPrune = append(toPrune, chanID)
		log.Tracef("Pruning zombie channel with ChannelID(%v)", chanID)
	}
	err = m.cfg.Graph.DeleteChannelEdges(
		m.cfg.StrictZombiePruning, true, toPrune...,
	)
	if err != nil {
		return fmt.Errorf("unable to delete zombie channels: %w", err)
	}

	// With the channels pruned, we'll also attempt to prune any nodes that
	// were a part of them.
	err = m.cfg.Graph.PruneGraphNodes()
	if err != nil && err != channeldb.ErrGraphNodesNotFound {
		return fmt.Errorf("unable to prune graph nodes: %w", err)
	}

	return nil
}

// syncGraphWithChain attempts to synchronize the current channel graph with
// the latest UTXO set state. This process involves pruning from the channel
// graph any channels which have been closed by spending their funding output
// since we've been down.
func (m *Manager) syncGraphWithChain() error {
	// First, we'll need to check to see if we're already in sync with the
	// latest state of the UTXO set.
	bestHash, bestHeight, err := m.cfg.Chain.GetBestBlock()
	if err != nil {
		return err
	}
	m.bestHeight = uint32(bestHeight)

	pruneHash, pruneHeight, err := m.cfg.Graph.PruneTip()
	if err != nil {
		switch {
		// If the graph has never been pruned, or hasn't fully been
		// created yet, then we don't treat this as an explicit error.
		case err == channeldb.ErrGraphNeverPruned:
		case err == channeldb.ErrGraphNotFound:
		default:
			return err
		}
	}

	log.Infof("Prune tip for Channel GraphDB: height=%v, hash=%v",
		pruneHeight, pruneHash)

	switch {

	// If the graph has never been pruned, then we can exit early as this
	// entails it's being created for the first time and hasn't seen any
	// block or created channels.
	case pruneHeight == 0 || pruneHash == nil:
		return nil

	// If the block hashes and heights match exactly, then we don't need to
	// prune the channel graph as we're already fully in sync.
	case bestHash.IsEqual(pruneHash) && uint32(bestHeight) == pruneHeight:
		return nil
	}

	// If the main chain blockhash at prune height is different from the
	// prune hash, this might indicate the database is on a stale branch.
	mainBlockHash, err := m.cfg.Chain.GetBlockHash(int64(pruneHeight))
	if err != nil {
		return err
	}

	// While we are on a stale branch of the chain, walk backwards to find
	// first common block.
	for !pruneHash.IsEqual(mainBlockHash) {
		log.Infof("channel graph is stale. Disconnecting block %v "+
			"(hash=%v)", pruneHeight, pruneHash)
		// Prune the graph for every channel that was opened at height
		// >= pruneHeight.
		_, err := m.cfg.Graph.DisconnectBlockAtHeight(pruneHeight)
		if err != nil {
			return err
		}

		pruneHash, pruneHeight, err = m.cfg.Graph.PruneTip()
		if err != nil {
			switch {
			// If at this point the graph has never been pruned, we
			// can exit as this entails we are back to the point
			// where it hasn't seen any block or created channels,
			// alas there's nothing left to prune.
			case err == channeldb.ErrGraphNeverPruned:
				return nil
			case err == channeldb.ErrGraphNotFound:
				return nil
			default:
				return err
			}
		}
		mainBlockHash, err = m.cfg.Chain.GetBlockHash(int64(pruneHeight))
		if err != nil {
			return err
		}
	}

	log.Infof("Syncing channel graph from height=%v (hash=%v) to height=%v "+
		"(hash=%v)", pruneHeight, pruneHash, bestHeight, bestHash)

	// If we're not yet caught up, then we'll walk forward in the chain
	// pruning the channel graph with each new block that hasn't yet been
	// consumed by the channel graph.
	var spentOutputs []*wire.OutPoint
	for nextHeight := pruneHeight + 1; nextHeight <= uint32(bestHeight); nextHeight++ {
		// Break out of the rescan early if a shutdown has been
		// requested, otherwise long rescans will block the daemon from
		// shutting down promptly.
		select {
		case <-m.quit:
			return ErrGraphManagerShuttingDown
		default:
		}

		// Using the next height, request a manual block pruning from
		// the chainview for the particular block hash.
		log.Infof("Filtering block for closed channels, at height: %v",
			int64(nextHeight))
		nextHash, err := m.cfg.Chain.GetBlockHash(int64(nextHeight))
		if err != nil {
			return err
		}
		log.Tracef("Running block filter on block with hash: %v",
			nextHash)
		filterBlock, err := m.cfg.ChainView.FilterBlock(nextHash)
		if err != nil {
			return err
		}

		// We're only interested in all prior outputs that have been
		// spent in the block, so collate all the referenced previous
		// outpoints within each tx and input.
		for _, tx := range filterBlock.Transactions {
			for _, txIn := range tx.TxIn {
				spentOutputs = append(spentOutputs,
					&txIn.PreviousOutPoint)
			}
		}
	}

	// With the spent outputs gathered, attempt to prune the channel graph,
	// also passing in the best hash+height so the prune tip can be updated.
	closedChans, err := m.cfg.Graph.PruneGraph(
		spentOutputs, bestHash, uint32(bestHeight),
	)
	if err != nil {
		return err
	}

	log.Infof("GraphDB pruning complete: %v channels were closed since "+
		"height %v", len(closedChans), pruneHeight)
	return nil
}

// isZombieChannel takes two edge policy updates and determines if the
// corresponding channel should be considered a zombie. The first boolean is
// true if the policy update from node 1 is considered a zombie, the second
// boolean is that of node 2, and the final boolean is true if the channel
// is considered a zombie.
func (m *Manager) isZombieChannel(e1, e2 *models.ChannelEdgePolicy) (bool, bool,
	bool) {

	chanExpiry := m.cfg.ChannelPruneExpiry

	e1Zombie := e1 == nil || time.Since(e1.LastUpdate) >= chanExpiry
	e2Zombie := e2 == nil || time.Since(e2.LastUpdate) >= chanExpiry

	var e1Time, e2Time time.Time
	if e1 != nil {
		e1Time = e1.LastUpdate
	}
	if e2 != nil {
		e2Time = e2.LastUpdate
	}

	return e1Zombie, e2Zombie, m.IsZombieChannel(e1Time, e2Time)
}

// IsZombieChannel takes the timestamps of the latest channel updates for a
// channel and returns true if the channel should be considered a zombie based
// on these timestamps.
func (m *Manager) IsZombieChannel(updateTime1,
	updateTime2 time.Time) bool {

	chanExpiry := m.cfg.ChannelPruneExpiry

	e1Zombie := updateTime1.IsZero() ||
		time.Since(updateTime1) >= chanExpiry

	e2Zombie := updateTime2.IsZero() ||
		time.Since(updateTime2) >= chanExpiry

	// If we're using strict zombie pruning, then a channel is only
	// considered live if both edges have a recent update we know of.
	if m.cfg.StrictZombiePruning {
		return e1Zombie || e2Zombie
	}

	// Otherwise, if we're using the less strict variant, then a channel is
	// considered live if either of the edges have a recent update.
	return e1Zombie && e2Zombie
}

// IsStaleEdgePolicy returns true if the graph source has a channel edge for
// the passed channel ID (and flags) that have a more recent timestamp.
//
// NOTE: This method is part of the Graph interface.
func (m *Manager) IsStaleEdgePolicy(chanID lnwire.ShortChannelID,
	timestamp time.Time, flags lnwire.ChanUpdateChanFlags) bool {

	edge1Timestamp, edge2Timestamp, exists, isZombie, err :=
		m.cfg.Graph.HasChannelEdge(chanID.ToUint64())
	if err != nil {
		log.Debugf("Check stale edge policy got error: %v", err)
		return false

	}

	// If we know of the edge as a zombie, then we'll make some additional
	// checks to determine if the new policy is fresh.
	if isZombie {
		// When running with AssumeChannelValid, we also prune channels
		// if both of their edges are disabled. We'll mark the new
		// policy as stale if it remains disabled.
		if m.cfg.AssumeChannelValid {
			isDisabled := flags&lnwire.ChanUpdateDisabled ==
				lnwire.ChanUpdateDisabled
			if isDisabled {
				return true
			}
		}

		// Otherwise, we'll fall back to our usual ChannelPruneExpiry.
		return time.Since(timestamp) > m.cfg.ChannelPruneExpiry
	}

	// If we don't know of the edge, then it means it's fresh (thus not
	// stale).
	if !exists {
		return false
	}

	// As edges are directional edge node has a unique policy for the
	// direction of the edge they control. Therefore we first check if we
	// already have the most up to date information for that edge. If so,
	// then we can exit early.
	switch {
	// A flag set of 0 indicates this is an announcement for the "first"
	// node in the channel.
	case flags&lnwire.ChanUpdateDirection == 0:
		return !edge1Timestamp.Before(timestamp)

	// Similarly, a flag set of 1 indicates this is an announcement for the
	// "second" node in the channel.
	case flags&lnwire.ChanUpdateDirection == 1:
		return !edge2Timestamp.Before(timestamp)
	}

	return false
}
