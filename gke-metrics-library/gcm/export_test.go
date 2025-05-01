package gcm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"gke-internal/gke-metrics/gke-metrics-library/context/detach"
	"gke-internal/gke-metrics/gke-metrics-library/logging"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	cpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	logtest "gke-internal/gke-metrics/gke-metrics-library/test/logging"
	dpb "google.golang.org/genproto/googleapis/api/distribution"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// Example of how to instantiate and use the exporter with TimeSeries that depend on the Prometheus
// process_start_time_seconds (for example, CUMULATIVE metrics scraped from a Prometheus endpoint).
func Example_prometheusExporter() {
	ctx := context.TODO()
	gcmClient, err := CreateClient(ctx, "monitoring.googleapis.com:443" /*cloudMonarchEndpoint*/)
	if err != nil {
		glog.Fatalf("Failed to create a new GCM client")
	}
	exampleContainerResource := metrics.NewK8sContainer("12345678" /* ProjectNumber */, "example-location", "example-cluster", "example-namespace", "example-pod", "example-container")
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("Failed creating logger: %v", err)
		logger = zap.NewNop()
	}
	gcmExporter, err := NewExporter(ctx, Options{
		SelfObservabilityResource: exampleContainerResource,
		GCMClient:                 gcmClient,
		Logger:                    logger,
	})
	if err != nil {
		glog.Fatalf("Failed to create an exporter: %v", err)
	}

	// Shutdown the exporter before your program terminates to clean up.
	// The Shutdown function should run with valid context even if parent context is done.
	// http://go/gotip/episodes/31
	defer func() {
		if err := gcmExporter.Shutdown(detach.IgnoreCancel(ctx)); err != nil {
			// Handle shutdown error here...
		}
	}()

	// Set TimeseriesAreReady to false to avoid flushing the buffer to GCM when
	// a batch (200 timeseries) is complete.
	// This is useful if your CUMULATIVE timeseries don't contain all necessary information
	// to be sent to GCM. For example, some the input CUMULATIVE timeseries may not contain
	// the start_time field set, since this value depend on the process_start_time_seconds metric.
	// In this example, we set TimeseriesAreReady to false until we see the process_start_time_seconds
	// metric in the input and update the CUMULATIVE timeseries with their start_time.
	gcmExporter.CumulativeTimeseriesAreReady(false)

	inputTimeseries := []*mrpb.TimeSeries{cumulativeTimeseries1}
	for _, timeseries := range inputTimeseries {
		// ExportTimeSeries will either buffer the timeseries until a GCM batch is
		// full (200 timeseries), or it will call an RPC to GCM when the batch is full.
		if err := gcmExporter.ExportTimeSeries(ctx, timeseries); err != nil {
			// Handle export error here...
		}
		if timeseries.GetMetric().GetType() == "process_start_time_seconds" {
			// Update the start_time of CUMULATIVE timeseries
			// Then set TimeseriesAreReady to true to instruct the exporter to send timeseries to GCM
			// when a batch is complete.
			gcmExporter.CumulativeTimeseriesAreReady(true)
		}
	}
	// Always flush the buffer after finishing processing a scrape or before
	// terminating the program. So, all remaining metrics that are in the buffer
	// can be sent to GCM.
	if err := gcmExporter.Flush(ctx); err != nil {
		// Handle flush error here...
	}
	// If your input start_time depends on the process_start_time_seconds metric,
	// reset your TimeseriesAreReady to false at the end of every scrape
	// So, you don't send metrics without the start_time.
	gcmExporter.CumulativeTimeseriesAreReady(false)
}

var (
	testContainerResource = metrics.NewK8sContainer("12345678", "us-central1-c", "fake-cluster", "kube-system", "fake-pod", "fake-container")
	fakeResource          = metrics.CreateMonitoredResource(testContainerResource)
	errTestFailedRequest  = fmt.Errorf("failed request")
	testStartTime         = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	testEndTime           = time.Date(2021, time.January, 2, 0, 0, 0, 0, time.UTC)
	testRequestName       = "projects/12345678"
	testTargetName        = "test-target"
	gaugeTimeseries1      = &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Labels: map[string]string{"type": "drop"},
			Type:   "gauge_metric_1",
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		Resource:   fakeResource,
		Points: []*mrpb.Point{{
			Interval: &cpb.TimeInterval{
				EndTime: timestamppb.New(time.Unix(1644263053, 0)),
			},
			Value: &cpb.TypedValue{
				Value: &cpb.TypedValue_DoubleValue{DoubleValue: 195},
			},
		}},
	}
	cumulativeTimeseries1 = &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Labels: map[string]string{"code": "201"},
			Type:   "cumulative_metric_1",
		},
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		Resource:   fakeResource,
		Points: []*mrpb.Point{{
			Interval: &cpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: 1644263050},
				EndTime:   &timestamppb.Timestamp{Seconds: 1644263053},
			},
			Value: &cpb.TypedValue{
				Value: &cpb.TypedValue_DoubleValue{DoubleValue: 1},
			},
		}},
	}
)

func TestExportTimeSeries(t *testing.T) {
	testcases := []struct {
		desc                         string
		splitGaugeBufferOption       bool
		cumulativeTimeseriesAreReady bool
		timeSeriesToExport           []*mrpb.TimeSeries
		wantBufferAfterExport        []*mrpb.TimeSeries
		wantGaugeBufferAfterExport   []*mrpb.TimeSeries
		wantReqs                     []*monitoringpb.CreateTimeSeriesRequest
		wantLogLines                 []logtest.Line
	}{
		{
			desc:                         "dont export: small buffer",
			cumulativeTimeseriesAreReady: true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize-2), cumulativeTimeseries1),
			wantBufferAfterExport:        append(createBuffer(gaugeTimeseries1, maxBatchSize-2), cumulativeTimeseries1),
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs:                     nil,
		},

		{
			desc:                         "dont export: timeseriesAreReady==false",
			cumulativeTimeseriesAreReady: false,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize), cumulativeTimeseries1),
			wantBufferAfterExport:        append(createBuffer(gaugeTimeseries1, maxBatchSize), cumulativeTimeseries1),
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs:                     nil,
		},
		{
			desc:                         "export: bufferSize = maxBatchSize",
			cumulativeTimeseriesAreReady: true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize-1), cumulativeTimeseries1),
			wantBufferAfterExport:        []*mrpb.TimeSeries{},
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: append(createBuffer(gaugeTimeseries1, maxBatchSize-1), cumulativeTimeseries1),
				},
			},
			wantLogLines: []logtest.Line{{
				Level:   zapcore.DebugLevel,
				Message: "Finished exporting metrics to Cloud Monitoring",
				Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
			}},
		},
		{
			desc:                         "export: bufferSize = 2*maxBatchSize",
			cumulativeTimeseriesAreReady: true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, 2*maxBatchSize-1), cumulativeTimeseries1),
			wantBufferAfterExport:        []*mrpb.TimeSeries{},
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: append(createBuffer(gaugeTimeseries1, maxBatchSize-1), cumulativeTimeseries1),
				},
			},
			wantLogLines: []logtest.Line{
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
			},
		},
		{
			desc:                         "export: bufferSize = 2*maxBatchSize+1",
			cumulativeTimeseriesAreReady: true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, 2*maxBatchSize), cumulativeTimeseries1),
			// The last cumulativeTimeseries1 timeseries added won't be part of the first 2 batches,
			// so it won't be exported and will remain in the buffer until either Flush() is called or
			// the buffer has at least maxBatchSize.
			wantBufferAfterExport:      []*mrpb.TimeSeries{cumulativeTimeseries1},
			wantGaugeBufferAfterExport: []*mrpb.TimeSeries{},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
			},
			wantLogLines: []logtest.Line{
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
			},
		},
		// Test cases for when splitGaugeBuffer is set to true. This changes the behavior of the
		// exporter. The exporter will still export GAUGE metrics even when timeSeriesAreReady is set to
		// false.
		{
			desc:                         "dont export: small buffer, splitGaugeBuffer=true",
			cumulativeTimeseriesAreReady: true,
			splitGaugeBufferOption:       true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize-2), cumulativeTimeseries1),
			wantBufferAfterExport:        []*mrpb.TimeSeries{cumulativeTimeseries1},
			wantGaugeBufferAfterExport:   createBuffer(gaugeTimeseries1, maxBatchSize-2),
			wantReqs:                     nil,
		},
		{
			desc:                         "only export GAUGE metrics when both buffers are full and timeseriesAreReady=false and splitGaugeBuffer=true",
			cumulativeTimeseriesAreReady: false,
			splitGaugeBufferOption:       true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize), createBuffer(cumulativeTimeseries1, maxBatchSize*2)...),
			wantBufferAfterExport:        createBuffer(cumulativeTimeseries1, maxBatchSize*2),
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
			},
			wantLogLines: []logtest.Line{{
				Level:   zapcore.DebugLevel,
				Message: "Finished exporting metrics to Cloud Monitoring",
				Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
			}},
		},
		{
			desc:                         "export gauge and cumulative when both buffers are full and timeseriesAreReady=true and splitGaugeBuffer=true",
			cumulativeTimeseriesAreReady: true,
			splitGaugeBufferOption:       true,
			timeSeriesToExport:           append(createBuffer(gaugeTimeseries1, maxBatchSize), createBuffer(cumulativeTimeseries1, maxBatchSize)...),
			wantBufferAfterExport:        []*mrpb.TimeSeries{},
			wantGaugeBufferAfterExport:   []*mrpb.TimeSeries{},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, maxBatchSize),
				},
			},
			wantLogLines: []logtest.Line{
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 0)},
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			client := &fakeClient{}
			testLogger := logtest.NewTestLogger(t, zap.DebugLevel)
			e := &exporter{
				requestName: testRequestName,
				buffer: &buffer{
					timeseries: make([]*mrpb.TimeSeries, 0, maxBatchSize),
				},
				gaugeBuffer: &buffer{
					timeseries:         make([]*mrpb.TimeSeries, 0, maxBatchSize),
					timeSeriesAreReady: true,
				},
				splitGaugeBuffer: tc.splitGaugeBufferOption,
				gcmClient:        client,
				logger:           testLogger.Logger,
			}
			e.initSelfObservabilityMetrics(nil)
			// Set the cumulative metrics readiness.
			e.CumulativeTimeseriesAreReady(tc.cumulativeTimeseriesAreReady)
			for _, ts := range tc.timeSeriesToExport {
				if err := e.ExportTimeSeries(context.Background(), ts); err != nil {
					t.Fatalf("e.ExportTimeseries(_, %v)=%q, want nil error", ts, err)
				}
			}
			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("e.ExportTimeseries() got unexpected diff on requests (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantBufferAfterExport, e.buffer.timeseries, protocmp.Transform()); diff != "" {
				t.Errorf("Buffer content after ExportTimeSeries() returned an unexpected diff (-want +got)\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantGaugeBufferAfterExport, e.gaugeBuffer.timeseries, protocmp.Transform()); diff != "" {
				t.Errorf("GaugeBuffer content after ExportTimeSeries() returned an unexpected diff (-want +got)\n%s", diff)
			}
			diff, err := testLogger.Expect(logtest.Expect{Lines: tc.wantLogLines})
			if err != nil {
				t.Errorf("ExportTimeSeries() produced invalid logs: %v", err)
			}
			if len(diff) > 0 {
				t.Errorf("ExportTimeSeries() produced unexpected logs: %s", diff)
			}
		})
	}
}

func TestExportTimeSeriesErrors(t *testing.T) {
	testcases := []struct {
		desc               string
		timeseriesAreReady bool
		buffer             []*mrpb.TimeSeries
		ts                 *mrpb.TimeSeries
		wantReqs           []*monitoringpb.CreateTimeSeriesRequest
		wantLogLines       []logtest.Line
	}{
		{
			desc:               "export a nil timeseries",
			timeseriesAreReady: true,
			buffer:             createBuffer(gaugeTimeseries1, maxBatchSize),
			ts:                 nil,
		},
		{
			desc:               "fail one request",
			timeseriesAreReady: true,
			buffer:             createBuffer(gaugeTimeseries1, maxBatchSize),
			ts:                 gaugeTimeseries1,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
			},
			wantLogLines: []logtest.Line{
				{
					Level:   zapcore.ErrorLevel,
					Message: "Failed to export metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Error(errTestFailedRequest)},
				},
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 1), zap.Int("failed_requests", 1)},
				},
			},
		},
		{
			desc:               "fail multiple requests and sample the error log",
			timeseriesAreReady: true,
			buffer:             createBuffer(gaugeTimeseries1, 3*maxBatchSize),
			ts:                 gaugeTimeseries1,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
			},
			wantLogLines: []logtest.Line{
				{
					Level:   zapcore.ErrorLevel,
					Message: "Failed to export metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Error(errTestFailedRequest)},
				},
				{
					Level:   zapcore.ErrorLevel,
					Message: "Failed to export metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Error(errTestFailedRequest)},
				},
				{
					Level:   zapcore.DebugLevel,
					Message: "Finished exporting metrics to Cloud Monitoring",
					Fields:  []zapcore.Field{zap.Int("total_requests", 3), zap.Int("failed_requests", 3)},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			client := &fakeClient{gcmReturnsError: errTestFailedRequest}
			testLogger := logtest.NewTestLogger(t, zap.DebugLevel)
			e := &exporter{
				requestName: testRequestName,
				buffer: &buffer{
					timeseries:         tc.buffer,
					timeSeriesAreReady: tc.timeseriesAreReady,
				},
				gcmClient: client,
				logger:    logging.RateLimitedLogger(testLogger.Logger, 10*defaultSelfObservabilityExportInterval, 2),
			}
			e.initSelfObservabilityMetrics(nil)
			if err := e.ExportTimeSeries(context.Background(), tc.ts); err == nil {
				t.Fatalf("e.ExportTimeseries(_, %v) = nil, want error", tc.ts)
			}
			diff, err := testLogger.Expect(logtest.Expect{Lines: tc.wantLogLines})
			if err != nil {
				t.Errorf("ExportTimeSeries(%v) produced invalid logs: %v", tc.ts, err)
			}
			if len(diff) > 0 {
				t.Errorf("ExportTimeSeries(%v) produced unexpected logs: %s", tc.ts, diff)
			}
		})
	}
}

func TestFlush(t *testing.T) {
	testcases := []struct {
		desc        string
		buffer      *buffer
		gaugeBuffer *buffer
		wantReqs    []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:     "not export: empty buffer",
			buffer:   nil,
			wantReqs: nil,
		},
		{
			desc:        "export buffers even if their sizes < maxBatchSize",
			buffer:      &buffer{timeseries: createBuffer(cumulativeTimeseries1, maxBatchSize-1), timeSeriesAreReady: true},
			gaugeBuffer: &buffer{timeseries: createBuffer(gaugeTimeseries1, maxBatchSize-1), timeSeriesAreReady: true},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize-1),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, maxBatchSize-1),
				},
			},
		},
		{
			desc:        "export buffers when their sizes == maxBatchSize",
			buffer:      &buffer{timeseries: createBuffer(cumulativeTimeseries1, maxBatchSize), timeSeriesAreReady: true},
			gaugeBuffer: &buffer{timeseries: createBuffer(gaugeTimeseries1, maxBatchSize), timeSeriesAreReady: true},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, maxBatchSize),
				},
			},
		},
		{
			desc:        "export buffers in batches when their sizes == 2*maxBatchSize + 1",
			buffer:      &buffer{timeseries: createBuffer(cumulativeTimeseries1, maxBatchSize*2+1), timeSeriesAreReady: true},
			gaugeBuffer: &buffer{timeseries: createBuffer(gaugeTimeseries1, maxBatchSize*2+1), timeSeriesAreReady: true},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, 1),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			client := &fakeClient{}
			e := &exporter{
				requestName:               testRequestName,
				buffer:                    tc.buffer,
				gaugeBuffer:               tc.gaugeBuffer,
				gcmClient:                 client,
				selfObservabilityResource: testContainerResource,
				logger:                    zap.NewNop(),
			}
			e.initSelfObservabilityMetrics(nil)
			if tc.gaugeBuffer != nil {
				e.splitGaugeBuffer = true
			}
			if err := e.Flush(context.Background()); err != nil {
				t.Fatalf("e.Flush()=%q, want nil error", err)
			}
			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("e.Flush() got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFlushConcurrency(t *testing.T) {
	name := "Test flush concurrency bug 393547030"
	t.Run(name, func(t *testing.T) {
		client := &fakeClient{}
		buffer := &buffer{timeseries: createBuffer(cumulativeTimeseries1, maxBatchSize*1000+1), timeSeriesAreReady: true}
		e := &exporter{
			requestName:               testRequestName,
			buffer:                    buffer,
			gcmClient:                 client,
			selfObservabilityResource: testContainerResource,
			logger:                    zap.NewNop(),
		}
		e.initSelfObservabilityMetrics(nil)
		var wg sync.WaitGroup
		context := context.Background()
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := e.Flush(context); err != nil {
					t.Errorf("e.Flush()=%q, want nil error", err)
				}
			}()
		}

		if err := e.Flush(context); err != nil {
			t.Errorf("e.Flush()=%q, want nil error", err)
		}

		wg.Wait()
	})
}

func TestFlushErrors(t *testing.T) {
	testcases := []struct {
		desc            string
		buffer          *buffer
		gaugeBuffer     *buffer
		gcmReturnsError error
		wantReqs        []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:            "Fail to Flush cumulative metrics when timeseriesAreReady==false",
			gcmReturnsError: nil,
			buffer: &buffer{
				timeseries:         createBuffer(cumulativeTimeseries1, maxBatchSize+1),
				timeSeriesAreReady: false,
			},
			gaugeBuffer: &buffer{
				timeseries:         createBuffer(gaugeTimeseries1, 1),
				timeSeriesAreReady: true,
			},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
			},
		},
		{
			desc:            "Fail to Flush cumulative metrics when GCM fails",
			gcmReturnsError: errTestFailedRequest,
			buffer: &buffer{
				timeseries:         createBuffer(cumulativeTimeseries1, 1),
				timeSeriesAreReady: true,
			},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(cumulativeTimeseries1, 1),
				},
			},
		},
		{
			desc:            "Fail to Flush gauge metrics when GCM fails",
			gcmReturnsError: errTestFailedRequest,
			gaugeBuffer: &buffer{
				timeseries:         createBuffer(gaugeTimeseries1, 1),
				timeSeriesAreReady: true,
			},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			client := &fakeClient{gcmReturnsError: tc.gcmReturnsError}
			e := &exporter{
				requestName:               testRequestName,
				selfObservabilityResource: testContainerResource,
				buffer:                    tc.buffer,
				gaugeBuffer:               tc.gaugeBuffer,
				gcmClient:                 client,
				logger:                    zap.NewNop(),
			}
			e.initSelfObservabilityMetrics(nil)
			if tc.gaugeBuffer != nil {
				e.splitGaugeBuffer = true
			}
			if err := e.Flush(context.Background()); err == nil {
				t.Fatalf("e.Flush() returned nil, want error")
			}
			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("e.Flush() got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

// Test if the self-observability goroutine scrapes properly and all self-observability metrics exist.
func TestExportSelfObservability(t *testing.T) {
	// Creates a list of GAUGE INT64 metrics.
	createGaugeMetrics := func(count int) []*metrics.GaugeInt64 {
		gauges := []*metrics.GaugeInt64{}
		for i := 0; i < count; i++ {
			gauges = append(gauges,
				metrics.NewGaugeInt64(metrics.DescriptorOpts{
					Name: fmt.Sprintf("gauge_metric_%d", i),
				}))
		}
		return gauges
	}
	// Creates a list of gauge time series. You can customize the metric name suffix index, by passing
	// a non-zero value to nameIndexStartsFrom int. For example, if you want to generate metrics:
	// gauge_metric_3 and gauge_metric_4, call createGaugeTimeSeries(2 /* two metrics */, 3)
	createGaugeTimeSeries := func(value int, nameIndexStartsFrom int) []*mrpb.TimeSeries {
		var timeseries []*mrpb.TimeSeries
		for i := 0; i < value; i++ {
			timeseries = append(timeseries, &mrpb.TimeSeries{
				Metric: &metricpb.Metric{
					Type:   fmt.Sprintf("gauge_metric_%d", i+nameIndexStartsFrom),
					Labels: map[string]string{},
				},
				MetricKind: metricpb.MetricDescriptor_GAUGE,
				ValueType:  metricpb.MetricDescriptor_INT64,
				Resource:   fakeResource,
				Points: []*mrpb.Point{{
					Value: &cpb.TypedValue{
						Value: &cpb.TypedValue_Int64Value{Int64Value: 0},
					}},
				},
			})
		}
		return timeseries
	}
	testCases := []struct {
		desc                               string
		sleep                              time.Duration
		gcmError                           error
		additionalSelfObservabilityMetrics []*metrics.GaugeInt64
		exportTimeSeries                   []*mrpb.TimeSeries
		wantReqs                           []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:     "0 scrape interval",
			sleep:    time.Second,
			wantReqs: nil,
		},
		{
			desc:  "1 scrape interval",
			sleep: 3 * time.Second,
			exportTimeSeries: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{Type: "metric_test"},
			}},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:  "2 scrape intervals",
			sleep: 5 * time.Second,
			exportTimeSeries: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{Type: "metric_test"},
			}},
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						}),
					},
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						// 1 self observability metric was exported in the last export interval
						exportedTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						}),
						gRPCRequestLatencyTimeSeries(1, testTargetName),
					},
				},
			},
		},
		{
			desc:                               "1 scrape interval with a GCM error fails gracefully",
			sleep:                              3 * time.Second,
			additionalSelfObservabilityMetrics: createGaugeMetrics(maxBatchSize + 1),
			gcmError:                           errors.New("error"),
			wantReqs:                           nil,
		},
		{
			desc:  "2 scrape intervals with additional self-observability metrics",
			sleep: 5 * time.Second,
			exportTimeSeries: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{Type: "metric_test"},
			}},
			additionalSelfObservabilityMetrics: createGaugeMetrics(20),
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: append(createGaugeTimeSeries(20, 0),
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						})),
				},
				{
					Name: testRequestName,
					TimeSeries: append(
						createGaugeTimeSeries(20, 0),
						// 21 self observability metrics were exported in the last export interval
						exportedTimeSeries(21, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						}),
						gRPCRequestLatencyTimeSeries(1, testTargetName)),
				},
			},
		},
		{
			desc:  "Self-observability metrics are batched",
			sleep: 3 * time.Second,
			exportTimeSeries: []*mrpb.TimeSeries{{
				Metric: &metricpb.Metric{Type: "metric_test"},
			}},
			// 201 additional self-observability metrics
			additionalSelfObservabilityMetrics: createGaugeMetrics(maxBatchSize + 1),
			// 201 additional self-observability metrics + max_buffer_size metric = 202 metrics.
			// So we expect a batch with 200 metrics + a second batch with the remaining 2 metrics.
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: append(createGaugeTimeSeries(maxBatchSize-1, 0),
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						})),
				},
				{ // Any remaining self-observability metrics will be sent in a subsequent batch.
					Name:       testRequestName,
					TimeSeries: createGaugeTimeSeries(2, maxBatchSize-1),
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			additionalSelfObsMetrics := []GKEMetric{}
			for _, m := range tc.additionalSelfObservabilityMetrics {
				// Always set a value to the additional self-observability metrics,
				// otherwise, they won't be exported.
				m.SetWithoutLabels(0)
				additionalSelfObsMetrics = append(additionalSelfObsMetrics, m)
			}
			ctx := context.Background()
			client := &fakeClient{gcmReturnsError: tc.gcmError, doNotStoreMetricsOnError: true}
			options := Options{
				SelfObservabilityResource:          testContainerResource,
				GCMClient:                          client,
				Logger:                             zap.NewNop(),
				AdditionalSelfObservabilityMetrics: additionalSelfObsMetrics,
				SelfObservabilityExportInterval:    2 * time.Second,
				TargetName:                         testTargetName,
			}
			e, err := NewExporter(ctx, options)
			defer e.cancel()
			if err != nil {
				t.Fatalf("NewExporter(ctx, %v) = %q, want nil error", options, err)
			}
			// Export some timeseries so that our self-observability metrics are
			// incremented.
			for _, ts := range tc.exportTimeSeries {
				if err := e.ExportTimeSeries(ctx, ts); err != nil {
					t.Fatalf("ExportTimeSeries(ctx, %v) = %q, want nil error", ts, err)
				}
			}
			time.Sleep(tc.sleep)

			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("exportSelfObservability(ctx) got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

// Test on the functionality of exporting correct values and labels for `timeseries_exported` and
// `completed_grpcs`.
func TestTotalTimeSeriesExportedAndCompletedGRPCsMetric(t *testing.T) {
	testCases := []struct {
		desc                      string
		applicationMetrics        []*mrpb.TimeSeries
		applicationMetricsError   error
		observabilityMetricsError error
		wantReqs                  []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:                      "export both application and observability metrics with nil error",
			applicationMetrics:        createBuffer(gaugeTimeseries1, maxBatchSize+1),
			applicationMetricsError:   nil,
			observabilityMetricsError: nil,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{ // request of self-observability metrics
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						// The following metrics exist but are ignored for comparison,
						// they apply to all the requests of self-observability in this test:
						// - grpcRequestLatency
						// - maxObservedBufferSize
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
				{ // request of self-observability metrics
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						// TimeSeries for the last request for self-observability metrics, this number includes
						// all the metrics that actually exist, including the self-observability metrics that
						// we ignore, on purpose, on this test. For example, grpcRequestLatency.
						// maxObservedBufferSize is not exported, because we don't call ExportTimeSeries() in
						// this test.
						exportedTimeSeries(3, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:                      "export both application and observability metrics with ok status",
			applicationMetrics:        createBuffer(gaugeTimeseries1, maxBatchSize+1),
			applicationMetricsError:   status.Error(codes.OK, ""),
			observabilityMetricsError: status.Error(codes.OK, ""),
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						exportedTimeSeries(3, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:                      "export application metrics with Invalid Argument, export self observability metrics with nil error",
			applicationMetrics:        createBuffer(gaugeTimeseries1, maxBatchSize+1),
			applicationMetricsError:   status.Error(codes.InvalidArgument, "One or more TimeSeries could not be written: Unknown metric"),
			observabilityMetricsError: nil,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.InvalidArgument.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.InvalidArgument.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.InvalidArgument.String(),
							keyTargetName:       testTargetName,
						}),
						exportedTimeSeries(3, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.InvalidArgument.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:                      "export application metrics with nil error, export self observability metrics with Unknown error",
			applicationMetrics:        createBuffer(gaugeTimeseries1, maxBatchSize+1),
			applicationMetricsError:   nil,
			observabilityMetricsError: status.Error(codes.Unknown, "One or more TimeSeries could not be written: Unknown error Xxxx."),
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						exportedTimeSeries(3, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:                      "export application and observability metrics with errors not compatible with grpc status",
			applicationMetrics:        createBuffer(gaugeTimeseries1, maxBatchSize+1),
			applicationMetricsError:   fmt.Errorf("not an error in Status representation"),
			observabilityMetricsError: fmt.Errorf("not an error in Status representation"),
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, maxBatchSize),
				},
				{
					Name:       testRequestName,
					TimeSeries: createBuffer(gaugeTimeseries1, 1),
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						exportedTimeSeries(201, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
						exportedTimeSeries(3, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeSelfObservability,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.Unknown.String(),
							keyTargetName:       testTargetName,
						}),
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client := &fakeClient{
				gcmReturnsError: tc.applicationMetricsError,
				metricsAllowList: map[string]struct{}{
					"gauge_metric_1": {},
					"kubernetes.io/internal/metrics_exporter/timeseries_exported": {},
					"kubernetes.io/internal/metrics_exporter/completed_grpcs":     {},
				},
			}
			e := &exporter{
				requestName: testRequestName,
				gcmClient:   client,
				buffer: &buffer{
					timeseries:         tc.applicationMetrics,
					timeSeriesAreReady: true,
				},
				requestNameForSelfObservabilityMetrics: testRequestName,
				gcmClientForSelfObservabilityMetrics:   client,
				selfObservabilityResource:              testContainerResource,
				logger:                                 zap.NewNop(),
				targetName:                             testTargetName,
			}
			e.initSelfObservabilityMetrics(nil)
			ctx := context.Background()
			// First, we Flush() the application metrics that are in the buffer.
			// This will ensure that some self-observability metrics are incremented,
			// so we can verify them in the next test step.
			// We sometimes force an error from GCM when calling Flush () to see if the
			// self-observability metrics will capture that.
			if err := e.Flush(ctx); tc.applicationMetricsError == nil && err != nil {
				t.Errorf("Flush()=%q, want nil error", err)
			} else if tc.applicationMetricsError != nil && err == nil {
				t.Errorf("Flush()=nil, want error")
			}

			client.setError(tc.observabilityMetricsError)
			// Export self-observability metric with metricTypeApplication.
			// Since it is the first time exporting self-observability metrics, there is only data about
			// user application metrics. After this request, there will be data about self-observability
			// metrics, thus we run exportSelfObservability() again to get those metrics.
			if err := e.exportSelfObservability(ctx); err != tc.observabilityMetricsError {
				t.Errorf("exportSelfObservability()=%q, want %q for application metric type", err, tc.observabilityMetricsError)
			}
			// Export self-observability metrics with metricTypeSelfObservability.
			if err := e.exportSelfObservability(ctx); err != tc.observabilityMetricsError {
				t.Errorf("exportSelfObservability()=%q, want %q for self-observability metric type", err, tc.observabilityMetricsError)
			}
			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("exportSelfObservability() got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMaxBufferSizeMetric(t *testing.T) {
	testCases := []struct {
		desc          string
		initialBuffer []*mrpb.TimeSeries
		inputTs       *mrpb.TimeSeries
		wantReqs      []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:          "update max buffer size",
			initialBuffer: []*mrpb.TimeSeries{},
			inputTs:       gaugeTimeseries1,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						maxBufferSizeTimeSeries(1, map[string]string{
							keyTargetName: testTargetName,
						}),
					},
				},
			},
		},
		{
			desc:          "current buffer size is smaller than the current max buffer size",
			initialBuffer: createBuffer(cumulativeTimeseries1, 25),
			inputTs:       cumulativeTimeseries1,
			wantReqs: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: testRequestName,
					TimeSeries: []*mrpb.TimeSeries{
						maxBufferSizeTimeSeries(25, map[string]string{
							keyTargetName: testTargetName,
						}),
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			client := &fakeClient{
				metricsAllowList: map[string]struct{}{
					// We only care about this metric in this test.
					"kubernetes.io/internal/metrics_exporter/max_observed_buffer_size": {},
				},
			}
			e := &exporter{
				requestName: testRequestName,
				gcmClient:   client,
				buffer: &buffer{
					timeSeriesAreReady: true,
				},
				selfObservabilityResource:              testContainerResource,
				requestNameForSelfObservabilityMetrics: testRequestName,
				gcmClientForSelfObservabilityMetrics:   client,
				logger:                                 zap.NewNop(),
				targetName:                             testTargetName,
			}
			e.initSelfObservabilityMetrics(nil)
			// Export the initial buffer to set an initial maximum observed buffer size.
			for _, ts := range tc.initialBuffer {
				if err := e.ExportTimeSeries(ctx, ts); err != nil {
					t.Fatalf("ExportTimeSeries()=%q, want nil error", err)
				}
			}
			// Flush anything that's in the buffer, so our current buffer size is 0 again.
			if err := e.Flush(ctx); err != nil {
				t.Fatalf("Flush()=%q, want nil error", err)
			}

			if err := e.ExportTimeSeries(ctx, tc.inputTs); err != nil {
				t.Errorf("ExportTimeSeries()=%q, want nil error", err)
			}
			if err := e.exportSelfObservability(ctx); err != nil {
				t.Errorf("exportSelfObservability()=%q, want nil error", err)
			}
			if diff := compareRequests(tc.wantReqs, client.requests()); diff != "" {
				t.Errorf("exportSelfObservability() got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

func createBuffer(ts *mrpb.TimeSeries, size int) []*mrpb.TimeSeries {
	buffer := make([]*mrpb.TimeSeries, size)
	for i := 0; i < size; i++ {
		buffer[i] = ts
	}
	return buffer
}

func TestNewExporter(t *testing.T) {
	testClient := &fakeClient{}
	testcases := []struct {
		desc                                       string
		options                                    Options
		wantRequestName                            string
		wantTimeseriesAreReady                     bool
		wantSelfObservabilityResource              metrics.Resource
		wantSelfObservabilityExportInterval        time.Duration
		wantGaugeBuffer                            *buffer
		wantLogger                                 *zap.Logger
		wantRequestNameForSelfObservabilityMetrics string
	}{
		{
			desc: "OK, default self observability scrape interval",
			options: Options{
				SelfObservabilityResource: testContainerResource,
				GCMClient:                 testClient,
				Logger:                    zap.NewNop(),
			},
			wantRequestName: testRequestName,
			wantRequestNameForSelfObservabilityMetrics: testRequestName,
			wantTimeseriesAreReady:                     true,
			wantSelfObservabilityResource:              testContainerResource,
			wantSelfObservabilityExportInterval:        defaultSelfObservabilityExportInterval,
			wantLogger:                                 zap.NewNop(),
		},
		{
			desc: "OK",
			options: Options{
				SelfObservabilityResource:       testContainerResource,
				GCMClient:                       testClient,
				SelfObservabilityExportInterval: time.Second,
				Logger:                          zap.NewNop(),
			},
			wantRequestName: testRequestName,
			wantRequestNameForSelfObservabilityMetrics: testRequestName,
			wantTimeseriesAreReady:                     true,
			wantSelfObservabilityResource:              testContainerResource,
			wantSelfObservabilityExportInterval:        time.Second,
			wantLogger:                                 zap.NewNop(),
		},
		{
			desc: "OK: gaugeBuffer enabled",
			options: Options{
				SplitGaugeBuffer:                true,
				SelfObservabilityResource:       testContainerResource,
				GCMClient:                       testClient,
				SelfObservabilityExportInterval: time.Second,
				Logger:                          zap.NewNop(),
			},
			wantGaugeBuffer: &buffer{
				timeseries:         make([]*mrpb.TimeSeries, 0, maxBatchSize),
				timeSeriesAreReady: true,
			},
			wantRequestName: testRequestName,
			wantRequestNameForSelfObservabilityMetrics: testRequestName,
			wantTimeseriesAreReady:                     true,
			wantSelfObservabilityResource:              testContainerResource,
			wantSelfObservabilityExportInterval:        time.Second,
			wantLogger:                                 zap.NewNop(),
		},
		{
			desc: "nil logger is replaced with a non-nil logger",
			options: Options{
				SelfObservabilityResource:       testContainerResource,
				GCMClient:                       testClient,
				SelfObservabilityExportInterval: time.Second,
				Logger:                          nil,
			},
			wantRequestName: testRequestName,
			wantRequestNameForSelfObservabilityMetrics: testRequestName,
			wantTimeseriesAreReady:                     true,
			wantSelfObservabilityResource:              testContainerResource,
			wantSelfObservabilityExportInterval:        time.Second,
			wantLogger:                                 zap.NewNop(),
		},
		{
			desc: "OK separate gcm client for self observability metrics",
			options: Options{
				SelfObservabilityResource:            testContainerResource,
				GCMClient:                            testClient,
				Logger:                               zap.NewNop(),
				ProjectNumber:                        "1111",
				GCMClientForSelfObservabilityMetrics: &fakeClient{},
			},
			wantRequestName:                            "projects/1111",
			wantTimeseriesAreReady:                     true,
			wantRequestNameForSelfObservabilityMetrics: "projects/12345678",
			wantSelfObservabilityResource:              testContainerResource,
			wantSelfObservabilityExportInterval:        defaultSelfObservabilityExportInterval,
			wantLogger:                                 zap.NewNop(),
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := NewExporter(context.Background(), tc.options)
			defer got.cancel()
			if err != nil {
				t.Errorf("NewExporter(%v) = %v, want nil", tc.options, err)
			}

			if tc.wantRequestName != got.requestName {
				t.Errorf("NewExporter(%v) returned requestName = %q, want %q", tc.options, got.requestName, tc.wantRequestName)
			}
			if got.buffer == nil {
				t.Fatalf("NewExporter(%v) returned buffer = nil, want non-nil", tc.options)
			}
			if diff := cmp.Diff(tc.wantGaugeBuffer, got.gaugeBuffer, protocmp.Transform(), cmp.AllowUnexported(buffer{})); diff != "" {
				t.Errorf("NewExporter(%v) returned unexpected diff on gaugeBuffer (-want +got):\n%s", tc.options, diff)
			}
			if diff := cmp.Diff(tc.wantSelfObservabilityResource, got.selfObservabilityResource, protocmp.Transform(), cmp.AllowUnexported(metrics.Resource{})); diff != "" {
				t.Errorf("NewExporter(%v) returned unexpected diff on selfObservabilityResource (-want +got):\n%s", tc.options, diff)
			}
			if tc.wantSelfObservabilityExportInterval != got.selfObservabilityExportInterval {
				t.Errorf("NewExporter(%v) returned wantSelfObservabilityExportInterval = %v, want %v", tc.options, got.selfObservabilityExportInterval, tc.wantSelfObservabilityExportInterval)
			}
			if diff := cmp.Diff(tc.wantLogger, got.logger, protocmp.Transform(), cmp.AllowUnexported(zap.Logger{}), cmpopts.IgnoreFields(zap.Logger{}, "core")); diff != "" {
				t.Errorf("NewExporter(%v) returned logger = %v, want %v", tc.options, got.logger, tc.wantLogger)
			}
			if tc.wantRequestNameForSelfObservabilityMetrics != got.requestNameForSelfObservabilityMetrics {
				t.Errorf("NewExporter(%v) returned requestNameForSelfObservabilityMetrics = %q, want %q", tc.options, got.requestNameForSelfObservabilityMetrics, tc.wantRequestNameForSelfObservabilityMetrics)
			}

			if got.cancel == nil {
				t.Errorf("exporter %v cancel = nil, want non-nil", got)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	testcases := []struct {
		desc         string
		options      Options
		wantExporter *exporter
	}{

		{
			desc: "empty project number",
			options: Options{
				SelfObservabilityResource: metrics.NewK8sContainer("", "", "", "", "", ""),
				GCMClient:                 &fakeClient{},
			},
		},
		{
			desc: "nil GCM client",
			options: Options{
				SelfObservabilityResource: testContainerResource,
				GCMClient:                 nil,
			},
		},
		{
			desc: "empty ProjectNumber when specifying GCMClientForSelfObservabilityMetrics",
			options: Options{
				SelfObservabilityResource:            testContainerResource,
				GCMClient:                            &fakeClient{},
				Logger:                               zap.NewNop(),
				GCMClientForSelfObservabilityMetrics: &fakeClient{},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			if e, err := NewExporter(context.Background(), tc.options); err == nil {
				e.cancel()
				t.Errorf("NewExporter(%v) = nil, want error", tc.options)
			}
		})
	}
}

func TestRequestsToGCMAreTimeCapped(t *testing.T) {
	testcases := []struct {
		desc                  string
		timeout               time.Duration
		wantGCMRequestTimeout time.Duration
	}{

		{
			desc:                  "Default timeout (45s)",
			wantGCMRequestTimeout: 45 * time.Second,
		},
		{
			desc:                  "Override default timeout to 100s",
			timeout:               100 * time.Second,
			wantGCMRequestTimeout: 100 * time.Second,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			testClient := &fakeClient{}
			opts := Options{
				GCMClient:                 testClient,
				MetricsExportTimeout:      tc.timeout,
				Logger:                    zap.NewNop(),
				SelfObservabilityResource: testContainerResource,
			}
			e, err := NewExporter(context.Background(), opts)
			if err != nil {
				t.Fatalf("NewExporter(%v) = %v, want nil", opts, err)
			}
			defer e.cancel()

			if err := e.exportToGCM(ctx, createBuffer(cumulativeTimeseries1, 1), false /* isSelfObservability */); err != nil {
				t.Fatalf("exportToGCM() = %v, want nil", err)
			}

			// To make these tests cheaper, we don't wait until the request really hits its deadline
			// Instead, we capture the context deadline time and reverse engineer the timeout duration.
			// Because of that, we compare the want/got timeout durations, considering a jitter of 5s.
			// This should be long enough to not make these tests flaky.
			want := tc.wantGCMRequestTimeout.Seconds()
			got := testClient.requestTimeout.Seconds()
			if math.Abs(want-got) > 5 {
				t.Errorf("GCM was called with %v timeout, want %v timeout (+/- 5s)", testClient.requestTimeout, tc.wantGCMRequestTimeout)
			}
		})
	}
}

func exportedTimeSeries(value int, labels map[string]string) *mrpb.TimeSeries {
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   "kubernetes.io/internal/metrics_exporter/timeseries_exported",
			Labels: labels,
		},
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Resource:   fakeResource,
		Points: []*mrpb.Point{
			{
				Interval: &cpb.TimeInterval{
					StartTime: timestamppb.New(testStartTime),
					EndTime:   timestamppb.New(testEndTime),
				},
				Value: &cpb.TypedValue{
					Value: &cpb.TypedValue_Int64Value{Int64Value: int64(value)},
				},
			},
		},
		Unit: "1",
	}
}

func maxBufferSizeTimeSeries(value int, labels map[string]string) *mrpb.TimeSeries {
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   "kubernetes.io/internal/metrics_exporter/max_observed_buffer_size",
			Labels: labels,
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Resource:   fakeResource,
		Points: []*mrpb.Point{
			{
				Interval: &cpb.TimeInterval{
					EndTime: timestamppb.New(testEndTime),
				},
				Value: &cpb.TypedValue{
					Value: &cpb.TypedValue_Int64Value{Int64Value: int64(value)},
				},
			},
		},
		Unit: "1",
	}
}

func completedGRPCsTimeSeries(value int, labels map[string]string) *mrpb.TimeSeries {
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type:   "kubernetes.io/internal/metrics_exporter/completed_grpcs",
			Labels: labels,
		},
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_INT64,
		Resource:   fakeResource,
		Points: []*mrpb.Point{
			{
				Interval: &cpb.TimeInterval{
					StartTime: timestamppb.New(testStartTime),
					EndTime:   timestamppb.New(testEndTime),
				},
				Value: &cpb.TypedValue{
					Value: &cpb.TypedValue_Int64Value{Int64Value: int64(value)},
				},
			},
		},
		Unit: "1",
	}
}

func gRPCRequestLatencyTimeSeries(count int, targetName string) *mrpb.TimeSeries {
	bucketCount := make([]int64, len(defaultMillisecondsDistribution)+1)
	bucketCount[0] = int64(count)
	return &mrpb.TimeSeries{
		Metric: &metricpb.Metric{
			Type: "kubernetes.io/internal/metrics_exporter/grpc_request_latency",
			Labels: map[string]string{
				keyGRPCClientMethod: "google.monitoring.v3.MetricService/CreateServiceTimeSeries",
				keyTargetName:       targetName,
			},
		},
		MetricKind: metricpb.MetricDescriptor_CUMULATIVE,
		ValueType:  metricpb.MetricDescriptor_DISTRIBUTION,
		Resource:   fakeResource,
		Unit:       "ms",
		Points: []*mrpb.Point{
			{
				Interval: &cpb.TimeInterval{
					StartTime: timestamppb.New(testStartTime),
					EndTime:   timestamppb.New(testEndTime),
				},
				Value: &cpb.TypedValue{
					Value: &cpb.TypedValue_DistributionValue{
						DistributionValue: &dpb.Distribution{
							Count:                 int64(count),
							Mean:                  0,
							SumOfSquaredDeviation: 0,
							BucketOptions: &dpb.Distribution_BucketOptions{
								Options: &dpb.Distribution_BucketOptions_ExplicitBuckets{
									ExplicitBuckets: &dpb.Distribution_BucketOptions_Explicit{
										Bounds: defaultMillisecondsDistribution,
									},
								},
							},
							BucketCounts: bucketCount,
						},
					},
				},
			},
		},
	}
}

func compareRequests(wantRequests []*monitoringpb.CreateTimeSeriesRequest, gotRequests []*monitoringpb.CreateTimeSeriesRequest) string {
	return cmp.Diff(wantRequests, gotRequests, protocmp.Transform(),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.SortRepeatedFields(&monitoringpb.CreateTimeSeriesRequest{}, "time_series"),
		protocmp.FilterMessage(&dpb.Distribution{}, compareDistributionValue()))
}

func sumInt64SliceValue(in []int64) int64 {
	sum := int64(0)
	for _, v := range in {
		sum += v
	}
	return sum
}

// When comparing distribution value, only check
// - the count of data points are expected
// - the total count in bucket_counts are expected (it doesn't matter which bucket
// the data point falls in because we don't mock time.Now() in some distribution tests.)
func compareDistributionValue() cmp.Option {
	return cmp.Comparer(func(a, b protocmp.Message) bool {
		if a["count"] != b["count"] {
			return false
		}
		aSlice, ok := a["bucket_counts"].([]int64)
		if !ok {
			return false
		}
		bSlice, ok := b["bucket_counts"].([]int64)
		if !ok {
			return false
		}
		if sumInt64SliceValue(aSlice) != sumInt64SliceValue(bSlice) {
			return false
		}
		return true
	})
}

func TestBufferReadyToBeExported(t *testing.T) {
	testCases := []struct {
		desc   string
		buffer *buffer
		want   bool
	}{
		{
			desc: "timeSeries are ready and the buffer size is >= the max batch size",
			buffer: &buffer{
				timeseries:         createBuffer(cumulativeTimeseries1, maxBatchSize),
				timeSeriesAreReady: true,
			},
			want: true,
		},
		{
			desc: "timeSeries are not ready and the buffer size is >= the max batch size",
			buffer: &buffer{
				timeseries:         createBuffer(cumulativeTimeseries1, maxBatchSize),
				timeSeriesAreReady: false,
			},
			want: false,
		},
		{
			desc: "timeSeries are ready and the buffer size is < the max batch size",
			buffer: &buffer{
				timeseries:         createBuffer(cumulativeTimeseries1, maxBatchSize-1),
				timeSeriesAreReady: true,
			},
			want: false,
		},
		{
			desc:   "nil buffer",
			buffer: nil,
			want:   false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.buffer.readyToBeExported(); got != tc.want {
				t.Errorf("readyToBeExported() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBufferCumulativeTimeSeriesAreReady(t *testing.T) {
	testCases := []struct {
		desc                         string
		buffer                       *buffer
		cumulativeTimeSeriesAreReady bool
	}{
		{
			desc:                         "OK: false",
			buffer:                       &buffer{timeSeriesAreReady: true},
			cumulativeTimeSeriesAreReady: false,
		},
		{
			desc:                         "OK: true",
			buffer:                       &buffer{timeSeriesAreReady: false},
			cumulativeTimeSeriesAreReady: true,
		},
		{
			desc:   "nil buffer: doesn't crash",
			buffer: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			tc.buffer.cumulativeTimeSeriesAreReady(tc.cumulativeTimeSeriesAreReady)
			if tc.buffer == nil {
				return
			}
			if tc.buffer.timeSeriesAreReady != tc.cumulativeTimeSeriesAreReady {
				t.Errorf("cumulativeTimeSeriesAreReady() = %v, want %v", tc.buffer.timeSeriesAreReady, tc.cumulativeTimeSeriesAreReady)
			}
		})
	}
}

func TestCustomSelfObservabilityGCMClient(t *testing.T) {
	testCases := []struct {
		desc                          string
		applicationTimeSeries         []*mrpb.TimeSeries
		wantApplicationRequests       []*monitoringpb.CreateTimeSeriesRequest
		wantSelfObservabilityRequests []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc: "separate clients for application and self observability metrics",
			applicationTimeSeries: []*mrpb.TimeSeries{
				{
					Metric: &metricpb.Metric{Type: "application_metric_1"},
				},
				{
					Metric: &metricpb.Metric{Type: "application_metric_2"},
				},
			},
			wantApplicationRequests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: "projects/consumer-project-number",
					TimeSeries: []*mrpb.TimeSeries{
						{
							Metric: &metricpb.Metric{Type: "application_metric_1"},
						},
						{
							Metric: &metricpb.Metric{Type: "application_metric_2"},
						},
					},
				},
			},
			wantSelfObservabilityRequests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: "projects/12345678",
					TimeSeries: []*mrpb.TimeSeries{
						maxBufferSizeTimeSeries(2, map[string]string{
							keyTargetName: testTargetName,
						}),
						completedGRPCsTimeSeries(1, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						exportedTimeSeries(2, map[string]string{
							keyMetricType:       metricTypeApplication,
							keyGRPCClientMethod: gRPCMethodCreateServiceTimeSeries,
							keyGRPCClientStatus: codes.OK.String(),
							keyTargetName:       testTargetName,
						}),
						gRPCRequestLatencyTimeSeries(1, testTargetName),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			selfObservabilityClient := &fakeClient{}
			applicationClient := &fakeClient{}
			ctx := context.Background()

			options := Options{
				ProjectNumber:                        "consumer-project-number",
				GCMClient:                            applicationClient,
				Logger:                               zap.NewNop(),
				SelfObservabilityExportInterval:      2 * time.Second,
				SelfObservabilityResource:            testContainerResource,
				GCMClientForSelfObservabilityMetrics: selfObservabilityClient,
				TargetName:                           testTargetName,
			}
			e, err := NewExporter(ctx, options)
			defer e.cancel()
			if err != nil {
				t.Fatalf("NewExporter(ctx, %v) = %q, want nil error", options, err)
			}

			// Export some timeseries so that both clients export some metrics.
			for _, ts := range tc.applicationTimeSeries {
				if err := e.ExportTimeSeries(ctx, ts); err != nil {
					t.Fatalf("ExportTimeSeries(ctx, %v) = %q, want nil error", ts, err)
				}
			}
			if err := e.Flush(ctx); err != nil {
				t.Fatalf("Flush(ctx) = %q, want nil error", err)
			}
			// Sleep more than 1 SelfObservabilityExportInterval to ensure self-observability metrics are
			// exported once, since self-observability metrics are exported in a separate go routine.
			time.Sleep(3 * time.Second)

			if diff := compareRequests(tc.wantApplicationRequests, applicationClient.requests()); diff != "" {
				t.Errorf("defaultClient got unexpected diff on requests (-want +got):\n%s", diff)
			}
			if diff := compareRequests(tc.wantSelfObservabilityRequests, selfObservabilityClient.requests()); diff != "" {
				t.Errorf("selfObservabilityClient got unexpected diff on requests (-want +got):\n%s", diff)
			}
		})
	}
}

// Test if the self-observability metrics default to the correct name or respect the passed name.
func TestSelfObservabilityHasCorrectMetricNames(t *testing.T) {

	testCases := []struct {
		desc                          string
		metricNameOverride            *SelfObservabilityMetricNameOverride
		wantSelfObservabilityRequests []*monitoringpb.CreateTimeSeriesRequest
	}{{
		metricNameOverride: nil,
		desc:               "does not have custom self observability metric names set",
		wantSelfObservabilityRequests: []*monitoringpb.CreateTimeSeriesRequest{
			&monitoringpb.CreateTimeSeriesRequest{
				TimeSeries: []*mrpb.TimeSeries{
					&mrpb.TimeSeries{
						Metric: &metricpb.Metric{Type: "application_metric_1"},
					}}},
			&monitoringpb.CreateTimeSeriesRequest{
				TimeSeries: []*mrpb.TimeSeries{
					&mrpb.TimeSeries{
						Metric: &metricpb.Metric{Type: timeSeriesExportedMetricName},
					},
					&mrpb.TimeSeries{
						Metric: &metricpb.Metric{Type: maxObservedBufferSizeMetricName},
					},
					&mrpb.TimeSeries{
						Metric: &metricpb.Metric{Type: grpcRequestLatencyMetricName},
					},
					&mrpb.TimeSeries{
						Metric: &metricpb.Metric{Type: completedGRPCsMetricName},
					},
				}},
		}},
		{
			metricNameOverride: &SelfObservabilityMetricNameOverride{
				"metricname1",
				"metricname2",
				"metricname3",
				"metricname4",
			},
			desc: "has custom self observability metric names set",
			wantSelfObservabilityRequests: []*monitoringpb.CreateTimeSeriesRequest{
				&monitoringpb.CreateTimeSeriesRequest{
					TimeSeries: []*mrpb.TimeSeries{
						&mrpb.TimeSeries{
							Metric: &metricpb.Metric{Type: "application_metric_1"},
						}}},
				&monitoringpb.CreateTimeSeriesRequest{
					TimeSeries: []*mrpb.TimeSeries{
						&mrpb.TimeSeries{
							Metric: &metricpb.Metric{Type: "metricname1"},
						},
						&mrpb.TimeSeries{
							Metric: &metricpb.Metric{Type: "metricname2"},
						},
						&mrpb.TimeSeries{
							Metric: &metricpb.Metric{Type: "metricname3"},
						},
						&mrpb.TimeSeries{
							Metric: &metricpb.Metric{Type: "metricname4"},
						},
					}},
			}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			ctx := context.Background()

			client := &fakeClient{gcmReturnsError: nil, doNotStoreMetricsOnError: true}
			options := Options{
				SelfObservabilityResource:            testContainerResource,
				GCMClient:                            client,
				SelfObservabilityExportInterval:      2 * time.Second,
				SelfObservabilityMetricNameOverrides: testCase.metricNameOverride,
			}

			e, err := NewExporter(ctx, options)
			defer e.cancel()
			if err != nil {
				t.Fatalf("NewExporter(ctx, %v) = %q, want nil error", options, err)
			}

			applicationTimeSeries := []*mrpb.TimeSeries{
				{
					Metric: &metricpb.Metric{Type: "application_metric_1"},
				},
			}

			// Export some timeseries
			for _, ts := range applicationTimeSeries {
				if err := e.ExportTimeSeries(ctx, ts); err != nil {
					t.Fatalf("ExportTimeSeries(ctx, %v) = %q, want nil error", ts, err)
				}
			}
			if err := e.Flush(ctx); err != nil {
				t.Fatalf("Flush(ctx) = %q, want nil error", err)
			}
			// Sleep more than 1 SelfObservabilityExportInterval to ensure self-observability metrics are
			// exported once, since self-observability metrics are exported in a separate go routine.
			time.Sleep(3 * time.Second)

			tsCreateRequest := client.requests()

			if diff := cmp.Diff(testCase.wantSelfObservabilityRequests,
				tsCreateRequest,
				protocmp.Transform(),
				protocmp.IgnoreFields(&monitoringpb.CreateTimeSeriesRequest{}, "name"),
				protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
				protocmp.IgnoreFields(&metricpb.Metric{}, "labels"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "points"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "resource"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "value_type"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "metric_kind"),
				protocmp.SortRepeatedFields(&monitoringpb.CreateTimeSeriesRequest{}, "time_series"),
				protocmp.FilterMessage(&dpb.Distribution{}, compareDistributionValue())); diff != "" {
				t.Errorf("got unexpected diff on requests (-want +got):\n%s", diff)
			}

		})
	}
}

type fakeClient struct {
	mu              sync.Mutex
	reqs            []*monitoringpb.CreateTimeSeriesRequest
	gcmReturnsError error
	// Set this to true if this fakeClient should fail fast and not store the incoming request
	// if gcmReturnsError is set.
	doNotStoreMetricsOnError bool
	metricsAllowList         map[string]struct{}
	requestTimeout           time.Duration
}

func (c *fakeClient) CreateServiceTimeSeries(ctx context.Context, in *monitoringpb.CreateTimeSeriesRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.doNotStoreMetricsOnError && c.gcmReturnsError != nil {
		return &emptypb.Empty{}, c.gcmReturnsError
	}

	deadline, ok := ctx.Deadline()
	if ok {
		c.requestTimeout = deadline.Sub(now)
	}
	allowedTimeSeries := []*mrpb.TimeSeries{}
	// If no allowlist was provided, we allow everything.
	if len(c.metricsAllowList) == 0 {
		allowedTimeSeries = in.GetTimeSeries()
	}

	for _, ts := range in.GetTimeSeries() {
		if _, ok := c.metricsAllowList[ts.GetMetric().GetType()]; ok {
			allowedTimeSeries = append(allowedTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
		}
	}

	if len(allowedTimeSeries) != 0 {
		c.reqs = append(c.reqs, &monitoringpb.CreateTimeSeriesRequest{Name: in.GetName(), TimeSeries: allowedTimeSeries})
	}
	return &emptypb.Empty{}, c.gcmReturnsError
}

func (c *fakeClient) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gcmReturnsError = err
}

func (c *fakeClient) requests() []*monitoringpb.CreateTimeSeriesRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reqs
}
