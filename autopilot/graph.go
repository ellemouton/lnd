package autopilot

import (
	"context"
	"encoding/hex"
	"net"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

var (
	testRBytes, _ = hex.DecodeString("8ce2bc69281ce27da07e6683571319d18e949ddfa2965fb6caa1bf0314f882d7")
	testSBytes, _ = hex.DecodeString("299105481d63e0f4bc2a88121167221b6700d72a0ead154c03be696a292d24ae")
	testRScalar   = new(btcec.ModNScalar)
	testSScalar   = new(btcec.ModNScalar)
	_             = testRScalar.SetByteSlice(testRBytes)
	_             = testSScalar.SetByteSlice(testSBytes)
	testSig       = ecdsa.NewSignature(testRScalar, testSScalar)

	chanIDCounter uint64 // To be used atomically.
)

// databaseChannelGraph wraps a channeldb.ChannelGraph instance with the
// necessary API to properly implement the autopilot.ChannelGraph interface.
//
// TODO(roasbeef): move inmpl to main package?
type databaseChannelGraph struct {
	db GraphSource
}

// A compile time assertion to ensure databaseChannelGraph meets the
// autopilot.ChannelGraph interface.
var _ ChannelGraph = (*databaseChannelGraph)(nil)

// ChannelGraphFromDatabase returns an instance of the autopilot.ChannelGraph
// backed by a GraphSource.
func ChannelGraphFromDatabase(db GraphSource) ChannelGraph {
	return &databaseChannelGraph{
		db: db,
	}
}

// ForEachNode is a higher-order function that should be called once for each
// connected node within the channel graph. If the passed callback returns an
// error, then execution should be terminated.
//
// NOTE: Part of the autopilot.ChannelGraph interface.
func (d *databaseChannelGraph) ForEachNode(ctx context.Context,
	cb func(context.Context, *models.LightningNode) error, reset func()) error {

	return d.db.ForEachNode(ctx, func(node *models.LightningNode) error {
		// We'll skip over any node that doesn't have any advertised
		// addresses. As we won't be able to reach them to actually
		// open any channels.
		if len(node.Addresses) == 0 {
			return nil
		}

		return cb(ctx, node)
	}, reset)
}

func (d *databaseChannelGraph) ForEachNodesChannels(ctx context.Context,
	cb func(*models.LightningNode, []*graphdb.NodeChannel) error,
	reset func()) error {

	return d.db.ForEachNodesChannels(ctx, cb, reset)
}

// databaseChannelGraphCached wraps a channeldb.ChannelGraph instance with the
// necessary API to properly implement the autopilot.ChannelGraph interface.
type databaseChannelGraphCached struct {
	db GraphSource
}

// A compile time assertion to ensure databaseChannelGraphCached meets the
// autopilot.ChannelGraph interface.
var _ ChannelGraph = (*databaseChannelGraphCached)(nil)

// ChannelGraphFromCachedDatabase returns an instance of the
// autopilot.ChannelGraph backed by a live, open channeldb instance.
func ChannelGraphFromCachedDatabase(db GraphSource) ChannelGraph {
	return &databaseChannelGraphCached{
		db: db,
	}
}

// dbNodeCached is a wrapper struct around a database transaction for a
// channeldb.LightningNode. The wrapper methods implement the autopilot.Node
// interface.
type dbNodeCached struct {
	node     route.Vertex
	channels map[uint64]*graphdb.DirectedChannel
}

// PubKey is the identity public key of the node.
//
// NOTE: Part of the autopilot.Node interface.
func (nc dbNodeCached) PubKey() [33]byte {
	return nc.node
}

// Addrs returns a slice of publicly reachable public TCP addresses that the
// peer is known to be listening on.
//
// NOTE: Part of the autopilot.Node interface.
func (nc dbNodeCached) Addrs() []net.Addr {
	// TODO: Add addresses to be usable by autopilot.
	return []net.Addr{}
}

// ForEachChannel is a higher-order function that will be used to iterate
// through all edges emanating from/to the target node. For each active
// channel, this function should be called with the populated ChannelEdge that
// describes the active channel.
//
// NOTE: Part of the autopilot.Node interface.
func (nc dbNodeCached) ForEachChannel(ctx context.Context,
	cb func(context.Context, ChannelEdge) error) error {

	for cid, channel := range nc.channels {
		edge := ChannelEdge{
			ChanID:   lnwire.NewShortChanIDFromInt(cid),
			Capacity: channel.Capacity,
			Peer: &models.LightningNode{
				PubKeyBytes: channel.OtherNode,
			},
		}

		if err := cb(ctx, edge); err != nil {
			return err
		}
	}

	return nil
}

// ForEachNode is a higher-order function that should be called once for each
// connected node within the channel graph. If the passed callback returns an
// error, then execution should be terminated.
//
// NOTE: Part of the autopilot.ChannelGraph interface.
func (dc *databaseChannelGraphCached) ForEachNode(ctx context.Context,
	cb func(context.Context, *models.LightningNode) error, reset func()) error {

	return dc.db.ForEachNodeCached(ctx, func(n route.Vertex,
		channels map[uint64]*graphdb.DirectedChannel) error {

		if len(channels) > 0 {
			// TODO(ELLE): put the interface back.
			return cb(ctx, &models.LightningNode{
				PubKeyBytes: n,
			})
		}
		return nil
	}, reset)
}

func (dc *databaseChannelGraphCached) ForEachNodesChannels(ctx context.Context,
	cb func(*models.LightningNode, []*graphdb.NodeChannel) error,
	reset func()) error {

	return dc.db.ForEachNodesChannels(ctx, cb, reset)
}

// memNode is a purely in-memory implementation of the autopilot.Node
// interface.
type memNode struct {
	pub *btcec.PublicKey

	chans []ChannelEdge

	addrs []net.Addr
}

// Median returns the median value in the slice of Amounts.
func Median(vals []btcutil.Amount) btcutil.Amount {
	sort.Slice(vals, func(i, j int) bool {
		return vals[i] < vals[j]
	})

	num := len(vals)
	switch {
	case num == 0:
		return 0

	case num%2 == 0:
		return (vals[num/2-1] + vals[num/2]) / 2

	default:
		return vals[num/2]
	}
}
