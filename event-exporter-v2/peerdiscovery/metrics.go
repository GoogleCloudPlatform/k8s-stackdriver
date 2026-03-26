/*
Copyright 2024 Google Inc.

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

package peerdiscovery

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	peerCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:      "peer_count",
			Help:      "Current number of known peer pods",
			Subsystem: "event_exporter",
		},
	)

	peerUpdatesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "peer_updates_total",
			Help:      "Total number of peer list updates from Endpoints changes",
			Subsystem: "event_exporter",
		},
	)
)

func init() {
	prometheus.MustRegister(peerCount, peerUpdatesTotal)
}
