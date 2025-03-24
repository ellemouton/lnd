package graphdb

import (
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

// NodeRTx represents transaction object with an underlying node associated that
// can be used to make further queries to the graph under the same transaction.
// This is useful for consistency during graph traversal and queries.
type NodeRTx interface {
	// Node returns the raw information of the node.
	Node() *models.LightningNode

	// ForEachChannel can be used to iterate over the node's channels under
	// the same transaction used to fetch the node.
	ForEachChannel(func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	// FetchNode fetches the node with the given pub key under the same
	// transaction used to fetch the current node. The returned node is also
	// a NodeRTx and any operations on that NodeRTx will also be done under
	// the same transaction.
	FetchNode(node route.Vertex) (NodeRTx, error)
}

// NodeTraverser is an abstract read only interface that provides information
// about nodes and their edges. The interface is about providing fast read-only
// access to the graph and so if a cache is available, it should be used.
type NodeTraverser interface {
	// ForEachNodeDirectedChannel calls the callback for every channel of
	// the given node.
	ForEachNodeDirectedChannel(nodePub route.Vertex,
		cb func(channel *DirectedChannel) error) error

	// FetchNodeFeatures returns the features of the given node.
	FetchNodeFeatures(nodePub route.Vertex) (*lnwire.FeatureVector, error)
}

// V1Store is an interface that describes the persistence layer for the graph
// as advertised by the V1 gossip protocol.
type V1Store interface {
	// HighestChanID returns the "highest" known channel ID in the channel
	// graph. This represents the "newest" channel from the PoV of the
	// chain. This method can be used by peers to quickly determine if
	// they're graphs are in sync.
	HighestChanID() (uint64, error)

	// ChanUpdatesInHorizon returns all the known channel edges which have
	// at least one edge that has an update timestamp within the specified
	// horizon.
	ChanUpdatesInHorizon(startTime, endTime time.Time) ([]ChannelEdge,
		error)

	// NodeUpdatesInHorizon returns all the known lightning node which have
	// an update timestamp within the passed range. This method can be used
	// by two nodes to quickly determine if they have the same set of up to
	// date node announcements.
	NodeUpdatesInHorizon(startTime,
		endTime time.Time) ([]models.LightningNode, error)

	// IsPublicNode is a helper method that determines whether the node with
	// the given public key is seen as a public node in the graph from the
	// graph's source node's point of view.
	IsPublicNode(pubKey [33]byte) (bool, error)

	// FilterKnownChanIDs takes a set of channel IDs and return the subset
	// of chan ID's that we don't know and are not known zombies of the
	// passed set. In other words, we perform a set difference of our set of
	// chan ID's and the ones passed in. This method can be used by callers
	// to determine the set of channels another peer knows of that we don't.
	// The ChannelUpdateInfos for the known zombies is also returned.
	FilterKnownChanIDs(chansInfo []ChannelUpdateInfo) ([]uint64,
		[]ChannelUpdateInfo, error)

	// FilterChannelRange returns the channel ID's of all known channels
	// which were mined in a block height within the passed range. The
	// channel IDs are grouped by their common block height. This method can
	// be used to quickly share with a peer the set of channels we know of
	// within a particular range to catch them up after a period of time
	// offline. If withTimestamps is true then the timestamp info of the
	// latest received channel update messages of the channel will be
	// included in the response.
	FilterChannelRange(startHeight, endHeight uint32,
		withTimestamps bool) ([]BlockChannelRange, error)

	// FetchChanInfos returns the set of channel edges that correspond to
	// the passed channel ID's. If an edge is the query is unknown to the
	// database, it will skipped and the result will contain only those
	// edges that exist at the time of the query. This can be used to
	// respond to peer queries that are seeking to fill in gaps in their
	// view of the channel graph.
	FetchChanInfos(chanIDs []uint64) ([]ChannelEdge, error)

	// FetchChannelEdgesByID attempts to lookup the two directed edges for
	// the channel identified by the channel ID. If the channel can't be
	// found, then ErrEdgeNotFound is returned. A struct which houses the
	// general information for the channel itself is returned as well as two
	// structs that contain the routing policies for the channel in either
	// direction.
	//
	// ErrZombieEdge an be returned if the edge is currently marked as a
	// zombie within the database. In this case, the ChannelEdgePolicy's
	// will be nil, and the ChannelEdgeInfo will only include the public
	// keys of each node.
	FetchChannelEdgesByID(chanID uint64) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// FetchChannelEdgesByOutpoint attempts to lookup the two directed edges
	// for the channel identified by the funding outpoint. If the channel
	// can't be found, then ErrEdgeNotFound is returned. A struct which
	// houses the general information for the channel itself is returned as
	// well as two structs that contain the routing policies for the channel
	// in either direction.
	FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// ForEachNode iterates through all the stored vertices/nodes in the
	// graph, executing the passed callback with each node encountered. If
	// the callback returns an error, then the transaction is aborted and
	// the iteration stops early. Any operations performed on the NodeTx
	// passed to the call-back are executed under the same read transaction
	// and so, methods on the NodeTx object _MUST_ only be called from
	// within the call-back.
	ForEachNode(cb func(tx NodeRTx) error) error

	// ForEachNodeCacheable iterates through all the stored vertices/nodes
	// in the graph, executing the passed callback with each node
	// encountered. If the callback returns an error, then the transaction
	// is aborted and the iteration stops early.
	ForEachNodeCacheable(cb func(route.Vertex,
		*lnwire.FeatureVector) error) error

	// ForEachChannel can be used to iterate over the node's channels under
	// the same transaction used to fetch the node.
	ForEachChannel(f func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	// ForEachNodeDirectedChannel iterates through all channels of a given
	// node, executing the passed callback on the directed edge representing
	// the channel and its incoming policy. If the callback returns an
	// error, then the iteration is halted with the error propagated back
	// up to the caller.
	//
	// Unknown policies are passed into the callback as nil values.
	ForEachNodeDirectedChannel(nodePub route.Vertex,
		cb func(channel *DirectedChannel) error) error

	// FetchNodeFeatures returns the features of the given node.
	FetchNodeFeatures(nodePub route.Vertex) (*lnwire.FeatureVector, error)

	AddChannelEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	GraphSession(cb func(graph NodeTraverser) error) error

	ForEachNodeCached(cb func(node route.Vertex,
		chans map[uint64]*DirectedChannel) error) error

	AddLightningNode(node *models.LightningNode,
		op ...batch.SchedulerOption) error

	DeleteLightningNode(nodePub route.Vertex) error

	MarkEdgeLive(chanID uint64) error

	DeleteChannelEdges(strictZombiePruning, markZombie bool,
		chanIDs ...uint64) ([]*models.ChannelEdgeInfo, error)

	DisconnectBlockAtHeight(height uint32) (
		[]*models.ChannelEdgeInfo, error)

	PruneGraph(spentOutputs []*wire.OutPoint,
		blockHash *chainhash.Hash, blockHeight uint32) (
		[]*models.ChannelEdgeInfo, []route.Vertex, error)

	PruneGraphNodes() ([]route.Vertex, error)

	MarkEdgeZombie(chanID uint64,
		pubKey1, pubKey2 [33]byte) error

	UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) (route.Vertex, route.Vertex, error)

	HasLightningNode(nodePub [33]byte) (time.Time, bool, error)

	SourceNode() (*models.LightningNode, error)

	ChannelID(chanPoint *wire.OutPoint) (uint64, error)

	LookupAlias(pub *btcec.PublicKey) (string, error)

	FetchLightningNode(nodePub route.Vertex) (
		*models.LightningNode, error)

	ForEachNodeChannel(nodePub route.Vertex,
		cb func(*models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	NumZombies() (uint64, error)

	SetSourceNode(node *models.LightningNode) error

	ForEachSelfNodeChannel(cb func(chanPoint wire.OutPoint,
		havePolicy bool, otherNode *models.LightningNode) error) error

	PutClosedScid(scid lnwire.ShortChannelID) error
	IsClosedScid(lnwire.ShortChannelID) (bool, error)

	PruneTip() (*chainhash.Hash, uint32, error)
	ChannelView() ([]EdgePoint, error)
	DisabledChannelIDs() ([]uint64, error)
	HasChannelEdge(chanID uint64) (time.Time, time.Time, bool, bool, error)
	AddEdgeProof(chanID lnwire.ShortChannelID,
		proof *models.ChannelAuthProof) error

	AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr,
		error)

	IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte)
}
