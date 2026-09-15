// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	prom "github.com/SAP/rootstock/internal/collectors/prometheus"
	"github.com/SAP/rootstock/internal/config"
	"github.com/SAP/rootstock/internal/redfish"
)

const (
	metricsEndpoint = "/metrics"
	healthEndpoint  = "/healthz"
	readyEndpoint   = "/ready"
	probeEndpoint   = "/probe"
)

// probeHandler creates an HTTP handler for the /probe endpoint that dynamically
// probes PDUs based on the 'target' query parameter using Prometheus collector
func probeHandler(cfg *config.Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		target := req.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "target parameter is required", http.StatusBadRequest)
			logger.Warn("probe request missing target parameter")
			return
		}

		// Read PDU ID from "labels" query parameter
		pduID := req.URL.Query().Get("labels")
		if pduID == "" {
			http.Error(w, "labels parameter is required", http.StatusBadRequest)
			logger.Warn("probe request missing labels parameter")
			return
		}

		logger.Info("probing target", "target", target, "pduID", pduID)

		// Parse target URL
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid target URL: %v", err), http.StatusBadRequest)
			logger.Error("invalid target URL", "target", target, "error", err)
			return
		}

		redfishClient := redfish.NewClient(targetURL, logger, cfg.RedfishPassword, cfg.RedfishUsername, time.Duration(cfg.ScrapeTimeoutSeconds)*time.Second)

		logger.Info("Using PDU ID from labels parameter", "pduID", pduID)
		// Create a temporary collector for this target
		collector := prom.NewCollector(redfishClient, pduID, logger)

		// Create a temporary registry for this specific scrape
		registry := prometheus.NewRegistry()
		registry.MustRegister(collector)

		// Use promhttp to handle the metrics collection and formatting
		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			ErrorLog:      slog.NewLogLogger(logger.Handler(), slog.LevelError),
			ErrorHandling: promhttp.ContinueOnError,
		})

		h.ServeHTTP(w, req)
	}
}

func main() {
	cfg, err := config.ReadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Env == "LOCAL" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	} else {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	}

	slog.Info("rootstock starting up", "port", cfg.Port, "env", cfg.Env)

	r := chi.NewRouter()
	r.Use(middleware.Heartbeat(healthEndpoint))

	r.Handle(metricsEndpoint, promhttp.Handler())

	r.Get(readyEndpoint, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get(probeEndpoint, probeHandler(cfg, slog.Default()))

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
