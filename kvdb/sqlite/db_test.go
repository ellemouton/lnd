//go:build kvdb_sqlite
// +build kvdb_sqlite

package sqlite

import (
	"testing"

	"github.com/btcsuite/btcwallet/walletdb/walletdbtest"
	"github.com/lightningnetwork/lnd/kvdb/common_sql"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// TestInterface performs all interfaces tests for this database driver.
func TestInterface(t *testing.T) {
	// dbType is the database type name for this driver.
	dir := t.TempDir()
	ctx := context.Background()

	common_sql.Init(0)

	sqlDB, err := NewSqliteBackend(ctx, &Config{
		DBPath:     dir,
		DBFileName: "tmp.db",
	}, "temp")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	walletdbtest.TestInterface(
		t, dbType, ctx, &Config{
			DBPath:     dir,
			DBFileName: "tmp.db",
		}, "temp",
	)
}
