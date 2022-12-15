//go:build !kvdb_sqlite
// +build !kvdb_sqlite

package kvdb

import (
	"fmt"

	"github.com/btcsuite/btcwallet/walletdb"
)

var errSqliteNotAvailable = fmt.Errorf("sqlite backend not available")

// SqliteBackend is conditionally set to false when the kvdb_sqlite build tag is
// not defined. This will allow testing of other database backends.
const SqliteBackend = false

// StartSqliteTestBackend is a stub returning nil, and errSqliteNotAvailable
// error.
func StartSqliteTestBackend(path, name, table string) (walletdb.DB, error) {
	return nil, errSqliteNotAvailable
}
