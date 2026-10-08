// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import {
	"fmt"
	"testing"
	"time"
}

func TestNewManager(t *testing.T) {
	ips := []string{"10.0.0.1", "10.0.0.2"}
	credentials := auth{username: "admin", password: "pass"}
	interval := 30 *time.Second

	m := NewManager(ips, credentials, interval)

	if len(m.devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(m.devices))
	}

	for _, ip := range ips {
		d, exists := m.devices[ip]
		if !exists {
			t.Errorf("expected device for IP %s, not found", ip)
		}
		if d.ip != ip {
			t.Errorf("expected ip %s, got %s", ip, d.ip)
		}
		if d.currentlyScraped {
			t.Errorf("new device should be currently scraped")
		}
	}
}

//Manager with one device, never scraped before
//getScrapeJob returns a due device
//more tests to follow



