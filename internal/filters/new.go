// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	lock         sync.RWMutex
	filterConfig filterFile
)

const (
	// SNMP identifies SNMP-based filters.
	SNMP protocol = "snmp"
	// Redfish identifies Redfish-based filters.
	Redfish protocol = "redfish"

	// Label names injected into every rule by Update to identify the rule's
	// origin within the filter definition.
	LabelProtocol     = "protocol"
	LabelManufacturer = "manufacturer"
	LabelRoute        = "route"
)

type protocol string

func (p protocol) valid() bool {
	switch p {
	case SNMP, Redfish:
		return true
	default:
		return false
	}
}

// Rule describes a single metric extracted from a Redfish response. It maps a
// Prometheus metric name (Key) to a value selector (Value) together with the
// metric type, help text and the set of labels to attach.
type Rule struct {
	Key    string            `yaml:"key"`
	Value  string            `yaml:"value"`
	Type   string            `yaml:"type"`
	Help   string            `yaml:"help"`
	Labels map[string]string `yaml:"labels"`
}

// Update parses the given filter definition content and updates the
// package-local filter representation. It injects the origin labels (protocol,
// manufacturer, route) into every rule and validates that the combination of
// (key, labels) is unique within each route before publishing. It must be
// called at least once before any of the Get* accessors are used, and may be
// called again at runtime to reload the filters.
func Update(filterContent []byte) error {
	var parsed filterFile

	if err := yaml.Unmarshal(filterContent, &parsed); err != nil {
		return fmt.Errorf("parsing filter content error: %w", err)
	}

	injectOriginLabels(parsed)

	if err := validateFilters(parsed); err != nil {
		return err
	}

	lock.Lock()
	filterConfig = parsed
	lock.Unlock()

	return nil
}

// GetRules returns the rules for the given protocol and manufacturer, keyed by
// route URI.
func GetRules(protocol protocol, manufacturer string) (map[string][]Rule, error) {
	m, err := lookup(protocol, manufacturer)
	if err != nil {
		return nil, err
	}
	res := make(map[string][]Rule, len(m.Rules))
	for k, v := range m.Rules {
		res[k] = slices.Clone(v)
	}

	return res, nil
}

// GetRule returns the first rule whose key matches rule for the given protocol
// and manufacturer, searching across all routes.
func GetRule(protocol protocol, manufacturer, rule string) (Rule, error) {
	m, err := lookup(protocol, manufacturer)
	if err != nil {
		return Rule{}, err
	}

	for _, rules := range m.Rules {
		for _, r := range rules {
			if r.Key == rule {
				return r, nil
			}
		}
	}

	return Rule{}, fmt.Errorf("unknown rule %q for protocol %q manufacturer %q", rule, protocol, manufacturer)
}

// filterFile is the on-disk representation of the metrics filter config file.
type filterFile struct {
	Filters map[protocol]map[string]manufacturerFilters `yaml:"filters"`
}

type manufacturerFilters struct {
	Metadata string            `yaml:"metadata"`
	Rules    map[string][]Rule `yaml:"rules"`
}

// injectOriginLabels adds the protocol, manufacturer and route labels to every
// rule so that metrics produced from different origins remain distinguishable.
// Each rule receives its own fresh label map so that labels shared via YAML
// anchors/aliases are never mutated across rules.
func injectOriginLabels(file filterFile) {
	for protocol, manufacturers := range file.Filters {
		for manufacturer, mf := range manufacturers {
			for route, rules := range mf.Rules {
				for i := range rules {
					labels := make(map[string]string, len(rules[i].Labels)+3)
					maps.Copy(labels, rules[i].Labels)

					labels[LabelProtocol] = string(protocol)
					labels[LabelManufacturer] = manufacturer
					labels[LabelRoute] = route

					rules[i].Labels = labels
				}
			}
		}
	}
}

// validateFilters ensures that the combination of (key, labels) is unique
// within each route. Two rules in the same route collide when they share the
// same key and the exact same set of label name/value pairs.
func validateFilters(file filterFile) error {
	empty := struct{}{}
	for protocol, manufacturers := range file.Filters {
		for manufacturer, mf := range manufacturers {
			for route, rules := range mf.Rules {
				seen := make(map[string]struct{})

				for _, rule := range rules {
					sig := ruleSignature(rule)
					if _, ok := seen[sig]; ok {
						return fmt.Errorf(
							"duplicate rule (key, labels) combination %q within protocol %q manufacturer %q route %q",
							sig, protocol, manufacturer, route,
						)
					}

					seen[sig] = empty
				}
			}
		}
	}

	return nil
}

// ruleSignature builds a deterministic, order-independent signature for a
// rule's (key, labels) combination so it can be compared for uniqueness.
func ruleSignature(rule Rule) string {
	labels := make([]string, 0, len(rule.Labels))
	for name, value := range rule.Labels {
		labels = append(labels, name+"="+value)
	}

	sort.Strings(labels)

	return rule.Key + "\x00" + strings.Join(labels, "\x00")
}

// GetRoutes returns the route URIs defined for the given protocol and
// manufacturer.
func GetRoutes(protocol protocol, manufacturer string) ([]string, error) {
	m, err := lookup(protocol, manufacturer)
	if err != nil {
		return nil, err
	}

	routes := make([]string, 0, len(m.Rules))
	for route := range m.Rules {
		routes = append(routes, route)
	}

	return routes, nil
}

// lookup resolves the manufacturer definition for the given protocol and
// manufacturer, returning a descriptive error when either is unknown.
func lookup(protocol protocol, manufacturer string) (manufacturerFilters, error) {
	if !protocol.valid() {
		return manufacturerFilters{}, fmt.Errorf("unknown protocol %q", protocol)
	}

	lock.RLock()
	defer lock.RUnlock()

	manufacturers, ok := filterConfig.Filters[protocol]
	if !ok {
		return manufacturerFilters{}, fmt.Errorf("unknown protocol %q", protocol)
	}

	m, ok := manufacturers[manufacturer]
	if !ok {
		return manufacturerFilters{}, fmt.Errorf("unknown manufacturer %q for protocol %q", manufacturer, protocol)
	}

	return m, nil
}
