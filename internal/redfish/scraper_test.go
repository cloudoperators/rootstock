// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
			"EquipmentType": "RackPDU", "Name": "PDU-1",
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

// TODO: rewrite tests once scraper and model are redesigned.
func TestPlaceholder(t *testing.T) {
	t.Skip("scraper tests will be rewritten with the new design")
}
