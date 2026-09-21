// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package redfish_test

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/SAP/rootstock/internal/redfish"
)

// Run with: make test-integration
// Or manually:
//
//	go test -tags integration -v ./internal/redfish/...
//
// Requires the DMTF mockup server running with the PDU mockup:
//
//	docker run -p 8000:8000 \
//	  -v /path/to/pdu-mockup:/usr/src/app/pdu-mockup \
//	  dmtf/redfish-mockup-server:latest -D pdu-mockup

var mockupBase = func() string {
	if v := os.Getenv("REDFISH_MOCKUP_URL"); v != "" {
		return v
	}
	return "http://localhost:8000"
}()

func newTestClient(t *testing.T) redfish.Client {
	t.Helper()
	base, err := url.Parse(mockupBase)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return redfish.NewClient(base, logger, "admin", "password", 10*time.Second)
}

// TestLogin verifies that Login() reaches the API, sends credentials,
// and successfully parses the X-Auth-Token from the response headers.
func TestLogin(t *testing.T) {
	t.Logf("  %-22s: POST /redfish/v1/SessionService/Sessions", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()
	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)
	t.Logf("  %-22s: session created, X-Auth-Token received", "result")
}

// TestLoginAndClose verifies the full session lifecycle:
// Login creates a session, Close deletes it.
func TestLoginAndClose(t *testing.T) {
	t.Logf("  %-22s: POST sessions → DELETE session", "flow")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	t.Logf("  %-22s: session created", "login")

	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	t.Logf("  %-22s: session deleted", "close")
}

// TestGetPowerEquipment verifies that GetPowerEquipment returns a valid response
// with a RackPDUs link.
func TestGetPowerEquipment(t *testing.T) {
	t.Logf("  %-22s: GET /redfish/v1/PowerEquipment", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	result, err := client.GetPowerEquipment(ctx)
	if err != nil {
		t.Fatalf("GetPowerEquipment failed: %v", err)
	}
	if result.RackPDUs.ODataID == "" {
		t.Fatal("response missing RackPDUs link")
	}
	t.Logf("  %-22s: %v", "RackPDUs link", result.RackPDUs.ODataID)
}

// TestGetRackPDUs verifies that GetRackPDUs returns a collection with at least
// one member, and that each member has an @odata.id link.
func TestGetRackPDUs(t *testing.T) {
	t.Logf("  %-22s: GET /redfish/v1/PowerEquipment/RackPDUs", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	result, err := client.GetRackPDUs(ctx)
	if err != nil {
		t.Fatalf("GetRackPDUs failed: %v", err)
	}
	if len(result.Members) == 0 {
		t.Fatal("expected at least one member in RackPDUs collection")
	}
	t.Logf("  %-22s: %d", "member count", len(result.Members))
	for i, m := range result.Members {
		t.Logf("  member[%d] @odata.id   : %v", i, m.ODataID)
	}
}

// TestGetPDU verifies that GetPDU returns typed fields for PDU "1",
// including a non-empty ID and a Metrics link.
func TestGetPDU(t *testing.T) {
	t.Logf("  %-22s: GET /redfish/v1/PowerEquipment/RackPDUs/1", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	result, err := client.GetPDU(ctx, "1")
	if err != nil {
		t.Fatalf("GetPDU failed: %v", err)
	}
	if result.ID == 0 {
		t.Fatal("response missing Id field")
	}
	if result.Metrics.ODataID == "" {
		t.Fatal("response missing Metrics link")
	}
	t.Logf("  %-22s: %v", "PDU Id", result.ID)
	t.Logf("  %-22s: %v", "PDU Name", result.Name)
	t.Logf("  %-22s: %v", "Manufacturer", result.Manufacturer)
	t.Logf("  %-22s: %v", "Model", result.Model)
	t.Logf("  %-22s: %v", "Metrics link", result.Metrics.ODataID)
}

// TestGetPDUMetrics verifies that GetPDUMetrics returns power readings with
// non-negative values for PowerWatts and EnergykWh.
func TestGetPDUMetrics(t *testing.T) {
	t.Logf("  %-22s: GET /redfish/v1/PowerEquipment/RackPDUs/1/Metrics", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	result, err := client.GetPDUMetrics(ctx, "1")
	if err != nil {
		t.Fatalf("GetPDUMetrics failed: %v", err)
	}
	if result.PowerWatts.Reading < 0 {
		t.Fatalf("PowerWatts.Reading should be non-negative, got %v", result.PowerWatts.Reading)
	}
	if result.EnergykWh.Reading < 0 {
		t.Fatalf("EnergykWh.Reading should be non-negative, got %v", result.EnergykWh.Reading)
	}
	t.Logf("  %-22s: %.2f W", "PowerWatts.Reading", result.PowerWatts.Reading)
	t.Logf("  %-22s: %.2f VA", "ApparentVA", result.PowerWatts.ApparentVA)
	t.Logf("  %-22s: %.2f VAR", "ReactiveVAR", result.PowerWatts.ReactiveVAR)
	t.Logf("  %-22s: %.4f", "PowerFactor", result.PowerWatts.PowerFactor)
	t.Logf("  %-22s: %.4f kWh", "EnergykWh.Reading", result.EnergykWh.Reading)
}

// TestGetPDUNotFound verifies that GetPDU returns an error for a non-existent PDU ID.
func TestGetPDUNotFound(t *testing.T) {
	t.Logf("  %-22s: GET /redfish/v1/PowerEquipment/RackPDUs/nonexistent", "endpoint")
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	_, err := client.GetPDU(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent PDU, got nil")
	}
	t.Logf("  %-22s: %v", "error (expected)", err)
}
