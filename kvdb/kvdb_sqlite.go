//go:build kvdb_sqlite
// +build kvdb_sqlite

package kvdb

import (
	"context"
	"time"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
)

const SqlBackend = true

func StartSqliteTestBackend(path, name, table string) (walletdb.DB, error) {
	return sqlite.NewSqliteBackend(
		context.Background(), &sqlite.Config{
			DBPath:     path,
			DBFileName: name,
			Timeout:    time.Second * 30,
		}, table,
	)
}
