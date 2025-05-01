package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// GaugeInt64 is a GAUGE INT64 metric.
type GaugeInt64 struct {
	// Metric metadata (e.g., metric name, unit).
	options DescriptorOpts

	// A map of gaugeInt64 streams. We store one stream per different label key/value combinations.
	// The map's key is a hash of the labels and the value is the corresponding stream for that label
	// combination.
	streams map[uint64]*gaugeInt64

	// A mutex lock for the streams map.
	mu sync.RWMutex
}

// gaugeInt64 stores the state of a Gauge stream. We store one stream per
// (metric_name, metric label key/value) combination.
type gaugeInt64 struct {
	// gaugeInt64 metadata.
	options *DescriptorOpts
	// The stream value itself. It is initialized with 0.
	value atomic.Int64
	// Metric labels key/value for this stream
	labels map[string]string
}

// NewGaugeInt64 creates a new GaugeInt64 metric with options.
func NewGaugeInt64(options DescriptorOpts) *GaugeInt64 {
	return &GaugeInt64{
		options: options,
		streams: make(map[uint64]*gaugeInt64),
	}
}

// SetWithoutLabels sets a value to a metric that has no labels.
// If your metric has labels, this is the wrong function to use.
// Please use (*GaugeInt64).WithLabels() instead.
func (g *GaugeInt64) SetWithoutLabels(value int64) {
	g.WithLabels(nil).Set(value)
}

// AddWithoutLabels adds change to a metric that has no labels.
// If your metric has labels, this is the wrong function to use.
// Please use (*GaugeInt64).WithLabels() instead.
func (g *GaugeInt64) AddWithoutLabels(change int64) {
	g.WithLabels(nil).Add(change)
}

// WithLabels returns a single metric stream with the specified labels.
func (g *GaugeInt64) WithLabels(labels map[string]string) *gaugeInt64 {
	id := hashLabels(labels)

	// A fast path for existing labels.
	g.mu.RLock()
	stream, ok := g.streams[id]
	g.mu.RUnlock()
	if ok {
		return stream
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	stream, ok = g.streams[id]
	if !ok {
		// If the stream (combination
		// of metric name and metric labels names/values) does not exist yet, create a new stream
		// internally with initial stored value 0.
		stream = newGaugeInt64(&g.options, labels)
		g.streams[id] = stream
	}
	return stream
}

// ExportTimeSeries creates a TimeSeries for this metric and it calls export on it.
// TimeSeries is how a metric is represented in the Cloud Monitoring API.
func (g *GaugeInt64) ExportTimeSeries(resource Resource, export func(*mrpb.TimeSeries)) {
	g.mu.RLock()
	var streamsCopy []*gaugeInt64
	for _, stream := range g.streams {
		streamsCopy = append(streamsCopy, stream)
	}
	g.mu.RUnlock()

	for _, stream := range streamsCopy {
		export(stream.createTimeSeries(resource, now()))
	}
}

// newGaugeInt64 creates a gaugeInt64 object.
// It takes a copy of the labels map to ensure they are immutable across the processes' lifetime.
func newGaugeInt64(options *DescriptorOpts, labels map[string]string) *gaugeInt64 {
	return &gaugeInt64{
		options: options,
		labels:  copyLabels(labels),
	}
}

// Set atomically sets the metric value.
func (g *gaugeInt64) Set(value int64) {
	g.value.Store(value)
}

// Add atomically adds change to the existing metric value.
func (g *gaugeInt64) Add(change int64) {
	g.value.Add(change)
}

// Unset clears the value for a metric stream with the given labels.
// To clear the values for all possible labels of the metric, use Reset.
// If the metric has no labels and you want to clear its singular value,
// also use Reset.
func (g *GaugeInt64) Unset(labels map[string]string) {
	id := hashLabels(labels)

	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.streams, id)
}

// Reset removes all metric streams from this metric object.
func (g *GaugeInt64) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.streams = make(map[uint64]*gaugeInt64)
}

// Compact resizes the internal map of streams to match the current active set.
// This is useful in contexts where the number of streams may grow quite large,
// then shrink, and the caller wants to be able to reclaim the space used by the
// stream map.
func (g *GaugeInt64) Compact() {
	g.mu.Lock()
	defer g.mu.Unlock()
	newStreams := make(map[uint64]*gaugeInt64, len(g.streams))
	for id, stream := range g.streams {
		newStreams[id] = stream
	}
	g.streams = newStreams
}

// createTimeSeries returns a TimeSeries that is created from the current state of the metric.
// EndTime is the data collection time.
func (g *gaugeInt64) createTimeSeries(resource Resource, end time.Time) *mrpb.TimeSeries {
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   g.options.Name,
			Labels: g.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Unit:       g.options.Unit,
		Points:     []*mrpb.Point{CreateGaugePoint(CreateInt64TypedValue(g.value.Load()), end)},
	}
}
