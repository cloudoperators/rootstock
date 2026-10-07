// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"

	"github.com/cloudoperators/rootstock/pkg/log"
)

type RackPDUMetricData struct {
	PERackPDUMetrics *PERackPDUMetrics
	PERackPDU        *PERackPDU
}

type EnvironmentalSensorsData struct {
	// TODO
}

type PERackPDUMetrics struct {
	DataSourceURI string
	Reading       float64
	ApparentVA    float64
	ReactiveVAR   float64
	PowerFactor   float64
}

type PERackPDU struct {
	PowerCapacityVA float64
}

type Cache struct {
	mu     sync.RWMutex
	logger log.MinInterface

	rackPDUMData             map[string]RackPDUMetricData
	environmentalSensorsData map[string]EnvironmentalSensorsData
}

func NewCache() *Cache {
	logger := log.Default()

	return &Cache{
		logger:                   logger,
		rackPDUMData:             make(map[string]RackPDUMetricData),
		environmentalSensorsData: make(map[string]EnvironmentalSensorsData),
	}
}

func (c *Cache) GetRackPDU(key string) (*RackPDUMetricData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.rackPDUMData[key]
	if !exists {
		return nil, nil
	}

	return &metricData, nil
}

func (c *Cache) SetRackPDU(key string, metricData *RackPDUMetricData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rackPDUMData[key] = *metricData
}

func (c *Cache) GeEnvironmentalSensorsData(key string) (*EnvironmentalSensorsData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.environmentalSensorsData[key]
	if !exists {
		return nil, nil
	}

	return &metricData, nil
}

func (c *Cache) SeEnvironmentalSensorsData(key string, metricData *EnvironmentalSensorsData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.environmentalSensorsData[key] = *metricData
}
