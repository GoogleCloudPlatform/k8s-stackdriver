package metrics

import (
	"time"

	mgrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
)

// StatelessGaugeInt64 is a type used for creating stateless Gauge Int64 metrics.
type StatelessGaugeInt64 struct {
	opts DescriptorOpts
}

// NewStatelessGaugeInt64 creates a new stateless GaugeInt64 object with opts.
func NewStatelessGaugeInt64(opts DescriptorOpts) *StatelessGaugeInt64 {
	return &StatelessGaugeInt64{opts: opts}
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Gauge metric type and Int64 value type.
func (sgi *StatelessGaugeInt64) ExportTimeSeries(resource Resource, value int64, labels map[string]string, endTime time.Time, export func(*mgrpb.TimeSeries)) {
	point := CreateGaugePoint(CreateInt64TypedValue(value), endTime)
	ts := &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   sgi.opts.Name,
			Labels: labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Unit:       sgi.opts.Unit,
		Points:     []*mgrpb.Point{point},
	}
	export(ts)
}

// StatelessGaugeDouble is a type used for creating stateless Gauge Double metrics.
type StatelessGaugeDouble struct {
	opts DescriptorOpts
}

// NewStatelessGaugeDouble creates a new GaugeDouble object with opts.
func NewStatelessGaugeDouble(opts DescriptorOpts) *StatelessGaugeDouble {
	return &StatelessGaugeDouble{opts: opts}
}

// ExportTimeSeries creates a TimeSeries from the data provided, with Gauge metric type and Double value type.
func (sgd *StatelessGaugeDouble) ExportTimeSeries(resource Resource, value float64, labels map[string]string, endTime time.Time, export func(*mgrpb.TimeSeries)) {
	point := CreateGaugePoint(CreateDoubleTypedValue(value), endTime)
	ts := &mgrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   sgd.opts.Name,
			Labels: labels,
		},
		Resource:   CreateMonitoredResource(resource),
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Unit:       sgd.opts.Unit,
		Points:     []*mgrpb.Point{point},
	}
	export(ts)
}
