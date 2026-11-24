// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FilterConfig is the top-level representation of filter.yaml. It describes how
// to build Metric objects from raw scraper payloads by declaring, per protocol
// and route, the metrics to emit along with their value reference and label set.
type FilterConfig struct {
	Filters Filters `yaml:"filters"`
}

// Filters groups the metric rules by protocol.
type Filters struct {
	// Redfish is keyed first by vendor, then by route. Each route holds an
	// ordered list of metric rules.
	Redfish map[string]map[string][]MetricRule `yaml:"redfish"`
}

// MetricRule describes how to build a single metric from a redfish route
// payload.
type MetricRule struct {
	// Key is the final metric name.
	Key string `yaml:"key"`
	// Value is a JSON reference (e.g. ".Reading") resolved against the raw
	// payload to obtain the metric's sample value.
	Value string `yaml:"value"`
	// Type is the prometheus metric type (e.g. "gauge", "counter").
	Type MetricType `yaml:"type"`
	// Help is the metric help text.
	Help string `yaml:"help"`
	// Labels maps a label name to a value specification. A leading-dot value
	// (e.g. ".Status.State") is a JSON reference resolved against the raw
	// payload; anything else is treated as a constant.
	Labels map[string]string `yaml:"labels"`
}

// NewRedfishFilter reads and parses the redfish filter configuration at path.
func NewRedfishFilter(path string) (*FilterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read filter config %q: %w", path, err)
	}

	var cfg FilterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter config %q: %w", path, err)
	}

	return &cfg, nil
}
