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

	value1 := &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{{MetricName: "metric1", Value: *resource.NewQuantity(100, resource.DecimalSI)}},
	}

	value2 := &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{{MetricName: "metric2", Value: *resource.NewQuantity(200, resource.DecimalSI)}},
	}

	tests := []struct {
		name        string
		cacheWindow time.Duration
		setup       func(cache *externalMetricCache)
		key         cacheKey
		wantValue   *external_metrics.ExternalMetricValueList
		wantFound   bool
		sleep       time.Duration
	}{
		{
			name:        "cache hit",
			cacheWindow: 1 * time.Second,
			setup: func(cache *externalMetricCache) {
				cache.Set(key1, value1)
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
		},
		{
			name:        "cache miss",
			cacheWindow: 1 * time.Second,
			setup:       func(cache *externalMetricCache) {},
			key:         key2,
			wantValue:   nil,
			wantFound:   false,
		},
		{
			name:        "cache expired",
			cacheWindow: 1 * time.Second,
			setup: func(cache *externalMetricCache) {
				cache.Set(key1, value1)
			},
			key:       key1,
			wantValue: nil,
			wantFound: false,
			sleep:     2 * time.Second,
		},
		{
			name:        "cache not expired with large window",
			cacheWindow: 10 * time.Second,
			setup: func(cache *externalMetricCache) {
				cache.Set(key1, value1)
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
			sleep:     2 * time.Second,
		},
		{
			name:        "cache hit after set",
			cacheWindow: 1 * time.Second,
			setup: func(cache *externalMetricCache) {
				cache.Set(key2, value2)
			},
			key:       key2,
			wantValue: value2,
			wantFound: true,
		},
		{
			name:        "cache hit after set and get",
			cacheWindow: 1 * time.Second,
			setup: func(cache *externalMetricCache) {
				cache.Set(key1, value1)
				cache.Get(key1)
			},
			key:       key1,
			wantValue: value1,
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newExternalMetricCache(tt.cacheWindow)
			tt.setup(cache)
			if tt.sleep > 0 {
				time.Sleep(tt.sleep)
			}
			gotValue, gotFound := cache.Get(tt.key)

			if gotFound != tt.wantFound {
				t.Errorf("Get() gotFound = %v, want %v", gotFound, tt.wantFound)
			}

			if diff := cmp.Diff(tt.wantValue, gotValue); diff != "" {
				t.Errorf("Get() mismatch (-want +got):\n%s", diff)
			}

		})
	}
}
