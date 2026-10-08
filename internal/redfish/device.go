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


type DeviceConfig struct {
    IP    string
    PDUID string
}

type device struct {
	deviceID            string
	ip                  string
	pduID				string
	auth                Auth
	lastScraped         time.Time
	currentlyScraped    bool
	consecutiveFailures []ScrapeResult
}

