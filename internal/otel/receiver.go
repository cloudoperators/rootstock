// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/SAP/rootstock/internal/redfish"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Use case: use the client to fetch data, then turn it into Prometheus metrics.

// Possible functionalities:
// It uses client.go to get JSON from the PDU,
// then reads the values out of that JSON (watts, amps, volts, energy in kWh) and records them as Prometheus metrics.

type Receiver struct {
	redfishClient *redfish.Client

	cancel context.CancelFunc
	wg     sync.WaitGroup
	next   consumer.Metrics
}

func NewReceiver(baseURL *url.URL, logger *slog.Logger, username string, password string, timeout time.Duration) *Receiver {
	return &Receiver{
		redfishClient: redfish.NewClient(baseURL, logger, username, password, timeout),
	}
}

func (r *Receiver) Collect(ctx context.Context) {
	if err := r.redfishClient.Login(ctx); err != nil {
		return
	}
	defer r.redfishClient.Close(ctx)

	ticker := time.NewTicker(10000 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			metrics := pmetric.NewMetrics()

			//getting the value from redfish
			var powerEquipment map[string]interface{}
			if err := r.redfishClient.Get(ctx, "/redfish/v1/PowerEquipment", &powerEquipment); err != nil {
				// Handle error - log and continue
				continue
			}

			rm := metrics.ResourceMetrics().AppendEmpty()
			sm := rm.ScopeMetrics().AppendEmpty()

			metric := sm.Metrics().AppendEmpty()
			metric.SetName("metric_name")

			r.next.ConsumeMetrics(ctx, metrics)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Receiver) Start(
	ctx context.Context,
	host component.Host,
) error {

	ctx, r.cancel = context.WithCancel(ctx)

	r.wg.Add(1)

	go func() {
		defer r.wg.Done()
		r.Collect(ctx)
	}()

	return nil
}

func (r *Receiver) Shutdown(
	ctx context.Context,
) error {

	if r.cancel != nil {
		r.cancel()
	}

	done := make(chan struct{})

	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}
