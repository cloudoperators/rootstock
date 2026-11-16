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

// NewScraper returns a function that logs in to one PDU, reads all of its
// metrics, and sends them to out. The returned function is what the manager calls.
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

		metrics, err := scrapeAll(ctx, client, logger, job)
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
) ([]rawdata.RawMetric, error) {
	var root map[string]any
	if err := client.Get(ctx, rackPDUsPath, &root); err != nil {
		return nil, fmt.Errorf("list rack PDUs: %w", err)
	}

	now := time.Now()

	var metrics []rawdata.RawMetric
	for _, pduURL := range childURLs(root) {
		metrics = append(metrics, scrapePDU(ctx, client, logger, job, pduURL, now)...)
	}

	return metrics, nil
}

// scrapePDU reads every metric for a single PDU: the PDU document itself, its
// Metrics document, and its Mains, Outlets and Branches collections.
func scrapePDU(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
	pduURL string,
	now time.Time,
) []rawdata.RawMetric {
	pduID := path.Base(pduURL)
	pduLabels := map[string]string{"pdu_id": pduID}

	var metrics []rawdata.RawMetric

	metrics = append(metrics, readDocument(
		ctx, client, logger, job, pduURL, "", pduLabels, now,
	)...)
	metrics = append(metrics, readDocument(
		ctx, client, logger, job, pduURL+"/Metrics", "Metrics", pduLabels, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, pduURL+"/Mains", "Mains", "main_id", pduID, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, pduURL+"/Outlets", "Outlets", "outlet_id", pduID, now,
	)...)
	metrics = append(metrics, readCollection(
		ctx, client, logger, job, pduURL+"/Branches", "Branches", "branch_id", pduID, now,
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
			ctx, client, logger, job, memberURL, prefix, labels, now,
		)...)
	}

	return metrics
}

// readDocument reads one Redfish document and turns every number inside it into
// a RawMetric, named by its position in the JSON (e.g. "PowerWatts/Reading").
func readDocument(
	ctx context.Context,
	client Client,
	logger log.MinInterface,
	job manager.ScrapeJob,
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

	findNumbers(prefix, doc, func(name string, value float64) {
		metrics = append(metrics, rawdata.RawMetric{
			DeviceIP:  job.IP,
			Protocol:  job.Protocol,
			Name:      name,
			Value:     value,
			Labels:    labels,
			Timestamp: now,
		})
	})

	return metrics
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

// findNumbers walks a decoded JSON value and calls emit for every number it
// finds, passing the "/"-joined path of keys that lead to that number.
func findNumbers(prefix string, node any, emit func(name string, value float64)) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.Contains(key, "@odata") {
				continue
			}

			name := key
			if prefix != "" {
				name = prefix + "/" + key
			}

			findNumbers(name, child, emit)
		}
	case []any:
		for _, child := range value {
			findNumbers(prefix, child, emit)
		}
	case float64:
		emit(prefix, value)
	}
}
