//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"database/sql"
	"testing"

	"github.com/lightningnetwork/lnd/sqldb"
)

// newBatchQuerier creates a new BatchedSQLQueries instance for testing
// using a PostgreSQL database fixture.
func newBatchQuerier(t testing.TB) BatchedSQLQueries {
	pgFixture := sqldb.NewTestPgFixture(
		t, sqldb.DefaultPostgresFixtureLifetime,
	)
	t.Cleanup(func() {
		pgFixture.TearDown(t)
	})

	return newBatchQuerierWithFixture(t, pgFixture)
}

// newBatchQuerierWithFixture creates a new BatchedSQLQueries instance for
// testing using a PostgreSQL database fixture.
func newBatchQuerierWithFixture(t testing.TB,
	pgFixture *sqldb.TestPgFixture) BatchedSQLQueries {

	db := sqldb.NewTestPostgresDB(t, pgFixture).BaseDB

	return sqldb.NewTransactionExecutor(
		db, func(tx *sql.Tx) SQLQueries {
			return db.WithTx(tx)
		},
	)
}
