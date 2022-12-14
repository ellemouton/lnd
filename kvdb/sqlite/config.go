package sqlite

import "time"

// Config holds sqlite configuration data.
//
//nolint:lll
type Config struct {
	DBPath  string        `long:"dbpath" description:"The path to the sqlite database file."`
	Timeout time.Duration `long:"timeout" description:"Database connection timeout. Set to zero to disable."`
}
