package metrics

import (
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/testing/protocmp"
	tpb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	testNodeResource               = NewK8sNode(testProjectName, testLocation, testCluster, testNode)
	testContainerResource          = NewK8sContainer(testProjectName, testLocation, testCluster, testNamespace, testPod, testContainer)
	testPodResource                = NewK8sPod(testProjectName, testLocation, testCluster, testNamespace, testPod)
	testMonitoredNodeResource      = CreateMonitoredResource(testNodeResource)
	testMonitoredContainerResource = CreateMonitoredResource(testContainerResource)
	testMonitoredPodResource       = CreateMonitoredResource(testPodResource)
)

func TestCreateTimeSeriesForStatelessCumulativeDouble(t *testing.T) {
	testCases := []struct {
		desc           string
		descriptorOpts DescriptorOpts
		resource       Resource
		value          float64
		labels         map[string]string
		startTime      time.Time
		endTime        time.Time
		want           *mgrpb.TimeSeries
	}{
		{
			desc: "metric with value, labels",
			descriptorOpts: DescriptorOpts{
				Name:        testMetricName,
				Description: testMetricDescription,
				Unit:        "1",
			},
			resource: testContainerResource,
			value:    1.0,
			labels: map[string]string{
				"test-key": "test-value",
			},
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: testMetricName,
					Labels: map[string]string{
						"test-key": "test-value",
					},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{
								DoubleValue: 1.0,
							},
						},
					},
				},
			},
		},
		{
			desc: "metric with value, no labels",
			descriptorOpts: DescriptorOpts{
				Name:        testMetricName,
				Description: testMetricDescription,
				Unit:        "1",
			},
			resource:  testContainerResource,
			value:     1.0,
			labels:    nil,
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DoubleValue{
								DoubleValue: 1.0,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cumulativeDouble := NewStatelessCumulativeDouble(tc.descriptorOpts)
			var got *mgrpb.TimeSeries
			export := func(ts *mgrpb.TimeSeries) { got = ts }
			cumulativeDouble.ExportTimeSeries(tc.resource, tc.value, tc.labels, tc.startTime, tc.endTime, export)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("cumulativeDouble.CreateTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestCreateTimeSeriesForStatelessCumulativeInt64(t *testing.T) {
	testCases := []struct {
		desc           string
		descriptorOpts DescriptorOpts
		resource       Resource
		value          int64
		labels         map[string]string
		startTime      time.Time
		endTime        time.Time
		want           *mgrpb.TimeSeries
	}{
		{
			desc: "metric with value, labels",
			descriptorOpts: DescriptorOpts{
				Name:        testMetricName,
				Description: testMetricDescription,
				Unit:        "1",
			},
			resource: testPodResource,
			value:    1,
			labels: map[string]string{
				"test-key": "test-value",
			},
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: testMetricName,
					Labels: map[string]string{
						"test-key": "test-value",
					},
				},
				Resource:   testMonitoredPodResource,
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{
								Int64Value: 1,
							},
						},
					},
				},
			},
		},
		{
			desc: "metric with value, no labels",
			descriptorOpts: DescriptorOpts{
				Name:        testMetricName,
				Description: testMetricDescription,
				Unit:        "1",
			},
			resource:  testPodResource,
			value:     1,
			labels:    nil,
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{},
				},
				Resource:   testMonitoredPodResource,
				MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_Int64Value{
								Int64Value: 1,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cumulativeInt64 := NewStatelessCumulativeInt64(tc.descriptorOpts)
			var got *mgrpb.TimeSeries
			export := func(ts *mgrpb.TimeSeries) { got = ts }
			cumulativeInt64.ExportTimeSeries(tc.resource, tc.value, tc.labels, tc.startTime, tc.endTime, export)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("cumulativeInt64.CreateTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}
