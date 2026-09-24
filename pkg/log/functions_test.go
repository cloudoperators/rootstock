// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0
//

package log_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudoperators/rootstock/pkg/log"
)

// captureDefault installs a fresh Default logger that writes to a MockTarget,
// and restores the original Default when the test ends.
func captureDefault(t *testing.T, level log.Level) log.MockTarget {
	t.Helper()

	target := log.NewMockTarget()
	original := log.Default()

	t.Cleanup(func() { log.SetDefault(original) })
	log.SetDefaultLogger(
		log.WithLevel(level.String()),
		log.WithoutCaller(),
		log.SetStacktrace(false),
		log.WithInfoTarget(target),
		log.WithErrorTarget(target),
	)

	return target
}

// TestPackageLevelFunctions checks that each top-level log function routes to Default.
func TestPackageLevelFunctions(t *testing.T) {
	tests := []struct {
		name    string
		level   log.Level
		logFn   func()
		wantMsg string
	}{
		{
			name:    "Info",
			level:   log.InfoLvl,
			logFn:   func() { log.Info("pkg-info-msg", "k", "v") },
			wantMsg: "pkg-info-msg",
		},
		{
			name:    "Warn",
			level:   log.InfoLvl,
			logFn:   func() { log.Warn("pkg-warn-msg", "k", "v") },
			wantMsg: "pkg-warn-msg",
		},
		{
			name:    "Error",
			level:   log.InfoLvl,
			logFn:   func() { log.Error("pkg-error-msg", "k", "v") },
			wantMsg: "pkg-error-msg",
		},
		{
			name:    "Debug",
			level:   log.DebugLvl,
			logFn:   func() { log.Debug("pkg-debug-msg", "k", "v") },
			wantMsg: "pkg-debug-msg",
		},
		{
			name:    "DebugContext",
			level:   log.DebugLvl,
			logFn:   func() { log.DebugContext(context.Background(), "pkg-debug-ctx-msg") },
			wantMsg: "pkg-debug-ctx-msg",
		},
		{
			name:    "InfoContext",
			level:   log.InfoLvl,
			logFn:   func() { log.InfoContext(context.Background(), "pkg-debug-ctx-msg") },
			wantMsg: "pkg-debug-ctx-msg",
		},
		{
			name:    "WarnContext",
			level:   log.WarnLvl,
			logFn:   func() { log.WarnContext(context.Background(), "pkg-error-ctx-msg") },
			wantMsg: "pkg-error-ctx-msg",
		},
		{
			name:    "ErrorContext",
			level:   log.InfoLvl,
			logFn:   func() { log.ErrorContext(context.Background(), "pkg-error-ctx-msg") },
			wantMsg: "pkg-error-ctx-msg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := captureDefault(t, tc.level)
			tc.logFn()

			if !strings.Contains(target.ReadAllString(), tc.wantMsg) {
				t.Errorf("expected %q in output, got: %s", tc.wantMsg, target.ReadAllString())
			}
		})
	}
}

// TestPackageLevelFormatFunctions checks that the f-suffix functions apply [fmt.Sprintf].
func TestPackageLevelFormatFunctions(t *testing.T) {
	tests := []struct {
		name    string
		level   log.Level
		logFn   func()
		wantMsg string
	}{
		{
			name:    "Infof",
			level:   log.InfoLvl,
			logFn:   func() { log.Infof("info-%s-%d", "fmt", 1) },
			wantMsg: "info-fmt-1",
		},
		{
			name:    "Warnf",
			level:   log.InfoLvl,
			logFn:   func() { log.Warnf("warn-%s-%d", "fmt", 2) },
			wantMsg: "warn-fmt-2",
		},
		{
			name:    "Errorf",
			level:   log.InfoLvl,
			logFn:   func() { log.Errorf("error-%s-%d", "fmt", 3) },
			wantMsg: "error-fmt-3",
		},
		{
			name:    "Debugf",
			level:   log.DebugLvl,
			logFn:   func() { log.Debugf("debug-%s-%d", "fmt", 4) },
			wantMsg: "debug-fmt-4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := captureDefault(t, tc.level)
			tc.logFn()

			if !strings.Contains(target.ReadAllString(), tc.wantMsg) {
				t.Errorf("expected %q in output, got: %s", tc.wantMsg, target.ReadAllString())
			}
		})
	}
}

// TestPackageLevelDebugSuppressedAtInfoLevel verifies that Debug does not
// produce output when Default is at info level.
func TestPackageLevelDebugSuppressedAtInfoLevel(t *testing.T) {
	target := captureDefault(t, log.InfoLvl)

	log.Debug("should-not-appear")

	if strings.Contains(target.ReadAllString(), "should-not-appear") {
		t.Errorf("debug message should be suppressed at info level, got: %s", target.ReadAllString())
	}
}

// TestAddCallerAttrStructure verifies AddCaller returns a [slog.Group] attr with file and func keys.
func TestAddCallerAttrStructure(t *testing.T) {
	attr := log.AddCaller()

	if attr.Key != "caller" {
		t.Errorf("expected attr.Key = %q, got %q", "caller", attr.Key)
	}

	group := attr.Value.Group()
	if len(group) == 0 {
		t.Fatal("expected non-empty group in AddCaller attr")
	}

	keys := make(map[string]string, len(group))
	for _, a := range group {
		keys[a.Key] = a.Value.String()
	}

	for _, expected := range []string{"file", "func"} {
		v, ok := keys[expected]
		if !ok {
			t.Errorf("expected %q key in caller group", expected)
		} else if v == "" {
			t.Errorf("expected non-empty value for %q key", expected)
		}
	}
}

// TestAddCallerReportsCallingLocation verifies that AddCaller captures the file and function
// of the direct caller.
func TestAddCallerReportsCallingLocation(t *testing.T) {
	attr := log.AddCaller()
	group := attr.Value.Group()

	keys := make(map[string]string, len(group))
	for _, a := range group {
		keys[a.Key] = a.Value.String()
	}

	if !strings.Contains(keys["file"], "functions_test.go") {
		t.Errorf("expected file to contain 'functions_test.go', got %q", keys["file"])
	}

	if !strings.Contains(keys["func"], "TestAddCallerReportsCallingLocation") {
		t.Errorf("expected func to contain 'TestAddCallerReportsCallingLocation', got %q", keys["func"])
	}
}
