package stats

import (
	"context"
	"math"
	"net"
	"runtime"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/autopilot"
	"github.com/lightningnetwork/lnd/discovery"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type GraphSource struct {
	DB *graphdb.ChannelGraph
	*ChanGraphStatsCollector
	IsGraphSynced func() (bool, error)
}

func (g *GraphSource) NewPathFindTx(ctx context.Context) (graphdb.RTx, error) {
	return g.DB.NewPathFindTx(ctx)
}

func (g *GraphSource) ForEachNodeDirectedChannel(ctx context.Context, tx graphdb.RTx, node route.Vertex, cb func(channel *graphdb.DirectedChannel) error) error {
	return g.DB.ForEachNodeDirectedChannel(ctx, tx, node, cb)
}

func (g *GraphSource) FetchNodeFeatures(ctx context.Context, tx graphdb.RTx, node route.Vertex) (*lnwire.FeatureVector, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) FetchChannelEdgesByID(ctx context.Context, chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) FetchChannelEdgesByOutpoint(ctx context.Context, point *wire.OutPoint) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) AddrsForNode(ctx context.Context, nodePub *btcec.PublicKey) (bool, []net.Addr, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) NetworkStats(ctx context.Context, excludeNodes map[route.Vertex]struct{}, excludeChannels map[uint64]struct{}) (*models.NetworkStats, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) GraphBootstrapper(ctx context.Context) (discovery.NetworkPeerBootstrapper, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) BetweenessCentrality(ctx context.Context) (map[autopilot.NodeID]*BetweenessCentrality, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) ForEachChannel(ctx context.Context, cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) HasLightningNode(ctx context.Context, nodePub [33]byte) (time.Time, bool, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) LookupAlias(ctx context.Context, pub *btcec.PublicKey) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) ForEachNodeChannel(ctx context.Context, nodePub route.Vertex, cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) ForEachNode(ctx context.Context, cb func(*models.LightningNode) error) error {
	//TODO implement me
	panic("implement me")
}

func (g *GraphSource) FetchLightningNode(ctx context.Context, nodePub route.Vertex) (*models.LightningNode, error) {
	return g.DB.FetchLightningNode(ctx, nodePub)
}

func (g *GraphSource) IsSynced(ctx context.Context) (bool, error) {
	return g.IsGraphSynced()
}

type StatsCollector interface {
	NetworkStats(ctx context.Context,
		excludeNodes map[route.Vertex]struct{},
		excludeChannels map[uint64]struct{}) (*models.NetworkStats,
		error)

	GraphBootstrapper(ctx context.Context) (discovery.NetworkPeerBootstrapper, error)

	BetweenessCentrality(ctx context.Context) (map[autopilot.NodeID]*BetweenessCentrality, error)
}

type BetweenessCentrality struct {
	Normalized    float64
	NonNormalized float64
}

type ChanGraphStatsCollector struct {
	DB *graphdb.ChannelGraph
}

func (c *ChanGraphStatsCollector) BetweenessCentrality(
	_ context.Context) (map[autopilot.NodeID]*BetweenessCentrality, error) {

	// Calculate betweenness centrality if requested. Note that depending on the
	// graph size, this may take up to a few minutes.
	channelGraph := autopilot.ChannelGraphFromDatabase(c.DB)
	centralityMetric, err := autopilot.NewBetweennessCentralityMetric(
		runtime.NumCPU(),
	)
	if err != nil {
		return nil, err
	}
	if err := centralityMetric.Refresh(channelGraph); err != nil {
		return nil, err
	}

	centrality := make(map[autopilot.NodeID]*BetweenessCentrality)

	for nodeID, val := range centralityMetric.GetMetric(true) {
		centrality[nodeID] = &BetweenessCentrality{
			Normalized: val,
		}
	}

	for nodeID, val := range centralityMetric.GetMetric(false) {
		if _, ok := centrality[nodeID]; !ok {
			centrality[nodeID] = &BetweenessCentrality{
				Normalized: val,
			}
			continue
		}
		centrality[nodeID].NonNormalized = val
	}

	return centrality, nil
}

var _ StatsCollector = (*ChanGraphStatsCollector)(nil)

func (c *ChanGraphStatsCollector) GraphBootstrapper(_ context.Context) (discovery.NetworkPeerBootstrapper, error) {
	chanGraph := autopilot.ChannelGraphFromDatabase(c.DB)

	return discovery.NewGraphBootstrapper(chanGraph)
}

func (c *ChanGraphStatsCollector) NetworkStats(ctx context.Context,
	excludeNodes map[route.Vertex]struct{},
	excludeChannels map[uint64]struct{}) (*models.NetworkStats, error) {

	var (
		numNodes             uint32
		numChannels          uint32
		maxChanOut           uint32
		totalNetworkCapacity btcutil.Amount
		minChannelSize       btcutil.Amount = math.MaxInt64
		maxChannelSize       btcutil.Amount
		medianChanSize       btcutil.Amount
	)

	// We'll use this map to de-duplicate channels during our traversal.
	// This is needed since channels are directional, so there will be two
	// edges for each channel within the graph.
	seenChans := make(map[uint64]struct{})

	// We also keep a list of all encountered capacities, in order to
	// calculate the median channel size.
	var allChans []btcutil.Amount

	// We'll run through all the known nodes in the within our view of the
	// network, tallying up the total number of nodes, and also gathering
	// each node so we can measure the graph diameter and degree stats
	// below.

	err := c.DB.ForEachNodeCached(ctx, func(node route.Vertex,
		edges map[uint64]*graphdb.DirectedChannel) error {

		// Increment the total number of nodes with each iteration.
		if _, ok := excludeNodes[node]; !ok {
			numNodes++
		}

		// For each channel we'll compute the out degree of each node,
		// and also update our running tallies of the min/max channel
		// capacity, as well as the total channel capacity. We pass
		// through the DB transaction from the outer view so we can
		// re-use it within this inner view.
		var outDegree uint32
		for _, edge := range edges {
			if _, ok := excludeChannels[edge.ChannelID]; ok {
				continue
			}

			// Bump up the out degree for this node for each
			// channel encountered.
			outDegree++

			// If we've already seen this channel, then we'll
			// return early to ensure that we don't double-count
			// stats.
			if _, ok := seenChans[edge.ChannelID]; ok {
				return nil
			}

			// Compare the capacity of this channel against the
			// running min/max to see if we should update the
			// extrema.
			chanCapacity := edge.Capacity
			if chanCapacity < minChannelSize {
				minChannelSize = chanCapacity
			}
			if chanCapacity > maxChannelSize {
				maxChannelSize = chanCapacity
			}

			// Accumulate the total capacity of this channel to the
			// network wide-capacity.
			totalNetworkCapacity += chanCapacity

			numChannels++

			seenChans[edge.ChannelID] = struct{}{}
			allChans = append(allChans, edge.Capacity)
		}

		// Finally, if the out degree of this node is greater than what
		// we've seen so far, update the maxChanOut variable.
		if outDegree > maxChanOut {
			maxChanOut = outDegree
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Find the median.
	medianChanSize = autopilot.Median(allChans)

	// If we don't have any channels, then reset the minChannelSize to zero
	// to avoid outputting NaN in encoded JSON.
	if numChannels == 0 {
		minChannelSize = 0
	}

	// Graph diameter.
	channelGraph := autopilot.ChannelGraphFromCachedDatabase(c.DB)
	simpleGraph, err := autopilot.NewSimpleGraph(channelGraph)
	if err != nil {
		return nil, err
	}
	// start := time.Now()
	diameter := simpleGraph.DiameterRadialCutoff()

	//log.Infof("elapsed time for diameter (%d) calculation: %v", diameter,
	// 	time.Since(start))

	// Query the graph for the current number of zombie channels.
	numZombies, err := c.DB.NumZombies(ctx)
	if err != nil {
		return nil, err
	}

	return &models.NetworkStats{
		Diameter:             diameter,
		MaxChanOut:           maxChanOut,
		NumNodes:             numNodes,
		NumChannels:          numChannels,
		TotalNetworkCapacity: totalNetworkCapacity,
		MinChanSize:          minChannelSize,
		MaxChanSize:          maxChannelSize,
		MedianChanSize:       medianChanSize,
		NumZombies:           numZombies,
	}, nil
}
