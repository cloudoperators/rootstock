// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (o *loggerOptions) zapLogger() *zap.Logger {
	// High-priority output should also go to standard error, and low-priority
	// output should also go to standard out.
	// It is useful for Kubernetes deployment.
	// Kubernetes interprets os.Stdout log items as INFO and os.Stderr log items
	// as ERROR by default.
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= o.level && lvl < zapcore.ErrorLevel
	})

	ecfg := zapcore.EncoderConfig{
		NameKey: "logger",

		TimeKey:     "ts",
		EncodeTime:  zapcore.ISO8601TimeEncoder,
		LevelKey:    "lvl",
		EncodeLevel: zapcore.CapitalColorLevelEncoder,

		MessageKey:     "msg",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}

	if o.addStacktrace {
		ecfg.StacktraceKey = "stacktrace"
	}

	if o.timeFormat != "" {
		ecfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format(o.timeFormat))
		}
	}

	var consoleEncoder zapcore.Encoder

	if o.jsonFormat {
		ecfg.EncodeLevel = zapcore.CapitalLevelEncoder
		consoleEncoder = zapcore.NewJSONEncoder(ecfg)
	} else {
		consoleEncoder = zapcore.NewConsoleEncoder(ecfg)
	}

	// Join the outputs, encoders, and level-handling functions into
	// zapcore.
	//nolint:mnd // 2 writers: error and info and 2 times writers for each additional target
	cores := make([]zapcore.Core, 0, 2*len(o.addTarget)+2)
	cores = append(cores,
		zapcore.NewCore(consoleEncoder, o.infoTarget, lowPriority),
		zapcore.NewCore(consoleEncoder, o.errTarget, highPriority),
	)

	for _, target := range o.addTarget {
		cores = append(cores,
			zapcore.NewCore(consoleEncoder, target, highPriority),
			zapcore.NewCore(consoleEncoder, target, lowPriority),
		)
	}

	// From a zapcore.Core, it's easy to construct a Logger.
	res := zap.New(
		zapcore.NewTee(cores...),
		zap.AddCaller(),
		zap.AddCallerSkip(0),
	)
	zap.RedirectStdLog(res)

	return res
}
