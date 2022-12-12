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

	common_sql.Init(1)

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

//
//// TestPanic tests recovery from panic conditions.
//func TestPanic(t *testing.T) {
//	db := newTestSqliteDB(t)
//
//	//f, err := NewFixture("")
//	//require.NoError(t, err)
//
//	err := db.Update(func(tx walletdb.ReadWriteTx) error {
//		bucket, err := tx.CreateTopLevelBucket([]byte("test"))
//		require.NoError(t, err)
//
//		// Stop database server.
//		err = db.Close()
//		require.NoError(t, err)
//
//		// Keep trying to get data until Get panics because the
//		// connection is lost.
//		for i := 0; i < 100; i++ {
//			bucket.Get([]byte("key"))
//			time.Sleep(100 * time.Millisecond)
//		}
//
//		return nil
//	}, func() {})
//	require.Error(t, err)
//	require.Contains(t, err.Error(), "sql: database is closed")
//}
