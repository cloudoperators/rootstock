// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package processing consumes scraped readings and publishes matching cache results.
package processing

import (
	"context"
	"errors"
	"sync"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

// Start launches workers and returns their lifecycle signals. Workers stop on
// context cancellation or input closure. The caller owns the input channel.
func Start(
	ctx context.Context,
	input <-chan Submission,
	results cache.Cache[CachedResult],
	workerCount int,
) (Runtime, error) {
	// Validate before launching any goroutines.
	if workerCount < 1 {
		return Runtime{}, errors.New("processing: worker count must be positive")
	}

	if input == nil || results == nil {
		return Runtime{}, errors.New("processing: input queue and result cache are required")
	}

	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}

	ready := make(chan struct{})
	done := make(chan error, 1)

	var started, stopped sync.WaitGroup
	started.Add(workerCount)

	for range workerCount {
		stopped.Go(func() {
			started.Done()
			consume(ctx, input, results)
		})
	}

	go func() {
		started.Wait()
		close(ready)
		stopped.Wait()

		done <- nil

		close(done)
	}()

	return Runtime{Ready: ready, Done: done}, nil
}

//nolint:godox // The TODO records processing work intentionally deferred by this minimal implementation.
func consume(ctx context.Context, input <-chan Submission, results cache.Cache[CachedResult]) {
	for {
		if ctx.Err() != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case submission, ok := <-input:
			if !ok {
				return
			}
			// TODO: Implement the required reading-to-result transformations once
			// defined. Currently, scraped values are copied unchanged.
			result := Result(submission.Reading)
			results.Set(result.PDUID, CachedResult{SubmissionID: submission.ID, Result: result})
		}
	}
}
