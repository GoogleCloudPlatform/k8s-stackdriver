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

func TestCreateTimeSeriesForStatelessGaugeDouble(t *testing.T) {
	testCases := []struct {
		desc           string
		descriptorOpts DescriptorOpts
		resource       Resource
		value          float64
		labels         map[string]string
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
			endTime: testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: testMetricName,
					Labels: map[string]string{
						"test-key": "test-value",
					},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							EndTime: tpb.New(testEndTime),
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
			resource: testContainerResource,
			value:    1.0,
			labels:   nil,
			endTime:  testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{},
				},
				Resource:   testMonitoredContainerResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_DOUBLE,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							EndTime: tpb.New(testEndTime),
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
			gaugeDouble := NewStatelessGaugeDouble(tc.descriptorOpts)
			var got *mgrpb.TimeSeries
			export := func(ts *mgrpb.TimeSeries) { got = ts }
			gaugeDouble.ExportTimeSeries(tc.resource, tc.value, tc.labels, tc.endTime, export)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("gaugeDouble.CreateTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestCreateTimeSeriesForStatelessGaugeInt64(t *testing.T) {
	testCases := []struct {
		desc           string
		descriptorOpts DescriptorOpts
		resource       Resource
		value          int64
		labels         map[string]string
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
			resource: testNodeResource,
			value:    1,
			labels: map[string]string{
				"test-key": "test-value",
			},
			endTime: testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type: testMetricName,
					Labels: map[string]string{
						"test-key": "test-value",
					},
				},
				Resource:   testMonitoredNodeResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							EndTime: tpb.New(testEndTime),
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
			resource: testNodeResource,
			value:    1,
			labels:   nil,
			endTime:  testEndTime,
			want: &mgrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   testMetricName,
					Labels: nil,
				},
				Resource:   testMonitoredNodeResource,
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Unit:       "1",
				Points: []*mgrpb.Point{
					&mgrpb.Point{
						Interval: &cpb.TimeInterval{
							EndTime: tpb.New(testEndTime),
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
			gaugeInt64 := NewStatelessGaugeInt64(tc.descriptorOpts)
			var got *mgrpb.TimeSeries
			export := func(ts *mgrpb.TimeSeries) { got = ts }
			gaugeInt64.ExportTimeSeries(tc.resource, tc.value, tc.labels, tc.endTime, export)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("gaugeInt64.CreateTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}
