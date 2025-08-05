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

package translator

import (
	"time"

	utilcache "k8s.io/apimachinery/pkg/util/cache"
)

type metricKindCache struct {
	cache *utilcache.LRUExpireCache
	ttl   time.Duration
}

type metricKindCacheKey struct {
	project string
	name    string
}

type cachedMetricInfo struct {
	MetricKind string
	ValueType  string
}

func newMetricKindCache(size int, ttl time.Duration) *metricKindCache {
	return &metricKindCache{
		cache: utilcache.NewLRUExpireCache(size),
		ttl:   ttl,
	}
}

func (c *metricKindCache) get(key metricKindCacheKey) (info cachedMetricInfo, ok bool) {
	var cachedValue interface{}
	if c.cache == nil {
		return
	} else if cachedValue, ok = c.cache.Get(key); !ok {
		return
	} else if info, ok = cachedValue.(cachedMetricInfo); !ok {
		c.cache.Remove(key)
	}
	return
}

func (c *metricKindCache) add(key metricKindCacheKey, value cachedMetricInfo) {
	if c.cache == nil {
		return
	}
	c.cache.Add(key, value, c.ttl)
}
