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

// TestRedfishPipeline runs the end-to-end Redfish test case: it scrapes the
// mocked PDUs, runs them through the processing layer, and validates the results
// by reading them back from the result cache.
//
// Component chain:
//
//	testdata/pdu-mockup Redfish fixture
//	  -> mockpdu.NewRedfish() -> performance.Source      [mock PDUs]
//	  -> performance.Scrape (reads a Reading)            [scraping edge]
//	  -> raw data queue                                  [processing layer]
//	  -> data-processor                                  [processing layer]
//	  -> cache.Cache[Result] (result cache)              [read-out]
//
// The result cache is an in-test stand-in for the architecture's in-service
// result cache component.
//
// The Redfish scenario is currently not completely implemented: the data
// processors are not available, so the test scrapes the mocked PDUs, confirms
// the result cache is empty, and reports the missing processing layer via
// t.Fatal.
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

		// Processing layer: push the reading onto the raw data queue, have the
		// data-processor transform it into a performance.Result and store it
		// with resultCache.Set(id, result). The data processors are not
		// available.
		_ = reading
	}

	// Validate by reading out the result cache. Each Result is read back and
	// checked against the reading values, e.g.
	//
	//	got, err := performance.ReadResult(resultCache, id)
	//	require.NoError(t, err)
	//	require.Equal(t, reading.PDUID, got.PDUID)
	//	require.Equal(t, reading.PowerWatts, got.PowerWatts)
	//	require.Equal(t, reading.PowerCapacityVA, got.PowerCapacityVA)
	//	... (ApparentVA, ReactiveVAR, PowerFactor, EnergykWh)
	//
	// The result cache is empty, so the read-out returns a cache miss.
	if _, err := performance.ReadResult(resultCache, ids[0]); !errors.Is(err, cache.ErrCacheNotFound) {
		t.Fatalf("expected empty result cache (cache miss), got err=%v", err)
	}

	t.Fatal("data processors not available: the Redfish scenario does not run end to end")
}

// BenchmarkPipeline measures a full processing pass over the representative
// Redfish PDU fixture: scrape (from the mock PDU) -> raw data queue ->
// data-processor -> store in the result cache.
//
// The Redfish scenario is currently not completely implemented: the data
// processors are not available, so the full scrape -> queue -> processor ->
// cache pass cannot run. The benchmark scrapes the mocked PDUs, then reports the
// missing processing layer via b.Fatal.
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

			// Processing layer: push the scraped docs onto the raw data queue,
			// let the data-processor transform them and store a
			// performance.Result in the result cache, then read it back with
			// performance.ReadResult. The data processors are not available.
		}
	}

	b.Fatal("data processors not available: the Redfish scenario does not run end to end")
}

// BenchmarkPipelineScale measures how a full processing pass scales with the
// number of PDUs (mass PDU scanning / horizontal scaling). Each sub-benchmark
// drives N synthesized PDUs through the pipeline into the result cache.
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

					// Processing layer: queue -> processor -> cache, as in
					// BenchmarkPipeline. The data processors are not available.
				}
			}

			b.Fatal("data processors not available: the Redfish scenario does not run end to end")
		})
	}
}

// BenchmarkPipelineParallel measures concurrent processing of distinct PDUs
// through a shared pipeline and result cache.
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

			// Processing layer: queue -> processor -> cache, as in
			// BenchmarkPipeline. The data processors are not available.

			i++
		}
	})

	b.Fatal("data processors not available: the Redfish scenario does not run end to end")
}
