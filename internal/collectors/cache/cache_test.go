// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"sync"
	"testing"
)

func TestCache(t *testing.T) {
	type RackPDU struct {
		ID     string
		Name   string
		Metric float64
	}

	type setEntry struct {
		key   string
		value any
	}

	tests := []struct {
		name       string
		newCache   func(*testing.T) anyCacher
		setEntries []setEntry
		getKey     string
		wantValue  any
		wantError  error
	}{
		{
			name:       "string cache - set and get",
			newCache:   wrapCache[string],
			setEntries: []setEntry{{"key1", "value1"}},
			getKey:     "key1",
			wantValue:  "value1",
		},
		{
			name:       "string cache - overwrite value",
			newCache:   wrapCache[string],
			setEntries: []setEntry{{"key1", "value1"}, {"key1", "value2"}},
			getKey:     "key1",
			wantValue:  "value2",
		},
		{
			name:      "cache miss - nonexistent key",
			newCache:  wrapCache[string],
			getKey:    "nonexistent",
			wantError: ErrCacheNotFound,
		},
		{
			name:       "int cache - set and get",
			newCache:   wrapCache[int],
			setEntries: []setEntry{{"answer", 42}},
			getKey:     "answer",
			wantValue:  42,
		},
		{
			name:     "struct cache - set and get",
			newCache: wrapCache[RackPDU],
			setEntries: []setEntry{
				{"device1", RackPDU{ID: "device1", Name: "Test Device", Metric: 67.89}},
			},
			getKey:    "device1",
			wantValue: RackPDU{ID: "device1", Name: "Test Device", Metric: 67.89},
		},
		{
			name:     "struct cache - multiple devices, get first",
			newCache: wrapCache[RackPDU],
			setEntries: []setEntry{
				{"dev1", RackPDU{ID: "dev1", Name: "Device 1", Metric: 10.5}},
				{"dev2", RackPDU{ID: "dev2", Name: "Device 2", Metric: 20.5}},
			},
			getKey:    "dev1",
			wantValue: RackPDU{ID: "dev1", Name: "Device 1", Metric: 10.5},
		},
		{
			name:     "struct cache - multiple devices, get second",
			newCache: wrapCache[RackPDU],
			setEntries: []setEntry{
				{"dev1", RackPDU{ID: "dev1", Name: "Device 1", Metric: 10.5}},
				{"dev2", RackPDU{ID: "dev2", Name: "Device 2", Metric: 20.5}},
			},
			getKey:    "dev2",
			wantValue: RackPDU{ID: "dev2", Name: "Device 2", Metric: 20.5},
		},
		{
			name:     "any cache - get string value",
			newCache: wrapCache[any],
			setEntries: []setEntry{
				{"string", "hello"},
				{"int", 123},
				{"struct", RackPDU{ID: "test"}},
			},
			getKey:    "string",
			wantValue: "hello",
		},
		{
			name:       "any cache - get int value",
			newCache:   wrapCache[any],
			setEntries: []setEntry{{"string", "hello"}, {"int", 123}},
			getKey:     "int",
			wantValue:  123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.newCache(t)
			for _, e := range tt.setEntries {
				c.Set(e.key, e.value)
			}

			value, err := c.Get(tt.getKey)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("unexpected error: got %v, want %v", err, tt.wantError)
			}
			if tt.wantError == nil && value != tt.wantValue {
				t.Errorf("unexpected value: got %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := New[int]()
	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(2 * goroutines) // 200 wait groups for both read and write

	// Writers
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				cache.Set("key", id*iterations+j)
			}
		}(i)
	}

	// Readers
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				_, _ = cache.Get("key")
			}
		}()
	}

	wg.Wait()

	_, err := cache.Get("key")
	if err != nil {
		t.Errorf("expected key to exist after concurrent operations, got error: %v", err)
	}
}

type anyCacher interface {
	Set(key string, val any)
	Get(key string) (any, error)
}

type cacheAdapter[T any] struct {
	t *testing.T
	c Cache[T]
}

func (a *cacheAdapter[T]) Set(key string, val any) {
	v, ok := val.(T)
	if !ok {
		var t T
		a.t.Errorf("illegal type passed to caches Set method: expecting %T but got %T", t, val)
		a.t.Fail()
	}
	a.c.Set(key, v)
}
func (a *cacheAdapter[T]) Get(key string) (any, error) { return a.c.Get(key) }

func wrapCache[T any](t *testing.T) anyCacher {
	t.Helper()
	return &cacheAdapter[T]{t: t, c: New[T]()}
}
