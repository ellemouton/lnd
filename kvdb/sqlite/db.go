//go:build kvdb_sqlite
// +build kvdb_sqlite

package sqlite

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/common_sql"
	_ "modernc.org/sqlite" // Register relevant drivers.
)

const (
	// sqliteOptionPrefix is the string prefix sqlite uses to set various
	// options. This is used in the following format:
	//   * sqliteOptionPrefix || option_name = option_value.
	sqliteOptionPrefix = "_pragma"
)

var postgresReplacements = common_sql.PostgresCmdReplacements{
	"BYTEA":                 "BLOB",
	"BIGSERIAL PRIMARY KEY": "INTEGER PRIMARY KEY AUTOINCREMENT",
}

// fileExists returns true if the file exists, and false otherwise.
func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// NewSqliteBackend returns a db object initialized with the passed backend
// config. If a sqlite connection cannot be estabished, then an error is
// returned.
func NewSqliteBackend(ctx context.Context, cfg *Config, prefix string) (
	walletdb.DB, error) {

	dbFilePath := filepath.Join(cfg.DBPath, cfg.DBFileName)
	if !fileExists(dbFilePath) {
		if !fileExists(cfg.DBPath) {
			if err := os.MkdirAll(cfg.DBPath, 0700); err != nil {
				return nil, err
			}
		}
	}

	// The set of pragma options are accepted using query options.
	pragmaOptions := []struct {
		name  string
		value string
	}{
		{
			name:  "busy_timeout",
			value: "5000",
		},
		{
			name:  "foreign_keys",
			value: "on",
		},
		{
			name:  "journal_mode",
			value: "WAL",
		},
	}
	sqliteOptions := make(url.Values)
	for _, option := range pragmaOptions {
		sqliteOptions.Add(
			sqliteOptionPrefix,
			fmt.Sprintf("%v=%v", option.name, option.value),
		)
	}

	// Construct the DSN which is just the database file name, appended
	// with the series of pragma options as a query URL string. For more
	// details on the formatting here, see the modernc.org/sqlite docs:
	// https://pkg.go.dev/modernc.org/sqlite#Driver.Open.
	dsn := fmt.Sprintf("%v?%v", dbFilePath, sqliteOptions.Encode())

	// Compose system table names.
	config := &common_sql.Config{
		DriverName:              "sqlite",
		Dsn:                     dsn,
		Timeout:                 cfg.Timeout,
		TableNamePrefix:         prefix,
		PostgresCmdReplacements: postgresReplacements,
	}

	return common_sql.NewSqlBackend(ctx, config)
}
