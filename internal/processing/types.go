// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package processing

// Reading is one scraped PDU measurement, before processing.
type Reading struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Result is one processed, cache-ready PDU measurement.
type Result struct {
	PDUID           string
	PowerWatts      float64
	ApparentVA      float64
	ReactiveVAR     float64
	PowerFactor     float64
	EnergykWh       float64
	PowerCapacityVA float64
}

// Submission identifies one reading, including repeated readings from the same PDU.
type Submission struct {
	ID      uint64
	Reading Reading
}

// CachedResult retains the submission ID so consumers can distinguish fresh results.
type CachedResult struct {
	SubmissionID uint64
	Result       Result
}

// Runtime reports worker readiness and shutdown. Ready closes once all workers
// have started. Done delivers the terminal error and closes after all workers exit.
type Runtime struct {
	Ready <-chan struct{}
	Done  <-chan error
}
