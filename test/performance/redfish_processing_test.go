// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build performance

package performance_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/test/performance"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// resultTimeout bounds how long the suite waits for the data-processors to
// populate the result cache before failing. While the data-processors are not
// available this always elapses, so the test and benchmarks fail by timeout.
const resultTimeout = 5 * time.Second

// runCycle runs one submit-to-collect cycle for the given PDU IDs: it scrapes
// and enqueues each PDU (producer edge), then awaits the Results for all of them
// (consumer edge), returning once every submitted ID has a Result. It fails via
// tb if scraping errors or the results are not collected within resultTimeout.
//
// This is the unit of work the suite measures: a PDU scraped, submitted for
// processing, processed, and its result received. While the data-processors are
// not available the await never resolves and runCycle fails by timeout.
func runCycle(tb testing.TB, source performance.Source, resultCache cache.Cache[performance.Result], ids []string) map[string]performance.Result {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), resultTimeout)
	defer cancel()

	submitted := make([]string, 0, len(ids))

	for _, id := range ids {
		// Scraping edge.
		reading, err := performance.Scrape(ctx, source, id)
		if err != nil {
			tb.Fatalf("scrape %q: %v", id, err)
		}

		// Producer edge: enqueue the reading onto the raw data queue and remember
		// it. From here the autonomous data-processors entity would drain the
		// queue, transform each Reading into a performance.Result, and store it in
		// the result cache; the suite does not do that work. The data-processors
		// are not available.
		_ = reading

		submitted = append(submitted, id)
	}

	// Consumer edge: await the promise that resolves once every submitted ID has
	// a Result in the result cache.
	select {
	case results := <-performance.AwaitResults(ctx, resultCache, submitted):
		return results
	case <-ctx.Done():
		tb.Fatalf("timed out after %s waiting for results: the data-processors are "+
			"not implemented, so the raw data queue is never drained into the result "+
			"cache (%d/%d PDUs still unprocessed)", resultTimeout, len(submitted), len(submitted))

		return nil
	}
}

// TestRedfishPipeline runs the Redfish test case. The suite owns the two ends of
// the pipeline: it scrapes the mocked PDUs and enqueues the readings onto the raw
// data queue (producer), then awaits the results from the result cache (consumer).
// The data-processors in between are an autonomous entity, not driven by the test.
//
// Completion is promise-based: the test remembers the IDs it submitted and awaits
// performance.AwaitResults, which resolves only once every submitted ID has a
// Result in the result cache. The test passes when the promise resolves and the
// Results validate; it fails if the promise does not resolve within resultTimeout.
//
// Component chain:
//
//	testdata/pdu-mockup Redfish fixture
//	  -> mockpdu.NewRedfish() -> performance.Source      [mock PDUs]
//	  -> performance.Scrape (reads a Reading)            [scraping edge, test]
//	  -> enqueue onto raw data queue                     [producer edge, test]
//	        --- boundary: the test stops here ---
//	  -> raw data queue -> data-processors -> result cache   [autonomous, not the test]
//	  -> performance.AwaitResults(result cache)          [consumer edge, test]
//
// The result cache is an in-test stand-in for the architecture's in-service
// result cache component.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so nothing drains the raw data queue
// into the result cache. The promise therefore never resolves and the test fails
// when it times out, reporting that the data-processors are not implemented.
func TestRedfishPipeline(t *testing.T) {
	// Setup: initialize the mock PDU source and connect it to the scraper.
	source, err := mockpdu.NewRedfish()
	if err != nil {
		t.Fatalf("build mock PDU source: %v", err)
	}

	ids := source.IDs()
	if len(ids) == 0 {
		t.Fatal("mock PDU source exposed no PDUs")
	}

	// Result cache: the read-out layer.
	resultCache := cache.New[performance.Result]()

	// Run one submit-to-collect cycle over every PDU. On resolution, validate
	// each Result against its scraped reading, e.g.
	//
	//	require.Equal(t, reading.PDUID, results[id].PDUID)
	//	require.Equal(t, reading.PowerWatts, results[id].PowerWatts)
	//	... (ApparentVA, ReactiveVAR, PowerFactor, EnergykWh, PowerCapacityVA)
	//
	// While the data-processors are not available this times out inside runCycle.
	results := runCycle(t, source, resultCache, ids)
	if len(results) != len(ids) {
		t.Fatalf("expected %d results, got %d", len(ids), len(results))
	}
	// TODO: validate each Result against its scraped Reading once the
	// data-processors exist.
}

// BenchmarkPipeline measures the unit of work for a single representative Redfish
// PDU: one PDU scraped, submitted for processing, processed, and its result
// received. Each b.N iteration runs one full submit-to-collect cycle over that
// one PDU.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so the cycle never collects a result
// and each iteration fails by timeout (reported via b.Fatal inside runCycle). No
// ns/op is produced until the data-processors exist.
func BenchmarkPipeline(b *testing.B) {
	source, err := mockpdu.NewRedfish()
	if err != nil {
		b.Fatalf("build mock PDU source: %v", err)
	}

	ids := source.IDs()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		// Unit of work: one full cycle over the PDU set (a single PDU here).
		resultCache := cache.New[performance.Result]()
		runCycle(b, source, resultCache, ids)
	}
}

// BenchmarkPipelineScale measures the same per-PDU unit of work while N distinct
// PDUs are in flight (mass PDU scanning / horizontal scaling), to see how the
// per-PDU cost behaves as the system scales. Each b.N iteration runs one full
// submit-to-collect cycle over all N PDUs; the result is normalized and reported
// as ns/pdu via b.ReportMetric, so pdus=1..1000 compare on one per-PDU axis.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so the cycle never collects results
// and each sub-benchmark fails by timeout (reported via b.Fatal inside runCycle).
func BenchmarkPipelineScale(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run("pdus="+strconv.Itoa(n), func(b *testing.B) {
			source, err := mockpdu.NewRedfishScaled(n)
			if err != nil {
				b.Fatalf("build scaled mock PDU source: %v", err)
			}

			ids := source.IDs()

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				// Unit of work: one full cycle over the N-PDU batch.
				resultCache := cache.New[performance.Result]()
				runCycle(b, source, resultCache, ids)
			}

			b.StopTimer()

			// Normalize to per-PDU so sub-benchmarks are comparable across N.
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/pdu")
		})
	}
}

// BenchmarkPipelineParallel measures the same per-PDU unit of work under
// concurrency: each parallel iteration runs one full submit-to-collect cycle for
// a single PDU against a shared result cache, exercising the queue and cache
// under concurrent load.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so the cycle never collects a result
// and the benchmark fails by timeout (reported via b.Fatal inside runCycle).
func BenchmarkPipelineParallel(b *testing.B) {
	const pduCount = 100

	source, err := mockpdu.NewRedfishScaled(pduCount)
	if err != nil {
		b.Fatalf("build scaled mock PDU source: %v", err)
	}

	ids := source.IDs()

	resultCache := cache.New[performance.Result]()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			id := ids[i%len(ids)]

			// Unit of work: one full cycle for a single PDU.
			runCycle(b, source, resultCache, []string{id})

			i++
		}
	})
}
