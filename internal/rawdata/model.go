// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import "time"

// RawMetric is the common output of any scraper before translation into Prometheus metrics.
type RawMetric struct {
	DeviceIP  string
	Protocol  string
	Key       string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}
