// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/manager"
	"github.com/cloudoperators/rootstock/internal/rawdata"
	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/pkg/log"
)

const (
	pathRackPDUs    = "/redfish/v1/PowerEquipment/RackPDUs"
	pathPowerRating = "/redfish/v1/PowerEquipment/RackPDUs/1"
	pathEnergetics  = "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics"
	pathMains       = "/redfish/v1/PowerEquipment/RackPDUs/1/Mains"
	pathMainCurrent = "/redfish/v1/PowerEquipment/RackPDUs/1/Mains/AC1"
	pathOutlets     = "/redfish/v1/PowerEquipment/RackPDUs/1/Outlets"
	pathOutletA1    = "/redfish/v1/PowerEquipment/RackPDUs/1/Outlets/A1"
	pathBranches    = "/redfish/v1/PowerEquipment/RackPDUs/1/Branches"
	pathBranchA     = "/redfish/v1/PowerEquipment/RackPDUs/1/Branches/A"
)

func jsonHandler(body any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func sessionDeleteHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// fullHandlers returns a complete set of valid Redfish responses for a single PDU.
func fullHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions":   loginHandler,
		"/redfish/v1/SessionService/Sessions/1": sessionDeleteHandler,
		pathRackPDUs: jsonHandler(map[string]any{
			"Members": []any{map[string]any{"@odata.id": pathPowerRating}},
		}),
		pathPowerRating: jsonHandler(map[string]any{
			"Id": "1", "PowerCapacityVA": 5000.0,
			"Manufacturer": "PANDUIT", "Model": "XYZ",
		}),
		pathEnergetics: jsonHandler(map[string]any{
			"PowerWatts": map[string]any{"Reading": 442.0, "ApparentVA": 444.0, "ReactiveVAR": 42.0, "PowerFactor": 0.99},
			"EnergykWh":  map[string]any{"Reading": 1234.0},
		}),
		pathMains: jsonHandler(map[string]any{
			"Members": []any{map[string]any{"@odata.id": pathMainCurrent}},
		}),
		pathMainCurrent: jsonHandler(map[string]any{
			"PolyPhaseVoltage":     []any{map[string]any{"Line1ToNeutral": map[string]any{"Reading": 230.0}}},
			"PolyPhaseCurrentAmps": []any{map[string]any{"Line1": map[string]any{"Reading": 1.9}}},
			"PolyPhasePowerWatts": []any{map[string]any{"Line1": map[string]any{
				"Reading": 442.0, "ApparentVA": 444.0, "ReactiveVAR": 42.0, "PowerFactor": 0.99,
			}}},
		}),
		pathOutlets: jsonHandler(map[string]any{
			"Outlets": []any{map[string]any{"@odata.id": pathOutletA1}},
		}),
		pathOutletA1: jsonHandler(map[string]any{
			"Status":           map[string]any{"State": "Enabled"},
			"OutletType":       "NEMA",
			"RatedCurrentAmps": 16.0,
			"Voltage":          map[string]any{"Reading": 230.0},
			"CurrentAmps":      map[string]any{"Reading": 1.5},
			"PowerWatts":       map[string]any{"Reading": 345.0},
		}),
		pathBranches: jsonHandler(map[string]any{
			"Branch": []any{map[string]any{"@odata.id": pathBranchA}},
		}),
		pathBranchA: jsonHandler(map[string]any{
			"RatedCurrentAmps":    32.0,
			"CircuitType":         "Branch",
			"CurrentAmps":         map[string]any{"Reading": 5.0},
			"PolyPhaseVoltage":    map[string]any{"Line1": map[string]any{"Reading": 230.0}},
			"PolyPhasePowerWatts": map[string]any{"Line1": map[string]any{"Reading": 442.0, "ApparentVA": 444.0}},
		}),
	}
}

func setupTestTLSServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}

	return httptest.NewTLSServer(mux)
}

// findMetric returns the first metric with the given key, or false if absent.
func findMetric(metrics []rawdata.RawMetric, key string) (rawdata.RawMetric, bool) {
	for _, m := range metrics {
		if m.Key == key {
			return m, true
		}
	}

	return rawdata.RawMetric{}, false
}

func TestScrape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filter     redfish.Filter
		breakPath  string // if set, this endpoint returns 500
		noToken    bool   // if true, login omits the auth token
		wantErr    bool
		wantStatus int
		wantCount  int // expected number of metrics, checked only when wantErr is false
	}{
		{
			name:       "empty filter passes every number and no metadata",
			filter:     redfish.Filter{},
			wantStatus: http.StatusOK,
			wantCount:  21,
		},
		{
			name:       "default filter keeps readings plus metadata info metrics",
			filter:     redfish.DefaultFilter(),
			wantStatus: http.StatusOK,
			wantCount:  14,
		},
		{
			name:       "login failure is fatal",
			noToken:    true,
			wantErr:    true,
			wantStatus: 0,
		},
		{
			name:       "rack PDUs listing failure is fatal",
			breakPath:  pathRackPDUs,
			wantErr:    true,
			wantStatus: 0,
		},
		{
			name:       "broken metrics document is skipped, not fatal",
			filter:     redfish.Filter{},
			breakPath:  pathEnergetics,
			wantStatus: http.StatusOK,
			wantCount:  16, // 21 minus the 5 Metrics numbers
		},
		{
			name:       "broken outlet document is skipped, not fatal",
			filter:     redfish.Filter{},
			breakPath:  pathOutletA1,
			wantStatus: http.StatusOK,
			wantCount:  17, // 21 minus the 4 Outlet numbers
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handlers := fullHandlers()

			if tc.noToken {
				handlers["/redfish/v1/SessionService/Sessions"] = func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
					w.WriteHeader(http.StatusCreated)
				}
			}

			if tc.breakPath != "" {
				handlers[tc.breakPath] = func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}

			server := setupTestTLSServer(t, handlers)
			defer server.Close()

			out := make(chan []rawdata.RawMetric, 1)
			scrape := redfish.NewScraper(out, log.NewMockLogger(), 5*time.Second, tc.filter)

			job := manager.ScrapeJob{
				IP:       strings.TrimPrefix(server.URL, "https://"),
				Username: "user",
				Password: "pass",
				Protocol: "redfish",
			}

			status, err := scrape(context.Background(), job)

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tc.wantStatus, status)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, status)

			metrics := <-out
			assert.Len(t, metrics, tc.wantCount)

			for _, m := range metrics {
				assert.Equal(t, "redfish", m.Protocol)
				assert.Equal(t, job.IP, m.DeviceIP)
				assert.False(t, m.Timestamp.IsZero())
				assert.Equal(t, "1", m.Labels["pdu_id"])
			}
		})
	}
}

func TestScrapeFilterEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filter      redfish.Filter
		key         string
		wantPresent bool
		checkValue  bool
		wantValue   float64
		wantLabels  map[string]string // subset-checked when non-nil
	}{
		{
			name:        "empty filter keeps a top-level number",
			filter:      redfish.Filter{},
			key:         "PowerCapacityVA",
			wantPresent: true,
			checkValue:  true,
			wantValue:   5000.0,
		},
		{
			name:        "empty filter keeps a prefixed reading",
			filter:      redfish.Filter{},
			key:         "Metrics/EnergykWh/Reading",
			wantPresent: true,
			checkValue:  true,
			wantValue:   1234.0,
		},
		{
			name:        "empty filter keeps a secondary electrical field",
			filter:      redfish.Filter{},
			key:         "Metrics/PowerWatts/ApparentVA",
			wantPresent: true,
		},
		{
			name:        "empty filter keeps a nested poly-phase reading",
			filter:      redfish.Filter{},
			key:         "Mains/PolyPhaseVoltage/Line1ToNeutral/Reading",
			wantPresent: true,
		},
		{
			name:        "empty filter emits no info metric",
			filter:      redfish.Filter{},
			key:         "info",
			wantPresent: false,
		},
		{
			name:        "default filter keeps readings",
			filter:      redfish.DefaultFilter(),
			key:         "Metrics/EnergykWh/Reading",
			wantPresent: true,
		},
		{
			name:        "default filter drops a top-level config number",
			filter:      redfish.DefaultFilter(),
			key:         "PowerCapacityVA",
			wantPresent: false,
		},
		{
			name:        "default filter drops a secondary electrical field",
			filter:      redfish.DefaultFilter(),
			key:         "Metrics/PowerWatts/ApparentVA",
			wantPresent: false,
		},
		{
			name:        "default filter drops an outlet config number",
			filter:      redfish.DefaultFilter(),
			key:         "Outlets/RatedCurrentAmps",
			wantPresent: false,
		},
		{
			name:        "default filter emits a PDU info metric with identity labels",
			filter:      redfish.DefaultFilter(),
			key:         "info",
			wantPresent: true,
			checkValue:  true,
			wantValue:   1.0,
			wantLabels:  map[string]string{"pdu_id": "1", "Manufacturer": "PANDUIT", "Model": "XYZ"},
		},
		{
			name:        "default filter emits a per-outlet info metric",
			filter:      redfish.DefaultFilter(),
			key:         "Outlets/info",
			wantPresent: true,
			wantLabels:  map[string]string{"outlet_id": "A1", "OutletType": "NEMA", "State": "Enabled"},
		},
		{
			name:        "default filter emits a per-branch info metric",
			filter:      redfish.DefaultFilter(),
			key:         "Branches/info",
			wantPresent: true,
			wantLabels:  map[string]string{"branch_id": "A", "CircuitType": "Branch"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			metric, ok := findMetric(runScrape(t, tc.filter), tc.key)
			require.Equal(t, tc.wantPresent, ok)

			if !tc.wantPresent {
				return
			}

			if tc.checkValue {
				assert.InDelta(t, tc.wantValue, metric.Value, 1e-6)
			}

			for label, want := range tc.wantLabels {
				assert.Equal(t, want, metric.Labels[label])
			}
		})
	}
}

func runScrape(t *testing.T, filter redfish.Filter) []rawdata.RawMetric {
	t.Helper()

	server := setupTestTLSServer(t, fullHandlers())
	t.Cleanup(server.Close)

	out := make(chan []rawdata.RawMetric, 1)
	scrape := redfish.NewScraper(out, log.NewMockLogger(), 5*time.Second, filter)

	job := manager.ScrapeJob{
		IP:       strings.TrimPrefix(server.URL, "https://"),
		Username: "user",
		Password: "pass",
		Protocol: "redfish",
	}

	status, err := scrape(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	return <-out
}
