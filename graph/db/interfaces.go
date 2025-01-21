package graphdb

import (
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// RoutingGraph is an abstract interface that provides information about nodes
// and edges to pathfinding.
type RoutingGraph interface {
	// ForEachNodeChannel calls the callback for every channel of the given
	// node.
	ForEachNodeChannel(nodePub route.Vertex,
		cb func(channel *DirectedChannel) error) error

	// FetchNodeFeatures returns the features of the given node.
	FetchNodeFeatures(nodePub route.Vertex) (*lnwire.FeatureVector, error)
}
