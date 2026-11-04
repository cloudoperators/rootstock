// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package performance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudoperators/rootstock/internal/collectors/cache"
	"github.com/cloudoperators/rootstock/test/performance"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

// TestRedfishPipeline runs the end-to-end Redfish test case: it scrapes the
// mocked PDUs, runs them through the processing layer, and validates the results
// by reading them back from the result cache.
//
// Component chain:
//
//	testdata/pdu-mockup Redfish fixture
//	  -> mockpdu.Client (as redfish.Client)              [mock PDUs]
//	  -> performance.Scrape (GetPERackPDU + Metrics)     [scraping edge]
//	  -> raw data queue                                  [processing layer]
//	  -> data-processor                                  [processing layer]
//	  -> cache.Cache[Result] (result cache)              [read-out]
//
// The Redfish scenario is currently not completely implemented: the data
// processors are not available, so the test scrapes the mocked PDUs, confirms
// the result cache is empty, and reports the missing processing layer via
// t.Fatal.
func TestRedfishPipeline(t *testing.T) {
	ctx := context.Background()

	// Setup: initialize the mock PDU source and connect it to the scraper.
	source, err := mockpdu.New()
	if err != nil {
		t.Fatalf("build mock PDU source: %v", err)
	}

	ids := source.IDs()
	if len(ids) == 0 {
		t.Fatal("mock PDU source exposed no PDUs")
	}

	// Result cache: the read-out layer.
	resultCache := cache.New[performance.Result]()

	for _, id := range ids {
		// Scraping edge.
		pdu, metrics, err := performance.Scrape(ctx, source, id)
		if err != nil {
			t.Fatalf("scrape %q: %v", id, err)
		}

		if pdu == nil || metrics == nil {
			t.Fatalf("scrape %q returned nil documents", id)
		}

		// Processing layer: push (pdu, metrics) onto the raw data queue, have
		// the data-processor transform them into a performance.Result and store
		// it with resultCache.Set(id, result). The data processors are not
		// available.
		_ = pdu
		_ = metrics
	}

	// Validate by reading out the result cache. Each Result is read back and
	// checked against the fixture values, e.g.
	//
	//	got, err := performance.ReadResult(resultCache, id)
	//	require.NoError(t, err)
	//	require.Equal(t, metrics.ID, got.PDUID)
	//	require.Equal(t, metrics.PowerWatts.Reading, got.PowerWatts)
	//	require.Equal(t, pdu.PowerCapacityVA, got.PowerCapacityVA)
	//	... (ApparentVA, ReactiveVAR, PowerFactor, EnergykWh)
	//
	// The result cache is empty, so the read-out returns a cache miss.
	if _, err := performance.ReadResult(resultCache, ids[0]); !errors.Is(err, cache.ErrCacheNotFound) {
		t.Fatalf("expected empty result cache (cache miss), got err=%v", err)
	}

	t.Fatal("data processors not available: the Redfish scenario does not run end to end")
}
