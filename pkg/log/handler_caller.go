// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0
//
//nolint:depguard,godoclint // log pkg dependencies allowed in the log package
package log

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
)

const (
	callerGroup = "caller"
	funcKey     = "func"
	fileKey     = "file"

	callstackOffset = 7
)

type CallerHandler struct {
	slog.Handler

	addCaller bool
	offset    int
}

// Handle adds contextual attributes to the record before calling the underlying
// handler.
//
//nolint:gocritic // implements slog.Handler signature
func (h *CallerHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.addCaller {
		fileV, funcV := prepareCallerValues(callstackOffset + h.offset)

		r.AddAttrs(slog.Group(
			callerGroup,
			slog.String(fileKey, fileV),
			slog.String(funcKey, funcV),
		))
	}

	return h.Handler.Handle(ctx, r)
}

func prepareCallerValues(offset int) (fileValue, funcValue string) {
	file, line, name := getCaller(0 + offset)
	fileValue = fmt.Sprintf("%s/%s:%v",
		filepath.Base(filepath.Dir(file)),
		filepath.Base(file),
		line,
	)
	funcValue = filepath.Base(name)

	return fileValue, funcValue
}

func getCaller(lvl int) (file string, line int, name string) {
	var (
		pc uintptr
		ok bool
	)

	pc, file, line, ok = runtime.Caller(lvl)
	details := runtime.FuncForPC(pc)

	if ok && details != nil {
		return file, line, details.Name()
	}

	return file, line, "unknown"
}
