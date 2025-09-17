package invoicesrpc

import (
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
)

// GraphSource defines the graph interface required by the invoice rpc server.
type GraphSource interface {
	// FetchChannelEdgesByID attempts to look up the two directed edges for
	// the channel identified by the channel ID. If the channel can't be
	// found, then graphdb.ErrEdgeNotFound is returned.
	FetchChannelEdgesByID(v lnwire.GossipVersion, chanID uint64) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	IsAdvertisedNode(pubKey [33]byte) (bool, error)
}
