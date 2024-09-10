package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jrick/logrotate/rotator"
)

// RotatingLogWriter is a wrapper around the LogWriter that supports log file
// rotation.
type RotatingLogWriter struct {
	logWriter *LogWriter

	rotator *rotator.Rotator
}

// NewRotatingLogWriter creates a new file rotating log writer.
//
// NOTE: `InitLogRotator` must be called to set up log rotation after creating
// the writer.
func NewRotatingLogWriter(w *LogWriter) *RotatingLogWriter {
	return &RotatingLogWriter{logWriter: w}
}

// InitLogRotator initializes the log file rotator to write logs to logFile and
// create roll files in the same directory. It should be called as early on
// startup and possible and must be closed on shutdown by calling `Close`.
func (r *RotatingLogWriter) InitLogRotator(logFile string, maxLogFileSize int,
	maxLogFiles int) error {

	logDir, _ := filepath.Split(logFile)
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	r.rotator, err = rotator.New(
		logFile, int64(maxLogFileSize*1024), false, maxLogFiles,
	)
	if err != nil {
		return fmt.Errorf("failed to create file rotator: %w", err)
	}

	// Run rotator as a goroutine now but make sure we catch any errors
	// that happen in case something with the rotation goes wrong during
	// runtime (like running out of disk space or not being allowed to
	// create a new logfile for whatever reason).
	pr, pw := io.Pipe()
	go func() {
		err := r.rotator.Run(pr)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr,
				"failed to run file rotator: %v\n", err)
		}
	}()

	r.logWriter.RotatorPipe = pw

	return nil
}

// Close closes the underlying log rotator if it has already been created.
func (r *RotatingLogWriter) Close() error {
	if r.rotator != nil {
		return r.rotator.Close()
	}
	return nil
}
