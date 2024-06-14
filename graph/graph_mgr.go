package graph

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnutils"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwallet/btcwallet"
	"github.com/lightningnetwork/lnd/lnwallet/chanvalidate"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/multimutex"
	"github.com/lightningnetwork/lnd/routing/chainview"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/ticker"
	"github.com/pkg/errors"
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

	// defaultStatInterval governs how often the router will log non-empty
	// stats related to processing new channels, updates, or node
	// announcements.
	defaultStatInterval = time.Minute
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

	// Notifier is a reference to the ChainNotifier, used to grab
	// the latest blocks if the router is missing any.
	Notifier chainntnfs.ChainNotifier

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

	// IsAlias returns whether a passed ShortChannelID is an alias. This is
	// only used for our local channels.
	IsAlias func(scid lnwire.ShortChannelID) bool
}

// Manager maintains a view of the Lightning Network graph.
type Manager struct {
	started uint32 // To be used atomically.
	stopped uint32 // To be used atomically.

	bestHeight        uint32 // To be used atomically.
	ntfnClientCounter uint64 // To be used atomically.

	cfg *Config

	// selfNode is this node's public key. It is used to know which edges
	// and vertices not to prune.
	selfNode route.Vertex

	// newBlocks is a channel in which new blocks connected to the end of
	// the main chain are sent over, and blocks updated after a call to
	// UpdateFilter.
	newBlocks <-chan *chainview.FilteredBlock

	// staleBlocks is a channel in which blocks disconnected from the end
	// of our currently known best chain are sent over.
	staleBlocks <-chan *chainview.FilteredBlock

	// topologyClients maps a client's unique notification ID to a
	// topologyClient client that contains its notification dispatch
	// channel.
	topologyClients *lnutils.SyncMap[uint64, *topologyClient]

	// ntfnClientUpdates is a channel that's used to send new updates to
	// topology notification clients to the ChannelRouter. Updates either
	// add a new notification client, or cancel notifications for an
	// existing client.
	ntfnClientUpdates chan *topologyClientUpdate

	// networkUpdates is a channel that carries new topology updates
	// messages from outside the ChannelRouter to be processed by the
	// networkHandler.
	networkUpdates chan *routingMsg

	// channelEdgeMtx is a mutex we use to make sure we process only one
	// ChannelEdgePolicy at a time for a given channelID, to ensure
	// consistency between the various database accesses.
	channelEdgeMtx *multimutex.Mutex[uint64]

	// statTicker is a resumable ticker that logs the router's progress as
	// it discovers channels or receives updates.
	statTicker ticker.Ticker

	// stats tracks newly processed channels, updates, and node
	// announcements over a window of defaultStatInterval.
	stats *graphStats

	quit chan struct{}
	wg   sync.WaitGroup
}

// A compile time check to ensure ChannelRouter implements the
// ChannelGraphSource interface.
var _ ChannelGraphSource = (*Manager)(nil)

func NewManager(cfg *Config) (*Manager, error) {
	selfNode, err := cfg.Graph.SourceNode()
	if err != nil {
		return nil, err
	}

	return &Manager{
		cfg:               cfg,
		selfNode:          selfNode.PubKeyBytes,
		topologyClients:   &lnutils.SyncMap[uint64, *topologyClient]{},
		ntfnClientUpdates: make(chan *topologyClientUpdate),
		networkUpdates:    make(chan *routingMsg),
		channelEdgeMtx:    multimutex.NewMutex[uint64](),
		statTicker:        ticker.New(defaultStatInterval),
		stats:             new(graphStats),
		quit:              make(chan struct{}),
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

		// Once the instance is active, we'll fetch the channel we'll
		// receive notifications over.
		m.newBlocks = m.cfg.ChainView.FilteredBlocks()
		m.staleBlocks = m.cfg.ChainView.DisconnectedBlocks()

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
	go m.graphHandler()

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

func (m *Manager) graphHandler() {
	defer m.wg.Done()

	pruneTicker := time.NewTicker(m.cfg.GraphPruneInterval)
	defer pruneTicker.Stop()

	defer m.statTicker.Stop()

	m.stats.Reset()

	// We'll use this validation barrier to ensure that we process all jobs
	// in the proper order during parallel validation.
	//
	// NOTE: For AssumeChannelValid, we bump up the maximum number of
	// concurrent validation requests since there are no blocks being
	// fetched. This significantly increases the performance of IGD for
	// neutrino nodes.
	//
	// However, we dial back to use multiple of the number of cores when
	// fully validating, to avoid fetching up to 1000 blocks from the
	// backend. On bitcoind, this will empirically cause massive latency
	// spikes when executing this many concurrent RPC calls. Critical
	// subsystems or basic rpc calls that rely on calls such as GetBestBlock
	// will hang due to excessive load.
	//
	// See https://github.com/lightningnetwork/lnd/issues/4892.
	var validationBarrier *ValidationBarrier
	if m.cfg.AssumeChannelValid {
		validationBarrier = NewValidationBarrier(1000, m.quit)
	} else {
		validationBarrier = NewValidationBarrier(
			4*runtime.NumCPU(), m.quit,
		)
	}

	for {
		// If there are stats, resume the statTicker.
		if !m.stats.Empty() {
			m.statTicker.Resume()
		}

		select {
		// The graph prune ticker has ticked, so we'll examine the
		// state of the known graph to filter out any zombie channels
		// for pruning.
		case <-pruneTicker.C:
			if err := m.pruneZombieChans(); err != nil {
				log.Errorf("Unable to prune zombies: %v", err)
			}

		case chainUpdate, ok := <-m.staleBlocks:
			// If the channel has been closed, then this indicates
			// the daemon is shutting down, so we exit ourselves.
			if !ok {
				return
			}

			// Since this block is stale, we update our best height
			// to the previous block.
			blockHeight := uint32(chainUpdate.Height)
			atomic.StoreUint32(&m.bestHeight, blockHeight-1)

			// Update the channel graph to reflect that this block
			// was disconnected.
			_, err := m.cfg.Graph.DisconnectBlockAtHeight(blockHeight)
			if err != nil {
				log.Errorf("unable to prune graph with stale "+
					"block: %v", err)
				continue
			}

			// TODO(halseth): notify client about the reorg?

		// A new block has arrived, so we can prune the channel graph
		// of any channels which were closed in the block.
		case chainUpdate, ok := <-m.newBlocks:
			// If the channel has been closed, then this indicates
			// the daemon is shutting down, so we exit ourselves.
			if !ok {
				return
			}

			// We'll ensure that any new blocks received attach
			// directly to the end of our main chain. If not, then
			// we've somehow missed some blocks. Here we'll catch
			// up the chain with the latest blocks.
			currentHeight := atomic.LoadUint32(&m.bestHeight)
			switch {
			case chainUpdate.Height == currentHeight+1:
				err := m.updateGraphWithClosedChannels(
					chainUpdate,
				)
				if err != nil {
					log.Errorf("unable to prune graph "+
						"with closed channels: %v", err)
				}

			case chainUpdate.Height > currentHeight+1:
				log.Errorf("out of order block: expecting "+
					"height=%v, got height=%v",
					currentHeight+1, chainUpdate.Height)

				err := m.getMissingBlocks(currentHeight, chainUpdate)
				if err != nil {
					log.Errorf("unable to retrieve missing"+
						"blocks: %v", err)
				}

			case chainUpdate.Height < currentHeight+1:
				log.Errorf("out of order block: expecting "+
					"height=%v, got height=%v",
					currentHeight+1, chainUpdate.Height)

				log.Infof("Skipping channel pruning since "+
					"received block height %v was already"+
					" processed.", chainUpdate.Height)
			}

		// A new notification client update has arrived. We're either
		// gaining a new client, or cancelling notifications for an
		// existing client.
		case ntfnUpdate := <-m.ntfnClientUpdates:
			clientID := ntfnUpdate.clientID

			if ntfnUpdate.cancel {
				client, ok := m.topologyClients.LoadAndDelete(
					clientID,
				)
				if ok {
					close(client.exit)
					client.wg.Wait()

					close(client.ntfnChan)
				}

				continue
			}

			m.topologyClients.Store(clientID, &topologyClient{
				ntfnChan: ntfnUpdate.ntfnChan,
				exit:     make(chan struct{}),
			})

		// A new fully validated network update has just arrived. As a
		// result we'll modify the channel graph accordingly depending
		// on the exact type of the message.
		case update := <-m.networkUpdates:
			// We'll set up any dependants, and wait until a free
			// slot for this job opens up, this allows us to not
			// have thousands of goroutines active.
			validationBarrier.InitJobDependencies(update.msg)

			m.wg.Add(1)
			go m.handleNetworkUpdate(validationBarrier, update)

			// TODO(roasbeef): remove all unconnected vertexes
			// after N blocks pass with no corresponding
			// announcements.

		// Log any stats if we've processed a non-empty number of
		// channels, updates, or nodes. We'll only pause the ticker if
		// the last window contained no updates to avoid resuming and
		// pausing while consecutive windows contain new info.
		case <-m.statTicker.Ticks():
			if !m.stats.Empty() {
				log.Infof(m.stats.String())
			} else {
				m.statTicker.Pause()
			}
			m.stats.Reset()

		// The router has been signalled to exit, to we exit our main
		// loop so the wait group can be decremented.
		case <-m.quit:
			return
		}
	}
}

// getMissingBlocks walks through all missing blocks and updates the graph
// closed channels accordingly.
func (m *Manager) getMissingBlocks(currentHeight uint32,
	chainUpdate *chainview.FilteredBlock) error {

	outdatedHash, err := m.cfg.Chain.GetBlockHash(int64(currentHeight))
	if err != nil {
		return err
	}

	outdatedBlock := &chainntnfs.BlockEpoch{
		Height: int32(currentHeight),
		Hash:   outdatedHash,
	}

	epochClient, err := m.cfg.Notifier.RegisterBlockEpochNtfn(
		outdatedBlock,
	)
	if err != nil {
		return err
	}
	defer epochClient.Cancel()

	blockDifference := int(chainUpdate.Height - currentHeight)

	// We'll walk through all the outdated blocks and make sure we're able
	// to update the graph with any closed channels from them.
	for i := 0; i < blockDifference; i++ {
		var (
			missingBlock *chainntnfs.BlockEpoch
			ok           bool
		)

		select {
		case missingBlock, ok = <-epochClient.Epochs:
			if !ok {
				return nil
			}

		case <-m.quit:
			return nil
		}

		filteredBlock, err := m.cfg.ChainView.FilterBlock(
			missingBlock.Hash,
		)
		if err != nil {
			return err
		}

		err = m.updateGraphWithClosedChannels(filteredBlock)
		if err != nil {
			return err
		}
	}

	return nil
}

// updateGraphWithClosedChannels prunes the channel graph of closed channels
// that are no longer needed.
func (m *Manager) updateGraphWithClosedChannels(
	chainUpdate *chainview.FilteredBlock) error {

	// Once a new block arrives, we update our running track of the height
	// of the chain tip.
	blockHeight := chainUpdate.Height

	atomic.StoreUint32(&m.bestHeight, blockHeight)
	log.Infof("Pruning channel graph using block %v (height=%v)",
		chainUpdate.Hash, blockHeight)

	// We're only interested in all prior outputs that have been spent in
	// the block, so collate all the referenced previous outpoints within
	// each tx and input.
	var spentOutputs []*wire.OutPoint
	for _, tx := range chainUpdate.Transactions {
		for _, txIn := range tx.TxIn {
			spentOutputs = append(spentOutputs,
				&txIn.PreviousOutPoint)
		}
	}

	// With the spent outputs gathered, attempt to prune the channel graph,
	// also passing in the hash+height of the block being pruned so the
	// prune tip can be updated.
	chansClosed, err := m.cfg.Graph.PruneGraph(spentOutputs,
		&chainUpdate.Hash, chainUpdate.Height)
	if err != nil {
		log.Errorf("unable to prune routing table: %v", err)
		return err
	}

	log.Infof("Block %v (height=%v) closed %v channels", chainUpdate.Hash,
		blockHeight, len(chansClosed))

	if len(chansClosed) == 0 {
		return err
	}

	// Notify all currently registered clients of the newly closed channels.
	closeSummaries := createCloseSummaries(blockHeight, chansClosed...)
	m.notifyTopologyChange(&TopologyChange{
		ClosedChannels: closeSummaries,
	})

	return nil
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

// SyncedHeight returns the block height to which the router subsystem currently
// is synced to. This can differ from the above chain height if the goroutine
// responsible for processing the blocks isn't yet up to speed.
func (m *Manager) SyncedHeight() uint32 {
	return atomic.LoadUint32(&m.bestHeight)
}

func (m *Manager) AddTopologyChange(msg interface{}) {
}

// routingMsg couples a routing related routing topology update to the
// error channel.
type routingMsg struct {
	msg interface{}
	op  []batch.SchedulerOption
	err chan error
}

// handleNetworkUpdate is responsible for processing the update message and
// notifies topology changes, if any.
//
// NOTE: must be run inside goroutine.
func (m *Manager) handleNetworkUpdate(vb *ValidationBarrier,
	update *routingMsg) {

	defer m.wg.Done()
	defer vb.CompleteJob()

	// If this message has an existing dependency, then we'll wait until
	// that has been fully validated before we proceed.
	err := vb.WaitForDependants(update.msg)
	if err != nil {
		switch {
		case IsError(err, ErrVBarrierShuttingDown):
			update.err <- err

		case IsError(err, ErrParentValidationFailed):
			update.err <- newErrf(ErrIgnored, err.Error())

		default:
			log.Warnf("unexpected error during validation "+
				"barrier shutdown: %v", err)
			update.err <- err
		}

		return
	}

	// Process the routing update to determine if this is either a new
	// update from our PoV or an update to a prior vertex/edge we
	// previously accepted.
	err = m.processUpdate(update.msg, update.op...)
	update.err <- err

	// If this message had any dependencies, then we can now signal them to
	// continue.
	allowDependents := err == nil || IsError(err, ErrIgnored, ErrOutdated)
	vb.SignalDependants(update.msg, allowDependents)

	// If the error is not nil here, there's no need to send topology
	// change.
	if err != nil {
		// We now decide to log an error or not. If allowDependents is
		// false, it means there is an error and the error is neither
		// ErrIgnored or ErrOutdated. In this case, we'll log an error.
		// Otherwise, we'll add debug log only.
		if allowDependents {
			log.Debugf("process network updates got: %v", err)
		} else {
			log.Errorf("process network updates got: %v", err)
		}

		return
	}

	// Otherwise, we'll send off a new notification for the newly accepted
	// update, if any.
	var topChange TopologyChange
	err = m.addToTopologyChange(&topChange, update.msg)
	if err != nil {
		log.Errorf("unable to update topology change notification: %v",
			err)
		return
	}

	if !topChange.isEmpty() {
		m.notifyTopologyChange(&topChange)
	}
}

// processUpdate processes a new relate authenticated channel/edge, node or
// channel/edge update network update. If the update didn't affect the internal
// state of the draft due to either being out of date, invalid, or redundant,
// then error is returned.
func (m *Manager) processUpdate(msg interface{},
	op ...batch.SchedulerOption) error {

	switch msg := msg.(type) {
	case *channeldb.LightningNode:
		// Before we add the node to the database, we'll check to see
		// if the announcement is "fresh" or not. If it isn't, then
		// we'll return an error.
		err := m.assertNodeAnnFreshness(msg.PubKeyBytes, msg.LastUpdate)
		if err != nil {
			return err
		}

		if err := m.cfg.Graph.AddLightningNode(msg, op...); err != nil {
			return errors.Errorf("unable to add node %x to the "+
				"graph: %v", msg.PubKeyBytes, err)
		}

		log.Tracef("Updated vertex data for node=%x", msg.PubKeyBytes)
		m.stats.incNumNodeUpdates()

	case *models.ChannelEdgeInfo:
		log.Debugf("Received ChannelEdgeInfo for channel %v",
			msg.ChannelID)

		// Prior to processing the announcement we first check if we
		// already know of this channel, if so, then we can exit early.
		_, _, exists, isZombie, err := m.cfg.Graph.HasChannelEdge(
			msg.ChannelID,
		)
		if err != nil && err != channeldb.ErrGraphNoEdgesFound {
			return errors.Errorf("unable to check for edge "+
				"existence: %v", err)
		}
		if isZombie {
			return newErrf(ErrIgnored, "ignoring msg for zombie "+
				"chan_id=%v", msg.ChannelID)
		}
		if exists {
			return newErrf(ErrIgnored, "ignoring msg for known "+
				"chan_id=%v", msg.ChannelID)
		}

		// If AssumeChannelValid is present, then we are unable to
		// perform any of the expensive checks below, so we'll
		// short-circuit our path straight to adding the edge to our
		// graph. If the passed ShortChannelID is an alias, then we'll
		// skip validation as it will not map to a legitimate tx. This
		// is not a DoS vector as only we can add an alias
		// ChannelAnnouncement from the gossiper.
		scid := lnwire.NewShortChanIDFromInt(msg.ChannelID)
		if m.cfg.AssumeChannelValid || m.cfg.IsAlias(scid) {
			if err := m.cfg.Graph.AddChannelEdge(msg, op...); err != nil {
				return fmt.Errorf("unable to add edge: %w", err)
			}
			log.Tracef("New channel discovered! Link "+
				"connects %x and %x with ChannelID(%v)",
				msg.NodeKey1Bytes, msg.NodeKey2Bytes,
				msg.ChannelID)
			m.stats.incNumEdgesDiscovered()

			break
		}

		// Before we can add the channel to the channel graph, we need
		// to obtain the full funding outpoint that's encoded within
		// the channel ID.
		channelID := lnwire.NewShortChanIDFromInt(msg.ChannelID)
		fundingTx, err := m.fetchFundingTxWrapper(&channelID)
		if err != nil {
			// In order to ensure we don't erroneously mark a
			// channel as a zombie due to an RPC failure, we'll
			// attempt to string match for the relevant errors.
			//
			// * btcd:
			//    * https://github.com/btcsuite/btcd/blob/master/rpcserver.go#L1316
			//    * https://github.com/btcsuite/btcd/blob/master/rpcserver.go#L1086
			// * bitcoind:
			//    * https://github.com/bitcoin/bitcoin/blob/7fcf53f7b4524572d1d0c9a5fdc388e87eb02416/src/rpc/blockchain.cpp#L770
			//     * https://github.com/bitcoin/bitcoin/blob/7fcf53f7b4524572d1d0c9a5fdc388e87eb02416/src/rpc/blockchain.cpp#L954
			switch {
			case strings.Contains(err.Error(), "not found"):
				fallthrough

			case strings.Contains(err.Error(), "out of range"):
				// If the funding transaction isn't found at
				// all, then we'll mark the edge itself as a
				// zombie so we don't continue to request it.
				// We use the "zero key" for both node pubkeys
				// so this edge can't be resurrected.
				zErr := m.addZombieEdge(msg.ChannelID)
				if zErr != nil {
					return zErr
				}

			default:
			}

			return newErrf(ErrNoFundingTransaction, "unable to "+
				"locate funding tx: %v", err)
		}

		// Recreate witness output to be sure that declared in channel
		// edge bitcoin keys and channel value corresponds to the
		// reality.
		fundingPkScript, err := makeFundingScript(
			msg.BitcoinKey1Bytes[:], msg.BitcoinKey2Bytes[:],
			msg.Features,
		)
		if err != nil {
			return err
		}

		// Next we'll validate that this channel is actually well
		// formed. If this check fails, then this channel either
		// doesn't exist, or isn't the one that was meant to be created
		// according to the passed channel proofs.
		fundingPoint, err := chanvalidate.Validate(&chanvalidate.Context{
			Locator: &chanvalidate.ShortChanIDChanLocator{
				ID: channelID,
			},
			MultiSigPkScript: fundingPkScript,
			FundingTx:        fundingTx,
		})
		if err != nil {
			// Mark the edge as a zombie so we won't try to
			// re-validate it on start up.
			if err := m.addZombieEdge(msg.ChannelID); err != nil {
				return err
			}

			return newErrf(ErrInvalidFundingOutput, "output "+
				"failed validation: %w", err)
		}

		// Now that we have the funding outpoint of the channel, ensure
		// that it hasn't yet been spent. If so, then this channel has
		// been closed so we'll ignore it.
		chanUtxo, err := m.cfg.Chain.GetUtxo(
			fundingPoint, fundingPkScript, channelID.BlockHeight,
			m.quit,
		)
		if err != nil {
			if errors.Is(err, btcwallet.ErrOutputSpent) {
				zErr := m.addZombieEdge(msg.ChannelID)
				if zErr != nil {
					return zErr
				}
			}

			return newErrf(ErrChannelSpent, "unable to fetch utxo "+
				"for chan_id=%v, chan_point=%v: %v",
				msg.ChannelID, fundingPoint, err)
		}

		// TODO(roasbeef): this is a hack, needs to be removed
		// after commitment fees are dynamic.
		msg.Capacity = btcutil.Amount(chanUtxo.Value)
		msg.ChannelPoint = *fundingPoint
		if err := m.cfg.Graph.AddChannelEdge(msg, op...); err != nil {
			return errors.Errorf("unable to add edge: %v", err)
		}

		log.Debugf("New channel discovered! Link "+
			"connects %x and %x with ChannelPoint(%v): "+
			"chan_id=%v, capacity=%v",
			msg.NodeKey1Bytes, msg.NodeKey2Bytes,
			fundingPoint, msg.ChannelID, msg.Capacity)
		m.stats.incNumEdgesDiscovered()

		// As a new edge has been added to the channel graph, we'll
		// update the current UTXO filter within our active
		// FilteredChainView so we are notified if/when this channel is
		// closed.
		filterUpdate := []channeldb.EdgePoint{
			{
				FundingPkScript: fundingPkScript,
				OutPoint:        *fundingPoint,
			},
		}

		err = m.cfg.ChainView.UpdateFilter(
			filterUpdate, atomic.LoadUint32(&m.bestHeight),
		)
		if err != nil {
			return errors.Errorf("unable to update chain view: %v",
				err)
		}

	case *models.ChannelEdgePolicy:
		log.Debugf("Received ChannelEdgePolicy for channel %v",
			msg.ChannelID)

		// We make sure to hold the mutex for this channel ID,
		// such that no other goroutine is concurrently doing
		// database accesses for the same channel ID.
		m.channelEdgeMtx.Lock(msg.ChannelID)
		defer m.channelEdgeMtx.Unlock(msg.ChannelID)

		edge1Timestamp, edge2Timestamp, exists, isZombie, err :=
			m.cfg.Graph.HasChannelEdge(msg.ChannelID)
		if err != nil && err != channeldb.ErrGraphNoEdgesFound {
			return errors.Errorf("unable to check for edge "+
				"existence: %v", err)

		}

		// If the channel is marked as a zombie in our database, and
		// we consider this a stale update, then we should not apply the
		// policy.
		isStaleUpdate := time.Since(msg.LastUpdate) > m.cfg.ChannelPruneExpiry
		if isZombie && isStaleUpdate {
			return newErrf(ErrIgnored, "ignoring stale update "+
				"(flags=%v|%v) for zombie chan_id=%v",
				msg.MessageFlags, msg.ChannelFlags,
				msg.ChannelID)
		}

		// If the channel doesn't exist in our database, we cannot
		// apply the updated policy.
		if !exists {
			return newErrf(ErrIgnored, "ignoring update "+
				"(flags=%v|%v) for unknown chan_id=%v",
				msg.MessageFlags, msg.ChannelFlags,
				msg.ChannelID)
		}

		// As edges are directional edge node has a unique policy for
		// the direction of the edge they control. Therefore we first
		// check if we already have the most up to date information for
		// that edge. If this message has a timestamp not strictly
		// newer than what we already know of we can exit early.
		switch {

		// A flag set of 0 indicates this is an announcement for the
		// "first" node in the channel.
		case msg.ChannelFlags&lnwire.ChanUpdateDirection == 0:

			// Ignore outdated message.
			if !edge1Timestamp.Before(msg.LastUpdate) {
				return newErrf(ErrOutdated, "Ignoring "+
					"outdated update (flags=%v|%v) for "+
					"known chan_id=%v", msg.MessageFlags,
					msg.ChannelFlags, msg.ChannelID)
			}

		// Similarly, a flag set of 1 indicates this is an announcement
		// for the "second" node in the channel.
		case msg.ChannelFlags&lnwire.ChanUpdateDirection == 1:

			// Ignore outdated message.
			if !edge2Timestamp.Before(msg.LastUpdate) {
				return newErrf(ErrOutdated, "Ignoring "+
					"outdated update (flags=%v|%v) for "+
					"known chan_id=%v", msg.MessageFlags,
					msg.ChannelFlags, msg.ChannelID)
			}
		}

		// Now that we know this isn't a stale update, we'll apply the
		// new edge policy to the proper directional edge within the
		// channel graph.
		if err = m.cfg.Graph.UpdateEdgePolicy(msg, op...); err != nil {
			err := errors.Errorf("unable to add channel: %v", err)
			log.Error(err)
			return err
		}

		log.Tracef("New channel update applied: %v",
			newLogClosure(func() string { return spew.Sdump(msg) }))
		m.stats.incNumChannelUpdates()

	default:
		return errors.Errorf("wrong routing update message type")
	}

	return nil
}

// assertNodeAnnFreshness returns a non-nil error if we have an announcement in
// the database for the passed node with a timestamp newer than the passed
// timestamp. ErrIgnored will be returned if we already have the node, and
// ErrOutdated will be returned if we have a timestamp that's after the new
// timestamp.
func (m *Manager) assertNodeAnnFreshness(node route.Vertex,
	msgTimestamp time.Time) error {

	// If we are not already aware of this node, it means that we don't
	// know about any channel using this node. To avoid a DoS attack by
	// node announcements, we will ignore such nodes. If we do know about
	// this node, check that this update brings info newer than what we
	// already have.
	lastUpdate, exists, err := m.cfg.Graph.HasLightningNode(node)
	if err != nil {
		return errors.Errorf("unable to query for the "+
			"existence of node: %v", err)
	}
	if !exists {
		return newErrf(ErrIgnored, "Ignoring node announcement"+
			" for node not found in channel graph (%x)",
			node[:])
	}

	// If we've reached this point then we're aware of the vertex being
	// advertised. So we now check if the new message has a new time stamp,
	// if not then we won't accept the new data as it would override newer
	// data.
	if !lastUpdate.Before(msgTimestamp) {
		return newErrf(ErrOutdated, "Ignoring outdated "+
			"announcement for %x", node[:])
	}

	return nil
}

// fetchFundingTxWrapper is a wrapper around fetchFundingTx, except that it
// will exit if the router has stopped.
func (m *Manager) fetchFundingTxWrapper(chanID *lnwire.ShortChannelID) (
	*wire.MsgTx, error) {

	txChan := make(chan *wire.MsgTx, 1)
	errChan := make(chan error, 1)

	go func() {
		tx, err := m.fetchFundingTx(chanID)
		if err != nil {
			errChan <- err
			return
		}

		txChan <- tx
	}()

	select {
	case tx := <-txChan:
		return tx, nil

	case err := <-errChan:
		return nil, err

	case <-m.quit:
		return nil, ErrGraphManagerShuttingDown
	}
}

// fetchFundingTx returns the funding transaction identified by the passed
// short channel ID.
//
// TODO(roasbeef): replace with call to GetBlockTransaction? (would allow to
// later use getblocktxn)
func (m *Manager) fetchFundingTx(chanID *lnwire.ShortChannelID) (*wire.MsgTx,
	error) {

	// First fetch the block hash by the block number encoded, then use
	// that hash to fetch the block itself.
	blockNum := int64(chanID.BlockHeight)
	blockHash, err := m.cfg.Chain.GetBlockHash(blockNum)
	if err != nil {
		return nil, err
	}
	fundingBlock, err := m.cfg.Chain.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}

	// As a sanity check, ensure that the advertised transaction index is
	// within the bounds of the total number of transactions within a
	// block.
	numTxns := uint32(len(fundingBlock.Transactions))
	if chanID.TxIndex > numTxns-1 {
		return nil, fmt.Errorf("tx_index=#%v "+
			"is out of range (max_index=%v), network_chan_id=%v",
			chanID.TxIndex, numTxns-1, chanID)
	}

	return fundingBlock.Transactions[chanID.TxIndex].Copy(), nil
}

// addZombieEdge adds a channel that failed complete validation into the zombie
// index so we can avoid having to re-validate it in the future.
func (m *Manager) addZombieEdge(chanID uint64) error {
	// If the edge fails validation we'll mark the edge itself as a zombie
	// so we don't continue to request it. We use the "zero key" for both
	// node pubkeys so this edge can't be resurrected.
	var zeroKey [33]byte
	err := m.cfg.Graph.MarkEdgeZombie(chanID, zeroKey, zeroKey)
	if err != nil {
		return fmt.Errorf("unable to mark spent chan(id=%v) as a "+
			"zombie: %w", chanID, err)
	}

	return nil
}

// makeFundingScript is used to make the funding script for both segwit v0 and
// segwit v1 (taproot) channels.
//
// TODO(roasbeef: export and use elsewhere?
func makeFundingScript(bitcoinKey1, bitcoinKey2 []byte,
	chanFeatures []byte) ([]byte, error) {

	legacyFundingScript := func() ([]byte, error) {
		witnessScript, err := input.GenMultiSigScript(
			bitcoinKey1, bitcoinKey2,
		)
		if err != nil {
			return nil, err
		}
		pkScript, err := input.WitnessScriptHash(witnessScript)
		if err != nil {
			return nil, err
		}

		return pkScript, nil
	}

	if len(chanFeatures) == 0 {
		return legacyFundingScript()
	}

	// In order to make the correct funding script, we'll need to parse the
	// chanFeatures bytes into a feature vector we can interact with.
	rawFeatures := lnwire.NewRawFeatureVector()
	err := rawFeatures.Decode(bytes.NewReader(chanFeatures))
	if err != nil {
		return nil, fmt.Errorf("unable to parse chan feature "+
			"bits: %w", err)
	}

	chanFeatureBits := lnwire.NewFeatureVector(
		rawFeatures, lnwire.Features,
	)
	if chanFeatureBits.HasFeature(
		lnwire.SimpleTaprootChannelsOptionalStaging,
	) {

		pubKey1, err := btcec.ParsePubKey(bitcoinKey1)
		if err != nil {
			return nil, err
		}
		pubKey2, err := btcec.ParsePubKey(bitcoinKey2)
		if err != nil {
			return nil, err
		}

		fundingScript, _, err := input.GenTaprootFundingScript(
			pubKey1, pubKey2, 0,
		)
		if err != nil {
			return nil, err
		}

		return fundingScript, nil
	}

	return legacyFundingScript()
}

// AddNode is used to add information about a node to the router database. If
// the node with this pubkey is not present in an existing channel, it will
// be ignored.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) AddNode(node *channeldb.LightningNode,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: node,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case m.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-m.quit:
			return ErrGraphManagerShuttingDown
		}
	case <-m.quit:
		return ErrGraphManagerShuttingDown
	}
}

// AddEdge is used to add edge/channel to the topology of the router, after all
// information about channel will be gathered this edge/channel might be used
// in construction of payment path.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) AddEdge(edge *models.ChannelEdgeInfo,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: edge,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case m.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-m.quit:
			return ErrGraphManagerShuttingDown
		}
	case <-m.quit:
		return ErrGraphManagerShuttingDown
	}
}

// UpdateEdge is used to update edge information, without this message edge
// considered as not fully constructed.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) UpdateEdge(update *models.ChannelEdgePolicy,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: update,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case m.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-m.quit:
			return ErrGraphManagerShuttingDown
		}
	case <-m.quit:
		return ErrGraphManagerShuttingDown
	}
}

// CurrentBlockHeight returns the block height from POV of the router subsystem.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) CurrentBlockHeight() (uint32, error) {
	_, height, err := m.cfg.Chain.GetBestBlock()

	return uint32(height), err
}

// GetChannelByID return the channel by the channel id.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) GetChannelByID(chanID lnwire.ShortChannelID) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	return m.cfg.Graph.FetchChannelEdgesByID(chanID.ToUint64())
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. channeldb.ErrGraphNodeNotFound is returned if the node doesn't exist
// within the graph.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) FetchLightningNode(node route.Vertex) (
	*channeldb.LightningNode, error) {

	return m.cfg.Graph.FetchLightningNode(nil, node)
}

// ForEachNode is used to iterate over every node in router topology.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) ForEachNode(
	cb func(*channeldb.LightningNode) error) error {

	return m.cfg.Graph.ForEachNode(
		func(_ kvdb.RTx, n *channeldb.LightningNode) error {
			return cb(n)
		})
}

// ForAllOutgoingChannels is used to iterate over all outgoing channels owned by
// the router.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) ForAllOutgoingChannels(cb func(kvdb.RTx,
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy) error) error {

	return m.cfg.Graph.ForEachNodeChannel(nil, m.selfNode,
		func(tx kvdb.RTx, c *models.ChannelEdgeInfo,
			e *models.ChannelEdgePolicy,
			_ *models.ChannelEdgePolicy) error {

			if e == nil {
				return fmt.Errorf("channel from self node " +
					"has no policy")
			}

			return cb(tx, c, e)
		},
	)
}

// AddProof updates the channel edge info with proof which is needed to
// properly announce the edge to the rest of the network.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) AddProof(chanID lnwire.ShortChannelID,
	proof *models.ChannelAuthProof) error {

	info, _, _, err := m.cfg.Graph.FetchChannelEdgesByID(chanID.ToUint64())
	if err != nil {
		return err
	}

	info.AuthProof = proof

	return m.cfg.Graph.UpdateChannelEdge(info)
}

// IsStaleNode returns true if the graph source has a node announcement for the
// target node with a more recent timestamp.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) IsStaleNode(node route.Vertex,
	timestamp time.Time) bool {

	// If our attempt to assert that the node announcement is fresh fails,
	// then we know that this is actually a stale announcement.
	err := m.assertNodeAnnFreshness(node, timestamp)
	if err != nil {
		log.Debugf("Checking stale node %x got %v", node, err)
		return true
	}

	return false
}

// IsPublicNode determines whether the given vertex is seen as a public node in
// the graph from the graph's source node's point of view.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) IsPublicNode(node route.Vertex) (bool, error) {
	return m.cfg.Graph.IsPublicNode(node)
}

// IsKnownEdge returns true if the graph source already knows of the passed
// channel ID either as a live or zombie edge.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) IsKnownEdge(chanID lnwire.ShortChannelID) bool {
	_, _, exists, isZombie, _ := m.cfg.Graph.HasChannelEdge(
		chanID.ToUint64(),
	)
	return exists || isZombie
}

// MarkEdgeLive clears an edge from our zombie index, deeming it as live.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (m *Manager) MarkEdgeLive(chanID lnwire.ShortChannelID) error {
	return m.cfg.Graph.MarkEdgeLive(chanID.ToUint64())
}
