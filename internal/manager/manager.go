// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"sync"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

// Scraper is implemented by protocol-specific scrapers (e.g. Redfish, SNMP).
type Scraper interface {
	// Scrape performs a single scrape of the given device and returns the result.
	Scrape(ctx context.Context, job ScrapeJob) ScrapeResult
}
type Manager interface {
	// StartWorker launches the configured number of scraper workers.
	StartWorker(ctx context.Context)
}

type manager struct {
	devices      map[string]*device
	mu           sync.RWMutex
	interval     time.Duration // how often each device should be scraped
	workerCount  int
	timeout      time.Duration // max time a single scrape attempt may take
	pollInterval time.Duration // how long a worker sleeps when no device is ready to scrape
	scraper      Scraper
	logger       log.MinInterface
}

// takes the list of IPs, creates a device for each one and puts then in the map
func NewManager(
	configs []DeviceConfig,
	credentials Auth,
	interval time.Duration,
	workerCount int,
	timeout time.Duration,
	pollInterval time.Duration,
	scraper Scraper,
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
		devices:      devices,
		interval:     interval,
		workerCount:  workerCount,
		timeout:      timeout,
		pollInterval: pollInterval,
		scraper:      scraper,
		logger:       logger,
	}
}

// worker calls this to get its next job
func (m *manager) getScrapeJob() *ScrapeJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range m.devices {
		// failureCount drives the backoff: each consecutive failure adds one
		// extra interval before the device is retried.
		failureCount := len(d.consecutiveFailures)

		if failureCount >= 10 {
			m.logger.Error("device repeatedly failing, resetting", "device", d.deviceID)
			d.consecutiveFailures = nil
			failureCount = 0
		}

		waitTime := max(time.Duration(failureCount)*m.interval, m.interval)

		if !d.currentlyScraped && time.Since(d.lastScraped) >= waitTime {
			d.currentlyScraped = true

			return &ScrapeJob{
				DeviceID: d.deviceID,
				IP:       d.ip,
				Auth:     d.auth,
			}
		}
	}

	return nil
}

// worker calls this after finising. Updates the device.
func (m *manager) updateDevice(deviceID string, timestamp time.Time, result ScrapeResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	d, exists := m.devices[deviceID]
	if !exists {
		return
	}

	d.currentlyScraped = false
	d.lastScraped = timestamp

	if result.Err != nil {
		d.consecutiveFailures = append(d.consecutiveFailures, result)
	} else {
		d.consecutiveFailures = nil
	}
}

// runWorker is the main loop for a single worker goroutine. It continuously polls
// for the next device to scrape, calls the scraper, and reports the result back.
func (m *manager) runWorker(ctx context.Context) {
	for {
		job := m.getScrapeJob()
		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(m.pollInterval):
			}

			continue
		}

		scrapeCtx, cancel := context.WithTimeout(ctx, m.timeout)
		timestamp := time.Now()
		result := m.scraper.Scrape(scrapeCtx, *job)

		defer cancel()

		m.updateDevice(job.DeviceID, timestamp, result)
	}
}

func (m *manager) StartWorker(ctx context.Context) {
	for range m.workerCount {
		go m.runWorker(ctx)
	}
}
