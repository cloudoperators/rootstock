// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

// talk to the PDU over HTTP.
// Open an HTTP connection to a PDU hostname
// Log in with username/password (Redfish authentication)
// Fetch a URL from the PDU and return the JSON
// Log out and close the connection cleanly

type Client interface {
	// Login opens an authenticated Redfish session with the PDU.
	Login(context.Context) error
	// Close terminates the Redfish session.
	Close(context.Context) error
	// Get fetches urlPath and decodes the JSON response into result.
	Get(ctx context.Context, urlPath string, result any) error
}

type ClientImpl struct {
	baseURL    *url.URL     // Base URL of the PDU Redfish API (e.g., https://pdu01.example.com)
	httpClient *http.Client // HTTP client with timeout and TLS configuration

	username string // Authentication username
	password string // Authentication password

	authToken  string // Session authentication token (X-Auth-Token)
	sessionURL string // Session URL for logout

	logger  log.MinInterface // Structured logger for debugging and audit
	timeout time.Duration
}

func NewClient(baseURL *url.URL, logger log.MinInterface, username, password string, timeout time.Duration) Client {
	if logger == nil {
		logger = log.Default()
	}

	return &ClientImpl{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				// PDUs ship with self-signed certificates, so certificate verification is intentionally skipped.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // G402: self-signed PDU certs
			},
		},
		baseURL:  baseURL,
		logger:   logger,
		timeout:  timeout,
		username: username,
		password: password,
	}
}

// Login creates redfish session.
func (c *ClientImpl) Login(ctx context.Context) error {
	payload := map[string]string{
		"UserName": c.username,
		"Password": c.password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}

	loginURL := c.baseURL.ResolveReference(&url.URL{Path: "/redfish/v1/SessionService/Sessions"})

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		loginURL.String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create Redfish session: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", "error", err)
		}
	}()

	token := resp.Header.Get("X-Auth-Token")
	if token == "" {
		return errors.New("login response did not contain X-Auth-Token")
	}

	sessionURL := resp.Header.Get("Location")
	if sessionURL == "" {
		return errors.New("login response did not contain session Location")
	}

	c.authToken = token
	c.sessionURL = sessionURL

	c.logger.Info("Redfish session created")

	return nil
}

// Close terminates redfish session.
func (c *ClientImpl) Close(ctx context.Context) error {
	if c.sessionURL == "" || c.authToken == "" {
		return nil // No session to clean up
	}

	sessionURL := c.baseURL.ResolveReference(&url.URL{Path: c.sessionURL})

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, sessionURL.String(), http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("X-Auth-Token", c.authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("close Redfish session: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", "error", err)
		}
	}()

	c.authToken = ""
	c.sessionURL = ""

	return nil
}

func (c *ClientImpl) Get(ctx context.Context, urlPath string, result any) error {
	fullURL := c.baseURL.ResolveReference(&url.URL{Path: urlPath})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), http.NoBody)
	if err != nil {
		return err
	}

	if c.authToken != "" {
		req.Header.Set("X-Auth-Token", c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", urlPath, err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status code %d", urlPath, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode GET %s response: %w", urlPath, err)
	}

	return nil
}
