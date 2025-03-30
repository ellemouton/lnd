package graphdb

import (
	"github.com/lightningnetwork/lnd/sqldb"
)

type SQLQueries interface {
}

type BatchedSQLQueries interface {
	SQLQueries

	sqldb.BatchedTx[SQLQueries]
}

type SQLStore struct {
	db BatchedSQLQueries

	// Temporary fall-back to the KVStore so that we can implement the
	// interface incrementally.
	*KVStore
}

var _ V1Store = (*SQLStore)(nil)

// NewSQLStore creates a new SQLStore instance given an open BatchedSQLQueries
// storage backend.
func NewSQLStore(db BatchedSQLQueries, kvStore *KVStore) *SQLStore {
	return &SQLStore{
		db:      db,
		KVStore: kvStore,
	}
}
