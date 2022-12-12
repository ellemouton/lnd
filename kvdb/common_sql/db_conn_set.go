//go:build kvdb_postgres || kvdb_sqlite
// +build kvdb_postgres kvdb_sqlite

package common_sql

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/jackc/pgx/v4/stdlib"
)

// dbConn stores the actual connection and a user count.
type dbConn struct {
	db    *sql.DB
	count int
}

// dBConnSet stores a set of connections.
type dBConnSet struct {
	dbConn         map[string]*dbConn
	maxConnections int

	sync.Mutex
}

// newDbConnSet initializes a new set of connections.
func newDbConnSet(maxConnections int) *dBConnSet {
	return &dBConnSet{
		dbConn:         make(map[string]*dbConn),
		maxConnections: maxConnections,
	}
}

// Open opens a new database connection. If a connection already exists for the
// given dsn, the existing connection is returned.
func (d *dBConnSet) Open(driver, dsn string) (*sql.DB, error) {
	d.Lock()
	defer d.Unlock()

	if dbConn, ok := d.dbConn[dsn]; ok {
		dbConn.count++

		return dbConn.db, nil
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	// Limit maximum number of open connections. This is useful to prevent
	// the server from running out of connections and returning an error.
	// With this client-side limit in place, lnd will wait for a connection
	// to become available.
	if d.maxConnections != 0 {
		db.SetMaxOpenConns(d.maxConnections)
	}

	d.dbConn[dsn] = &dbConn{
		db:    db,
		count: 1,
	}

	return db, nil
}

// Close closes the connection with the given dsn. If there are still other
// users of the same connection, this function does nothing.
func (d *dBConnSet) Close(dsn string) error {
	d.Lock()
	defer d.Unlock()

	dbConn, ok := d.dbConn[dsn]
	if !ok {
		return fmt.Errorf("connection not found: %v", dsn)
	}

	// Reduce user count.
	dbConn.count--

	// Do not close if there are other users.
	if dbConn.count > 0 {
		return nil
	}

	// Close connection.
	delete(d.dbConn, dsn)

	return dbConn.db.Close()
}
