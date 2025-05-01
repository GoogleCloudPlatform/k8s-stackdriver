package metrics

import (
	"sort"
	"sync"
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

var (
	cumulativeOpts = DescriptorOpts{
		Name:   testMetricName,
		Unit:   "count",
		Labels: []LabelDescriptor{{Name: "key"}, {Name: "key2"}},
	}
	nodeMonitoringResource = NewK8sNode("123", "example-location", "example-cluster", "example-node")
	fakeStartTime          = time.Now()
	fakeEndTime            = fakeStartTime.Add(time.Second)
	cumulativeLabel1       = map[string]string{"label": "value"}
	cumulativeLabel2       = map[string]string{"label2": "value2"}
	cumulativeLabel3       = map[string]string{"label": "value3", "label2": "value3"}
)

func TestCumulativeInt64ExportTimeSeries(t *testing.T) {
	type metricData struct {
		labels     map[string]string
		valueToAdd int64
	}

	testcases := []struct {
		desc           string
		inputs         []metricData
		wantTimeSeries map[string]*mrpb.TimeSeries
	}{
		{
			desc:           "no inputs",
			inputs:         []metricData{},
			wantTimeSeries: map[string]*mrpb.TimeSeries{},
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
					labels:     cumulativeLabel1,
					valueToAdd: 4,
				},
				{
					labels:     cumulativeLabel2,
					valueToAdd: 3,
				},
				{
					labels:     cumulativeLabel2,
					valueToAdd: 3,
				},
				{
					labels:     cumulativeLabel3,
					valueToAdd: 4,
				},
				{
					labels:     cumulativeLabel3,
					valueToAdd: 4,
				},
				{
					labels:     cumulativeLabel3,
					valueToAdd: 0,
				},
			},
			wantTimeSeries: map[string]*mrpb.TimeSeries{
				"test-metric-name": &mrpb.TimeSeries{
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: map[string]string{},
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 2},
						},
					}},
				},
				"test-metric-name/label_value": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: cumulativeLabel1,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mrpb.Point{{
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
						Labels: cumulativeLabel2,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 6},
						},
					}},
				},
				"test-metric-name/label_value3/label2_value3": {
					Metric: &metricpb.Metric{
						Type:   testMetricName,
						Labels: cumulativeLabel3,
					},
					Resource:   CreateMonitoredResource(nodeMonitoringResource),
					MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
					ValueType:  metricpb.MetricDescriptor_INT64,
					Unit:       "count",
					Points: []*mrpb.Point{{
						Interval: &cpb.TimeInterval{
							StartTime: timestamppb.New(fakeStartTime),
							EndTime:   timestamppb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{Int64Value: 8},
						},
					}},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			// Always reset the time for each test case.
			originalNow := now
			defer func() { now = originalNow }()
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }

			gotCumulative := NewCumulativeInt64(cumulativeOpts)

			for _, input := range tc.inputs {
				if len(input.labels) != 0 {
					gotCumulative.WithLabels(input.labels).Add(input.valueToAdd)
				} else {
					gotCumulative.AddWithoutLabels(input.valueToAdd)
				}
			}
			fakeExporter := &fakeExporter{got: make(map[string]*mrpb.TimeSeries, len(gotCumulative.cumulatives))}

			// Mock the end time.
			now = func() time.Time { return fakeEndTime }

			gotCumulative.ExportTimeSeries(nodeMonitoringResource, fakeExporter.export)
			if diff := cmp.Diff(tc.wantTimeSeries, fakeExporter.got,
				protocmp.Transform()); diff != "" {
				t.Errorf("Exported Cumulative TimeSeries returned unexpected diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestCumulativeInt64WithLabels(t *testing.T) {
	now = func() time.Time {
		return fakeStartTime
	}
	testcases := []struct {
		desc                string
		labels              map[string]string
		existingCumulatives *CumulativeInt64
		wantCumulatives     *CumulativeInt64
		wantCumulative      *cumulativeInt64
	}{
		{
			desc:   "add a new cumulative metric with one label",
			labels: cumulativeLabel1,
			existingCumulatives: &CumulativeInt64{
				options:     cumulativeOpts,
				cumulatives: make(map[uint64]*cumulativeInt64),
				initTime:    now(),
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel1): newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
		},
		{
			desc:   "find an existing cumulative metric with one label",
			labels: cumulativeLabel1,
			existingCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel1): newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel1): newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
		},
		{
			desc:   "add a new cumulative metrics with multiple labels",
			labels: cumulativeLabel3,
			existingCumulatives: &CumulativeInt64{
				options:     cumulativeOpts,
				cumulatives: make(map[uint64]*cumulativeInt64),
				initTime:    now(),
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel3): newCumulativeInt64(&cumulativeOpts, cumulativeLabel3, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel3, fakeStartTime),
		},
		{
			desc:   "find an existing cumulative metric with multiple labels",
			labels: cumulativeLabel3,
			existingCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel3): newCumulativeInt64(&cumulativeOpts, cumulativeLabel3, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel3): newCumulativeInt64(&cumulativeOpts, cumulativeLabel3, fakeStartTime),
				},
				initTime: fakeStartTime,
			},
			wantCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel3, fakeStartTime),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.existingCumulatives.WithLabels(tc.labels)
			if diff := cmp.Diff(tc.wantCumulative, got, cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{})); diff != "" {
				t.Errorf("CumulativeInt64.WithLabels(%v) returned diff (-want, +got):\n%s", tc.labels, diff)
			}
			if diff := cmp.Diff(tc.wantCumulatives, tc.existingCumulatives,
				cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{}),
				cmpopts.IgnoreFields(CumulativeInt64{}, "mu")); diff != "" {
				t.Errorf("CumulativeInt64.WithLabels(%v) did not update a metric correctly, returning diff on CumulativeInt64 (-want, +got):\n%s", tc.labels, diff)
			}
		})
	}
}

func TestCumulativeInt64InitStreamTimestamp(t *testing.T) {
	now = func() time.Time {
		return fakeStartTime
	}
	oneHourAgo := fakeStartTime.Add(-time.Hour)
	oneHourLater := fakeStartTime.Add(time.Hour)
	testcases := []struct {
		desc            string
		metric          func() (*CumulativeInt64, error)
		wantCumulatives *CumulativeInt64
		wantErr         bool
	}{
		{
			desc: "simple case",
			metric: func() (*CumulativeInt64, error) {
				metric := NewCumulativeInt64(cumulativeOpts)
				return metric, metric.InitStreamTimestamp(cumulativeLabel1, oneHourAgo)
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel1): newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, oneHourAgo),
				},
				initTime: fakeStartTime,
			},
		},
		{
			desc: "cannot init the same stream twice",
			metric: func() (*CumulativeInt64, error) {
				metric := NewCumulativeInt64(cumulativeOpts)
				if err := metric.InitStreamTimestamp(cumulativeLabel1, oneHourAgo); err != nil {
					return metric, err
				}
				return metric, metric.InitStreamTimestamp(cumulativeLabel1, oneHourLater)
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(cumulativeLabel1): newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, oneHourAgo),
				},
				initTime: fakeStartTime,
			},
			wantErr: true,
		},
		{
			desc: "InitStreamTimestamp with nil labels",
			metric: func() (*CumulativeInt64, error) {
				metric := NewCumulativeInt64(cumulativeOpts)
				return metric, metric.InitStreamTimestamp(nil, oneHourAgo)
			},
			wantCumulatives: &CumulativeInt64{
				options: cumulativeOpts,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): newCumulativeInt64(&cumulativeOpts, nil, oneHourAgo),
				},
				initTime: fakeStartTime,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.metric()
			if (err != nil) != tc.wantErr {
				t.Fatalf("CumulativeInt64.InitStreamTimestamp returned err=%v (want err=%v)", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantCumulatives, got,
				cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{}),
				cmpopts.IgnoreFields(CumulativeInt64{}, "mu")); diff != "" {
				t.Errorf("CumulativeInt64.InitStreamTimestamp did not update a metric correctly, returning diff on CumulativeInt64 (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestCumulativeInt64AddWithoutLabels(t *testing.T) {
	testcases := []struct {
		desc                string
		valueToAdd          int64
		existingCumulatives *CumulativeInt64
		wantCumulatives     *CumulativeInt64
		wantMetricValue     int64
	}{
		{
			desc:       "add a new cumulative metric without labels",
			valueToAdd: 1,
			existingCumulatives: &CumulativeInt64{
				options:     cumulativeOpts,
				initTime:    fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{},
			},
			wantCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 1),
				},
			},
			wantMetricValue: 1,
		},
		{
			desc:       "update to an existing cumulative metric without labels",
			valueToAdd: 1,
			existingCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 1),
				},
			},
			wantCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 2),
				},
			},
			wantMetricValue: 2,
		},
		{
			desc:       "add negative value to a new cumulative metric without labels, not decrement the metric value",
			valueToAdd: -1,
			existingCumulatives: &CumulativeInt64{
				options:     cumulativeOpts,
				initTime:    fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{},
			},
			wantCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 0),
				},
			},
			wantMetricValue: 0,
		},
		{
			desc:       "add negative value to an existing cumulative metric without labels, not decrement the metric value",
			valueToAdd: -1,
			existingCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 1),
				},
			},
			wantCumulatives: &CumulativeInt64{
				options:  cumulativeOpts,
				initTime: fakeStartTime,
				cumulatives: map[uint64]*cumulativeInt64{
					hashLabels(nil): createTestCumulativeInt64(cumulativeOpts, nil, 1),
				},
			},
			wantMetricValue: 1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.existingCumulatives.AddWithoutLabels(tc.valueToAdd); tc.wantMetricValue != got {
				t.Errorf("CumulativeInt64.AddWithoutLabels(%v)=%v, wanted %v", tc.valueToAdd, got, tc.wantMetricValue)
			}
			if diff := cmp.Diff(tc.wantCumulatives, tc.existingCumulatives,
				cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{}, Resource{}, sync.RWMutex{}, sync.Mutex{}),
				cmpopts.IgnoreFields(CumulativeInt64{}, "mu")); diff != "" {
				t.Errorf("CumulativeInt64.AddWithoutLabels(%v) returned diff on CumulativeInt64 (-want, +got):\n%s", tc.valueToAdd, diff)
			}
		})
	}
}

func TestCumulativeInt64Add(t *testing.T) {
	testcases := []struct {
		desc               string
		valueToAdd         int64
		existingCumulative *cumulativeInt64
		wantCumulative     *cumulativeInt64
		wantMetricValue    int64
	}{
		{
			desc:               "add to a new cumulative metric",
			valueToAdd:         1,
			existingCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
			wantCumulative:     createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 1),
			wantMetricValue:    int64(1),
		},
		{
			desc:               "adding 0 to a new cumulative metric will initialize it with a 0 value",
			valueToAdd:         0,
			existingCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
			wantCumulative:     createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 0),
			wantMetricValue:    int64(0),
		},
		{
			desc:               "add to a cumulative metric with existing value",
			valueToAdd:         1,
			existingCumulative: createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 1),
			wantCumulative:     createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 2),
			wantMetricValue:    int64(2),
		},
		{
			desc:               "add negative value to a new cumulative metric, not decrement the metric value",
			valueToAdd:         -1,
			existingCumulative: newCumulativeInt64(&cumulativeOpts, cumulativeLabel1, fakeStartTime),
			wantCumulative:     createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 0),
			wantMetricValue:    int64(0),
		},
		{
			desc:               "add negative value to a cumulative metric with existing value",
			valueToAdd:         -1,
			existingCumulative: createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 1),
			wantCumulative:     createTestCumulativeInt64(cumulativeOpts, cumulativeLabel1, 1),
			wantMetricValue:    int64(1),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.existingCumulative.Add(tc.valueToAdd); got != tc.wantMetricValue {
				t.Errorf("cumulativeInt64.Add(%v)=%v, wanted %v", tc.valueToAdd, got, tc.wantMetricValue)
			}
			if diff := cmp.Diff(tc.wantCumulative, tc.existingCumulative,
				cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{}, Resource{})); diff != "" {
				t.Errorf("cumulativeInt64.Add(%v) returned diff on cumulativeInt64 (-want, +got):\n%s", tc.valueToAdd, diff)
			}
		})
	}
}

func TestCumulativeInt64Set(t *testing.T) {
	metric := NewCumulativeInt64(cumulativeOpts)
	testcases := []struct {
		desc      string
		operation func()
		wantValue int64
	}{
		{
			desc:      "start from 0",
			wantValue: 0,
		},
		{
			desc:      "set to -1 which fails",
			operation: func() { metric.WithLabels(nil).Set(-1) },
			wantValue: 0,
		},
		{
			desc:      "set from 0 to 0",
			operation: func() { metric.WithLabels(nil).Set(0) },
			wantValue: 0,
		},
		{
			desc:      "set from 0 to 1",
			operation: func() { metric.WithLabels(nil).Set(1) },
			wantValue: 1,
		},
		{
			desc:      "set from 1 to 5",
			operation: func() { metric.WithLabels(nil).Set(5) },
			wantValue: 5,
		},
		{
			desc:      "set from 5 to 5 which does nothing",
			operation: func() { metric.WithLabels(nil).Set(5) },
			wantValue: 5,
		},
		{
			desc:      "add 2",
			operation: func() { metric.WithLabels(nil).Add(2) },
			wantValue: 7,
		},
		{
			desc:      "set to 6 which fails",
			operation: func() { metric.WithLabels(nil).Set(6) },
			wantValue: 7,
		},
		{
			desc: "reset and then set to 6 which succeeds",
			operation: func() {
				metric.Reset()
				metric.WithLabels(nil).Set(6)
			},
			wantValue: 6,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.operation != nil {
				tc.operation()
			}
			if got := metric.WithLabels(nil).Add(0); got != tc.wantValue {
				t.Errorf("cumulativeInt64 had value %d, wanted %v", got, tc.wantValue)
			}
		})
	}
}

func TestCumulativeInt64Reset(t *testing.T) {
	testcases := []struct {
		desc           string
		existingMetric func() *CumulativeInt64
	}{
		{
			desc: "Reset a metric with existing streams only keeps its options",
			existingMetric: func() *CumulativeInt64 {
				metric := NewCumulativeInt64(cumulativeOpts)
				metric.AddWithoutLabels(0)
				metric.AddWithoutLabels(1)
				metric.AddWithoutLabels(2)
				return metric
			},
		},
		{
			desc: "Reset a metric with no streams only resets its start time",
			existingMetric: func() *CumulativeInt64 {
				return NewCumulativeInt64(cumulativeOpts)
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
			// Mock the start time.
			now = func() time.Time { return fakeStartTime }
			existingMetric := tc.existingMetric()
			existingMetric.Reset()
			diffOpts := []cmp.Option{
				cmp.AllowUnexported(CumulativeInt64{}, cumulativeInt64{}, atomic.Int64{}, Resource{}),
				cmpopts.IgnoreFields(CumulativeInt64{}, "mu"),
			}
			wantMetric := NewCumulativeInt64(cumulativeOpts)
			if diff := cmp.Diff(wantMetric, existingMetric, diffOpts...); diff != "" {
				t.Errorf("Reset() returned diff (-want, +got):\n%s", diff)
			}

			// Ensure that no TimeSeries would be exported after a metric reset.
			gotTimeSeries := []*mrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
			})
			if tsCount := len(gotTimeSeries); tsCount > 0 {
				t.Errorf("ExportTimeSeries() returned %v timeseries after a metric was reset, want no timeseries\n", gotTimeSeries)
			}

			// Ensure that a call to Add...() after a metric reset creates a single TimeSeries with a
			// reset start time.
			existingMetric.AddWithoutLabels(100)
			wantTimeSeries := []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "count",
				Points: []*mrpb.Point{{
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

func TestCumulativeInt64Unset(t *testing.T) {
	originalNow := now
	t.Cleanup(func() {
		now = originalNow
	})
	oneHourLater := fakeStartTime.Add(time.Hour)
	metricOptions := DescriptorOpts{
		Name:        testMetricName,
		Description: "description",
	}
	testcases := []struct {
		desc   string
		metric func() *CumulativeInt64
		want   []*mrpb.TimeSeries
	}{
		{
			desc: "Unsetting a metric works",
			metric: func() *CumulativeInt64 {
				now = func() time.Time { return fakeStartTime }
				metric := NewCumulativeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Add(1)
				metric.WithLabels(map[string]string{"label 2": "value 2"}).Add(2)
				metric.Unset(map[string]string{"label": "value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label 2": "value 2"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: timestamppb.New(fakeStartTime),
						EndTime:   timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 2},
					},
				}},
			}},
		},
		{
			desc: "Unsetting a non-existent stream does nothing",
			metric: func() *CumulativeInt64 {
				now = func() time.Time { return fakeStartTime }
				metric := NewCumulativeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Add(1)
				metric.Unset(map[string]string{"label": "not the actual value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: timestamppb.New(fakeStartTime),
						EndTime:   timestamppb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 1},
					},
				}},
			}},
		},
		{
			desc: "Unsetting and re-setting a stream works",
			metric: func() *CumulativeInt64 {
				now = func() time.Time { return fakeStartTime }
				metric := NewCumulativeInt64(metricOptions)
				metric.WithLabels(map[string]string{"label": "value"}).Add(1)
				metric.Unset(map[string]string{"label": "value"})
				if err := metric.InitStreamTimestamp(map[string]string{"label": "value"}, oneHourLater); err != nil {
					t.Fatal(err)
				}
				metric.WithLabels(map[string]string{"label": "value"}).Add(2)
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: timestamppb.New(oneHourLater),
						EndTime:   timestamppb.New(fakeEndTime),
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

func TestCumulativeInt64ExportTimeSeriesDoesNotBlockWithLabels(t *testing.T) {
	gotCumulative := NewCumulativeInt64(cumulativeOpts)
	exportStarted := make(chan struct{})
	withLabelsDone := make(chan struct{})
	gotCumulative.WithLabels(map[string]string{"label": "value"})

	go gotCumulative.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
		close(exportStarted)
		<-withLabelsDone
	})

	<-exportStarted
	go func() {
		// We use a different label to ensure that writer lock is necessary.
		gotCumulative.WithLabels(map[string]string{"new-label": "value"})
		close(withLabelsDone)
	}()

	select {
	case <-withLabelsDone:
	case <-time.After(time.Second):
		t.Errorf("WithLabels() did blocked with labels")
	}
}

type fakeExporter struct {
	got map[string]*mrpb.TimeSeries
}

func (e *fakeExporter) export(ts *mrpb.TimeSeries) {
	labels := ts.GetMetric().GetLabels()
	var sortedLabelKeys []string
	for k := range labels {
		sortedLabelKeys = append(sortedLabelKeys, k)
	}
	sort.Strings(sortedLabelKeys)

	id := ts.GetMetric().GetType()
	for _, labelKey := range sortedLabelKeys {
		id += "/" + labelKey + "_" + labels[labelKey]
	}
	e.got[id] = ts
}

func createTestCumulativeInt64(options DescriptorOpts, labels map[string]string, value int64) *cumulativeInt64 {
	cumulative := newCumulativeInt64(&options, labels, fakeStartTime)
	cumulative.value.Store(value)
	return cumulative
}
