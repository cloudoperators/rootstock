// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/SAP/rootstock/pkg/log"
)

// talk to the PDU over HTTP.
// Open an HTTP connection to a PDU hostname
// Log in with username/password (Redfish authentication)
// Fetch a URL from the PDU and return the JSON
// Log out and close the connection cleanly

type Client interface {
	Login(context.Context) error
	Close(context.Context) error
	GetPowerEquipment(context.Context) (*PowerEquipment, error)
	GetRackPDUs(context.Context) (*RackPDUsCollection, error)
	GetPERackPDU(context.Context, string) (*RackPDU, error)
	GetPERackPDUMetrics(context.Context, string) (*PDUMetrics, error)
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

// PowerEquipment represents the PowerEquipment collection
type PowerEquipment struct {
	ODataID   string `json:"@odata.id"`
	ODataType string `json:"@odata.type"`
	Name      string `json:"Name"`
	RackPDUs  struct {
		ODataID string `json:"@odata.id"`
	} `json:"RackPDUs"`
}

// RackPDUsCollection represents the collection of RackPDUs
type RackPDUsCollection struct {
	ODataID      string `json:"@odata.id"`
	ODataType    string `json:"@odata.type"`
	Name         string `json:"Name"`
	Members      []Link `json:"Members"`
	MembersCount int    `json:"Members@odata.count"`
}

// Link represents a Redfish link object
type Link struct {
	ODataID string `json:"@odata.id"`
}

// RackPDU represents a single RackPDU device
type RackPDU struct {
	ODataID         string `json:"@odata.id"`
	ODataType       string `json:"@odata.type"`
	ID              string `json:"Id"`
	Name            string `json:"Name"`
	EquipmentType   string `json:"EquipmentType"`
	FirmwareVersion string `json:"FirmwareVersion"`
	Manufacturer    string `json:"Manufacturer"`
	Model           string `json:"Model"`
	PartNumber      string `json:"PartNumber"`
	SerialNumber    string `json:"SerialNumber"`
	Version         string `json:"Version"`
	// PowerState      string `json:"PowerState"`
	Status       Status `json:"Status"`
	Metrics      Link   `json:"Metrics"`
	Outlets      Link   `json:"Outlets"`
	OutletGroups Link   `json:"OutletGroups"`
	Mains        Link   `json:"Mains"`
	Branches     Link   `json:"Branches"`
	// PowerSupplies   Link   `json:"PowerSupplies"`
	RatedPowerWatts int     `json:"RatedPowerWatts"`
	UUID            string  `json:"UUID"`
	PowerCapacityVA float64 `json:"PowerCapacityVA"`
	Location        struct {
		Placement struct {
			Row string `json:"Row"`
		} `json:"Placement"`
	} `json:"Location"`
}

// Status represents the status of a Redfish resource
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// PDUMetrics represents the metrics for a PDU
type PDUMetrics struct {
	ODataID            string        `json:"@odata.id"`
	ODataType          string        `json:"@odata.type"`
	ID                 string        `json:"Id"`
	Name               string        `json:"Name"`
	PowerWatts         SensorReading `json:"PowerWatts"`
	EnergykWh          SensorReading `json:"EnergykWh"`
	TemperatureCelsius SensorReading `json:"TemperatureCelsius"`
}

// SensorReading represents a sensor reading with value and metadata
type SensorReading struct {
	DataSourceURI string  `json:"DataSourceUri"`
	Reading       float64 `json:"Reading"`
	ApparentVA    float64 `json:"ApparentVA"`
	ReactiveVAR   float64 `json:"ReactiveVAR"`
	PowerFactor   float64 `json:"PowerFactor"`
}

func NewClient(baseURL *url.URL, logger log.MinInterface, username, password string, timeout time.Duration) Client {
	if logger == nil {
		logger = log.Default()
	}

	return &ClientImpl{
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
func (c *ClientImpl) Close(ctx context.Context) error {
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

func (c *ClientImpl) get(ctx context.Context, path string, result any) error {
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
func (c *ClientImpl) GetPowerEquipment(ctx context.Context) (*PowerEquipment, error) {
	var result PowerEquipment
	if err := c.get(ctx, "/redfish/v1/PowerEquipment", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRackPDUs retrieves the Rack PDUs collection
func (c *ClientImpl) GetRackPDUs(ctx context.Context) (*RackPDUsCollection, error) {
	var result RackPDUsCollection
	if err := c.get(ctx, "/redfish/v1/PowerEquipment/RackPDUs", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPDU retrieves details for a specific PDU
func (c *ClientImpl) GetPERackPDU(ctx context.Context, pduID string) (*RackPDU, error) {
	var result RackPDU
	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPDUMetrics retrieves metrics for a specific PDU
func (c *ClientImpl) GetPERackPDUMetrics(ctx context.Context, pduID string) (*PDUMetrics, error) {
	var result PDUMetrics
	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Metrics", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
