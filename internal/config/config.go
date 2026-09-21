// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServiceName string `default:"rootstock" envconfig:"SERVICE_NAME"`

	ServiceEnv string `default:"PROD" envconfig:"SERVICE_ENV"`

	ServicePort int `default:"9220" envconfig:"SERVICE_PORT"`

	// Redfish credentials used to authenticate against PDUs.
	RedfishUsername string `envconfig:"REDFISH_USERNAME" required:"true"`
	RedfishPassword string `envconfig:"REDFISH_PASSWORD" required:"true"`

	ScrapeTimeoutSeconds int `default:"30" envconfig:"SCRAPE_TIMEOUT"`
}

func ReadConfig() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return &c, nil
}
