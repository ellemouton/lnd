package graph

import (
	"context"

	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChannelGraphSource represents the source of information about the topology
// of the lightning network. It's responsible for the addition of nodes, edges,
// applying edge updates, and returning the current block height with which the
// topology is synchronized.
type ChannelGraphSource interface {
	// AddNode is used to add information about a node to the router
	// database. If the node with this pubkey is not present in an existing
	// channel, it will be ignored.
	AddNode(ctx context.Context, node *models.Node,
		op ...batch.SchedulerOption) error

	// AddEdge is used to add edge/channel to the topology of the router,
	// after all information about channel will be gathered this
	// edge/channel might be used in construction of payment path.
	AddEdge(ctx context.Context, edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	// UpdateEdge is used to update edge information, without this message
	// edge considered as not fully constructed.
	UpdateEdge(ctx context.Context, policy *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) error

	// IsStaleNode returns true if the graph source has a node announcement
	// for the target node/version that is at least as fresh as the passed
	// announcement. This method will also return true if we don't have an
	// active channel announcement for the target node/version.
	IsStaleNode(ctx context.Context, v lnwire.GossipVersion,
		nodePub route.Vertex, updateTimestamp lnwire.Timestamp) bool

	// IsStaleEdgePolicy returns true if the graph source has a channel
	// edge for the passed policy that has a more recent announcement.
	IsStaleEdgePolicy(policy *models.ChannelEdgePolicy) bool

	// CurrentBlockHeight returns the block height from POV of the router
	// subsystem.
	CurrentBlockHeight() (uint32, error)
}
