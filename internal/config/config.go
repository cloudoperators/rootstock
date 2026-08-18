// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package config holds the runtime configuration for the rootstock service.

package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServiceName string `envconfig:"SERVICE_NAME" default:"rootstock"`

	Env string `envconfig:"SERVICE_ENV" default:"PROD"`

	Port int `envconfig:"PORT" default:"9220"`

	// Redfish credentials used to authenticate against PDUs.
	RedfishUsername string `envconfig:"REDFISH_USERNAME" required:"true"`
	RedfishPassword string `envconfig:"REDFISH_PASSWORD" required:"true"`

	ScrapeTimeoutSeconds int `envconfig:"SCRAPE_TIMEOUT" default:"30"`
}

func ReadConfig() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return &c, nil
}
