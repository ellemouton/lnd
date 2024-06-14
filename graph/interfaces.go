package graph

import (
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type Graph interface {
	// IsStaleEdgePolicy returns true if the graph source has a channel
	// edge for the passed channel ID (and flags) that have a more recent
	// timestamp.
	IsStaleEdgePolicy(chanID lnwire.ShortChannelID, timestamp time.Time,
		flags lnwire.ChanUpdateChanFlags) bool
}

type GraphDB interface {
	PruneTip() (*chainhash.Hash, uint32, error)

	PruneGraph(spentOutputs []*wire.OutPoint, blockHash *chainhash.Hash,
		blockHeight uint32) ([]*models.ChannelEdgeInfo, error)

	ChannelView() ([]channeldb.EdgePoint, error)

	PruneGraphNodes() error

	SourceNode() (*channeldb.LightningNode, error)

	DisabledChannelIDs() ([]uint64, error)

	FetchChanInfos(chanIDs []uint64) ([]channeldb.ChannelEdge, error)

	ChanUpdatesInHorizon(startTime, endTime time.Time) (
		[]channeldb.ChannelEdge, error)

	DeleteChannelEdges(strictZombiePruning, markZombie bool,
		chanIDs ...uint64) error

	DisconnectBlockAtHeight(height uint32) ([]*models.ChannelEdgeInfo,
		error)

	HasChannelEdge(chanID uint64) (time.Time, time.Time, bool, bool, error)

	FetchChannelEdgesByID(chanID uint64) (*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error)

	AddLightningNode(node *channeldb.LightningNode,
		op ...batch.SchedulerOption) error

	AddChannelEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	MarkEdgeZombie(chanID uint64, pubKey1, pubKey2 [33]byte) error

	UpdateEdgePolicy(edge *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) error

	HasLightningNode(nodePub [33]byte) (time.Time, bool, error)

	FetchLightningNode(tx kvdb.RTx, nodePub route.Vertex) (
		*channeldb.LightningNode, error)

	ForEachNode(cb func(kvdb.RTx, *channeldb.LightningNode) error) error

	ForEachNodeChannel(tx kvdb.RTx, nodePub route.Vertex,
		cb func(kvdb.RTx, *models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	UpdateChannelEdge(edge *models.ChannelEdgeInfo) error

	IsPublicNode(pubKey [33]byte) (bool, error)

	MarkEdgeLive(chanID uint64) error
}

// ChannelGraphSource represents the source of information about the topology
// of the lightning network. It's responsible for the addition of nodes, edges,
// applying edge updates, and returning the current block height with which the
// topology is synchronized.
type ChannelGraphSource interface {
	// AddNode is used to add information about a node to the router
	// database. If the node with this pubkey is not present in an existing
	// channel, it will be ignored.
	AddNode(node *channeldb.LightningNode,
		op ...batch.SchedulerOption) error

	// AddEdge is used to add edge/channel to the topology of the router,
	// after all information about channel will be gathered this
	// edge/channel might be used in construction of payment path.
	AddEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	// AddProof updates the channel edge info with proof which is needed to
	// properly announce the edge to the rest of the network.
	AddProof(chanID lnwire.ShortChannelID,
		proof *models.ChannelAuthProof) error

	// UpdateEdge is used to update edge information, without this message
	// edge considered as not fully constructed.
	UpdateEdge(policy *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) error

	// IsStaleNode returns true if the graph source has a node announcement
	// for the target node with a more recent timestamp. This method will
	// also return true if we don't have an active channel announcement for
	// the target node.
	IsStaleNode(node route.Vertex, timestamp time.Time) bool

	// IsPublicNode determines whether the given vertex is seen as a public
	// node in the graph from the graph's source node's point of view.
	IsPublicNode(node route.Vertex) (bool, error)

	// IsKnownEdge returns true if the graph source already knows of the
	// passed channel ID either as a live or zombie edge.
	IsKnownEdge(chanID lnwire.ShortChannelID) bool

	// MarkEdgeLive clears an edge from our zombie index, deeming it as
	// live.
	MarkEdgeLive(chanID lnwire.ShortChannelID) error

	// ForAllOutgoingChannels is used to iterate over all channels
	// emanating from the "source" node which is the center of the
	// star-graph.
	ForAllOutgoingChannels(cb func(tx kvdb.RTx,
		c *models.ChannelEdgeInfo,
		e *models.ChannelEdgePolicy) error) error

	// CurrentBlockHeight returns the block height from POV of the router
	// subsystem.
	CurrentBlockHeight() (uint32, error)

	// GetChannelByID return the channel by the channel id.
	GetChannelByID(chanID lnwire.ShortChannelID) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// FetchLightningNode attempts to look up a target node by its identity
	// public key. channeldb.ErrGraphNodeNotFound is returned if the node
	// doesn't exist within the graph.
	FetchLightningNode(route.Vertex) (*channeldb.LightningNode, error)

	// ForEachNode is used to iterate over every node in the known graph.
	ForEachNode(func(node *channeldb.LightningNode) error) error
}
