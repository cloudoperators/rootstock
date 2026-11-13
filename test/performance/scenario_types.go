// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package performance supplies protocol-neutral fixtures and a submit-and-await
// harness for asynchronous processing benchmarks.
package performance

import (
	"context"
	"time"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

// Reading is one scraped PDU measurement, before processing.
type Reading struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Result is one processed, cache-ready PDU measurement.
type Result struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Source is a protocol-neutral mock PDU source.
type Source interface {
	// IDs enumerates the stable PDU IDs served by the source.
	IDs() []string
	// Read returns one scraped measurement for the given PDU ID.
	Read(ctx context.Context, pduID string) (Reading, error)
}

// Submission identifies a single reading across repeated benchmark cycles.
type Submission struct {
	ID      uint64
	Reading Reading
}

// CachedResult preserves submission identity alongside the processed value.
// The processor stores it under Result.PDUID in the result cache.
type CachedResult struct {
	SubmissionID uint64
	Result       Result
}

// ProcessorRuntime represents an independently running processor. Ready closes once all
// workers are ready. Done delivers the terminal error (nil on clean shutdown),
// then closes, only after every worker has exited. StartProcessor must return
// promptly and all started resources must respect cancellation, including when
// startup fails.
type ProcessorRuntime struct {
	Ready <-chan struct{}
	Done  <-chan error
}

// StartProcessor is the connection point for the future production processor.
// Its adapter must preserve submission IDs; it must not substitute test-side
// processing for production work. It does not own or close the input queue.
type StartProcessor func(context.Context, <-chan Submission, cache.Cache[CachedResult], int) (ProcessorRuntime, error)

// ScenarioConfig configures one isolated processor runtime. Zero durations use the
// defaults below; QueueCapacity may be zero for an unbuffered queue.
type ScenarioConfig struct {
	WorkerCount        int
	QueueCapacity      int
	CycleTimeout       time.Duration
	StartupTimeout     time.Duration
	ShutdownTimeout    time.Duration
	ResultPollInterval time.Duration
	StartProcessor     StartProcessor
}

func (s ScenarioConfig) defaults() ScenarioConfig {
	if s.CycleTimeout == 0 {
		s.CycleTimeout = 5 * time.Second
	}

	if s.StartupTimeout == 0 {
		s.StartupTimeout = 5 * time.Second
	}

	if s.ShutdownTimeout == 0 {
		s.ShutdownTimeout = 5 * time.Second
	}

	if s.ResultPollInterval == 0 {
		s.ResultPollInterval = time.Millisecond
	}

	return s
}

func (s ScenarioConfig) valid() bool {
	return s.WorkerCount > 0 && s.QueueCapacity >= 0 &&
		s.CycleTimeout > 0 && s.StartupTimeout > 0 &&
		s.ShutdownTimeout > 0 && s.ResultPollInterval > 0
}
