// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"strconv"
	"testing"
)

// BenchmarkCacheSet measures the cost of writing to the result cache, the final
// stage of the processing pipeline.
func BenchmarkCacheSet(b *testing.B) {
	c := New[int]()

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		c.Set("key", i)
	}
}

// BenchmarkCacheGet measures the cost of reading a hit from the result cache.
func BenchmarkCacheGet(b *testing.B) {
	c := New[int]()
	c.Set("key", 42)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, _ = c.Get("key")
	}
}

// BenchmarkCacheSetParallel measures write contention under concurrent access,
// reflecting many scrapers writing results simultaneously ("horizontal
// scaling"). Each goroutine writes to a distinct key to exercise the map rather
// than a single hot entry.
func BenchmarkCacheSetParallel(b *testing.B) {
	c := New[int]()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			c.Set(strconv.Itoa(i%1024), i)

			i++
		}
	})
}

// BenchmarkCacheGetParallel measures read contention under concurrent access.
func BenchmarkCacheGetParallel(b *testing.B) {
	c := New[int]()
	for i := range 1024 {
		c.Set(strconv.Itoa(i), i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			_, _ = c.Get(strconv.Itoa(i % 1024))

			i++
		}
	})
}
