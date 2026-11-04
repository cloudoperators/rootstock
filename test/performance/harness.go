// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package performance contains the performance-testing harness for rootstock.
//
// The harness exercises the processing pipeline for a PDU: mocked PDUs are fed
// in at the scraping edge, driven through the processing layer (raw data queue
// -> data-processor), and the results are read back from the result cache. No
// network, deployed container, or Prometheus scrape is involved, so the
// measurement isolates processing cost.
//
// The Redfish scenario is currently not completely implemented: the data
// processors (the raw data queue and data-processor of the processing layer) are
// not available, so the scrape -> queue -> processor -> cache pass cannot run
// end to end. The scraping edge (Scrape) and the result-cache read-out
// (ReadResult) are exercised directly.
package performance

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/internal/redfish"
)

// Result is the processed, cache-ready representation of a single PDU's
// measurements. It is a small, transport-agnostic value so that the result
// cache does not depend on the Redfish wire types. It is the shape the
// data-processor produces and stores in the result cache.
type Result struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Scrape reads the raw Redfish documents for a single PDU from the source. It is
// the scraping edge of the pipeline, using the redfish.Client interface to fetch
// the PDU's metrics and resource documents.
func Scrape(
	ctx context.Context, source redfish.Client, pduID string,
) (pdu *redfish.RackPDU, metrics *redfish.PDUMetrics, err error) {
	metrics, err = source.GetPERackPDUMetrics(ctx, pduID)
	if err != nil {
		return nil, nil, err
	}

	pdu, err = source.GetPERackPDU(ctx, pduID)
	if err != nil {
		return nil, nil, err
	}

	return pdu, metrics, nil
}

// ReadResult reads back a single processed Result from the result cache. It is
// the result-cache read-out used to validate what the pipeline produced.
func ReadResult(c cache.Cache[Result], pduID string) (Result, error) {
	return c.Get(pduID)
}

// --- Processing layer ---------------------------------------------------------
//
// The processing layer sits between the scraping edge and the result cache:
//
//	scrape (Scrape) -> raw data queue -> data-processor -> cache.Cache[Result]
//
// The raw data queue carries scraped Redfish documents from the scraping edge to
// the data-processor, which transforms them into a cache-ready Result and stores
// them in cache.Cache[Result].
//
// The data processors are not available, so this layer is not wired up and the
// Redfish scenario does not run end to end.
