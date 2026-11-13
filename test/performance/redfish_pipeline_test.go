// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build performance

package performance_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/test/performance"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// Connect the production processor adapter here when it becomes available.
var startProductionProcessor performance.StartProcessor

var redfishScenarios = []struct {
	name          string
	pdus          int
	workers       int
	queueCapacity int
}{
	{name: "single", pdus: 1, workers: 1, queueCapacity: 1},
	{name: "batch/pdus=10", pdus: 10, workers: 4, queueCapacity: 10},
	{name: "batch/pdus=100", pdus: 100, workers: 4, queueCapacity: 100},
	{name: "batch/pdus=1000", pdus: 1000, workers: 4, queueCapacity: 100},
}

func scrapeInputs(tb testing.TB, n int) []performance.Reading {
	tb.Helper()

	source, err := mockpdu.NewRedfishScaled("../../testdata/pdu-mockup", n)
	require.NoError(tb, err)

	inputs := make([]performance.Reading, 0, n)

	for _, id := range source.IDs() {
		reading, err := source.Read(tb.Context(), id)
		require.NoError(tb, err)

		inputs = append(inputs, reading)
	}

	return inputs
}

func verifyResults(tb testing.TB, inputs []performance.Reading, results []performance.Result) {
	tb.Helper()
	require.Len(tb, results, len(inputs))

	for i, input := range inputs {
		// Update expected values here if production processing transforms fields.
		require.Equal(tb, performance.Result(input), results[i])
	}
}

func TestRedfishPipeline(t *testing.T) {
	for _, tc := range redfishScenarios {
		t.Run(tc.name, func(t *testing.T) {
			h := performance.NewHarness(t, performance.ScenarioConfig{
				WorkerCount: tc.workers, QueueCapacity: tc.queueCapacity,
				StartProcessor: startProductionProcessor,
			})

			inputs := scrapeInputs(t, tc.pdus)
			for range 2 {
				results, err := h.SubmitAndWait(t.Context(), inputs)
				require.NoError(t, err)
				verifyResults(t, inputs, results)
			}
		})
	}
}

// BenchmarkRedfishProcessing measures enqueue-to-result completion, excluding mock
// construction, scraping, readiness, validation, and shutdown.
func BenchmarkRedfishProcessing(b *testing.B) {
	for _, tc := range redfishScenarios {
		b.Run(tc.name, func(b *testing.B) {
			h := performance.NewHarness(b, performance.ScenarioConfig{
				WorkerCount: tc.workers, QueueCapacity: tc.queueCapacity,
				StartProcessor: startProductionProcessor,
			})
			inputs := scrapeInputs(b, tc.pdus)
			results, err := h.SubmitAndWait(b.Context(), inputs)
			require.NoError(b, err)
			verifyResults(b, inputs, results)
			b.ReportAllocs()

			for b.Loop() {
				results, err = h.SubmitAndWait(b.Context(), inputs)
				if err != nil {
					b.Fatal(err)
				}
			}

			verifyResults(b, inputs, results)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(tc.pdus), "ns/result")
			b.ReportMetric(float64(b.N)*float64(tc.pdus)/b.Elapsed().Seconds(), "results/s")
		})
	}
}

// BenchmarkRedfishProcessingConcurrent uses disjoint PDU inputs for each producer, so the
// latest-per-PDU cache cannot overwrite another producer's pending result.
func BenchmarkRedfishProcessingConcurrent(b *testing.B) {
	cases := []struct {
		name      string
		producers int
		workers   int
	}{
		{name: "producers=2/workers=1", producers: 2, workers: 1},
		{name: "producers=4/workers=4", producers: 4, workers: 4},
		{name: "producers=8/workers=4", producers: 8, workers: 4},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			h := performance.NewHarness(b, performance.ScenarioConfig{
				WorkerCount: tc.workers, QueueCapacity: tc.producers,
				StartProcessor: startProductionProcessor,
			})
			inputs := scrapeInputs(b, tc.producers)
			results, err := h.SubmitAndWait(b.Context(), inputs)
			require.NoError(b, err)
			verifyResults(b, inputs, results)

			var (
				next atomic.Int64
				wg   sync.WaitGroup
			)

			errors := make(chan error, tc.producers)

			ctx, cancel := context.WithCancel(b.Context())
			defer cancel()

			wg.Add(tc.producers)
			b.ReportAllocs()
			b.ResetTimer()

			for _, input := range inputs {
				go func() {
					defer wg.Done()

					for next.Add(1) <= int64(b.N) {
						if _, err := h.SubmitAndWait(ctx, []performance.Reading{input}); err != nil {
							errors <- err

							cancel()

							return
						}
					}
				}()
			}

			wg.Wait()
			b.StopTimer()
			close(errors)

			for err := range errors {
				b.Error(err)
			}

			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "results/s")
		})
	}
}
