// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import "time"

type Auth struct {
	Username string
	Password string
}

// ScrapeResult holds the outcome of a single scrape attempt against a device.
type ScrapeResult struct {
	StatusCode int
	Err        error
}

// DeviceConfig is the input configuration for a single device from the static list.
type DeviceConfig struct {
	IP string
}

type device struct {
	deviceID            string
	ip                  string
	auth                Auth
	lastScraped         time.Time
	currentlyScraped    bool
	consecutiveFailures []ScrapeResult
}
