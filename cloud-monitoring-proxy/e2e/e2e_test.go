//go:build e2e

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

// End-to-end verification against a live cluster running the proxy.
// Requires: ADC with monitoring read on the project, kubectl context
// pointing at the e2e cluster, the proxy deployed per deploy/manifests.
//
//	make e2e
//
// Env (defaults match hack/cluster-up.sh): E2E_PROJECT, E2E_CLUSTER,
// E2E_LOCATION, E2E_NAMESPACE.
package e2e

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
)

var (
	project  = os.Getenv("E2E_PROJECT")
	cluster  = envOr("E2E_CLUSTER", "cmproxy-e2e")
	location = envOr("E2E_LOCATION", "us-central1-a")
	ns       = envOr("E2E_NAMESPACE", "cloud-monitoring-proxy")

	baseURL = "http://localhost:19093"
)

// httpc bounds every request so a dropped port-forward surfaces as an
// error rather than hanging the suite.
var httpc = &http.Client{Timeout: 30 * time.Second}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func TestMain(m *testing.M) {
	if project == "" {
		fmt.Fprintln(os.Stderr, "E2E_PROJECT must be set")
		os.Exit(1)
	}
	// Self-healing port-forward: kubectl port-forward drops on long
	// sessions; restart it until the suite is done.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for ctx.Err() == nil {
			pf := exec.CommandContext(ctx, "kubectl", "-n", ns,
				"port-forward", "svc/cloud-monitoring-proxy", "19093:9090")
			// Discard output: inheriting go test's stderr pipe makes the
			// test binary wait on the child's I/O after exit.
			pf.Stdout, pf.Stderr = io.Discard, io.Discard
			pf.WaitDelay = 5 * time.Second
			_ = pf.Run()
			if ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, "port-forward exited; restarting in 2s")
				time.Sleep(2 * time.Second)
			}
		}
	}()

	ok := false
	for i := 0; i < 30; i++ {
		if resp, err := httpc.Get(baseURL + "/readyz"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				ok = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "proxy never became ready through port-forward")
		cancel()
		os.Exit(1)
	}
	code := m.Run()
	cancel()
	os.Exit(code)
}

func scrapeErr() (map[string]*dto.MetricFamily, error) {
	resp, err := httpc.Get(baseURL + "/metrics")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	parser := expfmt.NewTextParser(model.LegacyValidation)
	return parser.TextToMetricFamilies(resp.Body)
}

func scrape(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()
	fams, err := scrapeErr()
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	return fams
}

func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func metricValue(m *dto.Metric) float64 {
	switch {
	case m.Gauge != nil:
		return m.Gauge.GetValue()
	case m.Counter != nil:
		return m.Counter.GetValue()
	default:
		return math.NaN()
	}
}

// TestFamiliesPresent: the preset families are served with plausible values.
func TestFamiliesPresent(t *testing.T) {
	fams := scrape(t)
	cpu, ok := fams["kubernetes_io:node_cpu_allocatable_utilization"]
	if !ok || len(cpu.Metric) < 2 {
		t.Fatalf("node cpu family missing or too few series: %v", cpu)
	}
	for _, m := range cpu.Metric {
		if v := metricValue(m); v <= 0 || v > 1.5 {
			t.Errorf("implausible node cpu utilization %v on %s", v, labelValue(m, "node_name"))
		}
		if labelValue(m, "cluster_name") != cluster || labelValue(m, "monitored_resource") != "k8s_node" {
			t.Errorf("scope labels wrong: %v", m.GetLabel())
		}
	}
	if mem, ok := fams["kubernetes_io:node_memory_used_bytes"]; !ok || metricValue(mem.Metric[0]) < 1e8 {
		t.Errorf("node memory family missing or implausible")
	}
	for _, want := range []string{
		"kubernetes_io:container_restart_count",
		"kubernetes_io:container_memory_used_bytes",
		"kube_pod_status_phase", // kube-state package via prometheus.googleapis.com restore
		"gcmproxy_build_info",
		"gcmproxy_data_age_seconds",
	} {
		if _, ok := fams[want]; !ok {
			t.Errorf("family %s missing from scrape", want)
		}
	}
}

// TestCrossCheckAgainstGCM: proxy gauge values must equal one of the two
// most recent points GCM itself serves for the same series (the proxy may
// legitimately be one sample behind).
func TestCrossCheckAgainstGCM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := gcm.New(ctx, project, 4, gcm.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	fams := scrape(t)
	checked := 0
	for _, mt := range []struct{ typ, fam string }{
		{"kubernetes.io/node/cpu/allocatable_utilization", "kubernetes_io:node_cpu_allocatable_utilization"},
		{"kubernetes.io/node/memory/used_bytes", "kubernetes_io:node_memory_used_bytes"},
		{"kubernetes.io/container/restart_count", "kubernetes_io:container_restart_count"},
	} {
		fam, ok := fams[mt.fam]
		if !ok {
			t.Errorf("family %s missing", mt.fam)
			continue
		}
		end := time.Now()
		series, err := c.ListSeries(ctx, gcm.SeriesQuery{
			MetricType: mt.typ,
			ExtraFilters: []string{
				`resource.labels.cluster_name = "` + cluster + `"`,
				`resource.labels.location = "` + location + `"`,
			},
			Start: end.Add(-15 * time.Minute), End: end, PageSize: 1000,
		})
		if err != nil {
			t.Fatalf("direct GCM read %s: %v", mt.typ, err)
		}
		// Index GCM's last two points per series key (node or pod/container).
		type recent struct{ v0, v1 float64 }
		direct := map[string]recent{}
		for _, ts := range series {
			// memory_type (metric label) distinguishes evictable vs
			// non-evictable series on the same node/container.
			key := ts.GetResource().GetLabels()["node_name"] + "/" +
				ts.GetResource().GetLabels()["pod_name"] + "/" +
				ts.GetResource().GetLabels()["container_name"] + "/" +
				ts.GetMetric().GetLabels()["memory_type"]
			pts := ts.GetPoints() // newest first
			r := recent{v0: math.NaN(), v1: math.NaN()}
			if len(pts) > 0 {
				r.v0 = pointVal(pts[0])
			}
			if len(pts) > 1 {
				r.v1 = pointVal(pts[1])
			}
			direct[key] = r
		}
		for _, m := range fam.Metric {
			key := labelValue(m, "node_name") + "/" + labelValue(m, "pod_name") + "/" +
				labelValue(m, "container_name") + "/" + labelValue(m, "memory_type")
			r, ok := direct[key]
			if !ok {
				t.Errorf("%s: proxy serves %q but direct GCM read does not", mt.fam, key)
				continue
			}
			v := metricValue(m)
			if v != r.v0 && v != r.v1 {
				t.Errorf("%s %q: proxy=%v, GCM last two points=%v,%v", mt.fam, key, v, r.v0, r.v1)
			}
			checked++
		}
	}
	if checked < 3 {
		t.Fatalf("cross-checked only %d series, want >= 3", checked)
	}
	t.Logf("cross-checked %d series against direct GCM reads", checked)
}

// TestDataAgeWithinBounds: served staleness must stay inside
// ingestDelay + 2×samplePeriod (+ the 30s lookback margin).
func TestDataAgeWithinBounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := gcm.New(ctx, project, 4, gcm.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	dc := gcm.NewDescriptorCache(c)
	if err := dc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	fams := scrape(t)
	ages, ok := fams["gcmproxy_data_age_seconds"]
	if !ok || len(ages.Metric) == 0 {
		t.Fatal("gcmproxy_data_age_seconds missing")
	}
	checked := 0
	for _, m := range ages.Metric {
		mt := labelValue(m, "metric_type")
		md, ok := dc.Get(mt)
		if !ok {
			continue
		}
		delay := md.GetMetadata().GetIngestDelay().AsDuration()
		period := md.GetMetadata().GetSamplePeriod().AsDuration()
		if period == 0 {
			continue
		}
		bound := (delay + 2*period + 30*time.Second).Seconds()
		if age := metricValue(m); age > bound {
			t.Errorf("%s: data age %.0fs exceeds bound %.0fs (delay %s, period %s)", mt, age, bound, delay, period)
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("checked only %d families", checked)
	}
	t.Logf("data age within bounds for %d metric types", checked)
}

// TestBilledEstimateAdvances: over ~70s (one poll cycle), the billed-series
// estimate must advance by roughly the cached-series count (loose sanity
// bounds; all entries poll at 60s).
func TestBilledEstimateAdvances(t *testing.T) {
	sum := func(fams map[string]*dto.MetricFamily, name string) float64 {
		total := 0.0
		if f, ok := fams[name]; ok {
			for _, m := range f.Metric {
				total += metricValue(m)
			}
		}
		return total
	}
	s1 := scrape(t)
	before := sum(s1, "gcmproxy_billed_series_estimate_total")
	cached := sum(s1, "gcmproxy_series_cached")
	time.Sleep(70 * time.Second)
	s2 := scrape(t)
	delta := sum(s2, "gcmproxy_billed_series_estimate_total") - before

	if delta <= 0 {
		t.Fatalf("billed estimate did not advance over a poll cycle")
	}
	// 70s covers 1–2 cycles; empty types bill min-1 each (55 types configured).
	if delta < 0.5*cached || delta > 2.5*cached+2*55 {
		t.Errorf("billed delta %.0f implausible vs %f cached series", delta, cached)
	}
	t.Logf("billed series delta over one cycle: %.0f (cached %.0f)", delta, cached)
}

// TestDeltaCountersMonotonic: delta-accumulated counters (pod network)
// never decrease between scrapes.
func TestDeltaCountersMonotonic(t *testing.T) {
	read := func() map[string]float64 {
		out := map[string]float64{}
		if f, ok := scrape(t)["kubernetes_io:pod_network_received_bytes_count"]; ok {
			for _, m := range f.Metric {
				out[labelValue(m, "pod_name")] = metricValue(m)
			}
		}
		return out
	}
	first := read()
	if len(first) == 0 {
		t.Skip("no pod network data yet")
	}
	time.Sleep(65 * time.Second)
	second := read()
	for pod, v1 := range first {
		if v2, ok := second[pod]; ok && v2 < v1 {
			t.Errorf("delta counter went backwards for %s: %v -> %v", pod, v1, v2)
		}
	}
	t.Logf("monotonicity held for %d pod series", len(first))
}

// TestStalenessEviction: series of a deleted workload disappear within
// lookback + staleAfter + sweep interval. Slow (~10m); skipped in -short.
func TestStalenessEviction(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	// -1 = scrape failed (transient port-forward drop); treated as
	// "unknown", never as "evicted".
	hasSample := func() int {
		fams, err := scrapeErr()
		if err != nil {
			t.Logf("transient scrape error (retrying): %v", err)
			return -1
		}
		n := 0
		if f, ok := fams["kubernetes_io:container_uptime"]; ok {
			for _, m := range f.Metric {
				if strings.HasPrefix(labelValue(m, "pod_name"), "sample-workload-") {
					n++
				}
			}
		}
		return n
	}

	// Presence first (may need to wait out ingest delay on a fresh env).
	waitFor(t, "sample workload series present", 6*time.Minute, func() bool { return hasSample() > 0 })

	if err := exec.Command("kubectl", "-n", "default", "scale", "deployment/sample-workload", "--replicas=0").Run(); err != nil {
		t.Fatalf("scale down: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("kubectl", "-n", "default", "scale", "deployment/sample-workload", "--replicas=2").Run()
	})

	// Budget: points keep appearing within the lookback window (~4m for
	// these metrics), then staleAfter (5m) + sweep cadence (2.5m).
	waitFor(t, "sample workload series evicted", 14*time.Minute, func() bool { return hasSample() == 0 })
	t.Log("evicted after workload deletion")
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Second)
	}
	t.Fatalf("timed out (%s) waiting for %s", timeout, what)
}

func pointVal(p *monitoringpb.Point) float64 {
	switch v := p.GetValue().GetValue().(type) {
	case *monitoringpb.TypedValue_DoubleValue:
		return v.DoubleValue
	case *monitoringpb.TypedValue_Int64Value:
		return float64(v.Int64Value)
	default:
		return math.NaN()
	}
}
