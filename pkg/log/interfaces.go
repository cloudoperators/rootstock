// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"context"
	"log/slog"
)

type MinInterface interface {
	// Info logs with optional key-value pairs for additional information.
	Info(msg string, args ...any)
	// Error logs with optional key-value pairs for additional information.
	Error(msg string, args ...any)
	// Warn logs with optional key-value pairs for additional information.
	Warn(msg string, args ...any)
	// Debug logs with optional key-value pairs for additional information.
	Debug(msg string, args ...any)
	// Log logs with context, level, message, and optional key-value pairs for additional information.
	Log(ctx context.Context, level slog.Level, msg string, args ...any)
	// LogAttrs logs with context, level, message, and optional attributes for additional information.
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
}

type ContextInterface interface {
	MinInterface

	// InfoContext logs with context and optional key-value pairs for additional information.
	InfoContext(ctx context.Context, msg string, args ...any)
	// ErrorContext logs with context and optional key-value pairs for additional information.
	ErrorContext(ctx context.Context, msg string, args ...any)
	// WarnContext logs with context and optional key-value pairs for additional information.
	WarnContext(ctx context.Context, msg string, args ...any)
	// DebugContext logs with context and optional key-value pairs for additional information.
	DebugContext(ctx context.Context, msg string, args ...any)
}

type FullInterface interface {
	MinInterface
	ContextInterface

	// Enabled checks if the logger is enabled for the given context and level.
	Enabled(ctx context.Context, level slog.Level) bool
	// Handler returns the slog.Handler used by the logger.
	Handler() slog.Handler
	// With returns a new logger with the given key-value pairs.
	With(args ...any) FullInterface
	// WithGroup returns a new logger with the given group name.
	WithGroup(name string) *slog.Logger
	// Sync flushes any buffered log entries to the underlying output.
	Sync()
	// New returns a child logger from the existing logger interface.
	New(options ...Option) FullInterface
	// Update allows to dynamically update the options for an existing logger.
	Update(options ...Option)
}
