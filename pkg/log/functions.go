package log

import (
	"context"
	"fmt"
	"log/slog"
)

const (
	// offset to skip the current function and the logger functions.
	callstackFunctionOffset = 3
)

func ensureLogger() {
	if defaultLogger == nil {
		SetDefaultLogger(WithLevel(InfoLvl), WithTextFormat(), SetStacktrace(true))
	}
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	ensureLogger()
	defaultLogger.DebugContext(ctx, msg, args...)
}

func Debug(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Debug(msg, args...)
}

func Debugf(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Debug(fmt.Sprintf(msg, args...))
}

func Info(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Info(msg, args...)
}

func Infof(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Info(fmt.Sprintf(msg, args...))
}

func Warn(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Warn(msg, args...)
}

func Warnf(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Warn(fmt.Sprintf(msg, args...))
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	ensureLogger()
	defaultLogger.ErrorContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Error(msg, args...)
}

func Errorf(msg string, args ...any) {
	ensureLogger()
	defaultLogger.Error(fmt.Sprintf(msg, args...))
}

func AddCaller() slog.Attr {
	fileV, funcV := prepareCallerValues(callstackFunctionOffset)

	return slog.Group(
		callerGroup,
		slog.String(fileKey, fileV),
		slog.String(funcKey, funcV),
	)
}
