// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/SAP/rootstock/internal/shutdown"
	"github.com/SAP/rootstock/pkg/log"
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
	ListenAndServe()
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
		&http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           r,
			ReadHeaderTimeout: serverTimeout,
			WriteTimeout:      serverTimeout,
			IdleTimeout:       serverTimeout,
		},
		logger,
	}
}

type server struct {
	server *http.Server
	log    log.MinInterface
}

func (s *server) ListenAndServe() {
	s.log.Info("server listening", "addr", s.server.Addr)
	go func() {
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
	}()
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

	//TODO: wait until the server shutdown is complete. Only then return
	time.Sleep(200 * time.Millisecond)
}

// func ListenAndServe(port int, logger log.MinInterface) {
// 	if server != nil {
// 		logger.Error("function called with http server still active - ignoring", "port", port)
// 		return
// 	}

// 	r := chi.NewRouter()
// 	r.Use(middleware.Heartbeat(healthEndpoint))

// 	r.Handle(metricsEndpoint, promhttp.Handler())

// 	r.Get(readyEndpoint, func(w http.ResponseWriter, _ *http.Request) {
// 		w.WriteHeader(http.StatusOK)
// 	})

// 	r.Get(probeEndpoint, func(w http.ResponseWriter, _ *http.Request) {
// 		// PDU scraping can be implemented here
// 		w.WriteHeader(http.StatusNotImplemented)
// 	})

// 	addr := fmt.Sprintf(":%d", port)
// 	logger.Info("server listening", "addr", addr)

// 	server = &http.Server{
// 		Addr:              addr,
// 		Handler:           r,
// 		ReadHeaderTimeout: serverTimeout,
// 		WriteTimeout:      serverTimeout,
// 		IdleTimeout:       serverTimeout,
// 	}
// 	go func() {
// 		err := server.ListenAndServe()
// 		if err == nil {
// 			return
// 		}

// 		if errors.Is(err, http.ErrServerClosed) {
// 			logger.Info("server closed")
// 			return
// 		}

// 		logger.Error("server failed", log.KeyError, err)
// 		shutdown.StopNow()
// 	}()
// }

// func StopServer() {
// 	if server == nil {
// 		return
// 	}

// 	err := server.Close()
// 	server = nil
// }
