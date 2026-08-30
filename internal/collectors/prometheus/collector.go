// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"time"

	"github.com/SAP/rootstock/internal/redfish"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// PowerReading represents power metrics from Redfish
type PowerReading struct {
	DataSourceUri string  `json:"DataSourceUri"`
	Reading       float64 `json:"Reading"`
	ApparentVA    float64 `json:"ApparentVA"`
	ReactiveVAR   float64 `json:"ReactiveVAR"`
	PowerFactor   float64 `json:"PowerFactor"`
}

// PowerDistributionMetrics represents the Redfish PDU metrics
type PowerDistributionMetrics struct {
	ODataType  string       `json:"@odata.type"`
	ID         string       `json:"Id"`
	Name       string       `json:"Name"`
	PowerWatts PowerReading `json:"PowerWatts"`
}

type Target struct {
	Name     string
	Endpoint string
	Username string
	Password string
}

type Collector struct {
	redfishClient *redfish.Client
}

func NewCollector(redfishClient *redfish.Client) *Collector {
	return &Collector{
		redfishClient: redfishClient,
	}
}

func (c *Collector) Collect(
	ctx context.Context,
	target Target,
	baseURL *url.URL,
	logger *slog.Logger,
	username, password string,
	timeout time.Duration,
) (pmetric.Metrics, error) {
	redfishClient := redfish.NewClient(baseURL, logger, username, password, timeout)

	// Get PDU metrics from Redfish
	data, err := redfishClient.GetPDUMetrics(ctx, "1")
	if err != nil {
		return pmetric.Metrics{}, err
	}

	// Re-marshal to JSON bytes, then unmarshal into struct
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return pmetric.Metrics{}, err
	}

	var pduMetrics PowerDistributionMetrics
	if err := json.Unmarshal(dataBytes, &pduMetrics); err != nil {
		return pmetric.Metrics{}, err
	}

	metrics := pmetric.NewMetrics()

	// resource
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "rootstock")
	rm.Resource().Attributes().PutStr("pdu.name", target.Name)
	rm.Resource().Attributes().PutStr("pdu.id", pduMetrics.ID)

	// scope
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("power")

	// Add power reading metric (Watts)
	powerMetric := sm.Metrics().AppendEmpty()
	powerMetric.SetName("pdu_power_watts")
	powerMetric.SetDescription("PDU power consumption in watts")
	powerMetric.SetUnit("W")

	// Add apparent power metric (VA)
	apparentMetric := sm.Metrics().AppendEmpty()
	apparentMetric.SetName("pdu_apparent_power_va")
	apparentMetric.SetDescription("PDU apparent power in volt-amperes")
	apparentMetric.SetUnit("VA")

	// Add reactive power metric (VAR)
	reactiveMetric := sm.Metrics().AppendEmpty()
	reactiveMetric.SetName("pdu_reactive_power_var")
	reactiveMetric.SetDescription("PDU reactive power in volt-amperes reactive")
	reactiveMetric.SetUnit("VAR")

	// Add power factor metric (ratio)
	pfMetric := sm.Metrics().AppendEmpty()
	pfMetric.SetName("pdu_power_factor")
	pfMetric.SetDescription("PDU power factor (ratio of real power to apparent power)")
	pfMetric.SetUnit("1")

	return metrics, nil
}
