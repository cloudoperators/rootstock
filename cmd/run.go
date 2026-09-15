// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SAP/rootstock/internal/communication/rest"
	"github.com/SAP/rootstock/internal/shutdown"
	"github.com/SAP/rootstock/pkg/log"
	"github.com/spf13/cobra"
)

var (
	scrapeTimeoutDefault = 30 * time.Second
	redfishPassword      *string

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
	redfishCredFile = NewParameter(
		"redfish.credentialsFile", "REDFISH_CREDENTIALS_FILE", "redfish-credentials-file",
		"file that holds the password for the redfish authorization",
		"redfish_password",
	)
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

			dynamicRunServer(logger)

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
	redfishCredFile.Register(c.Flags())

	return c
}

var (
	// for test monkey patching
	runServer   = rest.ListenAndServe
	childLogger = func(l log.FullInterface, name string) log.MinInterface { return l.New(log.WithName(name)) }
)

func dynamicRunServer(l log.FullInterface) {
	rest.StopServer()
	l.Info("starting HTTP server", "port", port.Value(), "pw", redfishPassword)
	runServer(port.Value(), childLogger(l, "run"))

}

func initRedfishCredentials() {
	file := filepath.Join(configFolder.Value(), redfishCredFile.Value())
	err := InitFileConfig(
		file,
		func(content []byte) error {
			newPassword := string(content)
			if redfishPassword != nil && *redfishPassword == newPassword {
				return nil
			}
			redfishPassword = &newPassword
			dynamicRunServer(logger)
			return nil
		},
	)
	fatalOnError(err, fmt.Sprintf("failed to register file watch on %q", file))
}
