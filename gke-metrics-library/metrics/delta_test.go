package metrics

import (
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	deltaOpts = DescriptorOpts{
		Name:   testMetricName,
		Unit:   "count",
		Labels: []LabelDescriptor{{Name: "key"}, {Name: "key2"}},
	}
	deltaLabel1 = map[string]string{"label": "value"}
	deltaLabel2 = map[string]string{"label2": "value2"}
	deltaLabel3 = map[string]string{"label": "value3", "label2": "value3"}
)

func TestNewDeltaInt64(t *testing.T) {
	testcases := []struct {
		desc      string
		wantDelta *DeltaInt64
	}{
		{
			desc: "new Delta metric",
			wantDelta: &DeltaInt64{
				opts:  deltaOpts,
				value: map[uint64]*deltaInt64{},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaInt64(deltaOpts)
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, protocmp.Transform()); diff != "" {
				t.Errorf("Create new DeltaInt64 returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaInt64AddWithLabels(t *testing.T) {
	type metricData struct {
		valueToAdd int64
		labels     map[string]string
		startTime  time.Time
	}
	FakeStartTime1 := time.Now()
	FakeStartTime2 := FakeStartTime1.Add(time.Second)
	FakeStartTime3 := FakeStartTime2.Add(time.Second)
	testcases := []struct {
		desc      string
		inputs    []metricData
		wantDelta *DeltaInt64
	}{
		{
			desc: "add value to a new Delta metric",
			inputs: []metricData{
				{
					valueToAdd: 1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
			},
			wantDelta: &DeltaInt64{
				opts: deltaOpts,
				value: map[uint64]*deltaInt64{
					hashLabels(deltaLabel1): newDeltaInt64(1, deltaLabel1, FakeStartTime1),
				},
			},
		},
		{
			desc: "add value to a Delta metric with existing label",
			inputs: []metricData{
				{
					valueToAdd: 1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2,
					labels:     deltaLabel1,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3,
					labels:     deltaLabel1,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaInt64{
				opts: deltaOpts,
				value: map[uint64]*deltaInt64{
					hashLabels(deltaLabel1): newDeltaInt64(3, deltaLabel1, FakeStartTime3),
				},
			},
		},
		{
			desc: "add value to a Delta metric with new labels",
			inputs: []metricData{
				{
					valueToAdd: 1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2,
					labels:     deltaLabel2,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3,
					labels:     deltaLabel3,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaInt64{
				opts: deltaOpts,
				value: map[uint64]*deltaInt64{
					hashLabels(deltaLabel1): newDeltaInt64(1, deltaLabel1, FakeStartTime1),
					hashLabels(deltaLabel2): newDeltaInt64(2, deltaLabel2, FakeStartTime2),
					hashLabels(deltaLabel3): newDeltaInt64(3, deltaLabel3, FakeStartTime3),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaInt64(deltaOpts)
			for _, input := range tc.inputs {
				// Mock the start time.
				now = func() time.Time { return input.startTime }
				gotDelta.AddWithLabels(input.valueToAdd, input.labels)
			}
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, cmp.AllowUnexported(DeltaInt64{}, deltaInt64{}),
				cmpopts.IgnoreFields(DeltaInt64{}, "mu")); diff != "" {
				t.Errorf("Add value to DeltaInt64 metric with labels returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaInt64AddWithoutLabels(t *testing.T) {
	type metricData struct {
		valueToAdd int64
		startTime  time.Time
	}
	FakeStartTime1 := time.Now()
	FakeStartTime2 := FakeStartTime1.Add(time.Second)
	FakeStartTime3 := FakeStartTime2.Add(time.Second)
	testcases := []struct {
		desc      string
		inputs    []metricData
		wantDelta *DeltaInt64
	}{
		{
			desc: "add value to a new Delta metric without labels",
			inputs: []metricData{
				{
					valueToAdd: 1,
					startTime:  FakeStartTime1,
				},
			},
			wantDelta: &DeltaInt64{
				opts: deltaOpts,
				value: map[uint64]*deltaInt64{
					hashLabels(nil): newDeltaInt64(1, nil, FakeStartTime1),
				},
			},
		},
		{
			desc: "update value to an existing Delta metric without label",
			inputs: []metricData{
				{
					valueToAdd: 1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaInt64{
				opts: deltaOpts,
				value: map[uint64]*deltaInt64{
					hashLabels(nil): newDeltaInt64(3, nil, FakeStartTime3),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaInt64(deltaOpts)
			for _, input := range tc.inputs {
				// Mock the start time.
				now = func() time.Time { return input.startTime }
				gotDelta.AddWithoutLabels(input.valueToAdd)
			}
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, cmp.AllowUnexported(DeltaInt64{}, deltaInt64{}),
				cmpopts.IgnoreFields(DeltaInt64{}, "mu")); diff != "" {
				t.Errorf("Add value to DeltaInt64 metric with labels returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaInt64ExportTimeSeries(t *testing.T) {
	type metricData struct {
		labels     map[string]string
		valueToAdd int64
	}

	testcases := []struct {
		desc           string
		inputs         []metricData
		wantTimeSeries map[string]*mgrpb.TimeSeries
	}{
		{
			desc:           "no inputs",
			inputs:         []metricData{},
			wantTimeSeries: map[string]*mgrpb.TimeSeries{},
		},
		{
			desc: "multiple inputs",
			inputs: []metricData{
				{
					// labels is nil, it is identical to label is map[string]string{}
					labels:     nil,
					valueToAdd: 1,
				},
				{
					labels:     map[string]string{},
					valueToAdd: 1,
				},
				{
					labels:     deltaLabel1,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel2,
					valueToAdd: 3,
				},
				{
					labels:     deltaLabel2,
					valueToAdd: 3,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 0,
				},
			},
			wantTimeSeries: map[string]*mgrpb.TimeSeries{
				"test-metric-name": &mgrpb.TimeSeries{
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: map[string]string{},
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
						},
					}},
				},
				"test-metric-name/label_value": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel1,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 4},
						},
					}},
				},
				"test-metric-name/label2_value2": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel2,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 3},
						},
					}},
				},
				"test-metric-name/label_value3/label2_value3": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel3,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 0},
						},
					}},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }
			gotDelta := NewDeltaInt64(deltaOpts)
			for _, input := range tc.inputs {
				if len(input.labels) != 0 {
					gotDelta.AddWithLabels(input.valueToAdd, input.labels)
				} else {
					gotDelta.AddWithoutLabels(input.valueToAdd)
				}
			}
			fakeExporter := &fakeExporter{got: make(map[string]*mgrpb.TimeSeries, len(gotDelta.value))}

			// Mock the end time.
			now = func() time.Time { return fakeEndTime }

			gotDelta.ExportTimeSeries(nodeMonitoringResource, fakeExporter.export)
			if diff := cmp.Diff(tc.wantTimeSeries, fakeExporter.got,
				protocmp.Transform()); diff != "" {
				t.Errorf("Exported Delta TimeSeries returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaInt64Reset(t *testing.T) {
	testcases := []struct {
		desc           string
		existingMetric func() *DeltaInt64
	}{
		{
			desc: "Reset a metric with existing streams only keeps its options",
			existingMetric: func() *DeltaInt64 {
				metric := NewDeltaInt64(deltaOpts)
				metric.AddWithoutLabels(0)
				metric.AddWithoutLabels(1)
				metric.AddWithoutLabels(2)
				return metric
			},
		},
		{
			desc: "Reset a metric with no streams only resets its start time",
			existingMetric: func() *DeltaInt64 {
				return NewDeltaInt64(deltaOpts)
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }
			existingMetric := tc.existingMetric()
			existingMetric.Reset()
			diffOpts := []cmp.Option{
				cmp.AllowUnexported(DeltaInt64{}, deltaInt64{}, Resource{}),
				cmpopts.IgnoreFields(DeltaInt64{}, "mu"),
			}
			wantMetric := NewDeltaInt64(deltaOpts)
			if diff := cmp.Diff(wantMetric, existingMetric, diffOpts...); diff != "" {
				t.Errorf("Reset() returned diff (-want, +got):\n%s", diff)
			}

			// Ensure that no TimeSeries would be exported after a metric reset.
			gotTimeSeries := []*mgrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mgrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mgrpb.TimeSeries))
			})
			if tsCount := len(gotTimeSeries); tsCount > 0 {
				t.Errorf("ExportTimeSeries() returned %v timeseries after a metric was reset, want no timeseries\n", gotTimeSeries)
			}

			// Ensure that a call to Add...() after a metric reset creates a single TimeSeries with a
			// reset start time.
			existingMetric.AddWithoutLabels(100)
			wantTimeSeries := []*mgrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_DELTA,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "count",
				Points: []*mgrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: timestamppb.New(fakeStartTime),
						EndTime:   timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 100},
					},
				}},
			}}
			// Mock the end time, so we have a deterministic output.
			now = func() time.Time { return fakeEndTime }
			gotTimeSeries = []*mgrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mgrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mgrpb.TimeSeries))
			})
			if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, protocmp.Transform()); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestNewDeltaDouble(t *testing.T) {
	testcases := []struct {
		desc      string
		wantDelta *DeltaDouble
	}{
		{
			desc: "new Delta metric",
			wantDelta: &DeltaDouble{
				opts:  deltaOpts,
				value: map[uint64]*deltaDouble{},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaDouble(deltaOpts)
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, protocmp.Transform()); diff != "" {
				t.Errorf("Create new DeltaDouble returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaDoubleAddWithLabels(t *testing.T) {
	type metricData struct {
		valueToAdd float64
		labels     map[string]string
		startTime  time.Time
	}
	FakeStartTime1 := time.Now()
	FakeStartTime2 := FakeStartTime1.Add(time.Second)
	FakeStartTime3 := FakeStartTime2.Add(time.Second)
	testcases := []struct {
		desc      string
		inputs    []metricData
		wantDelta *DeltaDouble
	}{
		{
			desc: "add value to a new Delta metric",
			inputs: []metricData{
				{
					valueToAdd: 1.1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
			},
			wantDelta: &DeltaDouble{
				opts: deltaOpts,
				value: map[uint64]*deltaDouble{
					hashLabels(deltaLabel1): newDeltaDouble(1.1, deltaLabel1, FakeStartTime1),
				},
			},
		},
		{
			desc: "add value to a Delta metric with existing label",
			inputs: []metricData{
				{
					valueToAdd: 1.1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2.2,
					labels:     deltaLabel1,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3.3,
					labels:     deltaLabel1,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaDouble{
				opts: deltaOpts,
				value: map[uint64]*deltaDouble{
					hashLabels(deltaLabel1): newDeltaDouble(3.3, deltaLabel1, FakeStartTime3),
				},
			},
		},
		{
			desc: "add value to a Delta metric with new labels",
			inputs: []metricData{
				{
					valueToAdd: 1.1,
					labels:     deltaLabel1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2.2,
					labels:     deltaLabel2,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3.3,
					labels:     deltaLabel3,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaDouble{
				opts: deltaOpts,
				value: map[uint64]*deltaDouble{
					hashLabels(deltaLabel1): newDeltaDouble(1.1, deltaLabel1, FakeStartTime1),
					hashLabels(deltaLabel2): newDeltaDouble(2.2, deltaLabel2, FakeStartTime2),
					hashLabels(deltaLabel3): newDeltaDouble(3.3, deltaLabel3, FakeStartTime3),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaDouble(deltaOpts)
			for _, input := range tc.inputs {
				// Mock the start time.
				now = func() time.Time { return input.startTime }
				gotDelta.AddWithLabels(input.valueToAdd, input.labels)
			}
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, cmp.AllowUnexported(DeltaDouble{}, deltaDouble{}),
				cmpopts.IgnoreFields(DeltaDouble{}, "mu")); diff != "" {
				t.Errorf("Add value to DeltaDouble metric with labels returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaDoubleAddWithoutLabels(t *testing.T) {
	type metricData struct {
		valueToAdd float64
		startTime  time.Time
	}
	FakeStartTime1 := time.Now()
	FakeStartTime2 := FakeStartTime1.Add(time.Second)
	FakeStartTime3 := FakeStartTime2.Add(time.Second)
	testcases := []struct {
		desc      string
		inputs    []metricData
		wantDelta *DeltaDouble
	}{
		{
			desc: "add value to a new Delta metric without labels",
			inputs: []metricData{
				{
					valueToAdd: 1.1,
					startTime:  FakeStartTime1,
				},
			},
			wantDelta: &DeltaDouble{
				opts: deltaOpts,
				value: map[uint64]*deltaDouble{
					hashLabels(nil): newDeltaDouble(1.1, nil, FakeStartTime1),
				},
			},
		},
		{
			desc: "update value to a existing Delta metric without labels",
			inputs: []metricData{
				{
					valueToAdd: 1.1,
					startTime:  FakeStartTime1,
				},
				{
					valueToAdd: 2.2,
					startTime:  FakeStartTime2,
				},
				{
					valueToAdd: 3.3,
					startTime:  FakeStartTime3,
				},
			},
			wantDelta: &DeltaDouble{
				opts: deltaOpts,
				value: map[uint64]*deltaDouble{
					hashLabels(nil): newDeltaDouble(3.3, nil, FakeStartTime3),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotDelta := NewDeltaDouble(deltaOpts)
			for _, input := range tc.inputs {
				// Mock the start time.
				now = func() time.Time { return input.startTime }
				gotDelta.AddWithoutLabels(input.valueToAdd)
			}
			if diff := cmp.Diff(tc.wantDelta.value, gotDelta.value, cmp.AllowUnexported(DeltaDouble{}, deltaDouble{}),
				cmpopts.IgnoreFields(DeltaDouble{}, "mu")); diff != "" {
				t.Errorf("Add value to DeltaDouble metric with labels returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaDoubleExportTimeSeries(t *testing.T) {
	type metricData struct {
		labels     map[string]string
		valueToAdd float64
	}

	testcases := []struct {
		desc           string
		inputs         []metricData
		wantTimeSeries map[string]*mgrpb.TimeSeries
	}{
		{
			desc:           "no inputs",
			inputs:         []metricData{},
			wantTimeSeries: map[string]*mgrpb.TimeSeries{},
		},
		{
			desc: "multiple inputs",
			inputs: []metricData{
				{
					// labels is nil, it is identical to label is map[string]string{}
					labels:     nil,
					valueToAdd: 1,
				},
				{
					labels:     map[string]string{},
					valueToAdd: 1,
				},
				{
					labels:     deltaLabel1,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel2,
					valueToAdd: 3,
				},
				{
					labels:     deltaLabel2,
					valueToAdd: 3,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 4,
				},
				{
					labels:     deltaLabel3,
					valueToAdd: 0,
				},
			},
			wantTimeSeries: map[string]*mgrpb.TimeSeries{
				"test-metric-name": &mgrpb.TimeSeries{
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: map[string]string{},
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_DOUBLE,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1},
						},
					}},
				},
				"test-metric-name/label_value": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel1,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_DOUBLE,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 4},
						},
					}},
				},
				"test-metric-name/label2_value2": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel2,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_DOUBLE,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 3},
						},
					}},
				},
				"test-metric-name/label_value3/label2_value3": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: deltaLabel3,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_DELTA,
					ValueType:  metricpb.MetricDescriptor_DOUBLE,
					Unit:       "count",
					Points: []*mgrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{DoubleValue: 0},
						},
					}},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }
			gotDelta := NewDeltaDouble(deltaOpts)
			for _, input := range tc.inputs {
				if len(input.labels) != 0 {
					gotDelta.AddWithLabels(input.valueToAdd, input.labels)
				} else {
					gotDelta.AddWithoutLabels(input.valueToAdd)
				}
			}
			fakeExporter := &fakeExporter{got: make(map[string]*mgrpb.TimeSeries, len(gotDelta.value))}

			// Mock the end time.
			now = func() time.Time { return fakeEndTime }

			gotDelta.ExportTimeSeries(nodeMonitoringResource, fakeExporter.export)
			if diff := cmp.Diff(tc.wantTimeSeries, fakeExporter.got,
				protocmp.Transform()); diff != "" {
				t.Errorf("Exported Delta TimeSeries returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDeltaDoubleReset(t *testing.T) {
	testcases := []struct {
		desc           string
		existingMetric func() *DeltaDouble
	}{
		{
			desc: "Reset a metric with existing streams only keeps its options",
			existingMetric: func() *DeltaDouble {
				metric := NewDeltaDouble(deltaOpts)
				metric.AddWithoutLabels(1.1)
				metric.AddWithoutLabels(2.2)
				metric.AddWithoutLabels(3.3)
				return metric
			},
		},
		{
			desc: "Reset a metric with no streams only resets its start time",
			existingMetric: func() *DeltaDouble {
				return NewDeltaDouble(deltaOpts)
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }
			existingMetric := tc.existingMetric()
			existingMetric.Reset()
			diffOpts := []cmp.Option{
				cmp.AllowUnexported(DeltaDouble{}, deltaDouble{}, Resource{}),
				cmpopts.IgnoreFields(DeltaDouble{}, "mu"),
			}
			wantMetric := NewDeltaDouble(deltaOpts)
			if diff := cmp.Diff(wantMetric, existingMetric, diffOpts...); diff != "" {
				t.Errorf("Reset() returned diff (-want, +got):\n%s", diff)
			}

			// Ensure that no TimeSeries would be exported after a metric reset.
			gotTimeSeries := []*mgrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mgrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mgrpb.TimeSeries))
			})
			if tsCount := len(gotTimeSeries); tsCount > 0 {
				t.Errorf("ExportTimeSeries() returned %v timeseries after a metric was reset, want no timeseries\n", gotTimeSeries)
			}

			// Ensure that a call to Add...() after a metric reset creates a single TimeSeries with a
			// reset start time.
			existingMetric.AddWithoutLabels(100.0)
			wantTimeSeries := []*mgrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_DELTA,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Unit:       "count",
				Points: []*mgrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: timestamppb.New(fakeStartTime),
						EndTime:   timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DoubleValue{DoubleValue: 100.0},
					},
				}},
			}}
			// Mock the end time, so we have a deterministic output.
			now = func() time.Time { return fakeEndTime }
			gotTimeSeries = []*mgrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mgrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mgrpb.TimeSeries))
			})
			if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, protocmp.Transform()); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}
