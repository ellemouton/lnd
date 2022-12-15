package sqlite

import "time"

// Config holds sqlite configuration data.
//
//nolint:lll
type Config struct {
	DBPath         string        `long:"dbpath" description:"The path to the sqlite database file."`
	Timeout        time.Duration `long:"timeout" description:"The time after which a database query should be timed out."`
	BusyTimeout    time.Duration `long:"busytimeout" description:"The maximum amount of time to wait for a database connection to become available for a query."`
	MaxConnections int           `long:"maxconnections" description:"The maximum number of open connections to the database. Set to zero for unlimited."`
}
