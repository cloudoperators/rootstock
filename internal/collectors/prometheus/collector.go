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
	rackPDUsBasePath = "/redfish/v1/PowerEquipment/RackPDUs/"
)

// redfishSensorReading is the subset of a Redfish SensorReading
// (PowerWatts, EnergykWh) that this collector emits.
type redfishSensorReading struct {
	Reading    float64 `json:"Reading"`
	ApparentVA float64 `json:"ApparentVA"`
}

// pduMetricsDocument is the subset of a RackPDU /Metrics document we emit.
type pduMetricsDocument struct {
	PowerWatts redfishSensorReading `json:"PowerWatts"`
	EnergykWh  redfishSensorReading `json:"EnergykWh"`
}

// pduDocument is the subset of a RackPDU document we emit.
type pduDocument struct {
	PowerCapacityVA float64 `json:"PowerCapacityVA"`
}

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
	// power holds the active power consumption in watts.
	power *prometheus.Desc
	// apparentPower holds the apparent power in volt-amperes.
	apparentPower *prometheus.Desc
	// energy holds the measured energy throughput in kWh.
	energy *prometheus.Desc
}

type powerRating struct {
	// powerCapacity holds the rated power capacity in volt-amperes.
	powerCapacity *prometheus.Desc
}

// type outletElectricals struct {
// 	name           *prometheus.Desc
// 	state          *prometheus.Desc
// 	odatautletType *prometheus.Desc
// 	ratedCurrent   *prometheus.Desc
// 	voltage        *prometheus.Desc
// 	current        *prometheus.Desc
// 	apparentPower  *prometheus.Desc
// }

// type mainCurrent struct {
// 	name          *prometheus.Desc
// 	voltage       *prometheus.Desc
// 	power         *prometheus.Desc
// 	apparentPower *prometheus.Desc
// }

// type branchCurrent struct {
// 	ratedCurrent *prometheus.Desc
// 	current      *prometheus.Desc
// }

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
				energy: prometheus.NewDesc(
					"energy_kwh",
					"PDU energy kWh",
					[]string{"pdu_id"},
					nil,
				),
			},
			powerRating: powerRating{
				powerCapacity: prometheus.NewDesc(
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
	ch <- c.rackPDU.energetics.energy
	// Power rating
	ch <- c.rackPDU.powerRating.powerCapacity
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// prometheus.Collector.Collect has no context parameter, so we bound the
	// Redfish calls here. The HTTP client timeout (set in NewClient) is the
	// ultimate backstop.
	ctx, cancel := context.WithTimeout(context.Background(), collectorTimeout)
	defer cancel()

	if err := c.redfishClient.Login(ctx); err != nil {
		c.logger.Error("failed to log in to PDU", log.KeyError, err, "pdu_id", c.pduID)
		return
	}
	defer func() {
		if err := c.redfishClient.Close(ctx); err != nil {
			c.logger.Warn("failed to close redfish session", "pdu_id", c.pduID, log.KeyError, err)
		}
	}()

	var pduMetrics pduMetricsDocument
	if err := c.redfishClient.Get(ctx, rackPDUsBasePath+c.pduID+"/Metrics", &pduMetrics); err != nil {
		c.logger.Error("failed to fetch PDU metrics", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	c.logger.Info("successfully scraped PDU metrics", "pdu_id", c.pduID)

	c.emit(ch, c.rackPDU.energetics.power, pduMetrics.PowerWatts.Reading)
	c.emit(ch, c.rackPDU.energetics.apparentPower, pduMetrics.PowerWatts.ApparentVA)
	c.emit(ch, c.rackPDU.energetics.energy, pduMetrics.EnergykWh.Reading)

	var pdu pduDocument
	if err := c.redfishClient.Get(ctx, rackPDUsBasePath+c.pduID, &pdu); err != nil {
		c.logger.Error("failed to fetch PDU power rating", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	c.emit(ch, c.rackPDU.powerRating.powerCapacity, pdu.PowerCapacityVA)
}

// emit sends one gauge sample, skipping zero values (treated as absent).
func (c *Collector) emit(ch chan<- prometheus.Metric, desc *prometheus.Desc, value float64) {
	if value == 0 {
		return
	}

	metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, c.pduID)
	if err != nil {
		c.logger.Error("failed to create metric", log.KeyError, err, "pdu_id", c.pduID)
		return
	}

	ch <- metric
}
