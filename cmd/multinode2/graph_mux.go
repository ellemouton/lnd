package main

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-errors/errors"
	"github.com/lightningnetwork/lnd"
	"github.com/lightningnetwork/lnd/channeldb"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type GraphSourceMux struct {
	remote lnd.GraphSource
	local  *graphdb.ChannelGraph

	// srcPub is a cached version of the local nodes own pub key bytes.
	srcPub *route.Vertex
	mu     sync.Mutex
}

// A compile-time check to ensure that GraphSourceMux implements GraphSource.
var _ lnd.GraphSource = (*GraphSourceMux)(nil)

func NewGraphBackend(local *graphdb.ChannelGraph,
	remote lnd.GraphSource) *GraphSourceMux {

	return &GraphSourceMux{
		local:  local,
		remote: remote,
	}
}

// NewReadTx returns a new read transaction that can be used other read calls to
// the backing graph.
//
// NOTE: this is part of the graphsession.ReadOnlyGraph interface.
func (g *GraphSourceMux) NewReadTx() (graphdb.RTx, error) {
	return newRTxSet(g.remote, g.local)
}

// ForEachNodeDirectedChannel iterates through all channels of a given
// node, executing the passed callback on the directed edge representing
// the channel and its incoming policy.
//
// If the node in question is the local node, then only the local node is
// queried since it will know all channels that it owns.
//
// Otherwise, we still query the local node in case the node in question is a
// peer with whom the local node has a private channel to. In that case we want
// to make sure to run the call-back on these directed channels since the remote
// node may not know of this channel. Finally, we call the remote node but skip
// any channels we have already handled.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) ForEachNodeDirectedChannel(tx graphdb.RTx,
	node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	srcPub, err := g.selfNodePub()
	if err != nil {
		return err
	}

	lTx, rTx, err := extractRTxSet(tx)
	if err != nil {
		return err
	}

	// If we are the source node, we know all our channels, so just use
	// local DB.
	if bytes.Equal(srcPub[:], node[:]) {
		return g.local.ForEachNodeDirectedChannel(lTx, node, cb)
	}

	// Call our local DB to collect any private channels we have.
	handledPeerChans := make(map[uint64]bool)
	err = g.local.ForEachNodeDirectedChannel(lTx, node,
		func(channel *graphdb.DirectedChannel) error {

			// If the other node is not us, we don't need to handle
			// it here since the remote node will handle it later.
			if !bytes.Equal(channel.OtherNode[:], srcPub[:]) {
				return nil
			}

			// Else, we call the call back ourselves on this
			// channel and mark that we have handled it.
			handledPeerChans[channel.ChannelID] = true

			return cb(channel)
		})
	if err != nil {
		return err
	}

	return g.remote.ForEachNodeDirectedChannel(rTx, node,
		func(channel *graphdb.DirectedChannel) error {

			// Skip any we have already handled.
			if handledPeerChans[channel.ChannelID] {
				return nil
			}

			return cb(channel)
		},
	)
}

// FetchNodeFeatures returns the features of a given node. If no features are
// known for the node, an empty feature vector is returned.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) FetchNodeFeatures(tx graphdb.RTx, node route.Vertex) (
	*lnwire.FeatureVector, error) {

	// Query the local DB first. If a non-empty set of features is returned,
	// we use these. Otherwise, the remote DB is checked.
	feats, err := g.local.FetchNodeFeatures(tx, node)
	if err != nil {
		return nil, err
	}

	if !feats.IsEmpty() {
		return feats, nil
	}

	return g.remote.FetchNodeFeatures(tx, node)
}

// ForEachNode iterates through all the stored vertices/nodes in the graph,
// executing the passed callback with each node encountered. If the callback
// returns an error, then the transaction is aborted and the iteration stops
// early.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) ForEachNode(tx graphdb.RTx,
	cb func(graphdb.RTx, *models.LightningNode) error) error {

	source, err := g.local.SourceNode()
	if err != nil {
		return err
	}

	err = cb(tx, source)
	if err != nil {
		return err
	}

	_, rTx, err := extractRTxSet(tx)
	if err != nil {
		return err
	}

	return g.remote.ForEachNode(rTx,
		func(tx graphdb.RTx, node *models.LightningNode) error {

			if bytes.Equal(
				node.PubKeyBytes[:], source.PubKeyBytes[:],
			) {
				return nil
			}

			return cb(tx, node)
		},
	)
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. If the node isn't found in the database, then ErrGraphNodeNotFound is
// returned. An optional transaction may be provided. If none is provided, then
// a new one will be created.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) FetchLightningNode(tx graphdb.RTx,
	nodePub route.Vertex) (*models.LightningNode, error) {

	srcPub, err := g.selfNodePub()
	if err != nil {
		return nil, err
	}

	lTx, rTx, err := extractRTxSet(tx)
	if err != nil {
		return nil, err
	}

	if bytes.Equal(srcPub[:], nodePub[:]) {
		return g.local.FetchLightningNode(lTx, nodePub)
	}

	return g.remote.FetchLightningNode(rTx, nodePub)
}

// ForEachNodeChannel iterates through all channels of the given node,
// executing the passed callback with an edge info structure and the policies
// of each end of the channel. The first edge policy is the outgoing edge *to*
// the connecting node, while the second is the incoming edge *from* the
// connecting node. If the callback returns an error, then the iteration is
// halted with the error propagated back up to the caller.
//
// Unknown policies are passed into the callback as nil values.
//
// If the caller wishes to re-use an existing boltdb transaction, then it
// should be passed as the first argument.  Otherwise, the first argument should
// be nil and a fresh transaction will be created to execute the graph
// traversal.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) ForEachNodeChannel(tx graphdb.RTx,
	nodePub route.Vertex, cb func(graphdb.RTx, *models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	lTx, rTx, err := extractRTxSet(tx)
	if err != nil {
		return err
	}

	// First query our own db since we may have chan info that our remote
	// does not know of (regarding our selves or our channel peers).
	var found bool
	err = g.local.ForEachNodeChannel(lTx, nodePub, func(tx graphdb.RTx,
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		found = true

		return cb(tx, info, policy, policy2)
	})
	// Only return the error if it was found.
	if err != nil && found {
		return err
	}

	if found {
		return nil
	}

	return g.remote.ForEachNodeChannel(rTx, nodePub, cb)
}

// TODO(elle): what about our private channels in direction from peer to us.
func (g *GraphSourceMux) ForEachNodeCached(cb func(node route.Vertex, chans map[uint64]*graphdb.DirectedChannel) error) error {
	srcPub, err := g.selfNodePub()
	if err != nil {
		return err
	}

	// Only for our own node do we call our local DB for this, else use the
	// remote.
	ourChans := make(map[uint64]*graphdb.DirectedChannel)
	err = g.local.ForEachNodeDirectedChannel(
		nil, srcPub, func(channel *graphdb.DirectedChannel) error {
			ourChans[channel.ChannelID] = channel

			return nil
		},
	)
	if err != nil {
		return err
	}

	err = cb(srcPub, ourChans)
	if err != nil {
		return err
	}

	return g.remote.ForEachNodeCached(func(node route.Vertex, chans map[uint64]*graphdb.DirectedChannel) error {
		// Skip our own node.
		if bytes.Equal(node[:], srcPub[:]) {
			return nil
		}

		return cb(node, chans)
	})
}

// FetchChannelEdgesByID attempts to look up the two directed edges for the
// channel identified by the channel ID. If the channel can't be found, then
// graphdb.ErrEdgeNotFound is returned.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) FetchChannelEdgesByID(chanID uint64) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	info, p1, p2, err := g.local.FetchChannelEdgesByID(chanID)
	if err == nil {
		return info, p1, p2, nil
	}

	return g.remote.FetchChannelEdgesByID(chanID)
}

// IsPublicNode is a helper method that determines whether the node with the
// given public key is seen as a public node in the graph from the graph's
// source node's point of view. This first checks the local node and then the
// remote if the node is not seen as public by the loca node.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) IsPublicNode(pubKey [33]byte) (bool, error) {
	isInLocalDB, err := g.local.IsPublicNode(pubKey)
	if err != nil {
		return false, err
	}
	if isInLocalDB {
		return true, nil
	}

	return g.remote.IsPublicNode(pubKey)
}

// FetchChannelEdgesByOutpoint returns the channel edge info and most recent
// channel edge policies for a given outpoint.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	edge, p1, p2, err := g.local.FetchChannelEdgesByOutpoint(point)
	if err == nil {
		return edge, p1, p2, nil
	}

	return g.remote.FetchChannelEdgesByOutpoint(point)
}

// AddrsForNode returns all known addresses for the target node public key. The
// returned boolean must indicate if the given node is unknown to the backing
// source. This merges the results from both the local and remote source.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) AddrsForNode(nodePub *btcec.PublicKey) (bool,
	[]net.Addr, error) {

	// Check both the local and remote sources and merge the results.
	return channeldb.NewMultiAddrSource(
		g.local, g.remote,
	).AddrsForNode(nodePub)
}

// ForEachChannel iterates through all the channel edges stored within the graph
// and invokes the passed callback for each edge. If the callback returns an
// error, then the transaction is aborted and the iteration stops early. An
// edge's policy structs may be nil if the ChannelUpdate in question has not yet
// been received for the channel.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	srcPub, err := g.selfNodePub()
	if err != nil {
		return err
	}

	ourChans := make(map[uint64]bool)
	err = g.local.ForEachNodeChannel(nil, srcPub, func(_ graphdb.RTx,
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		ourChans[info.ChannelID] = true

		return cb(info, policy, policy2)
	})
	if err != nil {
		return err
	}

	return g.remote.ForEachChannel(func(info *models.ChannelEdgeInfo,
		policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		if ourChans[info.ChannelID] {
			return nil
		}

		return cb(info, policy, policy2)
	})
}

// HasLightningNode determines if the graph has a vertex identified by the
// target node identity public key. If the node exists in the database, a
// timestamp of when the data for the node was lasted updated is returned along
// with a true boolean. Otherwise, an empty time.Time is returned with a false
// boolean.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) HasLightningNode(nodePub [33]byte) (time.Time, bool, error) {
	timeStamp, localHas, err := g.local.HasLightningNode(nodePub)
	if err != nil {
		return timeStamp, false, err
	}
	if localHas {
		return timeStamp, true, nil
	}

	return g.remote.HasLightningNode(nodePub)
}

// NumZombies returns the current number of zombie channels in the
// graph. This only queries the remote graph.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) NumZombies() (uint64, error) {
	return g.remote.NumZombies()
}

// LookupAlias attempts to return the alias as advertised by the target node.
// graphdb.ErrNodeAliasNotFound is returned if the alias is not found.
//
// NOTE: this is part of the GraphSource interface.
func (g *GraphSourceMux) LookupAlias(pub *btcec.PublicKey) (string, error) {
	// First check locally.
	alias, err := g.local.LookupAlias(pub)
	if err == nil {
		return alias, nil
	}
	if !errors.Is(err, graphdb.ErrNodeAliasNotFound) {
		return "", err
	}

	return g.remote.LookupAlias(pub)
}

// selfNodePub fetches the local nodes pub key. It first checks the cached value
// and if non exists, it queries the database.
func (g *GraphSourceMux) selfNodePub() (route.Vertex, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.srcPub != nil {
		return *g.srcPub, nil
	}

	source, err := g.local.SourceNode()
	if err != nil {
		return route.Vertex{}, err
	}

	pub, err := route.NewVertexFromBytes(source.PubKeyBytes[:])
	if err != nil {
		return route.Vertex{}, err
	}

	g.srcPub = &pub

	return *g.srcPub, nil
}

type rTxConstructor interface {
	NewReadTx() (graphdb.RTx, error)
}

// rTxSet is an implementation of graphdb.RTx which is backed a read transaction
// for the local graph and one for a remote graph.
type rTxSet struct {
	lRTx graphdb.RTx
	rRTx graphdb.RTx
}

// newMultiRTx uses the given rTxConstructors to begin a read transaction for
// each and returns a multiRTx that represents this open set of transactions.
func newRTxSet(localConstructor, remoteConstructor rTxConstructor) (*rTxSet,
	error) {

	localRTx, err := localConstructor.NewReadTx()
	if err != nil {
		return nil, err
	}

	remoteRTx, err := remoteConstructor.NewReadTx()
	if err != nil {
		_ = localRTx.Close()

		return nil, err
	}

	return &rTxSet{
		lRTx: localRTx,
		rRTx: remoteRTx,
	}, nil
}

// Close closes all the transactions held by multiRTx.
//
// NOTE: this is part of the graphdb.RTx interface.
func (s *rTxSet) Close() error {
	var returnErr error

	if s.lRTx != nil {
		if err := s.lRTx.Close(); err != nil {
			returnErr = err
		}
	}

	if s.rRTx != nil {
		if err := s.rRTx.Close(); err != nil {
			returnErr = err
		}
	}

	return returnErr
}

// MustImplementRTx is a helper method that ensures that the rTxSet type
// implements the RTx interface.
//
// NOTE: this is part of the graphdb.RTx interface.
func (s *rTxSet) MustImplementRTx() {}

// A compile-time check to ensure that multiRTx implements graphdb.RTx.
var _ graphdb.RTx = (*rTxSet)(nil)

// extractRTxSet is a helper function that casts an RTx into a rTxSet returns
// the local and remote RTxs respectively.
func extractRTxSet(tx graphdb.RTx) (graphdb.RTx, graphdb.RTx, error) {
	if tx == nil {
		return nil, nil, nil
	}

	set, ok := tx.(*rTxSet)
	if !ok {
		return nil, nil, fmt.Errorf("expected a rTxSet, got %T", tx)
	}

	return set.lRTx, set.rRTx, nil
}
