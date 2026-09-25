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

package translate

import "testing"

// Examples straight from the "PromQL for Cloud Monitoring" mapping doc,
// plus the prometheus.googleapis.com restoration cases.
func TestPromName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"kubernetes.io/container/cpu/limit_cores", "kubernetes_io:container_cpu_limit_cores"},
		{"kubernetes.io/container/cpu/core_usage_time", "kubernetes_io:container_cpu_core_usage_time"},
		{"compute.googleapis.com/instance/cpu/utilization", "compute_googleapis_com:instance_cpu_utilization"},
		{"logging.googleapis.com/log_entry_count", "logging_googleapis_com:log_entry_count"},
		{"agent.googleapis.com/disk/io_time", "agent_googleapis_com:disk_io_time"},
		{"custom.googleapis.com/opencensus/opencensus.io/http/server/request_count_by_method",
			"custom_googleapis_com:opencensus_opencensus_io_http_server_request_count_by_method"},
		// GMP-ingested Prometheus metrics: original name restored.
		{"prometheus.googleapis.com/kube_pod_status_phase/gauge", "kube_pod_status_phase"},
		{"prometheus.googleapis.com/apiserver_request_total/counter", "apiserver_request_total"},
		{"prometheus.googleapis.com/apiserver_request_duration_seconds/histogram", "apiserver_request_duration_seconds"},
		{"prometheus.googleapis.com/some_untyped_thing/unknown:counter", "some_untyped_thing"},
		{"prometheus.googleapis.com/DCGM_FI_DEV_GPU_UTIL/gauge", "DCGM_FI_DEV_GPU_UTIL"},
		// Degenerate inputs stay deterministic.
		{"nodomainmetric", "nodomainmetric"},
	}
	for _, tc := range cases {
		if got := PromName(tc.in); got != tc.want {
			t.Errorf("PromName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeLabelName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pod_name", "pod_name"},
		{"pod-name", "pod_name"},
		{"pod.name", "pod_name"},
		{"9lives", "_lives"},
		{"__reserved", "_reserved"},
		{"____very_reserved", "_very_reserved"},
		{"", "_"},
	}
	for _, tc := range cases {
		if got := SanitizeLabelName(tc.in); got != tc.want {
			t.Errorf("SanitizeLabelName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
