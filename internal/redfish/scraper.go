// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/manager"
)

// Scrape implements the redfish scrape function and satisfies the manager scraper signature.
func Scrape(_ context.Context, _ manager.ScrapeJob) (int, error) { //nolint:gocritic // must match manager scraper map
	// implement redfish scraper logic
	return 0, nil
}
