//go:build kvdb_sqlite
// +build kvdb_sqlite

package sqlite

import (
	"testing"

	"github.com/btcsuite/btcwallet/walletdb/walletdbtest"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// TestInterface performs all interfaces tests for this database driver.
func TestInterface(t *testing.T) {
	// dbType is the database type name for this driver.
	dir := t.TempDir()
	ctx := context.Background()

	sqlbase.Init(0)

	sqlDB, err := NewSqliteBackend(ctx, &Config{
		DBPath: dir,
	}, "tmp.db", "table")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	walletdbtest.TestInterface(t, dbType, ctx, &Config{
		DBPath: dir,
	}, "tmp.db", "temp")
}
