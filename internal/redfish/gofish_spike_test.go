// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package redfish_test

import (
	"testing"

	gofish "github.com/stmcginnis/gofish"
)

// Spike: verify GoFish can talk to the Panduit PDU mockup and read metrics.
// Run with: make test-integration
	c, err := gofish.Connect(gofish.ClientConfig{
		Endpoint: mockupBase,
		Username: "admin",
		Password: "password",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("gofish.Connect: %v", err)
	}
	defer c.Logout()
	t.Log("connected")

	pe, err := c.Service.PowerEquipment()
	if err != nil {
		t.Fatalf("PowerEquipment: %v", err)
	}
	if pe == nil {
		t.Fatal("PowerEquipment returned nil")
	}
	t.Logf("PowerEquipment: %s", pe.ODataID)

	pdus, err := pe.RackPDUs()
	if err != nil {
		t.Fatalf("RackPDUs: %v", err)
	}
	if len(pdus) == 0 {
		t.Fatal("no RackPDUs found")
	}
	t.Logf("RackPDUs count: %d", len(pdus))

	pdu := pdus[0]
	t.Logf("PDU Name:         %s", pdu.Name)
	t.Logf("PDU Manufacturer: %s", pdu.Manufacturer)
	t.Logf("PDU Model:        %s", pdu.Model)

	metrics, err := pdu.Metrics()
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics == nil {
		t.Fatal("Metrics returned nil")
	}

	pw := metrics.PowerWatts
	if pw.Reading != nil {
		t.Logf("PowerWatts.Reading:  %.2f W", *pw.Reading)
	}
	if pw.ApparentVA != nil {
		t.Logf("PowerWatts.ApparentVA:  %.2f VA", *pw.ApparentVA)
	}
	if pw.PowerFactor != nil {
		t.Logf("PowerWatts.PowerFactor: %.4f", *pw.PowerFactor)
	}

	ek := metrics.EnergykWh
	if ek.Reading != nil {
		t.Logf("EnergykWh.Reading:   %.4f kWh", *ek.Reading)
	}
}
