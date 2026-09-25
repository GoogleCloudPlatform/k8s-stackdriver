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

package poll

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/scope"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

var t0 = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func desc(typ string, kind metric.MetricDescriptor_MetricKind, resTypes []string, delay, period time.Duration) *metric.MetricDescriptor {
	md := &metric.MetricDescriptor{
		Type: typ, MetricKind: kind, ValueType: metric.MetricDescriptor_INT64,
		MonitoredResourceTypes: resTypes,
	}
	if delay > 0 || period > 0 {
		md.Metadata = &metric.MetricDescriptor_MetricDescriptorMetadata{
			IngestDelay: durationpb.New(delay), SamplePeriod: durationpb.New(period),
		}
	}
	return md
}

func ipoint(end time.Time, v int64) *monitoringpb.Point {
	return &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{Int64Value: v}},
	}
}

func tseries(resType string, points ...*monitoringpb.Point) *monitoringpb.TimeSeries {
	return &monitoringpb.TimeSeries{
		Resource: &monitoredres.MonitoredResource{Type: resType, Labels: map[string]string{"node_name": "n1"}},
		Metric:   &metric.Metric{},
		Points:   points,
	}
}

type env struct {
	fake  *gcm.Fake
	descs *gcm.DescriptorCache
	store *cache.Store
	mgr   *Manager
	errs  map[string][]string // metricType -> reasons
	mu    sync.Mutex
}

func newEnv(t *testing.T, descriptors ...*metric.MetricDescriptor) *env {
	t.Helper()
	e := &env{fake: &gcm.Fake{Descriptors: descriptors}, store: cache.NewStore(), errs: map[string][]string{}}
	e.descs = gcm.NewDescriptorCache(e.fake)
	if err := e.descs.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.mgr = New(e.fake, e.descs, e.store, scope.Scope{
		ProjectID: "p", Location: "us-central1", ClusterName: "c", ClusterScopeFilter: true,
	}, Hooks{OnPollError: func(mt, reason string) {
		e.mu.Lock()
		e.errs[mt] = append(e.errs[mt], reason)
		e.mu.Unlock()
	}})
	e.mgr.maxJitter = 0
	e.mgr.now = func() time.Time { return t0 }
	return e
}

func baseCfg(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *env) reasons(mt string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.errs[mt]...)
}

func TestAutoLookbackWindow(t *testing.T) {
	const mt = "kubernetes.io/node/x"
	e := newEnv(t, desc(mt, metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 240*time.Second, 60*time.Second))
	e.mgr.pollType(context.Background(), config.Metric{Type: mt}, mt, baseCfg(t))

	q := e.fake.SeriesCalls[0]
	if got, want := q.End.Sub(q.Start), 240*time.Second+60*time.Second+lookbackMargin; got != want {
		t.Errorf("lookback window = %v, want %v", got, want)
	}

	// Explicit override wins.
	e.mgr.pollType(context.Background(), config.Metric{Type: mt, Lookback: config.Duration(10 * time.Minute)}, mt, baseCfg(t))
	if q := e.fake.SeriesCalls[1]; q.End.Sub(q.Start) != 10*time.Minute {
		t.Errorf("override window = %v", q.End.Sub(q.Start))
	}

	// No metadata → default.
	const mt2 = "custom.googleapis.com/nometa"
	e2 := newEnv(t, desc(mt2, metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 0))
	e2.mgr.pollType(context.Background(), config.Metric{Type: mt2}, mt2, baseCfg(t))
	if q := e2.fake.SeriesCalls[0]; q.End.Sub(q.Start) != defaultLookback {
		t.Errorf("default window = %v", q.End.Sub(q.Start))
	}
}

func TestScopeFilterInjection(t *testing.T) {
	k8s := desc("kubernetes.io/node/x", metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second)
	prom := desc("prometheus.googleapis.com/kube_pod_status_phase/gauge", metric.MetricDescriptor_GAUGE, []string{"prometheus_target"}, 0, 30*time.Second)
	gce := desc("compute.googleapis.com/instance/cpu/utilization", metric.MetricDescriptor_GAUGE, []string{"gce_instance"}, 0, 60*time.Second)
	e := newEnv(t, k8s, prom, gce)
	cfg := baseCfg(t)

	e.mgr.pollType(context.Background(), config.Metric{Type: k8s.Type, Filter: `metric.labels.x = "y"`}, k8s.Type, cfg)
	extras := e.fake.SeriesCalls[0].ExtraFilters
	joined := strings.Join(extras, " | ")
	if !strings.Contains(joined, `resource.labels.cluster_name = "c"`) || !strings.Contains(joined, `resource.labels.location = "us-central1"`) {
		t.Errorf("k8s scope filter missing: %v", extras)
	}
	if !strings.Contains(joined, `metric.labels.x = "y"`) {
		t.Errorf("user filter not ANDed: %v", extras)
	}

	e.mgr.pollType(context.Background(), config.Metric{Type: prom.Type}, prom.Type, cfg)
	if got := strings.Join(e.fake.SeriesCalls[1].ExtraFilters, " "); !strings.Contains(got, `resource.labels.cluster = "c"`) {
		t.Errorf("prometheus_target scope filter wrong: %v", got)
	}

	// gce_instance with scoping on: refused, no API call.
	before := e.fake.NumSeriesCalls()
	e.mgr.pollType(context.Background(), config.Metric{Type: gce.Type}, gce.Type, cfg)
	if e.fake.NumSeriesCalls() != before {
		t.Error("unscopable metric was polled anyway")
	}
	if rs := e.reasons(gce.Type); len(rs) != 1 || rs[0] != "unscopable" {
		t.Errorf("reasons = %v", rs)
	}

	// Scoping off: gce_instance polls with no extras.
	e.mgr.scope.ClusterScopeFilter = false
	e.mgr.pollType(context.Background(), config.Metric{Type: gce.Type}, gce.Type, cfg)
	if got := e.fake.SeriesCalls[e.fake.NumSeriesCalls()-1].ExtraFilters; len(got) != 0 {
		t.Errorf("unscoped poll has extras: %v", got)
	}
}

func TestDeltaAccumulationAcrossPollsAndEviction(t *testing.T) {
	const mt = "kubernetes.io/node/events"
	e := newEnv(t, desc(mt, metric.MetricDescriptor_DELTA, []string{"k8s_node"}, 0, 60*time.Second))
	cfg := baseCfg(t)
	entry := config.Metric{Type: mt}

	step := func(at time.Time, pts ...*monitoringpb.Point) {
		e.mgr.now = func() time.Time { return at }
		e.fake.SetSeries(mt, []*monitoringpb.TimeSeries{tseries("k8s_node", pts...)})
		e.mgr.pollType(context.Background(), entry, mt, cfg)
	}
	step(t0, ipoint(t0, 10))
	step(t0.Add(time.Minute), ipoint(t0.Add(time.Minute), 5), ipoint(t0, 10)) // overlap resend
	step(t0.Add(2*time.Minute), ipoint(t0.Add(2*time.Minute), 2))

	var got float64
	e.store.Read(func(fams []*cache.Family) {
		got = fams[0].SortedSeries()[0].Value
	})
	if got != 17 {
		t.Fatalf("accumulated = %v, want 17", got)
	}

	// Series disappears; sweep evicts cache series and delta state.
	e.mgr.now = func() time.Time { return t0.Add(20 * time.Minute) }
	e.mgr.SweepOnce(5 * time.Minute)
	if fams, series := e.store.Stats(); fams != 0 || series != 0 {
		t.Errorf("sweep left %d/%d", fams, series)
	}
	// Reappearance restarts accumulation (counter reset semantics).
	step(t0.Add(21*time.Minute), ipoint(t0.Add(21*time.Minute), 4))
	e.store.Read(func(fams []*cache.Family) {
		if v := fams[0].SortedSeries()[0].Value; v != 4 {
			t.Errorf("post-eviction value = %v, want 4", v)
		}
	})
}

func TestQuotaErrorSkipsCycle(t *testing.T) {
	const mt = "kubernetes.io/node/x"
	e := newEnv(t, desc(mt, metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second))
	e.fake.SetErr(status.Error(codes.ResourceExhausted, "quota"))
	e.mgr.pollType(context.Background(), config.Metric{Type: mt}, mt, baseCfg(t))
	if rs := e.reasons(mt); len(rs) != 1 || rs[0] != "quota" {
		t.Errorf("reasons = %v", rs)
	}
	if _, series := e.store.Stats(); series != 0 {
		t.Error("failed poll wrote to store")
	}
}

func TestPrefixExpansionAndNoMatch(t *testing.T) {
	a := desc("kubernetes.io/node/a", metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second)
	b := desc("kubernetes.io/node/b", metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second)
	e := newEnv(t, a, b)
	cfg := baseCfg(t)

	e.mgr.pollEntry(context.Background(), config.Metric{TypePrefix: "kubernetes.io/node/"}, cfg)
	if n := e.fake.NumSeriesCalls(); n != 2 {
		t.Errorf("prefix polled %d types, want 2", n)
	}
	e.mgr.pollEntry(context.Background(), config.Metric{TypePrefix: "nosuch.io/"}, cfg)
	if rs := e.reasons("nosuch.io/"); len(rs) != 1 || rs[0] != "prefix_no_match" {
		t.Errorf("reasons = %v", rs)
	}
}

func TestRewireAndReady(t *testing.T) {
	a := desc("kubernetes.io/node/a", metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second)
	b := desc("kubernetes.io/node/b", metric.MetricDescriptor_GAUGE, []string{"k8s_node"}, 0, 60*time.Second)
	e := newEnv(t, a, b)

	cfgA, err := config.Parse([]byte("metrics:\n  - type: kubernetes.io/node/a\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfgB, err := config.Parse([]byte("metrics:\n  - type: kubernetes.io/node/b\n"))
	if err != nil {
		t.Fatal(err)
	}

	if e.mgr.Ready() {
		t.Error("Ready before any wiring with metrics pending")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e.mgr.Rewire(ctx, cfgA)
	waitFor(t, "first poll of a", func() bool {
		return countCalls(e.fake, "kubernetes.io/node/a") >= 1
	})
	if !e.mgr.Ready() {
		t.Error("not Ready after first poll")
	}

	e.mgr.Rewire(ctx, cfgB)
	waitFor(t, "first poll of b", func() bool {
		return countCalls(e.fake, "kubernetes.io/node/b") >= 1
	})
	aCalls := countCalls(e.fake, "kubernetes.io/node/a")
	time.Sleep(100 * time.Millisecond)
	if countCalls(e.fake, "kubernetes.io/node/a") != aCalls {
		t.Error("old scheduler still polling after rewire")
	}

	// Empty config → ready by definition.
	e.mgr.Rewire(ctx, baseCfg(t))
	e.mgr.firstPoll.Store(false)
	if !e.mgr.Ready() {
		t.Error("empty config not Ready")
	}
}

func countCalls(f *gcm.Fake, mt string) int {
	n := 0
	for i := 0; i < f.NumSeriesCalls(); i++ {
		if f.SeriesCalls[i].MetricType == mt {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Guard: translate.Stats surface used by hooks stays wired.
var _ = translate.Stats{}
