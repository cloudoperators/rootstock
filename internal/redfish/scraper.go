// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"

	"github.com/cloudoperators/rootstock/internal/manager"
)

type Scraper struct{}

func (*Scraper) Scrape(_ context.Context, _ manager.ScrapeJob) manager.ScrapeResult {
	// implement redfish scraper logic
	return manager.ScrapeResult{}
}
