// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package manager

import (
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
			deviceID: ip,
			ip:       ip,
			auth:     Auth{Username: "admin", Password: "pass"},
		}
	}

	return &manager{
		devices:      devices,
		interval:     30 * time.Second,
		workerCount:  2,
		timeout:      5 * time.Second,
		pollInterval: 1 * time.Second,
		logger:       log.Default(),
	}
}

// TestNewManager verifies that NewManager correctly registers all provided IPs as devices.
func TestNewManager(t *testing.T) {
	tests := []struct {
		name      string
		ips       []string
		wantCount int
	}{
		{name: "no devices", ips: []string{}, wantCount: 0},
		{name: "single device", ips: []string{"10.0.0.1"}, wantCount: 1},
		{name: "multiple devices", ips: []string{"10.0.0.1", "10.0.0.2"}, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(tt.ips...)

			if len(m.devices) != tt.wantCount {
				t.Errorf("expected %d devices, got %d", tt.wantCount, len(m.devices))
			}

			for _, ip := range tt.ips {
				d, exists := m.devices[ip]
				if !exists {
					t.Fatalf("expected device for IP %s, not found", ip)
				}

				if d.ip != ip {
					t.Errorf("expected ip %s, got %s", ip, d.ip)
				}

				if d.currentlyScraped {
					t.Errorf("new device should NOT be currently scraped")
				}
			}
		})
	}
}

// TestGetScrapeJob verifies that getScrapeJob returns a job only when a device is due for scraping.
func TestGetScrapeJob(t *testing.T) {
	tests := []struct {
		name                string
		ips                 []string
		currentlyScraped    bool
		lastScraped         time.Time
		consecutiveFailures []ScrapeResult
		wantJob             bool
		wantDeviceID        string
		wantFailureCount    int
	}{
		{
			name:         "device never scraped returns job",
			ips:          []string{"10.0.0.1"},
			wantJob:      true,
			wantDeviceID: "10.0.0.1",
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
			name:         "device scraped past interval returns job",
			ips:          []string{"10.0.0.1"},
			lastScraped:  time.Now().Add(-31 * time.Second),
			wantJob:      true,
			wantDeviceID: "10.0.0.1",
		},
		{
			name:        "device with failures applies backoff and returns nil",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-31 * time.Second),
			consecutiveFailures: []ScrapeResult{
				{Err: errors.New("timeout")},
				{Err: errors.New("timeout")},
			},
			wantJob: false,
		},
		{
			name:        "device with 10 failures resets and returns job",
			ips:         []string{"10.0.0.1"},
			lastScraped: time.Now().Add(-31 * time.Second),
			consecutiveFailures: func() []ScrapeResult {
				failures := make([]ScrapeResult, 10)
				for i := range failures {
					failures[i] = ScrapeResult{Err: errors.New("timeout")}
				}

				return failures
			}(),
			wantJob:          true,
			wantDeviceID:     "10.0.0.1",
			wantFailureCount: 0,
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
					t.Errorf("expected nil, got job for device %s", job.DeviceID)
				}

				return
			}

			if job == nil {
				t.Fatal("expected a job, got nil")
			}

			if job.DeviceID != tt.wantDeviceID {
				t.Errorf("expected device %s, got %s", tt.wantDeviceID, job.DeviceID)
			}

			if !m.devices["10.0.0.1"].currentlyScraped {
				t.Error("expected currentlyScraped to be true after getScrapeJob")
			}

			if len(m.devices["10.0.0.1"].consecutiveFailures) != tt.wantFailureCount {
				t.Errorf("expected %d failures, got %d", tt.wantFailureCount, len(m.devices["10.0.0.1"].consecutiveFailures))
			}
		})
	}
}

// updateDevice should clear failures and mark device available on success.
func TestUpdateDevice_Success(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].consecutiveFailures = []ScrapeResult{
		{Err: errors.New("timeout")},
		{Err: errors.New("timeout")},
	}

	m.updateDevice("10.0.0.1", time.Now(), ScrapeResult{Err: nil})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 0 {
		t.Errorf("expected 0 failures after success, got %d", len(d.consecutiveFailures))
	}

	if d.currentlyScraped {
		t.Error("expected currentlyScraped to be false after scrapeResult")
	}
}

// updateDevice should append to consecutiveFailures on error.
func TestUpdateDevice_Failures(t *testing.T) {
	m := newTestManager("10.0.0.1")

	m.updateDevice("10.0.0.1", time.Now(), ScrapeResult{Err: errors.New("connection refused")})
	m.updateDevice("10.0.0.1", time.Now(), ScrapeResult{Err: errors.New("timeout")})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(d.consecutiveFailures))
	}
}
