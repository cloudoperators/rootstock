// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/pkg/log"
)

func setupTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}

	return httptest.NewServer(mux)
}

func newTestClient(t *testing.T, serverURL string) redfish.Client {
	t.Helper()

	base, err := url.Parse(serverURL)
	require.NoError(t, err)

	return redfish.NewClient(base, log.NewMockLogger(), "testuser", "testpass", 5*time.Second)
}

func loginHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Auth-Token", "test-token")
	w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
	w.WriteHeader(http.StatusCreated)
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "valid url", baseURL: "https://pdu.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base, err := url.Parse(tc.baseURL)
			require.NoError(t, err)

			client := redfish.NewClient(base, log.NewMockLogger(), "user", "pass", 10*time.Second)
			assert.NotNil(t, client)
		})
	}
}

func TestLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		responseToken    string
		responseLocation string
		responseStatus   int
		wantErr          bool
	}{
		{
			name:             "success",
			responseToken:    "test-token-123",
			responseLocation: "/redfish/v1/SessionService/Sessions/1",
			responseStatus:   http.StatusCreated,
			wantErr:          false,
		},
		{
			name:             "missing token",
			responseToken:    "",
			responseLocation: "/redfish/v1/SessionService/Sessions/1",
			responseStatus:   http.StatusCreated,
			wantErr:          true,
		},
		{
			name:             "missing location",
			responseToken:    "test-token-123",
			responseLocation: "",
			responseStatus:   http.StatusCreated,
			wantErr:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
					if tc.responseToken != "" {
						w.Header().Set("X-Auth-Token", tc.responseToken)
					}
					if tc.responseLocation != "" {
						w.Header().Set("Location", tc.responseLocation)
					}
					w.WriteHeader(tc.responseStatus)
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.Login(context.Background())

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		loginFirst  bool
		wantDeleted bool
	}{
		{
			name:        "closes session after login",
			loginFirst:  true,
			wantDeleted: true,
		},
		{
			name:        "no-op without session",
			loginFirst:  false,
			wantDeleted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deleteCalled := false

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/SessionService/Sessions/1": func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						deleteCalled = true
					}
					w.WriteHeader(http.StatusOK)
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			ctx := context.Background()

			if tc.loginFirst {
				require.NoError(t, client.Login(ctx))
			}

			require.NoError(t, client.Close(ctx))
			assert.Equal(t, tc.wantDeleted, deleteCalled)
		})
	}
}

func TestGetPowerEquipment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantName     string
	}{
		{
			name:         "success",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"Name": "Power Equipment"},
			wantErr:      false,
			wantName:     "Power Equipment",
		},
		{
			name:         "not found",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetPowerEquipment(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, result.Name)
		})
	}
}

func TestGetRackPDUs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		serverStatus     int
		serverBody       map[string]any
		wantErr          bool
		wantMembersCount int
		wantMemberIDs    []string
	}{
		{
			name:         "success",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Name":                "RackPDU Collection",
				"Members@odata.count": 2,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1"},
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/2"},
				},
			},
			wantErr:          false,
			wantMembersCount: 2,
			wantMemberIDs:    []string{"/redfish/v1/PowerEquipment/RackPDUs/1", "/redfish/v1/PowerEquipment/RackPDUs/2"},
		},
		{
			name:         "not found",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUs(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantMembersCount, result.MembersCount)

			gotIDs := make([]string, len(result.Members))
			for i, m := range result.Members {
				gotIDs[i] = m.ODataID
			}
			assert.Equal(t, tc.wantMemberIDs, gotIDs)
		})
	}
}

func TestGetRackPDUPowerRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pduID        string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantName     string
		wantCapacity float64
		wantState    string
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Id":              "1",
				"Name":            "PDU-01",
				"PowerCapacityVA": 7200.0,
				"Status":          map[string]string{"State": "Enabled", "Health": "OK"},
			},
			wantErr:      false,
			wantName:     "PDU-01",
			wantCapacity: 7200.0,
			wantState:    "Enabled",
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUPowerRating(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, result.Name)
			assert.InDelta(t, tc.wantCapacity, result.PowerCapacityVA, 1e-6)
			assert.Equal(t, tc.wantState, result.Status.State)
		})
	}
}

func TestGetRackPDUEnergetics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pduID         string
		serverStatus  int
		serverBody    map[string]any
		wantErr       bool
		wantPower     float64
		wantApparent  float64
		wantReactive  float64
		wantPwrFactor float64
		wantEnergykWh float64
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Id":   "Metrics",
				"Name": "PDU-01 Metrics",
				"PowerWatts": map[string]float64{
					"Reading":     3500.5,
					"ApparentVA":  3600.0,
					"ReactiveVAR": 150.0,
					"PowerFactor": 0.97,
				},
				"EnergykWh": map[string]float64{
					"Reading": 12345.67,
				},
			},
			wantErr:       false,
			wantPower:     3500.5,
			wantApparent:  3600.0,
			wantReactive:  150.0,
			wantPwrFactor: 0.97,
			wantEnergykWh: 12345.67,
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Metrics": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUEnergetics(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.wantPower, result.PowerWatts.Power, 1e-6)
			assert.InDelta(t, tc.wantApparent, result.PowerWatts.ApparentPower, 1e-6)
			assert.InDelta(t, tc.wantReactive, result.PowerWatts.ReactivePower, 1e-6)
			assert.InDelta(t, tc.wantPwrFactor, result.PowerWatts.PowerFactor, 1e-6)
			assert.InDelta(t, tc.wantEnergykWh, result.EnergykWh.EnergykWh, 1e-6)
		})
	}
}

func TestGetRackPDUOutletElectricals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		pduID            string
		outletID         string
		serverStatus     int
		serverBody       map[string]any
		wantErr          bool
		wantState        string
		wantOutletType   string
		wantRatedCurrent float64
		wantVoltage      float64
		wantCurrent      float64
		wantPower        float64
	}{
		{
			name:         "success",
			pduID:        "1",
			outletID:     "OUTLET1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Status":           map[string]string{"State": "Enabled", "Health": "OK"},
				"OutletType":       "IEC_60320_C13",
				"RatedCurrentAmps": 16.0,
				"Voltage":          map[string]float64{"Reading": 231.0},
				"CurrentAmps":      map[string]float64{"Reading": 3.5},
				"PowerWatts":       map[string]float64{"Reading": 808.5, "ApparentVA": 808.5},
			},
			wantErr:          false,
			wantState:        "Enabled",
			wantOutletType:   "IEC_60320_C13",
			wantRatedCurrent: 16.0,
			wantVoltage:      231.0,
			wantCurrent:      3.5,
			wantPower:        808.5,
		},
		{
			name:         "not found",
			pduID:        "1",
			outletID:     "OUTLET999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			outletPath := "/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Outlets/" + tc.outletID
			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				outletPath: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUOutletElectricals(context.Background(), tc.pduID, tc.outletID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantState, result.Status.State)
			assert.Equal(t, tc.wantOutletType, result.OutletType)
			assert.InDelta(t, tc.wantRatedCurrent, result.RatedCurrent, 1e-6)
			assert.InDelta(t, tc.wantVoltage, result.Voltage.Voltage, 1e-6)
			assert.InDelta(t, tc.wantCurrent, result.Current.Current, 1e-6)
			assert.InDelta(t, tc.wantPower, result.ApparentPower.Power, 1e-6)
		})
	}
}

func TestGetRackPDUMainCurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pduID        string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantVoltage  []map[string]redfish.PhaseReading
		wantCurrent  []map[string]redfish.PhaseReading
		wantPower    []map[string]redfish.PowerWatts
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"PolyPhaseVoltage": []any{
					map[string]any{"Line1ToNeutral": map[string]float64{"Reading": 231.0}},
					map[string]any{"Line2ToNeutral": map[string]float64{"Reading": 231.0}},
					map[string]any{"Line3ToNeutral": map[string]float64{"Reading": 231.0}},
				},
				"PolyPhaseCurrentAmps": []any{
					map[string]any{"Line1": map[string]float64{"Reading": 6.68}},
					map[string]any{"Line2": map[string]float64{"Reading": 6.20}},
					map[string]any{"Line3": map[string]float64{"Reading": 5.96}},
				},
				"PolyPhasePowerWatts": []any{
					map[string]any{"Line1ToNeutral": map[string]float64{
						"Reading": 1535.0, "ApparentVA": 1544.0, "ReactiveVAR": 166.0, "PowerFactor": 0.994,
					}},
					map[string]any{"Line2ToNeutral": map[string]float64{
						"Reading": 1423.0, "ApparentVA": 1435.0, "ReactiveVAR": 185.0, "PowerFactor": 0.991,
					}},
					map[string]any{"Line3ToNeutral": map[string]float64{
						"Reading": 1370.0, "ApparentVA": 1379.0, "ReactiveVAR": 157.0, "PowerFactor": 0.993,
					}},
				},
			},
			wantErr: false,
			wantVoltage: []map[string]redfish.PhaseReading{
				{"Line1ToNeutral": {Reading: 231.0}},
				{"Line2ToNeutral": {Reading: 231.0}},
				{"Line3ToNeutral": {Reading: 231.0}},
			},
			wantCurrent: []map[string]redfish.PhaseReading{
				{"Line1": {Reading: 6.68}},
				{"Line2": {Reading: 6.20}},
				{"Line3": {Reading: 5.96}},
			},
			wantPower: []map[string]redfish.PowerWatts{
				{"Line1ToNeutral": {Power: 1535.0, ApparentPower: 1544.0, ReactivePower: 166.0, PowerFactor: 0.994}},
				{"Line2ToNeutral": {Power: 1423.0, ApparentPower: 1435.0, ReactivePower: 185.0, PowerFactor: 0.991}},
				{"Line3ToNeutral": {Power: 1370.0, ApparentPower: 1379.0, ReactivePower: 157.0, PowerFactor: 0.993}},
			},
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Mains/AC1": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUMainCurrent(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantVoltage, result.PolyPhaseVoltage)
			assert.Equal(t, tc.wantCurrent, result.PolyPhaseCurrentAmps)
			assert.Equal(t, tc.wantPower, result.PolyPhasePowerWatts)
		})
	}
}

func TestGetRackPDUBranchCurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		pduID            string
		branchID         string
		serverStatus     int
		serverBody       map[string]any
		wantErr          bool
		wantRatedCurrent float64
		wantCurrent      float64
		wantVoltage      map[string]redfish.PhaseReading
		wantPower        map[string]redfish.PowerWatts
	}{
		{
			name:         "success",
			pduID:        "1",
			branchID:     "A",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"RatedCurrentAmps": 32.0,
				"CurrentAmps":      map[string]float64{"Reading": 18.5},
				"PolyPhaseVoltage": map[string]any{
					"Line1ToNeutral": map[string]float64{"Reading": 231.0},
				},
				"PolyPhasePowerWatts": map[string]any{
					"Line1ToNeutral": map[string]float64{
						"Reading": 4273.5, "ApparentVA": 4281.0, "ReactiveVAR": 250.0, "PowerFactor": 0.998,
					},
				},
			},
			wantErr:          false,
			wantRatedCurrent: 32.0,
			wantCurrent:      18.5,
			wantVoltage: map[string]redfish.PhaseReading{
				"Line1ToNeutral": {Reading: 231.0},
			},
			wantPower: map[string]redfish.PowerWatts{
				"Line1ToNeutral": {Power: 4273.5, ApparentPower: 4281.0, ReactivePower: 250.0, PowerFactor: 0.998},
			},
		},
		{
			name:         "not found",
			pduID:        "1",
			branchID:     "Z",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			branchPath := "/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Branches/" + tc.branchID
			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				branchPath: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			result, err := client.GetRackPDUBranchCurrent(context.Background(), tc.pduID, tc.branchID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.wantRatedCurrent, result.RatedCurrent, 1e-6)
			assert.InDelta(t, tc.wantCurrent, result.Current.Current, 1e-6)
			assert.Equal(t, tc.wantVoltage, result.PolyPhaseVoltage)
			assert.Equal(t, tc.wantPower, result.PolyPhasePowerWatts)
		})
	}
}

func TestGetRackPDUOutletsIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pduID        string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantIDs      []string
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Outlets": []any{
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Outlets/OUTLET1"},
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Outlets/OUTLET2"},
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Outlets/OUTLET3"},
				},
			},
			wantErr: false,
			wantIDs: []string{"OUTLET1", "OUTLET2", "OUTLET3"},
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Outlets": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			ids, err := client.GetRackPDUOutletsIDs(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantIDs, ids)
		})
	}
}

func TestGetRackPDUBranchIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pduID        string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantIDs      []string
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Branch": []any{
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Branches/A"},
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Branches/B"},
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Branches/C"},
				},
			},
			wantErr: false,
			wantIDs: []string{"A", "B", "C"},
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Branches": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			ids, err := client.GetRackPDUBranchIDs(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantIDs, ids)
		})
	}
}

func TestGetRackPDUMainIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pduID        string
		serverStatus int
		serverBody   map[string]any
		wantErr      bool
		wantIDs      []string
	}{
		{
			name:         "success",
			pduID:        "1",
			serverStatus: http.StatusOK,
			serverBody: map[string]any{
				"Members@odata.count": 1,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/PowerEquipment/RackPDUs/1/Mains/AC1"},
				},
			},
			wantErr: false,
			wantIDs: []string{"AC1"},
		},
		{
			name:         "not found",
			pduID:        "999",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := setupTestServer(t, map[string]http.HandlerFunc{
				"/redfish/v1/SessionService/Sessions": loginHandler,
				"/redfish/v1/PowerEquipment/RackPDUs/" + tc.pduID + "/Mains": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.serverStatus)
					if tc.serverBody != nil {
						_ = json.NewEncoder(w).Encode(tc.serverBody)
					}
				},
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			require.NoError(t, client.Login(context.Background()))

			ids, err := client.GetRackPDUMainIDs(context.Background(), tc.pduID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantIDs, ids)
		})
	}
}
