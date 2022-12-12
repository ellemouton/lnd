//go:build !kvdb_sqlite
// +build !kvdb_sqlite

package kvdb

import (
	"fmt"

	"github.com/btcsuite/btcwallet/walletdb"
)

var errSqliteNotAvailable = fmt.Errorf("sqlite backend not available")

const SqlBackend = false

// StartSqliteTestBackend is a stub returning nil, and errSqliteNotAvailable
// error.
func StartSqliteTestBackend(path, name, table string) (walletdb.DB, error) {
	return nil, errSqliteNotAvailable
}
