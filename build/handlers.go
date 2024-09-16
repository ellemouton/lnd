package build

import (
	"fmt"
	"os"
	"strings"

	"github.com/btcsuite/btclog"
)

const (
	resetSeq = "0"
	boldSeq  = "1"
	faintSeq = "2"

	esc = '\x1b'
	csi = string(esc) + "["
)

// NewDefaultLoggers returns the standard console logger and rotating log
// writer loggers that we generally want to use. It also applies the various
// config options to the loggers.
func NewDefaultLoggers(cfg *LogConfig, rotator *RotatingLogWriter) (
	btclog.Handler, btclog.Handler) {

	var (
		consoleOpts []btclog.HandlerOption
		fileOpts    []btclog.HandlerOption
	)
	if cfg.File.NoTimestamps {
		fileOpts = append(fileOpts, btclog.WithNoTimestamp())
	}
	if cfg.Console.NoTimestamps {
		consoleOpts = append(consoleOpts, btclog.WithNoTimestamp())
	}
	if cfg.Console.Style {
		consoleOpts = append(consoleOpts,
			btclog.WithStyledLevel(
				func(l btclog.Level) string {
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

	consoleLogHandler := btclog.NewDefaultHandler(os.Stdout, consoleOpts...)
	logFileHandler := btclog.NewDefaultHandler(rotator, fileOpts...)

	return consoleLogHandler, logFileHandler
}

func styleString(s string, styles ...string) string {
	if len(styles) == 0 {
		return s
	}

	seq := strings.Join(styles, ";")
	if seq == "" {
		return s
	}

	return fmt.Sprintf("%s%sm%s%sm", csi, seq, s, csi+resetSeq)
}

type ansiColorSeq string

const (
	ansiColorSeqDarkTeal  ansiColorSeq = "38;5;30"
	ansiColorSeqDarkBlue  ansiColorSeq = "38;5;63"
	ansiColorSeqLightBlue ansiColorSeq = "38;5;86"
	ansiColorSeqYellow    ansiColorSeq = "38;5;192"
	ansiColorSeqRed       ansiColorSeq = "38;5;204"
	ansiColorSeqPink      ansiColorSeq = "38;5;134"
)

func ansiColoSeq(l btclog.Level) ansiColorSeq {
	switch l {
	case btclog.LevelTrace:
		return ansiColorSeqDarkTeal
	case btclog.LevelDebug:
		return ansiColorSeqDarkBlue
	case btclog.LevelWarn:
		return ansiColorSeqYellow
	case btclog.LevelError:
		return ansiColorSeqRed
	case btclog.LevelCritical:
		return ansiColorSeqPink
	default:
		return ansiColorSeqLightBlue
	}
}
