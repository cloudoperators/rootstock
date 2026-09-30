// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	prom "github.com/cloudoperators/rootstock/internal/collectors/prometheus"
	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/internal/shutdown"
	"github.com/cloudoperators/rootstock/pkg/log"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsEndpoint = "/metrics"
	healthEndpoint  = "/healthz"
	readyEndpoint   = "/ready"
	probeEndpoint   = "/probe"

	serverTimeout = 5 * time.Second
)

type Server interface {
	// ListenAndServe starts the rest server
	ListenAndServe()
	// Stop halts the rest server
	Stop(context.Context)
}

func New(port int, logger log.MinInterface) Server {
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat(healthEndpoint))

	r.Handle(metricsEndpoint, promhttp.Handler())

	r.Get(readyEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get(probeEndpoint, probeHandler())

	return &server{
		server: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           r,
			ReadHeaderTimeout: serverTimeout,
			WriteTimeout:      serverTimeout,
			IdleTimeout:       serverTimeout,
		},
		log: logger,
	}
}

type server struct {
	server *http.Server
	log    log.MinInterface
	wg     sync.WaitGroup
}

func (s *server) ListenAndServe() {
	s.log.Info("server listening", "addr", s.server.Addr)

	s.wg.Go(func() {
		err := s.server.ListenAndServe()
		if err == nil {
			return
		}

		if errors.Is(err, http.ErrServerClosed) {
			s.log.Info("server closed")
			return
		}

		s.log.Error("server failed", log.KeyError, err)
		shutdown.StopNow()
	})
}

func (s *server) Stop(ctx context.Context) {
	if s.server == nil {
		return
	}

	if err := s.server.Shutdown(ctx); err != nil {
		s.log.Error("failed to shutdown server", log.KeyError, err)
		shutdown.StopNow()
	}

	s.server = nil

	s.wg.Wait()
}

// probeHandler creates an HTTP handler for the /probe endpoint that dynamically
// probes PDUs based on the 'target' query parameter using Prometheus collector
func probeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		target := req.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "target parameter is required", http.StatusBadRequest)
			log.Warn("probe request missing target parameter")

			return
		}

		// Read PDU ID from "labels" query parameter
		pduID := req.URL.Query().Get("labels")
		if pduID == "" {
			http.Error(w, "labels parameter is required", http.StatusBadRequest)
			log.Warn("probe request missing labels parameter")

			return
		}

		log.Info("probing target", "target", target, "pduID", pduID)

		// Parse target URL
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid target URL: %v", err), http.StatusBadRequest)
			log.Error("invalid target URL", "target", target, log.KeyError, err)

			return
		}

		// Use default logger for this request
		redfishClient := redfish.NewClient(targetURL, log.Default(), "redfishPassword", "redfishUsername", 30*time.Second)

		log.Info("Using PDU ID from labels parameter", "pduID", pduID)
		// Create a temporary collector for this target
		collector := prom.NewCollector(redfishClient, pduID)

		// Create a temporary registry for this specific scrape
		registry := prometheus.NewRegistry()

		err = registry.Register(collector)
		if err != nil {
			http.Error(w, fmt.Sprintf("can not register prometheus collector: %v", err), http.StatusInternalServerError)
			log.Error("can not register prometheus collector", log.KeyError, err)

			return
		}

		// Use promhttp to handle the metrics collection and formatting
		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		})

		h.ServeHTTP(w, req)
	}
}
