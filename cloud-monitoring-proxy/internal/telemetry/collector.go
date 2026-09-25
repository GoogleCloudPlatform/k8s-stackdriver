/*
Copyright 2026 Google Inc.

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

package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
)

var (
	dataAgeDesc = prometheus.NewDesc(
		"gcmproxy_data_age_seconds",
		"Age of the newest cached point per metric type — the true staleness of served data (GCM ingestion delay + poll lag).",
		[]string{"metric_type"}, nil)
	seriesCachedDesc = prometheus.NewDesc(
		"gcmproxy_series_cached",
		"Series currently cached per metric type.",
		[]string{"metric_type"}, nil)
)

// cacheCollector computes freshness/size stats from the store at scrape
// time, so data age is always current rather than as-of the last poll.
type cacheCollector struct {
	store *cache.Store
	now   func() time.Time
}

// CacheCollector exposes gcmproxy_data_age_seconds and
// gcmproxy_series_cached from the store.
func CacheCollector(store *cache.Store) prometheus.Collector {
	return &cacheCollector{store: store, now: time.Now}
}

func (c *cacheCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- dataAgeDesc
	ch <- seriesCachedDesc
}

func (c *cacheCollector) Collect(ch chan<- prometheus.Metric) {
	now := c.now()
	c.store.Read(func(fams []*cache.Family) {
		for _, f := range fams {
			var newest time.Time
			for _, sr := range f.Series {
				if sr.PointEnd.After(newest) {
					newest = sr.PointEnd
				}
			}
			if !newest.IsZero() {
				ch <- prometheus.MustNewConstMetric(dataAgeDesc, prometheus.GaugeValue,
					now.Sub(newest).Seconds(), f.MetricType)
			}
			ch <- prometheus.MustNewConstMetric(seriesCachedDesc, prometheus.GaugeValue,
				float64(len(f.Series)), f.MetricType)
		}
	})
}
