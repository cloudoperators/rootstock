// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/SAP/rootstock/internal/config"
	"github.com/SAP/rootstock/pkg/log"
	v "github.com/SAP/rootstock/pkg/version"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsEndpoint = "/metrics"
	healthEndpoint  = "/healthz"
	readyEndpoint   = "/ready"
	probeEndpoint   = "/probe"
)

// Version information set by link flags during build. We fall back to these sane
// default values when we build outside the Makefile context (e.g. go run, go build, or go test).
var (
	version      = "99.99.99"             // value from VERSION.yaml file
	buildDate    = "1970-01-01T00:00:00Z" // output from `date -u +'%Y-%m-%dT%H:%M:%SZ'`
	gitCommit    = ""                     // output from `git rev-parse HEAD`
	gitTag       = ""                     // output from `git describe --exact-match --tags HEAD` (if clean tree state)
	gitTreeState = ""                     // determined from `git status --porcelain`. either 'clean' or 'dirty'
)

func main() {
	cfg, err := config.ReadConfig()
	if err != nil {
		log.Error("failed to load config", log.KeyError, err)
		os.Exit(1)
	}

	if cfg.ServiceEnv == "LOCAL" {
		log.SetDefaultLogger(log.WithLevel("debug"), log.WithTextFormat())
	} else {
		log.SetDefaultLogger(log.WithLevel("info"))
	}

	if err := v.Init(version, buildDate, gitCommit, gitTag, gitTreeState); err != nil {
		log.Error("failed to initialize binary version", "error", err)
	}

	log.Info(
		"rootstock starting up",
		"version", v.ReadAll(),
		"port", cfg.ServicePort, "env", cfg.ServiceEnv,
	)

	r := chi.NewRouter()
	r.Use(middleware.Heartbeat(healthEndpoint))

	r.Handle(metricsEndpoint, promhttp.Handler())

	r.Get(readyEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get(probeEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		// PDU scraping can be implemented here
		w.WriteHeader(http.StatusNotImplemented)
	})

	addr := fmt.Sprintf(":%d", cfg.ServicePort)
	log.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Error("server failed", log.KeyError, err)
		os.Exit(1)
	}
}
