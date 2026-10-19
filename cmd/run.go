// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cloudoperators/rootstock/internal/communication/rest"
	"github.com/cloudoperators/rootstock/internal/shutdown"
	"github.com/cloudoperators/rootstock/pkg/log"
	v "github.com/cloudoperators/rootstock/pkg/version"
)

const (
	redfishCredFile = "redfish_pdus.yaml"
)

var (
	scrapeTimeoutDefault = 30 * time.Second
	scrapeTimeoutValue   = scrapeTimeoutDefault
	pduCredentials       map[string]rest.PDUCredentials

	port = NewParameter(
		"port", "PORT", "port",
		"port of the rest server",
		9220,
	)
	scrapeTimeout = NewParameter(
		"scrape.timeout", "SCRAPE_TIMEOUT", "scrape-timeout",
		"timeout for scraping upstream APIs (e.g. 15s, 1m)",
		scrapeTimeoutDefault.String(),
	)
)

func run() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: fmt.Sprintf(`run the %s server`, cliName),
		// Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, _ []string) {
			logger.Info("rootstock binary", "version", v.ReadAll())

			sTimeout, err := time.ParseDuration(scrapeTimeout.Value())
			if err != nil {
				logger.Error("failed to parse timeout", log.KeyError, err)
				shutdown.StopNow()

				return
			}

			scrapeTimeoutValue = sTimeout

			logger.Info(
				"rootstock configurations",
				"port", port.Value(),
				"scrapeTimeout", sTimeout.String(),
			)

			shutdown.AddStopFunction(func() { server.Stop(shutdown.GetContext()) })

			registerCredentialsWatchAndRunServer()

			ctx := shutdown.GetContext()
			<-ctx.Done()
			logger.Info("context done", "err", ctx.Err())

			if errors.Is(ctx.Err(), context.Canceled) {
				shutdown.StopNowWith(0)
			} else {
				shutdown.StopNow()
			}
		},
	}

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	port.Register(c.Flags())
	scrapeTimeout.Register(c.Flags())

	return c
}

var (
	serverLock sync.Mutex
	server     rest.Server
)

func registerCredentialsWatchAndRunServer() {
	file := filepath.Join(configFolder.Value(), redfishCredFile)
	err := InitFileConfig(
		file,
		func(content []byte) error {
			var newCreds map[string]rest.PDUCredentials
			if err := yaml.Unmarshal(content, &newCreds); err != nil {
				return fmt.Errorf("failed to parse credentials file: %w", err)
			}

			pduCredentials = newCreds

			dynamicRunServer(logger)

			return nil
		},
	)
	fatalOnError(err, fmt.Sprintf("failed to register file watch on %q", file))
}

func dynamicRunServer(l log.FullInterface) {
	serverLock.Lock()
	defer serverLock.Unlock()

	if server != nil {
		server.Stop(shutdown.GetContext())
	}

	server = rest.New(
		port.Value(),
		l,
		pduCredentials,
		scrapeTimeoutValue,
	)
	l.Info("starting HTTP server", "port", port.Value())
	server.ListenAndServe()
}
