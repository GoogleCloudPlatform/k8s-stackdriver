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

package export

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler serves the merged gatherers in Prometheus exposition
// format with standard content negotiation.
func MetricsHandler(gs ...prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(prometheus.Gatherers(gs), promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// ReadyzHandler returns 200 once ready() is true, 503 before.
func ReadyzHandler(ready func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if ready() {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintln(w, "ok")
			return
		}
		http.Error(w, "not ready: first poll cycle not completed", http.StatusServiceUnavailable)
	}
}
