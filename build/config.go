package build

// LogConfig holds logging configuration options.
//
//nolint:lll
type LogConfig struct {
	Console *consoleLoggerCfg `group:"console" namespace:"console" description:"The logger writing to stdout and stderr."`
	File    *LoggerConfig     `group:"file" namespace:"file" description:"The logger writing to LND's standard log file."`
}

// LoggerConfig holds options for a particular logger.
//
//nolint:lll
type LoggerConfig struct {
	Disable      bool `long:"disable" description:"Disable this logger."`
	NoTimestamps bool `long:"no-timestamps" description:"Omit timestamps from log lines."`
}

// consoleLoggerCfg extends the LoggerConfig struct by adding a Color option
// which is only available for a console logger.
type consoleLoggerCfg struct {
	LoggerConfig
	Color Color `long:"color" description:"Log color formatting options. By default, the color profile is off." choice:"off" choice:"auto" choice:"force"`
}

// DefaultLogConfig returns the default logging config options.
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		Console: &consoleLoggerCfg{
			Color: ColorOff,
		},
		File: &LoggerConfig{},
	}
}
