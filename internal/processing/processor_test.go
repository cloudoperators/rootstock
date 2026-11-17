// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package processing_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/internal/processing"
)

//nolint:godox // TODOs flag value checks tied to intentionally deferred processing.
func matchesResults(results cache.Cache[processing.CachedResult], want map[string]processing.CachedResult) bool {
	for id, expected := range want {
		got, err := results.Get(id)
		// TODO: Review full-result matching when the reading-to-result transformations are defined.
		if err != nil || got != expected {
			return false
		}
	}

	return true
}

//nolint:godox // TODOs flag value checks tied to intentionally deferred processing.
func assertCachedResults(
	t *testing.T, results cache.Cache[processing.CachedResult], want map[string]processing.CachedResult,
) {
	// Verify the published values after all workers have stopped.
	t.Helper()

	for id, expected := range want {
		got, err := results.Get(id)
		require.NoError(t, err)
		// TODO: Verify expected values for the implemented reading-to-result transformations.
		assert.Equal(t, expected, got)
	}
}

// TestProcessorLifecycle checks queue-to-cache delivery, submission identity, and worker shutdown.
// TODO: Value checks reflect temporary pass-through behavior. Update them when
// the required reading-to-result transformations are implemented.
//
//nolint:godox // TODOs tied to intentionally deferred processing.
func TestProcessorLifecycle(t *testing.T) {
	cases := []struct {
		name       string
		workers    int
		count      int
		closeInput bool
	}{
		{name: "single worker canceled after completion", workers: 1, count: 1},
		{name: "multiple workers canceled after completion", workers: 4, count: 16},
		{name: "closed input drains queued work", workers: 4, count: 16, closeInput: true},
		{name: "closed empty input", workers: 2, closeInput: true},
		{name: "cancel idle workers", workers: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			input := make(chan processing.Submission, tc.count)
			results := cache.New[processing.CachedResult]()

			want := make(map[string]processing.CachedResult, tc.count)
			for i := range tc.count {
				id := "pdu-" + strconv.Itoa(i)
				reading := processing.Reading{
					PDUID: id, PowerWatts: 100, ApparentVA: 120, ReactiveVAR: 20,
					PowerFactor: 0.9, EnergykWh: 42, PowerCapacityVA: 500,
				}

				submissionID := uint64(i + 1)
				input <- processing.Submission{ID: submissionID, Reading: reading}
				// Seed an old result to exercise replacement, including identity.
				results.Set(id, processing.CachedResult{Result: processing.Result{PDUID: id}})
				// TODO: Replace copied readings with expected outputs for the reading-to-result transformations.
				want[id] = processing.CachedResult{SubmissionID: submissionID, Result: processing.Result(reading)}
			}

			if tc.closeInput {
				close(input)
			}

			runtime, err := processing.Start(ctx, input, results, tc.workers)
			require.NoError(t, err)
			// Cleanup also joins workers if an assertion aborts the test early.
			t.Cleanup(func() {
				cancel()

				select {
				case <-runtime.Done:
				case <-time.After(time.Second):
					t.Error("workers did not stop during cleanup")
				}
			})

			select {
			case <-runtime.Ready:
			case <-time.After(time.Second):
				t.Fatal("workers did not become ready")
			}

			require.Eventually(t, func() bool {
				return matchesResults(results, want)
			}, time.Second, time.Millisecond)

			if !tc.closeInput {
				cancel()
			}

			select {
			case err := <-runtime.Done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("workers did not stop")
			}

			assertCachedResults(t, results, want)

			if !tc.closeInput {
				// The processor must leave caller-owned input open on shutdown.
				close(input)
			}
		})
	}
}

func TestProcessorStartErrors(t *testing.T) {
	cases := []struct {
		name        string
		workers     int
		nilInput    bool
		nilCache    bool
		cancel      bool
		wantMessage string
		wantErr     error
	}{
		{name: "zero workers", wantMessage: "worker count must be positive"},
		{name: "negative workers", workers: -1, wantMessage: "worker count must be positive"},
		{name: "missing input", workers: 1, nilInput: true, wantMessage: "input queue and result cache"},
		{name: "missing cache", workers: 1, nilCache: true, wantMessage: "input queue and result cache"},
		{name: "canceled context", workers: 1, cancel: true, wantErr: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			input := make(chan processing.Submission)
			results := cache.New[processing.CachedResult]()

			if tc.nilInput {
				input = nil
			}

			if tc.nilCache {
				results = nil
			}

			if tc.cancel {
				cancel()
			}

			runtime, err := processing.Start(ctx, input, results, tc.workers)
			require.Error(t, err)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			if tc.wantMessage != "" {
				require.ErrorContains(t, err, tc.wantMessage)
			}

			assert.Nil(t, runtime.Ready)
			assert.Nil(t, runtime.Done)
		})
	}
}
