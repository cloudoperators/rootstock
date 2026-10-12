// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"
	"sync"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

type Manager interface {
	// StartWorker launches the configured number of scraper workers.
	StartWorker(ctx context.Context)
}

type manager struct {
	devices     map[string]*device
	mu          sync.RWMutex
	interval    time.Duration // how often each device should be scraped
	workerCount int
	timeout     time.Duration
	logger      log.MinInterface
}

// takes the list of IPs, creates a device for each one and puts then in the map
func NewManager(
	configs []DeviceConfig,
	credentials Auth,
	interval time.Duration,
	workerCount int,
	timeout time.Duration,
	logger log.MinInterface,
) Manager {
	devices := make(map[string]*device)

	for _, cfg := range configs {
		devices[cfg.IP] = &device{
			deviceID: cfg.IP,
			ip:       cfg.IP,
			auth:     credentials,
		}
	}

	return &manager{
		devices:     devices,
		interval:    interval,
		workerCount: workerCount,
		timeout:     timeout,
		logger:      logger,
	}
}

// worker calls this to get its next job
func (m *manager) getScrapeJob() *device {
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

// worker calls this after finising. Updates the device.
func (m *manager) scrapeResult(deviceID string, timestamp time.Time, result ScrapeResult) {
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

func (m *manager) StartWorker(ctx context.Context) {
	for range m.workerCount {
		s := newScraper(m)
		go s.run(ctx)
	}
}
