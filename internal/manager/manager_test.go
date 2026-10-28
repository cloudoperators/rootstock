// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/pkg/log"
)

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
		pollInterval:   10 * time.Millisecond,
		logger:         log.Default(),
		results:        make(chan scrapeStatus, 2),
	}
}

func TestGetScrapeJobs(t *testing.T) {
	tests := []struct {
		name                string
		ips                 []string
		currentlyScraped    bool
		lastScraped         time.Time
		consecutiveFailures int
		wantCount           int    // expected number of jobs returned
		wantIP              string // checked only when wantCount == 1
		wantFailureCount    int    // checked only when wantCount == 1
	}{
		{
			name:      "device never scraped returns job",
			ips:       []string{"10.0.0.1"},
			wantCount: 1,
			wantIP:    "10.0.0.1",
		},
		{
			name:             "device currently being scraped returns nothing",
			ips:              []string{"10.0.0.1"},
			currentlyScraped: true,
			wantCount:        0,
		},
		{
			name:        "device scraped recently returns nothing",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-5 * time.Second),
			wantCount:   0,
		},
		{
			name:        "device scraped past interval returns job",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-31 * time.Second),
			wantCount:   1,
			wantIP:      "10.0.0.1",
		},
		{
			name:                "device with failures applies backoff",
			ips:                 []string{"10.0.0.1"},
			lastScraped:         time.Now().Add(-31 * time.Second),
			consecutiveFailures: 2, // waitTime = 60s, only 31s elapsed
			wantCount:           0,
		},
		{
			name:                "device with 10 consecutive failures resets failure count",
			ips:                 []string{"10.0.0.1"},
			lastScraped:         time.Now().Add(-31 * time.Second),
			consecutiveFailures: 10,
			wantCount:           1,
			wantIP:              "10.0.0.1",
			wantFailureCount:    0,
		},
		{
			name:      "no devices returns empty",
			ips:       []string{},
			wantCount: 0,
		},
		{
			name:      "two ready devices returns both",
			ips:       []string{"10.0.0.1", "10.0.0.2"},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(tt.ips...)
			for _, ip := range tt.ips {
				m.devices[ip].currentlyScraped = tt.currentlyScraped
				m.devices[ip].lastScraped = tt.lastScraped
				m.devices[ip].consecutiveFailures = tt.consecutiveFailures
			}

			jobs := m.getScrapeJobs()

			if len(jobs) != tt.wantCount {
				t.Fatalf("expected %d jobs, got %d", tt.wantCount, len(jobs))
			}

			if tt.wantCount != 1 {
				return
			}

			if jobs[0].IP != tt.wantIP {
				t.Errorf("expected IP %s, got %s", tt.wantIP, jobs[0].IP)
			}

			if !m.devices[tt.wantIP].currentlyScraped {
				t.Error("expected currentlyScraped to be true after getScrapeJobs")
			}

			if m.devices[tt.wantIP].consecutiveFailures != tt.wantFailureCount {
				t.Errorf("expected %d failures, got %d", tt.wantFailureCount, m.devices[tt.wantIP].consecutiveFailures)
			}
		})
	}
}

func TestUpdateState(t *testing.T) {
	tests := []struct {
		name            string
		initialFailures int
		outcome         scrapeStatus
		wantFailures    int
		wantLastScraped bool // true = lastScraped should be set to a non-zero time
	}{
		{
			name:            "success clears consecutive failures",
			initialFailures: 2,
			outcome:         scrapeStatus{ip: "10.0.0.1", timestamp: time.Now(), err: nil},
			wantFailures:    0,
			wantLastScraped: true,
		},
		{
			name:            "error increments consecutive failures",
			initialFailures: 0,
			outcome:         scrapeStatus{ip: "10.0.0.1", timestamp: time.Now(), err: errors.New("connection refused")},
			wantFailures:    1,
			wantLastScraped: true,
		},
		{
			name:            "shed does not count as failure and does not update lastScraped",
			initialFailures: 3,
			outcome:         scrapeStatus{ip: "10.0.0.1", shed: true},
			wantFailures:    3,
			wantLastScraped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("10.0.0.1")
			m.devices["10.0.0.1"].consecutiveFailures = tt.initialFailures
			m.devices["10.0.0.1"].currentlyScraped = true

			go m.updateState(t.Context())

			m.results <- tt.outcome

			// Poll until currentlyScraped is false — applyOutcome sets this last under the lock,
			// so once we observe it, all other state changes are also done.
			require.Eventually(t, func() bool {
				m.mu.RLock()
				defer m.mu.RUnlock()

				return !m.devices["10.0.0.1"].currentlyScraped
			}, 100*time.Millisecond, time.Millisecond)

			m.mu.RLock()
			failures := m.devices["10.0.0.1"].consecutiveFailures
			lastScraped := m.devices["10.0.0.1"].lastScraped
			m.mu.RUnlock()

			if failures != tt.wantFailures {
				t.Errorf("expected %d failures, got %d", tt.wantFailures, failures)
			}

			if tt.wantLastScraped && lastScraped.IsZero() {
				t.Error("expected lastScraped to be updated, got zero value")
			}

			if !tt.wantLastScraped && !lastScraped.IsZero() {
				t.Error("expected lastScraped to remain zero for shed job")
			}
		})
	}
}

func TestScrape(t *testing.T) {
	tests := []struct {
		name           string
		protocol       string
		scrapers       map[string]func(ctx context.Context, job ScrapeJob) (int, error)
		preFillScrapes int // pre-set activeScrapes to simulate being at capacity
		wantShed       bool
		wantErr        bool
		wantStatusCode int
	}{
		{
			name:     "success sends result with status code",
			protocol: "redfish",
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){
				"redfish": func(_ context.Context, _ ScrapeJob) (int, error) { return 200, nil },
			},
			wantStatusCode: 200,
		},
		{
			name:     "scraper error sends result with error",
			protocol: "redfish",
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){
				"redfish": func(_ context.Context, _ ScrapeJob) (int, error) {
					return 0, errors.New("connection refused")
				},
			},
			wantErr: true,
		},
		{
			name:     "unknown protocol sends error result",
			protocol: "snmp",
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){},
			wantErr:  true,
		},
		{
			name:           "at capacity sheds job without counting as failure",
			protocol:       "redfish",
			preFillScrapes: 2, // workerCount = 2, already at capacity
			scrapers: map[string]func(ctx context.Context, job ScrapeJob) (int, error){
				"redfish": func(_ context.Context, _ ScrapeJob) (int, error) { return 200, nil },
			},
			wantShed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("10.0.0.1")
			m.scrapers = tt.scrapers
			m.activeScrapes = tt.preFillScrapes

			go m.scrape(t.Context(), &ScrapeJob{
				IP:       "10.0.0.1",
				Username: "admin",
				Password: "pass",
				Protocol: tt.protocol,
			})

			select {
			case result := <-m.results:
				if result.shed != tt.wantShed {
					t.Errorf("expected shed=%v, got %v", tt.wantShed, result.shed)
				}

				if tt.wantErr && result.err == nil {
					t.Error("expected an error, got nil")
				}

				if !tt.wantErr && !tt.wantShed && result.err != nil {
					t.Errorf("expected no error, got %v", result.err)
				}

				if result.statusCode != tt.wantStatusCode {
					t.Errorf("expected status code %d, got %d", tt.wantStatusCode, result.statusCode)
				}

			case <-time.After(100 * time.Millisecond):
				t.Fatal("timed out waiting for scrape result")
			}
		})
	}
}

func TestDispatcher(t *testing.T) {
	t.Run("ready device gets dispatched and scraped", func(t *testing.T) {
		m := newTestManager("10.0.0.1")
		m.scrapers = map[string]func(ctx context.Context, job ScrapeJob) (int, error){
			"redfish": func(_ context.Context, _ ScrapeJob) (int, error) { return 200, nil },
		}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go m.dispatcher(ctx)

		select {
		case result := <-m.results:
			if result.ip != "10.0.0.1" {
				t.Errorf("expected result for 10.0.0.1, got %s", result.ip)
			}

			if result.err != nil {
				t.Errorf("expected no error, got %v", result.err)
			}

		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for scrape result")
		}
	})

	t.Run("canceled context exits cleanly without scraping", func(t *testing.T) {
		m := newTestManager("10.0.0.1")

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		done := make(chan struct{})

		go func() {
			m.dispatcher(ctx)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("dispatcher did not exit after context cancellation")
		}
	})
}
