// Package periodicexporter provides methods to export GKE Metrics Library metrics periodically.
// Caution: this exporter is not thread safe!
package periodicexporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"gke-internal/gke-metrics/gke-metrics-library/context/detach"
	"gke-internal/gke-metrics/gke-metrics-library/logging"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"
	"go.uber.org/zap"
)

// Exporter exports metrics to Cloud Monitoring
type Exporter struct {
	// gcmExporter is an exporter of metrics timeseries to Cloud Monitoring.
	gcmExporter GCMExporter
	// Cancel function to stop the periodic exporter goroutine.
	cancel context.CancelFunc
	// Mutex lock used to avoid data races when:
	//   - Context gets cancelled while the goroutine is exporting metrics.
	mu sync.Mutex
	// Metrics to export to Cloud Monitoring.
	metricsToExport []*MetricAndMonitoredResource
	// How often it should export metrics to Cloud Monitoring.
	exportInterval time.Duration
	// Rate limited logger used by the exporter
	logger *zap.Logger
}

// Options to create a new exporter.
type Options struct {
	// GCMExporter is an exporter of metrics timeseries to Cloud Monitoring.
	GCMExporter GCMExporter
	// What metrics to export to Cloud Monitoring.
	MetricsToExport []*MetricAndMonitoredResource
	// Default monitored resource (metric schema) of the metrics to export to Cloud Monitoring
	// (e.g., k8s_container). This field is required even if all your metrics are defined using
	// custom schemas and this is enforced during the exporter creation.
	DefaultMonitoredResource *metrics.Resource
	// How often the exporter should send metrics to Cloud Monitoring. 1 minute by default.
	ExportInterval time.Duration
	// Logger used by the exporter
	Logger *zap.Logger
}

// GCMExporter is an interface to a GCM exporter within the context of GKE Metrics Library.
type GCMExporter interface {
	ExportTimeSeries(context.Context, *mrpb.TimeSeries) error
	Flush(context.Context) error
	Shutdown(context.Context) error
}

// GKEMetric is a GKE Metrics metric.
type GKEMetric interface {
	ExportTimeSeries(metrics.Resource, func(*mrpb.TimeSeries))
}

// MetricAndMonitoredResource is a tuple of a GKE metrics and its monitored resource.
type MetricAndMonitoredResource struct {
	metric            GKEMetric
	monitoredResource *metrics.Resource
}

// Metric creates a new MetricAndMonitoredResource without specifying a custom monitored resource.
// This is useful if you set a default monitored resource when creating the periodic exporter.
func Metric(metric GKEMetric) *MetricAndMonitoredResource {
	return MetricWithMonitoredResource(metric, nil)
}

// MetricWithMonitoredResource creates a new MetricAndMonitoredResource with a custom monitored
// resource.
func MetricWithMonitoredResource(metric GKEMetric, monitoredResource *metrics.Resource) *MetricAndMonitoredResource {
	return &MetricAndMonitoredResource{
		metric:            metric,
		monitoredResource: monitoredResource,
	}
}

// New creates a new exporter to periodically send metrics to Cloud Monitoring.
func New(opts Options) (*Exporter, error) {
	if err := validateOptions(&opts); err != nil {
		return nil, err
	}
	return &Exporter{
		gcmExporter:     opts.GCMExporter,
		exportInterval:  opts.ExportInterval,
		metricsToExport: opts.MetricsToExport,
		logger:          logging.RateLimitedLogger(opts.Logger, 10*opts.ExportInterval, 2),
	}, nil
}

func validateOptions(opts *Options) error {
	if opts.DefaultMonitoredResource == nil {
		return fmt.Errorf("defaultMonitoredResource must be set")
	}
	if opts.Logger == nil {
		return fmt.Errorf("logger must be set")
	}
	if opts.ExportInterval == 0 {
		opts.ExportInterval = time.Minute
	}

	for _, m := range opts.MetricsToExport {
		if m == nil || m.metric == nil {
			return fmt.Errorf("a metric must not be nil")
		}
		if m.monitoredResource == nil {
			m.monitoredResource = opts.DefaultMonitoredResource
		}
	}

	return nil
}

// Start starts the periodic metrics exporter as a new goroutine. Please call Shutdown()
// to stop this goroutine and export the last batch of metrics.
func (e *Exporter) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		cancel()
	}
	go e.periodicExporter(ctx)
}

func (e *Exporter) periodicExporter(ctx context.Context) {
	tick := time.NewTicker(e.exportInterval)
	defer tick.Stop()
	e.logger.Info("Starting periodic metrics exporter...")
	for {
		select {
		case <-ctx.Done():
			e.exportAll(ctx, true)
			return
		case <-tick.C:
			// Add a lock to prevent Shutdown being called during an export request.
			// Without this lock, exportAll() will fail with context cancelled error
			// and the error logs may be confusing to the users of this library.
			e.mu.Lock()
			// There might be a case that the ticker ticked and got blocked, because the lock is locked
			// by Shutdown. Shutdown then cancels the context and unlocks the lock.
			// Next, this function will try to export metrics with an already cancelled context.
			// To avoid this case, we do an error check on the context and return if ctx.Done is closed
			// (e.g., because ctx is cancelled).
			if err := ctx.Err(); err != nil {
				e.exportAll(ctx, true)
				e.mu.Unlock()
				return
			}

			e.exportAll(ctx, false)
			e.mu.Unlock()
		}
	}
}

func (e *Exporter) exportAll(ctx context.Context, ignoreContextCancellation bool) {
	if ignoreContextCancellation {
		// Create a detached context to make sure that any remaining operation
		// can be closed/shutdown correctly even when the parent context is cancelled.
		ctx = detach.IgnoreCancel(ctx)
	}
	exportFunction := func(ts *mrpb.TimeSeries) {
		if err := e.gcmExporter.ExportTimeSeries(ctx, ts); err != nil {
			e.logger.Error("Failed to export metrics to Cloud Monitoring", zap.Error(err))
		}
	}
	for _, m := range e.metricsToExport {
		m.metric.ExportTimeSeries(*m.monitoredResource, exportFunction)
	}
	// Always flush so we don't risk sending duplicate timeseries on the same batch,
	// which is not allowed by GCM.
	if err := e.gcmExporter.Flush(ctx); err != nil {
		e.logger.Error("Failed to flush metrics to Cloud Monitoring", zap.Error(err))
	}
}

// Shutdown stops the goroutine for exporting metrics periodically and the
// underlying GCM exporter.
func (e *Exporter) Shutdown(ctx context.Context) {
	// Create a detached context to make sure that any remaining operation
	// can be closed/shutdown correctly even when the parent context is cancelled.
	ctx = detach.IgnoreCancel(ctx)

	e.cancel()
	if err := e.gcmExporter.Shutdown(ctx); err != nil {
		e.logger.Error("Failed to shutdown GCM exporter", zap.Error(err))
	}
}
