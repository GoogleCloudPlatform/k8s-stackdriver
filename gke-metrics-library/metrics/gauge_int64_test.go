package metrics

import (
	"sync/atomic"
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestGaugeInt64SetWithoutLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
	}
	testcases := []struct {
		desc           string
		valueToSet     int64
		wantTimeSeries *mrpb.TimeSeries
	}{
		{
			desc:       "OK: set value in a metric",
			valueToSet: 30,
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: metricOptions.Name,
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(end),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 30},
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

			got := NewGaugeInt64(metricOptions)
			got.SetWithoutLabels(tc.valueToSet)
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff([]*mrpb.TimeSeries{tc.wantTimeSeries}, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeInt64SetWithLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}
	testcases := []struct {
		desc                 string
		labelAndMetricValues []labelAndInt64MetricValue
		wantTimeSeries       []*mrpb.TimeSeries
	}{
		{
			desc: "Set value in a metric with a nil label returns no error",
			labelAndMetricValues: []labelAndInt64MetricValue{
				{value: 1, label: nil},
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				&mrpb.TimeSeries{
					Metric: &metricpb.Metric{
						Type: metricOptions.Name,
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
						},
					}},
				},
			},
		},
		{
			desc: "OK: set value in a metric with multiple labels",
			labelAndMetricValues: []labelAndInt64MetricValue{
				{label: map[string]string{"label": "value"}, value: 1},
				{label: map[string]string{"label": "value 2"}, value: 20},
				{label: map[string]string{"label2": "value"}, value: 300},
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
						},
					}},
				},
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value 2"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 20},
						},
					}},
				},
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label2": "value"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 300},
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

			got := NewGaugeInt64(metricOptions)
			for _, labelAndMetricValue := range tc.labelAndMetricValues {
				got.WithLabels(labelAndMetricValue.label).Set(labelAndMetricValue.value)
			}
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff(tc.wantTimeSeries, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeInt64LabelsAreImmutable(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}
	testcases := []struct {
		desc           string
		withLabels     labelAndInt64MetricValue
		wantTimeSeries *mrpb.TimeSeries
	}{
		{
			desc:       "Modifying non-empty labels after WithLabels is called does not change exported timeSeries",
			withLabels: labelAndInt64MetricValue{map[string]string{"label": "value"}, 1},
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   metricOptions.Name,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(end),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
					},
				}},
			},
		},
		{
			desc:       "Modifying empty labels after WithLabels is called does not change exported timeSeries",
			withLabels: labelAndInt64MetricValue{map[string]string{}, 1},
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   metricOptions.Name,
					Labels: map[string]string{},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(end),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
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

			got := NewGaugeInt64(metricOptions)
			got.WithLabels(tc.withLabels.label).Set(tc.withLabels.value)

			// Now, we change the labels map after we passed it to WithLabels to ensure that the
			// metric will still be exported with the correct labels and values.
			tc.withLabels.label["label"] = "a different label value."

			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff([]*mrpb.TimeSeries{tc.wantTimeSeries}, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeInt64Reset(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        testMetricName,
		Description: "description",
	}
	testcases := []struct {
		desc           string
		existingMetric func() *GaugeInt64
	}{
		{
			desc: "Reset a metric with existing streams only keeps its options",
			existingMetric: func() *GaugeInt64 {
				metric := NewGaugeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1)
				metric.WithLabels(map[string]string{"label 2": "value 2"}).Set(2)
				return metric
			},
		},
		{
			desc: "Reset a metric with no streams is a no-op",
			existingMetric: func() *GaugeInt64 {
				return NewGaugeInt64(metricOptions)
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
				cmp.AllowUnexported(GaugeInt64{}, gaugeInt64{}, atomic.Int64{}, Resource{}),
				cmpopts.IgnoreFields(GaugeInt64{}, "mu"),
			}
			wantMetric := NewGaugeInt64(metricOptions)
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
			existingMetric.SetWithoutLabels(100)
			wantTimeSeries := []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 100},
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

func TestGaugeInt64Unset(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        testMetricName,
		Description: "description",
	}
	testcases := []struct {
		desc   string
		metric func() *GaugeInt64
		want   []*mrpb.TimeSeries
	}{
		{
			desc: "Unsetting a metric works",
			metric: func() *GaugeInt64 {
				metric := NewGaugeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1)
				metric.WithLabels(map[string]string{"label 2": "value 2"}).Set(2)
				metric.Unset(map[string]string{"label": "value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label 2": "value 2"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 2},
					},
				}},
			}},
		},
		{
			desc: "Unsetting a non-existent stream does nothing",
			metric: func() *GaugeInt64 {
				metric := NewGaugeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1)
				metric.Unset(map[string]string{"label": "not the actual value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
					},
				}},
			}},
		},
		{
			desc: "Unsetting and re-setting a stream works",
			metric: func() *GaugeInt64 {
				metric := NewGaugeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Set(1)
				metric.Unset(map[string]string{"label": "value"})
				metric.WithLabels(map[string]string{"label": "value"}).Set(2)
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						EndTime: timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 2},
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

type labelAndInt64MetricValue struct {
	label map[string]string
	value int64
}

func TestGaugeInt64AddWithoutLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
	}
	testcases := []struct {
		desc           string
		valuesToAdd    []int64
		wantTimeSeries *mrpb.TimeSeries
	}{
		{
			desc:        "OK: add values to a metric",
			valuesToAdd: []int64{30, 10, 40},
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: metricOptions.Name,
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 80},
						},
					},
				},
			},
		},
		{
			desc:        "OK: add values to a metric (including negative values)",
			valuesToAdd: []int64{30, -10, -40},
			wantTimeSeries: &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: metricOptions.Name,
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       metricOptions.Unit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							EndTime: timestamppb.New(end),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: -20},
						},
					},
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

			got := NewGaugeInt64(metricOptions)
			for _, v := range tc.valuesToAdd {
				got.AddWithoutLabels(v)
			}
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff([]*mrpb.TimeSeries{tc.wantTimeSeries}, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

type labelsAndValues struct {
	label  map[string]string
	values []int64
}

func TestGaugeInt64AddWithLabels(t *testing.T) {
	end := time.Now()
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
	}
	testcases := []struct {
		desc                 string
		LabelsAndValuesToAdd []labelsAndValues
		wantTimeSeries       []*mrpb.TimeSeries
	}{
		{
			desc: "OK: add values to a metric",
			LabelsAndValuesToAdd: []labelsAndValues{
				{
					label:  map[string]string{"label": "value-1"},
					values: []int64{30, 10, 40},
				}, {
					label:  map[string]string{"label": "value-2"},
					values: []int64{30, 10, 30},
				}, {
					label:  map[string]string{"label": "value-3"},
					values: []int64{30, -10, -40},
				},
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value-1"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								EndTime: timestamppb.New(end),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_Int64Value{Int64Value: 80},
							},
						},
					},
				},
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value-2"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								EndTime: timestamppb.New(end),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_Int64Value{Int64Value: 70},
							},
						},
					},
				},
				{
					Metric: &metricpb.Metric{
						Type:   metricOptions.Name,
						Labels: map[string]string{"label": "value-3"},
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: metricpb.MetricDescriptor_GAUGE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       metricOptions.Unit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								EndTime: timestamppb.New(end),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_Int64Value{Int64Value: -20},
							},
						},
					},
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

			got := NewGaugeInt64(metricOptions)
			for _, labelsAndValues := range tc.LabelsAndValuesToAdd {
				for _, v := range labelsAndValues.values {
					got.WithLabels(labelsAndValues.label).Add(v)
				}
			}
			got.ExportTimeSeries(testContainerResource, exportFn)

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff(tc.wantTimeSeries, gotTimeSeries, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGaugeInt64ExportTimeSeriesDoesNotBlockWithLabels(t *testing.T) {
	metricOptions := DescriptorOpts{
		Name:        "metric",
		Description: "description",
		Unit:        "unit",
		Labels:      []LabelDescriptor{{Name: "label", Description: "description"}},
	}
	metric := NewGaugeInt64(metricOptions)
	exportStarted := make(chan struct{})
	withLabelsDone := make(chan struct{})
	metric.WithLabels(map[string]string{"label": "value"})

	go metric.ExportTimeSeries(testContainerResource, func(ts *mrpb.TimeSeries) {
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
