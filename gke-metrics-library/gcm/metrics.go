package gcm

import (
	"fmt"

	"gke-internal/gke-metrics/gke-metrics-library/metrics"
)

const (
	// keyMetricType is the key of the metric type.
	keyMetricType = "metric_type"
	// metricTypeApplication is a value of metric type.
	metricTypeApplication = "application"
	// metricTypeSelfObservability is a value of metric type.
	metricTypeSelfObservability = "self_observability"

	// keyGRPCClientStatus is the key of the gRPCF client status.
	keyGRPCClientStatus = "grpc_client_status"

	// keyGRPCClientMethod is the key of the gRPC method used.
	keyGRPCClientMethod = "grpc_client_method"
	// keyTargetName is the key of the target name.
	keyTargetName = "target_name"
	// gRPCMethodCreateServiceTimeSeries is a gRPC method that is used by the exporter.
	gRPCMethodCreateServiceTimeSeries = "google.monitoring.v3.MetricService/CreateServiceTimeSeries"

	timeSeriesExportedMetricName    = "kubernetes.io/internal/metrics_exporter/timeseries_exported"
	maxObservedBufferSizeMetricName = "kubernetes.io/internal/metrics_exporter/max_observed_buffer_size"
	completedGRPCsMetricName        = "kubernetes.io/internal/metrics_exporter/completed_grpcs"
	grpcRequestLatencyMetricName    = "kubernetes.io/internal/metrics_exporter/grpc_request_latency"
)

var (
	defaultMillisecondsDistribution = []float64{0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000}
)

// LINT.IfChange(self_observability_metrics_names)
var (
	defaultSelfObservabilityMetricNames = SelfObservabilityMetricNameOverride{
		TimeSeriesExportedMetricName:    timeSeriesExportedMetricName,
		MaxObservedBufferSizeMetricName: maxObservedBufferSizeMetricName,
		CompletedGRPCsMetricName:        completedGRPCsMetricName,
		GrpcRequestLatencyMetricName:    grpcRequestLatencyMetricName,
	}
)

// LINT.ThenChange(
// :self_observability_metrics,
// :init_self_observability_metric,
// )

// SelfObservabilityMetricNameOverride defines custom metric names for each self observability metric
// Here are some sample metric names:
// tpu.googleapis.com/internal/metrics_exporter/max_observed_buffer_size
// kubernetes.io/internal/metrics_exporter/max_observed_buffer_size
type SelfObservabilityMetricNameOverride struct {
	TimeSeriesExportedMetricName    string
	GrpcRequestLatencyMetricName    string
	CompletedGRPCsMetricName        string
	MaxObservedBufferSizeMetricName string
}

// LINT.IfChange(self_observability_metrics)

// selfObservabilityMetrics contains all self observability metrics of the exporter.
type selfObservabilityMetrics struct {
	timeSeriesExported    *metrics.CumulativeInt64
	completedGRPCs        *metrics.CumulativeInt64
	maxObservedBufferSize *metrics.GaugeInt64
	grpcRequestLatency    *metrics.Distribution
}

// LINT.ThenChange(
// :self_observability_metrics_names,
// :init_self_observability_metric,
// )

// LINT.IfChange(init_self_observability_metric)
// This function must be called before starting startSelfObservability go routine in export.go.
// initSelfObservabilityMetrics initializes all self observability metrics.
func (e *exporter) initSelfObservabilityMetrics(overrideNames *SelfObservabilityMetricNameOverride) {
	names := defaultSelfObservabilityMetricNames
	if overrideNames != nil {
		e.logger.Info(fmt.Sprintf("SelfObservabilityMetricNameOverrides set to %v, updating self observability metric names.", *overrideNames))
		names = *overrideNames
	}
	e.selfObservabilityMetrics.timeSeriesExported = newTimeSeriesExported(names.TimeSeriesExportedMetricName)
	e.selfObservabilityMetrics.maxObservedBufferSize = newMaxObservedBufferSize(names.MaxObservedBufferSizeMetricName)
	e.selfObservabilityMetrics.grpcRequestLatency = newGRPCRequestLatency(names.GrpcRequestLatencyMetricName)
	e.selfObservabilityMetrics.completedGRPCs = newCompletedGRPCs(names.CompletedGRPCsMetricName)
}

// LINT.ThenChange(
// :self_observability_metrics,
// :self_observability_metrics_names,
// )

func newTimeSeriesExported(name string) *metrics.CumulativeInt64 {
	return metrics.NewCumulativeInt64(metrics.DescriptorOpts{
		Name:        name,
		Description: "Total number of TimeSeries exported to Cloud Monitoring.",
		Unit:        "1",
		Labels: []metrics.LabelDescriptor{
			{
				Name:        keyMetricType,
				Description: "The metric type that indicates if the exported timeseries are from application metrics or self observability metrics. Value must be one of application or self-observability.",
			},
			{
				Name:        keyGRPCClientStatus,
				Description: "The gRPC status of the exported timeseries.",
			},
			{
				Name:        keyGRPCClientMethod,
				Description: "gRPC method used.",
			},
			{
				Name:        keyTargetName,
				Description: "The target name of the exported timeseries.",
			},
		},
	})
}

func newMaxObservedBufferSize(name string) *metrics.GaugeInt64 {
	return metrics.NewGaugeInt64(metrics.DescriptorOpts{
		Name:        name,
		Description: "Maximum number of buffered TimeSeries.",
		Unit:        "1",
		Labels: []metrics.LabelDescriptor{
			{
				Name:        keyTargetName,
				Description: "The target name of the exported timeseries.",
			},
		},
	})
}

func newCompletedGRPCs(name string) *metrics.CumulativeInt64 {
	return metrics.NewCumulativeInt64(metrics.DescriptorOpts{
		Name:        name,
		Description: "Total number of gRPC requests sent to Cloud Monitoring.",
		Unit:        "1",
		Labels: []metrics.LabelDescriptor{
			{
				Name:        keyMetricType,
				Description: "The metric type that indicates if the gRPC requests contain metrics from application metrics or self observability metrics. Value must be one of application or self-observability.",
			},
			{
				Name:        keyGRPCClientStatus,
				Description: "The gRPC status of the requests.",
			},
			{
				Name:        keyGRPCClientMethod,
				Description: "gRPC method used.",
			},
			{
				Name:        keyTargetName,
				Description: "The target name of the exported timeseries.",
			},
		},
	})
}

// TODO(b/273327457): export grpc_client_status.
func newGRPCRequestLatency(name string) *metrics.Distribution {
	return metrics.NewDistribution(metrics.NewDistributionOpts(metrics.DescriptorOpts{
		Name:        name,
		Description: "Distribution of sent messages latency per gRPC, by method.",
		Unit:        "ms",
		Labels: []metrics.LabelDescriptor{
			{
				Name:        keyGRPCClientMethod,
				Description: "gRPC method used.",
			},
			{
				Name:        keyTargetName,
				Description: "The target name of the exported timeseries.",
			},
		},
	}, defaultMillisecondsDistribution).MustValidate())
}
