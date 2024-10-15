//go:build dev
// +build dev

package build

import (
	"fmt"

	btclogv1 "github.com/btcsuite/btclog"
	"github.com/btcsuite/btclog/v2"
)

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

// HandlerOptions returns the set of btclog.HandlerOptions that the state of the
// config struct translates to.
func (cfg *LoggerConfig) HandlerOptions() []btclog.HandlerOption {
	var opts []btclog.HandlerOption
	if cfg.NoTimestamps {
		opts = append(opts, btclog.WithNoTimestamp())
	}

	return opts
}

// consoleLoggerCfg extends the LoggerConfig struct by adding a Color option
// which is only available for a console logger.
//
//nolint:lll
type consoleLoggerCfg struct {
	LoggerConfig
	Style bool `long:"style" description:"If set, the output will be styled with color and fonts"`
}

// HandlerOptions returns the set of btclog.HandlerOptions that the state of the
// config struct translates to.
func (cfg *consoleLoggerCfg) HandlerOptions() []btclog.HandlerOption {
	opts := cfg.LoggerConfig.HandlerOptions()

	if !cfg.Style {
		return opts
	}

	return append(opts,
		btclog.WithStyledLevel(
			func(l btclogv1.Level) string {
				return styleString(
					fmt.Sprintf("[%s]", l),
					boldSeq,
					string(ansiColoSeq(l)),
				)
			},
		),
		btclog.WithStyledCallSite(
			func(file string, line int) string {
				str := fmt.Sprintf("%s:%d", file, line)

				return styleString(str, faintSeq)
			},
		),
		btclog.WithStyledKeys(func(key string) string {
			return styleString(key, faintSeq)
		}),
	)
}

// DefaultLogConfig returns the default logging config options.
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		Console: &consoleLoggerCfg{},
		File:    &LoggerConfig{},
	}
}
