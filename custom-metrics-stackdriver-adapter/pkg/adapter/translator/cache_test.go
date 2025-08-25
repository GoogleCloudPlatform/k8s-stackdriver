package translator

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	testProjectID  = "test-project"
	testMetricName = "external.googleapis.com/testmetric"
)

func TestExternalMetricCache(t *testing.T) {
	key1 := metricKindCacheKey{
		project: testProjectID,
		name:    testMetricName,
	}

	key2 := metricKindCacheKey{
		project: testProjectID,
		name:    testMetricName + "1",
	}

	key3 := metricKindCacheKey{
		project: testProjectID,
		name:    testMetricName + "2",
	}

	value1 := cachedMetricInfo{
		MetricKind: testMetricName,
		ValueType:  "INT64",
	}

	value2 := cachedMetricInfo{
		MetricKind: testMetricName + "1",
		ValueType:  "GAUGE",
	}

	value3 := cachedMetricInfo{
		MetricKind: testMetricName + "2",
		ValueType:  "DOUBLE",
	}

	tests := []struct {
		name      string
		cacheSize int
		cacheTTL  time.Duration
		setup     func(cache *metricKindCache)
		key       metricKindCacheKey
		wantValue cachedMetricInfo
		wantFound bool
		sleep     time.Duration
	}{
		{
			name:      "cache hit",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *metricKindCache) {
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
			setup:     func(cache *metricKindCache) {},
			key:       key2,
			wantValue: cachedMetricInfo{},
			wantFound: false,
		},
		{
			name:      "cache expired",
			cacheSize: 5,
			cacheTTL:  1 * time.Second,
			setup: func(cache *metricKindCache) {
				cache.add(key1, value1)
			},
			key:       key1,
			wantValue: cachedMetricInfo{},
			wantFound: false,
			sleep:     2 * time.Second,
		},
		{
			name:      "cache not expired with longer TTL",
			cacheSize: 5,
			cacheTTL:  10 * time.Second,
			setup: func(cache *metricKindCache) {
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
			setup: func(cache *metricKindCache) {
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
			setup: func(cache *metricKindCache) {
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
			setup: func(cache *metricKindCache) {
				cache.add(key1, value1)
				cache.add(key2, value2)
				// Trigger eviction
				cache.add(key3, value3)
			},
			key:       key1,
			wantValue: cachedMetricInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newMetricKindCache(tt.cacheSize, tt.cacheTTL)
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
