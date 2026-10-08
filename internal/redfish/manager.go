// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"sync"
	"time"
)

type Manager struct {
	devices 	map[string]*device
	mu			sync.RWMutex
	interval	time.Duration //how often each device should be scraped
}

//takes the list of IPs, creates a device for each one and puts then in the map
func NewManager(ips []string, credentials auth, interval time.Duration) *Manager {
	devices := make(map[string]*device)

	for _, ip := range ips {
		devices[ip] = &device{
			deviceID:	ip,
			ip:		 	ip,
			auth: 		credentials,
		}
	}

	return &Manager{
		devices:	devices,
		interval:	interval,
	}
}


//worker calls this to get its next job
func (m *Manager) getScrapeJob() *device {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range m.devices {
		if !d.currentlyScraped && time.Since(d.lastScraped) >= m.interval {
			d.currentlyScraped = true
			return d
		}
	}

	return nil
}

//worker calls this after finising. Updates the device.
func (m *Manager) scrapeResult(deviceID string, timestamp time.Time, result ScrapeResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, exists := m.devices[deviceID]
	if !exists {
		return
	}

	d.currentlyScraped = false
	d.lastScraped = timestamp

	if result.Err == nil {
		d.consecutiveFailures = nil
	} else {
		d.consecutiveFailures = append(d.consecutiveFailures, result)
	}
}