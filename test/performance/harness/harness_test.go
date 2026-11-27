// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package harness_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/test/performance/harness"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// testProcessor exercises harness mechanics only; it is never a benchmark
// substitute for the production processor.
func testProcessor(
	process func(harness.Submission, cache.Cache[harness.CachedResult]),
) harness.StartProcessor {
	// Build an independently running worker group for harness verification.
	return func(
		ctx context.Context,
		queue <-chan harness.Submission,
		results cache.Cache[harness.CachedResult],
		workers int,
	) (harness.ProcessorRuntime, error) {
		// Readiness and shutdown are reported independently of submissions.
		ready := make(chan struct{})
		done := make(chan error, 1)

		var started, stopped sync.WaitGroup
		started.Add(workers)
		stopped.Add(workers)

		for range workers {
			go func() {
				defer stopped.Done()

				started.Done()

				for {
					select {
					case <-ctx.Done():
						return
					case submission := <-queue:
						process(submission, results)
					}
				}
			}()
		}

		go func() {
			started.Wait()
			close(ready)
			stopped.Wait()

			done <- nil

			close(done)
		}()

		return harness.ProcessorRuntime{Ready: ready, Done: done}, nil
	}
}

type cycleOutcome struct {
	inputs  []harness.Reading
	results []harness.Result
	err     error
}

func runProducerCycles(
	ctx context.Context, h *harness.Harness, inputs []harness.Reading, cycles int,
) cycleOutcome {
	// Each cycle must observe freshly processed values for the same PDU inputs.
	var (
		results  []harness.Result
		previous float64
	)

	for range cycles {
		var err error

		results, err = h.SubmitAndWait(ctx, inputs)
		if err != nil {
			return cycleOutcome{err: err}
		}

		if results[0].PowerWatts <= previous {
			return cycleOutcome{err: errors.New("cycle returned stale result: power did not increase")}
		}

		previous = results[0].PowerWatts
	}

	return cycleOutcome{inputs: inputs, results: results}
}

func TestHarnessCycles(t *testing.T) {
	cases := []struct {
		name          string
		pdus          int
		cycles        int
		producers     int
		workers       int
		queueCapacity int
	}{
		{name: "single unbuffered", pdus: 1, cycles: 1, producers: 1, workers: 1},
		{name: "repeated batch uses fresh results", pdus: 10, cycles: 3, producers: 1, workers: 4, queueCapacity: 2},
		{name: "concurrent disjoint producers", pdus: 8, cycles: 3, producers: 4, workers: 2, queueCapacity: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				processed   atomic.Int64
				cacheSize   atomic.Int64
				runtimeDone <-chan error
			)

			start := testProcessor(func(s harness.Submission, c cache.Cache[harness.CachedResult]) {
				processed.Add(1)
				// Deliberately transform data so returning scraped inputs or stale
				// results would fail the expectations across repeated cycles.
				result := harness.Result(s.Reading)

				result.PowerWatts += float64(s.ID)
				if _, err := c.Get(result.PDUID); err != nil {
					cacheSize.Add(1)
				}

				c.Set(result.PDUID, harness.CachedResult{SubmissionID: s.ID, Result: result})
			})
			// This cleanup runs after the harness cleanup, verifying workers exited.
			t.Cleanup(func() {
				select {
				case <-runtimeDone:
				default:
					t.Error("processor workers still running after harness cleanup")
				}
			})
			h := harness.NewHarness(t, harness.Config{
				WorkerCount: tc.workers, QueueCapacity: tc.queueCapacity,
				StartProcessor: func(
					ctx context.Context, q <-chan harness.Submission,
					c cache.Cache[harness.CachedResult], workers int,
				) (harness.ProcessorRuntime, error) {
					// Retain the shutdown signal for the post-cleanup assertion.
					runtime, err := start(ctx, q, c, workers)
					runtimeDone = runtime.Done

					return runtime, err
				},
			})
			source, err := mockpdu.NewRedfishScaled("../../../testdata/pdu-mockup", tc.pdus)
			require.NoError(t, err)

			inputs := make([]harness.Reading, 0, tc.pdus)

			for _, id := range source.IDs() {
				reading, err := source.Read(t.Context(), id)
				require.NoError(t, err)
				assert.Equal(t, id, reading.PDUID)
				inputs = append(inputs, reading)
			}

			outcomes := make(chan cycleOutcome, tc.producers)

			var wg sync.WaitGroup

			for p := range tc.producers {
				partition := inputs[p*(tc.pdus/tc.producers) : (p+1)*(tc.pdus/tc.producers)]

				wg.Go(func() {
					outcomes <- runProducerCycles(t.Context(), h, partition, tc.cycles)
				})
			}

			wg.Wait()
			close(outcomes)

			for got := range outcomes {
				require.NoError(t, got.err)
				require.Len(t, got.results, len(got.inputs))

				for i, input := range got.inputs {
					assert.Equal(t, input.PDUID, got.results[i].PDUID)
					assert.Greater(t, got.results[i].PowerWatts, input.PowerWatts)
				}
			}

			assert.Equal(t, int64(tc.pdus*tc.cycles), processed.Load())
			assert.Equal(t, int64(tc.pdus), cacheSize.Load())
		})
	}
}

func TestHarnessCycleFailures(t *testing.T) {
	cases := []struct {
		name               string
		inputs             []harness.Reading
		cancel             bool
		noConsumer         bool
		wantErr            error
		wantMessage        string
		wantRuntimeStopped bool
	}{
		{name: "empty cycle", wantMessage: "at least one reading"},
		{name: "empty PDU ID", inputs: []harness.Reading{{}}, wantMessage: "empty, duplicated"},
		{name: "duplicate PDU IDs", inputs: []harness.Reading{{PDUID: "a"}, {PDUID: "a"}}, wantMessage: "duplicated"},
		{
			name: "missing result times out", inputs: []harness.Reading{{PDUID: "a"}},
			wantErr: context.DeadlineExceeded, wantMessage: "a#1", wantRuntimeStopped: true,
		},
		{
			name: "canceled submission", inputs: []harness.Reading{{PDUID: "a"}}, cancel: true,
			wantErr: context.Canceled, wantMessage: "enqueue", wantRuntimeStopped: true,
		},
		{
			name: "queue backpressure times out", inputs: []harness.Reading{{PDUID: "a"}, {PDUID: "b"}},
			noConsumer: true, wantErr: context.DeadlineExceeded,
			wantMessage: "enqueue PDU \"b\"", wantRuntimeStopped: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := testProcessor(func(harness.Submission, cache.Cache[harness.CachedResult]) {})
			if tc.noConsumer {
				start = func(
					ctx context.Context, _ <-chan harness.Submission,
					_ cache.Cache[harness.CachedResult], _ int,
				) (harness.ProcessorRuntime, error) {
					// Keep the runtime alive without consuming queued submissions.
					ready := make(chan struct{})
					done := make(chan error, 1)

					close(ready)

					go func() { <-ctx.Done(); done <- nil; close(done) }()

					return harness.ProcessorRuntime{Ready: ready, Done: done}, nil
				}
			}

			h := harness.NewHarness(t, harness.Config{
				WorkerCount: 1, QueueCapacity: 1, CycleTimeout: 20 * time.Millisecond,
				StartProcessor: start,
			})

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tc.cancel {
				cancel()
			}

			got, err := h.SubmitAndWait(ctx, tc.inputs)
			require.Error(t, err)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			require.ErrorContains(t, err, tc.wantMessage)
			assert.Nil(t, got)

			if tc.wantRuntimeStopped {
				_, err := h.SubmitAndWait(t.Context(), []harness.Reading{{PDUID: "next"}})
				require.ErrorContains(t, err, "runtime stopped")
				assert.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestHarnessRejectsOverlappingPDUs(t *testing.T) {
	cases := []struct {
		name         string
		inputs       []harness.Reading
		secondInputs []harness.Reading
		wantMessage  string
	}{
		{
			name: "single PDU", inputs: []harness.Reading{{PDUID: "a"}},
			secondInputs: []harness.Reading{{PDUID: "a"}}, wantMessage: "already in flight",
		},
		{
			name: "partial batch overlap", inputs: []harness.Reading{{PDUID: "a"}, {PDUID: "b"}},
			secondInputs: []harness.Reading{{PDUID: "c"}, {PDUID: "b"}}, wantMessage: "already in flight",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan struct{}, 1)
			release := make(chan struct{})

			var signalOnce sync.Once

			h := harness.NewHarness(t, harness.Config{
				WorkerCount: 1, QueueCapacity: len(tc.inputs),
				StartProcessor: testProcessor(func(s harness.Submission, c cache.Cache[harness.CachedResult]) {
					signalOnce.Do(func() { received <- struct{}{} })
					<-release
					c.Set(s.Reading.PDUID, harness.CachedResult{SubmissionID: s.ID, Result: harness.Result(s.Reading)})
				}),
			})

			var releaseOnce sync.Once

			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()

			finished := make(chan error, 1)

			go func() { _, err := h.SubmitAndWait(t.Context(), tc.inputs); finished <- err }()

			select {
			case <-received:
			case <-time.After(time.Second):
				t.Fatal("first submission was not received")
			}

			_, err := h.SubmitAndWait(t.Context(), tc.secondInputs)
			require.ErrorContains(t, err, tc.wantMessage)
			unblock()
			require.NoError(t, <-finished)
			// A rejected overlap must neither cancel the runtime nor reserve the
			// other PDU IDs from its partially checked batch.
			_, err = h.SubmitAndWait(t.Context(), tc.secondInputs)
			require.NoError(t, err)
		})
	}
}
