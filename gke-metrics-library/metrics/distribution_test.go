package metrics

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	dpb "google.golang.org/genproto/googleapis/api/distribution"
	mpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	tpb "google.golang.org/protobuf/types/known/timestamppb"
)

var (
	testUnit           = "1"
	testDescriptorOpts = DescriptorOpts{
		Name:        testMetricName,
		Unit:        testUnit,
		Description: "test-description",
		Labels:      nil,
	}
	testBucketsUpperBounds     = []float64{0}
	testLabels                 = map[string]string{"test_key": "test_value"}
	testLabels2                = map[string]string{"test_key2": "test_value2"}
	testDistributionOpts       = distributionOpts{descriptorOpts: testDescriptorOpts, bucketsUpperBounds: testBucketsUpperBounds}
	distributionWithTestLabels = newDistribution(testDistributionOpts, testLabels, testStartTime)
	distributionWithoutLabels  = newDistribution(testDistributionOpts, nil, testStartTime)
)

// Create a new Distribution, record values with or without labels, export TimeSeries.
func ExampleDistribution() {
	exampleResource := NewK8sNode("123", "example-location", "example-cluster", "example-node")
	exampleLabels := map[string]string{"some_label": "some_value"}

	// Create Distribution with labels.
	myDistributionWithLabels := NewDistribution(NewDistributionOpts(
		DescriptorOpts{
			Name:        "kubernetes.io/internal/<component_name>/<metric_name>",
			Description: "Some description.",
			Unit:        "some unit",
			Labels: []LabelDescriptor{
				{
					Name:        "some_label",
					Description: "Some label.",
				},
			},
		},
		[]float64{0} /*upperBounds*/).MustValidate())

	// Record value with labels.
	if err := myDistributionWithLabels.WithLabels(exampleLabels).Record(0); err != nil {
		fmt.Printf("error recording distribution with labels %v: %v", exampleLabels, err)
	}

	// Create Distribution without labels.
	myDistributionWithoutLabels := NewDistribution(NewDistributionOpts(
		DescriptorOpts{
			Name:        "kubernetes.io/internal/<component_name>/<metric_name>",
			Description: "Some description.",
			Unit:        "some unit",
		},
		[]float64{0} /*upperBounds*/).MustValidate())

	// Record value with no labels.
	if err := myDistributionWithoutLabels.RecordWithoutLabels(1.5); err != nil {
		fmt.Printf("error recording distribution without labels: %v", err)
	}

	export := func(ts *mrpb.TimeSeries) {
		// Wrap the actual export function with error handling logic here.
	}

	// Export TimeSeries.
	myDistributionWithoutLabels.ExportTimeSeries(exampleResource, export)
	myDistributionWithLabels.ExportTimeSeries(exampleResource, export)
}

func TestDistributionWithLabels(t *testing.T) {
	originalNow := now
	t.Cleanup(func() { now = originalNow })
	now = func() time.Time { return testStartTime }

	testCases := []struct {
		desc                  string
		existingDistribution  Distribution
		labels                map[string]string
		wantDistributionCount int
		wantDistribution      *distribution
	}{
		{
			desc:                  "WithLabels create and return a new distribution",
			existingDistribution:  *NewDistribution(testDistributionOpts),
			labels:                nil,
			wantDistributionCount: 1,
			wantDistribution: &distribution{
				options: testDistributionOpts,
				data: DistributionData{
					Count:                 0,
					Mean:                  0.0,
					SumOfSquaredDeviation: 0.0,
					Min:                   math.Inf(1),
					Max:                   math.Inf(-1),
					BucketCounts:          make([]int64, 2),
				},
				startTime: testStartTime,
			},
		},
		{
			desc: "WithLabels return existing distribution",
			existingDistribution: Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(testLabels): distributionWithTestLabels,
				},
			},
			labels:                testLabels,
			wantDistributionCount: 1,
			wantDistribution: &distribution{
				labels:  testLabels,
				options: testDistributionOpts,
				data: DistributionData{
					Count:                 0,
					Mean:                  0.0,
					SumOfSquaredDeviation: 0.0,
					Min:                   math.Inf(1),
					Max:                   math.Inf(-1),
					BucketCounts:          make([]int64, 2),
				},
				startTime: testStartTime,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.existingDistribution.WithLabels(tc.labels)
			if len(tc.existingDistribution.streams) != tc.wantDistributionCount {
				t.Errorf("WithLabels(%v) got %v distribution(s), want %v", tc.labels, len(tc.existingDistribution.streams), tc.wantDistributionCount)
			}
			if diff := cmp.Diff(tc.wantDistribution, got,
				cmp.AllowUnexported(Distribution{}, distribution{}, distributionOpts{}, DistributionData{}),
				cmpopts.IgnoreFields(Distribution{}, "mu"),
				cmpopts.IgnoreFields(distribution{}, "mu")); diff != "" {
				t.Errorf("WithLabels(%v) returned diff (-want +got):\n%s", tc.labels, diff)
			}
		})
	}
}

func TestDistributionInitStreamTimestamp(t *testing.T) {
	now = func() time.Time {
		return testStartTime
	}
	oneHourAgo := testStartTime.Add(-time.Hour)
	oneHourLater := testStartTime.Add(time.Hour)
	testcases := []struct {
		desc    string
		metric  func() (*Distribution, error)
		want    *Distribution
		wantErr bool
	}{
		{
			desc: "simple case",
			metric: func() (*Distribution, error) {
				metric := NewDistribution(testDistributionOpts)
				return metric, metric.InitStreamTimestamp(testLabels, oneHourAgo)
			},
			want: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(testLabels): newDistribution(testDistributionOpts, testLabels, oneHourAgo),
				},
				initTime: testStartTime,
			},
		},
		{
			desc: "cannot init the same label twice",
			metric: func() (*Distribution, error) {
				metric := NewDistribution(testDistributionOpts)
				if err := metric.InitStreamTimestamp(testLabels, oneHourAgo); err != nil {
					return metric, err
				}
				return metric, metric.InitStreamTimestamp(testLabels, oneHourLater)
			},
			want: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(testLabels): newDistribution(testDistributionOpts, testLabels, oneHourAgo),
				},
				initTime: testStartTime,
			},
			wantErr: true,
		},
		{
			desc: "InitStreamTimestamp with nil labels",
			metric: func() (*Distribution, error) {
				metric := NewDistribution(testDistributionOpts)
				return metric, metric.InitStreamTimestamp(nil, oneHourAgo)
			},
			want: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(nil): newDistribution(testDistributionOpts, nil, oneHourAgo),
				},
				initTime: testStartTime,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.metric()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Distribution.InitStreamTimestamp err=%v (want err=%v)", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got,
				cmp.AllowUnexported(Distribution{}, distribution{}, distributionOpts{}, DistributionData{}),
				cmpopts.IgnoreFields(Distribution{}, "mu"),
				cmpopts.IgnoreFields(distribution{}, "mu")); diff != "" {
				t.Errorf("Distribution.InitStreamTimestamp returned diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRecordWithoutLabels(t *testing.T) {
	testCases := []struct {
		desc                 string
		existingDistribution *Distribution
		wantDistribution     *Distribution
	}{
		{
			desc: "RecordWithoutLabels creates a new distribution and record",
			existingDistribution: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(testLabels): distributionWithTestLabels,
				},
			},
			wantDistribution: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(nil): &distribution{
						options: testDistributionOpts,
						data: DistributionData{
							Count:                 1,
							Mean:                  1.5,
							SumOfSquaredDeviation: 0.0,
							Min:                   1.5,
							Max:                   1.5,
							BucketCounts:          []int64{0, 1},
						},
					},
					hashLabels(testLabels): distributionWithTestLabels,
				},
			},
		},
		{
			desc: "RecordWithoutLabels updates existing distribution",
			existingDistribution: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(nil):        distributionWithoutLabels,
					hashLabels(testLabels): distributionWithTestLabels,
				},
			},
			wantDistribution: &Distribution{
				options: testDistributionOpts,
				streams: map[uint64]*distribution{
					hashLabels(nil): &distribution{
						options: testDistributionOpts,
						data: DistributionData{
							Count:                 1,
							Mean:                  1.5,
							Min:                   1.5,
							Max:                   1.5,
							BucketCounts:          []int64{0, 1},
							SumOfSquaredDeviation: 0.0,
						},
						startTime: testStartTime,
					},
					hashLabels(testLabels): distributionWithTestLabels,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if err := tc.existingDistribution.RecordWithoutLabels(1.5); err != nil {
				t.Errorf("RecordWithoutLabels() returned error: %v, want nil", err)
			}
			if diff := cmp.Diff(tc.wantDistribution, tc.existingDistribution,
				cmp.AllowUnexported(Distribution{}, distribution{}, distributionOpts{}, DistributionData{}),
				cmpopts.IgnoreFields(Distribution{}, "mu"),
				cmpopts.IgnoreFields(distribution{}, "mu")); diff != "" {
				t.Errorf("RecordWithoutLabels() returned diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDistributionRecordSuccess(t *testing.T) {
	testCases := []struct {
		desc           string
		upperbounds    []float64
		valuesToRecord []float64
		want           DistributionData
	}{
		{
			desc:           "one value",
			upperbounds:    []float64{0, 1},
			valuesToRecord: []float64{0},
			want: DistributionData{
				Count:                 1,
				Mean:                  0,
				Max:                   0,
				Min:                   0,
				SumOfSquaredDeviation: 0,
				// (-inf, 0) → 0
				// [0, 1) → 1
				// [1, +inf) → 0
				BucketCounts: []int64{0, 1, 0},
			},
		},
		{
			desc:           "multiple values in all buckets",
			upperbounds:    []float64{0, 10, 20},
			valuesToRecord: []float64{-1, 1, 2, 3, 15, 30},
			want: DistributionData{
				Count:                 6,
				Mean:                  8.333,
				Max:                   30,
				Min:                   -1,
				SumOfSquaredDeviation: 723.333,
				BucketCounts:          []int64{1, 3, 1, 1},
			},
		},
		{
			desc:           "multiple values in some buckets",
			upperbounds:    []float64{0, 10, 20, 30},
			valuesToRecord: []float64{-1, 1, 2, 3, 50},
			want: DistributionData{
				Count:                 5,
				Mean:                  11,
				Max:                   50,
				Min:                   -1,
				SumOfSquaredDeviation: 1910,
				BucketCounts:          []int64{1, 3, 0, 0, 1},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			distopts, err := NewDistributionOpts(testDescriptorOpts, tc.upperbounds).Validate()
			if err != nil {
				t.Errorf("NewDistributionOpts(%v, %v) returned error: %v, want nil", testDescriptorOpts, tc.upperbounds, err)
			}
			testDistribution := newDistribution(distopts, nil, testStartTime)
			for _, value := range tc.valuesToRecord {
				err := testDistribution.Record(value)
				if err != nil {
					t.Errorf("Record(%v) returned error: %v, want nil", value, err)
				}
			}
			opt := cmp.Comparer(func(x, y float64) bool {
				return math.Abs(x-y) < 0.001
			})
			if diff := cmp.Diff(tc.want, testDistribution.data,
				cmp.AllowUnexported(DistributionData{}), opt); diff != "" {
				t.Errorf("Record(%v) returned diff (-want +got):\n%s", tc.valuesToRecord, diff)
			}
		})
	}
}

func TestDistributionRecordFailure(t *testing.T) {
	testCases := []struct {
		desc  string
		value float64
	}{
		{
			desc:  "record NaN",
			value: math.NaN(),
		},
		{
			desc:  "record +Inf",
			value: math.Inf(1),
		},
		{
			desc:  "record -Inf",
			value: math.Inf(-1),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testDistribution := newDistribution(
				NewDistributionOpts(testDescriptorOpts, []float64{0.0}).MustValidate(),
				nil,
				testStartTime,
			)
			if err := testDistribution.Record(tc.value); err == nil {
				t.Errorf("Record(%v) returned nil, want error", tc.value)
			}
		})
	}
}

func TestDistributionSet(t *testing.T) {
	buckets := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	distopts, err := NewDistributionOpts(testDescriptorOpts, buckets).Validate()
	if err != nil {
		t.Fatalf("NewDistributionOpts(%v, %v) returned error: %v, want nil", testDescriptorOpts, buckets, err)
	}
	testDistribution := newDistribution(distopts, nil, testStartTime)
	testDistribution.Set(&DistributionData{
		Count:                 3,
		Mean:                  5,
		Min:                   1,
		Max:                   9,
		SumOfSquaredDeviation: 18,
		BucketCounts:          []int64{0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0},
	})
	want := &mrpb.TimeSeries{
		Metric: &mpb.Metric{
			Type: testMetricName,
		},
		Resource:   testMonitoredNodeResource,
		MetricKind: mpb.MetricDescriptor_CUMULATIVE,
		ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
		Unit:       testUnit,
		Points: []*mrpb.Point{
			{
				Interval: &cpb.TimeInterval{
					StartTime: tpb.New(testStartTime),
					EndTime:   tpb.New(testEndTime),
				},
				Value: &cpb.TypedValue{
					Value: &cpb.TypedValue_DistributionValue{
						DistributionValue: &dpb.Distribution{
							Count:                 3,
							Mean:                  5,
							SumOfSquaredDeviation: 18,
							BucketCounts:          []int64{0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0},
							BucketOptions: &dpb.Distribution_BucketOptions{
								Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
									ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
										Bounds: []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	got := testDistribution.createTimeSeries(testNodeResource, testEndTime)
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("CreateTimeSeries(%v, %v) returned diff (-want +got):\n%s", testNodeResource, testEndTime, diff)
	}
}

func TestDistributionCreateTimeSeries(t *testing.T) {
	testCases := []struct {
		desc         string
		resource     Resource
		distribution distribution
		startTime    time.Time
		endTime      time.Time
		want         *mrpb.TimeSeries
	}{
		{
			desc:     "distribution metric with no labels",
			resource: testNodeResource,
			distribution: distribution{
				options: NewDistributionOpts(testDescriptorOpts, testBucketsUpperBounds).MustValidate(),
				data: DistributionData{ // values {1,1}
					Count:                 2,
					Mean:                  1,
					SumOfSquaredDeviation: 0,
					Min:                   1,
					Max:                   1,
					BucketCounts:          []int64{0, 2},
				},
				startTime: testStartTime,
			},
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mrpb.TimeSeries{
				Metric: &mpb.Metric{
					Type: testMetricName,
				},
				Resource:   testMonitoredNodeResource,
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DistributionValue{
								DistributionValue: &dpb.Distribution{
									Count:                 2,
									Mean:                  1,
									SumOfSquaredDeviation: 0,
									BucketCounts:          []int64{0, 2},
									BucketOptions: &dpb.Distribution_BucketOptions{
										Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
												Bounds: testBucketsUpperBounds,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			desc:     "distribution metric with no data points in overflow bucket",
			resource: testNodeResource,
			distribution: distribution{
				options: NewDistributionOpts(testDescriptorOpts, []float64{0, 10}).MustValidate(),
				data: DistributionData{ // values {1,1}
					Count:                 2,
					Mean:                  1,
					SumOfSquaredDeviation: 0,
					Min:                   1,
					Max:                   1,
					BucketCounts:          []int64{0, 2, 0},
				},
				startTime: testStartTime,
			},
			startTime: testStartTime,
			endTime:   testEndTime,
			want: &mrpb.TimeSeries{
				Metric: &mpb.Metric{
					Type: testMetricName,
				},
				Resource:   testMonitoredNodeResource,
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(testStartTime),
							EndTime:   tpb.New(testEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DistributionValue{
								DistributionValue: &dpb.Distribution{
									Count:                 2,
									Mean:                  1,
									SumOfSquaredDeviation: 0,
									BucketCounts:          []int64{0, 2, 0},
									BucketOptions: &dpb.Distribution_BucketOptions{
										Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{0, 10},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.distribution.createTimeSeries(tc.resource, tc.endTime)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("CreateTimeSeries(%v, %v) returned diff (-want +got):\n%s", tc.resource, tc.endTime, diff)
			}
		})
	}
}

func TestExportTimeSeries(t *testing.T) {
	testCases := []struct {
		desc           string
		labels         []map[string]string
		wantTimeSeries []*mrpb.TimeSeries
	}{
		{
			desc:           "Distribution with no streams.",
			wantTimeSeries: []*mrpb.TimeSeries{},
		},
		{
			desc:   "Distribution with one nil stream.",
			labels: []map[string]string{nil},
			wantTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &mpb.Metric{
						Type: testMetricName,
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_CUMULATIVE,
					ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
					Unit:       testUnit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								StartTime: tpb.New(testStartTime),
								EndTime:   tpb.New(testEndTime),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_DistributionValue{
									DistributionValue: &dpb.Distribution{
										Count:                 0,
										Mean:                  0,
										SumOfSquaredDeviation: 0,
										BucketCounts:          []int64{0, 0},
										BucketOptions: &dpb.Distribution_BucketOptions{
											Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
												ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
													Bounds: testBucketsUpperBounds,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			desc: "Distribution with multiple streams.",
			labels: []map[string]string{
				nil,
				testLabels,
			},
			wantTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &mpb.Metric{
						Type: testMetricName,
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_CUMULATIVE,
					ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
					Unit:       testUnit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								StartTime: tpb.New(testStartTime),
								EndTime:   tpb.New(testEndTime),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_DistributionValue{
									DistributionValue: &dpb.Distribution{
										Count:                 0,
										Mean:                  0,
										SumOfSquaredDeviation: 0,
										BucketCounts:          []int64{0, 0},
										BucketOptions: &dpb.Distribution_BucketOptions{
											Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
												ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
													Bounds: testBucketsUpperBounds,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Metric: &mpb.Metric{
						Type:   testMetricName,
						Labels: testLabels,
					},
					Resource:   testMonitoredContainerResource,
					MetricKind: mpb.MetricDescriptor_CUMULATIVE,
					ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
					Unit:       testUnit,
					Points: []*mrpb.Point{
						{
							Interval: &cpb.TimeInterval{
								StartTime: tpb.New(testStartTime),
								EndTime:   tpb.New(testEndTime),
							},
							Value: &cpb.TypedValue{
								Value: &cpb.TypedValue_DistributionValue{
									DistributionValue: &dpb.Distribution{
										Count:                 0,
										Mean:                  0,
										SumOfSquaredDeviation: 0,
										BucketCounts:          []int64{0, 0},
										BucketOptions: &dpb.Distribution_BucketOptions{
											Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
												ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
													Bounds: testBucketsUpperBounds,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Always reset the time for each test case.
			originalNow := now
			defer func() { now = originalNow }()
			got := []*mrpb.TimeSeries{}
			export := func(ts *mrpb.TimeSeries) {
				got = append(got, ts)
			}
			// Set the current time to testStartTime
			now = func() time.Time {
				return testStartTime
			}
			metrics := NewDistribution(testDistributionOpts)
			for _, label := range tc.labels {
				// "Register" streams into the Distribution object.
				metrics.WithLabels(label)
			}

			// Set the current time to testEndTime
			now = func() time.Time {
				return testEndTime
			}
			metrics.ExportTimeSeries(testContainerResource, export)

			opts := cmp.Options{
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.Transform(),
			}
			if diff := cmp.Diff(tc.wantTimeSeries, got, opts...); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDistributionCopySafe(t *testing.T) {
	// This test makes sure that modifying fields in a Distribution object does not affect
	// the existing TimeSeries that was created from that Distribution object.
	d := newDistribution(testDistributionOpts, map[string]string{"key": "value"}, testStartTime)
	got := d.createTimeSeries(testContainerResource, testEndTime)
	// We deep clone the TimeSeries returned to dereference any pointers and make sure
	// that their values will not change implicitly along the test execution.
	want := proto.Clone(got).(*mrpb.TimeSeries)

	// TODO(b/272054922): fuzz the field values with go/gofuzzing
	// Changing all the fields that are needed for creating TimeSeries to make sure that this will
	// not change the TimeSeries that was generated from this Distribution object.
	// BucketUpperBounds and Labels are copied when they are passed in and never change.
	// Record updates all fields in distributionData.
	if err := d.Record(1); err != nil {
		t.Errorf("Record(1) returned error: %v, want nil error", err)
	}
	d.options.descriptorOpts.Name = "new_name"
	d.options.descriptorOpts.Unit = "new_unit"
	d.options.descriptorOpts.Description = "new description"
	for i := range d.options.descriptorOpts.Labels {
		d.options.descriptorOpts.Labels[i].Name = "new_label"
		d.options.descriptorOpts.Labels[i].Description = "new label description"
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Errorf("Existing TimeSeries object got unexpectedly updated after a Distribution object changed: diff (-want +got):\n%s", diff)
	}

}

func TestRecordThreadSafe(t *testing.T) {
	got := newDistribution(testDistributionOpts, nil, testStartTime)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(value float64) {
			defer wg.Done()
			err := got.Record(value)
			if err != nil {
				t.Errorf("distribution.Record(%v)=%v, want nil error", value, err)
			}
		}(float64(i))
	}
	wg.Wait()
	want := &distribution{
		options: testDistributionOpts,
		data: DistributionData{
			Count:                 10,
			Mean:                  4.5,
			Max:                   9,
			Min:                   0,
			SumOfSquaredDeviation: 82.5,
			BucketCounts:          []int64{0, 10},
		},
		startTime: testStartTime,
	}
	options := cmp.Options{
		cmp.AllowUnexported(distribution{}, distributionOpts{}, DistributionData{}),
		cmpopts.IgnoreFields(distribution{}, "mu"),
		cmp.Comparer(func(x, y float64) bool {
			return math.Abs(x-y) < 0.001
		}),
	}
	if diff := cmp.Diff(want, got, options); diff != "" {
		t.Errorf("distribution returned diff (-want +got):\n%s", diff)
	}
}

func TestDistributionReset(t *testing.T) {
	testCases := []struct {
		desc           string
		existingMetric func() *Distribution
	}{
		{
			desc: "reset a metric with existing streams",
			existingMetric: func() *Distribution {
				metric := NewDistribution(testDistributionOpts)
				if err := metric.WithLabels(testLabels).Record(2.5); err != nil {
					t.Errorf("Record(2.5) returned error: %v, want nil error", err)
				}
				if err := metric.RecordWithoutLabels(1.5); err != nil {
					t.Errorf("RecordWithoutLabels(1.5) returned error: %v, want nil error", err)
				}
				return metric
			},
		},
		{
			desc: "reset a metric with no existing streams",
			existingMetric: func() *Distribution {
				return NewDistribution(testDistributionOpts)
			},
		},
	}
	for _, tc := range testCases {
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
				cmp.AllowUnexported(Distribution{}, distribution{}, distributionOpts{}),
				cmpopts.IgnoreFields(Distribution{}, "mu"),
			}
			wantMetric := NewDistribution(testDistributionOpts)
			if diff := cmp.Diff(wantMetric, existingMetric, diffOpts...); diff != "" {
				t.Errorf("Reset() returned diff (-want, +got):\n%s", diff)
			}

			// Ensure that no TimeSeries would be exported after a metric reset.
			gotTimeSeries := []*mrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(testContainerResource, func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
			})
			if tsCount := len(gotTimeSeries); tsCount > 0 {
				t.Errorf("ExportTimeSeries() returned %v timeseries after a metric was reset, want no timeseries\n", gotTimeSeries)
			}

			// Ensure that a call to Record...() after a metric reset creates a single TimeSeries with a
			// reset start time.
			if err := existingMetric.RecordWithoutLabels(10.0); err != nil {
				t.Errorf("RecordWithoutLabels(10.0) returned error: %v, want nil error", err)
			}
			wantTimeSeries := []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type: testMetricName,
				},
				Resource:   CreateMonitoredResource(testContainerResource),
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{{
					Interval: &cpb.TimeInterval{
						StartTime: tpb.New(fakeStartTime),
						EndTime:   tpb.New(fakeEndTime),
					},
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_DistributionValue{
							DistributionValue: &dpb.Distribution{
								Count:                 1,
								Mean:                  10.0,
								SumOfSquaredDeviation: 0.0,
								BucketCounts:          []int64{0, 1},
								BucketOptions: &dpb.Distribution_BucketOptions{
									Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
										ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
											Bounds: testBucketsUpperBounds,
										},
									},
								},
							},
						},
					},
				}},
			}}
			// Mock the end time, so we have a deterministic output.
			now = func() time.Time { return fakeEndTime }
			gotTimeSeries = []*mrpb.TimeSeries{}
			existingMetric.ExportTimeSeries(testContainerResource, func(ts *mrpb.TimeSeries) {
				gotTimeSeries = append(gotTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
			})
			if diff := cmp.Diff(wantTimeSeries, gotTimeSeries, protocmp.Transform()); diff != "" {
				t.Errorf("ExportTimeSeries() returned diff (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDistributionUnset(t *testing.T) {
	originalNow := now
	t.Cleanup(func() {
		now = originalNow
	})
	oneHourLater := fakeStartTime.Add(time.Hour)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	testcases := []struct {
		desc   string
		metric func() *Distribution
		want   []*mrpb.TimeSeries
	}{
		{
			desc: "Unsetting a metric works",
			metric: func() *Distribution {
				now = func() time.Time { return fakeStartTime }
				metric := NewDistribution(testDistributionOpts)
				must(metric.WithLabels(map[string]string{"label": "value"}).Record(1))
				must(metric.WithLabels(map[string]string{"label 2": "value 2"}).Record(2))
				metric.Unset(map[string]string{"label": "value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label 2": "value 2"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(fakeStartTime),
							EndTime:   tpb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DistributionValue{
								DistributionValue: &dpb.Distribution{
									Count:                 1,
									Mean:                  2,
									SumOfSquaredDeviation: 0,
									BucketCounts:          []int64{0, 1},
									BucketOptions: &dpb.Distribution_BucketOptions{
										Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
												Bounds: testBucketsUpperBounds,
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		},
		{
			desc: "Unsetting a non-existent stream does nothing",
			metric: func() *Distribution {
				now = func() time.Time { return fakeStartTime }
				metric := NewDistribution(testDistributionOpts)
				must(metric.WithLabels(map[string]string{"label": "value"}).Record(1))
				metric.Unset(map[string]string{"label": "not the actual value"})
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(fakeStartTime),
							EndTime:   tpb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DistributionValue{
								DistributionValue: &dpb.Distribution{
									Count:                 1,
									Mean:                  1,
									SumOfSquaredDeviation: 0,
									BucketCounts:          []int64{0, 1},
									BucketOptions: &dpb.Distribution_BucketOptions{
										Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
												Bounds: testBucketsUpperBounds,
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		},
		{
			desc: "Unsetting and re-setting a stream works",
			metric: func() *Distribution {
				now = func() time.Time { return fakeStartTime }
				metric := NewDistribution(testDistributionOpts)
				must(metric.WithLabels(map[string]string{"label": "value"}).Record(1))
				metric.Unset(map[string]string{"label": "value"})
				if err := metric.InitStreamTimestamp(map[string]string{"label": "value"}, oneHourLater); err != nil {
					t.Fatal(err)
				}
				must(metric.WithLabels(map[string]string{"label": "value"}).Record(2))
				return metric
			},
			want: []*mrpb.TimeSeries{{
				Metric: &mpb.Metric{
					Type:   testMetricName,
					Labels: map[string]string{"label": "value"},
				},
				Resource:   CreateMonitoredResource(nodeMonitoringResource),
				MetricKind: mpb.MetricDescriptor_CUMULATIVE,
				ValueType:  mpb.MetricDescriptor_DISTRIBUTION,
				Unit:       testUnit,
				Points: []*mrpb.Point{
					{
						Interval: &cpb.TimeInterval{
							StartTime: tpb.New(oneHourLater),
							EndTime:   tpb.New(fakeEndTime),
						},
						Value: &cpb.TypedValue{
							Value: &cpb.TypedValue_DistributionValue{
								DistributionValue: &dpb.Distribution{
									Count:                 1,
									Mean:                  2,
									SumOfSquaredDeviation: 0,
									BucketCounts:          []int64{0, 1},
									BucketOptions: &dpb.Distribution_BucketOptions{
										Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
												Bounds: testBucketsUpperBounds,
											},
										},
									},
								},
							},
						},
					},
				},
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

func TestValidate(t *testing.T) {
	testCases := []struct {
		desc              string
		bucketUpperBounds []float64
		expectError       bool
	}{
		{
			desc:              "bucketUpperBounds length less than 1",
			bucketUpperBounds: []float64{},
			expectError:       true,
		},
		{
			desc:              "bucketUpperBounds length is greater than 0, not sorted",
			bucketUpperBounds: []float64{5, 2, 3},
			expectError:       true,
		},
		{
			desc:              "bucketUpperBounds length is greater than 0, ascending order",
			bucketUpperBounds: []float64{1, 2, 3},
			expectError:       false,
		},
		{
			desc:              "bucketUpperBounds length is greater than 0, descending order",
			bucketUpperBounds: []float64{3, 2, 1},
			expectError:       true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if _, err := NewDistributionOpts(testDescriptorOpts, tc.bucketUpperBounds).Validate(); (err != nil) != tc.expectError {
				t.Errorf("NewDistributionOpts(_, %v).Validate() = %v, expect error: %v", tc.bucketUpperBounds, err, tc.expectError)
			}
		})
	}
}

func TestIsInfinite(t *testing.T) {
	testCases := []struct {
		desc   string
		value  float64
		expect bool
	}{
		{
			desc:   "negative inf",
			value:  math.Inf(-1),
			expect: true,
		},
		{
			desc:   "positive inf",
			value:  math.Inf(1),
			expect: true,
		},
		{
			desc:   "not a number",
			value:  math.NaN(),
			expect: true,
		},
		{
			desc:   "finite number",
			value:  1,
			expect: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := isInfinite(tc.value); got != tc.expect {
				t.Errorf("isInfinite(%v) = %v, want %v", tc.value, got, tc.expect)
			}
		})
	}
}

func TestFindIndex(t *testing.T) {
	testCases := []struct {
		desc      string
		arr       []float64
		value     float64
		wantIndex int
	}{
		{
			desc:      "value smaller than first entry",
			arr:       []float64{1, 2},
			value:     0,
			wantIndex: 0,
		},
		{
			desc:      "value larger than last entry",
			arr:       []float64{1, 2},
			value:     3,
			wantIndex: 2,
		},
		{
			desc:      "value is same as boundary",
			arr:       []float64{1, 2},
			value:     1,
			wantIndex: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := findIndex(tc.arr, tc.value)
			if got != tc.wantIndex {
				t.Errorf("findIndex(%v, %v) got %v, want %v", tc.arr, tc.value, got, tc.wantIndex)
			}
		})
	}
}

func TestDistributionExportTimeSeriesDoesNotBlockWithLabels(t *testing.T) {
	gotDistribution := NewDistribution(testDistributionOpts)
	exportStarted := make(chan struct{})
	withLabelsDone := make(chan struct{})
	gotDistribution.WithLabels(map[string]string{"label": "value"})

	go gotDistribution.ExportTimeSeries(nodeMonitoringResource, func(ts *mrpb.TimeSeries) {
		close(exportStarted)
		<-withLabelsDone
	})

	<-exportStarted
	go func() {
		// We use a different label to ensure that writer lock is necessary.
		gotDistribution.WithLabels(map[string]string{"new-label": "value"})
		close(withLabelsDone)
	}()

	select {
	case <-withLabelsDone:
	case <-time.After(time.Second):
		t.Errorf("WithLabels() did blocked with labels")
	}
}
