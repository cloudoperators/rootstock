// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0
//
//nolint:depguard // log pkg dependencies allowed in the log package
package log

import (
	"context"
	"fmt"
	"log/slog"
)

func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

func Debugf(msg string, args ...any) {
	Default().Debug(fmt.Sprintf(msg, args...))
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

func Infof(msg string, args ...any) {
	Default().Info(fmt.Sprintf(msg, args...))
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

func Warnf(msg string, args ...any) {
	Default().Warn(fmt.Sprintf(msg, args...))
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

func Errorf(msg string, args ...any) {
	Default().Error(fmt.Sprintf(msg, args...))
}

func AddCaller() slog.Attr {
	// offset to skip the current function and the logger functions.
	const callstackFunctionOffset = 3

	fileV, funcV := prepareCallerValues(callstackFunctionOffset)

	return slog.Group(
		callerGroup,
		slog.String(fileKey, fileV),
		slog.String(funcKey, funcV),
	)
}
