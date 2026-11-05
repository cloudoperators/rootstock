// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// Package mockpdu provides fixture-based, in-memory PDU sources for rootstock's
// performance tests. Each source satisfies performance.Source, feeding mocked
// PDU data into the processing pipeline without touching the network so the
// measurement captures processing cost rather than I/O latency.
//
// The Redfish source lives in redfish_pdu.go and the SNMP source in snmp_pdu.go;
// both are constructed by protocol (NewRedfish, NewSNMP) and return a
// performance.Source. The shared fixture-loading helpers live here.
//
// Fixtures are read once at construction from the DMTF Redfish PDU mockup under
// testdata/pdu-mockup, the same data the integration tests use, so there is a
// single source of truth for the mocked device data.
package mockpdu

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Protocol identifies which device protocol a fixture represents. It maps to a
// subtree of the testdata/pdu-mockup directory and doubles as the default PDU
// key exposed by a mock source. Naming by protocol keeps the source data
// self-documenting and makes the extension path for future protocols (for
// example SNMP) explicit.
type Protocol string

const (
	// Redfish is a representative, fully-populated Redfish RackPDU.
	Redfish Protocol = "redfish"
	// SNMP is the SNMP PDU protocol. It is not implemented yet: fixturePaths
	// has no SNMP case, so NewSNMP returns an "unsupported protocol" error.
	SNMP Protocol = "snmp"
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

// decode reads and JSON-decodes a single fixture document into out.
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
