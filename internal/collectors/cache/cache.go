// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"errors"
	"fmt"
	"sync"

	"github.com/cloudoperators/rootstock/pkg/log"
)

var errCacheNotFound = errors.New("cache not found")

type RackPDUMetricData struct {
	PERackPDUMetrics *PERackPDUMetrics
	PERackPDU        *PERackPDU
}

type EnvironmentalSensorsData struct {
	// Environmental sensors data structure will be defined when implementing environmental sensor metrics collection
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

type cacheImpl struct {
	mu     sync.RWMutex
	logger log.MinInterface

	rackPDUMData             map[string]RackPDUMetricData
	environmentalSensorsData map[string]EnvironmentalSensorsData
}

type Cache interface {
	// GetRackPDU returns rack PDU data from the cache
	GetRackPDU(string) (*RackPDUMetricData, error)
	// SetRackPDU saves rack PDU data in the cache
	SetRackPDU(string, *RackPDUMetricData)
	// GeEnvironmentalSensorsData gets environmental sensors data in the cache
	GetEnvironmentalSensorsData(string) (*EnvironmentalSensorsData, error)
	// SetEnvironmentalSensorsData sets environmtal sensors data in the cache
	SetEnvironmentalSensorsData(string, *EnvironmentalSensorsData)
}

func NewCache(logger log.MinInterface) Cache {
	return &cacheImpl{
		logger:                   logger,
		mu:                       sync.RWMutex{},
		rackPDUMData:             make(map[string]RackPDUMetricData),
		environmentalSensorsData: make(map[string]EnvironmentalSensorsData),
	}
}

func (c *cacheImpl) GetRackPDU(key string) (*RackPDUMetricData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.rackPDUMData[key]
	if !exists {
		return nil, fmt.Errorf("rack PDU %s: %w", key, errCacheNotFound)
	}

	return &metricData, nil
}

func (c *cacheImpl) SetRackPDU(key string, metricData *RackPDUMetricData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rackPDUMData[key] = *metricData
}

func (c *cacheImpl) GetEnvironmentalSensorsData(key string) (*EnvironmentalSensorsData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.environmentalSensorsData[key]
	if !exists {
		return nil, fmt.Errorf("environmental sensors %s: %w", key, errCacheNotFound)
	}

	return &metricData, nil
}

func (c *cacheImpl) SetEnvironmentalSensorsData(key string, metricData *EnvironmentalSensorsData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.environmentalSensorsData[key] = *metricData
}
