// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"fmt"
	"testing"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

func newTestManager(ips ...string) *Manager {
	configs := make([]DeviceConfig, len(ips))
	for i, ip := range ips {
		configs[i] = DeviceConfig{IP: ip}
	}

	return NewManager(configs, Auth{Username: "admin", Password: "pass"}, 30*time.Second, 2, 5*time.Second, log.Default())
}

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

func TestGetScrapeJobReturnsDueDevice(t *testing.T) {
	m := newTestManager("10.0.0.1")

	job := m.getScrapeJob()

	if job == nil {
		t.Fatal("expected a job, got nil")
	}

	if job.deviceID != "10.0.0.1" {
		t.Errorf("expected device 10.0.0.1, got %s", job.deviceID)
	}

	if !job.currentlyScraped {
		t.Error("expected currentlyScraped to be true after getScrapeJob")
	}
}

func TestGetScrapeJobSkipsCurrentlyScraped(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].currentlyScraped = true

	job := m.getScrapeJob()

	if job != nil {
		t.Error("expected nil, device is already being scraped")
	}
}

func TestGetScrapeJobSkipsRecentlyScraped(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].lastScraped = time.Now().Add(-5 * time.Second)

	job := m.getScrapeJob()

	if job != nil {
		t.Error("expected nil, device was scraped recently")
	}
}

func TestScrapeResultSuccessClearsFailures(t *testing.T) {
	m := newTestManager("10.0.0.1")
	m.devices["10.0.0.1"].consecutiveFailures = []ScrapeResult{
		{Err: fmt.Errorf("timeout")},
		{Err: fmt.Errorf("timeout")},
	}

	m.scrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: nil})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 0 {
		t.Errorf("expected 0 failures after success, got %d", len(d.consecutiveFailures))
	}

	if d.currentlyScraped {
		t.Error("expected currentlyScraped to be false after scrapeResult")
	}
}

func TestScrapeResultAccumulatesFailures(t *testing.T) {
	m := newTestManager("10.0.0.1")

	m.scrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: fmt.Errorf("connection refused")})
	m.scrapeResult("10.0.0.1", time.Now(), ScrapeResult{Err: fmt.Errorf("timeout")})

	d := m.devices["10.0.0.1"]
	if len(d.consecutiveFailures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(d.consecutiveFailures))
	}
}
