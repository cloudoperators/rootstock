// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudoperators/rootstock/pkg/log"
)

// FlexID unmarshals a Redfish "Id" field that may be either a JSON string
// ("Metrics") or a JSON number (1) depending on the vendor firmware.
// The Panduit G6 returns numeric IDs; the DMTF mock returns string IDs.
type FlexID string

func (f *FlexID) UnmarshalJSON(b []byte) error {
	// Try string first (DMTF mock, most vendors)
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = FlexID(s)
		return nil
	}
	// Fall back to number (Panduit G6 firmware)
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("Id field: expected string or number, got %s", b)
	}
	*f = FlexID(n.String())
	return nil
}

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
	// GetPERackPDU returns details for a single Rack PDU by ID.
	GetPERackPDU(context.Context, string) (*RackPDU, error)
	// GetPERackPDUMetrics returns the metrics for a single Rack PDU by ID.
	GetPERackPDUMetrics(context.Context, string) (*PDUMetrics, error)
}

type ClientImpl struct {
	baseURL    *url.URL     // Base URL of the PDU Redfish API (e.g., https://pdu01.example.com)
	httpClient *http.Client // HTTP client with timeout and TLS configuration

	username string // Authentication username
	password string // Authentication password

	logger  log.MinInterface // Structured logger for debugging and audit
	timeout time.Duration
}

// PowerEquipment represents the PowerEquipment collection.
type PowerEquipment struct {
	ODataID   string `json:"@odata.id"`
	ODataType string `json:"@odata.type"`
	Name      string `json:"Name"`
	RackPDUs  struct {
		ODataID string `json:"@odata.id"`
	} `json:"RackPDUs"`
}

// RackPDUsCollection represents the collection of RackPDUs.
type RackPDUsCollection struct {
	ODataID      string `json:"@odata.id"`
	ODataType    string `json:"@odata.type"`
	Name         string `json:"Name"`
	Members      []Link `json:"Members"`
	MembersCount int    `json:"Members@odata.count"`
}

// Link represents a Redfish link object.
type Link struct {
	ODataID string `json:"@odata.id"`
}

// RackPDU represents a single RackPDU device.
type RackPDU struct {
	ODataID         string `json:"@odata.id"`
	ODataType       string `json:"@odata.type"`
	ID              FlexID `json:"Id"`
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

// Status represents the status of a Redfish resource.
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// PDUMetrics represents the metrics for a PDU.
type PDUMetrics struct {
	ODataID            string        `json:"@odata.id"`
	ODataType          string        `json:"@odata.type"`
	ID                 string        `json:"Id"`
	Name               string        `json:"Name"`
	PowerWatts         SensorReading `json:"PowerWatts"`
	EnergykWh          SensorReading `json:"EnergykWh"`
	TemperatureCelsius SensorReading `json:"TemperatureCelsius"`
}

// SensorReading represents a sensor reading with value and metadata.
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
				// PDUs ship with self-signed certificates, so certificate verification is intentionally skipped.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // G402: self-signed PDU certs
				// Force Connection: close so the PDU frees its connection slot after each
				// request. Embedded PDU firmware (e.g. Panduit G6) allows very few
				// simultaneous connections; keeping them open causes subsequent scrapes
				// to hit the limit and hang.
				DisableKeepAlives: true,
			},
		},
		baseURL:  baseURL,
		logger:   logger,
		timeout:  timeout,
		username: username,
		password: password,
	}
}

// Login is a no-op for basic authentication.
//
// The Panduit G6 (and the Redfish data endpoints generally) accept HTTP Basic
// Auth on every request, so there is no session to establish. Credentials are
// attached per request in get(). The method is kept to satisfy the Client
// interface and the existing call sites.
func (c *ClientImpl) Login(_ context.Context) error {
	return nil
}

// Close is a no-op for basic authentication.
//
// There is no Redfish session to tear down when using Basic Auth.
func (c *ClientImpl) Close(_ context.Context) error {
	return nil
}

func (c *ClientImpl) get(ctx context.Context, path string, result any) error {
	fullURL := c.baseURL.ResolveReference(&url.URL{Path: path})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), http.NoBody)
	if err != nil {
		return err
	}

	// PDUs accept HTTP Basic Auth on every request; no session token is used.
	req.SetBasicAuth(c.username, c.password)

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
func (c *ClientImpl) GetPERackPDU(ctx context.Context, pduID string) (*RackPDU, error) {
	var result RackPDU

	endpoint := "/redfish/v1/PowerEquipment/RackPDUs/" + pduID
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetPDUMetrics retrieves metrics for a specific PDU.
func (c *ClientImpl) GetPERackPDUMetrics(ctx context.Context, pduID string) (*PDUMetrics, error) {
	var result PDUMetrics

	endpoint := fmt.Sprintf("/redfish/v1/PowerEquipment/RackPDUs/%s/Metrics", pduID)
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
