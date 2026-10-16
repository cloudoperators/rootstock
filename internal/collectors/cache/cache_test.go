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

	tests := []struct {
		name      string
		setup     func(t *testing.T)
		wantError error
	}{
		{
			name: "string cache - set and get",
			setup: func(t *testing.T) {
				cache := New[string]()
				cache.Set("key1", "value1")

				value, err := cache.Get("key1")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if value != "value1" {
					t.Errorf("expected value1, got %s", value)
				}
			},
		},
		{
			name: "string cache - overwrite value",
			setup: func(t *testing.T) {
				cache := New[string]()
				cache.Set("key1", "value1")
				cache.Set("key1", "value2")

				value, err := cache.Get("key1")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if value != "value2" {
					t.Errorf("expected value2, got %s", value)
				}
			},
		},
		{
			name: "cache miss - nonexistent key",
			setup: func(t *testing.T) {
				cache := New[string]()

				_, err := cache.Get("nonexistent")
				if !errors.Is(err, ErrCacheNotFound) {
					t.Errorf("expected ErrCacheNotFound, got %v", err)
				}
			},
		},
		{
			name: "int cache - set and get",
			setup: func(t *testing.T) {
				cache := New[int]()
				cache.Set("answer", 42)

				value, err := cache.Get("answer")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if value != 42 {
					t.Errorf("expected 42, got %d", value)
				}
			},
		},
		{
			name: "struct cache - set and get",
			setup: func(t *testing.T) {
				cache := New[RackPDU]()

				device := RackPDU{
					ID:     "device1",
					Name:   "Test Device",
					Metric: 67.89,
				}

				cache.Set("device1", device)

				retrieved, err := cache.Get("device1")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if retrieved.ID != device.ID || retrieved.Name != device.Name || retrieved.Metric != device.Metric {
					t.Errorf("retrieved device doesn't match: got %+v, want %+v", retrieved, device)
				}
			},
		},
		{
			name: "struct cache - multiple devices",
			setup: func(t *testing.T) {
				cache := New[RackPDU]()

				device1 := RackPDU{ID: "dev1", Name: "Device 1", Metric: 10.5}
				device2 := RackPDU{ID: "dev2", Name: "Device 2", Metric: 20.5}

				cache.Set("dev1", device1)
				cache.Set("dev2", device2)

				// Get first device
				val1, err := cache.Get("dev1")
				if err != nil {
					t.Fatalf("unexpected error for dev1: %v", err)
				}
				if val1.ID != "dev1" {
					t.Errorf("expected dev1, got %s", val1.ID)
				}

				// Get second device
				val2, err := cache.Get("dev2")
				if err != nil {
					t.Fatalf("unexpected error for dev2: %v", err)
				}
				if val2.ID != "dev2" {
					t.Errorf("expected dev2, got %s", val2.ID)
				}
			},
		},
		{
			name: "any cache - store different types",
			setup: func(t *testing.T) {
				cache := New[any]()

				cache.Set("string", "hello")
				cache.Set("int", 123)
				cache.Set("struct", RackPDU{ID: "test"})

				// Get string
				val1, err := cache.Get("string")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if str, ok := val1.(string); !ok || str != "hello" {
					t.Errorf("expected 'hello', got %v", val1)
				}

				// Get int
				val2, err := cache.Get("int")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if num, ok := val2.(int); !ok || num != 123 {
					t.Errorf("expected 123, got %v", val2)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
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
