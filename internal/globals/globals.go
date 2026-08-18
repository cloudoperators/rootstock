// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

// SPDX-FileCopyrightText: rootstock contributors
// SPDX-License-Identifier: Apache-2.0

// Package globals holds constants shared across the rootstock service:
// HTTP endpoint paths, default port, and the service name.
package globals

const (
	MetricsEndpoint = "/metrics"
	HealthEndpoint  = "/healthz"
	ReadyEndpoint   = "/ready"
	ProbeEndpoint   = "/probe"
	DefaultPort     = 9220
	ServiceName     = "rootstock"
)
