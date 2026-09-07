package prometheus

import (
	"context"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/SAP/rootstock/internal/redfish"
)

type Collector struct {
	mu            sync.Mutex
	redfishClient *redfish.Client
	pduID         string
	logger        *slog.Logger
	pduMetrics    *PDUMetrics
}

type PDUMetrics struct {
	DataSourceUri *prometheus.Desc
	Reading       *prometheus.Desc
	ApparentVA    *prometheus.Desc
	ReactiveVAR   *prometheus.Desc
	PowerFactor   *prometheus.Desc
}

func NewCollector(redfishClient *redfish.Client, pduID string, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}

	return &Collector{
		redfishClient: redfishClient,
		pduID:         pduID,
		logger:        logger,
		pduMetrics: &PDUMetrics{
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
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.pduMetrics.Reading
	ch <- c.pduMetrics.ApparentVA
	ch <- c.pduMetrics.ReactiveVAR
	ch <- c.pduMetrics.PowerFactor
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := context.Background()

	// Fetch PDU metrics from Redfish API
	metrics, err := c.redfishClient.GetPDUMetrics(ctx, c.pduID)
	if err != nil {
		c.logger.Error("failed to fetch PDU metrics", "error", err, "pdu_id", c.pduID)
		return
	}

	c.logger.Info("successfully scraped PDU metrics", "pdu_id", c.pduID)

	// Extract metrics from the response
	// PowerWatts metric
	if metrics.PowerWatts.Reading != 0 {
		ch <- prometheus.MustNewConstMetric(
			c.pduMetrics.Reading,
			prometheus.GaugeValue,
			metrics.PowerWatts.Reading,
			c.pduID,
		)
	}

	// ApparentVA metric
	if metrics.PowerWatts.ApparentVA != 0 {
		ch <- prometheus.MustNewConstMetric(
			c.pduMetrics.ApparentVA,
			prometheus.GaugeValue,
			metrics.PowerWatts.ApparentVA,
			c.pduID,
		)
	}

	// ReactiveVAR metric
	if metrics.PowerWatts.ReactiveVAR != 0 {
		ch <- prometheus.MustNewConstMetric(
			c.pduMetrics.ReactiveVAR,
			prometheus.GaugeValue,
			metrics.PowerWatts.ReactiveVAR,
			c.pduID,
		)
	}

	// PowerFactor metric
	if metrics.PowerWatts.PowerFactor != 0 {
		ch <- prometheus.MustNewConstMetric(
			c.pduMetrics.PowerFactor,
			prometheus.GaugeValue,
			metrics.PowerWatts.PowerFactor,
			c.pduID,
		)
	}
}
