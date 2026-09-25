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

package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

func TestGCMHooks(t *testing.T) {
	m := New("test")
	h := m.GCMHooks()
	h.OnAPICall("ListTimeSeries", codes.OK)
	h.OnAPICall("ListTimeSeries", codes.ResourceExhausted)
	h.OnSeriesReturned("kubernetes.io/node/x", 41)
	h.OnSeriesReturned("kubernetes.io/node/x", 0) // billed min 1

	if got := testutil.ToFloat64(m.apiRequests.WithLabelValues("ListTimeSeries", "OK")); got != 1 {
		t.Errorf("api_requests OK = %v", got)
	}
	if got := testutil.ToFloat64(m.apiRequests.WithLabelValues("ListTimeSeries", "ResourceExhausted")); got != 1 {
		t.Errorf("api_requests exhausted = %v", got)
	}
	if got := testutil.ToFloat64(m.billedSeries.WithLabelValues("kubernetes.io/node/x")); got != 42 {
		t.Errorf("billed_series = %v, want 42 (41 + min-1)", got)
	}
}

func TestPollHooks(t *testing.T) {
	m := New("test")
	h := m.PollHooks()
	h.OnPoll("mt", 250*time.Millisecond, 10,
		translate.Stats{DroppedValueType: 2, NoPoints: 3, BucketReset: 1},
		cache.ApplyStats{DroppedLimit: 4})
	h.OnPollError("mt", "quota")
	h.OnSweep(5, 1, 2)

	for _, tc := range []struct {
		vec  string
		got  float64
		want float64
	}{
		{"dropped value_type", testutil.ToFloat64(m.seriesDropped.WithLabelValues("value_type")), 2},
		{"dropped no_points", testutil.ToFloat64(m.seriesDropped.WithLabelValues("no_points")), 3},
		{"dropped limit", testutil.ToFloat64(m.seriesDropped.WithLabelValues("limit")), 4},
		{"bucket resets", testutil.ToFloat64(m.bucketResets.WithLabelValues("mt")), 1},
		{"poll errors", testutil.ToFloat64(m.pollErrors.WithLabelValues("mt", "quota")), 1},
		{"sweep series", testutil.ToFloat64(m.sweepEvicted.WithLabelValues("series")), 5},
		{"sweep delta", testutil.ToFloat64(m.sweepEvicted.WithLabelValues("delta_states")), 2},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.vec, tc.got, tc.want)
		}
	}
	if got := testutil.CollectAndCount(m.pollDuration); got != 1 {
		t.Errorf("poll_duration families = %d", got)
	}
}

func TestCacheCollector(t *testing.T) {
	t0 := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore()
	store.Apply("kubernetes.io/node/x", translate.FamilyUpdate{
		Name: "kubernetes_io:node_x", Kind: translate.KindGauge,
		Samples: []translate.Sample{
			{Labels: labels.FromStrings("a", "1"), Value: 1, PointEnd: t0.Add(-90 * time.Second)},
			{Labels: labels.FromStrings("a", "2"), Value: 2, PointEnd: t0.Add(-30 * time.Second)},
		},
	}, 10, t0)

	c := &cacheCollector{store: store, now: func() time.Time { return t0 }}
	want := `# HELP gcmproxy_data_age_seconds Age of the newest cached point per metric type — the true staleness of served data (GCM ingestion delay + poll lag).
# TYPE gcmproxy_data_age_seconds gauge
gcmproxy_data_age_seconds{metric_type="kubernetes.io/node/x"} 30
# HELP gcmproxy_series_cached Series currently cached per metric type.
# TYPE gcmproxy_series_cached gauge
gcmproxy_series_cached{metric_type="kubernetes.io/node/x"} 2
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

func TestConfigCollector(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte("metrics:\n  - type: a/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := config.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	c := ConfigCollector(s)
	want := `# HELP gcmproxy_config_load_errors_total Config reloads that failed (previous config kept).
# TYPE gcmproxy_config_load_errors_total counter
gcmproxy_config_load_errors_total 0
# HELP gcmproxy_config_stale 1 when the most recent config reload failed.
# TYPE gcmproxy_config_stale gauge
gcmproxy_config_stale 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

func TestBuildInfoAndRuntimeCollectors(t *testing.T) {
	m := New("v0.1.0-test")
	fams, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range fams {
		found[f.GetName()] = true
	}
	for _, want := range []string{"gcmproxy_build_info", "go_goroutines", "process_start_time_seconds"} {
		if !found[want] {
			t.Errorf("registry missing %s", want)
		}
	}
}
