// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	prom "github.com/cloudoperators/rootstock/internal/collectors/prometheus"
	"github.com/cloudoperators/rootstock/internal/shutdown"
	"github.com/cloudoperators/rootstock/pkg/log"
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

func New(
	port int, logger log.MinInterface, redfishUsername, redfishPassword string, scrapeTimeout time.Duration, cache *cache.Cache,
) Server {
	s := &server{
		server: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			ReadHeaderTimeout: serverTimeout,
			WriteTimeout:      serverTimeout,
			IdleTimeout:       serverTimeout,
		},
		log:             logger,
		redfishUsername: redfishUsername,
		redfishPassword: redfishPassword,
		scrapeTimeout:   scrapeTimeout,
		cache:           cache,
	}

	r := chi.NewRouter()
	r.Use(middleware.Heartbeat(healthEndpoint))

	r.Handle(metricsEndpoint, promhttp.Handler())

	r.Get(readyEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get(probeEndpoint, s.probeHandler())

	s.server.Handler = r

	return s
}

type server struct {
	server          *http.Server
	log             log.MinInterface
	wg              sync.WaitGroup
	redfishUsername string
	redfishPassword string
	scrapeTimeout   time.Duration
	cache           *cache.Cache
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
func (s *server) probeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		target := req.URL.Query().Get("target")
		if target == "" {
			http.Error(w, "target parameter is required", http.StatusBadRequest)
			s.log.Warn("probe request missing target parameter")

			return
		}

		// Read PDU ID from "labels" query parameter
		pduID := req.URL.Query().Get("labels")
		if pduID == "" {
			http.Error(w, "labels parameter is required", http.StatusBadRequest)
			s.log.Warn("probe request missing labels parameter")

			return
		}

		s.log.Info("probing target", "target", target, "pduID", pduID)

		// // Parse target URL
		// targetURL, err := url.Parse(target)
		// if err != nil {
		// 	http.Error(w, fmt.Sprintf("invalid target URL: %v", err), http.StatusBadRequest)
		// 	s.log.Error("invalid target URL", "target", target, log.KeyError, err)

		// 	return
		// }

		// // Create redfish client with configured credentials
		// redfishClient := redfish.NewClient(
		// 	targetURL,
		// 	s.log,
		// 	s.redfishUsername,
		// 	s.redfishPassword,
		// 	s.scrapeTimeout,
		// )

		s.log.Info("Using PDU ID from labels parameter", "pduID", pduID)
		// Create a temporary collector for this target
		collector := prom.NewCollector(pduID, s.cache)

		// Create a temporary registry for this specific scrape
		registry := prometheus.NewRegistry()

		err := registry.Register(collector)
		if err != nil {
			http.Error(
				w,
				fmt.Sprintf("can not register prometheus collector: %v", err),
				http.StatusInternalServerError,
			)
			s.log.Error("can not register prometheus collector", log.KeyError, err)

			return
		}

		// Use promhttp to handle the metrics collection and formatting
		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		})

		h.ServeHTTP(w, req)
	}
}
