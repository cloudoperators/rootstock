// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"time"
)

// scrapeStatus holds the outcome of a single scrape attempt against a device.
// It is the internal type sent on the results channel after a scrape completes.
type scrapeStatus struct {
	ip         string
	timestamp  time.Time
	statusCode int
	err        error
	shed       bool // true when the job was dropped due to capacity, not a device error
}

// DeviceConfig is the input configuration for a single device from the static list.
type DeviceConfig struct {
	IP       string
	Protocol string // eg : "redfish", "snmp"
}

// ScrapeJob is returned by getScrapeJob and contains only what a scraper needs to do its work.
type ScrapeJob struct {
	IP       string
	Username string
	Password string
	Protocol string // identifies which scraper to call
}

type device struct {
	ip                  string
	username            string
	password            string
	protocol            string // matches a key in manager.scrapers
	lastScraped         time.Time
	currentlyScraped    bool
	consecutiveFailures int // counts consecutive failures for backoff calculation
}
