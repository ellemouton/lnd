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

type V1Store interface {
	// Node table related methods.

	// Done.
	AddLightningNode(node *models.LightningNode,
		op ...batch.SchedulerOption) error

	// Done.
	AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error)

	// Done.
	ForEachNodeDirectedChannel(nodePub route.Vertex,
		cb func(channel *DirectedChannel) error) error

	// Done: need a unit test for this though.
	ForEachSourceNodeChannel(cb func(chanPoint wire.OutPoint,
		havePolicy bool, otherNode *models.LightningNode) error) error

	// Done
	ForEachNodeChannel(nodePub route.Vertex,
		cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	// Done.
	FetchNodeFeatures(nodePub route.Vertex) (
		*lnwire.FeatureVector, error)

	// Done.
	ForEachNodeCached(cb func(node route.Vertex,
		chans map[uint64]*DirectedChannel) error) error

	// Done.
	ForEachNode(cb func(tx NodeRTx) error) error

	// Done.
	ForEachNodeCacheable(cb func(route.Vertex,
		*lnwire.FeatureVector) error) error

	// Done.
	LookupAlias(pub *btcec.PublicKey) (string, error)

	// Done.
	DeleteLightningNode(nodePub route.Vertex) error

	// Done.
	NodeUpdatesInHorizon(startTime,
		endTime time.Time) ([]models.LightningNode, error)

	// Done.
	FetchLightningNode(nodePub route.Vertex) (
		*models.LightningNode, error)

	// Done.
	HasLightningNode(nodePub [33]byte) (time.Time, bool,
		error)

	// Done.
	IsPublicNode(pubKey [33]byte) (bool, error)

	// Done.
	GraphSession(cb func(graph NodeTraverser) error) error

	// Channel table methods
	// Done.
	ForEachChannel(cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	// Done
	DisabledChannelIDs() ([]uint64, error)

	// Done.
	AddChannelEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	// Done.
	HasChannelEdge(chanID uint64) (time.Time, time.Time, bool, bool,
		error)

	// Done.
	DeleteChannelEdges(strictZombiePruning, markZombie bool,
		chanIDs ...uint64) ([]*models.ChannelEdgeInfo, error)

	// DONE: TODO(elle): add test for this.
	AddEdgeProof(chanID lnwire.ShortChannelID,
		proof *models.ChannelAuthProof) error

	// Done.
	ChannelID(chanPoint *wire.OutPoint) (uint64, error)

	// Done.
	HighestChanID() (uint64, error)

	// Done.
	ChanUpdatesInHorizon(startTime, endTime time.Time) ([]ChannelEdge,
		error)

	// Done.
	FilterKnownChanIDs(chansInfo []ChannelUpdateInfo) ([]uint64,
		[]ChannelUpdateInfo, error)

	// Done.
	FilterChannelRange(startHeight, endHeight uint32, withTimestamps bool) (
		[]BlockChannelRange, error)

	// Done.
	FetchChanInfos(chanIDs []uint64) ([]ChannelEdge, error)

	// Done.
	FetchChannelEdgesByOutpoint(op *wire.OutPoint) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// Done.
	FetchChannelEdgesByID(chanID uint64) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// Done.
	ChannelView() ([]EdgePoint, error)

	// Zombie table.
	// Done.
	MarkEdgeZombie(chanID uint64,
		pubKey1, pubKey2 [33]byte) error
	// Done.
	MarkEdgeLive(chanID uint64) error
	// Done.
	IsZombieEdge(chanID uint64) (bool, [33]byte, [33]byte)
	// Done.
	NumZombies() (uint64, error)

	// Closed channel table.
	// Done.
	PutClosedScid(scid lnwire.ShortChannelID) error
	// Done.
	IsClosedScid(scid lnwire.ShortChannelID) (bool, error)

	// Channel Updates
	// Done.
	UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) (route.Vertex, route.Vertex, error)

	// Source node table.
	// Done.
	SourceNode() (*models.LightningNode, error)
	// Done.
	SetSourceNode(node *models.LightningNode) error

	// Prune log table.
	// Done.
	PruneTip() (*chainhash.Hash, uint32, error)

	// Done.
	PruneGraphNodes() ([]route.Vertex, error)

	// Done.
	PruneGraph(spentOutputs []*wire.OutPoint,
		blockHash *chainhash.Hash, blockHeight uint32) (
		[]*models.ChannelEdgeInfo, []route.Vertex, error)

	// Done.
	DisconnectBlockAtHeight(height uint32) ([]*models.ChannelEdgeInfo,
		error)
}
