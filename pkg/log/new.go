// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"log/slog"
	"strings"

	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

// use slog: https://betterstack.com/community/guides/logging/logging-in-go/

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

func AllLevels() []string {
	return []string{
		DebugLvl.String(),
		InfoLvl.String(),
		WarnLvl.String(),
		ErrorLvl.String(),
		FatalLvl.String(),
	}
}

func (l Level) String() string {
	switch l {
	case DebugLvl:
		return zapcore.DebugLevel.String()
	case InfoLvl:
		return zapcore.InfoLevel.String()
	case WarnLvl:
		return zapcore.WarnLevel.String()
	case ErrorLvl:
		return zapcore.ErrorLevel.String()
	case FatalLvl:
		return zapcore.FatalLevel.String()
	default:
		return "unknown"
	}
}

func ToLevel(rawLvl string) Level {
	l := strings.ToLower(rawLvl)
	switch l {
	case DebugLvl.String(), "-1":
		return DebugLvl
	case InfoLvl.String(), "0":
		return InfoLvl
	case WarnLvl.String(), "1":
		return WarnLvl
	case ErrorLvl.String(), "2":
		return ErrorLvl
	case FatalLvl.String(), "3":
		return FatalLvl
	default:
		Warnf("log level %q not available - defaulting to level %q", l, InfoLvl.String())
		return InfoLvl
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

	l := newLogger(o)

	l.Debug("logger initialized",
		slog.Group("options",
			slog.Any("lvl", o.level),
			slog.String("timeFormat", o.timeFormat),
			slog.Bool("jsonFormat", o.jsonFormat),
			slog.Bool("stacktrace", o.addStacktrace),
		),
	)
	return l
}

func newLogger(o *loggerOptions) *Logger {
	zapLogger := o.zapLogger()

	sLogger := slog.New(
		&CallerHandler{
			&ContextHandler{
				zapslog.NewHandler(
					zapLogger.Core(),
					zapslog.WithName(o.name),
					zapslog.WithCallerSkip(1),
				),
			},
			o.addCaller,
			0,
		},
	)
	return &Logger{sLogger, zapLogger, *o}
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
	r.addTarget = make([]zapcore.WriteSyncer, len(o.addTarget))
	copy(r.addTarget, o.addTarget)

	return &r
}
