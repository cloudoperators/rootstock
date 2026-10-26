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
	jobs           chan ScrapeJob    // dispatcher pushes jobs, workers read from it
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
		jobs:           make(chan ScrapeJob, workerCount),
		results:        make(chan scrapeStatus, workerCount),
	}
}

func (m *manager) Run(ctx context.Context) {
	go m.dispatcher(ctx)  // one dispatcher scanning for ready devices
	go m.updateState(ctx) // one state updater processing results

	for i := range m.workerCount {
		go m.runner(ctx, i) // N workers scraping
	}
}

// dispatcher scans for devices ready to scrape and pushes jobs onto the jobs channel.
// Runs as a single goroutine.
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

// getScrapeJob returns the next device that is due for scraping, or nil if none are ready.
func (m *manager) getScrapeJobs() []*ScrapeJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range m.devices {
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

			// TODO: fix return
			return &ScrapeJob{
				IP:       d.ip,
				Username: d.username,
				Password: d.password,
				Protocol: d.protocol,
			}
		}
	}

	return nil
}

func (m *manager) addScrape() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeScrapes >= m.workerCount {
		return fmt.Errorf("too many active scrapes: %d >= %d", m.activeScrapes, m.workerCount)
	}
	m.activeScrapes++
	return nil
}

func (m *manager) subtractScrape() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeScrapes == 0 {
		return
	}
	m.activeScrapes--
}

// TODO
func (m *manager) scrape(ctx context.Context, job *ScrapeJob) {
	if err := m.addScrape(); err != nil {
		m.logger.Error("shedding job: too many active scrapes",
			"ip", job.IP, "protocol", job.Protocol, log.KeyError, err,
		)
		return
	}

	scraper, ok := m.scrapers[job.Protocol]
	if !ok {
		m.logger.Error("no scraper found for protocol", "protocol", job.Protocol)
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

	m.subtractScrape()
}

// runner reads scrape jobs from the jobs channel, calls the appropriate
// scraper, and sends the outcome to the results channel.
func (m *manager) runner(ctx context.Context, id int) {
	logger := m.logger.With("component", "runner", "workerID", id)

	for {
		select {
		case <-ctx.Done():
			return
		case job := <-m.jobs: // blocks until dispatcher pushes a job
			// look up the scraper function for this device's protocol
			scrape, ok := m.scrapers[job.Protocol]
			if !ok {
				logger.Error("no scraper found for protocol", "protocol", job.Protocol)
				continue
			}

			// child context limits how long this single scrape may take
			scrapeCtx, cancel := context.WithTimeout(ctx, m.timeout)
			timestamp := time.Now()
			statusCode, err := scrape(scrapeCtx, job)

			cancel()

			// send outcome to results channel for updateState to process
			m.results <- scrapeStatus{
				ip:         job.IP,
				timestamp:  timestamp,
				statusCode: statusCode,
				err:        err,
			}
		}
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
			m.mu.Lock()

			d, exists := m.devices[outcome.ip]
			if !exists {
				m.mu.Unlock()
				continue
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

			m.mu.Unlock()
		}
	}
}
