// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

// WaitForResults waits synchronously for the current submissions, never accepting
// an older cache entry for the same PDU. It returns results in submission order.
// Polling contributes to measured completion latency. No goroutine outlives the
// call, even when the context expires.
func WaitForResults(
	ctx context.Context,
	c cache.Cache[CachedResult],
	submissions []Submission,
	interval time.Duration,
) ([]Result, error) {
	// Validate before constructing the polling ticker.
	if interval <= 0 {
		return nil, errors.New("poll interval must be positive")
	}

	results := make([]Result, len(submissions))

	pending := make(map[int]Submission, len(submissions))
	for i, submission := range submissions {
		pending[i] = submission
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			missing := make([]string, 0, len(pending))
			for _, s := range pending {
				missing = append(missing, fmt.Sprintf("%s#%d", s.Reading.PDUID, s.ID))
			}

			sort.Strings(missing)

			return nil, fmt.Errorf("waiting for submissions %v: %w", missing, err)
		}

		for i, s := range pending {
			value, err := c.Get(s.Reading.PDUID)
			if err != nil && !errors.Is(err, cache.ErrCacheNotFound) {
				return nil, fmt.Errorf("read result for %s#%d: %w", s.Reading.PDUID, s.ID, err)
			}

			if err == nil && value.SubmissionID == s.ID && value.Result.PDUID == s.Reading.PDUID {
				results[i] = value.Result
				delete(pending, i)
			}
		}

		if len(pending) == 0 {
			return results, nil
		}

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
