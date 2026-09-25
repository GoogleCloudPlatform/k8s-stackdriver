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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
)

func TestDiscoveryHandler(t *testing.T) {
	fake := &gcm.Fake{Descriptors: []*metric.MetricDescriptor{
		{
			Type: "kubernetes.io/node/cpu/core_usage_time", MetricKind: metric.MetricDescriptor_CUMULATIVE,
			ValueType: metric.MetricDescriptor_DOUBLE, Unit: "s{CPU}",
			MonitoredResourceTypes: []string{"k8s_node"}, Description: "cpu time",
			Metadata: &metric.MetricDescriptor_MetricDescriptorMetadata{
				SamplePeriod: durationpb.New(60 * time.Second),
				IngestDelay:  durationpb.New(4 * time.Minute),
			},
		},
		{Type: "prometheus.googleapis.com/kube_pod_status_phase/gauge", MetricKind: metric.MetricDescriptor_GAUGE,
			ValueType: metric.MetricDescriptor_DOUBLE, MonitoredResourceTypes: []string{"prometheus_target"}},
	}}
	descs := gcm.NewDescriptorCache(fake)
	if err := descs.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(DiscoveryHandler(descs))
	defer srv.Close()

	get := func(url string) discoveryResponse {
		t.Helper()
		resp, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var out discoveryResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	all := get(srv.URL)
	if all.Count != 2 || len(all.Metrics) != 2 {
		t.Fatalf("all: %+v", all)
	}
	k8s := all.Metrics[0]
	if k8s.PromName != "kubernetes_io:node_cpu_core_usage_time" || k8s.Kind != "CUMULATIVE" ||
		k8s.SamplePeriod != "1m0s" || k8s.IngestDelay != "4m0s" || k8s.Unit != "s{CPU}" {
		t.Errorf("k8s entry: %+v", k8s)
	}
	if gmp := all.Metrics[1]; gmp.PromName != "kube_pod_status_phase" {
		t.Errorf("gmp promName = %q", gmp.PromName)
	}

	filtered := get(srv.URL + "?prefix=kubernetes.io/")
	if filtered.Count != 1 || filtered.Metrics[0].Type != "kubernetes.io/node/cpu/core_usage_time" {
		t.Errorf("prefix filter: %+v", filtered)
	}
	if empty := get(srv.URL + "?prefix=nosuch/"); empty.Count != 0 {
		t.Errorf("empty filter: %+v", empty)
	}
}
