// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadFilterContent reads the example filter definition used across the tests.
func loadFilterContent(t *testing.T) []byte {
	t.Helper()

	content, err := os.ReadFile("filter.yaml")
	require.NoError(t, err)

	return content
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name          string
		filterContent []byte
		wantErr       string
		wantProtocol  protocol
		wantManuf     string
	}{
		{
			name:          "parses the example filter.yaml",
			filterContent: loadFilterContent(t),
			wantProtocol:  Redfish,
			wantManuf:     "Panduit",
		},
		{
			name:          "returns error on invalid yaml",
			filterContent: []byte("filters: [this is not valid"),
			wantErr:       "parsing filter content error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Update(tt.filterContent)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)

			manufacturers, ok := filterConfig.Filters[tt.wantProtocol]
			require.True(t, ok, "expected protocol %q to be parsed", tt.wantProtocol)

			_, ok = manufacturers[tt.wantManuf]
			assert.True(t, ok, "expected manufacturer %q to be parsed", tt.wantManuf)
		})
	}
}

func TestGetRoutes(t *testing.T) {
	require.NoError(t, Update(loadFilterContent(t)))

	tests := []struct {
		name         string
		protocol     protocol
		manufacturer string
		wantRoutes   []string
		wantErr      string
	}{
		{
			name:         "returns all routes for redfish/Panduit",
			protocol:     Redfish,
			manufacturer: "Panduit",
			wantRoutes: []string{
				"/redfish/v1/PowerEquipment/RackPDUs/#/Branches/#",
				"/redfish/v1/PowerEquipment/RackPDUs/#/Mains/#",
				"/redfish/v1/PowerEquipment/RackPDUs/#/Metrics",
			},
		},
		{
			name:         "returns error for protocol without filters",
			protocol:     SNMP,
			manufacturer: "Panduit",
			wantErr:      "unknown protocol",
		},
		{
			name:         "returns error for invalid protocol",
			protocol:     "bogus",
			manufacturer: "Panduit",
			wantErr:      "unknown protocol",
		},
		{
			name:         "returns error for unknown manufacturer",
			protocol:     Redfish,
			manufacturer: "Unknown",
			wantErr:      "unknown manufacturer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := GetRoutes(tt.protocol, tt.manufacturer)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			sort.Strings(routes)
			assert.Equal(t, tt.wantRoutes, routes)
		})
	}
}

func TestGetRules(t *testing.T) {
	require.NoError(t, Update(loadFilterContent(t)))

	tests := []struct {
		name            string
		protocol        protocol
		manufacturer    string
		wantRouteCounts map[string]int
		wantErr         string
	}{
		{
			name:         "returns rules keyed by route for redfish/Panduit",
			protocol:     Redfish,
			manufacturer: "Panduit",
			wantRouteCounts: map[string]int{
				"/redfish/v1/PowerEquipment/RackPDUs/#/Metrics":    5,
				"/redfish/v1/PowerEquipment/RackPDUs/#/Mains/#":    1,
				"/redfish/v1/PowerEquipment/RackPDUs/#/Branches/#": 4,
			},
		},
		{
			name:         "returns error for protocol without filters",
			protocol:     SNMP,
			manufacturer: "Panduit",
			wantErr:      "unknown protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := GetRules(tt.protocol, tt.manufacturer)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Len(t, rules, len(tt.wantRouteCounts))

			for route, count := range tt.wantRouteCounts {
				assert.Len(t, rules[route], count, "route %q", route)
			}
		})
	}
}

func TestGetRule(t *testing.T) {
	require.NoError(t, Update(loadFilterContent(t)))

	tests := []struct {
		name         string
		protocol     protocol
		manufacturer string
		rule         string
		wantValue    string
		wantType     string
		wantErr      string
	}{
		{
			name:         "returns matching rule by key",
			protocol:     Redfish,
			manufacturer: "Panduit",
			rule:         "power_factor",
			wantValue:    ".PowerWatts.PowerFactor",
			wantType:     "gauge",
		},
		{
			name:         "resolves YAML alias labels",
			protocol:     Redfish,
			manufacturer: "Panduit",
			rule:         "frequency_hz",
			wantValue:    ".FrequencyHz.Reading",
			wantType:     "gauge",
		},
		{
			name:         "returns error for unknown rule",
			protocol:     Redfish,
			manufacturer: "Panduit",
			rule:         "does_not_exist",
			wantErr:      "unknown rule",
		},
		{
			name:         "returns error for unknown manufacturer",
			protocol:     Redfish,
			manufacturer: "Unknown",
			rule:         "power_factor",
			wantErr:      "unknown manufacturer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := GetRule(tt.protocol, tt.manufacturer, tt.rule)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.rule, rule.Key)
			assert.Equal(t, tt.wantValue, rule.Value)
			assert.Equal(t, tt.wantType, rule.Type)
			assert.NotEmpty(t, rule.Labels, "common ID labels should be resolved via YAML alias")
			assert.Equal(t, `."@odata.id"`, rule.Labels["uri"])
			assert.Equal(t, ".Id", rule.Labels["id"])
			// Origin labels are injected by Update.
			assert.Equal(t, string(Redfish), rule.Labels[LabelProtocol])
			assert.Equal(t, "Panduit", rule.Labels[LabelManufacturer])
			assert.NotEmpty(t, rule.Labels[LabelRoute])
		})
	}
}

func TestValidateFilters(t *testing.T) {
	const route = "/redfish/v1/PowerEquipment/RackPDUs/#/Metrics"

	tests := []struct {
		name    string
		file    filterFile
		wantErr string
	}{
		{
			name: "unique (key, labels) within a route passes",
			file: filterFile{
				Filters: map[protocol]map[string]manufacturerFilters{
					Redfish: {
						"Panduit": {
							Rules: map[string][]Rule{
								route: {
									{Key: "power_watts", Labels: map[string]string{"id": ".Id"}},
									{Key: "energy_kwh", Labels: map[string]string{"id": ".Id"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "same key with different labels within a route passes",
			file: filterFile{
				Filters: map[protocol]map[string]manufacturerFilters{
					Redfish: {
						"Panduit": {
							Rules: map[string][]Rule{
								route: {
									{Key: "power_watts", Labels: map[string]string{"line": "L1"}},
									{Key: "power_watts", Labels: map[string]string{"line": "L2"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "same (key, labels) across different routes passes",
			file: filterFile{
				Filters: map[protocol]map[string]manufacturerFilters{
					Redfish: {
						"Panduit": {
							Rules: map[string][]Rule{
								route:        {{Key: "power_watts", Labels: map[string]string{"id": ".Id"}}},
								route + "/2": {{Key: "power_watts", Labels: map[string]string{"id": ".Id"}}},
							},
						},
					},
				},
			},
		},
		{
			name: "duplicate (key, labels) within a route fails",
			file: filterFile{
				Filters: map[protocol]map[string]manufacturerFilters{
					Redfish: {
						"Panduit": {
							Rules: map[string][]Rule{
								route: {
									{Key: "power_watts", Labels: map[string]string{"id": ".Id"}},
									{Key: "power_watts", Labels: map[string]string{"id": ".Id"}},
								},
							},
						},
					},
				},
			},
			wantErr: "duplicate rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilters(tt.file)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			assert.NoError(t, err)
		})
	}
}

// TestConcurrentAccess exercises simultaneous Update writes and Get* reads so
// the race detector can confirm filterConfig access is properly synchronized.
func TestConcurrentAccess(t *testing.T) {
	content := loadFilterContent(t)
	require.NoError(t, Update(content))

	const goroutines = 16

	var wg sync.WaitGroup

	wg.Add(goroutines * 2)

	for range goroutines {
		// Writers continuously reload the config.
		go func() {
			defer wg.Done()

			for range 100 {
				_ = Update(content)
			}
		}()

		// Readers continuously hit every accessor.
		go func() {
			defer wg.Done()

			for range 100 {
				_, _ = GetRoutes(Redfish, "Panduit")
				_, _ = GetRules(Redfish, "Panduit")
				_, _ = GetRule(Redfish, "Panduit", "power_factor")
			}
		}()
	}

	wg.Wait()
}
