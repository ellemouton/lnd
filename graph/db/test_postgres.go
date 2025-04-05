//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"database/sql"
	"testing"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

// NewTestDB is a helper function that creates an BBolt database for testing.
func NewTestDB(t testing.TB) *SQLStore {
	backend, backendCleanup, err := kvdb.GetTestBackend(t.TempDir(), "cgr")
	require.NoError(t, err)

	t.Cleanup(backendCleanup)

	graphStore, err := NewKVStore(backend)
	require.NoError(t, err)

	pgFixture := sqldb.NewTestPgFixture(
		t, sqldb.DefaultPostgresFixtureLifetime,
	)
	t.Cleanup(func() {
		pgFixture.TearDown(t)
	})

	db := sqldb.NewTestPostgresDB(t, pgFixture).BaseDB

	executor := sqldb.NewTransactionExecutor(
		db, func(tx *sql.Tx) SQLQueries {
			return db.WithTx(tx)
		},
	)

	return NewSQLStore(executor, graphStore)
}
