package metrics

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	distributionpb "google.golang.org/genproto/googleapis/api/distribution"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// Example usage is included in distribution_test.go

const (
	labelSeparator = "/"
)

// Distribution stores distribution (histogram) metrics.
// Internally, we store a distribution for each combination of labels.
type Distribution struct {
	// metadata of the Distribution
	options distributionOpts
	// A map of distributions for different label value combinations. The key is a hash of the labels,
	// value is the corresponding distribution for that label.
	streams map[uint64]*distribution
	// initTime is the time at which the metric was initialized or last reset.
	// Used as initial "start time" for streams.
	initTime time.Time
	// A mutual exclusion lock for distributions map.
	mu sync.RWMutex
}

// distribution contains distribution metric information for a specific combination of labels.
type distribution struct {
	// A mutual exclusion lock for distributions.
	mu      sync.RWMutex
	options distributionOpts
	// the distribution data
	data DistributionData
	// The labels that the distribution data is recording for. This field is copied when it is passed
	// in and never changes.
	labels map[string]string
	// Start time of this distribution stream.
	startTime time.Time
}

// DistributionData is the underlying data for a single distribution stream.
type DistributionData struct {
	// Count is the number of data points in total.
	// The sum of all values in `bucketCount` should be equal to `Count`.
	Count int64
	// Mean of the data points
	Mean float64
	// Sum of squared deviation of the data points
	SumOfSquaredDeviation float64
	// Min is the minimum sample recorded in this distribution.
	Min float64
	// Max is the maximum sample recorded in this distribution.
	Max float64
	// BucketCounts is the number of points that lie in each bucket.
	// It is of size `len(distributionOpts.bucketsUpperBounds)+1`.
	// The value of samples in bucket index `i` has upper bound
	// `distributionOpts.bucketsUpperBounds[i]` and lower bound
	// `distributionOpts.bucketsUpperBounds[i-1]`.
	// The first bucket (index 0) has no lower bound (i.e. -infinity).
	// The last bucket (at index `len(distributionOpts.bucketsUpperBounds)`)
	// has no upper bound (i.e. +infinity).
	BucketCounts []int64
}

// distributionOpts contains the metadata for distribution metrics.
type distributionOpts struct {
	// descriptor information
	descriptorOpts DescriptorOpts
	// bucketsUpperBounds is the upper bounds for the buckets. Values must be in ascending order. This
	// field is copied when it is passed in and never changes.
	bucketsUpperBounds []float64
}

// incompleteDistributionOpts contains the distributionOpts that needs to be validated. Users need
// to call either Validate() or MustValidate() to obtain a validated distributionOpts.
type incompleteDistributionOpts struct {
	// The distributionOpts that need to be validated.
	distributionOpts
}

// NewDistribution creates a new Distribution.
func NewDistribution(options distributionOpts) *Distribution {
	return &Distribution{
		options:  options,
		streams:  make(map[uint64]*distribution),
		initTime: now(),
	}
}

// RecordWithoutLabels records a value for histograms with no labels. This will return an error if the input
// value is not finite (inf, -inf, NaN), or if recording the value will cause the mean or standard deviation to overflow.
func (d *Distribution) RecordWithoutLabels(value float64) error {
	return d.WithLabels(nil).Record(value)
}

// WithLabels returns the Distribution that has the corresponding labels.
func (d *Distribution) WithLabels(labels map[string]string) *distribution {
	id := hashLabels(labels)

	// A fast path for existing labels.
	d.mu.RLock()
	distribution, ok := d.streams[id]
	d.mu.RUnlock()
	if ok {
		return distribution
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	distribution, ok = d.streams[id]
	if !ok {
		distribution = newDistribution(d.options, labels, d.initTime)
		d.streams[id] = distribution
	}
	return distribution
}

// InitStreamTimestamp initializes a single metric stream identified by the specified labels if it
// doesn't exist, and sets its timestamp to the given time.
// Returns an error if the metric already has a stream for the specified labels.
// This method allows customization of the start time. If you do not need to
// customize the stream timestamp, you do not need to call this function.
// If you need to clear a metric stream with a specific set of labels, use Unset.
// After calling Unset, if you need to recreate a metric stream with the
// same labels, you *must* call InitStreamTimestamp with the new start time.
// Otherwise, Monarch will perceive the distribution count as decreasing.
func (d *Distribution) InitStreamTimestamp(labels map[string]string, startTime time.Time) error {
	id := hashLabels(labels)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, found := d.streams[id]; found {
		return fmt.Errorf("metric stream with labels %v already exists", labels)
	}
	d.streams[id] = newDistribution(d.options, labels, startTime)
	return nil
}

// ExportTimeSeries exports TimeSeries for each combination of labels from current data.
func (d *Distribution) ExportTimeSeries(resource Resource, export func(*mgrpb.TimeSeries)) {
	d.mu.RLock()
	var streamsCopy []*distribution
	for _, distribution := range d.streams {
		streamsCopy = append(streamsCopy, distribution)
	}
	d.mu.RUnlock()

	for _, distribution := range streamsCopy {
		export(distribution.createTimeSeries(resource, now()))
	}
}

// Unset clears the value for a metric stream with the given labels.
// To clear the values for all possible labels of the metric, use Reset.
// If the metric has no labels and you want to clear its singular value,
// also use Reset.
// After calling Unset, if you need to recreate a metric stream with the
// same labels, you *must* call InitStreamTimestamp with the new start time.
// Otherwise, Monarch will perceive the distribution count as decreasing.
func (d *Distribution) Unset(labels map[string]string) {
	id := hashLabels(labels)

	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.streams, id)
}

// Reset will remove all distribution streams and reset the start time.
func (d *Distribution) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.streams = make(map[uint64]*distribution)
	d.initTime = now()
}

// Compact resizes the internal map of streams to match the current active set.
// This is useful in contexts where the number of streams may grow quite large,
// then shrink, and the caller wants to be able to reclaim the space used by the
// stream map.
func (d *Distribution) Compact() {
	d.mu.Lock()
	defer d.mu.Unlock()
	newStreams := make(map[uint64]*distribution, len(d.streams))
	for id, stream := range d.streams {
		newStreams[id] = stream
	}
	d.streams = newStreams
}

// Record records the data point to the Distribution. This function returns error if input value is
// not finite (inf, -inf, NaN), or recording the value will cause mean or standard deviation to overflow.
func (d *distribution) Record(value float64) error {
	if isInfinite(value) {
		return fmt.Errorf("%v is not finite", value)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.data.record(value, d.options.bucketsUpperBounds)
}

// Set sets all the data relevant to the Distribution.
// Most users should use Record instead, and let this library handle tracking
// the distribution data.
// Set is only useful for the case where the distribution data is already
// gathered and aggregated within an external source or other library, and
// that data needs to be reflected within this library.
// For all other uses, use Record.
func (d *distribution) Set(data *DistributionData) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.data = *data
	// Make a copy of the bucket counts so that the caller cannot modify what
	// this library sees.
	d.data.BucketCounts = copyInt64Slice(data.BucketCounts)
}

// NewDistributionOpts returns an incompleteDistributionOpts object that needs to be validated.
// Users either call Validate() or MustValidate() to obtain the validated distributionOpts object.
func NewDistributionOpts(descriptorOpts DescriptorOpts, bucketsUpperBounds []float64) *incompleteDistributionOpts {
	bucketsUpperBoundsCopy := make([]float64, len(bucketsUpperBounds))
	for i, v := range bucketsUpperBounds {
		bucketsUpperBoundsCopy[i] = v
	}
	return &incompleteDistributionOpts{distributionOpts: distributionOpts{descriptorOpts: descriptorOpts, bucketsUpperBounds: bucketsUpperBoundsCopy}}
}

// Validate validates if the incompleteDistributionOpts has correct parameters.
func (i *incompleteDistributionOpts) Validate() (distributionOpts, error) {
	if len(i.bucketsUpperBounds) < 1 {
		return distributionOpts{}, fmt.Errorf("length of upper bounds should be at least 1, got %d", len(i.bucketsUpperBounds))
	}
	if !sort.Float64sAreSorted(i.bucketsUpperBounds) {
		return distributionOpts{}, fmt.Errorf("upper bounds must be sorted in ascending order, got %v", i.bucketsUpperBounds)
	}
	return i.distributionOpts, nil
}

// MustValidate returns a distributionOpts if incompleteDistributionOpts is valid, or else it will
// panic.
func (i *incompleteDistributionOpts) MustValidate() distributionOpts {
	distributionOpts, err := i.Validate()
	if err != nil {
		panic(err)
	}
	return distributionOpts
}

// newDistribution creates a new distribution object that contains the actual distribution data.
func newDistribution(options distributionOpts, labels map[string]string, startTime time.Time) *distribution {
	return &distribution{
		options: options,
		labels:  copyLabels(labels),
		data: DistributionData{
			Min:          math.Inf(1),
			Max:          math.Inf(-1),
			BucketCounts: make([]int64, len(options.bucketsUpperBounds)+1),
		},
		startTime: startTime,
	}
}

// createTimeSeries creates TimeSeries from current data.
// start is the monitored resource start time. For example, this metric monitors kubernetes
// container X, then start is the container start time of X.
// EndTime is the data collection time.
func (d *distribution) createTimeSeries(resource Resource, end time.Time) *mgrpb.TimeSeries {
	d.mu.RLock()
	defer d.mu.RUnlock()
	point := CreateCumulativePoint(d.createDistributionTypedValue(), d.startTime, end)
	return &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   d.options.descriptorOpts.Name,
			Labels: d.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
		Unit:       d.options.descriptorOpts.Unit,
		Points:     []*mgrpb.Point{point},
	}
}

func (d *distribution) createDistributionTypedValue() *cpb.TypedValue {
	value := &distributionpb.Distribution{
		Count:                 d.data.Count,
		Mean:                  d.data.Mean,
		SumOfSquaredDeviation: d.data.SumOfSquaredDeviation,
		// BucketCounts need to be copied when creating TimeSeries because it is a slice and can be
		// updated later.
		BucketCounts: copyInt64Slice(d.data.BucketCounts),
		BucketOptions: &distributionpb.Distribution_BucketOptions{
			Options: &distributionpb.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distributionpb.Distribution_BucketOptions_Explicit{
					Bounds: d.options.bucketsUpperBounds,
				},
			},
		},
	}
	return &cpb.TypedValue{
		Value: &cpb.TypedValue_DistributionValue{
			DistributionValue: value,
		},
	}
}

func (dd *DistributionData) record(value float64, bucketbucketsUpperBounds []float64) error {
	newCount := dd.Count + 1
	dev := value - dd.Mean
	newMean := dd.Mean + dev/float64(newCount)
	newSSD := dd.SumOfSquaredDeviation + dev*(value-newMean)

	if isInfinite(newMean) || isInfinite(newSSD) {
		return fmt.Errorf("%.3g would overflow, mean: %v, sum of squared deviation: %v", value, newMean, newSSD)
	}

	dd.Count = newCount
	dd.Mean = newMean
	dd.SumOfSquaredDeviation = newSSD

	if dd.Min > value {
		dd.Min = value
	}
	if dd.Max < value {
		dd.Max = value
	}

	i := findIndex(bucketbucketsUpperBounds, value)
	dd.BucketCounts[i]++
	return nil
}

func isInfinite(f float64) bool {
	return math.IsInf(f, -1) || math.IsInf(f, 1) || math.IsNaN(f)
}

func findIndex(bucketsUpperBounds []float64, value float64) int {
	return sort.Search(len(bucketsUpperBounds), func(i int) bool { return bucketsUpperBounds[i] > value })
}

func copyInt64Slice(in []int64) []int64 {
	out := make([]int64, len(in))
	copy(out, in)
	return out
}
