//go:build kvdb_postgres
// +build kvdb_postgres

package postgres

import (
	"context"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
)

// postgresReplacements define a set of postgres keywords that should be swapped
// out with certain other sqlite keywords in any queries.
var sqlCmdReplacements = sqlbase.SQLiteCmdReplacements{
	"BLOB":                              "BYTEA",
	"INTEGER PRIMARY KEY AUTOINCREMENT": "BIGSERIAL PRIMARY KEY",
}

// newPostgresBackend returns a db object initialized with the passed backend
// config. If postgres connection cannot be established, then returns error.
func newPostgresBackend(ctx context.Context, config *Config, prefix string) (
	walletdb.DB, error) {

	cfg := &sqlbase.Config{
		DriverName:            "pgx",
		Dsn:                   config.Dsn,
		Timeout:               config.Timeout,
		Schema:                "public",
		TableNamePrefix:       prefix,
		SQLiteCmdReplacements: sqlCmdReplacements,
		WithTxLevelLock:       true,
	}

	return sqlbase.NewSqlBackend(ctx, cfg)
}
