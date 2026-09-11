// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// talk to the PDU over HTTP.
// Open an HTTP connection to a PDU hostname
// Log in with username/password (Redfish authentication)
// Fetch a URL from the PDU and return the JSON
// Log out and close the connection cleanly

type Client struct {
	baseURL    *url.URL     // Base URL of the PDU Redfish API (e.g., https://pdu01.example.com)
	httpClient *http.Client // HTTP client with timeout and TLS configuration

	username string // Authentication username
	password string // Authentication password

	authToken  string // Session authentication token (X-Auth-Token)
	sessionURL string // Session URL for logout

	logger  *slog.Logger // Structured logger for debugging and audit
	timeout time.Duration
}

func NewClient(baseURL *url.URL, logger *slog.Logger, username, password string, timeout time.Duration) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		baseURL:  baseURL,
		logger:   logger,
		timeout:  timeout,
		username: username,
		password: password,
	}
}

// Login creates redfish session
func (c *Client) Login(ctx context.Context) error {
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
	defer resp.Body.Close()

	token := resp.Header.Get("X-Auth-Token")
	if token == "" {
		return fmt.Errorf("login response did not contain X-Auth-Token")
	}

	sessionURL := resp.Header.Get("Location")
	if sessionURL == "" {
		return fmt.Errorf("login response did not contain session Location")
	}

	c.authToken = token
	c.sessionURL = sessionURL

	c.logger.Info("Redfish session created")

	return nil
}

// Close terminates redfish session
func (c *Client) Close(ctx context.Context) error {
	if c.sessionURL == "" || c.authToken == "" {
		return nil // No session to clean up
	}

	sessionURL := c.baseURL.ResolveReference(&url.URL{Path: c.sessionURL})
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		sessionURL.String(),
		nil,
	)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", c.authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("close Redfish session: %w", err)
	}
	defer resp.Body.Close()

	c.authToken = ""
	c.sessionURL = ""

	return nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	fullURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fullURL.String(),
		nil,
	)
	if err != nil {
		return err
	}

	if c.authToken != "" {
		req.Header.Set("X-Auth-Token", c.authToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status code %d", path, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode GET %s response: %w", path, err)
	}

	return nil
}

// GetPowerEquipment retrieves the Power Equipment collection
func (c *Client) GetPowerEquipment(ctx context.Context) (*PowerEquipment, error) {
	var result PowerEquipment
	if err := c.get(ctx, "/redfish/v1/PowerEquipment", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRackPDUs retrieves the Rack PDUs collection
func (c *Client) GetRackPDUs(ctx context.Context) (*RackPDUsCollection, error) {
	var result RackPDUsCollection
	if err := c.get(ctx, "/redfish/v1/PowerEquipment/RackPDUs", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPDU retrieves details for a specific PDU
func (c *Client) GetPDU(ctx context.Context, pduID string) (*RackPDU, error) {
	var result RackPDU
	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPDUMetrics retrieves metrics for a specific PDU
func (c *Client) GetPDUMetrics(ctx context.Context, pduID string) (*PDUMetrics, error) {
	var result PDUMetrics
	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Metrics", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
