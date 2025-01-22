package graphdb

import (
	"context"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

//type GraphReads interface {
//	GraphSessionFactory
//	invoicesrpc.GraphSource
//}

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

type Source interface {
	AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error)

	ForEachChannel(cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error

	FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	HasLightningNode(nodePub [33]byte) (time.Time, bool, error)

	FetchLightningNode(nodePub route.Vertex) (*models.LightningNode, error)

	ForEachNode(cb func(*models.LightningNode) error) error

	ForEachNodeChannel(ctx context.Context, nodePub route.Vertex,
		cb func(*models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	IsPublicNode(_ context.Context, pubKey [33]byte) (bool,
		error)

	FetchChannelEdgesByID(_ context.Context,
		chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	NumZombies() (uint64, error)

	ForEachNodeCached(cb func(node route.Vertex,
		chans map[uint64]*models.DirectedChannel) error) error

	NewRoutingGraphSession() (RoutingGraph, func() error,
		error)

	NewRoutingGraph() RoutingGraph

	ForEachNodeWithTx(ctx context.Context,
		cb func(NodeTx) error) error
}

// DB is an interface describing a persisted Lightning Network graph.
//
//nolint:interfacebloat
type DB interface {
	// PruneTip returns the block height and hash of the latest block that
	// has been used to prune channels in the graph. Knowing the "prune tip"
	// allows callers to tell if the graph is currently in sync with the
	// current best known UTXO state.
	PruneTip() (*chainhash.Hash, uint32, error)

	SetSourceNode(node *models.LightningNode) error

	AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error)

	// PruneGraph prunes newly closed channels from the channel graph in
	// response to a new block being solved on the network. Any transactions
	// which spend the funding output of any known channels within the graph
	// will be deleted. Additionally, the "prune tip", or the last block
	// which has been used to prune the graph is stored so callers can
	// ensure the graph is fully in sync with the current UTXO state. A
	// slice of channels that have been closed by the target block are
	// returned if the function succeeds without error.
	PruneGraph(spentOutputs []*wire.OutPoint, blockHash *chainhash.Hash,
		blockHeight uint32, nodeCB func(route.Vertex),
		edgeCB func(*models.ChannelEdgeInfo)) ([]*models.ChannelEdgeInfo, error)

	FilterChannelRange(startHeight, endHeight uint32,
		withTimestamps bool) ([]BlockChannelRange, error)

	// ChannelView returns the verifiable edge information for each active
	// channel within the known channel graph. The set of UTXO's (along with
	// their scripts) returned are the ones that need to be watched on
	// chain to detect channel closes on the resident blockchain.
	ChannelView() ([]models.EdgePoint, error)

	// PruneGraphNodes is a garbage collection method which attempts to
	// prune out any nodes from the channel graph that are currently
	// unconnected. This ensure that we only maintain a graph of reachable
	// nodes. In the event that a pruned node gains more channels, it will
	// be re-added back to the graph.
	PruneGraphNodes(func(node route.Vertex)) error

	// SourceNode returns the source node of the graph. The source node is
	// treated as the center node within a star-graph. This method may be
	// used to kick off a path finding algorithm in order to explore the
	// reachability of another node based off the source node.
	SourceNode() (*models.LightningNode, error)

	// DisabledChannelIDs returns the channel ids of disabled channels.
	// A channel is disabled when two of the associated ChanelEdgePolicies
	// have their disabled bit on.
	DisabledChannelIDs() ([]uint64, error)

	// FetchChanInfos returns the set of channel edges that correspond to
	// the passed channel ID's. If an edge is the query is unknown to the
	// database, it will skipped and the result will contain only those
	// edges that exist at the time of the query. This can be used to
	// respond to peer queries that are seeking to fill in gaps in their
	// view of the channel graph.
	FetchChanInfos(chanIDs []uint64) ([]ChannelEdge, error)

	// ChanUpdatesInHorizon returns all the known channel edges which have
	// at least one edge that has an update timestamp within the specified
	// horizon.
	ChanUpdatesInHorizon(startTime, endTime time.Time) (
		[]ChannelEdge, error)

	// DeleteChannelEdges removes edges with the given channel IDs from the
	// database and marks them as zombies. This ensures that we're unable to
	// re-add it to our database once again. If an edge does not exist
	// within the database, then ErrEdgeNotFound will be returned. If
	// strictZombiePruning is true, then when we mark these edges as
	// zombies, we'll set up the keys such that we require the node that
	// failed to send the fresh update to be the one that resurrects the
	// channel from its zombie state. The markZombie bool denotes whether
	// to mark the channel as a zombie.
	DeleteChannelEdges(strictZombiePruning, markZombie bool,
		edgeCB func(*models.ChannelEdgeInfo), chanIDs ...uint64) error

	// DisconnectBlockAtHeight is used to indicate that the block specified
	// by the passed height has been disconnected from the main chain. This
	// will "rewind" the graph back to the height below, deleting channels
	// that are no longer confirmed from the graph. The prune log will be
	// set to the last prune height valid for the remaining chain.
	// Channels that were removed from the graph resulting from the
	// disconnected block are returned.
	DisconnectBlockAtHeight(height uint32) ([]*models.ChannelEdgeInfo,
		error)

	// HasChannelEdge returns true if the database knows of a channel edge
	// with the passed channel ID, and false otherwise. If an edge with that
	// ID is found within the graph, then two time stamps representing the
	// last time the edge was updated for both directed edges are returned
	// along with the boolean. If it is not found, then the zombie index is
	// checked and its result is returned as the second boolean.
	HasChannelEdge(chanID uint64) (time.Time, time.Time, bool, bool, error)

	// FetchChannelEdgesByID attempts to lookup the two directed edges for
	// the channel identified by the channel ID. If the channel can't be
	// found, then ErrEdgeNotFound is returned. A struct which houses the
	// general information for the channel itself is returned as well as
	// two structs that contain the routing policies for the channel in
	// either direction.
	//
	// ErrZombieEdge an be returned if the edge is currently marked as a
	// zombie within the database. In this case, the ChannelEdgePolicy's
	// will be nil, and the ChannelEdgeInfo will only include the public
	// keys of each node.
	FetchChannelEdgesByID(ctx context.Context, chanID uint64) (*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error)

	// AddLightningNode adds a vertex/node to the graph database. If the
	// node is not in the database from before, this will add a new,
	// unconnected one to the graph. If it is present from before, this will
	// update that node's information. Note that this method is expected to
	// only be called to update an already present node from a node
	// announcement, or to insert a node found in a channel update.
	AddLightningNode(node *models.LightningNode,
		op ...batch.SchedulerOption) error

	// AddChannelEdge adds a new (undirected, blank) edge to the graph
	// database. An undirected edge from the two target nodes are created.
	// The information stored denotes the static attributes of the channel,
	// such as the channelID, the keys involved in creation of the channel,
	// and the set of features that the channel supports. The chanPoint and
	// chanID are used to uniquely identify the edge globally within the
	// database.
	AddChannelEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	// MarkEdgeZombie attempts to mark a channel identified by its channel
	// ID as a zombie. This method is used on an ad-hoc basis, when channels
	// need to be marked as zombies outside the normal pruning cycle.
	MarkEdgeZombie(chanID uint64, pubKey1, pubKey2 [33]byte) error

	// UpdateEdgePolicy updates the edge routing policy for a single
	// directed edge within the database for the referenced channel. The
	// `flags` attribute within the ChannelEdgePolicy determines which of
	// the directed edges are being updated. If the flag is 1, then the
	// first node's information is being updated, otherwise it's the second
	// node's information. The node ordering is determined by the
	// lexicographical ordering of the identity public keys of the nodes on
	// either side of the channel.
	UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
		cb func(fromNode, toNode route.Vertex, isUpdate1 bool),
		op ...batch.SchedulerOption) error

	// HasLightningNode determines if the graph has a vertex identified by
	// the target node identity public key. If the node exists in the
	// database, a timestamp of when the data for the node was lasted
	// updated is returned along with a true boolean. Otherwise, an empty
	// time.Time is returned with a false boolean.
	HasLightningNode(nodePub [33]byte) (time.Time, bool, error)

	// FetchLightningNode attempts to look up a target node by its identity
	// public key. If the node isn't found in the database, then
	// ErrGraphNodeNotFound is returned.
	FetchLightningNode(nodePub route.Vertex) (*models.LightningNode,
		error)

	// ForEachNodeChannel iterates through all channels of the given node,
	// executing the passed callback with an edge info structure and the
	// policies of each end of the channel. The first edge policy is the
	// outgoing edge *to* the connecting node, while the second is the
	// incoming edge *from* the connecting node. If the callback returns an
	// error, then the iteration is halted with the error propagated back up
	// to the caller.
	//
	// Unknown policies are passed into the callback as nil values.
	ForEachNodeChannel(ctx context.Context, nodePub route.Vertex, cb func(
		*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	ForEachChannel(cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	ForEachNode(cb func(*models.LightningNode) error) error

	ForEachNodeCacheable(cb func(GraphCacheNode) error) error

	ForEachNodeCached(cb func(node route.Vertex,
		chans map[uint64]*models.DirectedChannel) error) error

	FilterKnownChanIDs(chansInfo map[uint64]ChannelUpdateInfo) ([]uint64, error)

	IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte, error)

	ChannelID(chanPoint *wire.OutPoint) (uint64, error)

	HighestChanID() (uint64, error)

	NodeUpdatesInHorizon(startTime,
		endTime time.Time) ([]models.LightningNode, error)

	FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	ForEachNodeWithTx(ctx context.Context, cb func(NodeTx) error) error

	// UpdateChannelEdge retrieves and update edge of the graph database.
	// Method only reserved for updating an edge info after its already been
	// created. In order to maintain this constraints, we return an error in
	// the scenario that an edge info hasn't yet been created yet, but
	// someone attempts to update it.
	UpdateChannelEdge(edge *models.ChannelEdgeInfo) error

	// IsPublicNode is a helper method that determines whether the node with
	// the given public key is seen as a public node in the graph from the
	// graph's source node's point of view.
	IsPublicNode(_ context.Context, pubKey [33]byte) (bool, error)

	// MarkEdgeLive clears an edge from our zombie index, deeming it as
	// live.
	MarkEdgeLive(chanID uint64, db func(ChannelEdge)) error

	NumZombies() (uint64, error)

	GraphSessionFactory
}

// GraphSessionFactory can be used to produce a new Graph instance which can
// then be used for a path-finding session. Depending on the implementation,
// the Graph session will represent a DB connection where a read-lock is being
// held across calls to the backing Graph.
type GraphSessionFactory interface {
	// NewRoutingGraphSession will produce a new Graph to use for a
	// path-finding session. It returns the Graph along with a call-back
	// that must be called once Graph access is complete. This call-back
	// will close any read-only transaction that was created at Graph
	// construction time.
	NewRoutingGraphSession() (RoutingGraph, func() error, error)

	// NewRoutingGraph creates a new RoutingGraph instance without any
	// underlying read-lock. This method should be used when the caller does
	// not need to hold a read-lock across multiple calls to the underlying
	// graph source.
	NewRoutingGraph() RoutingGraph
}

type NodeTx interface {
	ForEachNodeChannel(ctx context.Context,
		nodePub route.Vertex, cb func(context.Context,
			*models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	FetchLightningNode(ctx context.Context, nodePub route.Vertex) (NodeTx,
		error)

	Node() *models.LightningNode
}
