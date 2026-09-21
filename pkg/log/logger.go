// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// obtained from https://gist.github.com/calam1/c2673b6b0a53918df033d71bbf958b56

package log

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"

	"go.uber.org/zap"
)

type Logger struct {
	*slog.Logger

	core    *zap.Logger
	options loggerOptions
}

func (l *Logger) Sync() {
	if l.core == nil {
		return
	}

	if err := l.core.Sync(); err != nil && !errors.Is(err, syscall.ENOTTY) && !errors.Is(err, syscall.EINVAL) {
		fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
	}
}

func (l *Logger) New(options ...Option) FullInterface {
	opts := l.options
	for _, option := range options {
		option(&opts)
	}

	return newLogger(&opts)
}

func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Logger.DebugContext(ctx, msg, args...)
}

func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

func (l *Logger) Debugf(msg string, args ...any) {
	l.Logger.Debug(fmt.Sprintf(msg, args...))
}

func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Logger.InfoContext(ctx, msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

func (l *Logger) Infof(msg string, args ...any) {
	l.Logger.Info(fmt.Sprintf(msg, args...))
}

func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Logger.WarnContext(ctx, msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

func (l *Logger) Warnf(msg string, args ...any) {
	l.Logger.Warn(fmt.Sprintf(msg, args...))
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Logger.ErrorContext(ctx, msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

func (l *Logger) Errorf(msg string, args ...any) {
	l.Logger.Error(fmt.Sprintf(msg, args...))
}

func (l *Logger) With(args ...any) FullInterface {
	return &Logger{
		Logger:  l.Logger.With(args...),
		core:    l.core,
		options: l.options,
	}
}
