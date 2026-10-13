// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/pkg/log"
)

// Helper function to create a test server with custom handlers
func setupTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}

	return httptest.NewServer(mux)
}

// Helper function to create a client for testing
func newTestClient(t *testing.T, serverURL string) redfish.Client {
	t.Helper()

	base, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	logger := log.NewMockLogger()

	return redfish.NewClient(base, logger, "testuser", "testpass", 5*time.Second)
}

// TestNewClient verifies that NewClient creates a client with correct configuration
func TestNewClient(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://pdu.example.com")
	logger := log.NewMockLogger()
	timeout := 10 * time.Second

	client := redfish.NewClient(baseURL, logger, "user", "pass", timeout)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

// TestLogin_Success verifies successful login flow
func TestLogin_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}

			// Verify request body
			var loginReq struct {
				Username string `json:"UserName"`
				Password string `json:"Password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
				t.Errorf("failed to decode login request: %v", err)
			}

			if loginReq.Username != "testuser" {
				t.Errorf("expected username 'testuser', got '%s'", loginReq.Username)
			}

			if loginReq.Password != "testpass" {
				t.Errorf("expected password 'testpass', got '%s'", loginReq.Password)
			}

			// Return success response
			w.Header().Set("X-Auth-Token", "test-token-123")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"Id": "1"})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	err := client.Login(ctx)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
}

// TestClose_Success verifies successful logout
func TestClose_Success(t *testing.T) {
	t.Parallel()

	loginCalled := false
	closeCalled := false

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			loginCalled = true

			w.Header().Set("X-Auth-Token", "test-token-123")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/SessionService/Sessions/1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}

			closeCalled = true

			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	// Login first
	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Then close
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !loginCalled {
		t.Error("login was not called")
	}

	if !closeCalled {
		t.Error("close was not called")
	}
}

// TestGetPowerEquipment_Success verifies successful power equipment retrieval
func TestGetPowerEquipment_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			// Verify auth token is sent
			if r.Header.Get("X-Auth-Token") != "test-token" {
				t.Errorf("missing or wrong X-Auth-Token header")
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"Name": "Power Equipment",
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetPowerEquipment(ctx)
	if err != nil {
		t.Fatalf("GetPowerEquipment failed: %v", err)
	}

	if result.Name != "Power Equipment" {
		t.Errorf("expected Name 'Power Equipment', got '%s'", result.Name)
	}
}

// TestGetRackPDUs_Success verifies successful rack PDUs retrieval
func TestGetRackPDUs_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"Name":                "RackPDUs Collection",
				"Members@odata.count": 2,
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUs(ctx)
	if err != nil {
		t.Fatalf("GetRackPDUs failed: %v", err)
	}

	if result.MembersCount != 2 {
		t.Errorf("expected MembersCount 2, got %d", result.MembersCount)
	}
}

// TestGetRackPDUPowerRating_Success verifies successful PDU power rating retrieval
func TestGetRackPDUPowerRating_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"Id":              "1",
				"Name":            "PDU-01",
				"PowerCapacityVA": 7200.0,
				"Status": map[string]string{
					"State":  "Enabled",
					"Health": "OK",
				},
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUPowerRating(ctx, "1")
	if err != nil {
		t.Fatalf("GetRackPDUPowerRating failed: %v", err)
	}

	if result.ID != "1" {
		t.Errorf("expected ID '1', got '%s'", result.ID)
	}

	if result.Name != "PDU-01" {
		t.Errorf("expected Name 'PDU-01', got '%s'", result.Name)
	}

	if result.PowerCapacityVA != 7200.0 {
		t.Errorf("expected PowerCapacityVA 7200.0, got %f", result.PowerCapacityVA)
	}

	if result.Status.State != "Enabled" {
		t.Errorf("expected Status.State 'Enabled', got '%s'", result.Status.State)
	}
}

// TestGetRackPDUEnergetics_Success verifies successful PDU energetics retrieval
func TestGetRackPDUEnergetics_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/1/Metrics": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"Id":   "1",
				"Name": "PDU-01 Metrics",
				"PowerWatts": map[string]float64{
					"Reading":    3500.5,
					"ApparentVA": 3600.0,
				},
				"EnergykWh": map[string]float64{
					"EnergykWh": 12345.67,
				},
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUEnergetics(ctx, "1")
	if err != nil {
		t.Fatalf("GetRackPDUEnergetics failed: %v", err)
	}

	if result.PowerWatts.Power != 3500.5 {
		t.Errorf("expected PowerWatts.Power 3500.5, got %f", result.PowerWatts.Power)
	}

	if result.PowerWatts.ApparentPower != 3600.0 {
		t.Errorf("expected PowerWatts.ApparentPower 3600.0, got %f", result.PowerWatts.ApparentPower)
	}

	if result.EnergykWh.EnergykWh != 12345.67 {
		t.Errorf("expected EnergykWh.EnergykWh 12345.67, got %f", result.EnergykWh.EnergykWh)
	}
}

// TestGet_NotFound verifies error handling for 404 responses
func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/999": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	_, err := client.GetRackPDUPowerRating(ctx, "999")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected status code 404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

// TestGetRackPDUOutletElectricals_Success verifies successful outlet electricals retrieval
func TestGetRackPDUOutletElectricals_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/1/Outlets/A1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"Status": map[string]string{
					"State":  "Enabled",
					"Health": "OK",
				},
				"OutletType":       "NEMA_5_20R",
				"RatedCurrentAmps": 16.0,
				"Voltage": map[string]float64{
					"Reading": 120.5,
				},
				"CurrentAmps": map[string]float64{
					"Reading": 8.2,
				},
				"PowerWatts": map[string]float64{
					"Reading":    988.1,
					"ApparentVA": 1000.0,
				},
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUOutletElectricals(ctx, "1", "A1")
	if err != nil {
		t.Fatalf("GetRackPDUOutletElectricals failed: %v", err)
	}

	if result.Status.State != "Enabled" {
		t.Errorf("expected Status.State 'Enabled', got '%s'", result.Status.State)
	}

	if result.Status.Health != "OK" {
		t.Errorf("expected Status.Health 'OK', got '%s'", result.Status.Health)
	}

	if result.OutletType != "NEMA_5_20R" {
		t.Errorf("expected OutletType 'NEMA_5_20R', got '%s'", result.OutletType)
	}

	if result.RatedCurrent != 16.0 {
		t.Errorf("expected RatedCurrent 16.0, got %f", result.RatedCurrent)
	}

	if result.Voltage.Voltage != 120.5 {
		t.Errorf("expected Voltage.Voltage 120.5, got %f", result.Voltage.Voltage)
	}

	if result.Current.Current != 8.2 {
		t.Errorf("expected Current.Current 8.2, got %f", result.Current.Current)
	}

	if result.ApparentPower.Power != 988.1 {
		t.Errorf("expected ApparentPower.Power 988.1, got %f", result.ApparentPower.Power)
	}

	if result.ApparentPower.ApparentPower != 1000.0 {
		t.Errorf("expected ApparentPower.ApparentPower 1000.0, got %f", result.ApparentPower.ApparentPower)
	}
}

// TestGetRackPDUMainCurrent_Success verifies successful main current retrieval
func TestGetRackPDUMainCurrent_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/1/Mains/AC1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"PolyPhaseVoltage": map[string]any{
					"Line1ToLine2": map[string]float64{
						"Reading": 208.5,
					},
					"Line2ToLine3": map[string]float64{
						"Reading": 207.8,
					},
					"Line3ToLine1": map[string]float64{
						"Reading": 208.2,
					},
				},
				"PolyPhasePowerWatts": map[string]any{
					"Line1ToLine2": map[string]float64{
						"Reading":    1200.5,
						"ApparentVA": 1250.0,
					},
					"Line2ToLine3": map[string]float64{
						"Reading":    1180.3,
						"ApparentVA": 1230.0,
					},
					"Line3ToLine1": map[string]float64{
						"Reading":    1195.7,
						"ApparentVA": 1240.0,
					},
				},
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUMainCurrent(ctx, "1")
	if err != nil {
		t.Fatalf("GetRackPDUMainCurrent failed: %v", err)
	}

	// Verify voltage readings
	if result.PolyPhaseVoltage.Line1ToLine2.Voltage != 208.5 {
		t.Errorf("expected Line1ToLine2.Voltage 208.5, got %f", result.PolyPhaseVoltage.Line1ToLine2.Voltage)
	}

	if result.PolyPhaseVoltage.Line2ToLine3.Voltage != 207.8 {
		t.Errorf("expected Line2ToLine3.Voltage 207.8, got %f", result.PolyPhaseVoltage.Line2ToLine3.Voltage)
	}

	if result.PolyPhaseVoltage.Line3ToLine1.Voltage != 208.2 {
		t.Errorf("expected Line3ToLine1.Voltage 208.2, got %f", result.PolyPhaseVoltage.Line3ToLine1.Voltage)
	}

	// Verify power readings
	if result.PolyPhasePowerWatts.Line1ToLine2.Power != 1200.5 {
		t.Errorf("expected Line1ToLine2.Power 1200.5, got %f", result.PolyPhasePowerWatts.Line1ToLine2.Power)
	}

	if result.PolyPhasePowerWatts.Line1ToLine2.ApparentPower != 1250.0 {
		t.Errorf("expected Line1ToLine2.ApparentPower 1250.0, got %f", result.PolyPhasePowerWatts.Line1ToLine2.ApparentPower)
	}

	if result.PolyPhasePowerWatts.Line2ToLine3.Power != 1180.3 {
		t.Errorf("expected Line2ToLine3.Power 1180.3, got %f", result.PolyPhasePowerWatts.Line2ToLine3.Power)
	}

	if result.PolyPhasePowerWatts.Line2ToLine3.ApparentPower != 1230.0 {
		t.Errorf("expected Line2ToLine3.ApparentPower 1230.0, got %f", result.PolyPhasePowerWatts.Line2ToLine3.ApparentPower)
	}

	if result.PolyPhasePowerWatts.Line3ToLine1.Power != 1195.7 {
		t.Errorf("expected Line3ToLine1.Power 1195.7, got %f", result.PolyPhasePowerWatts.Line3ToLine1.Power)
	}

	if result.PolyPhasePowerWatts.Line3ToLine1.ApparentPower != 1240.0 {
		t.Errorf("expected Line3ToLine1.ApparentPower 1240.0, got %f", result.PolyPhasePowerWatts.Line3ToLine1.ApparentPower)
	}
}

// TestGetRackPDUBranchCurrent_Success verifies successful branch current retrieval
func TestGetRackPDUBranchCurrent_Success(t *testing.T) {
	t.Parallel()

	server := setupTestServer(t, map[string]http.HandlerFunc{
		"/redfish/v1/SessionService/Sessions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		},
		"/redfish/v1/PowerEquipment/RackPDUs/1/Branches/B1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"RatedCurrentAmps": 32.0,
				"CurrentAmps": map[string]float64{
					"Reading": 18.5,
				},
			})
		},
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	result, err := client.GetRackPDUBranchCurrent(ctx, "1", "B1")
	if err != nil {
		t.Fatalf("GetRackPDUBranchCurrent failed: %v", err)
	}

	if result.RatedCurrent != 32.0 {
		t.Errorf("expected RatedCurrent 32.0, got %f", result.RatedCurrent)
	}

	if result.Current.Current != 18.5 {
		t.Errorf("expected Current.Current 18.5, got %f", result.Current.Current)
	}
}
