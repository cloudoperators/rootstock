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

	"github.com/cloudoperators/rootstock/internal/redfish"
)

// Run with: make test-integration
// Or manually:
//
//	go test -tags integration -v ./internal/redfish/...
//
// Requires the DMTF mockup server running with the PDU mockup:
//
// docker run -p 8000:8000 \
//   -v /path/to/pdu-mockup:/usr/src/app/pdu-mockup \
//   dmtf/redfish-mockup-server:latest -D pdu-mockup

var mockupBase = func() string {
	if v := os.Getenv("REDFISH_MOCKUP_URL"); v != "" {
		return v
	}
	return "http://localhost:8000"
}()

func newIntegrationClient(t *testing.T) redfish.Client {
	t.Helper()
	base, err := url.Parse(mockupBase)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return redfish.NewClient(base, logger, "admin", "password", 10*time.Second)
}

// TestLoginIntegration verifies that Login() reaches the API, sends credentials,
// and successfully parses the X-Auth-Token from the response headers.
func TestLoginIntegration(t *testing.T) {
	t.Logf("  %-22s: POST /redfish/v1/SessionService/Sessions", "endpoint")
	client := newIntegrationClient(t)
	ctx := context.Background()
	if err := client.Login(ctx); err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	defer client.Close(ctx)
	t.Logf("  %-22s: session created, X-Auth-Token received", "result")
}

// TestLoginAndCloseIntegration verifies the full session lifecycle:
// Login creates a session, Close deletes it.
func TestLoginAndCloseIntegration(t *testing.T) {
	t.Logf("  %-22s: POST sessions → DELETE session", "flow")
	client := newIntegrationClient(t)
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
