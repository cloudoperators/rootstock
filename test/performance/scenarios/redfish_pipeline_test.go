// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build performance

package scenarios_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/processing"
	"github.com/cloudoperators/rootstock/test/performance/harness"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

var startProductionProcessor harness.StartProcessor = processing.Start

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

// setup to pull input data for the processing queue from the mock pdus
func scrapeInputs(tb testing.TB, n int) []harness.Reading {
	tb.Helper()

	source, err := mockpdu.NewRedfishScaled("../../../testdata/pdu-mockup", n)
	require.NoError(tb, err)

	inputs := make([]harness.Reading, 0, n)

	for _, id := range source.IDs() {
		reading, err := source.Read(tb.Context(), id)
		require.NoError(tb, err)

		inputs = append(inputs, reading)
	}

	return inputs
}

func verifyResults(tb testing.TB, inputs []harness.Reading, results []harness.Result) {
	tb.Helper()
	require.Len(tb, results, len(inputs))

	for i, input := range inputs {
		// Update expected values here if production processing transforms fields.
		require.Equal(tb, harness.Result(input), results[i])
	}
}

// set up test harness, pull inputs and plug them into the processing input queue, then wait for the processor to finish
func TestRedfishPipeline(t *testing.T) {
	for _, scenario := range redfishScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			h := harness.NewHarness(t, harness.Config{
				WorkerCount: scenario.workers, QueueCapacity: scenario.queueCapacity,
				StartProcessor: startProductionProcessor,
			})

			inputs := scrapeInputs(t, scenario.pdus)
			// two cache cycles, empty and pre-filled.
			// harness must wait for new results, rather than accepting previous results
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
// note: if we start measuring after enqueuing, we would miss work, since the processor already started working
func BenchmarkRedfishProcessing(b *testing.B) {
	for _, scenario := range redfishScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			h := harness.NewHarness(b, harness.Config{
				WorkerCount: scenario.workers, QueueCapacity: scenario.queueCapacity,
				StartProcessor: startProductionProcessor,
			})
			inputs := scrapeInputs(b, scenario.pdus)
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
			// Each of b.N cycles completes scenario.pdus results.
			// Divide elapsed time by that total for average time per result (amortized),
			// or divide the total by elapsed seconds for throughput.
			// ns/result is the total measured time divided by the number of completed results, in nanoseconds.
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(scenario.pdus), "ns/result")
			b.ReportMetric(float64(b.N)*float64(scenario.pdus)/b.Elapsed().Seconds(), "results/s")
		})
	}
}

// BenchmarkRedfishProcessingConcurrent uses disjoint PDU inputs for each producer, so the
// latest-per-PDU cache cannot overwrite another producer's pending result.
func BenchmarkRedfishProcessingConcurrent(b *testing.B) {
	scenarios := []struct {
		name      string
		producers int
		workers   int
	}{
		{name: "producers=2/workers=1", producers: 2, workers: 1},
		{name: "producers=4/workers=4", producers: 4, workers: 4},
		{name: "producers=8/workers=4", producers: 8, workers: 4},
	}
	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			h := harness.NewHarness(b, harness.Config{
				WorkerCount: scenario.workers, QueueCapacity: scenario.producers,
				StartProcessor: startProductionProcessor,
			})
			inputs := scrapeInputs(b, scenario.producers)
			results, err := h.SubmitAndWait(b.Context(), inputs)
			require.NoError(b, err)
			verifyResults(b, inputs, results)

			// The WaitGroup tracks the producer goroutines. Each calls Done when it
			// exits, and Wait blocks until all producers have finished, so the timer
			// can stop and the error channel can close after their work is complete.
			var (
				next atomic.Int64
				wg   sync.WaitGroup
			)

			errors := make(chan error, scenario.producers)

			ctx, cancel := context.WithCancel(b.Context())
			defer cancel()

			wg.Add(scenario.producers)
			b.ReportAllocs()
			b.ResetTimer()

			// submit scraped inputs for async processing, wait for processor to complete
			for _, input := range inputs {
				go func() {
					defer wg.Done()

					for next.Add(1) <= int64(b.N) {
						// drop the results with _
						if _, err := h.SubmitAndWait(ctx, []harness.Reading{input}); err != nil {
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

			// Each operation completes one result; divide the total by elapsed seconds for throughput.
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "results/s")
		})
	}
}
