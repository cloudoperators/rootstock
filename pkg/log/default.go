// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	atomicDefault atomic.Pointer[Logger]

	defaultOptions = loggerOptions{
		level:         zap.InfoLevel,
		name:          "default",
		timeFormat:    time.RFC3339,
		jsonFormat:    true,
		addCaller:     true,
		callerOffset:  0,
		addStacktrace: true,
		infoTarget:    os.Stdout,
		errTarget:     os.Stderr,
		addTarget:     make([]zapcore.WriteSyncer, 0),
	}
)

// Default returns the current default logger. Safe for concurrent use.
func Default() *Logger {
	return atomicDefault.Load()
}

// SetDefault replaces the default logger with l. Safe for concurrent use.
func SetDefault(l *Logger) {
	atomicDefault.Store(l)
}

// This package uses 1 init function to ensure the Default logger is present.
func init() {
	atomicDefault.Store(newLogger(&defaultOptions))
}

func SetDefaultLogger(options ...Option) {
	o := defaultOptions.DeepCopy()
	for _, option := range options {
		option(o)
	}

	l := newLogger(o)
	atomicDefault.Store(l)
	slog.SetDefault(l.Logger)
	l.Debug("default logger configured",
		slog.Group("options",
			slog.Any("lvl", o.level),
			slog.String("timeFormat", o.timeFormat),
			slog.Bool("jsonFormat", o.jsonFormat),
			slog.Bool("stacktrace", o.addStacktrace),
		),
	)
}
