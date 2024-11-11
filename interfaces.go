package lnd

import (
	"github.com/lightningnetwork/lnd/graph/graphsession"
	"github.com/lightningnetwork/lnd/graph/sources"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
)

// GraphSource defines the read-only graph interface required by LND for graph
// related queries.
type GraphSource interface {
	graphsession.ReadOnlyGraph
	invoicesrpc.GraphSource
}

// A compile-time check to ensure that sources.ChanGraphSource implements the
// GraphSource interface.
var _ GraphSource = (*sources.ChanGraphSource)(nil)
