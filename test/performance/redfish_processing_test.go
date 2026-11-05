// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build performance

package performance_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/test/performance"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// TestRedfishPipeline runs the Redfish test case. The suite owns the two ends of
// the pipeline: it scrapes the mocked PDUs and enqueues the readings onto the raw
// data queue (producer), then reads the result cache back to validate (consumer).
// The data-processors in between are an autonomous entity, not driven by the test.
//
// Component chain:
//
//	testdata/pdu-mockup Redfish fixture
//	  -> mockpdu.NewRedfish() -> performance.Source      [mock PDUs]
//	  -> performance.Scrape (reads a Reading)            [scraping edge, test]
//	  -> enqueue onto raw data queue                     [producer edge, test]
//	        --- boundary: the test stops here ---
//	  -> raw data queue -> data-processors -> result cache   [autonomous, not the test]
//	  -> performance.ReadResult(result cache)            [consumer edge, test]
//
// The result cache is an in-test stand-in for the architecture's in-service
// result cache component.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so nothing drains the raw data queue
// into the result cache. The test scrapes the mocked PDUs, confirms the result
// cache is empty, and reports the missing data-processors via t.Fatal.
func TestRedfishPipeline(t *testing.T) {
	ctx := context.Background()

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

	for _, id := range ids {
		// Scraping edge.
		reading, err := performance.Scrape(ctx, source, id)
		if err != nil {
			t.Fatalf("scrape %q: %v", id, err)
		}

		// Producer edge: enqueue the reading onto the raw data queue. From here
		// the autonomous data-processors entity would drain the queue, transform
		// each Reading into a performance.Result, and store it in the result
		// cache; the test does not do that work. The data-processors are not
		// available.
		_ = reading
	}

	// Consumer edge: read the result cache back to validate what the
	// data-processors produced. Each Result is read back and checked against the
	// reading values, e.g.
	//
	//	got, err := performance.ReadResult(resultCache, id)
	//	require.NoError(t, err)
	//	require.Equal(t, reading.PDUID, got.PDUID)
	//	require.Equal(t, reading.PowerWatts, got.PowerWatts)
	//	require.Equal(t, reading.PowerCapacityVA, got.PowerCapacityVA)
	//	... (ApparentVA, ReactiveVAR, PowerFactor, EnergykWh)
	//
	// The result cache is empty, because no data-processors drained the queue.
	if _, err := performance.ReadResult(resultCache, ids[0]); !errors.Is(err, cache.ErrCacheNotFound) {
		t.Fatalf("expected empty result cache (cache miss), got err=%v", err)
	}

	t.Fatal("data processors not available: the Redfish scenario does not run end to end")
}

// BenchmarkPipeline measures the suite's producer edge over the representative
// Redfish PDU fixture: scrape (from the mock PDU) and enqueue the reading onto
// the raw data queue. The data-processors entity (raw data queue ->
// data-processors -> result cache) is autonomous and not driven by the benchmark.
//
// The Redfish scenario is currently not completely implemented: the
// data-processors entity is not available, so nothing drains the raw data queue
// into the result cache. The benchmark scrapes the mocked PDUs, then reports the
// missing data-processors via b.Fatal.
func BenchmarkPipeline(b *testing.B) {
	ctx := context.Background()

	source, err := mockpdu.NewRedfish()
	if err != nil {
		b.Fatalf("build mock PDU source: %v", err)
	}

	ids := source.IDs()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for _, id := range ids {
			// Scraping edge.
			if _, err := performance.Scrape(ctx, source, id); err != nil {
				b.Fatalf("scrape: %v", err)
			}

			// Producer edge: enqueue the scraped reading onto the raw data
			// queue. The autonomous data-processors entity (queue ->
			// data-processors -> result cache) is not available.
		}
	}

	b.Fatal("data processors not available: the Redfish scenario does not run end to end")
}

// BenchmarkPipelineScale measures how the suite's producer edge scales with the
// number of PDUs (mass PDU scanning / horizontal scaling). Each sub-benchmark
// scrapes N synthesized PDUs and enqueues their readings onto the raw data queue.
func BenchmarkPipelineScale(b *testing.B) {
	ctx := context.Background()

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
				for _, id := range ids {
					// Scraping edge.
					if _, err := performance.Scrape(ctx, source, id); err != nil {
						b.Fatalf("scrape: %v", err)
					}

					// Producer edge: enqueue the reading onto the raw data
					// queue, as in BenchmarkPipeline. The autonomous
					// data-processors entity is not available.
				}
			}

			b.Fatal("data processors not available: the Redfish scenario does not run end to end")
		})
	}
}

// BenchmarkPipelineParallel measures concurrent scraping and enqueuing of
// distinct PDUs onto a shared raw data queue (the suite's producer edge).
func BenchmarkPipelineParallel(b *testing.B) {
	ctx := context.Background()

	const pduCount = 100

	source, err := mockpdu.NewRedfishScaled(pduCount)
	if err != nil {
		b.Fatalf("build scaled mock PDU source: %v", err)
	}

	ids := source.IDs()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			id := ids[i%len(ids)]

			// Scraping edge.
			if _, err := performance.Scrape(ctx, source, id); err != nil {
				b.Errorf("scrape: %v", err)

				return
			}

			// Producer edge: enqueue the reading onto the raw data queue, as in
			// BenchmarkPipeline. The autonomous data-processors entity is not
			// available.

			i++
		}
	})

	b.Fatal("data processors not available: the Redfish scenario does not run end to end")
}
