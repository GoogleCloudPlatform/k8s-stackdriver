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

package translator

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	componentMetricsAvailability = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "component_metrics_availability",
			Help: "Contains true(1.0) if /metrics endpoint of the component is available, otherwise false(0.0)",
		},
		[]string{"component"},
	)

	timeseriesPushed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "timeseries_pushed_total",
			Help: "Number of timeseries successfully pushed to the Stackdriver",
		},
		[]string{"component"},
	)

	timeseriesDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "timeseries_dropped_total",
			Help: "Number of timeseries dropped during a push to the Stackdriver",
		},
		[]string{"component"},
	)

	metricFamilyDropped = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "metric_family_dropped",
			Help: "Contains true(1.0) if metrics of specific family where not pushed, due to the incompatible change of metric descriptor, otherwise false(0.0)",
		},
		[]string{"component", "metric_name"},
	)
)

func init() {
	prometheus.MustRegister(componentMetricsAvailability)
	prometheus.MustRegister(timeseriesPushed)
	prometheus.MustRegister(timeseriesDropped)
	prometheus.MustRegister(metricFamilyDropped)
}
