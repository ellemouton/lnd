package build

import (
	"io"

	"github.com/btcsuite/btclog"
	"github.com/charmbracelet/lipgloss"
	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

const (
	traceColor    lipgloss.Color = "30"
	debugColor    lipgloss.Color = "63"
	infoColor     lipgloss.Color = "86"
	warnColor     lipgloss.Color = "192"
	errorColor    lipgloss.Color = "204"
	criticalColor lipgloss.Color = "134"

	traceTag    = "[TRC]"
	debugTag    = "[DBG]"
	infoTag     = "[INF]"
	warnTag     = "[WRN]"
	errorTag    = "[ERR]"
	criticalTag = "[CRT]"

	defaultTimeFormat = "2006-01-02 15:04:05.000"
)

// HandlerOptions is the signature of a functional option that can be used to
// change default settings of our log Handlers.
type HandlerOptions func(*handlerOptions)

// handlerOptions holds various Handler configurations.
type handlerOptions struct {
	reportTimestamp bool
	timeFormat      string
	color           Color
	style           *charm.Styles
}

// WithTimestamp is a functional option that can be used to adjust whether a
// timestamp should be included in logs.
func WithTimestamp(report bool) HandlerOptions {
	return func(opts *handlerOptions) {
		opts.reportTimestamp = report
	}
}

// WithColor is a functional option that can be used to change the default log
// output colour profile.
func WithColor(color Color) HandlerOptions {
	return func(opts *handlerOptions) {
		opts.color = color
	}
}

// buildHandler uses a handlerOptions to construct a Handler.
func (o *handlerOptions) buildHandler(w io.Writer) Handler {
	handler := charm.NewWithOptions(w, charm.Options{
		TimeFormat:      o.timeFormat,
		ReportTimestamp: o.reportTimestamp,
	})
	handler.SetStyles(o.style)
	handler.SetColorProfile(o.color.profile())

	return &charmHandler{handler}
}

// defaultConsoleOptions returns the default handlerOptions that we will use
// for console logs.
func defaultConsoleOptions() *handlerOptions {
	return &handlerOptions{
		reportTimestamp: true,
		timeFormat:      defaultTimeFormat,
		color:           ColorAuto,
		style:           defaultConsoleStyle(),
	}
}

// defaultLogFileOptions returns the default handlerOptions that we will use
// for log file logs.
func defaultLogFileOptions() *handlerOptions {
	return &handlerOptions{
		reportTimestamp: true,
		timeFormat:      defaultTimeFormat,
		color:           ColorOff,
		style:           defaultLogFileStyle(),
	}
}

// NewConsoleHandler creates a new Handler implementation with colour formatting
// which will render in a terminal that outputs logs in the logfmt format.
func NewConsoleHandler(w io.Writer, opts ...HandlerOptions) Handler {
	options := defaultConsoleOptions()
	for _, o := range opts {
		o(options)
	}

	return options.buildHandler(w)
}

// NewLogFileHandler creates a Handler implementation without any colour
// formatting that outputs logs in the logfmt format.
func NewLogFileHandler(w io.Writer, opts ...HandlerOptions) Handler {
	options := defaultLogFileOptions()
	for _, o := range opts {
		o(options)
	}

	return options.buildHandler(w)
}

// charmHandler holds a charm.Logger handler and adds some methods that can
// such that it implements the Handler interface.
type charmHandler struct {
	*charm.Logger
}

// A compile-time check to ensure that charmHandler implements Handler.
var _ Handler = (*charmHandler)(nil)

// Subsystem creates a new Handler with the given subsytem tag.
//
// NOTE: this is part of the Handler interface.
func (c *charmHandler) Subsystem(tag string) Handler {
	return &charmHandler{c.WithPrefix(tag)}
}

// SetLevel changes the logging level of the Handler to the passed level.
// It does so by converting the given btclog.Level to a charm.Level and then
// calling the charm handler's SetLevel method.
//
// NOTE: this is part of the btclog.Handler interface.
func (c *charmHandler) SetLevel(level btclog.Level) {
	l := charm.InfoLevel
	switch level {
	case btclog.LevelTrace:
		l = charm.Level(btclog.LevelTrace)
	case btclog.LevelDebug:
		l = charm.DebugLevel
	case btclog.LevelInfo:
		l = charm.InfoLevel
	case btclog.LevelWarn:
		l = charm.WarnLevel
	case btclog.LevelError:
		l = charm.ErrorLevel
	case btclog.LevelCritical:
		l = charm.Level(btclog.LevelCritical)
	case btclog.LevelOff:
	}
	c.Logger.SetLevel(l)
}

// Level returns the current logging level of the Handler. It does so by
// converting the charm handler's Level to the associated btclog.Level.
//
// NOTE: this is part of the btclog.Handler interface.
func (c *charmHandler) Level() btclog.Level {
	switch c.Logger.GetLevel() {
	case charm.DebugLevel:
		return btclog.LevelDebug
	case charm.InfoLevel:
		return btclog.LevelInfo
	case charm.WarnLevel:
		return btclog.LevelWarn
	case charm.ErrorLevel:
		return btclog.LevelError
	case charm.Level(btclog.LevelTrace):
		return btclog.LevelTrace
	case charm.Level(btclog.LevelCritical):
		return btclog.LevelCritical
	}

	return btclog.LevelOff
}

// Color determines the color output of a log for when TTY is supported.
type Color string

const (
	// ColorOff will log un-colored text.
	ColorOff Color = "off"

	// ColorAuto will use the color profile supported by the OS.
	ColorAuto Color = "auto"

	// ColorForce will force a 24-bit color profile.
	ColorForce Color = "force"
)

// profile converts a Color into the associated termenv.Profile type.
func (c Color) profile() termenv.Profile {
	switch c {
	case ColorAuto:
		return termenv.ColorProfile()
	case ColorForce:
		return termenv.TrueColor
	default:
		return termenv.Ascii
	}
}

func defaultConsoleStyle() *charm.Styles {
	style := charm.DefaultStyles()
	style.Levels[charm.Level(btclog.LevelTrace)] = lipgloss.NewStyle().
		SetString(traceTag).
		Bold(true).
		MaxWidth(5).
		Foreground(traceColor)
	style.Levels[charm.Level(btclog.LevelDebug)] = lipgloss.NewStyle().
		SetString(debugTag).
		Bold(true).
		MaxWidth(5).
		Foreground(debugColor)
	style.Levels[charm.Level(btclog.LevelInfo)] = lipgloss.NewStyle().
		SetString(infoTag).
		Bold(true).
		MaxWidth(5).
		Foreground(infoColor)
	style.Levels[charm.Level(btclog.LevelWarn)] = lipgloss.NewStyle().
		SetString(warnTag).
		Bold(true).
		MaxWidth(5).
		Foreground(warnColor)
	style.Levels[charm.Level(btclog.LevelError)] = lipgloss.NewStyle().
		SetString(errorTag).
		Bold(true).
		MaxWidth(5).
		Foreground(errorColor)
	style.Levels[charm.Level(btclog.LevelCritical)] = lipgloss.NewStyle().
		SetString(criticalTag).
		Bold(true).
		MaxWidth(5).
		Foreground(criticalColor)

	return style
}

func defaultLogFileStyle() *charm.Styles {
	style := charm.DefaultStyles()
	style.Levels[charm.Level(btclog.LevelTrace)] = lipgloss.NewStyle().
		SetString(traceTag)
	style.Levels[charm.Level(btclog.LevelDebug)] = lipgloss.NewStyle().
		SetString(debugTag)
	style.Levels[charm.Level(btclog.LevelInfo)] = lipgloss.NewStyle().
		SetString(infoTag)
	style.Levels[charm.Level(btclog.LevelWarn)] = lipgloss.NewStyle().
		SetString(warnTag)
	style.Levels[charm.Level(btclog.LevelError)] = lipgloss.NewStyle().
		SetString(errorTag)
	style.Levels[charm.Level(btclog.LevelCritical)] = lipgloss.NewStyle().
		SetString(criticalTag)

	return style
}
