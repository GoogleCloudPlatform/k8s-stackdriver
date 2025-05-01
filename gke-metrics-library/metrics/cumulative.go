package metrics

import (
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

var (
	// now is set globally to help mock in tests.
	// now is used for the cumulative start time.
	now = time.Now
)

// CumulativeInt64 is a type used for creating stateful Cumulative Int64 metrics.
type CumulativeInt64 struct {
	// Metadata of the CumulativeInt64.
	options DescriptorOpts
	// A map of stateful cumulativeInt64 for different label value combinations. The key is a hash of the labels,
	// value is the corresponding cumulativeInt64 for that label.
	cumulatives map[uint64]*cumulativeInt64
	// initTime is the time at which the metric was initialized or last reset.
	// Used as initial "start time" for cumulatives.
	initTime time.Time
	// A mutex lock for the cumulatives map.
	mu sync.RWMutex
}

// cumulativeInt64 is a type used to store stateful cumulative data.
type cumulativeInt64 struct {
	// Metadata of the cumulativeInt64.
	options *DescriptorOpts
	// Value of the cumulativeInt64, starts from 0.
	value atomic.Int64
	// The labels that the cumulative value is recording for.
	labels map[string]string
	// Start time of the cumulative stream.
	startTime time.Time
}

// NewCumulativeInt64 creates a new CumulativeInt64 object with options.
func NewCumulativeInt64(options DescriptorOpts) *CumulativeInt64 {
	return &CumulativeInt64{
		options:     options,
		cumulatives: make(map[uint64]*cumulativeInt64),
		initTime:    now(),
	}
}

// WithLabels returns a single metric with the specified labels. If the metric doesn't exist yet,
// it creates a metric with initial value 0 assumed to have a start time matching that when this
// top-level metric was initialized.
func (ci *CumulativeInt64) WithLabels(labels map[string]string) *cumulativeInt64 {
	id := hashLabels(labels)

	// A fast path for existing labels.
	ci.mu.RLock()
	counterInt64, ok := ci.cumulatives[id]
	ci.mu.RUnlock()
	if ok {
		return counterInt64
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()
	counterInt64, ok = ci.cumulatives[id]
	if !ok {
		counterInt64 = newCumulativeInt64(&ci.options, labels, ci.initTime)
		ci.cumulatives[id] = counterInt64
	}
	return counterInt64
}

// InitStreamTimestamp initializes a single metric stream identified by the specified labels if it
// doesn't exist, and sets its timestamp to the given time.
// Returns an error if the metric already has a stream for the specified labels.
// This method allows customization of the start time. If you do not need to
// customize the stream timestamp, you do not need to call this function.
// If you need to clear a metric stream with a specific set of labels, use Unset.
// After calling Unset, if you need to recreate a metric stream with the
// same labels, you *must* call InitStreamTimestamp with the new start time.
// Otherwise, Monarch will perceive the counter as decreasing.
func (ci *CumulativeInt64) InitStreamTimestamp(labels map[string]string, startTime time.Time) error {
	id := hashLabels(labels)
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if _, found := ci.cumulatives[id]; found {
		return fmt.Errorf("metric stream with labels %v already exists", labels)
	}
	ci.cumulatives[id] = newCumulativeInt64(&ci.options, labels, startTime)
	return nil
}

// AddWithoutLabels adds a value to the metric with no labels specified, and return the new value.
// A Cumulative metric must be monotonically non-decreasing and the add value should be non-negative.
// If your metric's definition has labels, this is the wrong function to use, please see (*CumulativeInt64).WithLabels().
func (ci *CumulativeInt64) AddWithoutLabels(value int64) int64 {
	return ci.WithLabels(nil).Add(value)
}

// ExportTimeSeries creates a TimeSeries from the data recorded for this metric, with Cumulative metric type and Int64 value type.
func (ci *CumulativeInt64) ExportTimeSeries(resource Resource, export func(*mrpb.TimeSeries)) {
	ci.mu.RLock()
	var cumulativesCopy []*cumulativeInt64
	for _, cumulative := range ci.cumulatives {
		cumulativesCopy = append(cumulativesCopy, cumulative)
	}
	ci.mu.RUnlock()

	for _, cumulative := range cumulativesCopy {
		export(cumulative.createTimeSeries(resource, now()))
	}
}

// newCumulativeInt64 creates a cumulativeInt64 with labels.
// It takes a copy of the labels to ensure they are immutable.
func newCumulativeInt64(options *DescriptorOpts, labels map[string]string, startTime time.Time) *cumulativeInt64 {
	return &cumulativeInt64{
		options:   options,
		labels:    copyLabels(labels),
		startTime: startTime,
	}
}

// Add adds value atomically to the metric and return the new value.
// A Cumulative metric must be monotonically non-decreasing, thus the add value must be non-negative.
// TODO(b/286271112): replace logging errors with returning errors
func (ci *cumulativeInt64) Add(value int64) int64 {
	if value < 0 {
		fmt.Fprintf(os.Stderr, "Cannot add negative value %d to a Cumulative metric (%s): cumulative metrics must be monotonically non-decreasing.", value, ci.options.Name)
		return ci.value.Load()
	}
	return ci.value.Add(value)
}

// Set sets the value of the stream atomically.
// Most users should use Add instead. Set is only useful in the case where the
// counter is already being tracked externally and the caller wants to copy
// its value into this library.
// A Cumulative metric must be monotonically non-decreasing, thus any call to
// Set must have a value larger or equal to the previous call to Set.
// If the counter resets from zero, call Unset prior to calling Set.
// TODO(b/286271112): replace logging errors with returning errors
func (ci *cumulativeInt64) Set(value int64) {
	if value < 0 {
		fmt.Fprintf(os.Stderr, "Cannot add negative value %d to a Cumulative metric (%s): cumulative metrics must be monotonically non-decreasing.", value, ci.options.Name)
		return
	}
	oldValue := int64(math.MinInt64) // This value guarantees that the first CaS fails.
	for !ci.value.CompareAndSwap(oldValue, value) {
		oldValue = ci.value.Load()
		if value < oldValue {
			fmt.Fprintf(os.Stderr, "Cannot reduce cumulative metric (tried to set %s from %d to %d): values must be monotonically non-decreasing.", ci.options.Name, oldValue, value)
			return
		}
	}
}

// createTimeSeries returns a TimeSeries that is created from the current state of the metric.
// EndTime is the data collection time.
func (ci *cumulativeInt64) createTimeSeries(resource Resource, end time.Time) *mrpb.TimeSeries {
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   ci.options.Name,
			Labels: ci.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Unit:       ci.options.Unit,
		Points:     []*mrpb.Point{CreateCumulativePoint(CreateInt64TypedValue(ci.value.Load()), ci.startTime, end)},
	}
}

func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		// Avoid allocating an empty map on the heap if there are no labels.
		return nil
	}
	copy := make(map[string]string, len(labels))
	for k, v := range labels {
		copy[k] = v
	}
	return copy
}

// Unset clears the value for a metric stream with the given labels.
// To clear the values for all possible labels of the metric, use Reset.
// If the metric has no labels and you want to clear its singular value,
// also use Reset.
// After calling Unset, if you need to recreate a metric stream with the
// same labels, you *must* call InitStreamTimestamp with the new start time.
// Otherwise, Monarch will perceive the counter as decreasing.
func (ci *CumulativeInt64) Unset(labels map[string]string) {
	id := hashLabels(labels)

	ci.mu.Lock()
	defer ci.mu.Unlock()
	delete(ci.cumulatives, id)
}

// Reset removes all metric streams, their values and resets this metric's init time.
func (ci *CumulativeInt64) Reset() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.cumulatives = make(map[uint64]*cumulativeInt64)
	ci.initTime = now()
}

// Compact resizes the internal map of streams to match the current active set.
// This is useful in contexts where the number of streams may grow quite large,
// then shrink, and the caller wants to be able to reclaim the space used by the
// stream map.
func (ci *CumulativeInt64) Compact() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	newCumulatives := make(map[uint64]*cumulativeInt64, len(ci.cumulatives))
	for id, cumulative := range ci.cumulatives {
		newCumulatives[id] = cumulative
	}
	ci.cumulatives = newCumulatives
}
