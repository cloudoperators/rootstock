// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package mockpdu provides in-memory PDU sources loaded from JSON fixtures.
// Constructors read the fixtures once; subsequent reads require no file or
// network access.
package mockpdu

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudoperators/rootstock/internal/redfish"
)

type redfishFixture struct {
	pdu     redfish.RackPDU
	metrics redfish.PDUMetrics
}

// fixtureDirectory is the directory containing the mock server's redfish tree,
// normally testdata/pdu-mockup. PDU 1 is the template for all mock devices.
func loadRedfishFixture(fixtureDirectory string) (redfishFixture, error) {
	deviceDirectory := filepath.Join(fixtureDirectory, "redfish", "v1", "PowerEquipment", "RackPDUs", "1")

	var fixture redfishFixture
	if err := readJSONFixture(filepath.Join(deviceDirectory, "index.json"), &fixture.pdu); err != nil {
		return redfishFixture{}, err
	}

	if err := readJSONFixture(filepath.Join(deviceDirectory, "Metrics", "index.json"), &fixture.metrics); err != nil {
		return redfishFixture{}, err
	}

	return fixture, nil
}

func readJSONFixture(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("mockpdu: read fixture %s: %w", path, err)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("mockpdu: decode fixture %s: %w", path, err)
	}

	return nil
}
