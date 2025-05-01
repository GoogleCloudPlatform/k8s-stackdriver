package metrics

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// GaugeDouble is a GAUGE DOUBLE metric.
type GaugeDouble struct {
	// Metric metadata (e.g., metric name, unit).
	options DescriptorOpts

	// A map of gaugeDouble streams. We store one stream per different label key/value combinations.
	// The map's key is a hash of the labels and the value is the corresponding stream for that label
	// combination.
	streams map[uint64]*gaugeDouble

	// A mutex lock for the streams map.
	mu sync.RWMutex
}

// gaugeDouble stores the state of a Gauge stream. We store one stream per
// (metric_name, metric label key/value) combination.
type gaugeDouble struct {
	// valBits contains the bits of the represented float64 value.
	// It must go first in the struct to guarantee alignment for atomic operations.
	// http://golang.org/pkg/sync/atomic/#pkg-note-BUG
	valueBits uint64

	// gaugeDouble metadata.
	options *DescriptorOpts

	// Metric labels key/value for this stream
	labels map[string]string
}

// NewGaugeDouble creates a new GaugeDouble metric with options.
func NewGaugeDouble(options DescriptorOpts) *GaugeDouble {
	return &GaugeDouble{
		options: options,
		streams: make(map[uint64]*gaugeDouble),
	}
}

// SetWithoutLabels sets a value to a metric that has no labels.
// If your metric has labels, this is the wrong function to use.
// Please use (*GaugeDouble).WithLabels() instead.
func (g *GaugeDouble) SetWithoutLabels(value float64) {
	g.WithLabels(nil).Set(value)
}

// WithLabels returns a single metric stream with the specified labels.
func (g *GaugeDouble) WithLabels(labels map[string]string) *gaugeDouble {
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
		// If the stream (combination of metric name and metric labels names/values) does not exist yet,
		// this function also creates a new stream internally with initial stored value 0.
		stream = newGaugeDouble(&g.options, labels)
		g.streams[id] = stream
	}
	return stream
}

// ExportTimeSeries creates a TimeSeries for this metric and it calls export on it.
// TimeSeries is how a metric is represented in the Cloud Monitoring API.
func (g *GaugeDouble) ExportTimeSeries(resource Resource, export func(*mrpb.TimeSeries)) {
	g.mu.RLock()
	var streamsCopy []*gaugeDouble
	for _, stream := range g.streams {
		streamsCopy = append(streamsCopy, stream)
	}
	g.mu.RUnlock()

	for _, stream := range streamsCopy {
		export(stream.createTimeSeries(resource, now()))
	}
}

// newGaugeDouble creates a gaugeDouble object.
// It takes a copy of the labels map to ensure they are immutable across the processes' lifetime.
func newGaugeDouble(options *DescriptorOpts, labels map[string]string) *gaugeDouble {
	return &gaugeDouble{
		options: options,
		labels:  copyLabels(labels),
	}
}

// Set atomically sets the metric value.
func (g *gaugeDouble) Set(value float64) {
	// sync/atomic does not have a float type, so we store the float64 value as bits.
	atomic.StoreUint64(&g.valueBits, math.Float64bits(value))
}

// Unset clears the value for a metric stream with the given labels.
// To clear the values for all possible labels of the metric, use Reset.
// If the metric has no labels and you want to clear its singular value,
// also use Reset.
func (g *GaugeDouble) Unset(labels map[string]string) {
	id := hashLabels(labels)

	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.streams, id)
}

// Reset removes all metric streams from this metric object.
func (g *GaugeDouble) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.streams = make(map[uint64]*gaugeDouble)
}

// Compact resizes the internal map of streams to match the current active set.
// This is useful in contexts where the number of streams may grow quite large,
// then shrink, and the caller wants to be able to reclaim the space used by the
// stream map.
func (g *GaugeDouble) Compact() {
	g.mu.Lock()
	defer g.mu.Unlock()
	newStreams := make(map[uint64]*gaugeDouble, len(g.streams))
	for id, stream := range g.streams {
		newStreams[id] = stream
	}
	g.streams = newStreams
}

// createTimeSeries returns a TimeSeries that is created from the current state of the metric.
// EndTime is the data collection time.
func (g *gaugeDouble) createTimeSeries(resource Resource, end time.Time) *mrpb.TimeSeries {
	// sync/atomic does not have a float type, so we store and retrieve the float64 value as bits.
	value := math.Float64frombits(atomic.LoadUint64(&g.valueBits))

	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   g.options.Name,
			Labels: g.labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Unit:       g.options.Unit,
		Points:     []*mrpb.Point{CreateGaugePoint(CreateDoubleTypedValue(value), end)},
	}
}
