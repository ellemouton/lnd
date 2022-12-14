//go:build !kvdb_postgres
// +build !kvdb_postgres

package common_sql

func Init(maxConnections int) {}
