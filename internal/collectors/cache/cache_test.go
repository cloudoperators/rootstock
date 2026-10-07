// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"testing"
)

func TestCache_SetAndGet(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	pduID := "pdu-1"

	// Create test metric data
	testData := &MetricData{
		PERackPDUMetrics: &PERackPDUMetrics{
			DataSourceURI: "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics",
			Reading:       1234.56,
			ApparentVA:    1300.0,
			ReactiveVAR:   200.0,
			PowerFactor:   0.95,
		},
		PERackPDU: &PERackPDU{
			PowerCapacityVA: 5000.0,
		},
	}

	// Test Set
	cache.Set(pduID, testData)

	// Test Get
	retrieved, err := cache.Get(pduID)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil")
	}

	// Verify PERackPDUMetrics
	if retrieved.PERackPDUMetrics == nil {
		t.Fatal("retrieved.PERackPDUMetrics is nil")
	}
	if retrieved.PERackPDUMetrics.DataSourceURI != testData.PERackPDUMetrics.DataSourceURI {
		t.Errorf("DataSourceURI = %v, want %v",
			retrieved.PERackPDUMetrics.DataSourceURI,
			testData.PERackPDUMetrics.DataSourceURI)
	}
	if retrieved.PERackPDUMetrics.Reading != testData.PERackPDUMetrics.Reading {
		t.Errorf("Reading = %v, want %v",
			retrieved.PERackPDUMetrics.Reading,
			testData.PERackPDUMetrics.Reading)
	}
	if retrieved.PERackPDUMetrics.ApparentVA != testData.PERackPDUMetrics.ApparentVA {
		t.Errorf("ApparentVA = %v, want %v",
			retrieved.PERackPDUMetrics.ApparentVA,
			testData.PERackPDUMetrics.ApparentVA)
	}
	if retrieved.PERackPDUMetrics.ReactiveVAR != testData.PERackPDUMetrics.ReactiveVAR {
		t.Errorf("ReactiveVAR = %v, want %v",
			retrieved.PERackPDUMetrics.ReactiveVAR,
			testData.PERackPDUMetrics.ReactiveVAR)
	}
	if retrieved.PERackPDUMetrics.PowerFactor != testData.PERackPDUMetrics.PowerFactor {
		t.Errorf("PowerFactor = %v, want %v",
			retrieved.PERackPDUMetrics.PowerFactor,
			testData.PERackPDUMetrics.PowerFactor)
	}

	// Verify PERackPDU
	if retrieved.PERackPDU == nil {
		t.Fatal("retrieved.PERackPDU is nil")
	}
	if retrieved.PERackPDU.PowerCapacityVA != testData.PERackPDU.PowerCapacityVA {
		t.Errorf("PowerCapacityVA = %v, want %v",
			retrieved.PERackPDU.PowerCapacityVA,
			testData.PERackPDU.PowerCapacityVA)
	}
}

func TestCache_GetNonExistentKey(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	// Get non-existent key
	retrieved, err := cache.Get("non-existent")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	// Should return nil for non-existent key
	if retrieved != nil {
		t.Error("Expected nil for non-existent key")
	}
}

func TestCache_Update(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	pduID := "pdu-1"

	// Initial data
	initialData := &MetricData{
		PERackPDUMetrics: &PERackPDUMetrics{
			Reading: 100.0,
		},
	}
	cache.Set(pduID, initialData)

	// Update with new data
	updatedData := &MetricData{
		PERackPDUMetrics: &PERackPDUMetrics{
			Reading: 200.0,
		},
	}
	cache.Set(pduID, updatedData)

	// Verify update
	retrieved, err := cache.Get(pduID)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if retrieved.PERackPDUMetrics.Reading != 200.0 {
		t.Errorf("Reading = %v, want 200.0", retrieved.PERackPDUMetrics.Reading)
	}
}

func TestCache_MultipleKeys(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	// Add multiple PDUs
	pdus := map[string]float64{
		"pdu-1": 100.0,
		"pdu-2": 200.0,
		"pdu-3": 300.0,
	}

	for pduID, reading := range pdus {
		data := &MetricData{
			PERackPDUMetrics: &PERackPDUMetrics{
				Reading: reading,
			},
		}
		cache.Set(pduID, data)
	}

	// Verify all PDUs
	for pduID, expectedReading := range pdus {
		retrieved, err := cache.Get(pduID)
		if err != nil {
			t.Fatalf("Get(%s) returned error: %v", pduID, err)
		}

		if retrieved.PERackPDUMetrics.Reading != expectedReading {
			t.Errorf("PDU %s: Reading = %v, want %v",
				pduID,
				retrieved.PERackPDUMetrics.Reading,
				expectedReading)
		}
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	pduID := "pdu-concurrent"

	var wg sync.WaitGroup
	concurrency := 100

	// Concurrent writes
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(value float64) {
			defer wg.Done()
			data := &MetricData{
				PERackPDUMetrics: &PERackPDUMetrics{
					Reading: value,
				},
			}
			cache.Set(pduID, data)
		}(float64(i))
	}
	wg.Wait()

	// Verify cache is still functional
	retrieved, err := cache.Get(pduID)
	if err != nil {
		t.Fatalf("Get() after concurrent writes returned error: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil after concurrent writes")
	}

	// Just verify we got some value (any of the concurrent writes)
	if retrieved.PERackPDUMetrics == nil {
		t.Error("PERackPDUMetrics is nil after concurrent writes")
	}
}

func TestCache_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	var wg sync.WaitGroup
	iterations := 100

	// Initialize some data
	for i := 0; i < 10; i++ {
		pduID := "pdu-" + string(rune('0'+i))
		data := &MetricData{
			PERackPDUMetrics: &PERackPDUMetrics{
				Reading: float64(i * 100),
			},
		}
		cache.Set(pduID, data)
	}

	// Concurrent reads and writes
	wg.Add(iterations * 2)

	// Readers
	for i := 0; i < iterations; i++ {
		go func(idx int) {
			defer wg.Done()
			pduID := "pdu-" + string(rune('0'+(idx%10)))
			_, err := cache.Get(pduID)
			if err != nil {
				t.Errorf("Concurrent Get() failed: %v", err)
			}
		}(i)
	}

	// Writers
	for i := 0; i < iterations; i++ {
		go func(idx int) {
			defer wg.Done()
			pduID := "pdu-" + string(rune('0'+(idx%10)))
			data := &MetricData{
				PERackPDUMetrics: &PERackPDUMetrics{
					Reading: float64(idx),
				},
			}
			cache.Set(pduID, data)
		}(i)
	}

	wg.Wait()

	// Verify cache integrity
	for i := 0; i < 10; i++ {
		pduID := "pdu-" + string(rune('0'+i))
		retrieved, err := cache.Get(pduID)
		if err != nil {
			t.Errorf("Final Get(%s) failed: %v", pduID, err)
		}
		if retrieved == nil {
			t.Errorf("Final Get(%s) returned nil", pduID)
		}
	}
}
