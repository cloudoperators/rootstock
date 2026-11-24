// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFilterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
		assert  func(t *testing.T, cfg *FilterConfig)
	}{
		{
			name: "parses the repository filter.yaml layout",
			yaml: mustReadFile(t, "filter.yaml"),
			assert: func(t *testing.T, cfg *FilterConfig) {
				// SNMP flat key rule.
				snmp := cfg.Filters.SNMP.Keys
				require.Contains(t, snmp, "hw_current")
				assert.Equal(t, "current", snmp["hw_current"].NewKey)

				// Redfish vendor -> route -> key rule.
				route := cfg.Filters.Redfish["Panduit1"]["/redfish/v1/PowerEquipment/RackPDUs/#/Metrics"]
				rule, ok := route.Keys[".PowerWatts.Reading"]
				require.True(t, ok)
				assert.Equal(t, "power", rule.NewKey)
				assert.Equal(t, "Watt", rule.Labels.Add["unit"])
				assert.Equal(t, ".PowerWatts.DataSourceUri", rule.Labels.Add["uri"])
				assert.ElementsMatch(t, []string{".Id", ".Name"}, rule.Labels.Remove)
			},
		},
		{
			name:    "returns error for invalid yaml",
			yaml:    "filters: [this is not a map",
			wantErr: "failed to unmarshal filter config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := ParseFilterConfig([]byte(tt.yaml))

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.assert != nil {
				tt.assert(t, cfg)
			}
		})
	}
}

func TestLoadFilterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name: "loads the repository filter.yaml",
			path: "filter.yaml",
		},
		{
			name:    "returns error when file is missing",
			path:    filepath.Join(t.TempDir(), "does-not-exist.yaml"),
			wantErr: "failed to read filter config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := LoadFilterConfig(tt.path)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}

func TestFilterConfigFilterSNMP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *FilterConfig
		metric Metric
		assert func(t *testing.T, got Metric)
	}{
		{
			name: "renames matched key",
			config: &FilterConfig{Filters: Filters{SNMP: KeyFilter{Keys: KeyRules{
				"hw_current": {NewKey: "current"},
			}}}},
			metric: Metric{Protocol: SNMP, RawKey: RawKey{Name: "hw_current"}, Value: 25, Labels: map[string]any{"pdu": "1"}},
			assert: func(t *testing.T, got Metric) {
				assert.Equal(t, "current", got.FinalKey)
				assert.Equal(t, float64(25), got.Value)
			},
		},
		{
			name: "leaves unmatched key untouched",
			config: &FilterConfig{Filters: Filters{SNMP: KeyFilter{Keys: KeyRules{
				"hw_current": {NewKey: "current"},
			}}}},
			metric: Metric{Protocol: SNMP, RawKey: RawKey{Name: "model"}, Value: 1, Labels: map[string]any{"model": "EPA126"}},
			assert: func(t *testing.T, got Metric) {
				// Unmatched metric passes through with no FinalKey set.
				assert.Empty(t, got.FinalKey)
				assert.Equal(t, "model", got.RawKey.Name)
			},
		},
		{
			name: "adds constant and existing-label-reference labels, removes labels",
			config: &FilterConfig{Filters: Filters{SNMP: KeyFilter{Keys: KeyRules{
				"hw_current": {
					NewKey: "current",
					Labels: LabelRules{
						Add:    map[string]string{"unit": "Ampere", "circuit": "pduP2BrCktMonBrCctID"},
						Remove: []string{"pduP2BrCktMonBrCctBreakerOutletMap"},
					},
				},
			}}}},
			metric: Metric{
				Protocol: SNMP,
				RawKey:   RawKey{Name: "hw_current"},
				Value:    25,
				Labels: map[string]any{
					"pduP2BrCktMonBrCctID":               "Red-1 C1-A",
					"pduP2BrCktMonBrCctBreakerOutletMap": "1,2,3,4",
				},
			},
			assert: func(t *testing.T, got Metric) {
				labels := got.Labels
				// Constant add.
				assert.Equal(t, "Ampere", labels["unit"])
				// Reference add copies an existing label's value.
				assert.Equal(t, "Red-1 C1-A", labels["circuit"])
				// Remove drops by exact label name.
				assert.NotContains(t, labels, "pduP2BrCktMonBrCctBreakerOutletMap")
				// Original referenced label retained.
				assert.Equal(t, "Red-1 C1-A", labels["pduP2BrCktMonBrCctID"])
			},
		},
		{
			name:   "does not mutate the input metric labels",
			config: &FilterConfig{Filters: Filters{SNMP: KeyFilter{Keys: KeyRules{"hw_current": {NewKey: "current", Labels: LabelRules{Add: map[string]string{"unit": "Ampere"}}}}}}},
			metric: Metric{Protocol: SNMP, RawKey: RawKey{Name: "hw_current"}, Labels: map[string]any{"pdu": "1"}},
			assert: func(t *testing.T, got Metric) {
				assert.Contains(t, got.Labels, "unit")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Capture the original label map to assert non-mutation.
			original := copyLabels(tt.metric.Labels)

			got := tt.config.FilterSNMP(tt.metric)

			tt.assert(t, got)

			// Input must be unchanged.
			assert.Equal(t, original, tt.metric.Labels, "input metric labels were mutated")
		})
	}
}

func TestFilterConfigFilterRedfish(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFilterConfig("filter.yaml")
	require.NoError(t, err)

	tests := []struct {
		name    string
		vendor  string
		route   string
		metric  Metric
		payload map[string]any
		assert  func(t *testing.T, got Metric)
	}{
		{
			name:   "resolves json reference, adds constant, removes by trailing path element",
			vendor: "Panduit1",
			route:  "/redfish/v1/PowerEquipment/RackPDUs/#/Metrics",
			metric: Metric{
				Protocol: Redfish,
				RawKey:   RawKey{Name: ".PowerWatts.Reading"},
				Value:    123,
				Labels:   map[string]any{"Id": "abc", "Name": "pdu", "keep": "yes"},
			},
			payload: map[string]any{
				"PowerWatts": map[string]any{
					"Reading":       123.0,
					"DataSourceUri": "/redfish/v1/source",
				},
			},
			assert: func(t *testing.T, got Metric) {
				assert.Equal(t, "power", got.FinalKey)
				assert.Equal(t, "Watt", got.Labels["unit"])
				assert.Equal(t, "/redfish/v1/source", got.Labels["uri"])
				// ".Id" and ".Name" removed by trailing path element.
				assert.NotContains(t, got.Labels, "Id")
				assert.NotContains(t, got.Labels, "Name")
				assert.Equal(t, "yes", got.Labels["keep"])
			},
		},
		{
			name:   "unknown vendor passes through unchanged",
			vendor: "Unknown",
			route:  "/redfish/v1/PowerEquipment/RackPDUs/#/Metrics",
			metric: Metric{Protocol: Redfish, RawKey: RawKey{Name: ".PowerWatts.Reading"}},
			assert: func(t *testing.T, got Metric) {
				assert.Empty(t, got.FinalKey)
			},
		},
		{
			name:   "unknown route passes through unchanged",
			vendor: "Panduit1",
			route:  "/redfish/v1/other",
			metric: Metric{Protocol: Redfish, RawKey: RawKey{Name: ".PowerWatts.Reading"}},
			assert: func(t *testing.T, got Metric) {
				assert.Empty(t, got.FinalKey)
			},
		},
		{
			name:   "unmatched key passes through unchanged",
			vendor: "Panduit1",
			route:  "/redfish/v1/PowerEquipment/RackPDUs/#/Metrics",
			metric: Metric{Protocol: Redfish, RawKey: RawKey{Name: ".SomethingElse"}},
			assert: func(t *testing.T, got Metric) {
				assert.Empty(t, got.FinalKey)
			},
		},
		{
			name:   "missing json reference resolves to empty string",
			vendor: "Panduit1",
			route:  "/redfish/v1/PowerEquipment/RackPDUs/#/Metrics",
			metric: Metric{Protocol: Redfish, RawKey: RawKey{Name: ".PowerWatts.Reading"}},
			payload: map[string]any{
				"PowerWatts": map[string]any{"Reading": 1.0},
			},
			assert: func(t *testing.T, got Metric) {
				assert.Equal(t, "", got.Labels["uri"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cfg.FilterRedfish(tt.metric, tt.vendor, tt.route, tt.payload)

			tt.assert(t, got)
		})
	}
}
