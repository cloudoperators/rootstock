// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/SAP/rootstock/internal/redfish"
	"github.com/SAP/rootstock/pkg/log"
)

const (
	CollectorTimeout = 25 * time.Second
)

type Collector struct {
	mu            sync.Mutex
	redfishClient redfish.Client
	pduID         string
	logger        log.MinInterface

	peRackPDUMetrics *PERackPDUMetrics
	peRackPDU        *PERackPDU
}

type PERackPDUMetrics struct {
	DataSourceUri *prometheus.Desc
	Reading       *prometheus.Desc
	ApparentVA    *prometheus.Desc
	ReactiveVAR   *prometheus.Desc
	PowerFactor   *prometheus.Desc
}

type PERackPDU struct {
	PowerCapacityVA *prometheus.Desc
}

func NewCollector(redfishClient redfish.Client, pduID string) *Collector {
	logger := log.Default()

	return &Collector{
		redfishClient: redfishClient,
		pduID:         pduID,
		logger:        logger,
		peRackPDUMetrics: &PERackPDUMetrics{
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
		peRackPDU: &PERackPDU{
			PowerCapacityVA: prometheus.NewDesc(
				"power_capacity_va",
				"the maximum power rated as aparent power",
				[]string{"pdu_id"},
				nil,
			),
		},
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.peRackPDUMetrics.Reading
	ch <- c.peRackPDUMetrics.ApparentVA
	ch <- c.peRackPDUMetrics.ReactiveVAR
	ch <- c.peRackPDUMetrics.PowerFactor
	ch <- c.peRackPDU.PowerCapacityVA
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a context with timeout for the API call
	// Note: prometheus.Collector interface doesn't accept context as parameter,
	// so we create one here with a reasonable timeout. The HTTP client's timeout
	// (configured in NewClient) provides the ultimate backstop.
	ctx, cancel := context.WithTimeout(context.Background(), CollectorTimeout)
	defer cancel()

	// Fetch PDU metrics from Redfish API
	pduPERackMetrics, err := c.redfishClient.GetPERackPDUMetrics(ctx, c.pduID)
	if err != nil {
		c.logger.Error("failed to fetch PDU metrics", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	c.logger.Info("successfully scraped PDU metrics", "pdu_id", c.pduID)

	// Extract metrics from the response
	// PowerWatts metric
	if pduPERackMetrics.PowerWatts.Reading != 0 {
		metric, err := prometheus.NewConstMetric(
			c.peRackPDUMetrics.Reading,
			prometheus.GaugeValue,
			pduPERackMetrics.PowerWatts.Reading,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerWatts metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// ApparentVA metric
	if pduPERackMetrics.PowerWatts.ApparentVA != 0 {
		metric, err := prometheus.NewConstMetric(
			c.peRackPDUMetrics.ApparentVA,
			prometheus.GaugeValue,
			pduPERackMetrics.PowerWatts.ApparentVA,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create ApparentVA metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// ReactiveVAR metric
	if pduPERackMetrics.PowerWatts.ReactiveVAR != 0 {
		metric, err := prometheus.NewConstMetric(
			c.peRackPDUMetrics.ReactiveVAR,
			prometheus.GaugeValue,
			pduPERackMetrics.PowerWatts.ReactiveVAR,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create ReactiveVAR metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// PowerFactor metric
	if pduPERackMetrics.PowerWatts.PowerFactor != 0 {
		metric, err := prometheus.NewConstMetric(
			c.peRackPDUMetrics.PowerFactor,
			prometheus.GaugeValue,
			pduPERackMetrics.PowerWatts.PowerFactor,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerFactor metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	peRackPDU, err := c.redfishClient.GetPERackPDU(ctx, c.pduID)
	if err != nil {
		c.logger.Error("failed to fetch PDU metrics", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	if peRackPDU.PowerCapacityVA != 0 {
		metric, err := prometheus.NewConstMetric(
			c.peRackPDU.PowerCapacityVA,
			prometheus.GaugeValue,
			peRackPDU.PowerCapacityVA,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerCapacityVA metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}

	}
}
