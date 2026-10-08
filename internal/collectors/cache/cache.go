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

type CacheImpl struct {
	mu     sync.RWMutex
	logger log.MinInterface

	rackPDUMData             map[string]RackPDUMetricData
	environmentalSensorsData map[string]EnvironmentalSensorsData
}

type cache interface {
	GetRackPDU(string) (*RackPDUMetricData, error)
	SetRackPDU(string, *RackPDUMetricData)
	GeEnvironmentalSensorsData(string) (*EnvironmentalSensorsData, error)
	SeEnvironmentalSensorsData(string, *EnvironmentalSensorsData)
}

func NewCache(logger log.MinInterface) cache {
	return &CacheImpl{
		logger:                   logger,
		mu:                       sync.RWMutex{},
		rackPDUMData:             make(map[string]RackPDUMetricData),
		environmentalSensorsData: make(map[string]EnvironmentalSensorsData),
	}
}

func (c *CacheImpl) GetRackPDU(key string) (*RackPDUMetricData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.rackPDUMData[key]
	if !exists {
		return nil, fmt.Errorf("rack PDU %s: %w", key, errCacheNotFound)
	}

	return &metricData, nil
}

func (c *CacheImpl) SetRackPDU(key string, metricData *RackPDUMetricData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rackPDUMData[key] = *metricData
}

func (c *CacheImpl) GeEnvironmentalSensorsData(key string) (*EnvironmentalSensorsData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.environmentalSensorsData[key]
	if !exists {
		return nil, fmt.Errorf("environmental sensors %s: %w", key, errCacheNotFound)
	}

	return &metricData, nil
}

func (c *CacheImpl) SeEnvironmentalSensorsData(key string, metricData *EnvironmentalSensorsData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.environmentalSensorsData[key] = *metricData
}
