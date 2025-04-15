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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

func TestExternalMetricCache(t *testing.T) {
	key1 := cacheKey{
		namespace:      "ns1",
		metricSelector: "selector1",
		info:           provider.ExternalMetricInfo{Metric: "metric1"},
	}

	key2 := cacheKey{
		namespace:      "ns2",
		metricSelector: "selector2",
		info:           provider.ExternalMetricInfo{Metric: "metric2"},
	}

	key3 := cacheKey{
		namespace:      "ns3",
		metricSelector: "selector3",
		info:           provider.ExternalMetricInfo{Metric: "metric3"},
	}

	value1 := &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{{MetricName: "metric1", Value: *resource.NewQuantity(100, resource.DecimalSI)}},
	}

	value2 := &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{{MetricName: "metric2", Value: *resource.NewQuantity(200, resource.DecimalSI)}},
	}

	value3 := &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{{MetricName: "metric3", Value: *resource.NewQuantity(300, resource.DecimalSI)}},
	}

	tests := []struct {
		name      string
		cacheSize int
		cacheTTL  time.Duration
		setup     func(cache *externalMetricsCache)
		key       cacheKey
		wantValue *external_metrics.ExternalMetricValueList
		wantFound bool
		sleep     time.Duration
	}{
		{
			name:      "cache hit",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *externalMetricsCache) {
				cache.add(key1, value1)
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
		},
		{
			name:      "cache miss",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup:     func(cache *externalMetricsCache) {},
			key:       key2,
			wantValue: nil,
			wantFound: false,
		},
		{
			name:      "cache expired",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *externalMetricsCache) {
				cache.add(key1, value1)
			},
			key:       key1,
			wantValue: nil,
			wantFound: false,
			sleep:     2 * time.Second,
		},
		{
			name:      "cache not expired with longer TTL",
			cacheSize: 5,
			cacheTTL:  10 * time.Second,
			setup: func(cache *externalMetricsCache) {
				cache.add(key1, value1)
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
			sleep:     2 * time.Second,
		},
		{
			name:      "cache hit after set",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *externalMetricsCache) {
				cache.add(key2, value2)
			},
			key:       key2,
			wantValue: value2,
			wantFound: true,
		},
		{
			name:      "cache hit after set and get",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *externalMetricsCache) {
				cache.add(key1, value1)
				cache.get(key1) // Simulate a previous get
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
		},
		{
			name:      "cache eviction",
			cacheSize: 2,
			cacheTTL:  1 * time.Minute,
			setup: func(cache *externalMetricsCache) {
				cache.add(key1, value1)
				cache.add(key2, value2)
				// Trigger eviction
				cache.add(key3, value3)
			},
			key:       key1,
			wantValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newExternalMetricsCache(tt.cacheSize, tt.cacheTTL)
			tt.setup(cache)
			if tt.sleep > 0 {
				time.Sleep(tt.sleep)
			}
			gotValue, gotFound := cache.get(tt.key)

			if gotFound != tt.wantFound {
				t.Errorf("Get() gotFound = %v, want %v", gotFound, tt.wantFound)
			}

			if diff := cmp.Diff(tt.wantValue, gotValue); diff != "" {
				t.Errorf("Get() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
