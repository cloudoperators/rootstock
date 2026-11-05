// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package mockpdu

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/test/performance"
)

// Fixture holds the pre-decoded Redfish documents for a single mocked PDU.
// Decoding happens once when the mock is constructed so that per-iteration
// benchmark cost reflects pipeline processing, not JSON parsing of the fixture.
type Fixture struct {
	PDU     redfish.RackPDU
	Metrics redfish.PDUMetrics
}

// Client is a fixture-backed redfish.Client and a performance.Source for the
// Redfish protocol. It is safe for concurrent use: all state is populated at
// construction time and only read afterwards.
type Client struct {
	pdus map[string]Fixture
}

// compile-time assertions that the mock satisfies the production client
// interface and the performance Source abstraction.
var (
	_ redfish.Client     = (*Client)(nil)
	_ performance.Source = (*Client)(nil)
)

// NewRedfish returns a Redfish mock source backed by the Redfish PDU fixture
// read from testdata/pdu-mockup, exposed under the "redfish" key.
//
// It represents a single representative PDU, which is sufficient to characterize
// per-PDU processing cost. Use NewRedfishScaled to replicate it into many PDUs
// for horizontal-scaling benchmarks.
func NewRedfish() (performance.Source, error) {
	f, err := loadFixture(Redfish)
	if err != nil {
		return nil, err
	}

	return &Client{pdus: map[string]Fixture{string(Redfish): f}}, nil
}

// NewRedfishScaled returns a Redfish mock source exposing n PDUs, all cloned from
// the Redfish fixture and keyed "redfish-1".."redfish-n". It is used to benchmark
// how the processing pipeline scales with the number of PDUs ("mass PDU
// scanning / horizontal scaling").
func NewRedfishScaled(n int) (performance.Source, error) {
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

// IDs returns the PDU IDs the mock source can serve.
func (c *Client) IDs() []string {
	ids := make([]string, 0, len(c.pdus))
	for id := range c.pdus {
		ids = append(ids, id)
	}

	return ids
}

// Read returns the neutral performance.Reading for a single mocked PDU, mapping
// the Redfish documents into the protocol-agnostic form.
func (c *Client) Read(_ context.Context, pduID string) (performance.Reading, error) {
	f, ok := c.pdus[pduID]
	if !ok {
		return performance.Reading{}, fmt.Errorf("mockpdu: unknown PDU %q", pduID)
	}

	return performance.Reading{
		PDUID:           f.Metrics.ID,
		PowerWatts:      f.Metrics.PowerWatts.Reading,
		ApparentVA:      f.Metrics.PowerWatts.ApparentVA,
		ReactiveVAR:     f.Metrics.PowerWatts.ReactiveVAR,
		PowerFactor:     f.Metrics.PowerWatts.PowerFactor,
		EnergykWh:       f.Metrics.EnergykWh.Reading,
		PowerCapacityVA: f.PDU.PowerCapacityVA,
	}, nil
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

// loadFixture reads and decodes the Redfish PDU resource and metrics documents
// from the testdata/pdu-mockup directory. Reading happens once at construction
// time, before the benchmark timer starts.
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

// fixturePaths maps the Redfish protocol to its resource and metrics documents
// within the testdata/pdu-mockup directory.
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
