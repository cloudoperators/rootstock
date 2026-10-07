// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
)

func TestCollector_Describe(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	collector := NewCollector("pdu-test", c)

	ch := make(chan *prometheus.Desc, 10)
	done := make(chan struct{})

	go func() {
		collector.Describe(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Count descriptions
	count := 0
	for range ch {
		count++
	}

	expectedCount := 5 // Reading, ApparentVA, ReactiveVAR, PowerFactor, PowerCapacityVA
	if count != expectedCount {
		t.Errorf("Describe sent %d descriptors, want %d", count, expectedCount)
	}
}

func TestCollector_Collect_EmptyCache(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	collector := NewCollector("pdu-nonexistent", c)

	ch := make(chan prometheus.Metric, 10)
	done := make(chan struct{})

	go func() {
		collector.Collect(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Should emit no metrics when cache is empty
	count := 0
	for range ch {
		count++
	}

	if count != 0 {
		t.Errorf("Collect emitted %d metrics for empty cache, want 0", count)
	}
}

func TestCollector_Collect_WithMetrics(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	pduID := "pdu-1"

	// Populate cache
	testData := &cache.MetricData{
		PERackPDUMetrics: &cache.PERackPDUMetrics{
			DataSourceURI: "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics",
			Reading:       1234.56,
			ApparentVA:    1300.0,
			ReactiveVAR:   200.0,
			PowerFactor:   0.95,
		},
		PERackPDU: &cache.PERackPDU{
			PowerCapacityVA: 5000.0,
		},
	}
	c.Set(pduID, testData)

	collector := NewCollector(pduID, c)

	ch := make(chan prometheus.Metric, 10)
	done := make(chan struct{})

	go func() {
		collector.Collect(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Collect all metrics
	metrics := make([]prometheus.Metric, 0)
	for m := range ch {
		metrics = append(metrics, m)
	}

	expectedCount := 5 // All metrics have non-zero values
	if len(metrics) != expectedCount {
		t.Errorf("Collect emitted %d metrics, want %d", len(metrics), expectedCount)
	}

	// Verify metric values
	for _, metric := range metrics {
		m := &dto.Metric{}
		if err := metric.Write(m); err != nil {
			t.Fatalf("Failed to write metric: %v", err)
		}

		if m.Gauge == nil {
			t.Error("Expected gauge metric")
			continue
		}

		// Check label
		if len(m.Label) != 1 {
			t.Errorf("Expected 1 label, got %d", len(m.Label))
			continue
		}

		if m.Label[0].GetName() != "pdu_id" {
			t.Errorf("Expected label name 'pdu_id', got %s", m.Label[0].GetName())
		}

		if m.Label[0].GetValue() != pduID {
			t.Errorf("Expected label value '%s', got %s", pduID, m.Label[0].GetValue())
		}
	}
}

func TestCollector_Collect_WithZeroValues(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	pduID := "pdu-zero"

	// Populate cache with zero values
	testData := &cache.MetricData{
		PERackPDUMetrics: &cache.PERackPDUMetrics{
			Reading:     0.0,
			ApparentVA:  0.0,
			ReactiveVAR: 0.0,
			PowerFactor: 0.0,
		},
		PERackPDU: &cache.PERackPDU{
			PowerCapacityVA: 0.0,
		},
	}
	c.Set(pduID, testData)

	collector := NewCollector(pduID, c)

	ch := make(chan prometheus.Metric, 10)
	done := make(chan struct{})

	go func() {
		collector.Collect(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Should emit no metrics when all values are zero
	count := 0
	for range ch {
		count++
	}

	if count != 0 {
		t.Errorf("Collect emitted %d metrics for zero values, want 0", count)
	}
}

func TestCollector_Collect_WithPartialMetrics(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	pduID := "pdu-partial"

	// Populate cache with only some metrics
	testData := &cache.MetricData{
		PERackPDUMetrics: &cache.PERackPDUMetrics{
			Reading:     1234.56,
			ApparentVA:  0.0, // Zero - should not emit
			ReactiveVAR: 200.0,
			PowerFactor: 0.0, // Zero - should not emit
		},
		PERackPDU: &cache.PERackPDU{
			PowerCapacityVA: 5000.0,
		},
	}
	c.Set(pduID, testData)

	collector := NewCollector(pduID, c)

	ch := make(chan prometheus.Metric, 10)
	done := make(chan struct{})

	go func() {
		collector.Collect(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Count metrics
	count := 0
	for range ch {
		count++
	}

	expectedCount := 3 // Reading, ReactiveVAR, PowerCapacityVA
	if count != expectedCount {
		t.Errorf("Collect emitted %d metrics, want %d", count, expectedCount)
	}
}

func TestCollector_Collect_NilPERackPDU(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	pduID := "pdu-nil"

	// Populate cache with nil PERackPDU
	testData := &cache.MetricData{
		PERackPDUMetrics: &cache.PERackPDUMetrics{
			Reading: 1234.56,
		},
		PERackPDU: nil, // Nil - should not panic
	}
	c.Set(pduID, testData)

	collector := NewCollector(pduID, c)

	ch := make(chan prometheus.Metric, 10)
	done := make(chan struct{})

	go func() {
		collector.Collect(ch)
		close(ch)
		close(done)
	}()

	<-done

	// Should only emit Reading metric
	count := 0
	for range ch {
		count++
	}

	expectedCount := 1 // Only Reading
	if count != expectedCount {
		t.Errorf("Collect emitted %d metrics, want %d", count, expectedCount)
	}
}

func TestCollector_ThreadSafety(t *testing.T) {
	t.Parallel()

	c := cache.NewCache()
	pduID := "pdu-concurrent"

	testData := &cache.MetricData{
		PERackPDUMetrics: &cache.PERackPDUMetrics{
			Reading: 1234.56,
		},
	}
	c.Set(pduID, testData)

	collector := NewCollector(pduID, c)

	// Run multiple Collect calls concurrently
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			ch := make(chan prometheus.Metric, 10)
			collector.Collect(ch)
			close(ch)
			// Drain channel
			for range ch {
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
