package graph

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/fn/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnrpc/graphrpc"
	"github.com/lightningnetwork/lnd/lnutils"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// TODO: move remote client set-up to here and change to call backs on update
// so we can notify our own topology change clients.
type ChannelGraph struct {
	started atomic.Bool
	stopped atomic.Bool

	cfg *ChanGraphCfg

	graphCache *graphdb.GraphCache

	src graphdb.Source

	remoteSrc *graphrpc.Client

	ntfnClientCounter atomic.Uint64

	// topologyClients maps a client's unique notification ID to a
	// topologyClient client that contains its notification dispatch
	// channel.
	topologyClients *lnutils.SyncMap[uint64, *topologyClient]

	// ntfnClientUpdates is a channel that's used to send new updates to
	// topology notification clients to the Builder. Updates either
	// add a new notification client, or cancel notifications for an
	// existing client.
	ntfnClientUpdates chan *topologyClientUpdate

	cg     *fn.ContextGuard
	cancel fn.Option[context.CancelFunc]
}

type ChanGraphCfg struct {
	DB                 *graphdb.BoltStore
	WithCache          bool
	ParseAddressString func(strAddress string) (
		net.Addr, error)

	RemoteConfig *graphrpc.RemoteGraph
}

const (
	DefaultRemoteGraphRPCTimeout = 5 * time.Second
)

func NewChannelGraph(cfg *ChanGraphCfg) (
	*ChannelGraph, error) {

	var cache *graphdb.GraphCache
	if cfg.WithCache {
		cache = graphdb.NewGraphCache(graphdb.DefaultPreAllocCacheNumNodes)
	}

	var (
		src                graphdb.Source = cfg.DB
		remoteSourceClient *graphrpc.Client
	)
	if cfg.RemoteConfig != nil && cfg.RemoteConfig.Enable {
		if cache != nil {
			cfg.RemoteConfig.OnNewChannel = fn.Some(func(edge *models.ChannelEdgeInfo) {
				cache.AddChannel(edge, nil, nil)
			})
			cfg.RemoteConfig.OnChannelUpdate = fn.Some(func(edge *models.ChannelEdgePolicy, fromNode,
				toNode route.Vertex, edge1 bool) {

				cache.UpdatePolicy(edge, fromNode, toNode, edge1)
			})
			cfg.RemoteConfig.OnNodeUpsert = fn.Some(func(node route.Vertex, features *lnwire.FeatureVector) {
				cache.AddNodeFeatures(node, features)
			})
			// TODO: cfg.RemoteConfig.OnChanClose
		}
		remoteSourceClient = graphrpc.NewRemoteClient(
			cfg.RemoteConfig, cfg.ParseAddressString,
		)
		getLocalPub := func() (route.Vertex, error) {
			node, err := cfg.DB.SourceNode()
			if err != nil {
				return route.Vertex{}, err
			}

			return route.NewVertexFromBytes(node.PubKeyBytes[:])
		}

		src = graphdb.NewMuxedSource(remoteSourceClient, cfg.DB, getLocalPub)
	}

	return &ChannelGraph{
		cfg:               cfg,
		graphCache:        cache,
		src:               src,
		remoteSrc:         remoteSourceClient,
		topologyClients:   &lnutils.SyncMap[uint64, *topologyClient]{},
		ntfnClientUpdates: make(chan *topologyClientUpdate),
		cg:                fn.NewContextGuard(),
	}, nil
}

func (c *ChannelGraph) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return nil
	}

	if c.remoteSrc != nil {
		err := c.remoteSrc.Start(ctx)
		if err != nil {
			return err
		}
	}

	// The graph cache can be turned off (e.g. for mobile users) for a
	// speed/memory usage tradeoff.
	if c.graphCache != nil {
		err := c.populateCache()
		if err != nil {
			return err
		}
	}

	ctx, _ = c.cg.Create(ctx)

	c.cg.WgAdd(1)
	go c.goForever(ctx)

	return nil
}

func (c *ChannelGraph) populateCache() error {
	startTime := time.Now()
	log.Debugf("Populating in-memory channel graph, this might " +
		"take a while...")

	err := c.src.ForEachNode(func(node *models.LightningNode) error {
		c.graphCache.AddNodeFeatures(
			node.PubKeyBytes, node.Features,
		)

		return nil
	})
	if err != nil {
		return err
	}

	err = c.src.ForEachChannel(func(info *models.ChannelEdgeInfo,
		policy1, policy2 *models.ChannelEdgePolicy) error {

		c.graphCache.AddChannel(info, policy1, policy2)

		return nil
	})
	if err != nil {
		return err
	}

	log.Debugf("Finished populating in-memory channel graph (took "+
		"%v, %s)", time.Since(startTime), c.graphCache.Stats())

	return nil
}

func (c *ChannelGraph) Stop() error {
	if !c.stopped.CompareAndSwap(false, true) {
		return nil
	}

	c.cg.Quit()
	c.cg.WgWait()

	return nil
}

func (c *ChannelGraph) goForever(ctx context.Context) {
	defer c.cg.WgDone()

	for {

		select {
		// A new notification client update has arrived. We're either
		// gaining a new client, or cancelling notifications for an
		// existing client.
		case ntfnUpdate := <-c.ntfnClientUpdates:
			clientID := ntfnUpdate.clientID

			if ntfnUpdate.cancel {
				client, ok := c.topologyClients.LoadAndDelete(
					clientID,
				)
				if ok {
					close(client.exit)
					client.wg.Wait()

					close(client.ntfnChan)
				}

				continue
			}

			c.topologyClients.Store(clientID, &topologyClient{
				ntfnChan: ntfnUpdate.ntfnChan,
				exit:     make(chan struct{}),
			})

		case <-ctx.Done():
			return
		}
	}
}

func (c *ChannelGraph) PruneTip() (*chainhash.Hash, uint32, error) {
	return c.cfg.DB.PruneTip()
}

func (c *ChannelGraph) SetSourceNode(node *models.LightningNode) error {
	return c.cfg.DB.SetSourceNode(node)
}

func (c *ChannelGraph) SourceNode() (*models.LightningNode, error) {
	return c.cfg.DB.SourceNode()
}

func (c *ChannelGraph) PruneGraphNodes() error {
	return c.cfg.DB.PruneGraphNodes(func(node route.Vertex) {
		if c.graphCache != nil {
			c.graphCache.RemoveNode(node)
		}
	})
}

// ChanSeries only: therefore, only on if syncers on and so no need to check
// remote.
func (c *ChannelGraph) NodeUpdatesInHorizon(startTime,
	endTime time.Time) ([]models.LightningNode, error) {

	return c.cfg.DB.NodeUpdatesInHorizon(startTime, endTime)
}

// ChanSeries only: therefore, only on if syncers on and so no need to check
// remote.
func (c *ChannelGraph) FilterChannelRange(startHeight,
	endHeight uint32, withTimestamps bool) ([]graphdb.BlockChannelRange, error) {

	return c.cfg.DB.FilterChannelRange(startHeight, endHeight, withTimestamps)
}

// Builder only: used to maintain local graph.
func (c *ChannelGraph) ChannelView() ([]models.EdgePoint, error) {
	return c.cfg.DB.ChannelView()
}

// Used only by Builder for pruning local DB and ChanSeries. So no remote needed.
func (c *ChannelGraph) FetchChanInfos(chanIDs []uint64) ([]graphdb.ChannelEdge, error) {
	return c.cfg.DB.FetchChanInfos(chanIDs)
}

// Builder only to maintain local graph.
func (c *ChannelGraph) HasChannelEdge(chanID uint64) (time.Time, time.Time,
	bool, bool, error) {

	return c.cfg.DB.HasChannelEdge(chanID)
}

// Builder only.
func (c *ChannelGraph) DisabledChannelIDs() ([]uint64, error) {
	return c.cfg.DB.DisabledChannelIDs()
}

// ChanSeries and Builder only.
func (c *ChannelGraph) ChanUpdatesInHorizon(startTime, endTime time.Time) (
	[]graphdb.ChannelEdge, error) {

	return c.cfg.DB.ChanUpdatesInHorizon(startTime, endTime)
}

// Chan series only.
func (c *ChannelGraph) FilterKnownChanIDs(chansInfo map[uint64]graphdb.ChannelUpdateInfo,
	isZombieChan func(time.Time, time.Time) bool) ([]uint64, error) {

	newSCIDs, err := c.cfg.DB.FilterKnownChanIDs(chansInfo)
	if err != nil {
		return nil, err
	}

	var scids []uint64
	for _, scid := range newSCIDs {
		isZombie, _, _, err := c.cfg.DB.IsZombieEdge(scid)
		if err != nil {
			return nil, err
		}

		info, ok := chansInfo[scid]
		if !ok {
			return nil, fmt.Errorf("channel %v not found in "+
				"channel update info", scid)
		}

		// TODO(ziggie): Make sure that for the strict
		// pruning case we compare the pubkeys and
		// whether the right timestamp is not older than
		// the `ChannelPruneExpiry`.
		//
		// NOTE: The timestamp data has no verification
		// attached to it in the `ReplyChannelRange` msg
		// so we are trusting this data at this point.
		// However it is not critical because we are
		// just removing the channel from the localDB when
		// the timestamps are more recent. During the
		// querying of the gossip msg verification
		// happens as usual.
		// However we should start punishing peers when
		// they don't provide us honest data ?
		isStillZombie := isZombieChan(
			info.Node1UpdateTimestamp,
			info.Node2UpdateTimestamp,
		)

		switch {
		// If the edge is a known zombie and if we
		// would still consider it a zombie given the
		// latest update timestamps, then we skip this
		// channel.
		case isZombie && isStillZombie:
			continue

		// Otherwise, if we have marked it as a zombie
		// but the latest update timestamps could bring
		// it back from the dead, then we mark it alive,
		// and we let it be added to the set of IDs to
		// query our peer for.
		case isZombie && !isStillZombie:
			err := c.cfg.DB.MarkEdgeLive(scid, func(edge graphdb.ChannelEdge) {
				if c.graphCache != nil {
					c.graphCache.AddChannel(edge.Info, edge.Policy1, edge.Policy2)
				}
			})
			if err != nil {
				return nil, err
			}
		}

		scids = append(scids, scid)
	}

	return scids, nil
}

// Builder only.
func (c *ChannelGraph) MarkEdgeLive(chanID uint64) error {
	return c.cfg.DB.MarkEdgeLive(chanID, func(edge graphdb.ChannelEdge) {
		if c.graphCache != nil {
			c.graphCache.AddChannel(edge.Info, edge.Policy1, edge.Policy2)
		}
	})
}

// local only
func (c *ChannelGraph) DeleteChannelEdges(strictZombiePruning, markZombie bool,
	chanIDs ...uint64) error {

	return c.cfg.DB.DeleteChannelEdges(strictZombiePruning, markZombie,
		func(deletedEdge *models.ChannelEdgeInfo) {
			if c.graphCache != nil {
				c.graphCache.RemoveChannel(
					deletedEdge.NodeKey1Bytes,
					deletedEdge.NodeKey2Bytes,
					deletedEdge.ChannelID,
				)
			}
		}, chanIDs...,
	)
}

// chan series
func (c *ChannelGraph) HighestChanID() (uint64, error) {
	return c.cfg.DB.HighestChanID()
}

// builder
func (c *ChannelGraph) MarkEdgeZombie(chanID uint64, pubKey1,
	pubKey2 [33]byte) error {

	err := c.cfg.DB.MarkEdgeZombie(chanID, pubKey1, pubKey2)
	if err != nil {
		return err
	}

	if c.graphCache != nil {
		c.graphCache.RemoveChannel(pubKey1, pubKey2, chanID)
	}

	return nil
}

// Builder.
func (c *ChannelGraph) PruneGraph(spentOutputs []*wire.OutPoint,
	blockHash *chainhash.Hash, blockHeight uint32) (
	[]*models.ChannelEdgeInfo, error) {

	edges, err := c.cfg.DB.PruneGraph(spentOutputs, blockHash, blockHeight,
		func(node route.Vertex) {
			if c.graphCache != nil {
				c.graphCache.RemoveNode(node)
			}
		},
		func(edgeInfo *models.ChannelEdgeInfo) {
			if c.graphCache != nil {
				c.graphCache.RemoveChannel(
					edgeInfo.NodeKey1Bytes,
					edgeInfo.NodeKey2Bytes,
					edgeInfo.ChannelID,
				)
			}
		},
	)
	if err != nil {
		return nil, err
	}

	if c.graphCache != nil {
		log.Debugf("Pruned graph, cache now has %s",
			c.graphCache.Stats())
	}

	// Notify all currently registered clients of the newly closed channels.
	closeSummaries := createCloseSummaries(blockHeight, edges...)
	c.notifyTopologyChange(&TopologyChange{
		ClosedChannels: closeSummaries,
	})

	return edges, nil
}

// builder
func (c *ChannelGraph) DisconnectBlockAtHeight(height uint32) (
	[]*models.ChannelEdgeInfo, error) {

	edges, err := c.cfg.DB.DisconnectBlockAtHeight(height)
	if err != nil {
		return nil, err
	}

	if c.graphCache != nil {
		for _, edge := range edges {
			c.graphCache.RemoveChannel(
				edge.NodeKey1Bytes, edge.NodeKey2Bytes,
				edge.ChannelID,
			)
		}
	}

	return edges, nil
}

// Local only
func (c *ChannelGraph) ChannelID(chanPoint *wire.OutPoint) (uint64, error) {
	return c.cfg.DB.ChannelID(chanPoint)
}

// MUX: combine local and remote.
func (c *ChannelGraph) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr,
	error) {

	return c.src.AddrsForNode(nodePub)
}

// MUX: combine local and remote.
func (c *ChannelGraph) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	// Take this opportunity to update the graph cache.

	return c.src.ForEachChannel(func(info *models.ChannelEdgeInfo,
		policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		if c.graphCache != nil {
			c.graphCache.AddChannel(info, policy, policy2)
		}

		return cb(info, policy, policy2)
	})
}

// MUX: check local and remote.
func (c *ChannelGraph) FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	return c.src.FetchChannelEdgesByOutpoint(op)
}

// MUX: if cache does not have, push to remote graph.
func (c *ChannelGraph) AddLightningNode(node *models.LightningNode,
	op ...batch.SchedulerOption) error {

	opts := op
	if c.graphCache != nil {
		opts = append(opts, batch.OnUpdate(func() error {
			cNode := graphdb.NewGraphCacheNode(
				c.cfg.DB,
				node.PubKeyBytes, node.Features,
			)
			return c.graphCache.AddNode(cNode)
		}))
	}

	// TODO: Push to remote.

	err := c.cfg.DB.AddLightningNode(node, opts...)
	if err != nil {
		return err
	}

	c.topChange(context.TODO(), node)

	return nil
}

func (c *ChannelGraph) topChange(ctx context.Context, update interface{}) {
	// Otherwise, we'll send off a new notification for the newly accepted
	// update, if any.
	topChange := &TopologyChange{}
	err := addToTopologyChange(
		ctx, c.FetchChannelEdgesByID, topChange, update,
	)
	if err != nil {
		log.Errorf("unable to update topology change notification: %v",
			err)
		return
	}

	if !topChange.isEmpty() {
		c.notifyTopologyChange(topChange)
	}
}

// MUX: check local and remote.
func (c *ChannelGraph) HasLightningNode(nodePub [33]byte) (time.Time, bool, error) {
	return c.src.HasLightningNode(nodePub)
}

// MUX: check local and remote.
func (c *ChannelGraph) FetchLightningNode(nodePub route.Vertex) (
	*models.LightningNode, error) {

	return c.src.FetchLightningNode(nodePub)
}

// MUX: used by Describe graph. Take opportunity to update cache.
func (c *ChannelGraph) ForEachNode(cb func(*models.LightningNode) error) error {
	return c.src.ForEachNode(func(node *models.LightningNode) error {
		if c.graphCache != nil {
			c.graphCache.AddNodeFeatures(
				node.PubKeyBytes, node.Features,
			)
		}

		return cb(node)
	})
}

// MUX: check local and remote.
func (c *ChannelGraph) ForEachNodeChannel(nodePub route.Vertex,
	cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	// Take this opportunity to update the graph cache.
	return c.src.ForEachNodeChannel(context.Background(), nodePub,
		func(info *models.ChannelEdgeInfo,
			policy *models.ChannelEdgePolicy,
			policy2 *models.ChannelEdgePolicy) error {

			if c.graphCache != nil {
				c.graphCache.AddChannel(info, policy, policy2)
			}

			return cb(info, policy, policy2)
		})
}

// MUX: check local and remote.
func (c *ChannelGraph) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool,
	error) {

	return c.src.IsPublicNode(ctx, pubKey)
}

// MUX: check local and remote.
func (c *ChannelGraph) FetchChannelEdgesByID(ctx context.Context,
	chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	return c.src.FetchChannelEdgesByID(ctx, chanID)
}

// Mux: push to remote
func (c *ChannelGraph) AddChannelEdge(edge *models.ChannelEdgeInfo,
	op ...batch.SchedulerOption) error {

	opts := op
	if c.graphCache != nil {
		opts = append(opts, batch.OnUpdate(func() error {
			c.graphCache.AddChannel(edge, nil, nil)

			return nil
		}))
	}

	err := c.cfg.DB.AddChannelEdge(edge, opts...)
	if err != nil {
		return err
	}

	c.topChange(context.TODO(), edge)

	// TODO: push update to remote graph (if not private).

	return nil
}

// Mux: push to remote.
func (c *ChannelGraph) UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
	op ...batch.SchedulerOption) error {

	err := c.cfg.DB.UpdateEdgePolicy(
		edge, func(fromNode, toNode route.Vertex, isUpdate1 bool) {
			if c.graphCache != nil {
				c.graphCache.UpdatePolicy(
					edge, fromNode, toNode, isUpdate1,
				)
			}
		}, op...,
	)
	if err != nil {
		return err
	}

	c.topChange(context.TODO(), edge)

	// TODO: push update to remote graph (if not private).

	return nil
}

// MUX: just use remote.
func (c *ChannelGraph) NumZombies() (uint64, error) {
	return c.src.NumZombies()
}

// MUX: write through to local, remote and cache.
func (c *ChannelGraph) UpdateChannelEdge(edge *models.ChannelEdgeInfo) error {
	err := c.cfg.DB.UpdateChannelEdge(edge)
	if err != nil {
		return err
	}

	if c.graphCache != nil {
		c.graphCache.UpdateChannel(edge)
	}

	return nil
}

// MUX: use cache, else mux local and remote.
func (c *ChannelGraph) ForEachNodeCached(cb func(node route.Vertex,
	chans map[uint64]*models.DirectedChannel) error) error {

	if c.graphCache != nil {
		return c.graphCache.ForEachNode(cb)
	}

	return c.src.ForEachNodeCached(cb)
}

func (c *ChannelGraph) NewRoutingGraphSession() (graphdb.RoutingGraph, func() error,
	error) {

	if c.graphCache != nil {
		return c.NewRoutingGraph(), func() error { return nil }, nil
	}

	session, done, err := c.src.NewRoutingGraphSession()
	if err != nil {
		return nil, nil, err
	}

	return &chanGraphSession{
		c:     c,
		graph: session,
	}, done, nil
}

func (c *ChannelGraph) NewRoutingGraph() graphdb.RoutingGraph {
	return &chanGraphSession{
		c:     c,
		graph: c.src.NewRoutingGraph(),
	}
}

type chanGraphSession struct {
	c     *ChannelGraph
	graph graphdb.RoutingGraph
}

// MUX: use cache, else mux local and remote.
func (c *chanGraphSession) ForEachNodeChannel(ctx context.Context,
	node route.Vertex, cb func(channel *models.DirectedChannel) error) error {

	if c.c.graphCache != nil {
		return c.c.graphCache.ForEachChannel(node, cb)
	}

	return c.graph.ForEachNodeChannel(ctx, node, cb)
}

// MUX: use cache, else mux local and remote.
func (c *chanGraphSession) FetchNodeFeatures(ctx context.Context,
	node route.Vertex) (*lnwire.FeatureVector, error) {

	if c.c.graphCache != nil {
		return c.c.graphCache.GetFeatures(node), nil
	}

	return c.graph.FetchNodeFeatures(ctx, node)
}

var _ graphdb.RoutingGraph = (*chanGraphSession)(nil)

// MUX: use cache, else mux local and remote.
func (c *ChannelGraph) ForEachNodeWithTx(ctx context.Context,
	cb func(graphdb.NodeTx) error) error {

	return c.src.ForEachNodeWithTx(ctx, cb)
}

//// A compile time assertion to ensure ChannelGraph implements the GraphReads
//// interface.
//var _ GraphReads = (*ChannelGraph)(nil)
