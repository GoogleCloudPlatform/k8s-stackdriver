package metrics

import (
	"sync"
	"time"

	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// DeltaInt64 is a type used for creating Delta Int64 metrics.
type DeltaInt64 struct {
	// Metadata of the DeltaInt64.
	opts DescriptorOpts
	// Value of the DeltaInt64, for different label value combinations. The key is a hash of the labels,
	// value is the corresponding deltaInt64 for that label.
	value map[uint64]*deltaInt64
	// A mutex lock for DeltaInt64.
	mu sync.RWMutex
}

type deltaInt64 struct {
	// Metadata of the deltaInt64.
	value int64
	// The labels that the delta value is recording for.
	labels map[string]string
	// startTime is the time at which the metric was initialized or last reset.
	startTime time.Time
}

// NewDeltaInt64 creates a new DeltaInt64 object with options.
func NewDeltaInt64(opts DescriptorOpts) *DeltaInt64 {
	return &DeltaInt64{opts: opts, value: make(map[uint64]*deltaInt64)}
}

// newDeltaInt64 creates a deltaInt64 with labels.
// It takes a copy of the labels to ensure they are immutable.
func newDeltaInt64(value int64, labels map[string]string, startTime time.Time) *deltaInt64 {
	return &deltaInt64{
		value:     value,
		labels:    copyLabels(labels),
		startTime: startTime,
	}
}

// AddWithLabels adds a value to the metric with labels specified.
func (di *DeltaInt64) AddWithLabels(value int64, labels map[string]string) {
	id := hashLabels(labels)
	di.mu.Lock()
	_, ok := di.value[id]
	if !ok {
		deltaInt64 := newDeltaInt64(value, labels, now())
		di.value[id] = deltaInt64
	} else {
		di.value[id].value = value
		di.value[id].startTime = now()
	}
	di.mu.Unlock()
}

// AddWithoutLabels adds a value to the metric with no labels specified.
func (di *DeltaInt64) AddWithoutLabels(value int64) {
	di.AddWithLabels(value, nil)
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Delta metric type and Int64 value type.
func (di *DeltaInt64) ExportTimeSeries(resource Resource, export func(*mgrpb.TimeSeries)) {
	di.mu.RLock()
	var deltasCopy []*deltaInt64
	for _, delta := range di.value {
		deltasCopy = append(deltasCopy, delta)
	}
	di.mu.RUnlock()
	for _, delta := range deltasCopy {
		export(delta.createTimeSeries(di.opts, resource, now()))
	}
}

// createTimeSeries returns a TimeSeries that is created from the current state of the metric.
// EndTime is the data collection time.
func (di *deltaInt64) createTimeSeries(opts DescriptorOpts, resource Resource, end time.Time) *mgrpb.TimeSeries {
	return &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   opts.Name,
			Labels: di.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_DELTA,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Unit:       opts.Unit,
		Points:     []*mgrpb.Point{CreateDeltaPoint(CreateInt64TypedValue(di.value), di.startTime, end)},
	}
}

// Reset removes all metric streams, their values and resets this metric's init time.
func (di *DeltaInt64) Reset() {
	di.mu.Lock()
	defer di.mu.Unlock()
	di.value = make(map[uint64]*deltaInt64)
}

// DeltaDouble is a type used for creating Delta Double metrics.
type DeltaDouble struct {
	// Metadata of the DeltaDouble.
	opts DescriptorOpts
	// Value of the DeltaDouble, for different label value combinations. The key is a hash of the labels,
	// value is the corresponding deltaDouble for that label.
	value map[uint64]*deltaDouble
	// A mutex lock for DeltaDouble.
	mu sync.RWMutex
}

type deltaDouble struct {
	value float64
	// The labels that the delta value is recording for.
	labels map[string]string
	// startTime is the time at which the metric was initialized or last reset.
	startTime time.Time
}

// NewDeltaDouble creates a new DeltaDouble object with options.
func NewDeltaDouble(opts DescriptorOpts) *DeltaDouble {
	return &DeltaDouble{opts: opts, value: make(map[uint64]*deltaDouble)}
}

// newDeltaDouble creates a deltaDouble with labels.
// It takes a copy of the labels to ensure they are immutable.
func newDeltaDouble(value float64, labels map[string]string, startTime time.Time) *deltaDouble {
	return &deltaDouble{
		value:     value,
		labels:    copyLabels(labels),
		startTime: startTime,
	}
}

// AddWithLabels adds a value to the metric with labels specified.
func (dd *DeltaDouble) AddWithLabels(value float64, labels map[string]string) {
	id := hashLabels(labels)
	dd.mu.Lock()
	_, ok := dd.value[id]
	if !ok {
		deltaDouble := newDeltaDouble(value, labels, now())
		dd.value[id] = deltaDouble
	} else {
		dd.value[id].value = value
		dd.value[id].startTime = now()
	}
	dd.mu.Unlock()
}

// AddWithoutLabels adds a value to the metric with no labels specified.
func (dd *DeltaDouble) AddWithoutLabels(value float64) {
	dd.AddWithLabels(value, nil)
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Delta metric type and Double value type.
func (dd *DeltaDouble) ExportTimeSeries(resource Resource, export func(*mgrpb.TimeSeries)) {
	dd.mu.RLock()
	var deltasCopy []*deltaDouble
	for _, delta := range dd.value {
		deltasCopy = append(deltasCopy, delta)
	}
	dd.mu.RUnlock()
	for _, delta := range deltasCopy {
		export(delta.createTimeSeries(dd.opts, resource, now()))
	}
}

// createTimeSeries returns a TimeSeries that is created from the current state of the metric.
// EndTime is the data collection time.
func (dd *deltaDouble) createTimeSeries(opts DescriptorOpts, resource Resource, end time.Time) *mgrpb.TimeSeries {
	return &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   opts.Name,
			Labels: dd.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_DELTA,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Unit:       opts.Unit,
		Points:     []*mgrpb.Point{CreateDeltaPoint(CreateDoubleTypedValue(dd.value), dd.startTime, end)},
	}
}

// Reset removes all metric streams, their values and resets this metric's init time.
func (dd *DeltaDouble) Reset() {
	dd.mu.Lock()
	defer dd.mu.Unlock()
	dd.value = make(map[uint64]*deltaDouble)
}
