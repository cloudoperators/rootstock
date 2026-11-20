package model

import (
	"fmt"
	"net/netip"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// func New() (*Metric, error) {

// }

// Metric is the common output of any scraper before translation into Prometheus metrics.
type Metric struct {
	Metadata Metadata
	// RawKey    RawKey
	FinalKey  string
	Help      string
	Type      MetricType // TODO: ensure that only valid types enter in constructor
	Value     float64
	Labels    map[string]any // TODO: how to add required metaddata
	Timestamp time.Time
}

type Metadata struct {
	DeviceIP        netip.Addr
	Protocol        Protocol
	EquipmentType   string `json:"EquipmentType"`
	Name            string `json:"Name"`
	FirmwareVersion string `json:"FirmwareVersion"`
	Version         string `json:"Version"`
	Manufacturer    string `json:"Manufacturer"`
	Model           string `json:"Model"`
	SerialNumber    string `json:"SerialNumber"`
}

// type RawKey struct {
// 	Route   string
// 	JSONKey []string
// 	Name    string
// }

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

// ToPrometheus translates the Metric into a prometheus.Metric ready to be sent
// on a collector channel. The label names are derived from the Labels map and
// sorted deterministically so that the generated prometheus.Desc is stable
// across invocations.
func (m Metric) ToPrometheus() (prometheus.Metric, error) {
	labelNames := make([]string, 0, len(m.Labels))
	for name := range m.Labels {
		labelNames = append(labelNames, name)
	}

	sort.Strings(labelNames)

	labelValues := make([]string, 0, len(labelNames))
	for _, name := range labelNames {
		labelValues = append(labelValues, fmt.Sprintf("%v", m.Labels[name]))
	}

	desc := prometheus.NewDesc(m.FinalKey, m.Help, labelNames, nil)

	metric, err := prometheus.NewConstMetric(
		desc, m.Type.prometheusType(), m.Value, labelValues...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create prometheus metric (%q,%+v): %w",
			m.FinalKey, m.Labels, err,
		)
	}

	if m.Timestamp.IsZero() {
		return metric, nil
	}

	return prometheus.NewMetricWithTimestamp(m.Timestamp, metric), nil
}

func (t MetricType) prometheusType() prometheus.ValueType {
	switch t {
	case Gauge:
		return prometheus.GaugeValue
	case Counter:
		return prometheus.CounterValue
	default:
		return prometheus.UntypedValue
	}
}
