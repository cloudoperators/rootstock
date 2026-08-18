package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/SAP/rootstock/internal/config"
	"github.com/SAP/rootstock/internal/globals"
	"github.com/SAP/rootstock/internal/log"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.ReadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := log.NewLogger(cfg.ServiceName, cfg.Env)
	logger.Info("rootstock starting up")

	r := chi.NewRouter()
	r.Use(middleware.Heartbeat(globals.HealthEndpoint))

	r.Handle(globals.MetricsEndpoint, promhttp.Handler())

	r.Get(globals.ReadyEndpoint, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

}
