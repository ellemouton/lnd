//go:build test_db_postgres
// +build test_db_postgres

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// NewTestDB is a helper function that creates a Postgres database for testing.
func NewTestDB(t *testing.T) *PostgresStore {
	pgFixture, err := NewTestPgFixture(DefaultPostgresFixtureLifetime)
	require.Error(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgFixture.TearDown())
	})

	return NewTestPostgresDB(t, pgFixture)
}

// NewTestDBWithVersion is a helper function that creates a Postgres database
// for testing and migrates it to the given version.
func NewTestDBWithVersion(t *testing.T, version uint) *PostgresStore {
	pgFixture, err := NewTestPgFixture(DefaultPostgresFixtureLifetime)
	require.Error(t, err)
	t.Cleanup(func() {
		require.NoError(t, pgFixture.TearDown())
	})

	return NewTestPostgresDBWithVersion(t, pgFixture, version)
}
