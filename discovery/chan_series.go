package discovery

import (
	"context"
	"errors"
	"iter"
	"sort"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/fn/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/netann"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChannelGraphTimeSeries is an interface that provides time and block based
// querying into our view of the channel graph. New channels will have
// monotonically increasing block heights, and new channel updates will have
// increasing timestamps. Once we connect to a peer, we'll use the methods in
// this interface to determine if we're already in sync, or need to request
// some new information from them.
type ChannelGraphTimeSeries interface {
	// HighestChanID should return the channel ID of the channel we know of
	// that's furthest in the target chain. This channel will have a block
	// height that's close to the current tip of the main chain as we
	// know it.  We'll use this to start our QueryChannelRange dance with
	// the remote node.
	HighestChanID(ctx context.Context,
		chain chainhash.Hash) (*lnwire.ShortChannelID, error)

	// UpdatesInHorizon returns all known channel and node updates with an
	// update timestamp between the start time and end time. We'll use this
	// to catch up a remote node to the set of channel updates that they
	// may have missed out on within the target chain.
	UpdatesInHorizon(chain chainhash.Hash, startTime time.Time,
		endTime time.Time) iter.Seq2[lnwire.Message, error]

	// FilterKnownChanIDs takes a target chain, and a set of channel ID's,
	// and returns a filtered set of chan ID's. This filtered set of chan
	// ID's represents the ID's that we don't know of which were in the
	// passed superSet.
	FilterKnownChanIDs(chain chainhash.Hash,
		superSet []graphdb.ChannelUpdateInfo,
		isZombieChan func(graphdb.ChannelUpdateInfo) bool) (
		[]lnwire.ShortChannelID, error)

	// FilterChannelRange returns the set of channels that we created
	// between the start height and the end height. The channel IDs are
	// grouped by their common block height. We'll use this to to a remote
	// peer's QueryChannelRange message.
	FilterChannelRange(chain chainhash.Hash, startHeight, endHeight uint32,
		withTimestamps bool) ([]graphdb.BlockChannelRange, error)

	// FetchChanAnns returns a full set of channel announcements as well as
	// their updates that match the set of specified short channel ID's.
	// We'll use this to reply to a QueryShortChanIDs message sent by a
	// remote peer. The response will contain a unique set of
	// ChannelAnnouncements, the latest ChannelUpdate for each of the
	// announcements, and a unique set of NodeAnnouncements.
	FetchChanAnns(chain chainhash.Hash,
		shortChanIDs []lnwire.ShortChannelID) ([]lnwire.Message, error)

	// FetchChanUpdates returns the latest channel update messages for the
	// specified short channel ID. If no channel updates are known for the
	// channel, then an empty slice will be returned.
	FetchChanUpdates(chain chainhash.Hash,
		shortChanID lnwire.ShortChannelID) ([]lnwire.ChannelUpdate,
		error)
}

// ChanSeries is an implementation of the ChannelGraphTimeSeries
// interface backed by the channeldb ChannelGraph database. We'll provide this
// implementation to the AuthenticatedGossiper so it can properly use the
// in-protocol channel range queries to quickly and efficiently synchronize our
// channel state with all peers.
type ChanSeries struct {
	// graphs contains versioned views of the channel graph. The first
	// entry is always the v1 graph. Additional entries (e.g. v2) are
	// appended when the backend supports them.
	graphs []*graphdb.VersionedGraph
}

// NewChanSeries constructs a new ChanSeries backed by one or more versioned
// graph views. The first graph should be the v1 graph. Additional graphs
// (e.g. v2) are appended when the backend supports them.
func NewChanSeries(graphs ...*graphdb.VersionedGraph) *ChanSeries {
	return &ChanSeries{
		graphs: graphs,
	}
}

// HighestChanID should return is the channel ID of the channel we know of
// that's furthest in the target chain. This channel will have a block height
// that's close to the current tip of the main chain as we know it.  We'll use
// this to start our QueryChannelRange dance with the remote node.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) HighestChanID(ctx context.Context,
	_ chainhash.Hash) (*lnwire.ShortChannelID, error) {

	var highest uint64
	for _, g := range c.graphs {
		chanID, err := g.HighestChanID(ctx)
		if errors.Is(err, graphdb.ErrVersionNotSupportedForKVDB) {
			continue
		}
		if err != nil {
			return nil, err
		}

		if chanID > highest {
			highest = chanID
		}
	}

	shortChanID := lnwire.NewShortChanIDFromInt(highest)
	return &shortChanID, nil
}

// UpdatesInHorizon returns all known channel and node updates with an update
// timestamp between the start time and end time. We'll use this to catch up a
// remote node to the set of channel updates that they may have missed out on
// within the target chain.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) UpdatesInHorizon(chain chainhash.Hash,
	startTime, endTime time.Time) iter.Seq2[lnwire.Message, error] {

	return func(yield func(lnwire.Message, error) bool) {
		// The time-based horizon is a v1 gossip concept.
		// v2 uses block heights for update ordering and is
		// incompatible with time-based ranges. We only query
		// the first (v1) graph here. v2 channels flow through
		// the FilterChannelRange / FetchChanAnns path instead.
		if len(c.graphs) > 0 {
			c.yieldGraphHorizon(
				c.graphs[0], startTime, endTime, yield,
			)
		}
	}
}

// yieldGraphHorizon yields channel and node updates from a single versioned
// graph that fall within the given time horizon. Returns false if the caller
// should stop iterating.
func (c *ChanSeries) yieldGraphHorizon(g *graphdb.VersionedGraph,
	startTime, endTime time.Time,
	yield func(lnwire.Message, error) bool) bool {

	// Query for channels with updates in the horizon.
	chansInHorizon := g.ChanUpdatesInHorizon(
		context.TODO(), graphdb.ChanUpdateRange{
			StartTime: fn.Some(startTime),
			EndTime:   fn.Some(endTime),
		},
	)

	for channel, err := range chansInHorizon {
		if err != nil {
			yield(nil, err)
			return false
		}

		// If the channel hasn't been fully advertised yet, or
		// is a private channel, then we'll skip it as we can't
		// construct a full authentication proof if one is
		// requested.
		if channel.Info.AuthProof == nil {
			continue
		}

		//nolint:ll
		chanAnn, edge1, edge2, err := netann.CreateChanAnnouncement(
			channel.Info.AuthProof, channel.Info,
			channel.Policy1, channel.Policy2,
		)
		if err != nil {
			if !yield(nil, err) {
				return false
			}

			continue
		}

		if !yield(chanAnn, nil) {
			return false
		}

		// We don't want to send channel updates that don't
		// conform to the spec (anymore), so check to make sure
		// that these channel updates are valid before yielding
		// them.
		if edge1 != nil {
			err := netann.ValidateChannelUpdateFields(
				0, edge1,
			)
			if err != nil {
				log.Errorf("not sending invalid "+
					"channel update %v: %v",
					edge1, err)
			} else if !yield(edge1, nil) {
				return false
			}
		}
		if edge2 != nil {
			err := netann.ValidateChannelUpdateFields(
				0, edge2,
			)
			if err != nil {
				log.Errorf("not sending invalid "+
					"channel update %v: %v", edge2,
					err)
			} else if !yield(edge2, nil) {
				return false
			}
		}
	}

	// Send out all the node announcements that have an update within
	// the horizon as well. We send these after channels to ensure
	// that they follow any active channels they have.
	nodeAnnsInHorizon := g.NodeUpdatesInHorizon(
		context.TODO(), graphdb.NodeUpdateRange{
			StartTime: fn.Some(startTime),
			EndTime:   fn.Some(endTime),
		}, graphdb.WithIterPublicNodesOnly(),
	)
	for nodeAnn, err := range nodeAnnsInHorizon {
		if err != nil {
			yield(nil, err)
			return false
		}
		nodeUpdate, err := nodeAnn.NodeAnnouncement(true)
		if err != nil {
			if !yield(nil, err) {
				return false
			}

			continue
		}

		if !yield(nodeUpdate, nil) {
			return false
		}
	}

	return true
}

// FilterKnownChanIDs takes a target chain, and a set of channel ID's, and
// returns a filtered set of chan ID's. This filtered set of chan ID's
// represents the ID's that we don't know of which were in the passed superSet.
// A channel is considered "known" if it exists in any gossip version.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) FilterKnownChanIDs(_ chainhash.Hash,
	superSet []graphdb.ChannelUpdateInfo,
	isZombieChan func(graphdb.ChannelUpdateInfo) bool) (
	[]lnwire.ShortChannelID, error) {

	// Pass the superSet through each graph. Each graph removes channels
	// it knows about, so the output of one becomes the input of the next.
	// This ensures a channel known by any version is excluded.
	currentUnknowns := superSet
	for _, g := range c.graphs {
		unknownIDs, err := g.FilterKnownChanIDs(
			context.TODO(), currentUnknowns, isZombieChan,
		)
		if errors.Is(err, graphdb.ErrVersionNotSupportedForKVDB) {
			continue
		}
		if err != nil {
			return nil, err
		}

		// Build a ChannelUpdateInfo slice for the still-unknown
		// channels to pass to the next graph.
		unknownSet := make(
			map[uint64]struct{}, len(unknownIDs),
		)
		for _, id := range unknownIDs {
			unknownSet[id] = struct{}{}
		}

		var nextUnknowns []graphdb.ChannelUpdateInfo
		for _, info := range currentUnknowns {
			if _, ok := unknownSet[info.ShortChannelID.ToUint64()]; ok {
				nextUnknowns = append(nextUnknowns, info)
			}
		}
		currentUnknowns = nextUnknowns
	}

	filteredIDs := make(
		[]lnwire.ShortChannelID, 0, len(currentUnknowns),
	)
	for _, info := range currentUnknowns {
		filteredIDs = append(filteredIDs, info.ShortChannelID)
	}

	return filteredIDs, nil
}

// FilterChannelRange returns the set of channels that we created between the
// start height and the end height. The channel IDs are grouped by their common
// block height. We'll use this respond to a remote peer's QueryChannelRange
// message. Results from all gossip versions are merged.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) FilterChannelRange(_ chainhash.Hash, startHeight,
	endHeight uint32, withTimestamps bool) ([]graphdb.BlockChannelRange,
	error) {

	// Collect ranges from all graphs, merging by block height.
	heightMap := make(map[uint32]map[lnwire.ShortChannelID]graphdb.ChannelUpdateInfo)

	for _, g := range c.graphs {
		ranges, err := g.FilterChannelRange(
			context.TODO(), startHeight, endHeight,
			withTimestamps,
		)
		if errors.Is(err, graphdb.ErrVersionNotSupportedForKVDB) {
			continue
		}
		if err != nil {
			return nil, err
		}

		for _, r := range ranges {
			if _, ok := heightMap[r.Height]; !ok {
				heightMap[r.Height] = make(
					map[lnwire.ShortChannelID]graphdb.ChannelUpdateInfo,
				)
			}
			for _, ch := range r.Channels {
				// Higher gossip versions overwrite lower
				// ones for the same SCID.
				existing, ok := heightMap[r.Height][ch.ShortChannelID]
				if !ok || ch.Version > existing.Version {
					heightMap[r.Height][ch.ShortChannelID] = ch
				}
			}
		}
	}

	// Convert the map back to sorted BlockChannelRange slices.
	heights := make([]uint32, 0, len(heightMap))
	for h := range heightMap {
		heights = append(heights, h)
	}
	sort.Slice(heights, func(i, j int) bool {
		return heights[i] < heights[j]
	})

	result := make([]graphdb.BlockChannelRange, 0, len(heights))
	for _, h := range heights {
		chans := heightMap[h]
		channels := make(
			[]graphdb.ChannelUpdateInfo, 0, len(chans),
		)
		for _, ch := range chans {
			channels = append(channels, ch)
		}
		result = append(result, graphdb.BlockChannelRange{
			Height:   h,
			Channels: channels,
		})
	}

	return result, nil
}

// FetchChanAnns returns a full set of channel announcements as well as their
// updates that match the set of specified short channel ID's.  We'll use this
// to reply to a QueryShortChanIDs message sent by a remote peer. The response
// will contain a unique set of ChannelAnnouncements, the latest ChannelUpdate
// for each of the announcements, and a unique set of NodeAnnouncements.
// Invalid node announcements are skipped and logged for debugging purposes.
// All gossip versions are queried; higher versions take precedence.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) FetchChanAnns(chain chainhash.Hash,
	shortChanIDs []lnwire.ShortChannelID) ([]lnwire.Message, error) {

	chanIDs := make([]uint64, 0, len(shortChanIDs))
	for _, chanID := range shortChanIDs {
		chanIDs = append(chanIDs, chanID.ToUint64())
	}

	// Collect channel edges across all graphs. Higher-versioned graphs
	// are queried last so they overwrite lower versions for the same
	// channel ID.
	edgeMap := make(map[uint64]graphdb.ChannelEdge)
	for _, g := range c.graphs {
		channels, err := g.FetchChanInfos(context.TODO(), chanIDs)
		if errors.Is(err, graphdb.ErrVersionNotSupportedForKVDB) {
			continue
		}
		if err != nil {
			return nil, err
		}

		for _, ch := range channels {
			edgeMap[ch.Info.ChannelID] = ch
		}
	}

	// We'll use this map to ensure we don't send the same node
	// announcement more than one time as one node may have many channel
	// anns we'll need to send.
	nodePubsSent := make(map[route.Vertex]struct{})

	chanAnns := make([]lnwire.Message, 0, len(edgeMap)*3)
	for _, channel := range edgeMap {
		// If the channel doesn't have an authentication proof, then we
		// won't send it over as it may not yet be finalized, or be a
		// non-advertised channel.
		if channel.Info.AuthProof == nil {
			continue
		}

		chanAnn, edge1, edge2, err := netann.CreateChanAnnouncement(
			channel.Info.AuthProof, channel.Info,
			channel.Policy1, channel.Policy2,
		)
		if err != nil {
			return nil, err
		}

		chanAnns = append(chanAnns, chanAnn)
		if edge1 != nil {
			chanAnns = append(chanAnns, edge1)

			// If this edge has a validated node announcement, that
			// we haven't yet sent, then we'll send that as well.
			nodePub := channel.Node2.PubKeyBytes
			hasNodeAnn := channel.Node2.HaveAnnouncement()
			if _, ok := nodePubsSent[nodePub]; !ok && hasNodeAnn {
				nodeAnn, err := channel.Node2.NodeAnnouncement(
					true,
				)
				if err != nil {
					return nil, err
				}

				err = netann.ValidateNodeAnnFields(nodeAnn)
				if err != nil {
					log.Debugf("Skipping forwarding "+
						"invalid node announcement "+
						"%x: %v", nodeAnn.NodePub(), err)
				} else {
					chanAnns = append(chanAnns, nodeAnn)
					nodePubsSent[nodePub] = struct{}{}
				}
			}
		}
		if edge2 != nil {
			chanAnns = append(chanAnns, edge2)

			// If this edge has a validated node announcement, that
			// we haven't yet sent, then we'll send that as well.
			nodePub := channel.Node1.PubKeyBytes
			hasNodeAnn := channel.Node1.HaveAnnouncement()
			if _, ok := nodePubsSent[nodePub]; !ok && hasNodeAnn {
				nodeAnn, err := channel.Node1.NodeAnnouncement(
					true,
				)
				if err != nil {
					return nil, err
				}

				err = netann.ValidateNodeAnnFields(nodeAnn)
				if err != nil {
					log.Debugf("Skipping forwarding "+
						"invalid node announcement "+
						"%x: %v", nodeAnn.NodePub(), err)
				} else {
					chanAnns = append(chanAnns, nodeAnn)
					nodePubsSent[nodePub] = struct{}{}
				}
			}
		}
	}

	return chanAnns, nil
}

// FetchChanUpdates returns the latest channel update messages for the
// specified short channel ID. If no channel updates are known for the channel,
// then an empty slice will be returned. All gossip versions are checked;
// the first graph that has the channel returns its updates.
//
// NOTE: This is part of the ChannelGraphTimeSeries interface.
func (c *ChanSeries) FetchChanUpdates(chain chainhash.Hash,
	shortChanID lnwire.ShortChannelID) ([]lnwire.ChannelUpdate, error) {

	// Try each graph in reverse order (highest version first) so that
	// v2 updates are preferred over v1 for the same channel.
	for i := len(c.graphs) - 1; i >= 0; i-- {
		g := c.graphs[i]

		chanInfo, e1, e2, err := g.FetchChannelEdgesByID(
			context.TODO(), shortChanID.ToUint64(),
		)
		if err != nil {
			// Channel not found in this graph version, try next.
			if errors.Is(err, graphdb.ErrEdgeNotFound) {
				continue
			}

			return nil, err
		}

		chanUpdates := make([]lnwire.ChannelUpdate, 0, 2)
		if e1 != nil {
			chanUpdate, err := netann.ChannelUpdateFromEdge(
				chanInfo, e1,
			)
			if err != nil {
				return nil, err
			}

			chanUpdates = append(chanUpdates, chanUpdate)
		}
		if e2 != nil {
			chanUpdate, err := netann.ChannelUpdateFromEdge(
				chanInfo, e2,
			)
			if err != nil {
				return nil, err
			}

			chanUpdates = append(chanUpdates, chanUpdate)
		}

		return chanUpdates, nil
	}

	return nil, nil
}

// A compile-time assertion to ensure that ChanSeries meets the
// ChannelGraphTimeSeries interface.
var _ ChannelGraphTimeSeries = (*ChanSeries)(nil)
