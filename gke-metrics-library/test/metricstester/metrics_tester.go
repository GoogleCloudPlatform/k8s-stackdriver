// Package metricstester has utility functions to test metrics defined with the
// GKE Metrics Push library.
// This can only be used in tests. This is enforced by the BUILD rule testonly.
package metricstester

import (
	"fmt"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"
	distributionpb "google.golang.org/genproto/googleapis/api/distribution"
	metricpb "google.golang.org/genproto/googleapis/api/metric"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

// Metric is a GKE Push Library metric type in the context of unit tests.
type Metric interface {
	ExportTimeSeries(resource metrics.Resource, export func(*mrpb.TimeSeries))
}

// ExpectedMetricValue of a non-distribution metric. Use ExpectedDistributionMetricValue for
// distributions.
type ExpectedMetricValue struct {
	// Expected metric name.
	MetricName string
	// Expected metric labels and their values.
	Labels map[string]string
	// Expected metric value.
	MetricValue any
}

// ExpectedDistributionMetricValue of a distribution metric with streams.
type ExpectedDistributionMetricValue struct {
	// Expected metric name.
	MetricName string
	// A list of distribution streams, each stream contains expected labels, mean and count.
	Streams []*ExpectedDistributionStream
}

// ExpectedDistributionStream contains the expected value for a distribution stream.
type ExpectedDistributionStream struct {
	// Expected metric labels and their values.
	Labels map[string]string
	// Expected distribution mean.
	DistributionMean float64
	// Expected distribution count.
	DistributionCount int64
}

// ExpectedCumulativeMetricValue of a cumulative metric with streams.
type ExpectedCumulativeMetricValue struct {
	// Expected metric name.
	MetricName string
	// A list of cumulative streams, each stream contains expected labels and count.
	Streams []*ExpectedCumulativeStream
}

// ExpectedCumulativeStream contains the expected value for a cumulative stream: labels and count.
// We currently only support int64 for cumulative count.
type ExpectedCumulativeStream struct {
	// Expected metric labels and their values.
	Labels map[string]string
	// Expected cumulative count.
	MetricValue int64
}

// ExpectCumulativeInt64 verifies that the CUMULATIVE INT64 metric has all fields
// populated correctly (except metric Resource and the metric timestamps, to make testing simpler).
// and returns an error indicating the differences if the validation fails.
// ExpectCumulativeInt64 does not keep state of metrics, so ExpectedMetricValue should be
// the metric's absolute expected values
// For example, if your metric already has value 2 before starting the test case and you expect its
// value to be incremented by 1, you should set the value in ExpectedMetricValue to 3.
func ExpectCumulativeInt64(got Metric, expected ExpectedCumulativeMetricValue) error {
	wantTimeSeries := timeSeriesOfCumulativeInt64(expected)

	var gotTimeSeries []*mrpb.TimeSeries
	got.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
		gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
	})

	opts := cmp.Options{
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.Transform(),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "resource"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, opts); diff != "" {
		return fmt.Errorf("ExpectCumulativeInt64() returned diff (-want/+got)\n%s", diff)
	}
	return nil
}

func timeSeriesOfCumulativeInt64(expected ExpectedCumulativeMetricValue) []*mrpb.TimeSeries {
	if len(expected.Streams) == 0 {
		return nil
	}
	timeSeries := make([]*mrpb.TimeSeries, 0, len(expected.Streams))

	for _, stream := range expected.Streams {
		timeSeries = append(timeSeries, &mrpb.TimeSeries{
			Metric: &metricpb.Metric{
				Type:   expected.MetricName,
				Labels: stream.Labels,
			},
			MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
			ValueType:  metricpb.MetricDescriptor_INT64,
			Points: []*mrpb.Point{metrics.CreateCumulativePoint(metrics.CreateInt64TypedValue(stream.MetricValue),
				time.Time{}, time.Time{})},
			// We ignore the metric resource because this is only set at export time. In this test helper
			// we only care about the metric value itself, metric label values and their metadata.
		})
	}
	return timeSeries
}

// ExpectGaugeInt64 verifies that the GAUGE INT64 metric has all fields
// populated correctly (except metric Resource and the metric timestamps, to make testing simpler).
// and returns an error indicating the differences if the validation fails.
func ExpectGaugeInt64(metric Metric, expected ExpectedMetricValue) error {
	// This function is resilient to be called if the caller doesn't set an expected metric value.
	// It will consider that no metric streams should've been reported, in this case.
	if expected.MetricValue == nil {
		var gotTs *mrpb.TimeSeries
		metric.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
			gotTs = ts
		})
		if gotTs != nil {
			return fmt.Errorf("expected a metric object with no streams, got %v", metric)
		}
		return nil
	}

	metricValue, ok := expected.MetricValue.(int64)
	if !ok {
		return fmt.Errorf("invalid expected value type for INT64 gauge metric: want int64, got %T", expected.MetricValue)
	}
	want := &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   expected.MetricName,
			Labels: expected.Labels,
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Points: []*mrpb.Point{metrics.CreateGaugePoint(metrics.CreateInt64TypedValue(metricValue),
			time.Time{})},
		// We ignore the metric resource because this is only set at export time. In this test helper
		// we only care about the metric value itself, metric label values and their metadata.
	}

	var got *mrpb.TimeSeries
	metric.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
		got = proto.Clone(ts).(*mrpb.TimeSeries)
	})

	opts := cmp.Options{
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.Transform(),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "resource"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(want, got, opts); diff != "" {
		return fmt.Errorf("ExpectGaugeInt64(%v, %v) returned diff (-want/+got)\n%s", metric, expected, diff)
	}
	return nil
}

// ExpectGaugeDouble verifies that the GAUGE DOUBLE metric has all fields
// populated correctly (except metric Resource and the metric timestamps, to make testing simpler).
// and returns an error indicating the differences if the validation fails.
func ExpectGaugeDouble(metric Metric, expected ExpectedMetricValue) error {
	// This function is resilient to be called if the caller doesn't set an expected metric value.
	// It will consider that no metric streams should've been reported, in this case.
	if expected.MetricValue == nil {
		var gotTs *mrpb.TimeSeries
		metric.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
			gotTs = ts
		})
		if gotTs != nil {
			return fmt.Errorf("expected a metric object with no streams, got %v", metric)
		}
		return nil
	}

	metricValue, ok := expected.MetricValue.(float64)
	if !ok {
		return fmt.Errorf("invalid expected value type for DOUBLE gauge metric: want float64, got %T", expected.MetricValue)
	}
	want := &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   expected.MetricName,
			Labels: expected.Labels,
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points: []*mrpb.Point{metrics.CreateGaugePoint(metrics.CreateDoubleTypedValue(metricValue),
			time.Time{})},
		// We ignore the metric resource because this is only set at export time. In this test helper
		// we only care about the metric value itself, metric label values and their metadata.
	}

	var got *mrpb.TimeSeries
	metric.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
		got = proto.Clone(ts).(*mrpb.TimeSeries)
	})

	opts := cmp.Options{
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.Transform(),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "resource"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(want, got, opts); diff != "" {
		return fmt.Errorf("ExpectGaugeDouble(%v, %v) returned diff (-want/+got)\n%s", metric, expected, diff)
	}
	return nil
}

// ExpectDistribution verifies that the Distribution metric has all fields populated correctly
// (except metric Resource and the metric timestamps, to make testing simpler). For the distribution
// value, it verifies that the count and mean are as expected. It returns an error indicating the
// differences if the validation fails.
// ExpectDistribution does not keep state of metrics, any values should be
// the metric's absolute expected values instead of delta.
func ExpectDistribution(gotDistribution Metric, expected ExpectedDistributionMetricValue) error {
	wantTimeSeries := timeSeriesOfDistribution(expected)

	var gotTimeSeries []*mrpb.TimeSeries
	gotDistribution.ExportTimeSeries(metrics.Resource{}, func(ts *mrpb.TimeSeries) {
		gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
	})

	opts := cmp.Options{
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.Transform(),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "resource"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
		protocmp.FilterMessage(&distributionpb.Distribution{}, compareDistributionStreamValue()),
	}
	if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, opts); diff != "" {
		return fmt.Errorf("ExpectDistribution() returned diff (-want/+got)\nNote: for distribution value, we only compare count and mean\n%s", diff)
	}
	return nil
}

// timeSeriesOfDistribution converts the expected distribution streams with data into timeseries.
func timeSeriesOfDistribution(expected ExpectedDistributionMetricValue) []*mrpb.TimeSeries {
	if len(expected.Streams) == 0 {
		return nil
	}
	timeSeries := make([]*mrpb.TimeSeries, 0, len(expected.Streams))

	for _, stream := range expected.Streams {
		distributionTypedValue := &cpb.TypedValue{
			Value: &cpb.TypedValue_DistributionValue{
				DistributionValue: &distributionpb.Distribution{
					// For distribution metrics, we only compare the count.
					Count: stream.DistributionCount,
					Mean:  stream.DistributionMean,
				},
			},
		}
		timeSeries = append(timeSeries, &mrpb.TimeSeries{
			Metric: &metricpb.Metric{
				Type:   expected.MetricName,
				Labels: stream.Labels,
			},
			MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
			ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
			Points: []*mrpb.Point{metrics.CreateCumulativePoint(distributionTypedValue,
				time.Time{}, time.Time{})},
			// We ignore the metric resource because this is only set at export time. In this test helper
			// we only care about the metric value itself, metric label values and their metadata.
		})
	}
	return timeSeries
}

// When comparing the value of one distribution stream, only check that the count and mean of data
// points are expected.
func compareDistributionStreamValue() cmp.Option {
	return cmp.Comparer(func(a, b protocmp.Message) bool {
		if a["count"] != b["count"] {
			return false
		}
		if a["mean"] != b["mean"] {
			return false
		}
		return true
	})
}
