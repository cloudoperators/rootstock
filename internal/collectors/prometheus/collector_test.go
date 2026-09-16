package prometheus

// import (
// 	"context"
// 	"encoding/json"
// 	"log/slog"
// 	"net/http"
// 	"net/http/httptest"
// 	"net/url"
// 	"os"
// 	"testing"
// 	"time"
// )

// func TestCollector_Collect(t *testing.T) {
// 	// Create a mock Redfish server
// 	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		// Mock authentication endpoint
// 		if r.URL.Path == "/redfish/v1/SessionService/Sessions" {
// 			w.Header().Set("X-Auth-Token", "mock-token")
// 			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
// 			w.WriteHeader(http.StatusCreated)
// 			json.NewEncoder(w).Encode(map[string]interface{}{
// 				"Id": "1",
// 			})
// 			return
// 		}

// 		// Mock PDU metrics endpoint
// 		if r.URL.Path == "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics" {
// 			w.Header().Set("Content-Type", "application/json")
// 			response := map[string]interface{}{
// 				"@odata.type": "#PowerDistributionMetrics.v1_3_1.PowerDistributionMetrics",
// 				"Id":          "Metrics",
// 				"Name":        "PDU Metrics",
// 				"PowerWatts": map[string]interface{}{
// 					"DataSourceUri": "/redfish/v1/PowerEquipment/PowerShelves/1",
// 					"Reading":       1500.5,
// 					"ApparentVA":    1600.0,
// 					"ReactiveVAR":   300.0,
// 					"PowerFactor":   0.94,
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
// 	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
// 		Level: slog.LevelDebug,
// 	}))

// 	// Create collector
// 	collector := NewCollector(nil)

// 	// Create target
// 	target := Target{
// 		Name:     "TestPDU",
// 		Endpoint: mockServer.URL,
// 		Username: "testuser",
// 		Password: "testpass",
// 	}

// 	// Collect metrics
// 	ctx := context.Background()
// 	metrics, err := collector.Collect(ctx, target, baseURL, logger, "testuser", "testpass", 10*time.Second)
// 	if err != nil {
// 		t.Fatalf("Failed to collect metrics: %v", err)
// 	}

// 	// Verify metrics structure
// 	if metrics.ResourceMetrics().Len() == 0 {
// 		t.Fatal("Expected at least one resource metric")
// 	}

// 	rm := metrics.ResourceMetrics().At(0)
// 	if rm.ScopeMetrics().Len() == 0 {
// 		t.Fatal("Expected at least one scope metric")
// 	}

// 	sm := rm.ScopeMetrics().At(0)
// 	if sm.Metrics().Len() != 4 {
// 		t.Fatalf("Expected 4 metrics, got %d", sm.Metrics().Len())
// 	}

// 	// Verify each metric has data points with values
// 	for i := 0; i < sm.Metrics().Len(); i++ {
// 		metric := sm.Metrics().At(i)
// 		t.Logf("Checking metric: %s", metric.Name())

// 		switch metric.Type() {
// 		case 1: // Gauge
// 			gauge := metric.Gauge()
// 			if gauge.DataPoints().Len() == 0 {
// 				t.Errorf("Metric %s has no data points", metric.Name())
// 				continue
// 			}
// 			dp := gauge.DataPoints().At(0)
// 			value := dp.DoubleValue()
// 			t.Logf("Metric %s value: %f", metric.Name(), value)

// 			// Verify the value is set (not zero)
// 			switch metric.Name() {
// 			case "pdu_power_watts":
// 				if value != 1500.5 {
// 					t.Errorf("Expected power_watts to be 1500.5, got %f", value)
// 				}
// 			case "pdu_power_watts_apparent_va":
// 				if value != 1600.0 {
// 					t.Errorf("Expected power_watts_apparent_va to be 1600.0, got %f", value)
// 				}
// 			case "pdu_power_watts_reactive_power_var":
// 				if value != 300.0 {
// 					t.Errorf("Expected power_watts_reactive_power_var to be 300.0, got %f", value)
// 				}
// 			case "pdu_power_watts_power_factor":
// 				if value != 0.94 {
// 					t.Errorf("Expected power_watts_power_factor to be 0.94, got %f", value)
// 				}
// 			default:
// 				t.Errorf("Unexpected metric name: %s", metric.Name())
// 			}
// 		default:
// 			t.Errorf("Unexpected metric type for %s: %v", metric.Name(), metric.Type())
// 		}
// 	}
// }
