// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package mockpdu

import (
	"errors"

	"github.com/cloudoperators/rootstock/test/performance/harness"
)

// NewSNMP returns an error until an SNMP fixture format and source are implemented.
func NewSNMP() (harness.Source, error) {
	return nil, errors.New("mockpdu: SNMP source is not implemented")
}
