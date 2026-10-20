// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"testing"
)

func TestCache_SetAndGetRackPDU(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	pduID := "pdu-1"

	// Create test metric data
	testData := &RackPDUMetricData{
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

	// Test SetRackPDU
	cache.SetRackPDU(pduID, testData)

	// Test GetRackPDU
	retrieved, err := cache.GetRackPDU(pduID)
	if err != nil {
		t.Fatalf("GetRackPDU() returned error: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetRackPDU() returned nil")
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
	retrieved, err := cache.GetRackPDU("non-existent")
	if err != nil {
		t.Fatalf("GetRackPDU() returned error: %v", err)
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
	initialData := &RackPDUMetricData{
		PERackPDUMetrics: &PERackPDUMetrics{
			Reading: 100.0,
		},
	}
	cache.SetRackPDU(pduID, initialData)

	// Update with new data
	updatedData := &RackPDUMetricData{
		PERackPDUMetrics: &PERackPDUMetrics{
			Reading: 200.0,
		},
	}
	cache.SetRackPDU(pduID, updatedData)

	// Verify update
	retrieved, err := cache.GetRackPDU(pduID)
	if err != nil {
		t.Fatalf("GetRackPDU() returned error: %v", err)
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
		data := &RackPDUMetricData{
			PERackPDUMetrics: &PERackPDUMetrics{
				Reading: reading,
			},
		}
		cache.SetRackPDU(pduID, data)
	}

	// Verify all PDUs
	for pduID, expectedReading := range pdus {
		retrieved, err := cache.GetRackPDU(pduID)
		if err != nil {
			t.Fatalf("GetRackPDU(%s) returned error: %v", pduID, err)
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

	for i := range concurrency {
		go func(value float64) {
			defer wg.Done()

			data := &RackPDUMetricData{
				PERackPDUMetrics: &PERackPDUMetrics{
					Reading: value,
				},
			}
			cache.SetRackPDU(pduID, data)
		}(float64(i))
	}

	wg.Wait()

	// Verify cache is still functional
	retrieved, err := cache.GetRackPDU(pduID)
	if err != nil {
		t.Fatalf("GetRackPDU() after concurrent writes returned error: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetRackPDU() returned nil after concurrent writes")
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
	for i := range 10 {
		pduID := "pdu-" + string(rune('0'+i))
		data := &RackPDUMetricData{
			PERackPDUMetrics: &PERackPDUMetrics{
				Reading: float64(i * 100),
			},
		}
		cache.SetRackPDU(pduID, data)
	}

	// Concurrent reads and writes
	wg.Add(iterations * 2)

	// Readers
	for i := range iterations {
		go func(idx int) {
			defer wg.Done()

			pduID := "pdu-" + string(rune('0'+(idx%10)))

			_, err := cache.GetRackPDU(pduID)
			if err != nil {
				t.Errorf("Concurrent GetRackPDU() failed: %v", err)
			}
		}(i)
	}

	// Writers
	for i := range iterations {
		go func(idx int) {
			defer wg.Done()

			pduID := "pdu-" + string(rune('0'+(idx%10)))
			data := &RackPDUMetricData{
				PERackPDUMetrics: &PERackPDUMetrics{
					Reading: float64(idx),
				},
			}
			cache.SetRackPDU(pduID, data)
		}(i)
	}

	wg.Wait()

	// Verify cache integrity
	for i := range 10 {
		pduID := "pdu-" + string(rune('0'+i))

		retrieved, err := cache.GetRackPDU(pduID)
		if err != nil {
			t.Errorf("Final GetRackPDU(%s) failed: %v", pduID, err)
		}

		if retrieved == nil {
			t.Errorf("Final GetRackPDU(%s) returned nil", pduID)
		}
	}
}

func TestCache_EnvironmentalSensorsData(t *testing.T) {
	t.Parallel()

	cache := NewCache()
	sensorID := "sensor-1"

	// Create test environmental sensor data
	testData := &EnvironmentalSensorsData{
		// Currently empty as per TODO in cache.go
	}

	// Test Set
	cache.SeEnvironmentalSensorsData(sensorID, testData)

	// Test Get
	retrieved, err := cache.GeEnvironmentalSensorsData(sensorID)
	if err != nil {
		t.Fatalf("GeEnvironmentalSensorsData() returned error: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GeEnvironmentalSensorsData() returned nil")
	}
}

func TestCache_EnvironmentalSensorsData_NonExistent(t *testing.T) {
	t.Parallel()

	cache := NewCache()

	// Get non-existent sensor
	retrieved, err := cache.GeEnvironmentalSensorsData("non-existent")
	if err != nil {
		t.Fatalf("GeEnvironmentalSensorsData() returned error: %v", err)
	}

	// Should return nil for non-existent key
	if retrieved != nil {
		t.Error("Expected nil for non-existent sensor key")
	}
}
