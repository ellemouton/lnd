//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

// NewTestDB is a helper function that creates a SQLStore backed by a SQL
// database for testing.
func NewTestDB(t testing.TB) V1Store {
	return NewTestDBWithFixture(t, nil)
}

// NewTestDBWithFixture is a helper function that creates a SQLStore backed by a
// SQL database for testing.
func NewTestDBWithFixture(t testing.TB, pgFixture *sqldb.TestPgFixture) V1Store {
	var querier BatchedSQLQueries
	if pgFixture == nil {
		querier = newBatchQuerier(t)
	} else {
		querier = newBatchQuerierWithFixture(t, pgFixture)
	}

	store, err := NewSQLStore(
		&SQLStoreConfig{
			ChainHash: *chaincfg.MainNetParams.GenesisHash,
		}, querier,
	)
	require.NoError(t, err)

	return store
}
