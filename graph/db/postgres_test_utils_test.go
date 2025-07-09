//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"testing"

	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

func newTestFixture(t *testing.T) *sqldb.TestPgFixture {
	pgFixture := sqldb.NewTestPgFixture(t, sqldb.DefaultPostgresFixtureLifetime)
	t.Cleanup(func() {
		pgFixture.TearDown(t)
	})
	return pgFixture
}

func newReusedSQLStore(t *testing.T,
	pgFixture *sqldb.TestPgFixture) *SQLStore {

	store, ok := NewTestDBWithFixture(t, pgFixture).(*SQLStore)
	require.True(t, ok)
	return store
}
