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

import (
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var now = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)

func descriptor(typ string, kind metric.MetricDescriptor_MetricKind, vt metric.MetricDescriptor_ValueType) *metric.MetricDescriptor {
	return &metric.MetricDescriptor{Type: typ, MetricKind: kind, ValueType: vt, Description: "help text"}
}

func series(resType string, resLabels, metLabels map[string]string, points ...*monitoringpb.Point) *monitoringpb.TimeSeries {
	return &monitoringpb.TimeSeries{
		Resource: &monitoredres.MonitoredResource{Type: resType, Labels: resLabels},
		Metric:   &metric.Metric{Labels: metLabels},
		Points:   points, // most recent first, per ListTimeSeries contract
	}
}

func dpoint(end time.Time, v float64) *monitoringpb.Point {
	return &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: v}},
	}
}

func ipoint(end time.Time, v int64) *monitoringpb.Point {
	return &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{Int64Value: v}},
	}
}

func TestGaugeLatestPointAndLabels(t *testing.T) {
	md := descriptor("kubernetes.io/node/cpu/allocatable_utilization", metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE)
	ts := series("k8s_node",
		map[string]string{"project_id": "p", "cluster_name": "c", "node_name": "n1"},
		map[string]string{"cluster_name": "metric-side"}, // collides with resource label
		dpoint(now, 0.75), dpoint(now.Add(-time.Minute), 0.70))

	fam, st := New().Translate(md, []*monitoringpb.TimeSeries{ts}, "", now)
	if st != (Stats{}) {
		t.Fatalf("unexpected stats: %+v", st)
	}
	if fam.Name != "kubernetes_io:node_cpu_allocatable_utilization" || fam.Kind != KindGauge || fam.Help != "help text" {
		t.Fatalf("family = %+v", fam)
	}
	s := fam.Samples[0]
	if s.Value != 0.75 || !s.PointEnd.Equal(now) {
		t.Errorf("latest point not used: %+v", s)
	}
	if got := s.Labels.Get(MonitoredResourceLabel); got != "k8s_node" {
		t.Errorf("monitored_resource = %q", got)
	}
	if got := s.Labels.Get("cluster_name"); got != "c" {
		t.Errorf("resource label lost the plain name: %q", got)
	}
	if got := s.Labels.Get("metric_cluster_name"); got != "metric-side" {
		t.Errorf("colliding metric label = %q, want metric_ prefix", got)
	}
}

func TestCumulativeAndBool(t *testing.T) {
	md := descriptor("x.googleapis.com/restarts", metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_INT64)
	fam, _ := New().Translate(md, []*monitoringpb.TimeSeries{
		series("gce_instance", nil, nil, ipoint(now, 42)),
	}, "", now)
	if fam.Kind != KindCounter || fam.Samples[0].Value != 42 {
		t.Errorf("cumulative: %+v", fam)
	}

	mdb := descriptor("x.googleapis.com/up", metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_BOOL)
	tsb := series("gce_instance", nil, nil, &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(now)},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_BoolValue{BoolValue: true}},
	})
	fam, _ = New().Translate(mdb, []*monitoringpb.TimeSeries{tsb}, "", now)
	if fam.Samples[0].Value != 1 {
		t.Errorf("bool true != 1: %+v", fam.Samples[0])
	}
}

func TestStringDroppedAndRename(t *testing.T) {
	md := descriptor("x.googleapis.com/version", metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_STRING)
	ts := series("gce_instance", nil, nil, &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(now)},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_StringValue{StringValue: "v1"}},
	})
	fam, st := New().Translate(md, []*monitoringpb.TimeSeries{ts}, "custom_name", now)
	if len(fam.Samples) != 0 || st.DroppedValueType != 1 {
		t.Errorf("string not dropped: %+v %+v", fam, st)
	}
	if fam.Name != "custom_name" {
		t.Errorf("rename not applied: %q", fam.Name)
	}
}

func TestDeltaAccumulationAcrossPolls(t *testing.T) {
	md := descriptor("x.googleapis.com/events", metric.MetricDescriptor_DELTA, metric.MetricDescriptor_INT64)
	tr := New()
	lbls := map[string]string{"project_id": "p"}

	// Poll 1: two delta points (10 then 5, reverse order in response).
	fam, _ := tr.Translate(md, []*monitoringpb.TimeSeries{
		series("gce_instance", lbls, nil, ipoint(now, 5), ipoint(now.Add(-time.Minute), 10)),
	}, "", now)
	if fam.Kind != KindCounter || fam.Samples[0].Value != 15 {
		t.Fatalf("poll1 accumulated = %+v, want 15", fam.Samples[0])
	}

	// Poll 2: overlapping window resends the 5, adds a 7.
	fam, _ = tr.Translate(md, []*monitoringpb.TimeSeries{
		series("gce_instance", lbls, nil, ipoint(now.Add(time.Minute), 7), ipoint(now, 5)),
	}, "", now.Add(time.Minute))
	if fam.Samples[0].Value != 22 {
		t.Fatalf("poll2 accumulated = %v, want 22 (no double count)", fam.Samples[0].Value)
	}
	if !fam.Samples[0].PointEnd.Equal(now.Add(time.Minute)) {
		t.Errorf("PointEnd = %v", fam.Samples[0].PointEnd)
	}

	// Different label set = independent accumulator.
	fam, _ = tr.Translate(md, []*monitoringpb.TimeSeries{
		series("gce_instance", map[string]string{"project_id": "other"}, nil, ipoint(now.Add(time.Minute), 3)),
	}, "", now.Add(time.Minute))
	if fam.Samples[0].Value != 3 {
		t.Errorf("independent series polluted: %v", fam.Samples[0].Value)
	}
}

func TestSweepStaleResetsDeltaState(t *testing.T) {
	md := descriptor("x.googleapis.com/events", metric.MetricDescriptor_DELTA, metric.MetricDescriptor_INT64)
	tr := New()
	tr.Translate(md, []*monitoringpb.TimeSeries{series("gce_instance", nil, nil, ipoint(now, 9))}, "", now)

	if removed := tr.SweepStale(now.Add(-time.Hour)); removed != 0 {
		t.Fatalf("sweep removed fresh state: %d", removed)
	}
	if removed := tr.SweepStale(now.Add(time.Hour)); removed != 1 {
		t.Fatalf("sweep kept stale state: %d", removed)
	}
	// After eviction the counter restarts from zero — Prometheus counter
	// semantics absorb this as a reset.
	fam, _ := tr.Translate(md, []*monitoringpb.TimeSeries{
		series("gce_instance", nil, nil, ipoint(now.Add(2*time.Hour), 4)),
	}, "", now.Add(2*time.Hour))
	if fam.Samples[0].Value != 4 {
		t.Errorf("state not reset after sweep: %v", fam.Samples[0].Value)
	}
}

func explicitDist(count int64, mean float64, bounds []float64, bucketCounts []int64) *monitoringpb.TypedValue {
	return &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DistributionValue{
		DistributionValue: &distribution.Distribution{
			Count: count, Mean: mean,
			BucketOptions: &distribution.Distribution_BucketOptions{
				Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
					ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{Bounds: bounds},
				},
			},
			BucketCounts: bucketCounts,
		},
	}}
}

func TestDistributionToHistogram(t *testing.T) {
	md := descriptor("x.googleapis.com/latency", metric.MetricDescriptor_CUMULATIVE, metric.MetricDescriptor_DISTRIBUTION)
	// Buckets: (-inf,1)=2, [1,5)=3, [5,+inf)=4; count 9, mean 3 → sum 27.
	// Trailing overflow bucket supplied; also test trimming by omitting it below.
	p := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(now)},
		Value:    explicitDist(9, 3, []float64{1, 5}, []int64{2, 3, 4}),
	}
	fam, st := New().Translate(md, []*monitoringpb.TimeSeries{series("gce_instance", nil, nil, p)}, "", now)
	if st != (Stats{}) || fam.Kind != KindHistogram {
		t.Fatalf("stats %+v kind %v", st, fam.Kind)
	}
	h := fam.Samples[0].Hist
	if h.Count != 9 || h.Sum != 27 {
		t.Errorf("count/sum = %d/%v", h.Count, h.Sum)
	}
	wantBounds := []float64{1, 5}
	wantCum := []uint64{2, 5} // le=1 → 2; le=5 → 5; +Inf → 9 (Count)
	for i := range wantBounds {
		if h.UpperBounds[i] != wantBounds[i] || h.CumCounts[i] != wantCum[i] {
			t.Errorf("bucket %d: le=%v cum=%d, want le=%v cum=%d", i, h.UpperBounds[i], h.CumCounts[i], wantBounds[i], wantCum[i])
		}
	}

	// GCM trims trailing zero buckets: only underflow bucket present.
	p2 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(now)},
		Value:    explicitDist(2, 0.5, []float64{1, 5}, []int64{2}),
	}
	fam, _ = New().Translate(md, []*monitoringpb.TimeSeries{series("gce_instance", nil, nil, p2)}, "", now)
	h = fam.Samples[0].Hist
	if h.CumCounts[0] != 2 || h.CumCounts[1] != 2 || h.Count != 2 {
		t.Errorf("padded histogram wrong: %+v", h)
	}
}

func TestLinearAndExponentialBounds(t *testing.T) {
	lin := &distribution.Distribution_BucketOptions{Options: &distribution.Distribution_BucketOptions_LinearBuckets{
		LinearBuckets: &distribution.Distribution_BucketOptions_Linear{NumFiniteBuckets: 3, Width: 10, Offset: 5},
	}}
	if got := distBounds(lin); len(got) != 4 || got[0] != 5 || got[3] != 35 {
		t.Errorf("linear bounds = %v", got)
	}
	exp := &distribution.Distribution_BucketOptions{Options: &distribution.Distribution_BucketOptions_ExponentialBuckets{
		ExponentialBuckets: &distribution.Distribution_BucketOptions_Exponential{NumFiniteBuckets: 3, GrowthFactor: 2, Scale: 1},
	}}
	if got := distBounds(exp); len(got) != 4 || got[0] != 1 || got[3] != 8 {
		t.Errorf("exponential bounds = %v", got)
	}
}

func TestDeltaDistributionAccumulation(t *testing.T) {
	md := descriptor("x.googleapis.com/dcn_latency", metric.MetricDescriptor_DELTA, metric.MetricDescriptor_DISTRIBUTION)
	tr := New()
	mk := func(end time.Time, count int64, mean float64, counts []int64) *monitoringpb.Point {
		return &monitoringpb.Point{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
			Value:    explicitDist(count, mean, []float64{1, 5}, counts),
		}
	}
	fam, _ := tr.Translate(md, []*monitoringpb.TimeSeries{
		series("k8s_container", nil, nil, mk(now, 4, 2, []int64{1, 2, 1})),
	}, "", now)
	h := fam.Samples[0].Hist
	if h.Count != 4 || h.Sum != 8 || h.CumCounts[1] != 3 {
		t.Fatalf("poll1 hist: %+v", h)
	}

	// Poll 2: one new point; overlapping old point must not double-count.
	fam, _ = tr.Translate(md, []*monitoringpb.TimeSeries{
		series("k8s_container", nil, nil, mk(now.Add(time.Minute), 2, 5, []int64{0, 1, 1}), mk(now, 4, 2, []int64{1, 2, 1})),
	}, "", now.Add(time.Minute))
	h = fam.Samples[0].Hist
	if h.Count != 6 || h.Sum != 18 {
		t.Errorf("poll2 count/sum = %d/%v, want 6/18", h.Count, h.Sum)
	}
	if h.CumCounts[0] != 1 || h.CumCounts[1] != 4 {
		t.Errorf("poll2 cum = %v", h.CumCounts)
	}

	// Bucket layout change resets accumulation and is counted.
	p3 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(now.Add(2 * time.Minute))},
		Value:    explicitDist(1, 1, []float64{2, 10}, []int64{1}),
	}
	fam, st := tr.Translate(md, []*monitoringpb.TimeSeries{series("k8s_container", nil, nil, p3)}, "", now.Add(2*time.Minute))
	if st.BucketReset != 1 {
		t.Errorf("BucketReset = %d, want 1", st.BucketReset)
	}
	if h = fam.Samples[0].Hist; h.Count != 1 || h.UpperBounds[0] != 2 {
		t.Errorf("post-reset hist: %+v", h)
	}
}

func TestNoPointsCounted(t *testing.T) {
	md := descriptor("x.googleapis.com/empty", metric.MetricDescriptor_GAUGE, metric.MetricDescriptor_DOUBLE)
	fam, st := New().Translate(md, []*monitoringpb.TimeSeries{series("gce_instance", nil, nil)}, "", now)
	if len(fam.Samples) != 0 || st.NoPoints != 1 {
		t.Errorf("empty series not handled: %+v %+v", fam, st)
	}
}
