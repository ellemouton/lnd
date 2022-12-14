//go:build !kvdb_postgres && !kvdb_sqlite
// +build !kvdb_postgres,!kvdb_sqlite

package common_sql

func Init(maxConnections int) {}
