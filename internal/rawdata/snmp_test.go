// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exampleInput is the snmp_exporter text output used as the source of truth for
// these tests.
var exampleInput = filepath.Join("..", "..", "testdata", "snmp-outputs", "output1.txt")

func TestParseSNMPText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		wantErr       string
		wantCount     int
		wantProtocol  Protocol
		assertMetrics func(t *testing.T, metrics []Metric)
	}{
		{
			name:         "parses the example snmp output file",
			input:        mustReadFile(t, exampleInput),
			wantCount:    684,
			wantProtocol: SNMP,
			assertMetrics: func(t *testing.T, metrics []Metric) {
				// Every metric must carry the SNMP protocol and a non-empty name.
				for _, m := range metrics {
					assert.Equal(t, SNMP, m.Protocol)
					assert.NotEmpty(t, m.RawKey.Name, "metric name must not be empty")
				}

				// Labelless metric with a float value in scientific notation.
				uptime := findMetrics(metrics, "system_uptime")
				require.Len(t, uptime, 1)
				assert.Empty(t, uptime[0].Labels)
				assert.InDelta(t, 7.09652612e+08, uptime[0].Value, 0.001)

				// Single-label metric with value 1.
				model := findMetrics(metrics, "model")
				require.Len(t, model, 1)
				assert.Equal(t, "EPA126", model[0].Labels["model"])
				assert.Equal(t, float64(1), model[0].Value)

				// Multi-label info metric; value and labels preserved.
				comms := findLabeled(metrics, "communication_status_info", map[string]any{
					"communication_status": "commsGood",
					"pduNumber":            "99",
				})
				require.NotNil(t, comms)
				assert.Equal(t, float64(1), comms.Value)

				// Non-1 integer value carried through as float64.
				scraped := findMetrics(metrics, "snmp_scrape_pdus_returned")
				require.Len(t, scraped, 1)
				assert.Equal(t, "pdu", scraped[0].Labels["module"])
				assert.Equal(t, float64(727), scraped[0].Value)

				// A family that repeats across many rows keeps all of them.
				breakers := findMetrics(metrics, "breaker_circuit_id")
				assert.Len(t, breakers, 12)
			},
		},
		{
			name: "parses a minimal inline sample",
			input: "# HELP temp A temperature.\n" +
				"# TYPE temp gauge\n" +
				"temp{sensor=\"a\"} 42.5\n",
			wantCount:    1,
			wantProtocol: SNMP,
			assertMetrics: func(t *testing.T, metrics []Metric) {
				require.Len(t, metrics, 1)
				assert.Equal(t, "temp", metrics[0].RawKey.Name)
				assert.Equal(t, "a", metrics[0].Labels["sensor"])
				assert.Equal(t, 42.5, metrics[0].Value)
			},
		},
		{
			name:      "returns no metrics for empty input",
			input:     "",
			wantCount: 0,
		},
		{
			name:      "returns no metrics for comments only",
			input:     "# HELP only_help nothing follows\n# TYPE only_help gauge\n",
			wantCount: 0,
		},
		{
			name:    "returns error for malformed input",
			input:   "this is not valid prometheus text {\n",
			wantErr: "failed to parse SNMP text format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			metrics, err := ParseSNMPBytes([]byte(tt.input), nil)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, metrics, tt.wantCount)

			if tt.assertMetrics != nil {
				tt.assertMetrics(t, metrics)
			}
		})
	}
}

// mustReadFile reads a file for use as test input, failing the test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read test input %s", path)

	return string(data)
}

// findMetrics returns every metric with the given family name.
func findMetrics(metrics []Metric, name string) []Metric {
	var out []Metric
	for _, m := range metrics {
		if m.RawKey.Name == name {
			out = append(out, m)
		}
	}

	return out
}

// findLabeled returns the first metric with the given name whose labels are a
// superset of want, or nil if none match.
func findLabeled(metrics []Metric, name string, want map[string]any) *Metric {
	for i, m := range metrics {
		if m.RawKey.Name != name {
			continue
		}

		match := true
		for k, v := range want {
			if m.Labels[k] != v {
				match = false
				break
			}
		}

		if match {
			return &metrics[i]
		}
	}

	return nil
}
