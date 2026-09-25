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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/poll"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/scope"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

var t0 = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func fixtureStore() *cache.Store {
	s := cache.NewStore()
	s.Apply("kubernetes.io/node/cpu/allocatable_utilization", translate.FamilyUpdate{
		Name: "kubernetes_io:node_cpu_allocatable_utilization",
		Help: "node cpu utilization",
		Kind: translate.KindGauge,
		Samples: []translate.Sample{{
			Labels:   labels.FromStrings("monitored_resource", "k8s_node", "node_name", "n1"),
			Value:    0.5,
			PointEnd: t0,
		}},
	}, 10, t0)
	s.Apply("kubernetes.io/node/events", translate.FamilyUpdate{
		Name: "kubernetes_io:node_events",
		Help: "events",
		Kind: translate.KindCounter,
		Samples: []translate.Sample{{
			Labels:   labels.FromStrings("monitored_resource", "k8s_node", "node_name", "n1"),
			Value:    42,
			PointEnd: t0,
		}},
	}, 10, t0)
	s.Apply("x.googleapis.com/latency", translate.FamilyUpdate{
		Name: "x_googleapis_com:latency",
		Help: "latency",
		Kind: translate.KindHistogram,
		Samples: []translate.Sample{{
			Labels:   labels.FromStrings("monitored_resource", "gce_instance"),
			Hist:     &translate.HistogramValue{Count: 9, Sum: 27, UpperBounds: []float64{1, 5}, CumCounts: []uint64{2, 5}},
			PointEnd: t0,
		}},
	}, 10, t0)
	return s
}

const goldenNoTS = `# HELP kubernetes_io:node_cpu_allocatable_utilization node cpu utilization
# TYPE kubernetes_io:node_cpu_allocatable_utilization gauge
kubernetes_io:node_cpu_allocatable_utilization{monitored_resource="k8s_node",node_name="n1"} 0.5
# HELP kubernetes_io:node_events events
# TYPE kubernetes_io:node_events counter
kubernetes_io:node_events{monitored_resource="k8s_node",node_name="n1"} 42
# HELP x_googleapis_com:latency latency
# TYPE x_googleapis_com:latency histogram
x_googleapis_com:latency_bucket{monitored_resource="gce_instance",le="1"} 2
x_googleapis_com:latency_bucket{monitored_resource="gce_instance",le="5"} 5
x_googleapis_com:latency_bucket{monitored_resource="gce_instance",le="+Inf"} 9
x_googleapis_com:latency_sum{monitored_resource="gce_instance"} 27
x_googleapis_com:latency_count{monitored_resource="gce_instance"} 9
`

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("scrape status %d: %s", resp.StatusCode, body)
	}
	return string(body)
}

func TestGoldenExposition(t *testing.T) {
	store := fixtureStore()

	got := scrape(t, MetricsHandler(NewGatherer(store, func() bool { return false })))
	if got != goldenNoTS {
		t.Errorf("no-timestamp exposition mismatch:\n--- got ---\n%s--- want ---\n%s", got, goldenNoTS)
	}

	// Honest-timestamp mode: every sample line carries the GCM millis.
	got = scrape(t, MetricsHandler(NewGatherer(store, func() bool { return true })))
	suffix := " " + strconv.FormatInt(t0.UnixMilli(), 10)
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasSuffix(line, suffix) {
			t.Errorf("sample line missing timestamp%s: %q", suffix, line)
		}
	}

	// Round-trip: output must parse as valid exposition text under the
	// legacy (pre-UTF-8) name rules our GMP-style names target.
	parser := expfmt.NewTextParser(model.LegacyValidation)
	fams, err := parser.TextToMetricFamilies(strings.NewReader(got))
	if err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
	if len(fams) != 3 {
		t.Errorf("parsed %d families, want 3", len(fams))
	}
}

func TestReadyz(t *testing.T) {
	ready := false
	srv := httptest.NewServer(ReadyzHandler(func() bool { return ready }))
	defer srv.Close()
	if resp, _ := http.Get(srv.URL); resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("not-ready status = %d", resp.StatusCode)
	}
	ready = true
	if resp, _ := http.Get(srv.URL); resp.StatusCode != http.StatusOK {
		t.Errorf("ready status = %d", resp.StatusCode)
	}
}

// TestPipelineOverHTTP runs fake GCM → descriptor cache → poller → cache →
// Gatherer → real HTTP scrape.
func TestPipelineOverHTTP(t *testing.T) {
	md := &metric.MetricDescriptor{
		Type:       "kubernetes.io/node/cpu/allocatable_utilization",
		MetricKind: metric.MetricDescriptor_GAUGE, ValueType: metric.MetricDescriptor_DOUBLE,
		Description: "node cpu", MonitoredResourceTypes: []string{"k8s_node"},
	}
	fake := &gcm.Fake{Descriptors: []*metric.MetricDescriptor{md}}
	fake.SetSeries(md.Type, []*monitoringpb.TimeSeries{{
		Resource: &monitoredres.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "n1", "cluster_name": "c"}},
		Metric:   &metric.Metric{},
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(t0)},
			Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.75}},
		}},
	}})
	descs := gcm.NewDescriptorCache(fake)
	if err := descs.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	store := cache.NewStore()
	mgr := poll.New(fake, descs, store, scope.Scope{ProjectID: "p", Location: "l", ClusterName: "c", ClusterScopeFilter: true}, poll.Hooks{})

	cfg, err := config.Parse([]byte("metrics:\n  - type: " + md.Type + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Rewire(ctx, cfg)

	deadline := time.Now().Add(5 * time.Second)
	for !mgr.Ready() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !mgr.Ready() {
		t.Fatal("poller never became ready")
	}

	body := scrape(t, MetricsHandler(NewGatherer(store, func() bool { return false })))
	want := `kubernetes_io:node_cpu_allocatable_utilization{cluster_name="c",monitored_resource="k8s_node",node_name="n1"} 0.75`
	if !strings.Contains(body, want) {
		t.Errorf("scrape missing %q; got:\n%s", want, body)
	}
}
