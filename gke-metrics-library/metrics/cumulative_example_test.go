// package metrics_example provides examples of using the metrics
package metrics_test

import (
	"context"

	"gke-internal/gke-metrics/gke-metrics-library/gcm"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"go.uber.org/zap"
)

// ExampleCumulativeInt64 examplifies how to create, increment and export INT64 CUMULATIVE metrics.
func ExampleCumulativeInt64() {
	k8sContainerResource := metrics.NewK8sContainer(
		"123", /* Project Number */
		"example-location",
		"example-cluster",
		"example-namespace",
		"example-pod",
		"example-container")

	// Example of metrics with labels.
	myMetricWithLabels := metrics.NewCumulativeInt64(metrics.DescriptorOpts{
		Name:        "kubernetes.io/internal/addons/<component_name>/<metric_name>",
		Description: "Some description.",
		Unit:        "some unit",
		Labels:      []metrics.LabelDescriptor{{Name: "label"}, {Name: "label2"}},
	})
	// Update value for the metric with different label key-value combinations.
	myMetricWithLabels.WithLabels(map[string]string{"label": "value", "label2": "value2"}).Add(1)
	myMetricWithLabels.WithLabels(map[string]string{"label": "value", "label2": "value2"}).Add(2)

	myMetricWithLabels.WithLabels(map[string]string{"label": "value", "label2": "value3"}).Add(3)
	myMetricWithLabels.WithLabels(map[string]string{"label": "value", "label2": "value3"}).Add(4)

	// Example of metrics without labels.
	myMetricWithoutLabels := metrics.NewCumulativeInt64(metrics.DescriptorOpts{
		Name:        "kubernetes.io/internal/addons/<component_name>/<metric_name>",
		Description: "Some description.",
		Unit:        "some unit",
		Labels:      []metrics.LabelDescriptor{},
	})
	myMetricWithoutLabels.AddWithoutLabels(1)
	myMetricWithoutLabels.AddWithoutLabels(2)

	// Create a new GCM exporter. This will export metrics data to Cloud Monitoring.
	ctx := context.TODO()
	gcmClient, err := gcm.CreateClient(ctx, "monitoring.googleapis.com:443")
	if err != nil {
		// error handling
	}
	gcmExporter, err := gcm.NewExporter(ctx, gcm.Options{
		SelfObservabilityResource: k8sContainerResource,
		GCMClient:                 gcmClient,
		Logger:                    zap.NewNop(),
	})
	if err != nil {
		// error handling
	}

	exportFn := func(ts *mrpb.TimeSeries) {
		if err := gcmExporter.ExportTimeSeries(ctx, ts); err != nil {
			// error handling
		}
	}

	// Export metrics to Cloud Monitoring.
	myMetricWithLabels.ExportTimeSeries(k8sContainerResource, exportFn)
	myMetricWithoutLabels.ExportTimeSeries(k8sContainerResource, exportFn)
}
