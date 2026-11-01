// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/cloudoperators/rootstock/internal/manager"
	"github.com/cloudoperators/rootstock/internal/rawdata"
	"github.com/cloudoperators/rootstock/pkg/log"
)

// NewScraper returns a scraper function that fetches Redfish metrics and sends them to out.
// The returned function satisfies the manager scraper signature.
func NewScraper(
	out chan<- []rawdata.RawMetric,
	logger log.MinInterface,
	timeout time.Duration,
) func(ctx context.Context, job manager.ScrapeJob) (int, error) {
	return func(ctx context.Context, job manager.ScrapeJob) (int, error) {
		baseURL, err := url.Parse("https://" + job.IP)
		if err != nil {
			return 0, fmt.Errorf("parse device URL %q: %w", job.IP, err)
		}

		client := NewClient(baseURL, logger, job.Username, job.Password, timeout)

		if err := client.Login(ctx); err != nil {
			return 0, fmt.Errorf("login to %s: %w", job.IP, err)
		}

		defer func() {
			if err := client.Close(ctx); err != nil {
				logger.Warn("failed to close redfish session", "ip", job.IP, log.KeyError, err)
			}
		}()

		metrics, err := scrapeAll(ctx, client, job.IP)
		if err != nil {
			return 0, err
		}

		out <- metrics

		return http.StatusOK, nil
	}
}

// scrapeAll fetches metrics from all PDUs on the device.
func scrapeAll(ctx context.Context, client Client, deviceIP string) ([]rawdata.RawMetric, error) {
	rackPDUs, err := client.GetRackPDUs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get rack PDUs: %w", err)
	}

	now := time.Now()

	var result []rawdata.RawMetric

	for _, member := range rackPDUs.Members {
		pduID := path.Base(member.ODataID)

		pduMetrics, err := scrapeDevice(ctx, client, deviceIP, pduID, now)
		if err != nil {
			return nil, fmt.Errorf("scrape PDU %s: %w", pduID, err)
		}

		result = append(result, pduMetrics...)
	}

	return result, nil
}

// scrapeDevice fetches all metrics for a single PDU by calling one helper per endpoint.
func scrapeDevice(
	ctx context.Context,
	client Client,
	deviceIP string,
	pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	var result []rawdata.RawMetric

	powerRating, err := scrapePowerRating(ctx, client, deviceIP, pduID, now)
	if err != nil {
		return nil, err
	}

	result = append(result, powerRating...)

	energetics, err := scrapeEnergetics(ctx, client, deviceIP, pduID, now)
	if err != nil {
		return nil, err
	}

	result = append(result, energetics...)

	mainCurrent, err := scrapeMainCurrent(ctx, client, deviceIP, pduID, now)
	if err != nil {
		return nil, err
	}

	result = append(result, mainCurrent...)

	outlets, err := scrapeOutlets(ctx, client, deviceIP, pduID, now)
	if err != nil {
		return nil, err
	}

	result = append(result, outlets...)

	branches, err := scrapeBranches(ctx, client, deviceIP, pduID, now)
	if err != nil {
		return nil, err
	}

	result = append(result, branches...)

	return result, nil
}

// scrapeOutlets fetches voltage, current and power for every outlet on the PDU.
func scrapeOutlets(
	ctx context.Context,
	client Client,
	deviceIP, pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	outletIDs, err := client.GetRackPDUOutletsIDs(ctx, pduID)
	if err != nil {
		return nil, fmt.Errorf("get outlet IDs: %w", err)
	}

	var result []rawdata.RawMetric

	for _, outletID := range outletIDs {
		outlet, err := client.GetRackPDUOutletElectricals(ctx, pduID, outletID)
		if err != nil {
			return nil, fmt.Errorf("get outlet %s: %w", outletID, err)
		}

		labels := map[string]string{"pdu_id": pduID, "outlet_id": outletID}

		result = append(result,
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "RatedCurrentAmps",
				Value: outlet.RatedCurrent, Labels: labels, Timestamp: now,
			},
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "Voltage.Voltage",
				Value: outlet.Voltage.Voltage, Labels: labels, Timestamp: now,
			},
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "CurrentAmps.Current",
				Value: outlet.Current.Current, Labels: labels, Timestamp: now,
			},
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerWatts.Power",
				Value: outlet.ApparentPower.Power, Labels: labels, Timestamp: now,
			},
		)
	}

	return result, nil
}

// scrapeBranches fetches rated current, current and poly-phase metrics for every branch on the PDU.
func scrapeBranches(
	ctx context.Context,
	client Client,
	deviceIP, pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	branchIDs, err := client.GetRackPDUBranchIDs(ctx, pduID)
	if err != nil {
		return nil, fmt.Errorf("get branch IDs: %w", err)
	}

	var result []rawdata.RawMetric

	for _, branchID := range branchIDs {
		branch, err := client.GetRackPDUBranchCurrent(ctx, pduID, branchID)
		if err != nil {
			return nil, fmt.Errorf("get branch %s: %w", branchID, err)
		}

		labels := map[string]string{"pdu_id": pduID, "branch_id": branchID}

		result = append(result,
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "RatedCurrentAmps",
				Value: branch.RatedCurrent, Labels: labels, Timestamp: now,
			},
			rawdata.RawMetric{
				DeviceIP: deviceIP, Protocol: "redfish", Name: "CurrentAmps.Current",
				Value: branch.Current.Current, Labels: labels, Timestamp: now,
			},
		)

		for phase, reading := range branch.PolyPhaseVoltage {
			phaseLabels := map[string]string{"pdu_id": pduID, "branch_id": branchID, "phase": phase}

			result = append(result, rawdata.RawMetric{
				DeviceIP:  deviceIP,
				Protocol:  "redfish",
				Name:      "PolyPhaseVoltage.Reading",
				Value:     reading.Reading,
				Labels:    phaseLabels,
				Timestamp: now,
			})
		}

		for phase, pw := range branch.PolyPhasePowerWatts {
			phaseLabels := map[string]string{"pdu_id": pduID, "branch_id": branchID, "phase": phase}

			result = append(result,
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.Power",
					Value: pw.Power, Labels: phaseLabels, Timestamp: now,
				},
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.ApparentPower",
					Value: pw.ApparentPower, Labels: phaseLabels, Timestamp: now,
				},
			)
		}
	}

	return result, nil
}

func scrapePowerRating(
	ctx context.Context,
	client Client,
	deviceIP, pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	powerRating, err := client.GetRackPDUPowerRating(ctx, pduID)
	if err != nil {
		return nil, fmt.Errorf("get power rating: %w", err)
	}

	pduLabels := map[string]string{"pdu_id": pduID}

	return []rawdata.RawMetric{
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerCapacityVA",
			Value: powerRating.PowerCapacityVA, Labels: pduLabels, Timestamp: now,
		},
	}, nil
}

// scrapeEnergetics fetches live power and cumulative energy consumption for the PDU.
func scrapeEnergetics(
	ctx context.Context,
	client Client,
	deviceIP, pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	energetics, err := client.GetRackPDUEnergetics(ctx, pduID)
	if err != nil {
		return nil, fmt.Errorf("get energetics: %w", err)
	}

	pduLabels := map[string]string{"pdu_id": pduID}

	return []rawdata.RawMetric{
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerWatts.Power",
			Value: energetics.PowerWatts.Power, Labels: pduLabels, Timestamp: now,
		},
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerWatts.ApparentPower",
			Value: energetics.PowerWatts.ApparentPower, Labels: pduLabels, Timestamp: now,
		},
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerWatts.ReactivePower",
			Value: energetics.PowerWatts.ReactivePower, Labels: pduLabels, Timestamp: now,
		},
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "PowerWatts.PowerFactor",
			Value: energetics.PowerWatts.PowerFactor, Labels: pduLabels, Timestamp: now,
		},
		{
			DeviceIP: deviceIP, Protocol: "redfish", Name: "EnergykWh.EnergykWh",
			Value: energetics.EnergykWh.EnergykWh, Labels: pduLabels, Timestamp: now,
		},
	}, nil
}

// scrapeMainCurrent fetches poly-phase voltage, current and power for the PDU main feed.
func scrapeMainCurrent(
	ctx context.Context,
	client Client,
	deviceIP, pduID string,
	now time.Time,
) ([]rawdata.RawMetric, error) {
	mainCurrent, err := client.GetRackPDUMainCurrent(ctx, pduID)
	if err != nil {
		return nil, fmt.Errorf("get main current: %w", err)
	}

	var result []rawdata.RawMetric

	for _, phaseMap := range mainCurrent.PolyPhaseVoltage {
		for phase, reading := range phaseMap {
			result = append(result, rawdata.RawMetric{
				DeviceIP:  deviceIP,
				Protocol:  "redfish",
				Name:      "PolyPhaseVoltage.Reading",
				Value:     reading.Reading,
				Labels:    map[string]string{"pdu_id": pduID, "phase": phase},
				Timestamp: now,
			})
		}
	}

	for _, phaseMap := range mainCurrent.PolyPhaseCurrentAmps {
		for phase, reading := range phaseMap {
			result = append(result, rawdata.RawMetric{
				DeviceIP:  deviceIP,
				Protocol:  "redfish",
				Name:      "PolyPhaseCurrentAmps.Reading",
				Value:     reading.Reading,
				Labels:    map[string]string{"pdu_id": pduID, "phase": phase},
				Timestamp: now,
			})
		}
	}

	for _, phaseMap := range mainCurrent.PolyPhasePowerWatts {
		for phase, pw := range phaseMap {
			phaseLabels := map[string]string{"pdu_id": pduID, "phase": phase}

			result = append(result,
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.Power",
					Value: pw.Power, Labels: phaseLabels, Timestamp: now,
				},
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.ApparentPower",
					Value: pw.ApparentPower, Labels: phaseLabels, Timestamp: now,
				},
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.ReactivePower",
					Value: pw.ReactivePower, Labels: phaseLabels, Timestamp: now,
				},
				rawdata.RawMetric{
					DeviceIP: deviceIP, Protocol: "redfish", Name: "PolyPhasePowerWatts.PowerFactor",
					Value: pw.PowerFactor, Labels: phaseLabels, Timestamp: now,
				},
			)
		}
	}

	return result, nil
}
