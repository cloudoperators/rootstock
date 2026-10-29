// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

type Manager interface {
	// Run launches the configured number of scraper workers.
	Run(ctx context.Context)
}

type manager struct {
	devices        map[string]*device
	mu             sync.RWMutex
	scrapeInterval time.Duration // how often each device should be scraped
	workerCount    int
	timeout        time.Duration                                                    // max time per scrape attempt
	pollInterval   time.Duration                                                    // dispatcher scan frequency
	scrapers       map[string]func(ctx context.Context, job ScrapeJob) (int, error) // maps protocol to scraper
	logger         log.FullInterface
	results        chan scrapeStatus // workers push outcomes, state updater reads from it
	activeScrapes  int
}

// NewManager creates a manager from the given device configs and starts managing scrapes.
func NewManager(
	configs []DeviceConfig,
	username string,
	password string,
	scrapeInterval time.Duration,
	workerCount int,
	timeout time.Duration,
	pollInterval time.Duration,
	scrapers map[string]func(ctx context.Context, job ScrapeJob) (int, error),
	logger log.FullInterface,
) Manager {
	devices := make(map[string]*device)

	for _, cfg := range configs {
		devices[cfg.IP] = &device{
			ip:       cfg.IP,
			username: username,
			password: password,
			protocol: cfg.Protocol,
		}
	}

	return &manager{
		devices:        devices,
		scrapeInterval: scrapeInterval,
		workerCount:    workerCount,
		timeout:        timeout,
		pollInterval:   pollInterval,
		scrapers:       scrapers,
		logger:         logger,
		// Buffer must be >= workerCount: at most workerCount scrapes are ever in
		// flight, so a full buffer lets every scrape send its result without blocking
		// (even during shutdown when updateState has stopped reading), guaranteeing
		// no scrape goroutine leaks and every reserved worker slot is released.
		results: make(chan scrapeStatus, workerCount),
	}
}

func (m *manager) Run(ctx context.Context) {
	go m.dispatcher(ctx)  // one dispatcher scanning for ready devices
	go m.updateState(ctx) // one state updater processing results
}

// dispatcher scans for devices ready to scrape and launches a scrape goroutine
// for each. Runs as a single goroutine.
func (m *manager) dispatcher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.pollInterval):
			for _, job := range m.getScrapeJobs() {
				go m.scrape(ctx, job)
			}
		}
	}
}

// getScrapeJobs returns all devices due for scraping, up to the number of free
// worker slots, reserving a slot for each. Devices beyond capacity wait for the
// next poll rather than being launched and dropped.
func (m *manager) getScrapeJobs() []*ScrapeJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	var jobs []*ScrapeJob

	for _, d := range m.devices {
		// all worker slots are reserved; remaining devices wait for the next poll
		if m.activeScrapes >= m.workerCount {
			break
		}

		// failureCount drives the backoff: each consecutive failure adds one
		// extra interval before the device is retried.
		failureCount := d.consecutiveFailures

		if failureCount >= 10 {
			m.logger.Error("device repeatedly failing, resetting", "device", d.ip)
			d.consecutiveFailures = 0
			failureCount = 0
		}

		waitTime := max(time.Duration(failureCount)*m.scrapeInterval, m.scrapeInterval)

		if !d.currentlyScraped && time.Since(d.lastScraped) >= waitTime {
			d.currentlyScraped = true
			m.activeScrapes++

			jobs = append(jobs, &ScrapeJob{
				IP:       d.ip,
				Username: d.username,
				Password: d.password,
				Protocol: d.protocol,
			})
		}
	}

	return jobs
}

func (m *manager) scrape(ctx context.Context, job *ScrapeJob) {
	scraper, ok := m.scrapers[job.Protocol]
	if !ok {
		m.logger.Error("no scraper found for protocol", "protocol", job.Protocol)

		m.results <- scrapeStatus{
			ip:        job.IP,
			timestamp: time.Now(),
			err:       fmt.Errorf("no scraper for protocol %q", job.Protocol),
		}

		return
	}

	// child context limits how long this single scrape may take
	scrapeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	statusCode, err := scraper(scrapeCtx, *job)

	// send outcome to results channel for updateState to process
	m.results <- scrapeStatus{
		ip:         job.IP,
		timestamp:  time.Now(),
		statusCode: statusCode,
		err:        err,
	}
}

// updateState reads scrape outcomes from the results channel and updates device state.
// Runs as a single goroutine.
func (m *manager) updateState(ctx context.Context) {
	logger := m.logger.With("component", "updateState")

	for {
		select {
		case <-ctx.Done():
			return
		case outcome := <-m.results:
			m.recordScrapeResult(logger, outcome)
		}
	}
}

func (m *manager) recordScrapeResult(logger log.FullInterface, outcome scrapeStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// release the worker slot reserved for this job in getScrapeJobs
	if m.activeScrapes > 0 {
		m.activeScrapes--
	}

	d, exists := m.devices[outcome.ip]
	if !exists {
		return
	}

	d.currentlyScraped = false
	d.lastScraped = outcome.timestamp

	if outcome.err != nil {
		d.consecutiveFailures++
		logger.Error("scrape failed",
			"device", outcome.ip,
			"consecutiveFailures", d.consecutiveFailures,
			"error", outcome.err,
		)
	} else {
		d.consecutiveFailures = 0
	}
}
