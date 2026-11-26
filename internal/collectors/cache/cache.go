// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"fmt"
	"sync"
)

var ErrCacheNotFound = errors.New("cache entry not found")

// Cacher defines the interface for cache operations.
type Cache[T any] interface {
	// Get retrieves a value from the cache.
	// Returns ErrCacheNotFound if the key doesn't exist.
	Get(key string) (T, error)

	// Set stores a value in the cache.
	Set(key string, data T)
}

// Cache is a generic thread-safe in-memory cache.
type cache[T any] struct {
	mu   sync.RWMutex
	data map[string]T
}

// New creates a new cache.
func New[T any]() Cache[T] {
	return &cache[T]{
		data: make(map[string]T),
	}
}

// Get retrieves a value from the cache.
// Returns ErrCacheNotFound if the key doesn't exist.
func (c *cache[T]) Get(key string) (T, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var zero T

	entry, exists := c.data[key]
	if !exists {
		return zero, ErrCacheNotFound
	}

	return entry, nil
}

// Set stores a value in the cache.
func (c *cache[T]) Set(key string, data T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = data
}

func hello() {
	fmt.Println("hello")
}
