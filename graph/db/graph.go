package graphdb

import (
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type ChannelGraph struct {
	graphCache *GraphCache

	db DB
}

func (c *ChannelGraph) SetSourceNode(node *models.LightningNode) error {
	return c.db.SetSourceNode(node)
}

func (c *ChannelGraph) SourceNode() (*models.LightningNode, error) {
	return c.db.SourceNode()
}

func (c *ChannelGraph) ForEachChannel(cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return c.db.ForEachChannel(cb)
}

func (c *ChannelGraph) ForEachNodeCacheable(cb func(GraphCacheNode) error) error {
	return c.db.ForEachNodeCacheable(cb)
}

func (c *ChannelGraph) IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte, error) {
	return c.db.IsZombieEdge(chanID)
}

type chanGraphOpts struct {
	// withCache denotes whether the in-memory graph cache should be
	// used or a fallback version that uses the underlying database for
	// path finding.
	withCache bool

	// preAllocCacheNumNodes is the number of nodes we expect to be in the
	// graph cache, so we can pre-allocate the map accordingly.
	preAllocNumNodes int
}

func defaultChanGraphOpts() *chanGraphOpts {
	return &chanGraphOpts{
		withCache:        true,
		preAllocNumNodes: DefaultPreAllocCacheNumNodes,
	}
}

type ChanGraphOption func(*chanGraphOpts)

// WithPreAllocCacheNumNodes sets the PreAllocCacheNumNodes to n.
func WithPreAllocCacheNumNodes(n int) ChanGraphOption {
	return func(o *chanGraphOpts) {
		o.preAllocNumNodes = n
	}
}

// WithUseGraphCache sets the UseGraphCache option to the given value.
func WithUseGraphCache(use bool) ChanGraphOption {
	return func(o *chanGraphOpts) {
		o.withCache = use
	}
}

func NewChannelGraph(db *BoltStore, options ...ChanGraphOption) (*ChannelGraph,
	error) {

	opts := defaultChanGraphOpts()
	for _, option := range options {
		option(opts)
	}

	g := &ChannelGraph{
		db: db,
	}

	// The graph cache can be turned off (e.g. for mobile users) for a
	// speed/memory usage tradeoff.
	if opts.withCache {
		g.graphCache = NewGraphCache(opts.preAllocNumNodes)

		startTime := time.Now()
		log.Debugf("Populating in-memory channel graph, this might " +
			"take a while...")

		err := g.db.ForEachNodeCacheable(
			func(node GraphCacheNode) error {
				g.graphCache.AddNodeFeatures(node)

				return nil
			},
		)
		if err != nil {
			return nil, err
		}

		err = g.db.ForEachChannel(func(info *models.ChannelEdgeInfo,
			policy1, policy2 *models.ChannelEdgePolicy) error {

			g.graphCache.AddChannel(info, policy1, policy2)

			return nil
		})
		if err != nil {
			return nil, err
		}

		log.Debugf("Finished populating in-memory channel graph (took "+
			"%v, %s)", time.Since(startTime), g.graphCache.Stats())
	}

	return g, nil
}

func (c *ChannelGraph) PruneTip() (*chainhash.Hash, uint32, error) {
	return c.db.PruneTip()
}

func (c *ChannelGraph) AddLightningNode(node *models.LightningNode,
	op ...batch.SchedulerOption) error {

	opts := op
	if c.graphCache != nil {
		opts = append(opts, batch.OnUpdate(func() error {
			cNode := newGraphCacheNode(
				c.db,
				node.PubKeyBytes, node.Features,
			)
			return c.graphCache.AddNode(cNode)
		}))
	}

	return c.db.AddLightningNode(node, opts...)
}

func (c *ChannelGraph) PruneGraphNodes() error {
	return c.db.PruneGraphNodes(func(node route.Vertex) {
		if c.graphCache != nil {
			c.graphCache.RemoveNode(node)
		}
	})
}

func (c *ChannelGraph) NodeUpdatesInHorizon(startTime,
	endTime time.Time) ([]models.LightningNode, error) {

	return c.db.NodeUpdatesInHorizon(startTime, endTime)
}

func (c *ChannelGraph) FilterChannelRange(startHeight,
	endHeight uint32, withTimestamps bool) ([]BlockChannelRange, error) {

	return c.db.FilterChannelRange(startHeight, endHeight, withTimestamps)
}

func (c *ChannelGraph) ChannelView() ([]EdgePoint, error) {
	return c.db.ChannelView()
}

func (c *ChannelGraph) HasLightningNode(nodePub [33]byte) (time.Time, bool, error) {
	return c.db.HasLightningNode(nodePub)
}

func (c *ChannelGraph) FetchChanInfos(chanIDs []uint64) ([]ChannelEdge, error) {
	return c.db.FetchChanInfos(chanIDs)
}

func (c *ChannelGraph) FetchLightningNode(nodePub route.Vertex) (
	*models.LightningNode, error) {

	return c.db.FetchLightningNode(nodePub)
}

func (c *ChannelGraph) ForEachNode(cb func(*models.LightningNode) error) error {
	return c.db.ForEachNode(cb)
}

func (c *ChannelGraph) ForEachNodeChannel(nodePub route.Vertex,
	cb func(kvdb.RTx, *models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return c.db.ForEachNodeChannel(nodePub, cb)
}

func (c *ChannelGraph) HasChannelEdge(chanID uint64) (time.Time, time.Time,
	bool, bool, error) {

	return c.db.HasChannelEdge(chanID)
}

func (c *ChannelGraph) IsPublicNode(pubKey [33]byte) (bool, error) {
	return c.db.IsPublicNode(pubKey)
}

func (c *ChannelGraph) FetchChannelEdgesByID(chanID uint64) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	return c.db.FetchChannelEdgesByID(chanID)
}

func (c *ChannelGraph) DisabledChannelIDs() ([]uint64, error) {
	return c.db.DisabledChannelIDs()
}

func (c *ChannelGraph) ChanUpdatesInHorizon(startTime, endTime time.Time) (
	[]ChannelEdge, error) {

	return c.db.ChanUpdatesInHorizon(startTime, endTime)
}

func (c *ChannelGraph) FilterKnownChanIDs(chansInfo map[uint64]ChannelUpdateInfo,
	isZombieChan func(time.Time, time.Time) bool) ([]uint64, error) {

	newSCIDs, err := c.db.FilterKnownChanIDs(chansInfo)
	if err != nil {
		return nil, err
	}

	var scids []uint64
	for _, scid := range newSCIDs {
		isZombie, _, _, err := c.db.IsZombieEdge(scid)
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
		// just removing the channel from the db when
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
			err := c.MarkEdgeLive(scid)
			if err != nil {
				return nil, err
			}
		}

		scids = append(scids, scid)
	}

	return scids, nil
}

func (c *ChannelGraph) MarkEdgeLive(chanID uint64) error {
	return c.db.MarkEdgeLive(chanID, func(edge ChannelEdge) {
		if c.graphCache != nil {
			c.graphCache.AddChannel(edge.Info, edge.Policy1, edge.Policy2)
		}
	})
}

func (c *ChannelGraph) DeleteChannelEdges(strictZombiePruning, markZombie bool,
	chanIDs ...uint64) error {

	return c.db.DeleteChannelEdges(strictZombiePruning, markZombie,
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

func (c *ChannelGraph) HighestChanID() (uint64, error) {
	return c.db.HighestChanID()
}

func (c *ChannelGraph) MarkEdgeZombie(chanID uint64, pubKey1,
	pubKey2 [33]byte) error {

	err := c.db.MarkEdgeZombie(chanID, pubKey1, pubKey2)
	if err != nil {
		return err
	}

	if c.graphCache != nil {
		c.graphCache.RemoveChannel(pubKey1, pubKey2, chanID)
	}

	return nil
}

func (c *ChannelGraph) PruneGraph(spentOutputs []*wire.OutPoint,
	blockHash *chainhash.Hash, blockHeight uint32) (
	[]*models.ChannelEdgeInfo, error) {

	edges, err := c.db.PruneGraph(spentOutputs, blockHash, blockHeight,
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

	return edges, nil
}

func (c *ChannelGraph) AddChannelEdge(edge *models.ChannelEdgeInfo,
	op ...batch.SchedulerOption) error {

	opts := op
	if c.graphCache != nil {
		opts = append(opts, batch.OnUpdate(func() error {
			c.graphCache.AddChannel(edge, nil, nil)

			return nil
		}))
	}

	return c.db.AddChannelEdge(edge, opts...)
}

func (c *ChannelGraph) UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
	op ...batch.SchedulerOption) error {

	return c.db.UpdateEdgePolicy(
		edge, func(fromNode, toNode route.Vertex, isUpdate1 bool) {
			if c.graphCache != nil {
				c.graphCache.UpdatePolicy(
					edge, fromNode, toNode, isUpdate1,
				)
			}
		}, op...,
	)
}

func (c *ChannelGraph) ChannelID(chanPoint *wire.OutPoint) (uint64, error) {
	return c.db.ChannelID(chanPoint)
}

func (c *ChannelGraph) DisconnectBlockAtHeight(height uint32) (
	[]*models.ChannelEdgeInfo, error) {

	edges, err := c.db.DisconnectBlockAtHeight(height)
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

func (c *ChannelGraph) UpdateChannelEdge(edge *models.ChannelEdgeInfo) error {
	err := c.db.UpdateChannelEdge(edge)
	if err != nil {
		return err
	}

	if c.graphCache != nil {
		c.graphCache.UpdateChannel(edge)
	}

	return nil
}

func (c *ChannelGraph) DeleteLightningNode(nodePub route.Vertex) error {
	err := c.db.DeleteLightningNode(nodePub)
	if err != nil {
		return err
	}

	if c.graphCache != nil {
		c.graphCache.RemoveNode(nodePub)
	}

	return nil
}

func (c *ChannelGraph) ForEachNodeCached(cb func(node route.Vertex,
	chans map[uint64]*DirectedChannel) error) error {

	if c.graphCache != nil {
		return c.graphCache.ForEachNode(cb)
	}

	return c.db.ForEachNodeCached(cb)
}

func (c *ChannelGraph) NewRoutingGraphSession() (RoutingGraph, func() error,
	error) {

	if c.graphCache != nil {
		return c.NewRoutingGraph(), func() error { return nil }, nil
	}

	session, done, err := c.db.NewRoutingGraphSession()
	if err != nil {
		return nil, nil, err
	}

	return &chanGraphSession{
		c:     c,
		graph: session,
	}, done, nil
}

func (c *ChannelGraph) NewRoutingGraph() RoutingGraph {
	return &chanGraphSession{
		c:     c,
		graph: c.db.NewRoutingGraph(),
	}
}

type chanGraphSession struct {
	c     *ChannelGraph
	graph RoutingGraph
}

func (c *chanGraphSession) ForEachNodeChannel(ctx context.Context,
	node route.Vertex, cb func(channel *DirectedChannel) error) error {

	if c.c.graphCache != nil {
		return c.c.graphCache.ForEachChannel(node, cb)
	}

	return c.graph.ForEachNodeChannel(ctx, node, cb)
}

func (c *chanGraphSession) FetchNodeFeatures(ctx context.Context,
	node route.Vertex) (*lnwire.FeatureVector, error) {

	if c.c.graphCache != nil {
		return c.c.graphCache.GetFeatures(node), nil
	}

	return c.graph.FetchNodeFeatures(ctx, node)
}

var _ RoutingGraph = (*chanGraphSession)(nil)

// A compile time assertion to ensure ChannelGraph implements the GraphReads
// interface.
var _ GraphReads = (*ChannelGraph)(nil)
