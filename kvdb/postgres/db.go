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

	s := `
CREATE SCHEMA IF NOT EXISTS public;
CREATE TABLE IF NOT EXISTS public.TABLE_NAME
(
    key bytea NOT NULL,
    value bytea,
    parent_id bigint,
    id bigserial PRIMARY KEY,
    sequence bigint,
    CONSTRAINT TABLE_NAME_parent FOREIGN KEY (parent_id)
        REFERENCES public.TABLE_NAME (id)
        ON UPDATE NO ACTION
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS TABLE_NAME_p
    ON public.TABLE_NAME (parent_id);

CREATE UNIQUE INDEX IF NOT EXISTS TABLE_NAME_up
    ON public.TABLE_NAME
    (parent_id, key) WHERE parent_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS TABLE_NAME_unp 
    ON public.TABLE_NAME (key) WHERE parent_id IS NULL;
`

	cfg := &common_sql.Config{
		DriverName: "pgx",
		Dsn:        config.Dsn,
		Timeout:    config.Timeout,
		SchemaStr:  s,
	}

	return common_sql.NewSqlBackend(ctx, cfg, prefix)
}
