package routing

import (
	"context"

	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// RoutingGraph is an abstract interface that provides information about nodes
// and edges to pathfinding.
type RoutingGraph interface {
	// ForEachNodeChannel calls the callback for every channel of the given
	// node.
	ForEachNodeChannel(ctx context.Context, nodePub route.Vertex,
		cb func(channel *models.DirectedChannel) error) error

	// FetchNodeFeatures returns the features of the given node.
	FetchNodeFeatures(ctx context.Context,
		nodePub route.Vertex) (*lnwire.FeatureVector, error)
}
