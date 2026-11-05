// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package performance is the shared library for rootstock's performance tests.
// It provides the protocol-neutral PDU abstraction and processing helpers that
// the per-protocol scenario tests (for example the Redfish scenario in
// redfish_processing_test.go) build on.
//
// A Source is a protocol-agnostic PDU source: it enumerates PDU IDs and yields a
// neutral Reading per PDU at the scraping edge. Scrape reads from a Source, the
// processing layer turns a Reading into a cache-ready Result, and ReadResult
// reads that Result back from the result cache. No network, deployed container,
// or Prometheus scrape is involved, so the measurement isolates processing cost.
//
// Because the Source abstraction is protocol-neutral, adding a protocol (for
// example SNMP) means providing a Source implementation; this package does not
// need to change.
//
// The data processors are currently not available, so the processing layer that
// turns a Reading into a Result is not wired up and the scenarios do not run end
// to end.
package performance

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

// Reading is the protocol-neutral representation of a single PDU's measurements
// as produced at the scraping edge. It is the raw input to the processing layer.
type Reading struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Result is the processed, cache-ready representation of a single PDU's
// measurements. It is the shape the data-processor produces and stores in the
// result cache.
//
// Reading and Result carry the same fields today but are distinct pipeline
// stages: a Reading is the raw scraped input, a Result is the processed output.
// They are kept separate so the scraping edge and the data-processor stay
// decoupled.
type Result struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Source is a protocol-agnostic PDU source. Each protocol (Redfish, SNMP)
// provides an implementation; the scenario tests drive them uniformly.
type Source interface {
	// IDs returns the PDU IDs the source can serve.
	IDs() []string
	// Read returns the neutral Reading for a single PDU.
	Read(ctx context.Context, pduID string) (Reading, error)
}

// Scrape reads the neutral Reading for a single PDU from the source. It is the
// scraping edge of the pipeline and is independent of any PDU protocol.
func Scrape(ctx context.Context, source Source, pduID string) (Reading, error) {
	return source.Read(ctx, pduID)
}

// ReadResult reads back a single processed Result from the result cache. It is
// the result-cache read-out used to validate what the pipeline produced.
//
// The result cache here (cache.Cache[Result]) is an in-test stand-in for the
// architecture's in-service result cache component.
func ReadResult(c cache.Cache[Result], pduID string) (Result, error) {
	return c.Get(pduID)
}

// --- Processing layer ---------------------------------------------------------
//
// The processing layer sits between the scraping edge and the result cache:
//
//	scrape (Scrape) -> raw data queue -> data-processor -> cache.Cache[Result]
//
// The raw data queue carries scraped Readings from the scraping edge to the
// data-processor, which transforms them into a cache-ready Result and stores
// them in cache.Cache[Result].
//
// The data processors are not available, so this layer is not wired up and the
// scenarios do not run end to end.
