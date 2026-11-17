// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"context"
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
