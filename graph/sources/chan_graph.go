package sources

import (
	"context"

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

// ForEachNode iterates through all the stored vertices/nodes in the graph,
// executing the passed callback with each node encountered. If the callback
// returns an error, then the transaction is aborted and the iteration stops
// early.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) ForEachNode(_ context.Context,
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

// NewGraph creates a new Graph instance without any underlying read-lock. This
// method should be used when the caller does not need to hold a read-lock
// across multiple calls to the Graph.
//
// NOTE: this is part of the GraphSessionFactory interface.
func (s *DBSource) NewGraph() routing.Graph {
	return &dbSrcSession{db: s.db}
}

// NewGraphSession will produce a new Graph to use for a path-finding
// session. It returns the Graph along with a call-back that must be
// called once Graph access is complete. This call-back will close any
// read-only transaction that was created at Graph construction time.
//
// NOTE: this is part of the GraphSessionFactory interface.
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

// FetchNodeFeatures returns the features of the given node.
//
// NOTE: this is part of the Graph interface.
func (s *dbSrcSession) FetchNodeFeatures(_ context.Context,
	nodePub route.Vertex) (*lnwire.FeatureVector, error) {

	return s.db.FetchNodeFeatures(s.tx, nodePub)
}

// ForEachNodeChannel calls the callback for every channel of the given node.
//
// NOTE: this is part of the Graph interface.
func (s *dbSrcSession) ForEachNodeChannel(_ context.Context, node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	return s.db.ForEachNodeDirectedChannel(s.tx, node, cb)
}

// close closes the underlying transaction if there is one.
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

func (s *DBSource) SourceNode(_ context.Context) (*models.LightningNode,
	error) {

	return s.db.SourceNode()
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

// NumZombies returns the number of channels that the GraphSource considers to
// be zombies.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) NumZombies(_ context.Context) (uint64, error) {
	return s.db.NumZombies()
}

// ForEachNodeCached is similar to ForEachNode, but it utilizes the channel
// graph cache instead. Note that this doesn't return all the information the
// regular ForEachNode method does.
//
// NOTE: this is part of the GraphSource interface.
func (s *DBSource) ForEachNodeCached(_ context.Context,
	cb func(node route.Vertex,
		chans map[uint64]*graphdb.DirectedChannel) error) error {

	return s.db.ForEachNodeCached(cb)
}
