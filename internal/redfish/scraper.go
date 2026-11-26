// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/cloudoperators/rootstock/internal/manager"
	"github.com/cloudoperators/rootstock/internal/model"
	"github.com/cloudoperators/rootstock/pkg/log"
)

type ScrapeResult struct {
	DeviceIP string
	Metadata map[string]model.Metadata
	Routes   map[string][]byte
}

// NewScraper returns a function that the manager calls for each scrape job.
func NewScraper(
	logger log.MinInterface,
	timeout time.Duration,
	metadataPattern string,
	urlPatterns []string,
) func(ctx context.Context, job manager.ScrapeJob) (int, error) {
	return func(ctx context.Context, job manager.ScrapeJob) (int, error) {
		baseURL, err := url.Parse("https://" + job.IP)
		if err != nil {
			return 0, fmt.Errorf("parse device URL %q: %w", job.IP, err)
		}

		client := NewClient(baseURL, logger, job.Username, job.Password, timeout)

		err = client.Login(ctx)
		if err != nil {
			return 0, fmt.Errorf("login to %s: %w", job.IP, err)
		}

		defer func() {
			err := client.Close(ctx)
			if err != nil {
				logger.Warn("failed to close redfish session", "ip", job.IP, log.KeyError, err)
			}
		}()

		metadatas, err := fetchMetadata(ctx, client, logger, metadataPattern)
		if err != nil {
			return 0, fmt.Errorf("fetch metadata for %s: %w", job.IP, err)
		}

		routes := make(map[string][]byte)
		for _, pattern := range urlPatterns {
			urls, err := resolvePattern(ctx, client, logger, pattern)
			if err != nil {
				return 0, fmt.Errorf("resolve pattern %q: %w", pattern, err)
			}

			for _, u := range urls {
				var raw json.RawMessage
				err = client.Get(ctx, u, &raw)
				if err != nil {
					logger.Warn("skipping URL", "url", u, log.KeyError, err)
					continue
				}
				routes[u] = raw
			}
		}

		result := ScrapeResult{
			DeviceIP: job.IP,
			Metadata: metadatas,
			Routes:   routes,
		}

		logger.Info("scrape complete", "ip", job.IP, "metadata_count", len(result.Metadata), "route_count", len(result.Routes))

		// TODO: pass result to NewRedfish for metric construction

		return http.StatusOK, nil
	}
}

// fetches PDU root documents and decodes them into typed structs.
func fetchMetadata(ctx context.Context, client Client, logger log.MinInterface, metadataPattern string) (map[string]model.Metadata, error) {
	urls, err := resolvePattern(ctx, client, logger, metadataPattern)
	if err != nil {
		return nil, fmt.Errorf("resolve metadata pattern %q: %w", metadataPattern, err)
	}

	metadatas := make(map[string]model.Metadata)
	for _, u := range urls {
		var m model.Metadata
		if err := client.Get(ctx, u, &m); err != nil {
			logger.Warn("skipping metadata URL", "url", u, log.KeyError, err)
			continue
		}
		metadatas[u] = m
	}

	return metadatas, nil

}

// resolvePattern expands a single "#" wildcard in a URL pattern into concrete URLs.
func resolvePattern(ctx context.Context, client Client, logger log.MinInterface, pattern string) ([]string, error) {
	idx := strings.Index(pattern, "/#")
	if idx == -1 {
		return []string{pattern}, nil
	}

	prefix := pattern[:idx]
	rest := pattern[idx+2:]

	var collection map[string]any
	err := client.Get(ctx, prefix, &collection)
	if err != nil {
		return nil, fmt.Errorf("list collection at %q: %w", prefix, err)
	}

	var result []string
	for _, memberURL := range extractMemberURLs(collection) {
		memberID := path.Base(memberURL)
		expanded := prefix + "/" + memberID + rest

		sub, err := resolvePattern(ctx, client, logger, expanded)
		if err != nil {
			logger.Warn("skipping member", "url", memberURL, log.KeyError, err)
			continue
		}
		result = append(result, sub...)
	}

	return result, nil
}

// extractMemberURLs returns the list of URLs a Redfish collection links to. A collection
// stores them in an array of objects that each have an "@odata.id"; the array's
// key name varies (Members, Outlets, Branch), so we match on that shape.
func extractMemberURLs(doc map[string]any) []string {
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
