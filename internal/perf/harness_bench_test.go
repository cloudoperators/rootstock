// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package perf_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cloudoperators/rootstock/internal/perf"
	"github.com/cloudoperators/rootstock/internal/perf/mockpdu"
)

// BenchmarkPipeline measures a full processing pass over the representative
// Redfish PDU fixture: scrape (from the mock PDU) -> process -> store in the
// result cache. It is the baseline "mock PDU -> abstract result cache" benchmark.
func BenchmarkPipeline(b *testing.B) {
	ctx := context.Background()

	source, err := mockpdu.New()
	if err != nil {
		b.Fatalf("build mock PDU source: %v", err)
	}

	pipeline := perf.NewPipeline(source, source.IDs(), nil, nil)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := pipeline.Run(ctx); err != nil {
			b.Fatalf("pipeline run: %v", err)
		}
	}
}

// BenchmarkPipelineScale measures how a full processing pass scales with the
// number of PDUs ("mass PDU scanning / horizontal scaling"). Each sub-benchmark
// drives N synthesized PDUs through the pipeline into the result cache.
func BenchmarkPipelineScale(b *testing.B) {
	ctx := context.Background()

	for _, n := range []int{1, 10, 100, 1000} {
		b.Run("pdus="+strconv.Itoa(n), func(b *testing.B) {
			source, err := mockpdu.NewScaled(n)
			if err != nil {
				b.Fatalf("build scaled mock PDU source: %v", err)
			}

			pipeline := perf.NewPipeline(source, source.IDs(), nil, nil)

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				if err := pipeline.Run(ctx); err != nil {
					b.Fatalf("pipeline run: %v", err)
				}
			}
		})
	}
}

// BenchmarkPipelineParallel measures concurrent processing of distinct PDUs
// through a shared pipeline/result cache, reflecting the concurrent-scraper
// model described in the architecture design.
func BenchmarkPipelineParallel(b *testing.B) {
	ctx := context.Background()

	const pduCount = 100

	source, err := mockpdu.NewScaled(pduCount)
	if err != nil {
		b.Fatalf("build scaled mock PDU source: %v", err)
	}

	ids := source.IDs()
	pipeline := perf.NewPipeline(source, ids, nil, nil)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			id := ids[i%len(ids)]
			if err := pipeline.RunOne(ctx, id); err != nil {
				b.Errorf("pipeline run one: %v", err)

				return
			}

			i++
		}
	})
}
