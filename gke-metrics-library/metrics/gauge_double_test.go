package metrics

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	mpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestGaugeDoubleSetWithoutLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
	}
	testcases := []struct {
		desc           string
		valueToSet     float64
		wantTimeSeries *mrpb.TimeSeries
	}{
		{
			desc:       "OK: set value in a metric",
			valueToSet: 30.456,
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &mpb.Metric{
					Type: metricOptions.Name,
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: mpb.MetricDescriptor_GAUGE,
				ValueType:  mpb.MetricDescriptor_DOUBLE,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(end),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 30.456},
					},
				}},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			originalNow := now
			now = func() time.Time { return end }
			defer func() { now = originalNow }()

			gotTimeSeries := []*mrpb.TimeSeries{}
			exportFn := func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, ts)
			}

			got := NewGaugeDouble(metricOptions)
			got.SetWithoutLabels(tc.valueToSet)
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
				cmp.Comparer(func(x, y float64) bool {
					return math.Abs(x-y) < 0.001
				}),
			}
			if diff := cmp.Diff([]*mrpb.TimeSeries{tc.wantTimeSeries}, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeDoubleSetWithLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}
	testcases := []struct {
		desc                 string
		labelAndMetricValues []labelAndDoubleMetricValue
		wantTimeSeries       []*mrpb.TimeSeries
	}{
		{
			desc: "Set value in a metric with a nil label returns no error",
			labelAndMetricValues: []labelAndDoubleMetricValue{
				{value: 1.23456, label: nil},
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				&mrpb.TimeSeries{
					Metric: &mpb.Metric{
						Type: metricOptions.Name,
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_GAUGE,
					ValueType:  mpb.MetricDescriptor_DOUBLE,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1.23456},
						},
					}},
				},
			},
		},
		{
			desc: "OK: set value in a metric with multiple labels",
			labelAndMetricValues: []labelAndDoubleMetricValue{
				{label: map[string]string{"label": "value"}, value: 1.234},
				{label: map[string]string{"label": "value 2"}, value: 20.345},
				{label: map[string]string{"label2": "value"}, value: 300.456},
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &mpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_GAUGE,
					ValueType:  mpb.MetricDescriptor_DOUBLE,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1.234},
						},
					}},
				},
				{
					Metric: &mpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value 2"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_GAUGE,
					ValueType:  mpb.MetricDescriptor_DOUBLE,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 20.345},
						},
					}},
				},
				{
					Metric: &mpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label2": "value"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_GAUGE,
					ValueType:  mpb.MetricDescriptor_DOUBLE,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 300.456},
						},
					}},
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			originalNow := now
			now = func() time.Time { return end }
			defer func() { now = originalNow }()

			gotTimeSeries := []*mrpb.TimeSeries{}
			exportFn := func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, ts)
			}

			got := NewGaugeDouble(metricOptions)
			for _, labelAndMetricValue := range tc.labelAndMetricValues {
				got.WithLabels(labelAndMetricValue.label).Set(labelAndMetricValue.value)
			}
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
				cmp.Comparer(func(x, y float64) bool {
					return math.Abs(x-y) < 0.001
				}),
			}
			if diff := cmp.Diff(tc.wantTimeSeries, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeDoubleLabelsAreImmutable(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}

	labelAndMetricValue := labelAndDoubleMetricValue{map[string]string{"label": "value"}, 1.234}
	wantTimeSeries := &mrpb.TimeSeries{
		Metric: &mpb.Metric{
			Type:   metricOptions.Name,
			Labels: map[string]string{"label": "value"},
		},
		Resource:   testMonitoredContainerResource,
		MetricKind: mpb.MetricDescriptor_GAUGE,
		ValueType:  mpb.MetricDescriptor_DOUBLE,
		Unit:       metricOptions.Unit,
		Points: []*mrpb.Point{{
			Interval: &cpb.TimeInterval{
				EndTime: timestamppb.New(end),
			},
			Value: &cpb.TypedValue{
				Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1.234},
			},
		}},
	}

	originalNow := now
	now = func() time.Time { return end }
	defer func() { now = originalNow }()

	gotTimeSeries := []*mrpb.TimeSeries{}
	exportFn := func(ts *mrpb.TimeSeries) {
		gotTimeSeries = append(gotTimeSeries, ts)
	}

	got := NewGaugeDouble(metricOptions)
	got.WithLabels(labelAndMetricValue.label).Set(labelAndMetricValue.value)

	// Now, we change the labels map after we passed it to WithLabels to ensure that the
	// metric will still be exported with the correct labels and values.
	labelAndMetricValue.label["label"] = "a different label value."

	got.ExportTimeSeries(testContainerResource, exportFn)

	opts := []cmp.Option{
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.Transform(),
		cmp.Comparer(func(x, y float64) bool {
			return math.Abs(x-y) < 0.001
		}),
	}
	if diff := cmp.Diff([]*mrpb.TimeSeries{wantTimeSeries}, gotTimeSeries, opts...); diff != "" {
		t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
	}
}

func TestGaugeDoubleReset(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        testMetricName,
		Description: "description",
	}
	testcases := []struct {
		desc           string
		existingMetric func() *GaugeDouble
	}{
		{
			desc: "Reset a metric with existing streams only keeps its options",
			existingMetric: func() *GaugeDouble {
				metric := NewGaugeDouble(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1.0)
				metric.WithLabels(map[string]string{"label 2": "value 2"}).Set(2.0)
				return metric
			},
		},
		{
			desc: "Reset a metric with no streams is a no-op",
			existingMetric: func() *GaugeDouble {
				return NewGaugeDouble(metricOptions)
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Always reset the time for each test case.
			originalNow := now
			t.Cleanup(func() {
				now = originalNow
			})

			existingMetric := tc.existingMetric()
			existingMetric.Reset()
			diffOpts := []cmp.Option{
				cmp.AllowUnexported(GaugeDouble{}, gaugeDouble{}, atomic.Int64{}, Resource{}),
				cmpopts.IgnoreFields(GaugeDouble{}, "mu"),
			}
			wantMetric := NewGaugeDouble(metricOptions)
			if diff := cmp.Diff(wantMetric, existingMetric, diffOpts...); diff != "" {
				t.Errorf("Reset() returned diff (-want, +got):\n%s", diff)
			}

			// Ensure that no timeseries would be exported after a metric reset.
			gotTimeSeries := []*mrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
			})
			if tsCount := len(gotTimeSeries); tsCount > 0 {
				t.Errorf("ExportTimeSeries() returned %v timeseries after a metric was reset, want no timeseries\n", gotTimeSeries)
			}

			// Ensure that a call to Add...() after a metric reset creates a single timeseries with a
			// reset start time.
			existingMetric.SetWithoutLabels(100.0)
			wantTimeSeries := []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_GAUGE,
				ValueType:  mpb.MetricDescriptor_DOUBLE,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 100.0},
					},
				}},
			}}
			// Mock the end time, so we have a deterministic output.
			now = func() time.Time { return fakeEndTime }
			gotTimeSeries = []*mrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
			})
			if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, protocmp.Transform()); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeDoubleUnset(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        testMetricName,
		Description: "description",
	}
	testcases := []struct {
		desc   string
		metric func() *GaugeDouble
		want   []*mrpb.TimeSeries
	}{
		{
			desc: "Unsetting a metric works",
			metric: func() *GaugeDouble {
				metric := NewGaugeDouble(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1.0)
				metric.WithLabels(map[string]string{"label 2": "value 2"}).Set(2.0)
				metric.Unset(map[string]string{"label": "value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label 2": "value 2"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_GAUGE,
				ValueType:  mpb.MetricDescriptor_DOUBLE,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 2.0},
					},
				}},
			}},
		},
		{
			desc: "Unsetting a non-existent stream does nothing",
			metric: func() *GaugeDouble {
				metric := NewGaugeDouble(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1.0)
				metric.Unset(map[string]string{"label": "not the actual value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_GAUGE,
				ValueType:  mpb.MetricDescriptor_DOUBLE,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1.0},
					},
				}},
			}},
		},
		{
			desc: "Unsetting and re-setting a stream works",
			metric: func() *GaugeDouble {
				metric := NewGaugeDouble(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1.0)
				metric.Unset(map[string]string{"label": "value"})
				metric.WithLabels(map[string]string{"label": "value"}).Set(2.0)
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_GAUGE,
				ValueType:  mpb.MetricDescriptor_DOUBLE,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 2.0},
					},
				}},
			}},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Always reset the time for each test case.
			originalNow := now
			t.Cleanup(func() {
				now = originalNow
			})

			metric := tc.metric()
			// Mock the end time, so we have a deterministic output.
			now = func() time.Time { return fakeEndTime }

			check := func() {
				gotTimeSeries := []*mrpb.TimeSeries{}
				metric.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
					gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
				})
				if diff := cmp.Diff(tc.want, gotTimeSeries, protocmp.Transform()); diff != "" {
					t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
				}
			}
			check()
			// Verify that Compact does not change the output.
			metric.Compact()
			check()
		})
	}
}

func TestGaugeDoubleExportTimeSeriesDoesNotBlockWithLabels(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}
	metric := NewGaugeDouble(metricOptions)
	exportStarted := make(chan struct{})
	withLabelsDone := make(chan struct{})
	metric.WithLabels(map[string]string{"label": "value"})

	go metric.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
		close(exportStarted)
		<-withLabelsDone
	})

	<-exportStarted
	go func() {
		// We use a different label to ensure that writer lock is necessary.
		metric.WithLabels(map[string]string{"new-label": "value"})
		close(withLabelsDone)
	}()

	select {
	case <-withLabelsDone:
	case <-time.After(time.Second):
		t.Errorf("WithLabels() did blocked with labels")
	}
}

type labelAndDoubleMetricValue struct {
	label map[string]string
	value float64
}
