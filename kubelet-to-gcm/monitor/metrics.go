/*
Copyright 2018 Google Inc.

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

package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	successfullScrapes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "successfull_scrapes_total",
			Help: "Number of successfull scrapes of metrics from the endpoint",
		},
		[]string{"source"},
	)
	failedScrapes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "failed_scrapes_total",
			Help: "Number of failed scrapes of metrics from the endpoint",
		},
		[]string{"source"},
	)
	timeseriesPushed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "timeseries_pushed_total",
			Help: "Number of timeseries successfully pushed to the Stackdriver",
		},
	)

	timeseriesDropped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "timeseries_dropped_total",
			Help: "Number of timeseries dropped during a push to the Stackdriver",
		},
	)

	metricIngestionLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "metric_ingestion_latency_seconds",
			Help:    "Time passed from the moment, when metric was scraped from the monitored component till it was pushed to the Stackdriver",
			Buckets: prometheus.ExponentialBuckets(1.0, 1.5, 12),
		},
	)
)

func init() {
	prometheus.MustRegister(successfullScrapes)
	prometheus.MustRegister(failedScrapes)
	prometheus.MustRegister(timeseriesPushed)
	prometheus.MustRegister(timeseriesDropped)
	prometheus.MustRegister(metricIngestionLatency)
}

func observeSuccessfullScrape(source string) {
	successfullScrapes.WithLabelValues(source).Inc()
}

func observeFailedScrape(source string) {
	failedScrapes.WithLabelValues(source).Inc()
}

func observeSuccessfullRequest(batchSize int) {
	timeseriesPushed.Add(float64(batchSize))
}

func observeFailedRequest(batchSize int) {
	timeseriesDropped.Add(float64(batchSize))
}

func observeIngestionLatency(numTimeseries int, latency float64) {
	for i := 0; i < numTimeseries; i++ {
		metricIngestionLatency.Observe(latency)
	}
}
