// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SAP/rootstock/internal/communication/rest"
	"github.com/SAP/rootstock/internal/shutdown"
	"github.com/SAP/rootstock/pkg/log"
	"github.com/spf13/cobra"
)

var (
	scrapeTimeoutDefault = 30 * time.Second

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

	redfishUsername = NewParameter(
		"redfish.username", "REDFISH_USERNAME", "redfish-username",
		"username for the redfish authorization",
		"",
	)
	redfishPassword = NewParameter(
		"redfish.password", "REDFISH_PASSWORD", "redfish-password",
		"password for the redfish authorization",
		"",
	).WithBaseKey("secrets")
)

func run() *cobra.Command {

	c := &cobra.Command{
		Use:   "run",
		Short: fmt.Sprintf(`run the %s server`, cliName),
		// Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, _ []string) {
			logger.Info("starting " + cliName + " api")

			sTimeout, err := time.ParseDuration(scrapeTimeout.Value())
			if err != nil {
				logger.Error("failed to parse timeout", log.KeyError, err)
				shutdown.StopNow()
				return
			}

			logger.Info(
				"rootstock starting up",
				"port", port.Value(),
				"scrapeTimeout", sTimeout.String(),
			)

			go func() {
				log.Info("starting HTTP server", "port", port.Value())
				runServer(port.Value(), childLogger(logger, "run"))
			}()

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
	redfishUsername.Register(c.Flags())
	redfishPassword.Register(c.Flags())

	return c
}

var (
	// for test monkey patching
	runServer   = rest.ListenAndServe
	childLogger = func(l log.FullInterface, name string) log.MinInterface { return l.New(log.WithName(name)) }
)
