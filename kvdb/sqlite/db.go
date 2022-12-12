//go:build kvdb_sqlite
// +build kvdb_sqlite

package sqlite

import (
	"context"
	"errors"
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
func NewSqliteBackend(ctx context.Context, cfg *Config, path, name,
	prefix string) (

	walletdb.DB, error) {

	if path == "" {
		return nil, errors.New("a path to the sqlite db must be " +
			"specified")
	}

	if name == "" {
		return nil, errors.New("a name for the sqlite db must be " +
			"specified")
	}

	dbFilePath := filepath.Join(path, name)
	if !fileExists(dbFilePath) {
		if !fileExists(path) {
			if err := os.MkdirAll(path, 0700); err != nil {
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
			name:  "foreign_keys",
			value: "on",
		},
		{
			name:  "journal_mode",
			value: "WAL",
		},
		{
			name:  "busy_timeout",
			value: "5000",
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
	dsn := fmt.Sprintf(
		"%v?%v", filepath.Join(path, name), sqliteOptions.Encode(),
	)

	// Execute the create statements to set up a kv table in sqlite. Every
	// row points to the bucket that it is one via its parent_id field. A
	// NULL parent_id means that the key belongs to the upper-most bucket in
	// this table. A constraint on parent_id is enforcing referential
	// integrity.
	//
	// Furthermore there is a <table>_p index on parent_id that is required
	// for the foreign key constraint.
	//
	// Finally there are unique indices on (parent_id, key) to prevent the
	// same key being present in a bucket more than once (<table>_up and
	// <table>_unp). In postgres, a single index wouldn't enforce the unique
	// constraint on rows with a NULL parent_id. Therefore two indices are
	// defined.
	s := `
CREATE TABLE IF NOT EXISTS TABLE_NAME (
    key BLOB NOT NULL,
    value BLOB,
    parent_id BIGINT,
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sequence BIGINT,
    CONSTRAINT TABLE_NAME_parent FOREIGN KEY (parent_id)
	REFERENCES TABLE_NAME (id) 
	ON UPDATE NO ACTION
	ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS TABLE_NAME_p
    ON TABLE_NAME (parent_id);

CREATE UNIQUE INDEX IF NOT EXISTS TABLE_NAME_up
    ON TABLE_NAME
    (parent_id, key) WHERE parent_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS TABLE_NAME_unp
    ON TABLE_NAME (key) WHERE parent_id IS NULL;
`
	// Compose system table names.
	config := &common_sql.Config{
		DriverName: "sqlite",
		Dsn:        dsn,
		Timeout:    cfg.Timeout,
		SchemaStr:  s,
	}

	return common_sql.NewSqlBackend(ctx, config, prefix)
}
