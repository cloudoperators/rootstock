// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package pdu

// Use case: use the client to fetch data, then turn it into Prometheus metrics.

//Possible functionalities:

// It uses client.go to get JSON from the PDU,
// then reads the values out of that JSON (watts, amps, volts, energy in kWh) and records them as Prometheus metrics.
