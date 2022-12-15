//go:build !kvdb_postgres && !kvdb_sqlite
// +build !kvdb_postgres,!kvdb_sqlite

package sqlbase

func Init(maxConnections int) {}
