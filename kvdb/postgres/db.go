//go:build kvdb_postgres
// +build kvdb_postgres

package postgres

import (
	"context"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/common_sql"
)

// newPostgresBackend returns a db object initialized with the passed backend
// config. If postgres connection cannot be estabished, then returns error.
func newPostgresBackend(ctx context.Context, config *Config, prefix string) (
	walletdb.DB, error) {

	cfg := &common_sql.Config{
		DriverName:      "pgx",
		Dsn:             config.Dsn,
		Timeout:         config.Timeout,
		Schema:          "public",
		TableNamePrefix: prefix,
	}

	return common_sql.NewSqlBackend(ctx, cfg)
}
