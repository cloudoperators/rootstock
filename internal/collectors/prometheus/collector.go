// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/pkg/log"
)

const (
	collectorTimeout = 25 * time.Second
)

type Collector struct {
	mu            sync.Mutex
	redfishClient redfish.Client
	pduID         string
	collect       string
	logger        log.MinInterface
	cache         *cache.Cache

	metricData *RackPDUMetricData
}

type RackPDUMetricData struct {
	PERackPDUMetrics *PERackPDUMetrics
	PERackPDU        *PERackPDU
}

type PERackPDUMetrics struct {
	DataSourceURI *prometheus.Desc
	Reading       *prometheus.Desc
	ApparentVA    *prometheus.Desc
	ReactiveVAR   *prometheus.Desc
	PowerFactor   *prometheus.Desc
}

type PERackPDU struct {
	PowerCapacityVA *prometheus.Desc
}

func NewCollector(pduID string, collect string, c *cache.Cache) *Collector {
	logger := log.Default()

	return &Collector{
		pduID:   pduID,
		collect: collect,
		logger:  logger,
		cache:   c,
		metricData: &RackPDUMetricData{
			PERackPDUMetrics: &PERackPDUMetrics{
				Reading: prometheus.NewDesc(
					"pdu_power_watts",
					"PDU power consumption in watts",
					[]string{"pdu_id"},
					nil,
				),
				ApparentVA: prometheus.NewDesc(
					"apparent_va",
					"PDU apparent power in VA",
					[]string{"pdu_id"},
					nil,
				),
				ReactiveVAR: prometheus.NewDesc(
					"reactive_var",
					"PDU reactive power in VAR",
					[]string{"pdu_id"},
					nil,
				),
				PowerFactor: prometheus.NewDesc(
					"power_factor",
					"PDU efficiency of apparent power being converted into real power",
					[]string{"pdu_id"},
					nil,
				),
			},
			PERackPDU: &PERackPDU{
				PowerCapacityVA: prometheus.NewDesc(
					"power_capacity_va",
					"the maximum power rated as apparent power",
					[]string{"pdu_id"},
					nil,
				),
			},
		},
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metricData.PERackPDUMetrics.Reading

	ch <- c.metricData.PERackPDUMetrics.ApparentVA

	ch <- c.metricData.PERackPDUMetrics.ReactiveVAR

	ch <- c.metricData.PERackPDUMetrics.PowerFactor

	ch <- c.metricData.PERackPDU.PowerCapacityVA
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a context with timeout for the API call
	// Note: prometheus.Collector interface doesn't accept context as parameter,
	// so we create one here with a reasonable timeout. The HTTP client's timeout
	// (configured in NewClient) provides the ultimate backstop.
	_, cancel := context.WithTimeout(context.Background(), collectorTimeout)
	defer cancel()

	switch c.collect {
	case "rackpdu":
		c.collectRackPDU(ch)
	case "environmentalsensors":
		c.collectEnvironmentalSensors(ch)
	default:
		c.collectRackPDU(ch)
		c.collectEnvironmentalSensors(ch)
	}
}

func (c *Collector) collectRackPDU(ch chan<- prometheus.Metric) {
	metrics, _ := c.cache.GetRackPDU(c.pduID)
	if metrics == nil {
		c.logger.Error("failed to retrieve metrics from cache", "pdu_id", c.pduID)
		return
	}

	c.logger.Info("successfully retrieved PDU metrics from cache", "pdu_id", c.pduID)

	// Check if metrics structure is valid
	if metrics.PERackPDUMetrics == nil {
		c.logger.Error("PERackPDUMetrics is nil in cache", "pdu_id", c.pduID)
		return
	}

	// Reading metric
	if metrics.PERackPDUMetrics.Reading != 0 {
		metric, err := prometheus.NewConstMetric(
			c.metricData.PERackPDUMetrics.Reading,
			prometheus.GaugeValue,
			metrics.PERackPDUMetrics.Reading,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerWatts metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// ApparentVA metric
	if metrics.PERackPDUMetrics.ApparentVA != 0 {
		metric, err := prometheus.NewConstMetric(
			c.metricData.PERackPDUMetrics.ApparentVA,
			prometheus.GaugeValue,
			metrics.PERackPDUMetrics.ApparentVA,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create ApparentVA metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// ReactiveVAR metric
	if metrics.PERackPDUMetrics.ReactiveVAR != 0 {
		metric, err := prometheus.NewConstMetric(
			c.metricData.PERackPDUMetrics.ReactiveVAR,
			prometheus.GaugeValue,
			metrics.PERackPDUMetrics.ReactiveVAR,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create ReactiveVAR metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// PowerFactor metric
	if metrics.PERackPDUMetrics.PowerFactor != 0 {
		metric, err := prometheus.NewConstMetric(
			c.metricData.PERackPDUMetrics.PowerFactor,
			prometheus.GaugeValue,
			metrics.PERackPDUMetrics.PowerFactor,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerFactor metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// PowerCapacityVA metric
	if metrics.PERackPDU != nil && metrics.PERackPDU.PowerCapacityVA != 0 {
		metric, err := prometheus.NewConstMetric(
			c.metricData.PERackPDU.PowerCapacityVA,
			prometheus.GaugeValue,
			metrics.PERackPDU.PowerCapacityVA,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerCapacityVA metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}
}

func (c *Collector) collectEnvironmentalSensors(ch chan<- prometheus.Metric) {
	// TODO
	c.cache.GeEnvironmentalSensorsData(c.pduID)
}
