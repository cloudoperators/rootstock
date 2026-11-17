// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package harness manages processor lifecycles and submit-and-wait operations
// for performance scenarios.
package harness

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/internal/processing"
)

// Reading is one scraped PDU measurement, before processing.
type Reading = processing.Reading

// Result is one processed, cache-ready PDU measurement.
type Result = processing.Result

// Source is a protocol-neutral mock PDU source.
type Source interface {
	// IDs enumerates the stable PDU IDs served by the source.
	IDs() []string
	// Read returns one scraped measurement for the given PDU ID.
	Read(ctx context.Context, pduID string) (Reading, error)
}

// Submission identifies a single reading across repeated benchmark cycles.
type Submission = processing.Submission

// CachedResult preserves submission identity alongside the processed value.
// The processor stores it under Result.PDUID in the result cache.
type CachedResult = processing.CachedResult

// ProcessorRuntime represents an independently running processor. Ready closes once all
// workers are ready. Done delivers the terminal error (nil on clean shutdown),
// then closes, only after every worker has exited. StartProcessor must return
// promptly and all started resources must respect cancellation, including when
// startup fails.
type ProcessorRuntime = processing.Runtime

// StartProcessor connects a scenario to a production processor implementation.
// Its adapter must preserve submission IDs; it must not substitute test-side
// processing for production work. It does not own or close the input queue.
type StartProcessor func(context.Context, <-chan Submission, cache.Cache[CachedResult], int) (ProcessorRuntime, error)
