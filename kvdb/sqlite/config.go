package sqlite

import "time"

// Config holds sqlite configuration data.
//
//nolint:lll
type Config struct {
	Timeout time.Duration `long:"timeout" description:"Database connection timeout. Set to zero to disable."`
}
