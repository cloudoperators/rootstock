// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/cloudoperators/rootstock/internal/manager"
	"github.com/cloudoperators/rootstock/internal/rawdata"
	"github.com/cloudoperators/rootstock/pkg/log"
)

const rackPDUsPath = "/redfish/v1/PowerEquipment/RackPDUs"

// Filter narrows what a scrape emits. Both sets match on the leaf (the last
// "/"-separated segment) of a field.
type Filter struct {
	// Readings keeps only numeric values whose leaf is in the set. An empty
	// set keeps every number.
	Readings map[string]bool
	// Metadata is the set of string-valued leaves captured as labels on a
	// per-document info metric (value 1). An empty set emits no info metric.
	Metadata map[string]bool
}

// DefaultFilter keeps only the primary Reading of each measurement and captures
// the common identity, firmware and descriptor strings as metadata.
func DefaultFilter() Filter {
	return Filter{
		Readings: setOf("Reading"),
		Metadata: setOf(
			"Manufacturer", "Model", "SerialNumber", "PartNumber",
			"FirmwareVersion", "Version", "ProductionDate",
			"VoltageType", "OutletType", "NominalVoltage", "PhaseWiringType",
			"Health", "State", "PowerState", "BreakerState", "CircuitType",
		),
	}
}

func setOf(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}

	return set
}

// NewScraper returns a function that logs in to one PDU, reads all of its
// metrics, and sends them to out. The returned function is what the manager calls.
func NewScraper(
	out chan<- []rawdata.RawMetric,
	logger log.MinInterface,
	timeout time.Duration,
	filter Filter,
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

		metrics, err := scrapeAll(ctx, client, logger, job, filter)
		if err != nil {
			return 0, err
		}

		out <- metrics

		return http.StatusOK, nil
	}
}

// scrapeAll reads the list of PDUs on the device and scrapes each one.
func scrapeAll(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
	filter Filter,
) ([]rawdata.RawMetric, error) {
	var root map[string]any
	if err := client.Get(ctx, rackPDUsPath, &root); err != nil {
		return nil, fmt.Errorf("list rack PDUs: %w", err)
	}

	now := time.Now()

	var metrics []rawdata.RawMetric
	for _, pduURL := range childURLs(root) {
		metrics = append(metrics, scrapePDU(ctx, client, logger, job, filter, pduURL, now)...)
	}

	return metrics, nil
}

// scrapePDU reads every metric for a single PDU: the PDU document itself, its
// Metrics document, and its Mains, Outlets and Branches collections.
// Collections are a list of links to sub documents.
func scrapePDU(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
	filter Filter,
	pduURL string,
	now time.Time,
) []rawdata.RawMetric {
	pduID := path.Base(pduURL)
	pduLabels := map[string]string{"pdu_id": pduID}

	var metrics []rawdata.RawMetric

	metrics = append(metrics, readDocument(
		ctx, client, logger, job, filter, pduURL, "", pduLabels, now,
	)...)
	metrics = append(metrics, readDocument(
		ctx, client, logger, job, filter, pduURL+"/Metrics", "Metrics", pduLabels, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, filter, pduURL+"/Mains", "Mains", "main_id", pduID, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, filter, pduURL+"/Outlets", "Outlets", "outlet_id", pduID, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, filter, pduURL+"/Branches", "Branches", "branch_id", pduID, now,
	)...)

	return metrics
}

// readCollection reads a Redfish collection (a table of contents, like Outlets)
// and scrapes every document it links to, tagging each with the member's ID.
func readCollection(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
	filter Filter,
	urlPath, prefix, idLabel, pduID string,
	now time.Time,
) []rawdata.RawMetric {
	var collection map[string]any
	if err := client.Get(ctx, urlPath, &collection); err != nil {
		logger.Warn("skipping redfish collection", "path", urlPath, log.KeyError, err)
		return nil
	}

	var metrics []rawdata.RawMetric

	for _, memberURL := range childURLs(collection) {
		labels := map[string]string{"pdu_id": pduID, idLabel: path.Base(memberURL)}
		metrics = append(metrics, readDocument(
			ctx, client, logger, job, filter, memberURL, prefix, labels, now,
		)...)
	}

	return metrics
}

// readDocument reads one Redfish document and turns it into RawMetrics: every
// number whose leaf passes filter.Readings becomes a metric named by its
// position in the JSON (e.g. "PowerWatts/Reading"), and any string leaves in
// filter.Metadata are bundled onto a single per-document info metric.
func readDocument(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
	filter Filter,
	urlPath, prefix string,
	labels map[string]string,
	now time.Time,
) []rawdata.RawMetric {
	var doc map[string]any
	if err := client.Get(ctx, urlPath, &doc); err != nil {
		logger.Warn("skipping redfish document", "path", urlPath, log.KeyError, err)
		return nil
	}

	var metrics []rawdata.RawMetric

	collectReadings(prefix, doc, func(name string, value float64) {
		if !keepLeaf(name, filter.Readings) {
			return
		}

		metrics = append(metrics, rawdata.RawMetric{
			DeviceIP:  job.IP,
			Protocol:  job.Protocol,
			Key:       name,
			Value:     value,
			Labels:    labels,
			Timestamp: now,
		})
	})

	if info, ok := metadataMetric(doc, prefix, labels, filter.Metadata, job, now); ok {
		metrics = append(metrics, info)
	}

	return metrics
}

// metadataMetric collects the string leaves named in keep into a single info
// metric (value 1) whose labels are the document's labels plus one per captured
// string. The bool is false when no metadata string was found.
func metadataMetric(
	doc map[string]any,
	prefix string,
	labels map[string]string,
	keep map[string]bool,
	job manager.ScrapeJob,
	now time.Time,
) (rawdata.RawMetric, bool) {
	if len(keep) == 0 {
		return rawdata.RawMetric{}, false
	}

	merged := make(map[string]string, len(labels)+len(keep))
	for k, v := range labels {
		merged[k] = v
	}

	found := false
	collectMetadata(doc, keep, func(name, value string) {
		merged[name] = value
		found = true
	})

	if !found {
		return rawdata.RawMetric{}, false
	}

	key := "info"
	if prefix != "" {
		key = prefix + "/info"
	}

	return rawdata.RawMetric{
		DeviceIP:  job.IP,
		Protocol:  job.Protocol,
		Key:       key,
		Value:     1,
		Labels:    merged,
		Timestamp: now,
	}, true
}

// keepLeaf reports whether name's leaf (the segment after the last "/") is in
// keep. An empty keep set accepts everything.
func keepLeaf(name string, keep map[string]bool) bool {
	if len(keep) == 0 {
		return true
	}

	return keep[name[strings.LastIndex(name, "/")+1:]]
}

// childURLs returns the list of URLs a Redfish collection links to. A collection
// stores them in an array of objects that each have an "@odata.id"; the array's
// key name varies (Members, Outlets, Branch), so we match on that shape.
func childURLs(doc map[string]any) []string {
	var urls []string

	for _, value := range doc {
		items, ok := value.([]any)
		if !ok {
			continue
		}

		for _, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}

			if id, ok := obj["@odata.id"].(string); ok {
				urls = append(urls, id)
			}
		}

		if len(urls) > 0 {
			return urls
		}
	}

	return urls
}

// collectReadings walks a decoded JSON value and calls emit for every number it finds,
// passing the "/"-joined path of keys that lead to that number. jsonValue is the piece of
// JSON currently being examined: a map, a list, a number, or something we ignore.
func collectReadings(prefix string, jsonValue any, emit func(name string, value float64)) {
	switch value := jsonValue.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.Contains(key, "@odata") {
				continue
			}

			name := key
			if prefix != "" {
				name = prefix + "/" + key
			}

			collectReadings(name, child, emit)
		}
	case []any:
		for _, child := range value {
			collectReadings(prefix, child, emit)
		}
	case float64:
		emit(prefix, value)
	}
}

// collectMetadata walks a decoded JSON value and calls emit for every string
// whose key is in keep, passing that key and the string value. Nested objects
// and arrays are searched; keys are matched by their own name, not a path.
func collectMetadata(jsonValue any, keep map[string]bool, emit func(key, value string)) {
	switch value := jsonValue.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.Contains(key, "@odata") {
				continue
			}

			if str, ok := child.(string); ok {
				if keep[key] {
					emit(key, str)
				}

				continue
			}

			collectMetadata(child, keep, emit)
		}
	case []any:
		for _, child := range value {
			collectMetadata(child, keep, emit)
		}
	}
}
