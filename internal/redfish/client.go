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
	// GetPowerEquipment returns the PowerEquipment collection.
	GetPowerEquipment(context.Context) (*PowerEquipment, error)
	// GetRackPDUs returns the collection of Rack PDUs.
	GetRackPDUs(context.Context) (*RackPDUsCollection, error)
	// GetRackPDUPowerRating returns power ratings for a single Rack PDU by ID.
	GetRackPDUPowerRating(context.Context, string) (*RackPDU, error)
	// GetRackPDUEnergetics returns power and energy metrics for a single Rack PDU by ID.
	GetRackPDUEnergetics(context.Context, string) (*PDUEnergetics, error)
	// GetRackPDUOutletElectricals returns the power, voltage and current metrics for a single outlet Rack PDU by ID.
	GetRackPDUOutletElectricals(context.Context, string, string) (*OutletEletricals, error)
	// GetRackPDUMainCurrent returns the main current metrics for a single Rack PDU by ID.
	GetRackPDUMainCurrent(context.Context, string) (*MainCurrent, error)
	// GetRackPDUBranchCurrent returns the current metrics for a single branch Rack PDU by ID.
	GetRackPDUBranchCurrent(context.Context, string, string) (*BranchCurrent, error)
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

// PowerEquipment represents the PowerEquipment collection.
type PowerEquipment struct {
	Name string `json:"Name"`
}

// RackPDUsCollection represents the collection of RackPDUs.
type RackPDUsCollection struct {
	Name         string `json:"Name"`
	MembersCount int    `json:"Members@odata.count"`
}

// RackPDU represents a single RackPDU device.
type RackPDU struct {
	ID              string  `json:"Id"`
	Name            string  `json:"Name"`
	Status          Status  `json:"Status"`
	PowerCapacityVA float64 `json:"PowerCapacityVA"`
}

// Status represents the status of a Redfish resource.
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// PDUEnergeticsMetrics represents the metrics for a PDU.
type PDUEnergetics struct {
	ID         string     `json:"Id"`
	Name       string     `json:"Name"`
	PowerWatts PowerWatts `json:"PowerWatts"`
	EnergykWh  EnergykWh  `json:"EnergykWh"`
}

// PowerWatts represents a power and apparent power merics.
type PowerWatts struct {
	Power         float64 `json:"Reading"`
	ApparentPower float64 `json:"ApparentVA"`
}

// EnergykWh represents energy value.
type EnergykWh struct {
	EnergykWh float64 `json:"EnergykWh"` // we need this value in watts
}

// OutletElectricals represent outlet metrics.
type OutletEletricals struct {
	Status        Status     `json:"Status"`
	OutletType    string     `json:"OutletType"`
	RatedCurrent  float64    `json:"RatedCurrentAmps"`
	Voltage       Voltage    `json:"Voltage"`
	Current       Current    `json:"CurrentAmps"`
	ApparentPower PowerWatts `json:"PowerWatts"`
}

// Voltage represents volatage.
type Voltage struct {
	Voltage float64 `json:"Reading"`
}

// Current represents current.
type Current struct {
	Current float64 `json:"Reading"`
}

// MainCurrent represents main current power metrics.
type MainCurrent struct {
	// 3 phases voltage
	Line1ToLine2 PowerWatts `json:"Line1ToLine2"` //how many of these
}

// BranchCurrent represents branch current metrics.
type BranchCurrent struct {
	RatedCurrent float64     `json:"RatedCurrentAmps"`
	Current      CurrentAmps `json:"CurrentAmps"`
}

// CurrentAmps represents current metric.
type CurrentAmps struct {
	Current float64 `json:"Reading"`
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

func (c *ClientImpl) get(ctx context.Context, path string, result any) error {
	fullURL := c.baseURL.ResolveReference(&url.URL{Path: path})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), http.NoBody)
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

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Warn("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status code %d", path, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode GET %s response: %w", path, err)
	}

	return nil
}

// GetPowerEquipment retrieves the Power Equipment collection.
func (c *ClientImpl) GetPowerEquipment(ctx context.Context) (*PowerEquipment, error) {
	var result PowerEquipment
	if err := c.get(ctx, "/redfish/v1/PowerEquipment", &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetRackPDUs retrieves the Rack PDUs collection.
func (c *ClientImpl) GetRackPDUs(ctx context.Context) (*RackPDUsCollection, error) {
	var result RackPDUsCollection
	if err := c.get(ctx, "/redfish/v1/PowerEquipment/RackPDUs", &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetPDU retrieves details for a specific PDU.
func (c *ClientImpl) GetRackPDUPowerRating(ctx context.Context, pduID string) (*RackPDU, error) {
	var result RackPDU

	endpoint := "/redfish/v1/PowerEquipment/RackPDUs/" + pduID
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetPDUMetrics retrieves metrics for a specific PDU.
func (c *ClientImpl) GetRackPDUEnergetics(ctx context.Context, pduID string) (*PDUEnergetics, error) {
	var result PDUEnergetics

	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Metrics", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *ClientImpl) GetRackPDUOutletElectricals(ctx context.Context, pduID, outlet string) (*OutletEletricals, error) {
	var result OutletEletricals

	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Outlets/%s", pduID, outlet)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *ClientImpl) GetRackPDUMainCurrent(ctx context.Context, pduID string) (*MainCurrent, error) {
	var result MainCurrent

	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Mains/AC1", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *ClientImpl) GetRackPDUBranchCurrent(ctx context.Context, pduID, branch string) (*BranchCurrent, error) {
	var result BranchCurrent

	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Branches/%s", pduID, branch)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
