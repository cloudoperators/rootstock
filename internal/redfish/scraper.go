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
	"github.com/cloudoperators/rootstock/pkg/log"
)

// NewScraper returns a function that the manager calls for each scrape job.
// TODO: add urlPatterns parameter once model and filter.yaml design is finalised.
func NewScraper(
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

		// TODO: fetch URL patterns and build ScrapeResult.

		return http.StatusOK, nil
	}
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

// resolvePattern expands a single "#" wildcard in a URL pattern into concrete URLs.
func resolvePattern(ctx context.Context, client Client, logger log.MinInterface, pattern string) ([]string, error) {
	idx := strings.Index(pattern, "/#")
	if idx == -1 {
		return []string{pattern}, nil
	}

	prefix := pattern[:idx]
	rest := pattern[idx+2:]

	var collection map[string]any
	if err := client.Get(ctx, prefix, &collection); err != nil {
		return nil, fmt.Errorf("list collection at %q: %w", prefix, err)
	}

	var result []string
	for _, memberURL := range childURLs(collection) {
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
