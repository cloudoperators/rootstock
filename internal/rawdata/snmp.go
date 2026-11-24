// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import (
	"fmt"
	"io"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// ParseSNMPText parses metrics in the Prometheus text exposition format (as
// produced by snmp_exporter) into the package's common Metric representation.
//
// Each sample line becomes one Metric: the metric family name is stored in
// RawKey.Name, the family HELP text is stored in Help, every label name/value
// pair is copied into Labels, and the sample value is stored in Value.
// HELP/TYPE comment lines are consumed by the parser and do not produce
// Metrics.
//
// If filter is non-nil, each parsed metric is passed through
// FilterConfig.FilterSNMP for translation; a nil filter leaves metrics
// untranslated.
func ParseSNMPText(r io.Reader, filter *FilterConfig) ([]Metric, error) {
	// UTF8Validation is the default upstream name validation scheme
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SNMP text format: %w", err)
	}

	var metrics []Metric

	for name, family := range families {
		for _, m := range family.GetMetric() {
			labels := make(map[string]any, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}

			metric := Metric{
				Protocol: SNMP,
				RawKey:   RawKey{Name: name},
				Help:     family.GetHelp(),
				Type:     metricType(family.GetType()),
				Value:    metricValue(m),
				Labels:   labels,
			}

			if filter != nil {
				metric = filter.FilterSNMP(metric)
			}

			metrics = append(metrics, metric)
		}
	}

	return metrics, nil
}

// ParseSNMPBytes is a convenience wrapper around ParseSNMPText for byte input.
func ParseSNMPBytes(data []byte, filter *FilterConfig) ([]Metric, error) {
	return ParseSNMPText(strings.NewReader(string(data)), filter)
}

// metricType maps the parsed Prometheus metric family type onto the package's
// MetricType. snmp_exporter emits gauges and counters; any other type
// (untyped, etc.) falls back to Gauge, which is the safe default for the
// instantaneous values SNMP produces.
func metricType(t dto.MetricType) MetricType {
	if t == dto.MetricType_COUNTER {
		return Counter
	}
	return Gauge
}

// metricValue extracts the numeric sample value from a parsed metric,
// supporting the metric types the SNMP text output can emit. snmp_exporter
// emits gauges and counters; untyped and summary/histogram sums are handled
// defensively.
func metricValue(m *dto.Metric) float64 {
	switch {
	case m.Gauge != nil:
		return m.GetGauge().GetValue()
	case m.Counter != nil:
		return m.GetCounter().GetValue()
	case m.Untyped != nil:
		return m.GetUntyped().GetValue()
	default:
		return 0
	}
}
