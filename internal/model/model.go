// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"
)

type Protocol string

const (
	Redfish Protocol = "redfish"
	SNMP    Protocol = "snmp"
)

// RawKey captures exactly where a metric value came from, before any translation.
// type RawKey struct {
// 	Route   string
// 	JSONKey []string
// 	Name    string
// }

type Metadata struct {
	DeviceIP        string   `json:"-"`
	Protocol        Protocol `json:"-"`
	EquipmentType   string   `json:"EquipmentType"`
	Name            string   `json:"Name"`
	FirmwareVersion string   `json:"FirmwareVersion"`
	Manufacturer    string   `json:"Manufacturer"`
	Model           string   `json:"Model"`
	SerialNumber    string   `json:"SerialNumber"`
}

type MetricType string

const (
	Gauge   MetricType = "gauge"
	Counter MetricType = "counter"
)

// Metric is the common output of any scraper before translation into Prometheus metrics.
type Metric struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Help      string
	Type      MetricType
	Timestamp time.Time
}

func NewRedfish(route string, rawBytes []byte, metadata Metadata) (*Metric, error) {
	// TODO: decode rawBytes, extract values at JSON key paths,
	// apply filter.yaml rules for route+key,
	// construct Metric with metadata fields as labels.
	return nil, nil
}
