// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package harness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/test/performance/harness"
)

func TestWaitForResults(t *testing.T) {
	submissions := []harness.Submission{
		{ID: 10, Reading: harness.Reading{PDUID: "pdu-a"}},
		{ID: 11, Reading: harness.Reading{PDUID: "pdu-b"}},
	}

	cases := []struct {
		name        string
		entries     map[string]harness.CachedResult
		cancel      bool
		want        []harness.Result
		wantErr     error
		wantMissing string
	}{
		{
			name: "all matching results in submission order",
			entries: map[string]harness.CachedResult{
				"pdu-b": {SubmissionID: 11, Result: harness.Result{PDUID: "pdu-b", PowerWatts: 200}},
				"pdu-a": {SubmissionID: 10, Result: harness.Result{PDUID: "pdu-a", PowerWatts: 100}},
			},
			want: []harness.Result{{PDUID: "pdu-a", PowerWatts: 100}, {PDUID: "pdu-b", PowerWatts: 200}},
		},
		{
			name: "stale result cannot satisfy current submission",
			entries: map[string]harness.CachedResult{
				"pdu-a": {SubmissionID: 9, Result: harness.Result{PDUID: "pdu-a"}},
				"pdu-b": {SubmissionID: 11, Result: harness.Result{PDUID: "pdu-b"}},
			},
			wantErr: context.DeadlineExceeded, wantMissing: "pdu-a#10",
		},
		{
			name: "matching identity with wrong PDU is not completion",
			entries: map[string]harness.CachedResult{
				"pdu-a": {SubmissionID: 10, Result: harness.Result{PDUID: "other-pdu"}},
				"pdu-b": {SubmissionID: 11, Result: harness.Result{PDUID: "pdu-b"}},
			},
			wantErr: context.DeadlineExceeded, wantMissing: "pdu-a#10",
		},
		{name: "missing results", wantErr: context.DeadlineExceeded, wantMissing: "pdu-b#11"},
		{name: "canceled wait", cancel: true, wantErr: context.Canceled, wantMissing: "pdu-a#10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resultCache := cache.New[harness.CachedResult]()
			for key, value := range tc.entries {
				resultCache.Set(key, value)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()

			if tc.cancel {
				cancel()
			}

			got, err := harness.WaitForResults(ctx, resultCache, submissions, time.Millisecond)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorContains(t, err, tc.wantMissing)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
