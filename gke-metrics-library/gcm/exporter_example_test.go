package gcm_test

import (
	"context"
	"fmt"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/golang/glog"
	"gke-internal/gke-metrics/gke-metrics-library/gcm"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"
	"go.uber.org/zap"
)

// Example of how to export metrics to Cloud Monitoring/Cloud Monarch
// using the GKE Metrics Exporter.
func Example() {
	ctx := context.TODO()
	gcmClient, err := gcm.CreateClient(ctx, "monitoring.googleapis.com:443" /*cloudMonarchEndpoint*/)
	if err != nil {
		glog.Fatalf("Failed to create a new GCM client")
	}
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("Failed creating logger: %v", err)
		logger = zap.NewNop()
	}
	exampleContainerResource := metrics.NewK8sContainer("12345678" /* ProjectNumber */, "example-location", "example-cluster", "example-namespace", "example-pod", "example-container")
	gcmExporter, err := gcm.NewExporter(ctx, gcm.Options{
		SelfObservabilityResource: exampleContainerResource,
		GCMClient:                 gcmClient,
		Logger:                    logger,
	})
	if err != nil {
		glog.Fatalf("Failed to create an exporter: %v", err)
	}
	// Shutdown the exporter before your program terminates to clean up.
	defer func() {
		if err := gcmExporter.Shutdown(ctx); err != nil {
			// Handle shutdown error here...
		}
	}()

	inputTimeseries := []*mrpb.TimeSeries{ /* ... */ }
	for _, timeseries := range inputTimeseries {
		// ExportTimeSeries will either buffer the timeseries until a GCM batch is
		// full (200 timeseries), or it will call an RPC to GCM when the batch is full.
		if err := gcmExporter.ExportTimeSeries(ctx, timeseries); err != nil {
			// Handle export error here...
		}
	}
	// Always flush the buffer after finishing processing a scrape or before
	// terminating the program. So, all remaining metrics that are in the buffer
	// can be sent to GCM.
	if err := gcmExporter.Flush(ctx); err != nil {
		// Handle flush error here...
	}

}
