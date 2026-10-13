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
	configs := make([]DeviceConfig, len(ips))
	for i, ip := range ips {
		configs[i] = DeviceConfig{IP: ip}
	}

	m, ok := NewManager(
		configs,
		Auth{Username: "admin", Password: "pass"},
		30*time.Second,
		2,
		5*time.Second,
		nil,
		log.Default(),
	).(*manager)
	if !ok {
		panic("unexpected manager type")
	}

	return m
}

// TestNewManager verifies that NewManager correctly registers all provided IPs as devices.
func TestNewManager(t *testing.T) {
	m := newTestManager("10.0.0.1", "10.0.0.2")

	if len(m.devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(m.devices))
	}

	for _, ip := range []string{"10.0.0.1", "10.0.0.2"} {
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
}

// getScrapeJob should return a device that has never been scraped.
func TestGetScrapeJob_Due(t *testing.T) {
	m := newTestManager("10.0.0.1")

	job := m.getScrapeJob()

	if job == nil {
		t.Fatal("expected a job, got nil")
	}

	if job.DeviceID != "10.0.0.1" {
		t.Errorf("expected device 10.0.0.1, got %s", job.DeviceID)
	}

	if !m.devices["10.0.0.1"].currentlyScraped {
		t.Error("expected currentlyScraped to be true after getScrapeJob")
	}
}

// getScrapeJob should skip a device that is currently being scraped.
func TestGetScrapeJob_InProgress(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].currentlyScraped = true

	job := m.getScrapeJob()

	if job != nil {
		t.Error("expected nil, device is already being scraped")
	}
}

// getScrapeJob should skip a device scraped within the interval window.
func TestGetScrapeJob_Recent(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].lastScraped = time.Now().Add(-5 * time.Second)

	job := m.getScrapeJob()

	if job != nil {
		t.Error("expected nil, device was scraped recently")
	}
}

// reportScrapeResult should clear failures and mark device available on success.
func TestReportScrapeResult_Success(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].consecutiveFailures = []ScrapeResult{
		{Err: errors.New("timeout")},
		{Err: errors.New("timeout")},
	}

	m.reportScrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: nil})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 0 {
		t.Errorf("expected 0 failures after success, got %d", len(d.consecutiveFailures))
	}

	if d.currentlyScraped {
		t.Error("expected currentlyScraped to be false after scrapeResult")
	}
}

// reportScrapeResult should append to consecutiveFailures on error.
func TestReportScrapeResult_Failures(t *testing.T) {
	m := newTestManager("10.0.0.1")

	m.reportScrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: errors.New("connection refused")})
	m.reportScrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: errors.New("timeout")})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(d.consecutiveFailures))
	}
}
