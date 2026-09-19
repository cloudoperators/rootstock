// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/SAP/rootstock/cmd"
	"github.com/SAP/rootstock/internal/shutdown"
	"github.com/SAP/rootstock/pkg/log"
	v "github.com/SAP/rootstock/pkg/version"
)

// Version information set by link flags during build. We fall back to these sane
// default values when we build outside the Makefile context (e.g. go run, go build, or go test).
var (
	version      = "99.99.99"             // value from VERSION file
	buildDate    = "1970-01-01T00:00:00Z" // output from `date -u +'%Y-%m-%dT%H:%M:%SZ'`
	gitCommit    = ""                     // output from `git rev-parse HEAD`
	gitTag       = ""                     // output from `git describe --exact-match --tags HEAD` (if clean tree state)
	gitTreeState = ""                     // determined from `git status --porcelain`. either 'clean' or 'dirty'
)

func main() {
	if err := v.Init(version, buildDate, gitCommit, gitTag, gitTreeState); err != nil {
		log.Error("failed to initialize binary version", log.KeyError, err)
		shutdown.StopNow()
	}

	rootCmd := cmd.New()
	if err := rootCmd.Execute(); err != nil {
		log.Error("rootstock cli failed", log.KeyError, err)
		shutdown.StopNow()
	}

	shutdown.StopNowWith(0)
}
