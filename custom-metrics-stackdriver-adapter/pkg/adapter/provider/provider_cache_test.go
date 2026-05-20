package provider

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

func TestExternalMetricCache_PreventMutation(t *testing.T) {
	projectID := testProjectID  // newMockExternalMetricRoundTripper hardcodes this value.
	shortMetricName := "testmetric"
	fullMetricName := "external.googleapis.com/" + shortMetricName
	mockRoundTripper := newMockExternalMetricRoundTripper(
		fullMetricName,
		labels.Everything(),
		&sd.ListTimeSeriesResponse{
			TimeSeries: []*sd.TimeSeries{
				{
					Metric: &sd.Metric{
						Type:   fullMetricName,
					},
					Points: []*sd.Point{
						{
							Value: &sd.TypedValue{Int64Value: new(int64(100))},
							Interval: &sd.TimeInterval{
								EndTime: time.Now().Format(time.RFC3339Nano),
							},
						},
					},
					Resource: &sd.MonitoredResource{},
				},
			},
		},
	)
	mockSDService := translator.NewMockStackdriverService(t, mockRoundTripper)
	fakeTranslator := newFakeTranslator(t, mockSDService)
	p := &StackdriverProvider{
		stackdriverService: mockSDService,
		config:             &config.GceConfig{Project: projectID},
		translator:         fakeTranslator,
	}
	p.externalMetricsCache = newExternalMetricsCache(1, time.Minute)

	// First call: Cache Miss, fetches from SD, stores in cache.
	namespace := "default"
	selector := labels.SelectorFromSet(labels.Set{"resource.labels.project_id": projectID})
	metricInfo := provider.ExternalMetricInfo{Metric: shortMetricName}
	resp1, err := p.GetExternalMetric(context.Background(), namespace, selector, metricInfo)
	if err != nil {
		t.Fatalf("First GetExternalMetric failed: %v", err)
	}

	if len(resp1.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp1.Items))
	}

	// Mutate the returned response (simulating apiserver mutation)
	resp1.Items[0].MetricName = "MUTATED"

	// Second call: Cache Hit, should return a copy with the MetricName intact
	resp2, err := p.GetExternalMetric(context.Background(), namespace, selector, metricInfo)
	if err != nil {
		t.Fatalf("Second GetExternalMetric failed: %v", err)
	}

	if resp2.Items[0].MetricName != shortMetricName {
		t.Errorf("GetExternalMetric().Items[0].MetricName = %q, want %q", resp2.Items[0].MetricName, shortMetricName)
	}
}

func TestExternalMetricCache_ConcurrentAccess(t *testing.T) {
	projectID := testProjectID  // newMockExternalMetricRoundTripper hardcodes this value.
	shortMetricName := "testmetric"
	fullMetricName := "external.googleapis.com/" + shortMetricName
	mockRoundTripper := newMockExternalMetricRoundTripper(
		fullMetricName,
		labels.Everything(),
		&sd.ListTimeSeriesResponse{
			TimeSeries: []*sd.TimeSeries{
				{
					Metric: &sd.Metric{
						Type:   fullMetricName,
					},
					Points: []*sd.Point{
						{
							Value: &sd.TypedValue{Int64Value: new(int64(100))},
							Interval: &sd.TimeInterval{
								EndTime: time.Now().Format(time.RFC3339Nano),
							},
						},
					},
					Resource: &sd.MonitoredResource{},
				},
			},
		},
	)
	mockSDService := translator.NewMockStackdriverService(t, mockRoundTripper)
	fakeTranslator := newFakeTranslator(t, mockSDService)
	p := &StackdriverProvider{
		stackdriverService: mockSDService,
		config:             &config.GceConfig{Project: projectID},
		translator:         fakeTranslator,
	}
	p.externalMetricsCache = newExternalMetricsCache(1, time.Minute)

	// Warm up cache
	namespace := "default"
	selector := labels.SelectorFromSet(labels.Set{"resource.labels.project_id": projectID})
	metricInfo := provider.ExternalMetricInfo{Metric: shortMetricName}
	_, err := p.GetExternalMetric(context.Background(), namespace, selector, metricInfo)
	if err != nil {
		t.Fatalf("Failed to warm up cache: %v", err)
	}

	// Concurrently access the same value from multiple goroutines to check for data races
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := p.GetExternalMetric(context.Background(), namespace, selector, metricInfo)
			if err == nil && len(resp.Items) > 0 {
				// Mutate concurrently. If cache returns same pointer, this will race under -race
				resp.Items[0].MetricName = "MUTATED_CONCURRENT"
			}
		}()
	}
	wg.Wait()
}
