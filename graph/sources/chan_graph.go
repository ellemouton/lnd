package sources

import (
	"context"
	"fmt"

	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/graphsession"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChanGraphSource is an implementation of the lnd.GraphSource interface which
// can be used to make read-only graph queries.
type ChanGraphSource struct {
	db *graphdb.ChannelGraph
}

// NewChanGraphSource returns a new instance of the ChanGraphSource backed by
// a graphdb.ChannelGraph instance.
func NewChanGraphSource(db *graphdb.ChannelGraph) *ChanGraphSource {
	return &ChanGraphSource{
		db: db,
	}
}

// NewPathFindTx returns a new read transaction that can be used for a single
// path finding session. Will return nil if the graph cache is enabled for the
// underlying graphdb.ChannelGraph.
//
// NOTE: this is part of the graphsession.ReadOnlyGraph interface.
func (s *ChanGraphSource) NewPathFindTx(_ context.Context) (graphsession.RTx,
	error) {

	tx, err := s.db.NewPathFindTx()
	if err != nil {
		return nil, err
	}

	return newKVDBRTx(tx), nil
}

// ForEachNodeDirectedChannel iterates through all channels of a given node,
// executing the passed callback on the directed edge representing the channel
// and its incoming policy. If the callback returns an error, then the
// iteration is halted with the error propagated back up to the caller. An
// optional read transaction may be provided. If it is, then it will be cast
// into a kvdb.RTx and passed into the callback.
//
// Unknown policies are passed into the callback as nil values.
//
// NOTE: this is part of the graphsession.ReadOnlyGraph interface.
func (s *ChanGraphSource) ForEachNodeDirectedChannel(_ context.Context,
	tx graphsession.RTx, node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	kvdbTx, err := extractKVDBRTx(tx)
	if err != nil {
		return err
	}

	return s.db.ForEachNodeDirectedChannel(kvdbTx, node, cb)
}

// FetchNodeFeatures returns the features of a given node. If no features are
// known for the node, an empty feature vector is returned. An optional read
// transaction may be provided. If it is, then it will be cast into a kvdb.RTx
// and passed into the callback.
//
// NOTE: this is part of the graphsession.ReadOnlyGraph interface.
func (s *ChanGraphSource) FetchNodeFeatures(_ context.Context,
	tx graphsession.RTx, node route.Vertex) (*lnwire.FeatureVector, error) {

	kvdbTx, err := extractKVDBRTx(tx)
	if err != nil {
		return nil, err
	}

	return s.db.FetchNodeFeatures(kvdbTx, node)
}

// kvdbRTx is an implementation of graphdb.RTx backed by a KVDB database read
// transaction.
type kvdbRTx struct {
	kvdb.RTx
}

// newKVDBRTx constructs a kvdbRTx instance backed by the given kvdb.RTx.
func newKVDBRTx(tx kvdb.RTx) *kvdbRTx {
	return &kvdbRTx{tx}
}

// Close closes the underlying transaction.
//
// NOTE: this is part of the graphdb.RTx interface.
func (t *kvdbRTx) Close() error {
	if t.RTx == nil {
		return nil
	}

	return t.RTx.Rollback()
}

// MustImplementRTx is a helper method that ensures that the kvdbRTx type
// implements the RTx interface.
//
// NOTE: this is part of the graphdb.RTx interface.
func (t *kvdbRTx) MustImplementRTx() {}

// A compile-time assertion to ensure that kvdbRTx implements the RTx interface.
var _ graphsession.RTx = (*kvdbRTx)(nil)

// extractKVDBRTx is a helper function that casts an RTx into a kvdbRTx and
// errors if the cast fails.
func extractKVDBRTx(tx graphsession.RTx) (kvdb.RTx, error) {
	if tx == nil {
		return nil, nil
	}

	kvdbTx, ok := tx.(*kvdbRTx)
	if !ok {
		return nil, fmt.Errorf("expected a graphdb.kvdbRTx, got %T", tx)
	}

	return kvdbTx, nil
}
