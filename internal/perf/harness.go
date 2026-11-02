// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package perf contains the performance-testing harness for rootstock.
//
// The harness benchmarks the processing pipeline described in
// docs/testing/performance-testing.md: mocked PDUs are fed in at the scraping
// edge, driven through processing, and the results are observed landing in the
// abstract result cache. No network, deployed container, or Prometheus scrape
// is involved, so the measurement isolates processing cost.
//
// Scope note: the real processing-layer stages (ingestion queue, processor,
// result queue/cache wiring) described in docs/design/architecture.md do not
// exist yet. This package deliberately does NOT implement them. Instead it
// provides the measurement scaffolding and a single extension seam — the
// Processor interface — so that once a real processor is implemented it can be
// dropped into the existing benchmarks without rewriting the harness.
package perf

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/internal/redfish"
)

// Result is the processed, cache-ready representation of a single PDU's
// measurements. It is intentionally a small, transport-agnostic value so that
// the result cache (and the benchmark measuring it) does not depend on the
// Redfish wire types.
type Result struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Processor is the extension seam for the (not-yet-implemented) processing
// layer. An implementation takes the raw PDU documents scraped from a source
// and transforms them into a cache-ready Result.
//
// The real processor, once built, only needs to satisfy this interface to be
// benchmarked by the harness. Until then, DefaultProcessor provides a
// representative transform so the tooling is exercisable end-to-end.
type Processor interface {
	// Process transforms the raw scraped PDU documents into a cache-ready Result.
	Process(pdu *redfish.RackPDU, metrics *redfish.PDUMetrics) Result
}

// ProcessorFunc adapts a plain function to the Processor interface.
type ProcessorFunc func(pdu *redfish.RackPDU, metrics *redfish.PDUMetrics) Result

// Process implements Processor.
func (f ProcessorFunc) Process(pdu *redfish.RackPDU, metrics *redfish.PDUMetrics) Result {
	return f(pdu, metrics)
}

// DefaultProcessor is a straightforward field-mapping processor used by the
// harness until the real processing layer exists. It mirrors the field
// selection the Prometheus collector performs today, giving a representative
// per-PDU processing cost.
func DefaultProcessor() Processor {
	return ProcessorFunc(func(pdu *redfish.RackPDU, metrics *redfish.PDUMetrics) Result {
		r := Result{}

		if metrics != nil {
			r.PDUID = metrics.ID
			r.PowerWatts = metrics.PowerWatts.Reading
			r.ApparentVA = metrics.PowerWatts.ApparentVA
			r.ReactiveVAR = metrics.PowerWatts.ReactiveVAR
			r.PowerFactor = metrics.PowerWatts.PowerFactor
			r.EnergykWh = metrics.EnergykWh.Reading
		}

		if pdu != nil {
			r.PowerCapacityVA = pdu.PowerCapacityVA
		}

		return r
	})
}

// Pipeline wires the three measured stages together: a source (mocked PDU
// redfish.Client), a Processor, and the result cache. Run drives one full pass
// — scrape every PDU, process it, and store the Result in the cache — which is
// the unit a benchmark iterates over.
type Pipeline struct {
	source    redfish.Client
	processor Processor
	cache     cache.Cache[Result]
	pduIDs    []string
}

// NewPipeline assembles a Pipeline. If processor is nil, DefaultProcessor is
// used. If c is nil, a fresh in-memory result cache is created.
func NewPipeline(source redfish.Client, pduIDs []string, processor Processor, c cache.Cache[Result]) *Pipeline {
	if processor == nil {
		processor = DefaultProcessor()
	}

	if c == nil {
		c = cache.New[Result]()
	}

	return &Pipeline{
		source:    source,
		processor: processor,
		cache:     c,
		pduIDs:    pduIDs,
	}
}

// Cache exposes the underlying result cache so a benchmark can read back what
// the pipeline produced (the measurement point at the "abstract result cache").
func (p *Pipeline) Cache() cache.Cache[Result] { return p.cache }

// RunOne scrapes, processes, and caches a single PDU. It returns the first
// error encountered so benchmarks can fail fast on a broken fixture.
func (p *Pipeline) RunOne(ctx context.Context, pduID string) error {
	metrics, err := p.source.GetPERackPDUMetrics(ctx, pduID)
	if err != nil {
		return err
	}

	pdu, err := p.source.GetPERackPDU(ctx, pduID)
	if err != nil {
		return err
	}

	result := p.processor.Process(pdu, metrics)
	p.cache.Set(pduID, result)

	return nil
}

// Run performs one full pass over every configured PDU.
func (p *Pipeline) Run(ctx context.Context) error {
	for _, id := range p.pduIDs {
		if err := p.RunOne(ctx, id); err != nil {
			return err
		}
	}

	return nil
}
