// Package gcm exports timeseries to GCM.
package gcm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/hashicorp/go-multierror"
	"gke-internal/gke-metrics/gke-metrics-library/context/detach"
	"gke-internal/gke-metrics/gke-metrics-library/logging"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"
	"go.uber.org/zap"
	mpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	maxBatchSize                           = 200
	defaultSelfObservabilityExportInterval = 60 * time.Second
)

// Buffer of timeseries.
type buffer struct {
	// Buffer of timeseries.
	timeseries []*mrpb.TimeSeries
	// Indicates if the CUMULATIVE timeseries in this buffer are ready to export. This is true by
	// default.
	// This is needed for cases like a Prometheus scraper:
	//   * Prometheus cumulative metrics do not include a metric start time with each cumulative
	//     timeseries. However, a start time is needed for all cumulative metrics exported to Cloud
	//     Monitoring.
	//
	//     Prometheus, however, exports a process_start_time_seconds metric that can be
	//     used to infer the start time of the rest of the cumulative metrics.
	//
	//     The problem here is that process_start_time_seconds may appear too late in the Prometheus
	//     endpoint, after we had already buffered many cumulative timeseries.
	//
	//     That's why, when this exporter is used to alongside with the GKE Metrics Collector
	//     Prometheus scraper, we initially set timeSeriesAreReady to false, until we receive a
	//     process_start_time_seconds metric and update the start_time of all cumulative timeseries
	//     in the buffer.
	timeSeriesAreReady bool
}

// readyToBeExported indicates if the timeseries in this buffer are ready to be
// exported.
func (b *buffer) readyToBeExported() bool {
	if b == nil {
		return false
	}
	return b.timeSeriesAreReady && len(b.timeseries) >= maxBatchSize
}

// cumulativeTimeSeriesAreReady sets the buffer's timeSeriesAreReady flag.
func (b *buffer) cumulativeTimeSeriesAreReady(ready bool) {
	if b == nil {
		return
	}
	b.timeSeriesAreReady = ready
}

// exporter provides an exporter that exports timeseries to GCM.
type exporter struct {
	// Default requestName when exporting requests for application metrics. In the format of: "projects/" + project-number-string.
	// The project-number-string identifies one of the following:
	// - A Google Cloud project
	// - A Google Cloud project that is also a scoping project of a metrics scope
	// See more details: https://cloud.google.com/monitoring/api/v3?&_ga=2.201114030.-1640876989.1690385179#project_name
	requestName string

	// Client of GCM exporter for application metrics.
	gcmClient timeSeriesCreator

	// Request name used to export self-observability metrics to GCM.
	// In the format of: "projects/" + project-number-string.
	// It is always inferred from the self-observability monitored resource.
	requestNameForSelfObservabilityMetrics string

	// Client of GCM exporter for self observability metrics.
	// If a custom client for self-observability metrics is not provided,
	// it defaults to the application metrics client, which must always be provided.
	gcmClientForSelfObservabilityMetrics timeSeriesCreator

	// selfObservabilityExportInterval is the self observability export interval.
	// The default is 60 seconds.
	selfObservabilityExportInterval time.Duration

	// selfObservabilityResource is the monitored resource information for self observability metrics.
	selfObservabilityResource metrics.Resource

	// selfObservabilityMetrics contains all the self-observability metrics of the exporter.
	selfObservabilityMetrics selfObservabilityMetrics

	// additionalSelfObservabilityMetrics are extra self-observability metrics defined by the
	// library caller.
	additionalSelfObservabilityMetrics []GKEMetric

	// maxBufferSizeSeen is the maximum (biggest) size of the buffer the process has ever seen.
	maxBufferSizeSeen atomic.Int64

	// cancel is the context cancel function to stop the goroutine that exports self-observability
	// metrics to Cloud Monitoring.
	cancel context.CancelFunc

	// A mutual exclusion lock used for self-observability exporting goroutine.
	// It avoids the cases when:
	//   - Context got cancelled during exporting the self-observability metrics.
	//   - In tests, race condition happens when the self-observability goroutine isn't closed properly before a new testcase starts.
	mu sync.Mutex

	// Rate limited logger used by the exporter.
	logger *zap.Logger

	// metricsExportTimeout is the maximum duration of a gRPC to Cloud Monitoring.
	metricsExportTimeout time.Duration

	// If splitGaugeBuffer is true, the exporter will buffer GAUGE timeseries in a
	// buffer that is separate from the CUMULATIVE metrics.
	splitGaugeBuffer bool

	// targetName of the metrics being exported. This is only used in the self-observability metrics.
	targetName string

	// A mutual exclusion lock used for buffers.
	bufferMu sync.Mutex
	// Buffer of timeseries (may include both ready and not ready timeseries).
	//
	// If splitGaugeBuffer is set to false, this buffer will store
	// all timeseries to be exported.
	//
	// If splitGaugeBuffer is set to true, this buffer will only store CUMULATIVE metrics.
	buffer *buffer
	// Buffer of gauge timeseries (always ready timeseries, because GAUGE metrics do not
	// set a start timestamp).
	// This will only be populated if splitGaugeBuffer is set to true.
	gaugeBuffer *buffer
}

// timeSeriesCreator provides an easily testable translation to the cloud monitoring API.
// TODO(b/216286974): implement support to CreateTimeSeries
type timeSeriesCreator interface {
	// implement the method: http://google3/google/monitoring/v3/metric_service.proto;l=265;rcl=496435335
	CreateServiceTimeSeries(ctx context.Context, in *monitoringpb.CreateTimeSeriesRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

// GKEMetric is a GKE Metrics metric.
type GKEMetric interface {
	ExportTimeSeries(metrics.Resource, func(*mrpb.TimeSeries))
}

// Options for configuring the exporter.
type Options struct {
	// Client used to send requests to GCM.
	// When GCMClientForSelfObservabilityMetrics is specified,
	// GCMClient is used only for application metrics.
	GCMClient timeSeriesCreator

	// Optional.
	// Project number which GCMClient send metrics for.
	// It is only required if you specify a custom GCM client to send self-observability
	// metrics (e.g., GCMClientForSelfObservabilityMetrics).
	// If unspecified and you do not set GCMClientForSelfObservabilityMetrics,
	// ProjectNumber will be inferred from SelfObservabilityResource's "project_id".
	ProjectNumber string

	// Optional. Scrape interval for self observability metrics, default to 60 seconds.
	SelfObservabilityExportInterval time.Duration

	// Logger used by the exporter to log more detailed errors that may happen
	// when exporting data to Cloud Monitoring. It also logs metrics about
	// writes to Cloud Monitoring, being a backup debugging tool when Cloud Monitoring
	// is unavailable.
	Logger *zap.Logger

	// MetricsExportTimeout is the maximum duration of a gRPC to Cloud Monitoring. Defaults to 45s.
	MetricsExportTimeout time.Duration

	// If SplitGaugeBuffer is true, the exporter will buffer GAUGE timeseries in a
	// buffer that is separate from the CUMULATIVE metrics.
	SplitGaugeBuffer bool

	// Monitored resource information for self observability metrics.
	SelfObservabilityResource metrics.Resource

	// AdditionalSelfObservabilityMetrics lets you define more self-observability metrics to be
	// exported to Cloud Monitoring through this exporter.
	// NOTE: this is meant for the GKE Metrics team use. If you need to use this functionality,
	// please contact g/gke-metrics first.
	AdditionalSelfObservabilityMetrics []GKEMetric

	// Optional.
	// Client used to send requests to GCM for self observability metrics of the exporter.
	// When unspecified, it will use GCMClient by default.
	// NOTE: this is meant for Clustermetrics use. Clustermetrics sends application metrics with
	// project_id of the consumer project, and self-observability metrics with the project_id of the
	// hosted master project.
	GCMClientForSelfObservabilityMetrics timeSeriesCreator

	// Optional.
	// Overrides the Name attribute of each of the internal (self-observability) metrics used in
	// google3/cloud/kubernetes/metrics/common/gcm/metrics.go. This is useful when this library
	// runs outside of GKE and the metrics use a prefix different from kubernetes.io, e.g.,
	// when using automation instead of the legacy way for metrics intregration where
	// aliases allowlisting is not possible.
	SelfObservabilityMetricNameOverrides *SelfObservabilityMetricNameOverride

	// Optional.
	// Disable observability metrics. Provides the option for disabling self observability
	// metrics if the user does not want them.
	DisableSelfObservabilityMetrics bool

	// Optional.
	// TargetName of the metrics being exported. This is only used in the self-observability metrics.
	TargetName string
}

// NewExporter initializes an exporter.
// Shutdown must be called before the program terminates in order to send out the remaining metrics
// and clean up goroutines.
func NewExporter(ctx context.Context, options Options) (*exporter, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	// TODO(b/277103006): always require a ProjectNumber provided instead of being inferred.
	projectNumber := options.ProjectNumber
	if projectNumber == "" {
		projectNumber = options.SelfObservabilityResource.ProjectNumber()
	}
	e := &exporter{
		buffer: &buffer{
			timeseries:         make([]*mrpb.TimeSeries, 0, maxBatchSize),
			timeSeriesAreReady: true,
		},
		requestName:                            "projects/" + projectNumber,
		gcmClient:                              options.GCMClient,
		requestNameForSelfObservabilityMetrics: "projects/" + options.SelfObservabilityResource.ProjectNumber(),
		gcmClientForSelfObservabilityMetrics:   options.GCMClient,
		selfObservabilityResource:              options.SelfObservabilityResource,
		selfObservabilityExportInterval:        options.SelfObservabilityExportInterval,
		additionalSelfObservabilityMetrics:     options.AdditionalSelfObservabilityMetrics,
		logger:                                 logging.RateLimitedLogger(options.Logger, 10*options.SelfObservabilityExportInterval, 2),
		metricsExportTimeout:                   options.MetricsExportTimeout,
		splitGaugeBuffer:                       options.SplitGaugeBuffer,
		targetName:                             options.TargetName,
	}

	if options.GCMClientForSelfObservabilityMetrics != nil {
		e.gcmClientForSelfObservabilityMetrics = options.GCMClientForSelfObservabilityMetrics
	}

	if e.splitGaugeBuffer {
		e.gaugeBuffer = &buffer{
			timeseries:         make([]*mrpb.TimeSeries, 0, maxBatchSize),
			timeSeriesAreReady: true,
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		cancel()
	}

	e.initSelfObservabilityMetrics(options.SelfObservabilityMetricNameOverrides)

	if !options.DisableSelfObservabilityMetrics {
		go e.startSelfObservability(ctx)
	} else {
		e.logger.Warn("DisableSelfObservabilityMetrics enabled so not exporting self obesrvabilty metrics.")
	}

	return e, nil
}

// Validate validates if the Options object is valid.
func (o *Options) validate() error {
	if o.SelfObservabilityResource.ProjectNumber() == "" {
		return fmt.Errorf("ProjectNumber in SelfObservabilityResource is empty")
	}
	if o.GCMClient == nil {
		return fmt.Errorf("GCMClient is nil")
	}
	// TODO(b/277103006): always required a ProjectNumber regardless
	// GCMClientForSelfObservabilityMetrics is specified or not.
	if o.GCMClientForSelfObservabilityMetrics != nil && o.ProjectNumber == "" {
		return fmt.Errorf("please specify ProjectNumber: using a custom Cloud Monitoring client to write self-observability metrics, but did not set ProjectNumber option")
	}
	if o.SelfObservabilityExportInterval == 0 {
		o.SelfObservabilityExportInterval = defaultSelfObservabilityExportInterval
	}
	if o.Logger == nil {
		fmt.Println("Exporter logger was not provided. It will not write out log or errors")
		o.Logger = zap.NewNop()
	}
	if o.MetricsExportTimeout == 0 {
		o.MetricsExportTimeout = 45 * time.Second
	}
	return nil
}

// AddAdditionalSelfObservabilityMetrics adds additional metrics to be exported by this exporter as
// self-observability metric type.
func (e *exporter) AddAdditionalSelfObservabilityMetrics(metrics []GKEMetric) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.additionalSelfObservabilityMetrics = append(e.additionalSelfObservabilityMetrics, metrics...)
}

// ExportTimeSeries receives one timeseries a time, buffers the timeseries and exports the buffer if
// the buffer meets the requirements.
// When exporting the requests, one fail request will not fail all the following. For example, if the
// first batch in the buffer fails, the exporter exports on the next batch instead of failing all the
// timeseries/batches in the buffer.
func (e *exporter) ExportTimeSeries(ctx context.Context, ts *mrpb.TimeSeries) error {
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()
	if ts == nil {
		return fmt.Errorf("nil timeseries")
	}

	if ts.GetMetricKind() == mpb.MetricDescriptor_GAUGE && e.splitGaugeBuffer {
		return e.exportTimeSeries(ctx, ts, e.gaugeBuffer)
	}
	return e.exportTimeSeries(ctx, ts, e.buffer)
}

// ExportTimeSeries receives one timeseries a time, buffers the timeseries and exports the buffer if
// the buffer meets the requirements.
// When exporting the requests, one fail request will not fail all the following. For example, if the
// first batch in the buffer fails, the exporter exports on the next batch instead of failing all the
// timeseries/batches in the buffer.
func (e *exporter) exportTimeSeries(ctx context.Context, ts *mrpb.TimeSeries, buffer *buffer) error {
	if ts == nil {
		return fmt.Errorf("cannot export a nil timeseries")
	}
	if buffer == nil {
		return fmt.Errorf("cannot store metrics in a nil buffer")
	}

	buffer.timeseries = append(buffer.timeseries, ts)

	currentBufferSize := int64(len(buffer.timeseries))
	if currentBufferSize > e.maxBufferSizeSeen.Load() {
		e.maxBufferSizeSeen.Store(currentBufferSize)
		e.selfObservabilityMetrics.maxObservedBufferSize.WithLabels(map[string]string{
			keyTargetName: e.targetName,
		}).Set(e.maxBufferSizeSeen.Load())
	}
	if buffer.readyToBeExported() {
		return e.exportBuffer(ctx, buffer, false /* alsoExportIncompleteBatches */)
	}
	return nil
}

// Flush exports all the timeseries in buffer.
// This should always be called at the end of one export, to flush any non-exported metric.
func (e *exporter) Flush(ctx context.Context) error {
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()
	var errs error
	if e.splitGaugeBuffer {
		if err := e.flush(ctx, e.gaugeBuffer); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	if err := e.flush(ctx, e.buffer); err != nil {
		errs = multierror.Append(errs, err)
	}
	return errs
}

// flush exports all the timeseries in buffer.
// This should always be called at the end of one export, to flush any non-exported metric.
func (e *exporter) flush(ctx context.Context, buffer *buffer) error {
	if buffer == nil || len(buffer.timeseries) == 0 {
		return nil
	}
	// Always clear the buffer after a flush.
	defer func() { buffer.timeseries = nil }()
	return e.exportBuffer(ctx, buffer, true /* alsoExportIncompleteBatches */)
}

// Shutdown exports all the existing metrics in buffer, self observability metrics and stops the
// goroutine for collecting self observability metrics.
func (e *exporter) Shutdown(ctx context.Context) error {
	// Create a detached context to make sure that any remaining operation
	// can be closed/shutdown correctly even when the parent context is cancelled.
	ctx = detach.IgnoreCancel(ctx)
	// Stop the self observability collection goroutine.
	e.cancel()
	var errs error
	if err := e.Flush(ctx); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to flush: %q", err))
	}
	if err := e.exportSelfObservability(ctx); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to export self observability: %q", err))
	}
	return errs
}

// CumulativeTimeseriesAreReady refreshes the flag that indicates if CUMULATIVE metrics are ready
// to be exported.
func (e *exporter) CumulativeTimeseriesAreReady(timeseriesAreReady bool) {
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()
	e.buffer.cumulativeTimeSeriesAreReady(timeseriesAreReady)
}

// exportBuffer exports the buffered TimeSeries to Cloud Monitoring in batches of maxBatchSize.
// If alsoExportIncompleteBatches is set, it will also export any remaining timeseries that did not
// fit into the previous maxBatchSize requests in a subsequent batch.
func (e *exporter) exportBuffer(ctx context.Context, buffer *buffer, alsoExportIncompleteBatches bool) error {
	if !buffer.timeSeriesAreReady {
		return fmt.Errorf("aborting export to Cloud Monitoring: buffer contains timeseries that are not ready to be exported")
	}

	failedRequestCount := 0
	totalRequestCount := len(buffer.timeseries) / maxBatchSize
	if len(buffer.timeseries)%maxBatchSize > 0 && alsoExportIncompleteBatches {
		// This will ensure we flush the remainder timeseries in the buffer that were not exported in
		// full batches of maxBatchSize.
		totalRequestCount++
	}
	for i := 0; i < totalRequestCount; i++ {
		batchSize := maxBatchSize
		if len(buffer.timeseries) < maxBatchSize {
			batchSize = len(buffer.timeseries)
		}
		if err := e.exportToGCM(ctx, buffer.timeseries[0:batchSize], false /*isSelfObservability*/); err != nil {
			// We do not return if we fail to export a batch. Each batch is independent, so
			// we will still try to export the rest of the TimeSeries in the buffer.
			failedRequestCount++
			e.logger.Error("Failed to export metrics to Cloud Monitoring", zap.Error(err))
		}
		buffer.timeseries = buffer.timeseries[batchSize:]
	}

	e.logger.Debug("Finished exporting metrics to Cloud Monitoring",
		zap.Int("total_requests", totalRequestCount),
		zap.Int("failed_requests", failedRequestCount))
	if failedRequestCount > 0 {
		return fmt.Errorf("failed to export %d (out of %d) batches of metrics to Cloud Monitoring", failedRequestCount, totalRequestCount)
	}
	return nil
}

func (e *exporter) exportToGCM(ctx context.Context, batch []*mrpb.TimeSeries, isSelfObservability bool) error {
	batchSize := len(batch)
	if batchSize == 0 {
		return nil
	}
	if batchSize > maxBatchSize {
		return fmt.Errorf("more than %d timeseries in a request, got %d", maxBatchSize, batchSize)
	}

	var req *monitoringpb.CreateTimeSeriesRequest
	gcmClient := e.gcmClient
	if isSelfObservability {
		req = &monitoringpb.CreateTimeSeriesRequest{Name: e.requestNameForSelfObservabilityMetrics, TimeSeries: batch}
		gcmClient = e.gcmClientForSelfObservabilityMetrics
	} else {
		req = &monitoringpb.CreateTimeSeriesRequest{Name: e.requestName, TimeSeries: batch}
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, e.metricsExportTimeout)
	defer cancel() // release resources of the context

	beforeReq := time.Now()
	_, err := gcmClient.CreateServiceTimeSeries(ctxWithTimeout, req)

	e.addValueToObservabilityMetrics(isSelfObservability, int64(batchSize), err, float64(time.Since(beforeReq).Seconds()*1000.0))
	return err
}

func (e *exporter) startSelfObservability(ctx context.Context) {
	ticker := time.NewTicker(e.selfObservabilityExportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Add a lock to prevent Shutdown being called during an export self observability metric
			// request. Without the lock, exportSelfObservability() will fail with context cancelled error
			// and the error logs may be confusing to the users of this library.
			e.mu.Lock()
			// There might be a case that the ticker ticked, but the lock is locked by Shutdown.
			// Shutdown then cancelled the context and unlocked the lock.
			// Next, we'll try to export self observability metrics in this function with a cancelled context.
			// To avoid this case, we do an error check on the context and return if ctx.Done is closed
			// (e.g., because ctx is cancelled).
			if err := ctx.Err(); err != nil {
				e.logger.Info("Context returned an error, stop exporting self-observability metrics. This is expected during shutdowns", zap.Error(err))
				e.mu.Unlock()
				return
			}
			if err := e.exportSelfObservability(ctx); err != nil {
				e.logger.Error("Failed to export self-observability metrics to Cloud Monitoring", zap.Error(err))
			}
			e.mu.Unlock()
		}
	}
}

func (e *exporter) exportSelfObservability(ctx context.Context) error {
	buffer := []*mrpb.TimeSeries{}

	// Buffer self-observability up to maxBatchSize timeseries and push them right away to Cloud Monitoring.
	// Self-observability metrics are always "ready" to be exported, since they have all fields
	// filled at this point.
	exportFunction := func(ts *mrpb.TimeSeries) {
		buffer = append(buffer, ts)
		if len(buffer) != maxBatchSize {
			return
		}
		if err := e.exportToGCM(ctx, buffer, true /* isSelfObservability */); err != nil {
			e.logger.Error("Failed to export self-observability metrics to Cloud Monitoring", zap.Error(err))
		}
		buffer = nil
	}
	e.selfObservabilityMetrics.timeSeriesExported.ExportTimeSeries(e.selfObservabilityResource, exportFunction)
	e.selfObservabilityMetrics.maxObservedBufferSize.ExportTimeSeries(e.selfObservabilityResource, exportFunction)
	e.selfObservabilityMetrics.grpcRequestLatency.ExportTimeSeries(e.selfObservabilityResource, exportFunction)
	e.selfObservabilityMetrics.completedGRPCs.ExportTimeSeries(e.selfObservabilityResource, exportFunction)

	for _, m := range e.additionalSelfObservabilityMetrics {
		m.ExportTimeSeries(e.selfObservabilityResource, exportFunction)
	}

	// This ensures that all remaining metrics in the buffer are flushed, if any.
	return e.exportToGCM(ctx, buffer, true /* isSelfObservability */)
}

func (e *exporter) addValueToObservabilityMetrics(isSelfObservability bool, batchSize int64, gcmReturnsErr error, latencyMilliSec float64) {
	if err := e.selfObservabilityMetrics.grpcRequestLatency.WithLabels(map[string]string{
		keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
		keyTargetName:       e.targetName,
	}).Record(latencyMilliSec); err != nil {
		e.logger.Error("Failed to record gRPC request latency self-observability metric", zap.Error(err))
	}

	gRPCStatusCode := status.Convert(gcmReturnsErr).Code().String()
	metricType := metricTypeApplication
	if isSelfObservability {
		metricType = metricTypeSelfObservability
	}

	e.selfObservabilityMetrics.timeSeriesExported.WithLabels(map[string]string{
		keyMetricType:       metricType,
		keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
		keyGRPCClientStatus: gRPCStatusCode,
		keyTargetName:       e.targetName,
	}).Add(batchSize)
	e.selfObservabilityMetrics.completedGRPCs.WithLabels(map[string]string{
		keyMetricType:       metricType,
		keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
		keyGRPCClientStatus: gRPCStatusCode,
		keyTargetName:       e.targetName,
	}).Add(1)
}
