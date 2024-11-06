package lnd

import (
	"context"
	"math"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/autopilot"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/routing/route"
)

type StatsCollector interface {
	NetworkStats(ctx context.Context,
		excludeNodes map[route.Vertex]struct{},
		excludeChannels map[uint64]struct{}) (*models.NetworkStats,
		error)
}

type ChanGraphStatsCollector struct {
	*graphdb.ChannelGraph
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

	err := c.ForEachNodeCached(ctx, func(node route.Vertex,
		edges map[uint64]*graphdb.DirectedChannel) error {

		// Increment the total number of nodes with each iteration.
		if _, ok := excludeNodes[node]; !ok {
			numNodes++
		}

		// For each channel we'll compute the out degree of each node,
		// and also update our running tallies of the min/max channel
		// capacity, as well as the total channel capacity. We pass
		// through the db transaction from the outer view so we can
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
	channelGraph := autopilot.ChannelGraphFromCachedGraphSource(
		c.ChannelGraph,
	)
	simpleGraph, err := autopilot.NewSimpleGraph(channelGraph)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	diameter := simpleGraph.DiameterRadialCutoff()

	srvrLog.Infof("elapsed time for diameter (%d) calculation: %v", diameter,
		time.Since(start))

	// Query the graph for the current number of zombie channels.
	numZombies, err := c.NumZombies(ctx)
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
