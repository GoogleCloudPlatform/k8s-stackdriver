package metrics

import (
	"time"

	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// StatelessCumulativeInt64 is a type used for creating stateless Cumulative Int64 metrics.
type StatelessCumulativeInt64 struct {
	opts DescriptorOpts
}

// NewStatelessCumulativeInt64 creates a new StatelessCumulativeInt64 object with options.
func NewStatelessCumulativeInt64(opts DescriptorOpts) *StatelessCumulativeInt64 {
	return &StatelessCumulativeInt64{opts: opts}
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Cumulative metric type and Int64 value type.
func (sci *StatelessCumulativeInt64) ExportTimeSeries(resource Resource, value int64, labels map[string]string, startTime, endTime time.Time, export func(*mgrpb.TimeSeries)) {
	point := CreateCumulativePoint(CreateInt64TypedValue(value), startTime, endTime)
	ts := &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   sci.opts.Name,
			Labels: labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Unit:       sci.opts.Unit,
		Points:     []*mgrpb.Point{point},
	}
	export(ts)
}

// StatelessCumulativeDouble is a type used for creating stateless Cumulative Double metrics.
type StatelessCumulativeDouble struct {
	opts DescriptorOpts
}

// NewStatelessCumulativeDouble creates a new stateless CumulativeDouble object with opts.
func NewStatelessCumulativeDouble(opts DescriptorOpts) *StatelessCumulativeDouble {
	return &StatelessCumulativeDouble{opts: opts}
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Cumulative metric type and Double value type.
func (scd *StatelessCumulativeDouble) ExportTimeSeries(resource Resource, value float64, labels map[string]string, startTime, endTime time.Time, export func(*mgrpb.TimeSeries)) {
	point := CreateCumulativePoint(CreateDoubleTypedValue(value), startTime, endTime)
	ts := &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   scd.opts.Name,
			Labels: labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Unit:       scd.opts.Unit,
		Points:     []*mgrpb.Point{point},
	}
	export(ts)
}
