// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

// Config controls the harness runtime. Configurability lets scenarios compare
// the same pipeline under different loads using consistent execution rules,
// and lets unit tests establish controlled success and failure conditions.
// Zero durations use defaults; QueueCapacity may be zero for an unbuffered queue.
type Config struct {
	WorkerCount        int
	QueueCapacity      int
	CycleTimeout       time.Duration
	StartupTimeout     time.Duration
	ShutdownTimeout    time.Duration
	ResultPollInterval time.Duration
	StartProcessor     StartProcessor
}

func (c Config) defaults() Config {
	if c.CycleTimeout == 0 {
		c.CycleTimeout = 5 * time.Second
	}

	if c.StartupTimeout == 0 {
		c.StartupTimeout = 5 * time.Second
	}

	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 5 * time.Second
	}

	if c.ResultPollInterval == 0 {
		c.ResultPollInterval = time.Millisecond
	}

	return c
}

func (c Config) valid() bool {
	return c.WorkerCount > 0 && c.QueueCapacity >= 0 &&
		c.CycleTimeout > 0 && c.StartupTimeout > 0 &&
		c.ShutdownTimeout > 0 && c.ResultPollInterval > 0
}

// Harness owns a persistent queue, cache, and processor runtime per scenario.
// Concurrent cycles must use disjoint PDU IDs: the production-style cache holds
// only the latest result per PDU. This keeps cache size bounded by device count.
type Harness struct {
	ctx     context.Context //nolint:containedctx // Owns the runtime lifetime across multiple cycle contexts.
	cancel  context.CancelFunc
	queue   chan Submission
	results cache.Cache[CachedResult]
	cfg     Config
	nextID  atomic.Uint64
	mu      sync.Mutex
	active  map[string]bool
}

// NewHarness starts the processor and registers bounded shutdown before waiting
// for readiness. A missing processor configuration skips the scenario explicitly.
func NewHarness(tb testing.TB, cfg Config) *Harness {
	tb.Helper()

	if cfg.StartProcessor == nil {
		tb.Skip("no processor configured; set Config.StartProcessor")
	}

	cfg = cfg.defaults()
	if !cfg.valid() {
		tb.Fatal("invalid harness configuration")
	}

	ctx, cancel := context.WithCancel(tb.Context())
	h := &Harness{
		ctx: ctx, cancel: cancel, cfg: cfg,
		queue:   make(chan Submission, cfg.QueueCapacity),
		results: cache.New[CachedResult](),
		active:  make(map[string]bool),
	}
	tb.Cleanup(cancel)

	runtime, err := cfg.StartProcessor(ctx, h.queue, h.results, cfg.WorkerCount)
	if err != nil {
		tb.Fatalf("start processor: %v", err)
	}

	if runtime.Done == nil {
		tb.Fatal("processor must supply a shutdown channel")
	}

	tb.Cleanup(func() {
		cancel()

		timer := time.NewTimer(cfg.ShutdownTimeout)
		defer timer.Stop()

		select {
		case err := <-runtime.Done:
			if err != nil {
				tb.Errorf("processor shutdown: %v", err)
			}
		case <-timer.C:
			tb.Errorf("processor did not stop within %s", cfg.ShutdownTimeout)
		}
	})

	if runtime.Ready == nil {
		tb.Fatal("processor must supply a readiness channel")
	}

	timer := time.NewTimer(cfg.StartupTimeout)
	defer timer.Stop()

	select {
	case <-runtime.Ready:
	case <-runtime.Done:
		tb.Fatal("processor exited before readiness")
	case <-timer.C:
		tb.Fatalf("processor did not become ready within %s", cfg.StartupTimeout)
	case <-ctx.Done():
		tb.Fatalf("processor startup canceled: %v", ctx.Err())
	}

	return h
}

// SubmitAndWait enqueues already-scraped inputs and waits for one corresponding cache
// result per input. Identity allocation, queueing, and completion observation are
// part of the operation. Any enqueue/completion failure cancels the runtime so
// late work cannot overwrite results in a subsequent cycle.
func (h *Harness) SubmitAndWait(ctx context.Context, inputs []Reading) ([]Result, error) {
	if err := h.ctx.Err(); err != nil {
		return nil, fmt.Errorf("processor runtime stopped: %w", err)
	}

	if len(inputs) == 0 {
		return nil, errors.New("cycle requires at least one reading")
	}

	h.mu.Lock()

	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		if input.PDUID == "" || seen[input.PDUID] || h.active[input.PDUID] {
			h.mu.Unlock()
			return nil, fmt.Errorf("PDU %q is empty, duplicated, or already in flight", input.PDUID)
		}

		seen[input.PDUID] = true
	}

	for id := range seen {
		h.active[id] = true
	}

	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		for id := range seen {
			delete(h.active, id)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, h.cfg.CycleTimeout)
	defer cancel()

	// A cycle must stop on either caller cancellation or runtime shutdown.
	stop := context.AfterFunc(h.ctx, cancel) //nolint:contextcheck // Intentionally merges runtime and caller lifetimes.
	defer stop()

	// submit
	submissions := make([]Submission, len(inputs))
	for i, input := range inputs {
		if err := ctx.Err(); err != nil {
			h.cancel()
			return nil, fmt.Errorf("enqueue PDU %q: %w", input.PDUID, err)
		}

		submissions[i] = Submission{ID: h.nextID.Add(1), Reading: input}
		select {
		case <-ctx.Done():
			h.cancel()
			return nil, fmt.Errorf("enqueue PDU %q: %w", input.PDUID, ctx.Err())
		case h.queue <- submissions[i]:
		}
	}

	// collect
	results, err := WaitForResults(ctx, h.results, submissions, h.cfg.ResultPollInterval)
	if err != nil {
		h.cancel()
	}

	return results, err
}
