// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"sync"
	"testing"
)

func TestNewCache(t *testing.T) {
	cache := NewCache[string]()
	if cache == nil {
		t.Fatal("NewCache returned nil")
	}

	if cache.data == nil {
		t.Fatal("cache.data should be initialized")
	}

	if cache.logger == nil {
		t.Fatal("cache.logger should be initialized")
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache[string]()

	// Test Set and Get
	cache.Set("key1", "value1")
	value, err := cache.Get("key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Note: Get returns any, so we need to type assert
	strValue, ok := value.(string)
	if !ok {
		t.Fatalf("expected string value, got %T", value)
	}
	if strValue != "value1" {
		t.Errorf("expected value1, got %s", strValue)
	}

	// Test overwrite
	cache.Set("key1", "value2")
	value, err = cache.Get("key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	strValue, ok = value.(string)
	if !ok {
		t.Fatalf("expected string value, got %T", value)
	}
	if strValue != "value2" {
		t.Errorf("expected value2, got %s", strValue)
	}
}

func TestCacheGet_NotFound(t *testing.T) {
	cache := NewCache[string]()

	_, err := cache.Get("nonexistent")
	if !errors.Is(err, ErrCacheNotFound) {
		t.Errorf("expected ErrCacheNotFound, got %v", err)
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache[int]()
	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

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

	// Should not panic - cache is thread-safe
	_, err := cache.Get("key")
	if err != nil {
		t.Errorf("expected key to exist after concurrent operations, got error: %v", err)
	}
}

func TestCacheWithStructs(t *testing.T) {
	type RackPDU struct {
		ID     string
		Name   string
		Metric float64
	}

	cache := NewCache[RackPDU]()

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

	// Type assert since Get returns any
	deviceValue, ok := retrieved.(RackPDU)
	if !ok {
		t.Fatalf("expected Device type, got %T", retrieved)
	}

	if deviceValue.ID != device.ID || deviceValue.Name != device.Name || deviceValue.Metric != device.Metric {
		t.Errorf("retrieved device doesn't match: %+v", deviceValue)
	}
}
