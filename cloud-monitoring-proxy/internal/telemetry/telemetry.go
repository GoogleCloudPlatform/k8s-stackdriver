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

// Package telemetry defines the proxy's own gcmproxy_* metrics and the
// hook implementations that wire them through the gcm/poll/cache layers.
package telemetry

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"google.golang.org/grpc/codes"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/poll"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// Metrics owns the self-observability registry.
type Metrics struct {
	Registry *prometheus.Registry

	apiRequests   *prometheus.CounterVec
	billedSeries  *prometheus.CounterVec
	pollDuration  *prometheus.HistogramVec
	pollErrors    *prometheus.CounterVec
	seriesDropped *prometheus.CounterVec
	bucketResets  *prometheus.CounterVec
	sweepEvicted  *prometheus.CounterVec
}

func New(version string) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		apiRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_gcm_api_requests_total",
			Help: "Cloud Monitoring API calls by method and gRPC code.",
		}, []string{"method", "code"}),
		billedSeries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_billed_series_estimate_total",
			Help: "Estimated time series returned by reads (the unit GCM bills; min 1 per call).",
		}, []string{"metric_type"}),
		pollDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gcmproxy_poll_duration_seconds",
			Help:    "Duration of one metric type's poll (fetch+translate+apply).",
			Buckets: prometheus.DefBuckets,
		}, []string{"metric_type"}),
		pollErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_poll_errors_total",
			Help: "Polls that produced no update, by reason.",
		}, []string{"metric_type", "reason"}),
		seriesDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_series_dropped_total",
			Help: "Series dropped during translation/apply, by reason.",
		}, []string{"reason"}),
		bucketResets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_delta_bucket_resets_total",
			Help: "DELTA distribution series whose bucket layout changed (accumulation restarted).",
		}, []string{"metric_type"}),
		sweepEvicted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gcmproxy_sweep_evicted_total",
			Help: "Objects evicted by the staleness sweeper, by kind (series, families, delta_states).",
		}, []string{"kind"}),
	}
	reg.MustRegister(m.apiRequests, m.billedSeries, m.pollDuration, m.pollErrors,
		m.seriesDropped, m.bucketResets, m.sweepEvicted)

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "gcmproxy_build_info",
		Help:        "Build information.",
		ConstLabels: prometheus.Labels{"version": version},
	})
	buildInfo.Set(1)
	reg.MustRegister(buildInfo)
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

// GCMHooks instruments the API client.
func (m *Metrics) GCMHooks() gcm.Hooks {
	return gcm.Hooks{
		OnAPICall: func(method string, code codes.Code) {
			m.apiRequests.WithLabelValues(method, code.String()).Inc()
		},
		OnSeriesReturned: func(metricType string, n int) {
			if n < 1 {
				n = 1 // GCM bills at least one series per call
			}
			m.billedSeries.WithLabelValues(metricType).Add(float64(n))
		},
	}
}

// PollHooks instruments the poll pipeline.
func (m *Metrics) PollHooks() poll.Hooks {
	return poll.Hooks{
		OnPoll: func(metricType string, dur time.Duration, _ int, ts translate.Stats, cs cache.ApplyStats) {
			m.pollDuration.WithLabelValues(metricType).Observe(dur.Seconds())
			if ts.DroppedValueType > 0 {
				m.seriesDropped.WithLabelValues("value_type").Add(float64(ts.DroppedValueType))
			}
			if ts.NoPoints > 0 {
				m.seriesDropped.WithLabelValues("no_points").Add(float64(ts.NoPoints))
			}
			if cs.DroppedLimit > 0 {
				m.seriesDropped.WithLabelValues("limit").Add(float64(cs.DroppedLimit))
			}
			if ts.BucketReset > 0 {
				m.bucketResets.WithLabelValues(metricType).Add(float64(ts.BucketReset))
			}
		},
		OnPollError: func(metricType, reason string) {
			m.pollErrors.WithLabelValues(metricType, reason).Inc()
		},
		OnSweep: func(series, families, deltaStates int) {
			m.sweepEvicted.WithLabelValues("series").Add(float64(series))
			m.sweepEvicted.WithLabelValues("families").Add(float64(families))
			m.sweepEvicted.WithLabelValues("delta_states").Add(float64(deltaStates))
		},
	}
}

// ConfigCollector exposes the config store's health.
func ConfigCollector(s *config.Store) prometheus.Collector {
	loadErrs := prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "gcmproxy_config_load_errors_total",
		Help: "Config reloads that failed (previous config kept).",
	}, func() float64 { return float64(s.LoadErrors()) })
	stale := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gcmproxy_config_stale",
		Help: "1 when the most recent config reload failed.",
	}, func() float64 {
		if s.Stale() {
			return 1
		}
		return 0
	})
	return combined{loadErrs, stale}
}

type combined []prometheus.Collector

func (c combined) Describe(ch chan<- *prometheus.Desc) {
	for _, col := range c {
		col.Describe(ch)
	}
}
func (c combined) Collect(ch chan<- prometheus.Metric) {
	for _, col := range c {
		col.Collect(ch)
	}
}
