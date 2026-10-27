package models

import (
	"fmt"
	"strings"
)

type Raw struct {
	Metadata Metadata
	Data     any
}

type Metadata struct {
	DeviceType string
	IP         string
	Protocol   string
	Version    string
}

type RackPDU struct {
	PowerWatts PowerWatts `json:"PowerWatts"`
}

type PowerWatts struct {
	Reading float64 `json:"Reading"`
}

// this does not work yet and probably needs to be heavily refined.
// normally should use recursive resolution
// e.g. fullKey= .top.level.key
func extractValues(raw any, fullKey string) (any, error) {
	nestedKeys := strings.Split(fullKey, ".")
	for _, k := range nestedKeys {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot cast object to map[string]any for key %q", k)
		}
		subValue, ok := obj[k]
		if !ok {
			return nil, fmt.Errorf("subkey %q not found", k)
		}

	}
}
