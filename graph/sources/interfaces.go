package sources

import (
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/graph/session"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"github.com/lightningnetwork/lnd/netann"
)

// GraphSource defines the read-only graph interface required by LND for graph
// related queries.
type GraphSource interface {
	session.ReadOnlyGraph
	invoicesrpc.GraphSource
	netann.ChannelGraph
	channeldb.AddrSource
}
