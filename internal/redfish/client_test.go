// // SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// // SPDX-License-Identifier: Apache-2.0

package redfish

// import (
// 	"context"
// 	"encoding/json"
// 	"log/slog"
// 	"net/http"
// 	"net/http/httptest"
// 	"net/url"
// 	"testing"
// 	"time"
// )

// func TestClient_GetPDUMetrics(t *testing.T) {
// 	// Create a mock server
// 	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		// Mock authentication endpoint
// 		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == http.MethodPost {
// 			w.Header().Set("X-Auth-Token", "mock-token")
// 			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
// 			w.WriteHeader(http.StatusCreated)
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"Id": "1",
// 			})
// 			return
// 		}

// 		// Mock PDU metrics endpoint
// 		if r.URL.Path == "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics" && r.Method == http.MethodGet {
// 			w.Header().Set("Content-Type", "application/json")
// 			response := PDUMetrics{
// 				ODataID:   "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics",
// 				ODataType: "#PowerDistributionMetrics.v1_3_1.PowerDistributionMetrics",
// 				ID:        "Metrics",
// 				Name:      "PDU Metrics",
// 				PowerWatts: SensorReading{
// 					DataSourceUri: "/redfish/v1/PowerEquipment/PowerShelves/1",
// 					Reading:       1500.5,
// 					ApparentVA:    1600.0,
// 					ReactiveVAR:   300.0,
// 					PowerFactor:   0.94,
// 				},
// 			}
// 			json.NewEncoder(w).Encode(response)
// 			return
// 		}

// 		w.WriteHeader(http.StatusNotFound)
// 	}))
// 	defer mockServer.Close()

// 	// Parse the mock server URL
// 	baseURL, err := url.Parse(mockServer.URL)
// 	if err != nil {
// 		t.Fatalf("Failed to parse mock server URL: %v", err)
// 	}

// 	// Create logger
// 	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{
// 		Level: slog.LevelError,
// 	}))

// 	// Create client
// 	client := NewClient(baseURL, logger, "testuser", "testpass", 10*time.Second)

// 	// Login
// 	ctx := context.Background()
// 	err = client.Login(ctx)
// 	if err != nil {
// 		t.Fatalf("Failed to login: %v", err)
// 	}

// 	// Get PDU metrics
// 	metrics, err := client.GetPDUMetrics(ctx, "1")
// 	if err != nil {
// 		t.Fatalf("Failed to get PDU metrics: %v", err)
// 	}

// 	// Verify metrics
// 	if metrics == nil {
// 		t.Fatal("Expected metrics to be non-nil")
// 	}

// 	if metrics.PowerWatts.Reading != 1500.5 {
// 		t.Errorf("Expected PowerWatts.Reading to be 1500.5, got %f", metrics.PowerWatts.Reading)
// 	}

// 	if metrics.PowerWatts.ApparentVA != 1600.0 {
// 		t.Errorf("Expected PowerWatts.ApparentVA to be 1600.0, got %f", metrics.PowerWatts.ApparentVA)
// 	}

// 	if metrics.PowerWatts.ReactiveVAR != 300.0 {
// 		t.Errorf("Expected PowerWatts.ReactiveVAR to be 300.0, got %f", metrics.PowerWatts.ReactiveVAR)
// 	}

// 	if metrics.PowerWatts.PowerFactor != 0.94 {
// 		t.Errorf("Expected PowerWatts.PowerFactor to be 0.94, got %f", metrics.PowerWatts.PowerFactor)
// 	}
// }

// func TestClient_GetRackPDUs(t *testing.T) {
// 	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == http.MethodPost {
// 			w.Header().Set("X-Auth-Token", "mock-token")
// 			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
// 			w.WriteHeader(http.StatusCreated)
// 			json.NewEncoder(w).Encode(map[string]interface{}{"Id": "1"})
// 			return
// 		}

// 		if r.URL.Path == "/redfish/v1/PowerEquipment/RackPDUs" && r.Method == http.MethodGet {
// 			w.Header().Set("Content-Type", "application/json")
// 			response := RackPDUsCollection{
// 				ODataID:      "/redfish/v1/PowerEquipment/RackPDUs",
// 				ODataType:    "#PowerDistributionCollection.PowerDistributionCollection",
// 				Name:         "Rack PDUs Collection",
// 				MembersCount: 2,
// 				Members: []Link{
// 					{ODataID: "/redfish/v1/PowerEquipment/RackPDUs/1"},
// 					{ODataID: "/redfish/v1/PowerEquipment/RackPDUs/2"},
// 				},
// 			}
// 			json.NewEncoder(w).Encode(response)
// 			return
// 		}

// 		w.WriteHeader(http.StatusNotFound)
// 	}))
// 	defer mockServer.Close()

// 	baseURL, err := url.Parse(mockServer.URL)
// 	if err != nil {
// 		t.Fatalf("Failed to parse mock server URL: %v", err)
// 	}

// 	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
// 	client := NewClient(baseURL, logger, "testuser", "testpass", 10*time.Second)

// 	ctx := context.Background()
// 	err = client.Login(ctx)
// 	if err != nil {
// 		t.Fatalf("Failed to login: %v", err)
// 	}

// 	collection, err := client.GetRackPDUs(ctx)
// 	if err != nil {
// 		t.Fatalf("Failed to get RackPDUs collection: %v", err)
// 	}

// 	if collection == nil {
// 		t.Fatal("Expected collection to be non-nil")
// 	}

// 	if collection.MembersCount != 2 {
// 		t.Errorf("Expected MembersCount to be 2, got %d", collection.MembersCount)
// 	}

// 	if len(collection.Members) != 2 {
// 		t.Errorf("Expected 2 members, got %d", len(collection.Members))
// 	}
// }
