// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type allMetrics map[string]Metric

// Metric is the common output of any scraper before translation into Prometheus metrics.
type Metric struct {
	DeviceIP  netip.Addr
	Protocol  Protocol
	RawKey    RawKey
	FinalKey  string
	Help      string
	Type      MetricType
	Value     float64
	Labels    map[string]any
	Timestamp time.Time
}

type RawKey struct {
	Route   string
	JSONKey []string
	Name    string
}

type (
	Protocol   string
	MetricType string
)

const (
	Redfish Protocol = "redfish"
	SNMP    Protocol = "snmp"

	Gauge   MetricType = "gauge"
	Counter MetricType = "counter"
)

func NewRedfish(rawIP, route string, rawdata []byte) (*Metric, error) {
	// parse inputs
	//   - construct IP
	//   - json decode rawData
	//   - extract value
	//   - extract labels
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rawIP as IP: %w", err)
	}

	// construct final metric
	//   - find relevant translation
	//   - construct FinalKey
	//   - add translation labels

	return &Metric{
		DeviceIP: ip,
	}, nil

}

func NewSNMP(rawIP, route, name string, rawdata []byte) (*Metric, error) {
	return nil, fmt.Errorf("NewSNMP not implemented")
}

// ToPrometheus converts a Metric into a constant prometheus.Metric.
//
// The metric family name is taken from FinalKey if set (post-translation),
// otherwise from RawKey.Name. The Help field supplies the metric help text, and
// Type selects the Prometheus value type (Counter maps to CounterValue, any
// other value—including the unset zero value—maps to GaugeValue). Every entry
// in Labels is emitted as a variable label. Because Labels is a map (with
// non-deterministic iteration order), the label names and their values are
// collected in a single ordered pass so the Desc label names stay aligned with
// the NewConstMetric values.
//
// The DeviceIP, when valid, is added as an "instance" label so samples from
// different devices remain distinct time series.
func (m Metric) ToPrometheus() (prometheus.Metric, error) {
	name := m.RawKey.Name
	if m.FinalKey != "" {
		name = m.FinalKey
	}
	if name == "" {
		return nil, fmt.Errorf("cannot build prometheus metric: empty name")
	}

	// Single ordered pass: names[i] corresponds to values[i].
	names := make([]string, 0, len(m.Labels)+1)
	values := make([]string, 0, len(m.Labels)+1)
	for k, v := range m.Labels {
		names = append(names, k)
		values = append(values, fmt.Sprintf("%v", v))
	}

	if m.DeviceIP.IsValid() {
		names = append(names, "instance")
		values = append(values, m.DeviceIP.String())
	}

	valueType := prometheus.GaugeValue
	if m.Type == Counter {
		valueType = prometheus.CounterValue
	}

	desc := prometheus.NewDesc(name, m.Help, names, nil)
	return prometheus.NewConstMetric(desc, valueType, m.Value, values...)
}
