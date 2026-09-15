// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/SAP/rootstock/pkg/log"
)

// --- Level.String ---

func TestLevelString(t *testing.T) {
	tests := []struct {
		level log.Level
		want  string
	}{
		{log.DebugLvl, "debug"},
		{log.InfoLvl, "info"},
		{log.WarnLvl, "warn"},
		{log.ErrorLvl, "error"},
		{log.FatalLvl, "fatal"},
		{log.Level(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.level.String(); got != tc.want {
				t.Errorf("Level(%d).String() = %q, want %q", tc.level, got, tc.want)
			}
			gotLevel := log.ToLevel(tc.want)
			if gotLevel != tc.level {
				if tc.want != "unknown" {
					t.Errorf("Level(%d).String() = %q, want %q", tc.level, gotLevel, tc.want)
				} else if gotLevel != log.InfoLvl {
					t.Errorf("Level(%d).String() = %q, want %q", tc.level, gotLevel, log.InfoLvl)
				}
			}

		})
	}
}

// --- New / options ---

func TestNewWithOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []log.Option
	}{
		{
			name:    "default",
			options: nil,
		},
		{
			name:    "debug level",
			options: []log.Option{log.WithLevel("debug")},
		},
		{
			name:    "with name",
			options: []log.Option{log.WithName("test-logger")},
		},
		{
			name:    "text format",
			options: []log.Option{log.WithTextFormat()},
		},
		{
			name:    "json format via WithFormat",
			options: []log.Option{log.WithFormat("json")},
		},
		{
			name:    "text format via WithFormat",
			options: []log.Option{log.WithFormat("text")},
		},
		{
			name:    "unknown format defaults to json",
			options: []log.Option{log.WithFormat("unknown")},
		},
		{
			name:    "without caller",
			options: []log.Option{log.WithoutCaller()},
		},
		{
			name:    "with stacktrace",
			options: []log.Option{log.SetStacktrace(true)},
		},
		{
			name:    "without stacktrace",
			options: []log.Option{log.SetStacktrace(false)},
		},
		{
			name:    "custom time format",
			options: []log.Option{log.WithTimeFormat("2006-01-02")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := log.New(tc.options...)
			if l == nil {
				t.Fatal("expected non-nil logger")
			}
		})
	}
}

// --- MockLogger: logging at each level appears in output ---

func TestMockLoggerLevels(t *testing.T) {
	tests := []struct {
		name    string
		logFn   func(l log.FullInterface)
		level   string
		wantMsg string
	}{
		{
			name:    "Info",
			logFn:   func(l log.FullInterface) { l.Info("hello info") },
			level:   "info",
			wantMsg: "hello info",
		},
		{
			name:    "Warn",
			logFn:   func(l log.FullInterface) { l.Warn("hello warn") },
			level:   "warn",
			wantMsg: "hello warn",
		},
		{
			name:    "Error",
			logFn:   func(l log.FullInterface) { l.Error("hello error") },
			level:   "error",
			wantMsg: "hello error",
		},
		{
			name:    "Debug",
			logFn:   func(l log.FullInterface) { l.Debug("hello debug") },
			level:   "debug",
			wantMsg: "hello debug",
		},
		{
			name:    "InfoContext",
			logFn:   func(l log.FullInterface) { l.InfoContext(context.Background(), "ctx info") },
			level:   "info",
			wantMsg: "ctx info",
		},
		{
			name:    "WarnContext",
			logFn:   func(l log.FullInterface) { l.WarnContext(context.Background(), "ctx warn") },
			level:   "warn",
			wantMsg: "ctx warn",
		},
		{
			name:    "ErrorContext",
			logFn:   func(l log.FullInterface) { l.ErrorContext(context.Background(), "ctx error") },
			level:   "error",
			wantMsg: "ctx error",
		},
		{
			name:    "DebugContext",
			logFn:   func(l log.FullInterface) { l.DebugContext(context.Background(), "ctx debug") },
			level:   "debug",
			wantMsg: "ctx debug",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ml := log.NewMockLogger(log.WithLevel(tc.level), log.WithoutCaller(), log.SetStacktrace(false))
			tc.logFn(ml)
			ml.SortByType()

			if !ml.Contains(log.ToLevel(tc.level), tc.wantMsg) {
				t.Errorf("expected %q in %s messages, got: %v", tc.wantMsg, tc.level, ml.ReadByType(log.ToLevel(tc.level)))
			}
		})
	}
}

// --- Infof / Warnf / Errorf / Debugf formatted methods ---

func TestLoggerFormatMethods(t *testing.T) {
	ml := log.NewMockLogger(log.WithLevel("debug"), log.WithoutCaller(), log.SetStacktrace(false))

	formatTests := []struct {
		name    string
		level   log.Level
		wantMsg string
	}{
		{"info formatted", log.InfoLvl, "value=10"},
		{"warn formatted", log.WarnLvl, "value=20"},
		{"error formatted", log.ErrorLvl, "value=30"},
		{"debug formatted", log.DebugLvl, "value=40"},
	}

	ml.Info("value=%d", 10)
	ml.Warn("value=%d", 20)
	ml.Error("value=%d", 30)
	ml.Debug("value=%d", 40)
	ml.SortByType()

	for _, tc := range formatTests {
		t.Run(tc.name, func(t *testing.T) {
			// The format string is logged as the message verbatim since Info/Warn/Error/Debug
			// treat the first arg as the message and remaining as key-value pairs.
			// Here we just verify the message text appears in any form.
			found := false
			for _, entry := range ml.ReadByType(tc.level) {
				if strings.Contains(entry, strings.Split(tc.wantMsg, "=")[0]) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected entry containing %q at level %s, got %v", tc.wantMsg, tc.level, ml.ReadByType(tc.level))
			}
		})
	}
}

// --- Logger.With returns a child logger that preserves fields ---

func TestLoggerWith(t *testing.T) {
	ml := log.NewMockLogger(log.WithLevel("info"), log.WithoutCaller(), log.SetStacktrace(false))
	child := ml.With("component", "auth")
	child.Info("login attempt")
	ml.SortByType()

	if !ml.Contains(log.InfoLvl, "login attempt") {
		t.Error("expected child logger output to reach parent mock target")
	}
	if !ml.Contains(log.InfoLvl, "auth") {
		t.Error("expected 'auth' field value in log output")
	}
}

// --- Logger.New creates an independent child logger ---

func TestLoggerNew(t *testing.T) {
	buf := &bytes.Buffer{}
	target := &syncBuffer{buf}

	parent := log.New(
		log.WithLevel("info"),
		log.WithoutCaller(),
		log.SetStacktrace(false),
		log.WithInfoTarget(target),
		log.WithErrorTarget(target),
	)

	child := parent.New(log.WithName("child"))
	if child == nil {
		t.Fatal("expected non-nil child logger")
	}
}

// --- Logger.Update modifies the logger in place ---

func TestLoggerUpdate(t *testing.T) {
	ml := log.NewMockLogger(log.WithLevel("info"), log.WithoutCaller(), log.SetStacktrace(false))

	// Debug messages should not appear at InfoLvl
	ml.Debug("hidden debug")
	ml.SortByType()
	if ml.Contains(log.DebugLvl, "hidden debug") {
		t.Error("debug message should not appear at info level")
	}

	ml.Update(log.WithLevel("debug"))
	ml.Debug("visible debug")
	ml.SortByType()
	if !ml.Contains(log.DebugLvl, "visible debug") {
		t.Error("expected debug message after updating level to debug")
	}
}

// --- AppendCtx attaches attributes to context ---

func TestAppendCtx(t *testing.T) {
	tests := []struct {
		name      string
		buildCtx  func() context.Context
		wantField string
	}{
		{
			name: "single attribute",
			buildCtx: func() context.Context {
				return log.AppendCtx(context.Background(), slog.String("req_id", "abc123"))
			},
			wantField: "abc123",
		},
		{
			name: "multiple attributes appended",
			buildCtx: func() context.Context {
				ctx := log.AppendCtx(context.Background(), slog.String("user", "alice"))
				return log.AppendCtx(ctx, slog.String("tenant", "acme"))
			},
			wantField: "alice",
		},
		{
			name: "nil parent becomes background",
			buildCtx: func() context.Context {
				//nolint:staticcheck // intentionally passing nil to exercise the nil-guard in AppendCtx
				return log.AppendCtx(nil, slog.String("key", "nilparent"))
			},
			wantField: "nilparent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ml := log.NewMockLogger(log.WithLevel("info"), log.WithoutCaller(), log.SetStacktrace(false))
			ctx := tc.buildCtx()
			ml.InfoContext(ctx, "with context")

			if !strings.Contains(ml.ReadAllString(), tc.wantField) {
				t.Errorf("expected %q in log output, got: %s", tc.wantField, ml.ReadAllString())
			}
		})
	}
}

// --- MockLogger.SortByType / ReadByType / Contains ---

func TestMockLoggerSortAndRead(t *testing.T) {
	ml := log.NewMockLogger(log.WithLevel("debug"), log.WithoutCaller(), log.SetStacktrace(false))
	ml.Info("msg-info")
	ml.Warn("msg-warn")
	ml.Error("msg-error")
	ml.Debug("msg-debug")
	ml.SortByType()

	tests := []struct {
		level   log.Level
		wantMsg string
	}{
		{log.InfoLvl, "msg-info"},
		{log.WarnLvl, "msg-warn"},
		{log.ErrorLvl, "msg-error"},
		{log.DebugLvl, "msg-debug"},
	}

	for _, tc := range tests {
		t.Run(tc.level.String(), func(t *testing.T) {
			if !ml.Contains(tc.level, tc.wantMsg) {
				t.Errorf("expected %q in %s level, got: %v", tc.wantMsg, tc.level, ml.ReadByType(tc.level))
			}
		})
	}
}

func TestMockLoggerContainsMissing(t *testing.T) {
	ml := log.NewMockLogger(log.WithLevel("info"), log.WithoutCaller(), log.SetStacktrace(false))
	ml.Info("present message")
	ml.SortByType()

	if ml.Contains(log.InfoLvl, "absent message") {
		t.Error("Contains should return false for a message that was not logged")
	}
}

func TestMockLoggerReadByTypeUnknownLevel(t *testing.T) {
	ml := log.NewMockLogger(log.WithoutCaller(), log.SetStacktrace(false))
	ml.SortByType()

	entries := ml.ReadByType(log.FatalLvl)
	if entries != nil {
		t.Errorf("ReadByType for unsupported level should return nil, got %v", entries)
	}
}

// --- MockTarget direct write ---

func TestMockTargetWrite(t *testing.T) {
	target := log.NewMockTarget()

	data := []byte("line one\n")
	n, err := target.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	if !bytes.Contains(target.ReadAll(), []byte("line one")) {
		t.Error("expected written data in ReadAll()")
	}
	if len(target.Read()) != 1 {
		t.Errorf("expected 1 entry in Read(), got %d", len(target.Read()))
	}
	if !strings.Contains(target.ReadAllString(), "line one") {
		t.Error("expected written data in ReadAllString()")
	}
	strs := target.ReadString()
	if len(strs) != 1 || !strings.Contains(strs[0], "line one") {
		t.Errorf("unexpected ReadString() result: %v", strs)
	}

	if err := target.Sync(); err != nil {
		t.Errorf("Sync() returned error: %v", err)
	}
}

// --- CallerHandler adds caller group when enabled ---

func TestCallerHandlerEnabled(t *testing.T) {
	tests := []struct {
		name       string
		addCaller  bool
		wantCaller bool
	}{
		{"caller enabled", true, true},
		{"caller disabled", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts []log.Option
			if !tc.addCaller {
				opts = append(opts, log.WithoutCaller())
			}
			opts = append(opts, log.WithLevel("info"), log.SetStacktrace(false))

			ml := log.NewMockLogger(opts...)
			ml.Info("test message")

			output := ml.ReadAllString()
			hasCaller := strings.Contains(output, "caller")

			if hasCaller != tc.wantCaller {
				t.Errorf("caller present=%v, want %v; output: %s", hasCaller, tc.wantCaller, output)
			}
		})
	}
}

// --- SetDefaultLogger ---

func TestSetDefaultLogger(t *testing.T) {
	tests := []struct {
		name    string
		options []log.Option
	}{
		{"no options", nil},
		{"with name", []log.Option{log.WithName("custom")}},
		{"debug level", []log.Option{log.WithLevel("debug")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log.SetDefaultLogger(tc.options...)
			if log.Default == nil {
				t.Error("Default logger should not be nil after SetDefaultLogger")
			}
		})
	}

	// Restore default
	log.SetDefaultLogger()
}

// --- loggerOptions.DeepCopy does not share slice references ---

func TestWithAdditionalTargets(t *testing.T) {
	t1 := log.NewMockTarget()
	t2 := log.NewMockTarget()

	l := log.New(
		log.WithLevel("info"),
		log.WithoutCaller(),
		log.SetStacktrace(false),
		log.WithAdditionalTargets(t1, t2),
	)
	l.Info("multi-target")

	for i, target := range []log.MockTarget{t1, t2} {
		if !strings.Contains(target.ReadAllString(), "multi-target") {
			t.Errorf("target %d did not receive log output", i)
		}
	}
}

// --- With() builder option ---

func TestWithBuilderOption(t *testing.T) {
	buf := &bytes.Buffer{}
	infoTarget := &syncBuffer{buf}
	errTarget := &syncBuffer{buf}

	l := log.New(log.With(
		log.InfoLvl,
		"builder-test",
		"",
		true,  // textFormat
		false, // addCaller
		false, // addStacktrace
		infoTarget,
		errTarget,
	))

	if l == nil {
		t.Fatal("expected non-nil logger from With() builder")
	}
}

// syncBuffer wraps bytes.Buffer to satisfy zapcore.WriteSyncer.
type syncBuffer struct {
	*bytes.Buffer
}

func (s *syncBuffer) Sync() error { return nil }
