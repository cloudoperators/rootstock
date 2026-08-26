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

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/redfish/v1/SessionService/Sessions",
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

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		c.sessionURL,
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

func (c *Client) Get(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		path,
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

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode GET %s response: %w", path, err)
	}

	return nil
}
