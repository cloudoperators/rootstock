// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"go.uber.org/zap/zapcore"
)

type Option func(*loggerOptions)

// With sets multiple options for the logger.
func With(
	l Level,
	name string,
	timeFormat string,
	textFormat bool,
	addCaller bool,
	addStacktrace bool,
	infoTarget, errorTarget WriteSyncer,
	addTargets ...WriteSyncer,
) Option {
	at := make([]zapcore.WriteSyncer, 0, len(addTargets))
	for _, t := range addTargets {
		at = append(at, t)
	}

	return func(o *loggerOptions) {
		o.level = zapcore.Level(l)
		o.name = name
		o.timeFormat = timeFormat
		o.jsonFormat = !textFormat
		o.addCaller = addCaller
		o.addStacktrace = addStacktrace
		o.infoTarget = infoTarget
		o.errTarget = errorTarget
		o.addTarget = at
	}
}

// WithLevel sets the log level.
func WithLevel(l Level) Option {
	return func(o *loggerOptions) {
		o.level = zapcore.Level(l)
	}
}

// WithName sets the logger name.
func WithName(name string) Option {
	return func(o *loggerOptions) {
		o.name = name
	}
}

// WithTimeFormat sets the time format.
func WithTimeFormat(format string) Option {
	return func(o *loggerOptions) {
		o.timeFormat = format
	}
}

// WithTextFormat sets the log format to text (default is json).
func WithTextFormat() Option {
	return func(o *loggerOptions) {
		o.jsonFormat = false
	}
}

func WithFormat(format Format) Option {
	return func(o *loggerOptions) {
		switch format {
		case TextFormat:
			o.jsonFormat = false
		case JSONFormat:
			o.jsonFormat = true
		default:
			o.jsonFormat = true // Default to JSON format
		}
	}
}

// WithoutCaller removes the caller.
func WithoutCaller() Option {
	return func(o *loggerOptions) {
		o.addCaller = false
	}
}

// SetStacktrace sets the stacktrace.
func SetStacktrace(enabled bool) Option {
	return func(o *loggerOptions) {
		o.addStacktrace = enabled
	}
}

// WithInfoTarget sets the info target.
func WithInfoTarget(target WriteSyncer) Option {
	return func(o *loggerOptions) {
		o.infoTarget = target
	}
}

// WithErrorTarget sets the error target.
func WithErrorTarget(target WriteSyncer) Option {
	return func(o *loggerOptions) {
		o.errTarget = target
	}
}

// WithAdditionalTargets sets additional log targets.
func WithAdditionalTargets(targets ...WriteSyncer) Option {
	at := make([]zapcore.WriteSyncer, 0, len(targets))
	for _, t := range targets {
		at = append(at, t)
	}

	return func(o *loggerOptions) {
		o.addTarget = at
	}
}
