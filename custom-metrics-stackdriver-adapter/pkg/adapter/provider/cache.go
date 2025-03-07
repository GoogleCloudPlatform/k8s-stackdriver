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
	"time"

	utilcache "k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

// externalMetricsCache encapsulates the cache for external metrics.
type externalMetricsCache struct {
	cache *utilcache.LRUExpireCache
	ttl   time.Duration
}

// Cache key for GetExternalMetric requests.
type cacheKey struct {
	namespace      string
	metricSelector string
	info           provider.ExternalMetricInfo
}

func newExternalMetricsCache(size int, ttl time.Duration) *externalMetricsCache {
	return &externalMetricsCache{
		cache: utilcache.NewLRUExpireCache(size),
		ttl:   ttl,
	}
}

func (c *externalMetricsCache) get(key cacheKey) (*external_metrics.ExternalMetricValueList, bool) {
	if c.cache == nil {
		return nil, false
	}
	cachedValue, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	metrics, castOk := cachedValue.(*external_metrics.ExternalMetricValueList)
	if !castOk {
		// Remove corrupt entry
		c.cache.Remove(key)
		return nil, false
	}

	return metrics, true
}

func (c *externalMetricsCache) add(key cacheKey, value *external_metrics.ExternalMetricValueList) {
	if c.cache == nil {
		return
	}
	c.cache.Add(key, value, c.ttl)
}
