package graphdb

import (
	"fmt"

	"github.com/lightningnetwork/lnd/kvdb"
)

// RTx represents a database transaction that can only be used for graph DB
// reads.
type RTx interface {
	// Close closes the transaction.
	Close() error

	// MustImplementRTx is a helper method that ensures that the RTx
	// interface is implemented by the underlying type. This is useful since
	// the other methods in the interface are quite generic and so many
	// types will satisfy the interface if it only contains those methods.
	MustImplementRTx()
}

// KVDBRTx is an implementation of graphdb.RTx backed by a KVDB database read
// transaction.
type KVDBRTx struct {
	kvdb.RTx
}

// NewKVDBRTx constructs a KVDBRTx instance backed by the given kvdb.RTx.
func NewKVDBRTx(tx kvdb.RTx) *KVDBRTx {
	return &KVDBRTx{tx}
}

// Close closes the underlying transaction.
//
// NOTE: this is part of the graphdb.RTx interface.
func (t *KVDBRTx) Close() error {
	if t.RTx == nil {
		return nil
	}

	return t.RTx.Rollback()
}

// MustImplementRTx is a helper method that ensures that the KVDBRTx type
// implements the RTx interface.
//
// NOTE: this is part of the graphdb.RTx interface.
func (t *KVDBRTx) MustImplementRTx() {}

// A compile-time assertion to ensure that KVDBRTx implements the RTx interface.
var _ RTx = (*KVDBRTx)(nil)

// extractKVDBRTx is a helper function that casts an RTx into a KVDBRTx and
// errors if the cast fails.
func extractKVDBRTx(tx RTx) (kvdb.RTx, error) {
	if tx == nil {
		return nil, nil
	}

	kvdbTx, ok := tx.(*KVDBRTx)
	if !ok {
		return nil, fmt.Errorf("expected a graphdb.KVDBRTx, got %T", tx)
	}

	return kvdbTx, nil
}
