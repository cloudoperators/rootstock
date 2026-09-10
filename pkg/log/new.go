package log

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

// use slog: https://betterstack.com/community/guides/logging/logging-in-go/

var (
	defaultLogger *Logger

	defaultOptions = loggerOptions{
		level:         zap.DebugLevel,
		name:          "default",
		timeFormat:    time.RFC3339,
		jsonFormat:    true,
		addCaller:     true,
		addStacktrace: false,
		infoTarget:    os.Stdout,
		errTarget:     os.Stderr,
		addTarget:     make([]zapcore.WriteSyncer, 0),
	}
	defaultLoggerMutex = sync.RWMutex{}
)

const (
	DebugLvl Level = Level(zapcore.DebugLevel)
	InfoLvl  Level = Level(zapcore.InfoLevel)
	WarnLvl  Level = Level(zapcore.WarnLevel)
	ErrorLvl Level = Level(zapcore.ErrorLevel)
	FatalLvl Level = Level(zapcore.FatalLevel)

	JSONFormat Format = "json"
	TextFormat Format = "text"
)

type (
	Level  zapcore.Level
	Format string
)

func (l Level) String() string {
	switch l {
	case DebugLvl:
		return zapcore.DebugLevel.CapitalString()
	case InfoLvl:
		return zapcore.InfoLevel.CapitalString()
	case WarnLvl:
		return zapcore.WarnLevel.CapitalString()
	case ErrorLvl:
		return zapcore.ErrorLevel.CapitalString()
	case FatalLvl:
		return zapcore.FatalLevel.CapitalString()
	default:
		return "unknown"
	}
}

// A WriteSyncer is an [io.Writer] that can also flush any buffered data. Note
// that *[os.File] (and thus, [os.Stderr] and [os.Stdout]) implement WriteSyncer.
type WriteSyncer interface {
	zapcore.WriteSyncer
}

func New(options ...Option) FullInterface {
	// Default configuration
	o := defaultOptions.DeepCopy()
	// Apply provided options
	for _, option := range options {
		option(o)
	}

	return newLogger(o)
}

func newLogger(o *loggerOptions) *Logger {
	zapLogger := o.zapLogger()

	h := &CallerHandler{
		&ContextHandler{zapslog.NewHandler(
			zapLogger.Core(),
			zapslog.WithName(o.name),
			zapslog.WithCallerSkip(1),
		)},
		o.addCaller,
		1,
	}
	l := Logger{slog.New(h), zapLogger, *o}

	l.Debug("logger initialized",
		slog.Group("options",
			slog.Any("lvl", o.level),
			slog.String("timeFormat", o.timeFormat),
			slog.Bool("jsonFormat", o.jsonFormat),
			slog.Bool("strackTrace", o.addStacktrace),
		),
	)

	return &l
}

func SetDefaultLogger(options ...Option) {
	// Default configuration
	o := defaultOptions.DeepCopy()
	// Apply provided options
	for _, option := range options {
		option(o)
	}

	defaultLoggerMutex.Lock()
	defer defaultLoggerMutex.Unlock()

	defaultLogger = newLogger(o)
	slog.SetDefault(defaultLogger.Logger)
}

// Default returns the default logger. If not set, it will create a new logger with
// default options.
func Default() *Logger {
	defaultLoggerMutex.RLock()

	if defaultLogger != nil {
		defaultLoggerMutex.RUnlock()
		return defaultLogger
	}

	defaultLoggerMutex.RUnlock()

	defaultLoggerMutex.Lock()
	defer defaultLoggerMutex.Unlock()

	if defaultLogger == nil {
		// Avoid rewriting the default logger on race condition
		// Default options have remote audit disabled, so newLogger should not fail.
		logger := newLogger(&defaultOptions)

		defaultLogger = logger
		defaultLogger.Error("default logger not initialized - using default values")
	}

	return defaultLogger
}

type loggerOptions struct {
	level         zapcore.Level
	name          string
	timeFormat    string
	jsonFormat    bool
	addCaller     bool
	addStacktrace bool
	infoTarget    zapcore.WriteSyncer
	errTarget     zapcore.WriteSyncer
	addTarget     []zapcore.WriteSyncer
}

func (o *loggerOptions) DeepCopy() *loggerOptions {
	r := loggerOptions{}
	r.level = o.level
	r.name = o.name
	r.timeFormat = o.timeFormat
	r.jsonFormat = o.jsonFormat
	r.addCaller = o.addCaller
	r.addStacktrace = o.addStacktrace
	r.infoTarget = o.infoTarget
	r.errTarget = o.errTarget
	r.addTarget = make([]zapcore.WriteSyncer, 0, len(o.addTarget))
	copy(r.addTarget, o.addTarget)

	return &r
}
