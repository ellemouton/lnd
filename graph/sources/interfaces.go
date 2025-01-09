package sources

import (
	"bytes"
	"context"
	"errors"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/netann"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/routing/route"
)

// GraphSource defines the read-only graph interface required by LND for graph
// related queries.
//
//nolint:interfacebloat
type GraphSource interface {
	routing.GraphSessionFactory
	netann.ChannelGraph

	SourceNode(ctx context.Context) (*models.LightningNode, error)

	// FetchChannelEdgesByID attempts to look up the two directed edges for
	// the channel identified by the channel ID. If the channel can't be
	// found, then graphdb.ErrEdgeNotFound is returned.
	// (For invoices rpc).
	FetchChannelEdgesByID(ctx context.Context, chanID uint64) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// ForEachChannel iterates through all the channel edges stored within
	// the graph and invokes the passed callback for each edge. If the
	// callback returns an error, then the transaction is aborted and the
	// iteration stops early. An edge's policy structs may be nil if the
	// ChannelUpdate in question has not yet been received for the channel.
	ForEachChannel(ctx context.Context, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	// ForEachNode iterates through all the stored vertices/nodes in the
	// graph, executing the passed callback with each node encountered. If
	// the callback returns an error, then the transaction is aborted and
	// the iteration stops early.
	ForEachNode(ctx context.Context, cb func(NodeTx) error) error

	// ForEachNodeChannel iterates through all channels of the given node,
	// executing the passed callback with an edge info structure and the
	// policies of each end of the channel. The first edge policy is the
	// outgoing edge *to* the connecting node, while the second is the
	// incoming edge *from* the connecting node. If the callback returns an
	// error, then the iteration is halted with the error propagated back up
	// to the caller. Unknown policies are passed into the callback as nil
	// values.
	ForEachNodeChannel(ctx context.Context,
		nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	// FetchLightningNode attempts to look up a target node by its identity
	// public key. If the node isn't found in the database, then
	// graphdb.ErrGraphNodeNotFound is returned.
	FetchLightningNode(ctx context.Context, nodePub route.Vertex) (
		*models.LightningNode, error)

	// NumZombies returns the number of channels that the GraphSource
	// considers to be zombies.
	NumZombies(ctx context.Context) (uint64, error)

	// ForEachNodeCached is similar to ForEachNode, but it utilizes the
	// channel graph cache instead if one is available. Note that this also
	// doesn't return all the information the regular ForEachNode method
	// does.
	ForEachNodeCached(ctx context.Context, cb func(node route.Vertex,
		chans map[uint64]*graphdb.DirectedChannel) error) error
}

type NodeTx interface {
	ForEachNodeChannel(ctx context.Context,
		nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
			*models.ChannelEdgePolicy,
			*models.ChannelEdgePolicy) error) error

	FetchLightningNode(ctx context.Context, nodePub route.Vertex) (NodeTx,
		error)

	Node() *models.LightningNode
}

type GraphUtils interface {
	GraphSource

	AddrsForNode(ctx context.Context, nodePub *btcec.PublicKey) (bool,
		[]net.Addr, error)

	HasLightningNode(ctx context.Context, nodePub [33]byte) (time.Time,
		bool, error)

	LookupAlias(ctx context.Context, pub *btcec.PublicKey) (string, error)

	IsPublicNode(ctx context.Context, pubKey [33]byte) (bool, error)
}

type graphUtils struct {
	GraphSource
}

// NewGraphUtils creates a new instance of the GraphUtils.
func NewGraphUtils(gs GraphSource) GraphUtils {
	return &graphUtils{
		GraphSource: gs,
	}
}

func (u *graphUtils) AddrsForNode(ctx context.Context,
	nodePub *btcec.PublicKey) (bool, []net.Addr, error) {

	pubKey, err := route.NewVertexFromBytes(nodePub.SerializeCompressed())
	if err != nil {
		return false, nil, err
	}

	node, err := u.FetchLightningNode(ctx, pubKey)
	// We don't consider it an error if the graph is unaware of the node.
	switch {
	case err != nil && !errors.Is(err, graphdb.ErrGraphNodeNotFound):
		return false, nil, err

	case errors.Is(err, graphdb.ErrGraphNodeNotFound):
		return false, nil, nil
	}

	return true, node.Addresses, nil

}

// HasLightningNode determines if the graph has a vertex identified by
// the target node identity public key. If the node exists in the
// database, a timestamp of when the data for the node was lasted
// updated is returned along with a true boolean. Otherwise, an empty
// time.Time is returned with a false boolean.
func (u *graphUtils) HasLightningNode(ctx context.Context, nodePub [33]byte) (
	time.Time, bool, error) {

	node, err := u.FetchLightningNode(ctx, nodePub)
	if errors.Is(err, graphdb.ErrGraphNodeNotFound) {
		return time.Time{}, false, nil
	} else if err != nil {
		return time.Time{}, false, err
	}

	return node.LastUpdate, true, nil
}

// LookupAlias attempts to return the alias as advertised by the target
// node. graphdb.ErrNodeAliasNotFound is returned if the alias is not
// found.
func (u *graphUtils) LookupAlias(ctx context.Context,
	pub *btcec.PublicKey) (string, error) {

	node, err := u.FetchLightningNode(ctx, route.NewVertex(pub))
	if errors.Is(err, graphdb.ErrGraphNodeNotFound) {
		return "", graphdb.ErrNodeAliasNotFound
	} else if err != nil {
		return "", err
	}

	return node.Alias, nil
}

// IsPublicNode is a helper method that determines whether the node with
// the given public key is seen as a public node in the graph from the
// graph's source node's point of view.
func (u *graphUtils) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool,
	error) {

	sourceNode, err := u.SourceNode(ctx)
	if err != nil {
		return false, err
	}

	var (
		sourcePubKey = sourceNode.PubKeyBytes[:]
		nodeIsPublic bool
		errDone      = errors.New("done")
	)
	err = u.ForEachNodeChannel(ctx, pubKey, func(
		info *models.ChannelEdgeInfo, _,
		_ *models.ChannelEdgePolicy) error {

		// If this edge doesn't extend to the source node, we'll
		// terminate our search as we can now conclude that the node is
		// publicly advertised within the graph due to the local node
		// knowing of the current edge.
		if !bytes.Equal(info.NodeKey1Bytes[:], sourcePubKey) &&
			!bytes.Equal(info.NodeKey2Bytes[:], sourcePubKey) {

			nodeIsPublic = true
			return errDone
		}

		// Since the edge _does_ extend to the source node, we'll also
		// need to ensure that this is a public edge.
		if info.AuthProof != nil {
			nodeIsPublic = true
			return errDone
		}

		// Otherwise, we'll continue our search.
		return nil
	})
	if err != nil && !errors.Is(err, errDone) {
		return false, err
	}

	return nodeIsPublic, nil
}
