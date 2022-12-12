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
	SqlBackend         = true
	testMaxConnections = 50
)

func StartSqliteTestBackend(path, name, table string) (walletdb.DB, error) {
	common_sql.Init(testMaxConnections)
	return sqlite.NewSqliteBackend(
		context.Background(), &sqlite.Config{
			DBPath:     path,
			DBFileName: name,
			Timeout:    time.Second * 30,
		}, table,
	)
}
