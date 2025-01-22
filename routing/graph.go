package routing

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// FetchAmountPairCapacity determines the maximal public capacity between two
// nodes depending on the amount we try to send.
func FetchAmountPairCapacity(graph graphdb.RoutingGraph, source, nodeFrom,
	nodeTo route.Vertex, amount lnwire.MilliSatoshi) (btcutil.Amount,
	error) {

	ctx := context.Background()

	// Create unified edges for all incoming connections.
	//
	// Note: Inbound fees are not used here because this method is only used
	// by a deprecated router rpc.
	u := newNodeEdgeUnifier(source, nodeTo, false, nil)

	err := u.addGraphPolicies(ctx, graph)
	if err != nil {
		return 0, err
	}

	edgeUnifier, ok := u.edgeUnifiers[nodeFrom]
	if !ok {
		return 0, fmt.Errorf("no edge info for node pair %v -> %v",
			nodeFrom, nodeTo)
	}

	edge := edgeUnifier.getEdgeNetwork(amount, 0)
	if edge == nil {
		return 0, fmt.Errorf("no edge for node pair %v -> %v "+
			"(amount %v)", nodeFrom, nodeTo, amount)
	}

	return edge.capacity, nil
}
