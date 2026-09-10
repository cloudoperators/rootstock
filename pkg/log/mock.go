// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"bytes"
	"strings"
	"sync"

	"go.uber.org/zap/zapcore"
)

type MockFullInterface interface {
	FullInterface
	MockReaders
	// SortByType sorts the log entries by their type (DEBUG, ERROR, INFO, WARN).
	SortByType()
	// ReadByType returns all messages of the specified level.
	ReadByType(lvl Level) []string
	// Contains checks if the log contains a message of the specified level.
	Contains(lvl Level, msg string) bool
}

func NewMockLogger(options ...Option) MockFullInterface {
	logTarget := NewMockTarget()
	options = append(options, WithInfoTarget(logTarget), WithErrorTarget(logTarget))

	logger := New(options...)

	return &mockLogger{
		FullInterface: logger,
		MockReaders:   logTarget,
	}
}

type mockLogger struct {
	FullInterface
	MockReaders

	debugMsg []string
	errorMsg []string
	infoMsg  []string
	warnMsg  []string
}

func (m *mockLogger) SortByType() {
	m.debugMsg = nil
	m.errorMsg = nil
	m.infoMsg = nil
	m.warnMsg = nil

	for _, msg := range m.Read() {
		switch {
		case bytes.Contains(msg, []byte(zapcore.DebugLevel.CapitalString())):
			m.debugMsg = append(m.debugMsg, string(msg))
		case bytes.Contains(msg, []byte(zapcore.ErrorLevel.CapitalString())):
			m.errorMsg = append(m.errorMsg, string(msg))
		case bytes.Contains(msg, []byte(zapcore.InfoLevel.CapitalString())):
			m.infoMsg = append(m.infoMsg, string(msg))
		case bytes.Contains(msg, []byte(zapcore.WarnLevel.CapitalString())):
			m.warnMsg = append(m.warnMsg, string(msg))
		}
	}
}

func (m *mockLogger) ReadByType(lvl Level) []string {
	//nolint:exhaustive // not listed cases are not used by us
	switch lvl {
	case DebugLvl:
		return m.debugMsg
	case ErrorLvl:
		return m.errorMsg
	case InfoLvl:
		return m.infoMsg
	case WarnLvl:
		return m.warnMsg
	default:
		return nil
	}
}

func (m *mockLogger) Contains(lvl Level, msg string) bool {
	for _, entry := range m.ReadByType(lvl) {
		if strings.Contains(entry, msg) {
			return true
		}
	}

	return false
}

type MockReaders interface {
	// ReadAll returns all the data written to the logger.
	ReadAll() []byte
	// Read returns all the entries written to the logger as a slice of byte slices.
	Read() [][]byte
	// ReadAllString returns all the data written to the logger as a string.
	ReadAllString() string
	// ReadEntriesString returns all the entries written to the logger as a slice of strings.
	ReadString() []string
}

type MockTarget interface {
	MockReaders
	// Write writes data to the logger.
	Write(p []byte) (n int, err error)
	// Sync does nothing in this implementation.
	Sync() error
}

func NewMockTarget() MockTarget {
	return &testLogger{
		data: make([][]byte, 0),
		lock: sync.Mutex{},
	}
}

type testLogger struct {
	data [][]byte
	lock sync.Mutex
}

func (l *testLogger) ReadAllString() string {
	return string(l.ReadAll())
}

func (l *testLogger) ReadString() []string {
	all := l.Read()

	res := make([]string, 0, len(all))
	for _, entry := range all {
		res = append(res, string(entry))
	}

	return res
}

func (l *testLogger) ReadAll() []byte {
	return bytes.Join(l.Read(), []byte("\n"))
}

func (l *testLogger) Read() [][]byte {
	l.lock.Lock()
	defer l.lock.Unlock()

	res := make([][]byte, 0, len(l.data))
	for _, entry := range l.data {
		e := make([]byte, len(entry))
		copy(e, entry)
		res = append(res, e)
	}

	return res
}

func (l *testLogger) Write(p []byte) (n int, err error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	entry := make([]byte, len(p))
	copy(entry, p)
	l.data = append(l.data, entry)

	return len(p), nil
}

func (*testLogger) Sync() error {
	return nil
}
