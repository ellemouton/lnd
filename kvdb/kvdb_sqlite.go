//go:build kvdb_sqlite
// +build kvdb_sqlite

package kvdb

import (
	"context"
	"time"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/common_sql"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
)

const (
	// SqliteBackend is conditionally set to false when the kvdb_sqlite
	// build tag is not defined. This will allow testing of other database
	// backends.
	SqliteBackend = true

	testMaxConnections = 50
)

func StartSqliteTestBackend(path, table string) (walletdb.DB, error) {
	common_sql.Init(testMaxConnections)
	return sqlite.NewSqliteBackend(
		context.Background(), &sqlite.Config{
			DBPath:  path,
			Timeout: time.Second * 30,
		}, table,
	)
}
