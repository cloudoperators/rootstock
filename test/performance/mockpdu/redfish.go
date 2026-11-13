// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package mockpdu

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/test/performance"
)

// RedfishSource serves Redfish documents and scraped readings from immutable fixtures.
// Reads are safe to run concurrently.
type RedfishSource struct {
	pdus map[string]redfishFixture
}

var (
	_ redfish.Client     = (*RedfishSource)(nil)
	_ performance.Source = (*RedfishSource)(nil)
)

// NewRedfish loads PDU 1 from fixtureDirectory and exposes it as "redfish".
// fixtureDirectory must contain the redfish/v1 tree (e.g. testdata/pdu-mockup).
func NewRedfish(fixtureDirectory string) (performance.Source, error) {
	fixture, err := loadRedfishFixture(fixtureDirectory)
	if err != nil {
		return nil, err
	}

	return &RedfishSource{pdus: map[string]redfishFixture{"redfish": fixture}}, nil
}

// NewRedfishScaled loads PDU 1 once and copies it into n devices named
// "redfish-1" through "redfish-n". Measurements are identical for every device.
// fixtureDirectory must contain the redfish/v1 tree.
func NewRedfishScaled(fixtureDirectory string, n int) (performance.Source, error) {
	if n < 1 {
		return nil, fmt.Errorf("mockpdu: n must be >= 1, got %d", n)
	}

	base, err := loadRedfishFixture(fixtureDirectory)
	if err != nil {
		return nil, err
	}

	pdus := make(map[string]redfishFixture, n)
	for i := 1; i <= n; i++ {
		id := "redfish-" + strconv.Itoa(i)

		fixture := base
		fixture.pdu.ID = i
		fixture.pdu.Name = id
		pdus[id] = fixture
	}

	return &RedfishSource{pdus: pdus}, nil
}

// IDs returns PDU IDs in lexicographic order.
func (c *RedfishSource) IDs() []string {
	ids := make([]string, 0, len(c.pdus))
	for id := range c.pdus {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// Read maps the fixture measurements to a reading identified by pduID, not the
// metrics document's resource ID ("Metrics").
func (c *RedfishSource) Read(_ context.Context, pduID string) (performance.Reading, error) {
	fixture, err := c.lookup(pduID)
	if err != nil {
		return performance.Reading{}, err
	}

	return performance.Reading{
		PDUID:           pduID,
		PowerWatts:      fixture.metrics.PowerWatts.Reading,
		ApparentVA:      fixture.metrics.PowerWatts.ApparentVA,
		ReactiveVAR:     fixture.metrics.PowerWatts.ReactiveVAR,
		PowerFactor:     fixture.metrics.PowerWatts.PowerFactor,
		EnergykWh:       fixture.metrics.EnergykWh.Reading,
		PowerCapacityVA: fixture.pdu.PowerCapacityVA,
	}, nil
}

// Login requires no authentication for in-memory fixtures.
func (*RedfishSource) Login(context.Context) error { return nil }

// Close has no connections or background workers to release.
func (*RedfishSource) Close(context.Context) error { return nil }

// GetPowerEquipment returns a minimal PowerEquipment document pointing at the
// RackPDUs collection.
func (*RedfishSource) GetPowerEquipment(context.Context) (*redfish.PowerEquipment, error) {
	pe := &redfish.PowerEquipment{
		ODataID: "/redfish/v1/PowerEquipment",
		Name:    "PowerEquipment",
	}
	pe.RackPDUs.ODataID = "/redfish/v1/PowerEquipment/RackPDUs"

	return pe, nil
}

// GetRackPDUs returns the collection of mocked RackPDUs.
func (c *RedfishSource) GetRackPDUs(context.Context) (*redfish.RackPDUsCollection, error) {
	members := make([]redfish.Link, 0, len(c.pdus))
	for _, id := range c.IDs() {
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

// GetPERackPDU returns a copy so callers cannot modify the stored fixture.
func (c *RedfishSource) GetPERackPDU(_ context.Context, pduID string) (*redfish.RackPDU, error) {
	fixture, err := c.lookup(pduID)
	if err != nil {
		return nil, err
	}

	return &fixture.pdu, nil
}

// GetPERackPDUMetrics returns a copy so callers cannot modify the stored fixture.
func (c *RedfishSource) GetPERackPDUMetrics(_ context.Context, pduID string) (*redfish.PDUMetrics, error) {
	fixture, err := c.lookup(pduID)
	if err != nil {
		return nil, err
	}

	return &fixture.metrics, nil
}

func (c *RedfishSource) lookup(pduID string) (redfishFixture, error) {
	fixture, ok := c.pdus[pduID]
	if !ok {
		return redfishFixture{}, fmt.Errorf("mockpdu: unknown PDU %q", pduID)
	}

	return fixture, nil
}
