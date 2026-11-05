// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package performance is the shared library for rootstock's performance tests.
// It provides the protocol-neutral PDU abstraction and processing helpers that
// the per-protocol scenario tests (for example the Redfish scenario in
// redfish_processing_test.go) build on.
//
// A Source is a protocol-agnostic PDU source: it enumerates PDU IDs and yields a
// neutral Reading per PDU at the scraping edge. The suite owns two ends of the
// pipeline: Scrape reads a Reading from a Source and enqueues it onto the raw
// data queue (producer), and ReadResult reads a Result back from the result
// cache (consumer). The autonomous data-processors entity in between turns
// Readings into Results; the suite does not drive it. No network, deployed
// container, or Prometheus scrape is involved, so the measurement isolates
// processing cost.
//
// Because the Source abstraction is protocol-neutral, adding a protocol (for
// example SNMP) means providing a Source implementation; this package does not
// need to change.
//
// The data-processors are currently not available, so nothing drains the raw
// data queue into the result cache and the scenarios do not run end to end.
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
// measurements. It is the shape the data-processors produce and store in the
// result cache.
//
// Reading and Result carry the same fields today but are distinct pipeline
// stages: a Reading is the raw scraped input, a Result is the processed output.
// They are kept separate so the scraping edge and the data-processors stay
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
// the suite's consumer edge, used to validate what the data-processors produced.
//
// The result cache here (cache.Cache[Result]) is an in-test stand-in for the
// architecture's in-service result cache component.
func ReadResult(c cache.Cache[Result], pduID string) (Result, error) {
	return c.Get(pduID)
}

// --- Processing layer ---------------------------------------------------------
//
// The suite owns only the two ends of the pipeline:
//
//	test:            Scrape -> enqueue onto the raw data queue        [producer edge]
//	data-processors: raw data queue -> data-processors -> result cache    [autonomous]
//	test:            ReadResult(result cache)                         [consumer edge]
//
// The data-processors are an autonomous entity: on their own they pull Readings
// from the raw data queue, transform each into a cache-ready Result, and store
// it in the result cache. The suite does not run them, does not transform
// Readings, and does not write the result cache; it only enqueues (producer) and
// reads back (consumer).
//
// The data-processors are not available, so nothing drains the raw data queue
// into the result cache and the scenarios do not run end to end.
