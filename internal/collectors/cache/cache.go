// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"sync"

	"github.com/cloudoperators/rootstock/pkg/log"
)

var (
	ErrCacheNotFound = errors.New("cache not found")
	ErrCacheExpired  = errors.New("cache entry expired")
)

// Cache is a generic thread-safe in-memory cache.
type Cache[T any] struct {
	mu     sync.RWMutex
	logger log.MinInterface
	data   map[string]any
}

// NewCache creates a new cache.
func NewCache[T any]() *Cache[T] {
	logger := log.Default()

	return &Cache[T]{
		logger: logger,
		data:   make(map[string]any),
	}
}

// Get retrieves a value from the cache.
// Returns ErrCacheNotFound if the key doesn't exist.
func (c *Cache[T]) Get(key string) (any, error) {
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
func (c *Cache[T]) Set(key string, data T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = data
}
