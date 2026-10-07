package cache

import (
	"sync"

	"github.com/cloudoperators/rootstock/pkg/log"
)

type MetricData struct {
	PERackPDUMetrics *PERackPDUMetrics
	PERackPDU        *PERackPDU
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

	metricData map[string]MetricData
}

func NewCache() *Cache {
	logger := log.Default()

	return &Cache{
		logger:     logger,
		metricData: make(map[string]MetricData),
	}
}

func (c *Cache) Get(key string) (*MetricData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metricData, exists := c.metricData[key]
	if !exists {
		return nil, nil
	}

	return &metricData, nil
}

func (c *Cache) Set(key string, metricData *MetricData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metricData[key] = *metricData
}
