package sources

import (
	graphdb "github.com/lightningnetwork/lnd/graph/db"
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

// NewGraph creates a new Graph instance without any underlying read-lock. This
// method should be used when the caller does not need to hold a read-lock
// across multiple calls to the Graph.
//
// NOTE: this is part of the GraphSessionFactory interface.
func (s *DBSource) NewGraph() routing.Graph {
	return &dbSrcSession{src: s}
}

// NewGraphSession will produce a new routing.Graph to use for a
// path-finding session. It returns the routing.Graph along with a call-back
// that must be called once routing.Graph access is complete. This call-back
// will close any read-only transaction that was created at Graph construction
// time.
//
// NOTE: this is part of the GraphSessionFactory interface.
func (s *DBSource) NewGraphSession() (routing.Graph, func() error, error) {
	tx, err := s.db.NewPathFindTx()
	if err != nil {
		return nil, nil, err
	}

	session := &dbSrcSession{
		src: s,
		tx:  tx,
	}

	return session, session.close, nil
}

// dbSrcSession is an implementation of the routing.Graph interface where the
// same read-only transaction is held across calls.
type dbSrcSession struct {
	src *DBSource
	tx  kvdb.RTx
}

// FetchNodeFeatures returns the features of the given node.
//
// NOTE: this is part of the routing.Graph interface.
func (s *dbSrcSession) FetchNodeFeatures(nodePub route.Vertex) (
	*lnwire.FeatureVector, error) {

	return s.src.db.FetchNodeFeatures(s.tx, nodePub)
}

// ForEachNodeChannel calls the callback for every channel of the given node.
//
// NOTE: this is part of the routing.Graph interface.
func (s *dbSrcSession) ForEachNodeChannel(node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	return s.src.db.ForEachNodeDirectedChannel(s.tx, node, cb)
}

// close closes the underlying transaction if there is one.
func (s *dbSrcSession) close() error {
	if s.tx == nil {
		return nil
	}

	return s.tx.Rollback()
}
