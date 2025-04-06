//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"database/sql"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/sqldb"
)

var testFixture *sqldb.TestPgFixture

// NewTestDB is a helper function that creates an BBolt database for testing.
func NewTestDB(t testing.TB) *SQLStore {
	db := sqldb.NewTestPostgresDB(t, testFixture).BaseDB

	executor := sqldb.NewTransactionExecutor(
		db, func(tx *sql.Tx) SQLQueries {
			return db.WithTx(tx)
		},
	)

	return NewSQLStore(
		&SQLStoreConfig{ChainHash: *chaincfg.MainNetParams.GenesisHash},
		executor,
	)
}
