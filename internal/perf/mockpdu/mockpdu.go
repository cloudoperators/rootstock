// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package mockpdu provides a fixture-based, in-memory implementation of the
// redfish.Client interface for use in performance benchmarks.
//
// It represents the "mock PDU" edge of the processing pipeline: benchmarks feed
// mocked PDU data in through this client without touching the network, so the
// measurement captures processing cost rather than I/O latency. The fixtures
// are read once at startup from the DMTF Redfish PDU mockup under
// testdata/pdu-mockup — the same data the integration tests use — so there is a
// single source of truth for the mocked device data.
package mockpdu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/cloudoperators/rootstock/internal/redfish"
)

// mockupRoot resolves the testdata/pdu-mockup directory relative to this source
// file, so fixtures load correctly regardless of the process working directory
// (Go runs benchmarks from their package directory, not the repo root).
func mockupRoot() string {
	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled // only the file path is needed
	// see comment above on why we need this ugly triple parent hop.
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	return filepath.Join(repoRoot, "testdata", "pdu-mockup")
}

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

// Protocol identifies which device protocol a fixture represents. It maps to a
// subtree of the testdata/pdu-mockup directory and doubles as the default PDU
// key exposed by the mock client. Naming by protocol keeps the source data
// self-documenting and makes the extension path for future protocols (e.g.
// SNMP) explicit.
type Protocol string

const (
	// Redfish is a representative, fully-populated Redfish RackPDU.
	Redfish Protocol = "redfish"
)

// New returns a mock redfish.Client backed by the Redfish PDU fixture read from
// testdata/pdu-mockup, exposed under the "redfish" key.
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

// loadFixture reads and decodes the PDU resource and metrics documents for the
// given protocol from the testdata/pdu-mockup directory. Reading happens once at
// construction time, before the benchmark timer starts.
func loadFixture(proto Protocol) (Fixture, error) {
	var f Fixture

	pduPath, metricsPath, err := fixturePaths(proto)
	if err != nil {
		return Fixture{}, err
	}

	if err := decode(pduPath, &f.PDU); err != nil {
		return Fixture{}, err
	}

	if err := decode(metricsPath, &f.Metrics); err != nil {
		return Fixture{}, err
	}

	return f, nil
}

// fixturePaths maps a protocol to the resource and metrics documents within the
// testdata/pdu-mockup directory. Each supported protocol declares its own paths
// here; adding SNMP means adding a case, not restructuring the mock.
func fixturePaths(proto Protocol) (pduPath, metricsPath string, err error) {
	root := mockupRoot()

	switch proto {
	case Redfish:
		base := filepath.Join(root, "redfish", "v1", "PowerEquipment", "RackPDUs", "1")

		return filepath.Join(base, "index.json"),
			filepath.Join(base, "Metrics", "index.json"),
			nil
	default:
		return "", "", fmt.Errorf("mockpdu: unsupported protocol %q", proto)
	}
}

func decode(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("mockpdu: read fixture %s: %w", path, err)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("mockpdu: decode fixture %s: %w", path, err)
	}

	return nil
}
