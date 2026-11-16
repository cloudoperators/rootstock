// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package mockpdu_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudoperators/rootstock/internal/redfish"
	"github.com/cloudoperators/rootstock/test/performance/harness"
	"github.com/cloudoperators/rootstock/test/performance/mockpdu"
)

const (
	pduJSON     = `{"Id":1,"Name":"Fixture PDU","PowerCapacityVA":500}`
	metricsJSON = `{
	"Id":"Metrics",
	"PowerWatts":{"Reading":100,"ApparentVA":120,"ReactiveVAR":20,"PowerFactor":0.9},
	"EnergykWh":{"Reading":42}
}`
)

func createFixtures(t *testing.T, pduContent, metricsContent string) string {
	t.Helper()
	directory := t.TempDir()
	deviceDirectory := filepath.Join(directory, "redfish", "v1", "PowerEquipment", "RackPDUs", "1")
	require.NoError(t, os.MkdirAll(filepath.Join(deviceDirectory, "Metrics"), 0o700))

	if pduContent != "" {
		require.NoError(t, os.WriteFile(filepath.Join(deviceDirectory, "index.json"), []byte(pduContent), 0o600))
	}

	if metricsContent != "" {
		path := filepath.Join(deviceDirectory, "Metrics", "index.json")
		require.NoError(t, os.WriteFile(path, []byte(metricsContent), 0o600))
	}

	return directory
}

func TestRedfishSources(t *testing.T) {
	cases := []struct {
		name        string
		scaled      bool
		count       int
		wantIDs     []string
		wantReading harness.Reading
	}{
		{
			name: "single PDU", wantIDs: []string{"redfish"},
			wantReading: harness.Reading{
				PowerWatts: 100, ApparentVA: 120, ReactiveVAR: 20,
				PowerFactor: 0.9, EnergykWh: 42, PowerCapacityVA: 500,
			},
		},
		{
			name: "scaled PDUs", scaled: true, count: 12,
			wantIDs: []string{
				"redfish-1", "redfish-10", "redfish-11", "redfish-12", "redfish-2", "redfish-3",
				"redfish-4", "redfish-5", "redfish-6", "redfish-7", "redfish-8", "redfish-9",
			},
			wantReading: harness.Reading{
				PowerWatts: 100, ApparentVA: 120, ReactiveVAR: 20,
				PowerFactor: 0.9, EnergykWh: 42, PowerCapacityVA: 500,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := createFixtures(t, pduJSON, metricsJSON)

			var (
				source harness.Source
				err    error
			)
			if tc.scaled {
				source, err = mockpdu.NewRedfishScaled(directory, tc.count)
			} else {
				source, err = mockpdu.NewRedfish(directory)
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantIDs, source.IDs())
			client, ok := source.(redfish.Client)
			require.True(t, ok)
			// Reads must remain in-memory even if the input files disappear.
			require.NoError(t, os.RemoveAll(directory))

			for _, id := range tc.wantIDs {
				pdu, err := client.GetPERackPDU(t.Context(), id)
				require.NoError(t, err)
				metrics, err := client.GetPERackPDUMetrics(t.Context(), id)
				require.NoError(t, err)

				pdu.PowerCapacityVA = -1
				metrics.PowerWatts.Reading = -1
				reading, err := source.Read(t.Context(), id)
				require.NoError(t, err)

				want := tc.wantReading
				want.PDUID = id
				assert.Equal(t, want, reading, "modifying returned documents must not change the fixture")
			}

			collection, err := client.GetRackPDUs(t.Context())
			require.NoError(t, err)
			require.Len(t, collection.Members, len(tc.wantIDs))
			assert.Equal(t, len(tc.wantIDs), collection.MembersCount)

			for i, id := range tc.wantIDs {
				assert.Equal(t, "/redfish/v1/PowerEquipment/RackPDUs/"+id, collection.Members[i].ODataID)
			}

			_, err = source.Read(t.Context(), "unknown")
			require.ErrorContains(t, err, `unknown PDU "unknown"`)
			_, err = client.GetPERackPDU(t.Context(), "unknown")
			require.ErrorContains(t, err, `unknown PDU "unknown"`)
			_, err = client.GetPERackPDUMetrics(t.Context(), "unknown")
			require.ErrorContains(t, err, `unknown PDU "unknown"`)
		})
	}
}

func TestRedfishFixtureErrors(t *testing.T) {
	cases := []struct {
		name            string
		pduContent      string
		metricsContent  string
		count           int
		wantMessage     string
		wantMissingFile bool
	}{
		{name: "missing PDU", count: 1, wantMessage: "read fixture", wantMissingFile: true},
		{name: "missing metrics", pduContent: pduJSON, count: 1, wantMessage: "Metrics", wantMissingFile: true},
		{name: "invalid PDU JSON", pduContent: "{", count: 1, wantMessage: "decode fixture"},
		{
			name: "invalid metrics JSON", pduContent: pduJSON, metricsContent: "{", count: 1,
			wantMessage: "decode fixture",
		},
		{name: "zero devices", count: 0, wantMessage: "n must be >= 1"},
		{name: "negative devices", count: -1, wantMessage: "n must be >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := createFixtures(t, tc.pduContent, tc.metricsContent)
			source, err := mockpdu.NewRedfishScaled(directory, tc.count)
			require.ErrorContains(t, err, tc.wantMessage)
			assert.Nil(t, source)

			if tc.wantMissingFile {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}
