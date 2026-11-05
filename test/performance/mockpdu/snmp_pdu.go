// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package mockpdu

import (
	"github.com/cloudoperators/rootstock/test/performance"
)

// NewSNMP returns an SNMP mock source loaded from the SNMP fixtures, mirroring
// NewRedfish. It is the symmetric entry point a future snmp_processing_test.go
// uses to instantiate an SNMP-fixture-backed PDU through the protocol-neutral
// performance.Source abstraction.
//
// SNMP is not implemented yet: fixturePaths has no SNMP case, so loadFixture(SNMP)
// returns an "unsupported protocol" error and NewSNMP returns it unchanged. When
// an SNMP wire format and fixtures exist, add the SNMP case to fixturePaths and
// build the source here; callers do not change.
func NewSNMP() (performance.Source, error) {
	if _, err := loadFixture(SNMP); err != nil {
		return nil, err
	}

	// Unreachable until fixturePaths gains an SNMP case; kept so the symmetry
	// with NewRedfish is explicit.
	return nil, nil
}
