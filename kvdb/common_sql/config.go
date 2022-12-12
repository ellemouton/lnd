package common_sql

import "time"

type Config struct {
	DriverName string
	Dsn        string
	Timeout    time.Duration
	SchemaStr  string
}
