// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"
	"net/netip"
	"time"
)

type Protocol string

const (
	Redfish Protocol = "redfish"
	SNMP    Protocol = "snmp"
)

// RawKey captures exactly where a metric value came from, before any translation.
type RawKey struct {
	Route   string
	JSONKey []string
	Name    string
}

// Metric is the common output of any scraper before translation into Prometheus metrics.
// After scraping: RawKey is populated, FinalKey is empty.
// After translation: FinalKey holds the final Prometheus metric name.
type Metric struct {
	DeviceIP  netip.Addr
	Protocol  Protocol
	RawKey    RawKey
	FinalKey  string
	Value     float64
	Labels    map[string]any
	Timestamp time.Time
}

// NewRedfish is called once per key by the filter.yaml processor.
// It decodes rawdata, extracts the value at key, applies the filter.yaml
// rule for route+key, and returns one constructed Metric.
func NewRedfish(rawIP, route, key string, rawdata []byte) (*Metric, error) {
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP %q: %w", rawIP, err)
	}

	// TODO (next PR): decode rawdata, extract value at key path,
	// find filter.yaml rule for route+key,
	// construct FinalKey and add translation labels.

	return &Metric{
		DeviceIP: ip,
	}, nil
}
