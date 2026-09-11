// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Default *Logger

	defaultOptions = loggerOptions{
		level:         zap.InfoLevel,
		name:          "default",
		timeFormat:    time.RFC3339,
		jsonFormat:    true,
		addCaller:     true,
		addStacktrace: true,
		infoTarget:    os.Stdout,
		errTarget:     os.Stderr,
		addTarget:     make([]zapcore.WriteSyncer, 0),
	}
	defaultLoggerMutex = sync.RWMutex{}
)

// This package uses 1 init function to ensure the Default logger is present
func init() {
	Default = newLogger(&defaultOptions)
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

	Default = newLogger(o)
	slog.SetDefault(Default.Logger)
	Default.Debug("logger configured",
		slog.Group("options",
			slog.Any("lvl", o.level),
			slog.String("timeFormat", o.timeFormat),
			slog.Bool("jsonFormat", o.jsonFormat),
			slog.Bool("stacktrace", o.addStacktrace),
		),
	)
}
