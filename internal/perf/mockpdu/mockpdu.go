// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package mockpdu provides a fixture-backed, in-memory implementation of the
// redfish.Client interface for use in performance benchmarks.
//
// It represents the "mock PDU" edge of the processing pipeline: benchmarks feed
// mocked PDU data in through this client without touching the network, so the
// measurement captures processing cost rather than I/O latency. The fixtures
// are a copy of the DMTF Redfish PDU mockup also used by the integration tests
// (testdata/pdu-mockup) and are embedded at build time so the mock is
// self-contained and independent of the working directory.
package mockpdu

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cloudoperators/rootstock/internal/redfish"
)

// fixtures embeds the per-protocol fixtures required to drive the processing
// pipeline. Each protocol has two documents: its PDU resource (<protocol>.json)
// and its metrics (<protocol>-metrics.json). Today only the Redfish fixture
// exists; an SNMP fixture would be added as snmp.json / snmp-metrics.json.
//
//go:embed fixtures/*.json
var fixtures embed.FS

// Fixture holds the pre-decoded Redfish documents for a single mocked PDU.
// Decoding happens once when the mock is constructed so that per-iteration
// benchmark cost reflects pipeline processing, not JSON parsing of the fixture.
type Fixture struct {
	PDU     redfish.RackPDU
	Metrics redfish.PDUMetrics
}

// Client is a fixture-backed redfish.Client. It is safe for concurrent use:
// all state is populated at construction time and only read afterwards.
type Client struct {
	pdus map[string]Fixture
}

// compile-time assertion that the mock satisfies the production interface.
var _ redfish.Client = (*Client)(nil)

// Protocol identifies which device protocol a fixture represents. It doubles
// as the fixture file stem (fixtures/<protocol>.json and
// fixtures/<protocol>-metrics.json) and as the default PDU key exposed by the
// mock client. Naming by protocol keeps the fixtures self-documenting and makes
// the extension path for future protocols (e.g. SNMP) explicit.
type Protocol string

const (
	// Redfish is a representative, fully-populated Redfish RackPDU fixture.
	Redfish Protocol = "redfish"
)

// New returns a mock redfish.Client backed by the embedded Redfish PDU fixture,
// exposed under the "redfish" key.
//
// It represents a single representative PDU, which is sufficient to characterize
// per-PDU processing cost. Use NewScaled to replicate it into many PDUs for
// horizontal-scaling benchmarks.
func New() (*Client, error) {
	f, err := loadFixture(Redfish)
	if err != nil {
		return nil, err
	}

	return &Client{pdus: map[string]Fixture{string(Redfish): f}}, nil
}

// NewScaled returns a mock client exposing n PDUs, all cloned from the Redfish
// fixture and keyed "redfish-1".."redfish-n". It is used to benchmark how the
// processing pipeline scales with the number of PDUs ("mass PDU scanning /
// horizontal scaling").
func NewScaled(n int) (*Client, error) {
	if n < 1 {
		return nil, fmt.Errorf("mockpdu: n must be >= 1, got %d", n)
	}

	base, err := loadFixture(Redfish)
	if err != nil {
		return nil, err
	}

	pdus := make(map[string]Fixture, n)
	for i := 1; i <= n; i++ {
		id := string(Redfish) + "-" + strconv.Itoa(i)

		f := base // struct copy; redfish structs are value types
		f.PDU.ID = i
		f.PDU.Name = id
		pdus[id] = f
	}

	return &Client{pdus: pdus}, nil
}

// IDs returns the PDU IDs the mock client can serve.
func (c *Client) IDs() []string {
	ids := make([]string, 0, len(c.pdus))
	for id := range c.pdus {
		ids = append(ids, id)
	}

	return ids
}

// Login is a no-op for the mock; no session is required to read fixtures.
func (*Client) Login(context.Context) error { return nil }

// Close is a no-op for the mock.
func (*Client) Close(context.Context) error { return nil }

// GetPowerEquipment returns a minimal PowerEquipment document pointing at the
// RackPDUs collection.
func (*Client) GetPowerEquipment(context.Context) (*redfish.PowerEquipment, error) {
	pe := &redfish.PowerEquipment{
		ODataID: "/redfish/v1/PowerEquipment",
		Name:    "PowerEquipment",
	}
	pe.RackPDUs.ODataID = "/redfish/v1/PowerEquipment/RackPDUs"

	return pe, nil
}

// GetRackPDUs returns the collection of mocked RackPDUs.
func (c *Client) GetRackPDUs(context.Context) (*redfish.RackPDUsCollection, error) {
	members := make([]redfish.Link, 0, len(c.pdus))
	for id := range c.pdus {
		members = append(members, redfish.Link{
			ODataID: "/redfish/v1/PowerEquipment/RackPDUs/" + id,
		})
	}

	return &redfish.RackPDUsCollection{
		ODataID:      "/redfish/v1/PowerEquipment/RackPDUs",
		Name:         "RackPDU Collection",
		Members:      members,
		MembersCount: len(members),
	}, nil
}

// GetPERackPDU returns the resource document for a single mocked PDU.
func (c *Client) GetPERackPDU(_ context.Context, pduID string) (*redfish.RackPDU, error) {
	f, ok := c.pdus[pduID]
	if !ok {
		return nil, fmt.Errorf("mockpdu: unknown PDU %q", pduID)
	}

	pdu := f.PDU

	return &pdu, nil
}

// GetPERackPDUMetrics returns the metrics document for a single mocked PDU.
func (c *Client) GetPERackPDUMetrics(_ context.Context, pduID string) (*redfish.PDUMetrics, error) {
	f, ok := c.pdus[pduID]
	if !ok {
		return nil, fmt.Errorf("mockpdu: unknown PDU %q", pduID)
	}

	metrics := f.Metrics

	return &metrics, nil
}

// loadFixture decodes the embedded resource and metrics documents for the given
// protocol fixture (fixtures/<protocol>.json and fixtures/<protocol>-metrics.json).
func loadFixture(proto Protocol) (Fixture, error) {
	var f Fixture

	stem := "fixtures/" + string(proto)

	if err := decode(stem+".json", &f.PDU); err != nil {
		return Fixture{}, err
	}

	if err := decode(stem+"-metrics.json", &f.Metrics); err != nil {
		return Fixture{}, err
	}

	return f, nil
}

func decode(name string, out any) error {
	data, err := fixtures.ReadFile(name)
	if err != nil {
		return fmt.Errorf("mockpdu: read fixture %s: %w", name, err)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("mockpdu: decode fixture %s: %w", name, err)
	}

	return nil
}
