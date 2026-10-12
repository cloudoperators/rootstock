// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import "context"

type Scraper struct {
	manager *manager
}

func newScraper(manager *manager) *Scraper {
	return &Scraper{manager: manager}
}

func (*Scraper) run(_ context.Context) {
	// implement scraper logic
}
