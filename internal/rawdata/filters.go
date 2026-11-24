// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package rawdata

import (
	"fmt"
	"maps"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// FilterConfig is the top-level representation of filter.yaml. It maps the raw
// output of a scraper to final metric names and curates the label set via
// per-protocol, per-key rules.
type FilterConfig struct {
	Filters Filters `yaml:"filters"`
}

// Filters groups the key rules by protocol.
type Filters struct {
	// Redfish is keyed first by vendor, then by route, then by raw key.
	Redfish map[string]map[string]KeyFilter `yaml:"redfish"`
	// SNMP is a flat set of key rules; SNMP has no vendor/route dimension.
	SNMP KeyFilter `yaml:"snmp"`
}

// KeyFilter holds a set of key translation rules. It is used both for a single
// redfish route and for the flat SNMP rule set.
type KeyFilter struct {
	Keys KeyRules `yaml:"keys"`
}

// KeyRules maps a raw key to its translation rule.
type KeyRules map[string]KeyRule

// KeyRule describes how to translate a single raw key into a final metric.
type KeyRule struct {
	// NewKey is the final metric name. Empty means "keep the raw key".
	NewKey string `yaml:"newKey"`
	// Labels describes label mutations to apply.
	Labels LabelRules `yaml:"labels"`
}

// LabelRules describes label additions and removals for a matched metric.
type LabelRules struct {
	// Add maps a label name to a value specification. For SNMP the value is a
	// constant unless it names an existing label, in which case that label's
	// value is copied. For redfish a leading-dot value (e.g. ".Power.Uri") is a
	// JSON reference resolved against the raw payload; anything else is constant.
	Add map[string]string `yaml:"add"`
	// Remove lists label names to drop. For redfish the entries are JSON
	// references (e.g. ".Id"); the trailing path element is used as the label
	// name. For SNMP they are plain label names.
	Remove []string `yaml:"remove"`
}

// LoadFilterConfig reads and parses a filter configuration file.
func LoadFilterConfig(path string) (*FilterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read filter config %q: %w", path, err)
	}

	cfg, err := ParseFilterConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter config %q: %w", path, err)
	}

	return cfg, nil
}

// ParseFilterConfig parses a filter configuration from raw YAML bytes.
func ParseFilterConfig(data []byte) (*FilterConfig, error) {
	var cfg FilterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter config: %w", err)
	}

	return &cfg, nil
}

// FilterSNMP filters and translates a single SNMP metric. If the metric's raw
// key has a matching rule, the metric is translated (FinalKey set, labels
// added/removed); otherwise it is returned unchanged.
func (c *FilterConfig) FilterSNMP(m Metric) Metric {
	rule, ok := c.Filters.SNMP.Keys[m.RawKey.Name]
	if !ok {
		return m
	}

	m.FinalKey = finalKey(rule.NewKey, m.RawKey.Name)
	m.Labels = copyLabels(m.Labels)

	for name, spec := range rule.Labels.Add {
		m.Labels[name] = resolveSNMPValue(spec, m.Labels)
	}

	for _, name := range rule.Labels.Remove {
		delete(m.Labels, name)
	}

	return m
}

// resolveSNMPValue resolves an "add" value for SNMP: if the spec names an
// existing label, that label's value is copied; otherwise the spec is a literal
// constant.
func resolveSNMPValue(spec string, labels map[string]any) any {
	if v, ok := labels[spec]; ok {
		return v
	}

	return spec
}

// FilterRedfish filters and translates a single redfish metric for the given
// vendor and route, resolving JSON references against the decoded raw payload.
// When no rule matches the metric's raw key, the metric is returned unchanged.
//
// payload is the JSON-decoded raw document the metric was derived from, used to
// resolve leading-dot references in labels.add and labels.remove.
func (c *FilterConfig) FilterRedfish(m Metric, vendor, route string, payload map[string]any) Metric {
	routes, ok := c.Filters.Redfish[vendor]
	if !ok {
		return m
	}

	routeFilter, ok := routes[route]
	if !ok {
		return m
	}

	rule, ok := routeFilter.Keys[m.RawKey.Name]
	if !ok {
		return m
	}

	m.FinalKey = finalKey(rule.NewKey, m.RawKey.Name)
	m.Labels = copyLabels(m.Labels)

	for name, spec := range rule.Labels.Add {
		m.Labels[name] = resolveRedfishValue(spec, payload)
	}

	for _, ref := range rule.Labels.Remove {
		delete(m.Labels, labelNameFromRef(ref))
	}

	return m
}

// resolveRedfishValue resolves an "add" value for redfish: a leading-dot spec
// is a JSON reference into payload; anything else is a literal constant.
func resolveRedfishValue(spec string, payload map[string]any) any {
	if !strings.HasPrefix(spec, ".") {
		return spec
	}

	if v, ok := lookupJSONPath(payload, spec); ok {
		return v
	}

	return ""
}

// lookupJSONPath walks a dotted JSON reference such as ".PowerWatts.Reading"
// through nested maps and returns the value if present.
func lookupJSONPath(payload map[string]any, ref string) (any, bool) {
	parts := splitRef(ref)
	if len(parts) == 0 {
		return nil, false
	}

	var cur any = payload
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}

		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}

	return cur, true
}

// labelNameFromRef derives the label name to remove from a reference. For a
// JSON reference like ".Id" the final path element ("Id") is used; a plain name
// is returned unchanged.
func labelNameFromRef(ref string) string {
	parts := splitRef(ref)
	if len(parts) == 0 {
		return ref
	}

	return parts[len(parts)-1]
}

// splitRef splits a dotted reference into its non-empty path elements,
// tolerating a leading dot.
func splitRef(ref string) []string {
	ref = strings.TrimPrefix(ref, ".")
	if ref == "" {
		return nil
	}

	return strings.Split(ref, ".")
}

// finalKey returns newKey when set, otherwise the raw key is kept.
func finalKey(newKey, rawKey string) string {
	if newKey != "" {
		return newKey
	}

	return rawKey
}

func copyLabels(labels map[string]any) map[string]any {
	out := make(map[string]any, len(labels))
	maps.Copy(out, labels)

	return out
}
