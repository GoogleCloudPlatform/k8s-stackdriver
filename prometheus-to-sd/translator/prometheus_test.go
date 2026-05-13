package translator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

func TestPrometheusResponseBuild(t *testing.T) {
	// Create a valid Prometheus metrics response
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE test_metric counter
test_metric{label="value"} 42.0
# TYPE gauge_metric gauge
gauge_metric 123.5
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, 2, len(metrics))
	assert.Contains(t, metrics, "test_metric")
	assert.Contains(t, metrics, "gauge_metric")
}

func TestPrometheusResponseBuildWithSummary(t *testing.T) {
	// Create a response with summary metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890.0
# TYPE summary_metric summary
summary_metric{quantile="0.5"} 4.0
summary_metric{quantile="0.9"} 8.0
summary_metric_sum 42.0
summary_metric_count 10
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// Summary metrics should be flattened into _sum and _count
	assert.Equal(t, 3, len(metrics))
	assert.Contains(t, metrics, "summary_metric_sum")
	assert.Contains(t, metrics, "summary_metric_count")
}

func TestPrometheusResponseBuildWithInvalidFormat(t *testing.T) {
	// Create a response with invalid content type
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE test_metric counter
test_metric{label="value"} 42.0
`),
		header: http.Header{"Content-Type": []string{"text/html"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	_, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse format from header")
}

func TestPrometheusResponseBuildWithMalformedMetrics(t *testing.T) {
	// Create a response with malformed metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE test_metric counter
test_metric{label="value"
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	_, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.Error(t, err)
}

func TestPrometheusResponseBuildWithOmitComponentName(t *testing.T) {
	// Create a response with component-prefixed metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE test_component_test_metric counter
test_component_test_metric{label="value"} 42.0
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test_component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   true,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// Component name prefix should be removed
	assert.Equal(t, 1, len(metrics))
	assert.Contains(t, metrics, "test_metric")
}

func TestPrometheusResponseBuildWithDowncaseMetricNames(t *testing.T) {
	// Create a response with mixed case metric names
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE TestMetric counter
TestMetric{label="value"} 42.0
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: true,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// Metric names should be downcased
	assert.Equal(t, 1, len(metrics))
	assert.Contains(t, metrics, "testmetric")
}

func TestPrometheusResponseBuildWithWhitelistedMetrics(t *testing.T) {
	// Create a response with multiple metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE metric_a counter
metric_a{label="value"} 42.0
# TYPE metric_b counter
metric_b{label="value"} 123.0
# TYPE metric_c counter
metric_c{label="value"} 456.0
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{"metric_a", "metric_c"},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// Whitelisted metrics should be present
	assert.Contains(t, metrics, "metric_a")
	assert.Contains(t, metrics, "metric_c")
}

func TestPrometheusResponseBuildWithCustomMetricsPrefix(t *testing.T) {
	// Create a response with custom metrics prefix
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE test_metric counter
test_metric{label="value"} 42.0
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "custom.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// For custom metrics, metric descriptors should be validated
	assert.Equal(t, 1, len(metrics))
}

func TestPrometheusResponseBuildWithEmptyResponse(t *testing.T) {
	// Create an empty response
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(``),
		header:      http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	// Empty response should result in empty metrics map
	assert.NotNil(t, metrics)
	assert.Equal(t, 0, len(metrics))
}

func TestPrometheusResponseBuildWithHistogram(t *testing.T) {
	// Create a response with histogram metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890.0
# TYPE histogram_metric histogram
histogram_metric_bucket{le="1.0"} 1
histogram_metric_bucket{le="3.0"} 4
histogram_metric_bucket{le="+Inf"} 5
histogram_metric_sum 13.0
histogram_metric_count 5
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	// Histogram metrics should be preserved
	assert.Contains(t, metrics, "histogram_metric")
}

func TestPrometheusResponseBuildWithUntyped(t *testing.T) {
	// Create a response with untyped metrics
	metricsResponse := &PrometheusResponse{
		rawResponse: []byte(`
# TYPE untyped_metric untyped
untyped_metric 42.5
`),
		header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}},
	}

	commonConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			Component:            "test-component",
			MetricsPrefix:        "container.googleapis.com",
			Whitelisted:          []string{},
			WhitelistedLabelsMap: make(map[string]map[string]bool),
			PodConfig:            config.NewPodConfig("", "", "", "", "", "", "", ""),
		},
		OmitComponentName:   false,
		DowncaseMetricNames: false,
	}

	cache := NewMetricDescriptorCache(nil, commonConfig)
	metrics, err := metricsResponse.Build(context.Background(), commonConfig, cache)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Contains(t, metrics, "untyped_metric")
}

func TestScrapePrometheusMetrics(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer s.Close()
	for n := 0; n < 100000; n++ {
		resp, err := doPrometheusRequest(s.URL, config.AuthConfig{})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		resp.Body.Close()
	}
}
