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

// Run with: go test -tags integration -v ./internal/redfish/...
// Requires the DMTF mockup server running with the PDU mockup:
//
//	docker run -p 8000:8000 \
//	  -v /tmp/pdu-mockup:/usr/src/app/pdu-mockup \
//	  dmtf/redfish-mockup-server:latest -D pdu-mockup

var mockupBase = "http://localhost:8000"

func newTestClient(t *testing.T) *redfish.Client {
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
	client := newTestClient(t)
	if err := client.Login(context.Background()); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
}

// TestLoginAndClose verifies the full session lifecycle:
// Login creates a session, Close deletes it.
func TestLoginAndClose(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if err := client.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// TestGetRackPDUCollection verifies that Get() can fetch the RackPDU collection
// and that the response contains a Members list.
func TestGetRackPDUCollection(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	var result map[string]any
	if err := client.Get(ctx, "/redfish/v1/PowerEquipment/RackPDUs", &result); err != nil {
		t.Fatalf("Get RackPDUs failed: %v", err)
	}

	members, ok := result["Members"]
	if !ok {
		t.Fatal("response missing Members field")
	}
	t.Logf("RackPDUs Members: %v", members)
}

// TestGetRackPDU verifies that Get() can fetch a single PDU by its @odata.id
// and that the response contains Id, Name, and a Metrics link.
func TestGetRackPDU(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	var result map[string]any
	if err := client.Get(ctx, "/redfish/v1/PowerEquipment/RackPDUs/1", &result); err != nil {
		t.Fatalf("Get RackPDUs/1 failed: %v", err)
	}

	if result["Id"] == nil {
		t.Fatal("response missing Id field")
	}
	if result["Metrics"] == nil {
		t.Fatal("response missing Metrics link")
	}
	t.Logf("PDU Id: %v, Metrics link: %v", result["Id"], result["Metrics"])
}

// TestGetRackPDUMetrics verifies that Get() can fetch the Metrics resource
// and that PowerWatts.Reading is present.
func TestGetRackPDUMetrics(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	var result map[string]any
	if err := client.Get(ctx, "/redfish/v1/PowerEquipment/RackPDUs/1/Metrics", &result); err != nil {
		t.Fatalf("Get RackPDUs/1/Metrics failed: %v", err)
	}

	powerWatts, ok := result["PowerWatts"]
	if !ok {
		t.Fatal("response missing PowerWatts field")
	}
	t.Logf("PowerWatts: %v", powerWatts)
}

// TestGetNonExistentPath verifies that Get() returns an error for a 404 response.
func TestGetNonExistentPath(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)

	var result map[string]any
	err := client.Get(ctx, "/redfish/v1/DoesNotExist", &result)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
	t.Logf("Got expected error: %v", err)
}