// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package performance_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cloudoperators/rootstock/test/performance"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// These benchmarks measure the "mock PDU -> result cache" pass for the Redfish
// test case: scrape each mocked PDU, run it through the processing layer, and
// store the result in the result cache.
//
// The Redfish scenario is currently not completely implemented: the data
// processors are not available, so the full scrape -> queue -> processor ->
// cache pass cannot run. Each benchmark scrapes the mocked PDUs, then reports
// the missing processing layer via b.Fatal.

// BenchmarkPipeline measures a full processing pass over the representative
// Redfish PDU fixture: scrape (from the mock PDU) -> raw data queue ->
// data-processor -> store in the result cache.
func BenchmarkPipeline(b *testing.B) {
	ctx := context.Background()

	source, err := mockpdu.New()
	if err != nil {
		b.Fatalf("build mock PDU source: %v", err)
	}

	ids := source.IDs()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for _, id := range ids {
			// Scraping edge.
			if _, _, err := performance.Scrape(ctx, source, id); err != nil {
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
			source, err := mockpdu.NewScaled(n)
			if err != nil {
				b.Fatalf("build scaled mock PDU source: %v", err)
			}

			ids := source.IDs()

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				for _, id := range ids {
					// Scraping edge.
					if _, _, err := performance.Scrape(ctx, source, id); err != nil {
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

	source, err := mockpdu.NewScaled(pduCount)
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
			if _, _, err := performance.Scrape(ctx, source, id); err != nil {
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
