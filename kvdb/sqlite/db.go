//go:build kvdb_sqlite
// +build kvdb_sqlite

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	_ "modernc.org/sqlite" // Register relevant drivers.
)

const (
	// sqliteOptionPrefix is the string prefix sqlite uses to set various
	// options. This is used in the following format:
	//   * sqliteOptionPrefix || option_name = option_value.
	sqliteOptionPrefix = "_pragma"

	// sqliteTxLockImmediate is dsn option used to ensure that write
	// transactions are started immediately.
	sqliteTxLockImmediate = "_txlock=immediate"
)

// NewSqliteBackend returns a db object initialized with the passed backend
// config. If a sqlite connection cannot be established, then an error is
// returned.
func NewSqliteBackend(ctx context.Context, cfg *Config, fileName,
	prefix string) (walletdb.DB, error) {

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
	writeDSN := fmt.Sprintf(
		"%v?%v&%v", filepath.Join(cfg.DBPath, fileName),
		sqliteOptions.Encode(), sqliteTxLockImmediate,
	)
	writeCfg := &sqlbase.Config{
		DriverName:      "sqlite",
		Dsn:             writeDSN,
		Timeout:         cfg.Timeout,
		TableNamePrefix: prefix,
	}

	writeDB, err := sqlbase.NewSqlBackend(ctx, writeCfg)
	if err != nil {
		return nil, err
	}

	readDSN := fmt.Sprintf(
		"%v?%v", filepath.Join(cfg.DBPath, fileName),
		sqliteOptions.Encode(),
	)
	readCfg := &sqlbase.Config{
		DriverName:      "sqlite",
		Dsn:             readDSN,
		Timeout:         cfg.Timeout,
		TableNamePrefix: prefix,
	}

	readDB, err := sqlbase.NewSqlBackend(ctx, readCfg)
	if err != nil {
		return nil, err
	}

	return &sqliteDB{
		readDBConn:  readDB,
		writeDBConn: writeDB,
	}, nil
}

// sqliteDB contains two sqlite connections, one is optimised for reads as it
// uses the default BEGIN DEFERRED directive for starting a db transaction and
// the other is optimised for writes as it uses the BEGIN IMMEDIATE directive
// for starting a db transaction.
type sqliteDB struct {
	readDBConn  walletdb.DB
	writeDBConn walletdb.DB
}

// BeginReadTx opens a database read transaction.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) BeginReadTx() (walletdb.ReadTx, error) {
	return s.readDBConn.BeginReadTx()
}

// BeginReadWriteTx opens a database read+write transaction.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) BeginReadWriteTx() (walletdb.ReadWriteTx, error) {
	return s.writeDBConn.BeginReadWriteTx()
}

// Copy is a no-op.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) Copy(_ io.Writer) error {
	return errors.New("not implemented")
}

// Close closes both the write and read connections.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) Close() error {
	var returnErr error

	if err := s.writeDBConn.Close(); err != nil {
		returnErr = err
	}

	if err := s.readDBConn.Close(); err != nil {
		returnErr = err
	}

	return returnErr
}

// PrintStats returns all collected stats pretty printed into a string.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) PrintStats() string {
	return "stats not supported by SQLite driver"
}

// View opens a database read transaction and executes the function f with the
// transaction passed as a parameter. After f exits, the transaction is rolled
// back. If f errors, its error is returned, not a rollback error (if any
// occur). The passed reset function is called before the start of the
// transaction and can be used to reset intermediate state. As callers may
// expect retries of the f closure (depending on the database backend used),
// the reset function will be called before each retry respectively.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) View(f func(tx walletdb.ReadTx) error,
	reset func()) error {

	return s.readDBConn.View(f, reset)
}

// Update opens a database read/write transaction and executes the function f
// with the transaction passed as a parameter. After f exits, if f did not
// error, the transaction is committed. Otherwise, if f did error, the
// transaction is rolled back. If the rollback fails, the original error
// returned by f is still returned. If the commit fails, the commit error is
// returned. As callers may expect retries of the f closure (depending on the
// database backend used), the reset function will be called before each retry
// respectively.
//
// NOTE: this is part of the walletdb.DB interface.
func (s *sqliteDB) Update(f func(tx walletdb.ReadWriteTx) error,
	reset func()) error {

	return s.writeDBConn.Update(f, reset)
}

// A compile-time check to ensure that sqliteDB implements the walletdb.DB
// interface.
var _ walletdb.DB = (*sqliteDB)(nil)
