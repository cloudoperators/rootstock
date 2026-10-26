// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

// newTestManager creates a manager instance for testing purposes.
// It accepts a list of IPs and registers each as a device with fixed credentials and intervals.
func newTestManager(ips ...string) *manager {
	devices := make(map[string]*device)
	for _, ip := range ips {
		devices[ip] = &device{
			ip:       ip,
			username: "admin",
			password: "pass",
			protocol: "redfish",
		}
	}

	return &manager{
		devices:        devices,
		scrapeInterval: 30 * time.Second,
		workerCount:    2,
		timeout:        5 * time.Second,
		pollInterval:   1 * time.Second,
		logger:         log.Default(),
		jobs:           make(chan ScrapeJob, 2),    // buffered so tests can push without a goroutine already reading
		results:        make(chan scrapeStatus, 2), // buffered so tests can push without a goroutine already reading
	}
}

// TestGetScrapeJob verifies that getScrapeJob returns a job only when a device is due for scraping.
func TestGetScrapeJob(t *testing.T) {
	tests := []struct {
		name                string
		ips                 []string
		currentlyScraped    bool
		lastScraped         time.Time
		consecutiveFailures int
		wantJob             bool   // whether getScrapeJob should return a non-nil job
		wantIP              string // expected IP on the returned job; only checked when wantJob is true
		wantFailureCount    int    // expected consecutiveFailures after getScrapeJob runs; only checked when wantJob is true
	}{
		{
			name:    "device never scraped returns job",
			ips:     []string{"10.0.0.1"},
			wantJob: true,
			wantIP:  "10.0.0.1",
		},
		{
			name:             "device currently being scraped returns nil",
			ips:              []string{"10.0.0.1"},
			currentlyScraped: true,
			wantJob:          false,
		},
		{
			name:        "device scraped recently returns nil",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-5 * time.Second),
			wantJob:     false,
		},
		{
			name:        "device scraped past interval returns job",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-31 * time.Second),
			wantJob:     true,
			wantIP:      "10.0.0.1",
		},
		{
			name:                "device with failures applies backoff and returns nil",
			ips:                 []string{"10.0.0.1"},
			lastScraped:         time.Now().Add(-31 * time.Second),
			consecutiveFailures: 2,
			wantJob:             false,
		},
		{
			name:                "device with 10 failures resets and returns job",
			ips:                 []string{"10.0.0.1"},
			lastScraped:         time.Now().Add(-31 * time.Second),
			consecutiveFailures: 10,
			wantJob:             true,
			wantIP:              "10.0.0.1",
			wantFailureCount:    0,
		},
		{
			name:    "no devices returns nil",
			ips:     []string{},
			wantJob: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(tt.ips...)
			if len(tt.ips) > 0 {
				m.devices["10.0.0.1"].currentlyScraped = tt.currentlyScraped
				m.devices["10.0.0.1"].lastScraped = tt.lastScraped
				m.devices["10.0.0.1"].consecutiveFailures = tt.consecutiveFailures
			}

			job := m.getScrapeJob()

			if !tt.wantJob {
				if job != nil {
					t.Errorf("expected nil, got job for device %s", job.IP)
				}

				return
			}

			if job == nil {
				t.Fatal("expected a job, got nil")
			}

			if job.IP != tt.wantIP {
				t.Errorf("expected device %s, got %s", tt.wantIP, job.IP)
			}

			if !m.devices["10.0.0.1"].currentlyScraped {
				t.Error("expected currentlyScraped to be true after getScrapeJob")
			}

			if m.devices["10.0.0.1"].consecutiveFailures != tt.wantFailureCount {
				t.Errorf("expected %d failures, got %d", tt.wantFailureCount, m.devices["10.0.0.1"].consecutiveFailures)
			}
		})
	}
}

// TestUpdateState verifies that updateState correctly updates device state after a scrape result.
func TestUpdateState(t *testing.T) {
	tests := []struct {
		name            string
		initialFailures int   // consecutiveFailures on the device before the result arrives
		err             error // error in the scrapeStatus sent on results channel; nil means success
		wantFailures    int   // expected consecutiveFailures after updateState processes the result
	}{
		{
			name:            "success clears consecutive failures and marks device available",
			initialFailures: 2,
			err:             nil,
			wantFailures:    0,
		},
		{
			name:            "error increments consecutive failures",
			initialFailures: 0,
			err:             errors.New("connection refused"),
			wantFailures:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("10.0.0.1")
			m.devices["10.0.0.1"].consecutiveFailures = tt.initialFailures
			m.devices["10.0.0.1"].currentlyScraped = true

			ctx := t.Context()

			go m.updateState(ctx)

			m.results <- scrapeStatus{
				ip:        "10.0.0.1",
				timestamp: time.Now(),
				err:       tt.err,
			}

			time.Sleep(10 * time.Millisecond) // give the goroutine time to process the result before we read device state

			m.mu.RLock()
			failures := m.devices["10.0.0.1"].consecutiveFailures
			scraped := m.devices["10.0.0.1"].currentlyScraped
			m.mu.RUnlock()

			if failures != tt.wantFailures {
				t.Errorf("expected %d failures, got %d", tt.wantFailures, failures)
			}

			if scraped {
				t.Errorf("expected currentlyScraped to be false after scrape completes")
			}
		})
	}
}

// dispatcher should push a ready device onto the jobs channel for the worker to pick it up
func TestDispatcher(t *testing.T) {
	tests := []struct {
		name        string
		lastScraped time.Time
		wantJob     bool // whether we expect a job to appear on the jobs channel
		cancelCtx   bool // if true, context is canceled before the dispatcher starts, so it exits immediately
	}{
		{
			name:        "ready device: pushes job to channel",
			lastScraped: time.Time{}, // never scraped
			wantJob:     true,
		},
		{
			name:        "recently scraped device: does not push job",
			lastScraped: time.Now(), // just scraped
			wantJob:     false,
		},
		{
			name:        "context canceled before push: does not push job",
			lastScraped: time.Time{}, // ready - never scraped, but context canceled before dispatcher runs
			cancelCtx:   true,
			wantJob:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("10.0.0.1")
			m.pollInterval = 10 * time.Millisecond // reduced from 1s default so the test completes fast
			m.devices["10.0.0.1"].lastScraped = tt.lastScraped

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tt.cancelCtx {
				cancel() // cancel before dispatcher starts so it exits immediately on ctx.Done()
			}

			go m.dispatcher(ctx)

			select {
			case job := <-m.jobs:
				if !tt.wantJob {
					t.Errorf("expected no job, got job for device %s", job.IP)
				}
			case <-time.After(100 * time.Millisecond):
				if tt.wantJob {
					t.Fatal("timed out waiting for job on jobs channel")
				}
			}
		})
	}
}

// assertScrapeErr checks that err matches wantErrMsg. Empty wantErrMsg means no error is expected.
func assertScrapeErr(t *testing.T, err error, wantErrMsg string) {
	t.Helper()

	if wantErrMsg == "" {
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		return
	}

	if err == nil {
		t.Errorf("expected error %q, got nil", wantErrMsg)
		return
	}

	if err.Error() != wantErrMsg {
		t.Errorf("expected error %q, got %q", wantErrMsg, err.Error())
	}
}

// TestRunner verifies that runner reads jobs, calls the correct scraper, and pushes results.
func TestRunner(t *testing.T) {
	tests := []struct {
		name           string
		protocol       string
		scrapers       map[string]func(ctx context.Context, job ScrapeJob) (int, error)
		wantResult     bool   // whether we expect a scrapeStatus to appear on the results channel
		wantStatusCode int    // expected statusCode in the result; only checked when wantResult is true
		wantErrMsg     string // expected error message; empty means no error expected
	}{
		{
			name:     "known protocol calls scraper and pushes result",
			protocol: "redfish",
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){
				"redfish": func(_ context.Context, _ ScrapeJob) (int, error) {
					return 200, nil
				},
			},
			wantResult:     true,
			wantStatusCode: 200,
		},
		{
			name:     "scraper returns error pushes result with error",
			protocol: "redfish",
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){
				"redfish": func(_ context.Context, _ ScrapeJob) (int, error) {
					return 0, errors.New("connection refused")
				},
			},
			wantResult: true,
			wantErrMsg: "connection refused",
		},
		{
			name:       "unknown protocol does not push result",
			protocol:   "abc",
			scrapers:   map[string]func(ctx context.Context, job ScrapeJob) (int, error){},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("10.0.0.1")
			m.scrapers = tt.scrapers

			ctx := t.Context()

			go m.runner(ctx, 0)

			m.jobs <- ScrapeJob{
				IP:       "10.0.0.1",
				Username: "admin",
				Password: "pass",
				Protocol: tt.protocol,
			}

			select {
			case result := <-m.results:
				if !tt.wantResult {
					t.Errorf("expected no result, got result for device %s", result.ip)
					return
				}

				if result.statusCode != tt.wantStatusCode {
					t.Errorf("expected status code %d, got %d", tt.wantStatusCode, result.statusCode)
				}

				assertScrapeErr(t, result.err, tt.wantErrMsg)
			case <-time.After(100 * time.Millisecond):
				if tt.wantResult {
					t.Fatal("timed out waiting for result on results channel")
				}
			}
		})
	}
}
