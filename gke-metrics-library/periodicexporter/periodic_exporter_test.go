package periodicexporter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gke-internal/gke-metrics/gke-metrics-library/logging"
	"gke-internal/gke-metrics/gke-metrics-library/metrics"

	mrpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	mpb "google.golang.org/genproto/googleapis/api/metric"
	mdrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestMetricsAreExportedPeriodically(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed creating logger: %q", err)
	}

	testcases := []struct {
		desc                   string
		exportInterval         time.Duration
		sleepFor               time.Duration
		wantExportedTimeSeries []*mrpb.TimeSeries
	}{
		{
			desc:           "0 scrape: 0 stream per metric",
			exportInterval: 10 * time.Second,
			// Don't sleep, so we make sure we finish the test before the export interval.
			sleepFor:               0,
			wantExportedTimeSeries: nil,
		},
		{
			desc:           "1 scrape: 1 stream per metric",
			exportInterval: 1 * time.Second,
			// add some jitter to make sure the exporter function is called.
			sleepFor: 1*time.Second + 500*time.Millisecond,
			wantExportedTimeSeries: []*mrpb.TimeSeries{
				cumulativeTimeSeries(k8sPodMonitoredResourceProto, map[string]string{"label": "value"}, 12),
				gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
			},
		},
		{
			desc:           "2 scrapes: 2 streams per metric",
			exportInterval: 1 * time.Second,
			// add some jitter to make sure the exporter function is called.
			sleepFor: 2*time.Second + 500*time.Millisecond,
			wantExportedTimeSeries: []*mrpb.TimeSeries{
				cumulativeTimeSeries(k8sPodMonitoredResourceProto, map[string]string{"label": "value"}, 12),
				cumulativeTimeSeries(k8sPodMonitoredResourceProto, map[string]string{"label": "value"}, 12),
				gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
				gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.Background()
			cumulative := metrics.NewCumulativeInt64(cumulativeDescriptor)
			gauge := metrics.NewGaugeDouble(gaugeDescriptor)
			cumulative.WithLabels(map[string]string{"label": "value"}).Add(12)
			gauge.SetWithoutLabels(3.14)

			metricsExporter := &metricsExporter{}
			exporter, err := New(Options{
				MetricsToExport: []*MetricAndMonitoredResource{
					MetricWithMonitoredResource(cumulative, &k8sPodMonitoredResource),
					Metric(gauge),
				},
				DefaultMonitoredResource: &k8sNodeMonitoredResource,
				ExportInterval:           tc.exportInterval,
				Logger:                   logger,
				GCMExporter:              metricsExporter,
			})
			if err != nil {
				t.Fatalf("Failed creating exporter: %q", err)
			}

			defer exporter.Shutdown(ctx)
			exporter.Start(ctx)
			time.Sleep(tc.sleepFor)

			opts := cmp.Options{
				protocmp.Transform(),
				cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
				protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
				protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
			}
			if diff := cmp.Diff(tc.wantExportedTimeSeries, metricsExporter.gotFlushedTimeSeries(), opts); diff != "" {
				t.Errorf("exporter.Start() exported metrics returned diff (-want/+got):\n%s", diff)
			}
		})
	}
}

func TestMetricsAreExportedOnce_WhenContextIsDone(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed creating logger: %q", err)
	}

	cumulative := metrics.NewCumulativeInt64(cumulativeDescriptor)
	gauge := metrics.NewGaugeDouble(gaugeDescriptor)

	cumulative.WithLabels(map[string]string{"label": "value"}).Add(12)
	gauge.SetWithoutLabels(3.14)

	wantExportedTimeSeries := []*mrpb.TimeSeries{
		cumulativeTimeSeries(k8sNodeMonitoredResourceProto, map[string]string{"label": "value"}, 12),
		gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
	}

	metricsExporter := &metricsExporter{}
	exporter, err := New(Options{
		MetricsToExport: []*MetricAndMonitoredResource{
			Metric(cumulative),
			Metric(gauge),
		},
		DefaultMonitoredResource: &k8sNodeMonitoredResource,
		// Set a very long export interval to make sure the ticker won't tick during this test
		// case.
		ExportInterval: time.Hour,
		Logger:         logger,
		GCMExporter:    metricsExporter,
	})
	if err != nil {
		t.Fatalf("Failed creating exporter: %q", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)
	defer exporter.Shutdown(ctx)
	// Cancel context right away to check if metrics are still flushed before the goroutine
	// returns.
	cancel()
	// Add some jitter to make sure that the goroutine exports metrics before it finishes.
	time.Sleep(time.Second)

	opts := cmp.Options{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(wantExportedTimeSeries, metricsExporter.gotFlushedTimeSeries(), opts); diff != "" {
		t.Errorf("exporter.Start() exported metrics returned diff (-want/+got):\n%s", diff)
	}
}

func TestMetricsAreExportedOnce_WhenContextIsCancelledDuringATick(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed creating logger: %q", err)
	}

	cumulative := metrics.NewCumulativeInt64(cumulativeDescriptor)
	gauge := metrics.NewGaugeDouble(gaugeDescriptor)

	cumulative.WithLabels(map[string]string{"label": "value"}).Add(12)
	gauge.SetWithoutLabels(3.14)

	wantExportedTimeSeries := []*mrpb.TimeSeries{
		cumulativeTimeSeries(k8sNodeMonitoredResourceProto, map[string]string{"label": "value"}, 12),
		gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
	}

	metricsExporter := &metricsExporter{}
	exporter, err := New(Options{
		MetricsToExport: []*MetricAndMonitoredResource{
			Metric(cumulative),
			Metric(gauge),
		},
		DefaultMonitoredResource: &k8sNodeMonitoredResource,
		ExportInterval:           time.Second,
		Logger:                   logger,
		GCMExporter:              metricsExporter,
	})
	if err != nil {
		t.Fatalf("Failed creating exporter: %q", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)
	defer exporter.Shutdown(ctx)

	// Lock the exporter's mutex and sleep for 2 scrape intervals, to make sure the exporter ticker
	// will tick at least once and get blocked with a valid context.
	exporter.mu.Lock()
	time.Sleep(2 * time.Second)
	// Then, cancel the context and unlock the mutex. The exporter should get unblocked and still
	// be able to export the last set of metrics one last time.
	cancel()
	exporter.mu.Unlock()
	// Add some jitter to make sure that the goroutine exports metrics before it finishes.
	time.Sleep(time.Second)

	opts := cmp.Options{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(wantExportedTimeSeries, metricsExporter.gotFlushedTimeSeries(), opts); diff != "" {
		t.Errorf("exporter.Start() exported metrics returned diff (-want/+got):\n%s", diff)
	}
}

func TestMetricsAreExportedOnce_WhenShutdownIsCalled(t *testing.T) {
	ctx := context.Background()
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed creating logger: %q", err)
	}
	cumulative := metrics.NewCumulativeInt64(cumulativeDescriptor)
	gauge := metrics.NewGaugeDouble(gaugeDescriptor)
	cumulative.WithLabels(map[string]string{"label": "value"}).Add(12)
	gauge.SetWithoutLabels(3.14)

	wantExportedTimeSeries := []*mrpb.TimeSeries{
		cumulativeTimeSeries(k8sNodeMonitoredResourceProto, map[string]string{"label": "value"}, 12),
		gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
	}

	metricsExporter := &metricsExporter{}
	exporter, err := New(Options{
		MetricsToExport: []*MetricAndMonitoredResource{
			Metric(cumulative),
			Metric(gauge),
		},
		DefaultMonitoredResource: &k8sNodeMonitoredResource,
		// Set a very long export interval to make sure the ticker won't tick during this test
		// case.
		ExportInterval: time.Hour,
		Logger:         logger,
		GCMExporter:    metricsExporter,
	})
	if err != nil {
		t.Fatalf("Failed creating exporter: %q", err)
	}
	exporter.Start(ctx)
	exporter.Shutdown(ctx)
	// Add some jitter to make sure that the goroutine exports metrics before it finishes.
	time.Sleep(time.Second)

	opts := cmp.Options{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(wantExportedTimeSeries, metricsExporter.gotFlushedTimeSeries(), opts); diff != "" {
		t.Errorf("exporter.Start() exported metrics returned diff (-want/+got):\n%s", diff)
	}
}

func TestMetricsExport_FailsGracefullyAndRecovers(t *testing.T) {
	ctx := context.Background()
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed creating logger: %q", err)
	}

	cumulative := metrics.NewCumulativeInt64(cumulativeDescriptor)
	gauge := metrics.NewGaugeDouble(gaugeDescriptor)
	cumulative.WithLabels(map[string]string{"label": "value"}).Add(12)
	gauge.SetWithoutLabels(3.14)

	// First, expect the GCM exporter to fail all requests.
	metricsExporter := &metricsExporter{
		exportTimeSeriesError: errors.New("error"),
		flushError:            errors.New("error"),
	}
	exporter, err := New(Options{
		MetricsToExport: []*MetricAndMonitoredResource{
			Metric(cumulative),
			Metric(gauge),
		},
		DefaultMonitoredResource: &k8sNodeMonitoredResource,
		ExportInterval:           time.Second,
		Logger:                   logger,
		GCMExporter:              metricsExporter,
	})
	if err != nil {
		t.Fatalf("Failed creating exporter: %q", err)
	}

	exporter.Start(ctx)
	defer exporter.Shutdown(ctx)
	// Sleep for a scraping interval + some jitter to ensure the ticker ticks at least once and the
	// application doesn't crash during a failed Export of Flush.
	time.Sleep(1*time.Second + 100*time.Millisecond)

	// Now, the exporter no longer returns an error. The periodic exporter should recover and
	// send metrics.
	metricsExporter.withError(nil, nil)
	// Add some jitter to make sure that the goroutine exports metrics before it finishes.
	time.Sleep(time.Second + 100*time.Millisecond)

	wantExportedTimeSeries := []*mrpb.TimeSeries{
		cumulativeTimeSeries(k8sNodeMonitoredResourceProto, map[string]string{"label": "value"}, 12),
		gaugeTimeSeries(k8sNodeMonitoredResourceProto, 3.14),
	}
	opts := cmp.Options{
		protocmp.Transform(),
		cmpopts.SortSlices(func(a, b *mrpb.TimeSeries) bool { return a.String() < b.String() }),
		protocmp.IgnoreFields(&mrpb.Point{}, "interval"),
		protocmp.IgnoreFields(&mrpb.TimeSeries{}, "unit"),
	}
	if diff := cmp.Diff(wantExportedTimeSeries, metricsExporter.gotFlushedTimeSeries(), opts); diff != "" {
		t.Errorf("exporter.Start() exported metrics returned diff (-want/+got):\n%s", diff)
	}
}

func TestNew(t *testing.T) {
	testcases := []struct {
		desc         string
		options      Options
		wantExporter *Exporter
	}{
		{
			desc: "ExportInterval is set to 60s if missing",
			options: Options{
				DefaultMonitoredResource: &k8sNodeMonitoredResource,
				Logger:                   zap.NewNop(),
			},
			wantExporter: &Exporter{
				exportInterval: time.Minute,
				logger:         logging.RateLimitedLogger(zap.NewNop(), 10*time.Minute, 2),
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gotExporter, err := New(tc.options)
			if err != nil {
				t.Errorf("New(%v) = %v, want nil error", tc.options, err)
			}
			cmpOptions := cmp.Options{
				cmp.AllowUnexported(Exporter{}, metrics.Resource{}, zap.Logger{}),
				cmpopts.IgnoreFields(Exporter{}, "mu"),
				cmpopts.IgnoreFields(zap.Logger{}, "core"),
			}
			if diff := cmp.Diff(tc.wantExporter, gotExporter, cmpOptions); diff != "" {
				t.Errorf("New(%v) returned diff (-want/+got):\n%s", tc.options, diff)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	testcases := []struct {
		desc    string
		options Options
	}{
		{
			desc: "DefaultMonitoredResource missing",
			options: Options{
				MetricsToExport: []*MetricAndMonitoredResource{},
				ExportInterval:  time.Hour,
				Logger:          zap.NewNop(),
			},
		},
		{
			desc: "Logger missing",
			options: Options{
				MetricsToExport:          []*MetricAndMonitoredResource{},
				DefaultMonitoredResource: &k8sNodeMonitoredResource,
				ExportInterval:           time.Hour,
			},
		},
		{
			desc: "Nil MetricAndMonitoredResource",
			options: Options{
				MetricsToExport:          []*MetricAndMonitoredResource{nil},
				Logger:                   zap.NewNop(),
				DefaultMonitoredResource: &k8sNodeMonitoredResource,
				ExportInterval:           time.Hour,
			},
		},
		{
			desc: "Nil metric",
			options: Options{
				MetricsToExport:          []*MetricAndMonitoredResource{Metric(nil)},
				Logger:                   zap.NewNop(),
				DefaultMonitoredResource: &k8sNodeMonitoredResource,
				ExportInterval:           time.Hour,
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			if _, err := New(tc.options); err == nil {
				t.Errorf("New(%v) = nil, want error", tc.options)
			}
		})
	}
}

type metricsExporter struct {
	mu sync.Mutex

	exportedTimeSeries    []*mrpb.TimeSeries
	exportTimeSeriesError error

	flushedTimeSeries []*mrpb.TimeSeries
	flushError        error
}

func (me *metricsExporter) ExportTimeSeries(ctx context.Context, ts *mrpb.TimeSeries) error {
	// Do not store anything if the context is done, so we can test this package's behavior when the
	// caller cancels the context.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.exportTimeSeriesError != nil {
		return me.exportTimeSeriesError
	}
	me.exportedTimeSeries = append(me.exportedTimeSeries, proto.Clone(ts).(*mrpb.TimeSeries))
	return nil
}

func (me *metricsExporter) Flush(ctx context.Context) error {
	// Do not store anything if the context is done, so we can test this package's behavior when the
	// caller cancels the context.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.flushError != nil {
		return me.flushError
	}
	// "Flush" metrics, freeing up the exportedTimeSeries buffer.
	me.flushedTimeSeries = append(me.flushedTimeSeries, me.exportedTimeSeries...)
	me.exportedTimeSeries = []*mrpb.TimeSeries{}
	return nil
}

func (me *metricsExporter) Shutdown(_ context.Context) error {
	return nil
}

func (me *metricsExporter) withError(exportTimeSeriesError, flushError error) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.exportTimeSeriesError = exportTimeSeriesError
	me.flushError = flushError
}

func (me *metricsExporter) gotFlushedTimeSeries() []*mrpb.TimeSeries {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me.flushedTimeSeries
}

var (
	cumulativeDescriptor = metrics.DescriptorOpts{
		Name:   "cumulative_metric",
		Labels: []metrics.LabelDescriptor{{Name: "label"}},
	}
	gaugeDescriptor = metrics.DescriptorOpts{
		Name:   "gauge_metric",
		Labels: []metrics.LabelDescriptor{{Name: "label"}},
	}

	k8sNodeMonitoredResource = metrics.NewK8sNode("project", "location", "cluster_name", "node_name")
	k8sPodMonitoredResource  = metrics.NewK8sPod("project", "location", "cluster_name", "namespace",
		"pod_name")

	k8sNodeMonitoredResourceProto = &mdrpb.MonitoredResource{
		Type: "k8s_node",
		Labels: map[string]string{
			"project_id":   "project",
			"location":     "location",
			"cluster_name": "cluster_name",
			"node_name":    "node_name",
		},
	}
	k8sPodMonitoredResourceProto = &mdrpb.MonitoredResource{
		Type: "k8s_pod",
		Labels: map[string]string{
			"project_id":     "project",
			"location":       "location",
			"cluster_name":   "cluster_name",
			"namespace_name": "namespace",
			"pod_name":       "pod_name",
		},
	}

	cumulativeTimeSeries = func(monitoredResource *mdrpb.MonitoredResource, labels map[string]string, value int64) *mrpb.TimeSeries {
		return &mrpb.TimeSeries{
			Metric: &mpb.Metric{
				Type:   "cumulative_metric",
				Labels: labels,
			},
			Resource:   monitoredResource,
			MetricKind: mpb.MetricDescriptor_CUMULATIVE,
			ValueType:  mpb.MetricDescriptor_INT64,
			Points: []*mrpb.Point{metrics.CreateCumulativePoint(metrics.CreateInt64TypedValue(value),
				time.Time{}, time.Time{})},
		}
	}

	gaugeTimeSeries = func(monitoredResource *mdrpb.MonitoredResource, value float64) *mrpb.TimeSeries {
		return &mrpb.TimeSeries{
			Metric: &mpb.Metric{
				Type: "gauge_metric",
			},
			Resource:   monitoredResource,
			MetricKind: mpb.MetricDescriptor_GAUGE,
			ValueType:  mpb.MetricDescriptor_DOUBLE,
			Points: []*mrpb.Point{metrics.CreateGaugePoint(metrics.CreateDoubleTypedValue(value),
				time.Time{})},
		}
	}
)
