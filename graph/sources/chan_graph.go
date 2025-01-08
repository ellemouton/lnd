package sources

import (
	"context"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/routing/route"
)

// DBSource is an implementation of the GraphSource interface backed by a local
// persistence layer holding graph related data.
type DBSource struct {
	db *graphdb.ChannelGraph
}

// A compile-time check to ensure that sources.DBSource implements the
// GraphSource interface.
var _ GraphSource = (*DBSource)(nil)

// NewDBGSource returns a new instance of the DBSource backed by a
// graphdb.ChannelGraph instance.
func NewDBGSource(db *graphdb.ChannelGraph) *DBSource {
	return &DBSource{
		db: db,
	}
}

func (s *DBSource) ForEachNodeWithTx(_ context.Context,
	cb func(NodeTx) error) error {

	return s.db.ForEachNode(func(tx kvdb.RTx,
		node *models.LightningNode) error {

		return cb(newDBNodeWithTx(s.db, node, tx))
	})
}

type dbNodeWithTx struct {
	db   *graphdb.ChannelGraph
	node *models.LightningNode
	tx   kvdb.RTx
}

func newDBNodeWithTx(db *graphdb.ChannelGraph, node *models.LightningNode,
	tx kvdb.RTx) *dbNodeWithTx {

	return &dbNodeWithTx{
		db:   db,
		node: node,
		tx:   tx,
	}
}

func (n *dbNodeWithTx) Node() *models.LightningNode {
	return n.node
}

func (n *dbNodeWithTx) ForEachNodeChannel(_ context.Context,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return n.db.ForEachNodeChannelTx(n.tx, nodePub, func(tx kvdb.RTx,
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		return cb(info, policy, policy2)
	})
}

func (n *dbNodeWithTx) FetchLightningNode(_ context.Context,
	nodePub route.Vertex) (NodeTx, error) {

	node, err := n.db.FetchLightningNodeTx(n.tx, nodePub)
	if err != nil {
		return nil, err
	}

	return newDBNodeWithTx(n.db, node, n.tx), nil
}

type dbSrcSession struct {
	db *graphdb.ChannelGraph
	tx kvdb.RTx
}

func (s *DBSource) NewGraph() routing.Graph {
	return &dbSrcSession{db: s.db}
}

func (s *DBSource) NewGraphSession(_ context.Context) (routing.Graph,
	func() error, error) {

	tx, err := s.db.NewPathFindTx()
	if err != nil {
		return nil, nil, err
	}

	session := &dbSrcSession{
		db: s.db,
		tx: tx,
	}

	return session, session.close, nil
}

func (s *dbSrcSession) FetchNodeFeatures(_ context.Context,
	nodePub route.Vertex) (*lnwire.FeatureVector, error) {

	return s.db.FetchNodeFeatures(s.tx, nodePub)
}

func (s *dbSrcSession) ForEachNodeChannel(_ context.Context, node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	return s.db.ForEachNodeDirectedChannel(s.tx, node, cb)
}

func (s *dbSrcSession) close() error {
	if s.tx == nil {
		return nil
	}

	return s.tx.Rollback()
}

// FetchChannelEdgesByID attempts to look up the two directed edges for the
// channel identified by the channel ID. If the channel can't be found, then
// graphdb.ErrEdgeNotFound is returned.
//
// NOTE: this is part of the invoicesrpc.GraphSource interface.
func (s *DBSource) FetchChannelEdgesByID(_ context.Context,
	chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	return s.db.FetchChannelEdgesByID(chanID)
}

// IsPublicNode determines whether the node with the given public key is seen as
// a public node in the graph from the graph's source node's point of view.
//
// NOTE: this is part of the invoicesrpc.GraphSource interface.
func (s *DBSource) IsPublicNode(_ context.Context,
	pubKey [33]byte) (bool, error) {

	return s.db.IsPublicNode(pubKey)
}

// FetchChannelEdgesByOutpoint returns the channel edge info and most recent
// channel edge policies for a given outpoint.
//
// NOTE: this is part of the netann.ChannelGraph interface.
func (s *DBSource) FetchChannelEdgesByOutpoint(_ context.Context,
	point *wire.OutPoint) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	return s.db.FetchChannelEdgesByOutpoint(point)
}

// ForEachChannel iterates through all the channel edges stored within the graph
// and invokes the passed callback for each edge. If the callback returns an
// error, then the transaction is aborted and the iteration stops early. An
// edge's policy structs may be nil if the ChannelUpdate in question has not yet
// been received for the channel.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) ForEachChannel(_ context.Context,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return s.db.ForEachChannel(cb)
}

// ForEachNode iterates through all the stored vertices/nodes in the graph,
// executing the passed callback with each node encountered. If the callback
// returns an error, then the transaction is aborted and the iteration stops
// early.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) ForEachNode(_ context.Context,
	cb func(*models.LightningNode) error) error {

	return s.db.ForEachNode(func(_ kvdb.RTx,
		node *models.LightningNode) error {

		return cb(node)
	})
}

// HasLightningNode determines if the graph has a vertex identified by the
// target node identity public key. If the node exists in the database, a
// timestamp of when the data for the node was lasted updated is returned along
// with a true boolean. Otherwise, an empty time.Time is returned with a false
// boolean.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) HasLightningNode(_ context.Context,
	nodePub [33]byte) (time.Time, bool, error) {

	return s.db.HasLightningNode(nodePub)
}

// LookupAlias attempts to return the alias as advertised by the target node.
// graphdb.ErrNodeAliasNotFound is returned if the alias is not found.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) LookupAlias(_ context.Context,
	pub *btcec.PublicKey) (string, error) {

	return s.db.LookupAlias(pub)
}

// ForEachNodeChannel iterates through all channels of the given node, executing
// the passed callback with an edge info structure and the policies of each end
// of the channel. The first edge policy is the outgoing edge *to* the
// connecting node, while the second is the incoming edge *from* the connecting
// node. If the callback returns an error, then the iteration is halted with the
// error propagated back up to the caller. Unknown policies are passed into the
// callback as nil values.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) ForEachNodeChannel(_ context.Context,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return s.db.ForEachNodeChannel(nodePub, func(_ kvdb.RTx,
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		return cb(info, policy, policy2)
	})
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. If the node isn't found in the database, then
// graphdb.ErrGraphNodeNotFound is returned.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) FetchLightningNode(_ context.Context,
	nodePub route.Vertex) (*models.LightningNode, error) {

	return s.db.FetchLightningNode(nodePub)
}

func (s *DBSource) NumZombies(_ context.Context) (uint64, error) {
	return s.db.NumZombies()
}

func (s *DBSource) ForEachNodeCached(_ context.Context,
	cb func(node route.Vertex,
		chans map[uint64]*graphdb.DirectedChannel) error) error {

	return s.db.ForEachNodeCached(cb)
}
