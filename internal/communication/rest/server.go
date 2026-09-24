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

	"github.com/cloudoperators/rootstock/internal/shutdown"
	"github.com/cloudoperators/rootstock/pkg/log"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	r.Get(probeEndpoint, func(w http.ResponseWriter, _ *http.Request) {
		// PDU scraping can be implemented here
		w.WriteHeader(http.StatusNotImplemented)
	})

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
