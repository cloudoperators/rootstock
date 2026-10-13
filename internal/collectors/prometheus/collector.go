// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
	logger        log.MinInterface
	rackPDU       rackPDU
}

type rackPDU struct {
	energetics  energetics
	powerRating powerRating
}

type energetics struct {
	power         *prometheus.Desc
	apparentPower *prometheus.Desc
	energykWk     *prometheus.Desc
}

type powerRating struct {
	powerRating *prometheus.Desc
}

type outletElectricals struct {
	name           *prometheus.Desc
	state          *prometheus.Desc
	odatautletType *prometheus.Desc
	ratedCurrent   *prometheus.Desc
	voltage        *prometheus.Desc
	current        *prometheus.Desc
	apparentPower  *prometheus.Desc
}

type mainCurrent struct {
	name          *prometheus.Desc
	voltage       *prometheus.Desc
	power         *prometheus.Desc
	apparentPower *prometheus.Desc
}

type branchCurrent struct {
	ratedCurrent *prometheus.Desc
	current      *prometheus.Desc
}

func NewCollector(redfishClient redfish.Client, pduID string) *Collector {
	logger := log.Default()

	return &Collector{
		redfishClient: redfishClient,
		pduID:         pduID,
		logger:        logger,
		rackPDU: rackPDU{
			energetics: energetics{
				power: prometheus.NewDesc(
					"pdu_power_watts",
					"PDU power consumption in watts",
					[]string{"pdu_id"},
					nil,
				),
				apparentPower: prometheus.NewDesc(
					"apparent_power",
					"PDU apparent power in VA",
					[]string{"pdu_id"},
					nil,
				),
				energykWk: prometheus.NewDesc(
					"energy_kwh",
					"PDU energy kWh",
					[]string{"pdu_id"},
					nil,
				),
			},
			powerRating: powerRating{
				powerRating: prometheus.NewDesc(
					"power_rating",
					"PDU power rating",
					[]string{"pdu_id"},
					nil,
				),
			},
		},
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	// Energetics
	ch <- c.rackPDU.energetics.power
	ch <- c.rackPDU.energetics.apparentPower
	ch <- c.rackPDU.energetics.energykWk
	// Power rating
	ch <- c.rackPDU.powerRating.powerRating
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a context with timeout for the API call
	// Note: prometheus.Collector interface doesn't accept context as parameter,
	// so we create one here with a reasonable timeout. The HTTP client's timeout
	// (configured in NewClient) provides the ultimate backstop.
	ctx, cancel := context.WithTimeout(context.Background(), collectorTimeout)
	defer cancel()

	// Fetch PDU metrics from Redfish API
	rackPDUEnergetics, err := c.redfishClient.GetRackPDUEnergetics(ctx, c.pduID)
	if err != nil {
		c.logger.Error("failed to fetch PDU metrics", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	c.logger.Info("successfully scraped PDU metrics", "pdu_id", c.pduID)

	// Extract metrics from the response
	// PowerWatts metric
	if rackPDUEnergetics.PowerWatts.Power != 0 {
		metric, err := prometheus.NewConstMetric(
			c.rackPDU.energetics.power,
			prometheus.GaugeValue,
			rackPDUEnergetics.PowerWatts.Power,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create PowerWatts metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// ApparentPower metric
	if rackPDUEnergetics.PowerWatts.ApparentPower != 0 {
		metric, err := prometheus.NewConstMetric(
			c.rackPDU.energetics.apparentPower,
			prometheus.GaugeValue,
			rackPDUEnergetics.PowerWatts.ApparentPower,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create ApparentVA metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	// EnergykWh metric
	if rackPDUEnergetics.EnergykWh.EnergykWh != 0 {
		metric, err := prometheus.NewConstMetric(
			c.rackPDU.energetics.energykWk,
			prometheus.GaugeValue,
			rackPDUEnergetics.EnergykWh.EnergykWh,
			c.pduID,
		)
		if err != nil {
			c.logger.Error("failed to create EnergykWh metric", log.KeyError, err, "pdu_id", c.pduID)
		} else {
			ch <- metric
		}
	}

	rackPDU, err := c.redfishClient.GetRackPDUPowerRating(ctx, c.pduID)
	if err != nil {
		c.logger.Error("failed to fetch PDU metrics", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	if rackPDU.PowerCapacityVA == 0 {
		return
	}

	metric, err := prometheus.NewConstMetric(
		c.rackPDU.powerRating.powerRating,
		prometheus.GaugeValue,
		rackPDU.PowerCapacityVA,
		c.pduID,
	)
	if err != nil {
		c.logger.Error("failed to create PowerCapacityVA metric", log.KeyError, err, "pdu_id", c.pduID)
	} else {
		ch <- metric
	}
}
