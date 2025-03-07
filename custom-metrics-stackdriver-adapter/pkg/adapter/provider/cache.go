/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

// Cache for GetExternalMetric responses.
type externalMetricCache struct {
	cache       map[cacheKey]*externalMetricCacheEntry
	mu          sync.RWMutex
	cacheWindow time.Duration
}

// Cache key for GetExternalMetric requests.
type cacheKey struct {
	namespace      string
	metricSelector labels.Selector
	info           provider.ExternalMetricInfo
}

// Cache entry for GetExternalMetric responses.
type externalMetricCacheEntry struct {
	value     *external_metrics.ExternalMetricValueList
	timestamp time.Time
}

func newExternalMetricCache(cacheWindow time.Duration) *externalMetricCache {
	return &externalMetricCache{
		cache:       make(map[cacheKey]*externalMetricCacheEntry),
		cacheWindow: cacheWindow,
	}
}

func (c *externalMetricCache) Get(key cacheKey) (*external_metrics.ExternalMetricValueList, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}
	if entry.timestamp.After(time.Now().Add(-c.cacheWindow)) {
		return entry.value, true
	}

	// Entry expired.
	return nil, false
}

func (c *externalMetricCache) Set(key cacheKey, value *external_metrics.ExternalMetricValueList) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = &externalMetricCacheEntry{
		value:     value,
		timestamp: time.Now(),
	}
}
