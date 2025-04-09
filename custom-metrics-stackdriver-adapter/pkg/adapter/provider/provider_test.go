package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	sd "google.golang.org/api/monitoring/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testProjectID  = "test-project"
	testMetricName = "external.googleapis.com/testmetric"
	testNamespace  = "default"
	testLabelKey   = "label1"
	testLabelValue = "value1"
	resourceType   = "global"
)

var (
	now                 = time.Now()
	testSelector        = labels.SelectorFromSet(labels.Set{"resource.labels.project_id": testProjectID})
	testMetricInfo      = provider.ExternalMetricInfo{Metric: strings.TrimPrefix(testMetricName, "external.googleapis.com/")}
	testValueInt64      = int64(100)
	wantMetricValueList = &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{
			{
				MetricName: strings.TrimPrefix(testMetricName, "external.googleapis.com/"),
				Value:      resource.MustParse("100"),
				Timestamp:  metav1.NewTime(now),
				MetricLabels: map[string]string{
					"metric.labels." + testLabelKey: testLabelValue,
					"resource.labels.project_id":    testProjectID,
					"resource.type":                 resourceType,
				},
			},
		},
	}
	baseTimeSeriesResponse = &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Metric: &sd.Metric{
					Type:   testMetricName,
					Labels: map[string]string{testLabelKey: testLabelValue},
				},
				Points: []*sd.Point{
					{
						Value: &sd.TypedValue{Int64Value: &testValueInt64},
						Interval: &sd.TimeInterval{
							EndTime: now.Format(time.RFC3339Nano),
						},
					},
				},
				Resource: &sd.MonitoredResource{
					Type:   resourceType,
					Labels: map[string]string{"project_id": testProjectID},
				},
			},
		},
	}
)

func TestStackdriverProvider_GetExternalMetric(t *testing.T) {
	tests := []struct {
		name             string
		useCache         bool
		cachedValue      *external_metrics.ExternalMetricValueList
		mockRoundTripper http.RoundTripper
		wantCacheAdd     bool
		wantResponse     *external_metrics.ExternalMetricValueList
	}{
		{
			name:             "cache hit",
			useCache:         true,
			cachedValue:      wantMetricValueList,
			mockRoundTripper: &mockExternalMetricRoundTripper{},
			wantResponse:     wantMetricValueList,
		},
		{
			name:     "cache miss, successful fetch",
			useCache: true,
			mockRoundTripper: newMockExternalMetricRoundTripper(
				testMetricName,
				testSelector,
				baseTimeSeriesResponse,
			),
			wantResponse: wantMetricValueList,
			wantCacheAdd: true,
		},
		{
			name:     "cache disabled, successful fetch",
			useCache: false,
			mockRoundTripper: newMockExternalMetricRoundTripper(
				testMetricName,
				testSelector,
				baseTimeSeriesResponse,
			),
			wantResponse: wantMetricValueList,
			wantCacheAdd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockSDService := translator.NewMockStackdriverService(t, tt.mockRoundTripper)
			fakeTranslator := newFakeTranslator(t, mockSDService)
			p := &StackdriverProvider{
				stackdriverService: mockSDService,
				config:             &config.GceConfig{Project: testProjectID},
				translator:         fakeTranslator,
			}
			key := cacheKey{
				namespace:      testNamespace,
				metricSelector: testSelector.String(),
				info:           testMetricInfo,
			}
			if tt.useCache {
				mockCache := newExternalMetricsCache(1, time.Minute)
				if tt.cachedValue != nil {
					mockCache.add(key, tt.cachedValue)
				}
				p.externalMetricsCache = mockCache
			}

			// Act
			gotResponse, err := p.GetExternalMetric(context.Background(), testNamespace, testSelector, testMetricInfo)
			if err != nil {
				t.Errorf("GetExternalMetric() error = %v", err)
				return
			}

			// Assert
			if len(gotResponse.Items) != len(tt.wantResponse.Items) {
				t.Errorf("GetExternalMetric() got %d items, want %d", len(gotResponse.Items), len(tt.wantResponse.Items))
				return
			}

			for i := range gotResponse.Items {
				if !compareExternalMetricValue(gotResponse.Items[i], tt.wantResponse.Items[i]) {
					t.Errorf("GetExternalMetric() item[%d] got = %v, want %v (ignoring Timestamp)", i, gotResponse.Items[i], tt.wantResponse.Items[i])
				}
			}

			if tt.useCache && p.externalMetricsCache != nil {
				if tt.wantCacheAdd {
					_, added := p.externalMetricsCache.get(key)
					if !added {
						t.Errorf("wanted cache add but didn't happen")
					}
				}
			}
		})
	}
}

func TestStackdriverProvider_GetExternalMetric_CacheExpiration(t *testing.T) {
	externalCacheTTL := time.Second
	externalCacheSize := 1
	mockCache := newExternalMetricsCache(externalCacheSize, externalCacheTTL)
	mockRoundTripper := newMockExternalMetricRoundTripper(
		testMetricName,
		testSelector,
		baseTimeSeriesResponse,
	)
	mockSDService := translator.NewMockStackdriverService(t, mockRoundTripper)
	fakeTranslator := newFakeTranslator(t, mockSDService)
	p := &StackdriverProvider{
		stackdriverService:   mockSDService,
		config:               &config.GceConfig{Project: testProjectID},
		translator:           fakeTranslator,
		externalMetricsCache: mockCache,
	}

	// Verify that entry does not exist in cache at first.
	key := cacheKey{
		namespace:      testNamespace,
		metricSelector: testSelector.String(),
		info:           testMetricInfo,
	}
	_, cacheHit := p.externalMetricsCache.get(key)
	if cacheHit != false {
		t.Errorf("cache hit got = %v, want %v on first fetch", cacheHit, false)
	}

	// Should still be able to get external metric.
	firstResponse, err := p.GetExternalMetric(context.Background(), testNamespace, testSelector, testMetricInfo)
	if err != nil {
		t.Errorf("GetExternalMetric() error = %v", err)
		return
	}
	if !compareExternalMetricValue(firstResponse.Items[0], wantMetricValueList.Items[0]) {
		t.Errorf("GetExternalMetric() item[0] got = %v, want %v (ignoring Timestamp) on first fetch", firstResponse.Items[0], wantMetricValueList.Items[0])
	}

	// Verify entry is now added to the cache.
	_, cacheHit = p.externalMetricsCache.get(key)
	if cacheHit != true {
		t.Errorf("cache hit got = %v, want %v on second fetch", cacheHit, false)
	}

	// Wait for cache entry to expire and verify that its no longer in the cache.
	time.Sleep(externalCacheTTL + time.Second)
	_, cacheHit = p.externalMetricsCache.get(key)
	if cacheHit != false {
		t.Errorf("cache hit after expiration got = %v, want %v", cacheHit, false)
	}

	// Should still be able to get the external metric.
	secondResponse, err := p.GetExternalMetric(context.Background(), testNamespace, testSelector, testMetricInfo)
	if err != nil {
		t.Errorf("GetExternalMetric() error = %v after expiration", err)
		return
	}
	if !compareExternalMetricValue(secondResponse.Items[0], wantMetricValueList.Items[0]) {
		t.Errorf("GetExternalMetric() item[0] got = %v, want %v (ignoring Timestamp) after expiration", secondResponse.Items[0], wantMetricValueList.Items[0])
	}
}

// newFakeTranslator creates a fake translator for testing.
func newFakeTranslator(t *testing.T, mockSDService *sd.Service) *translator.Translator {
	return translator.NewFakeTranslatorForExternalMetricsWithSDService(time.Minute, time.Minute, testProjectID, now, mockSDService)
}

// newMockExternalMetricRoundTripper creates a configured mock RoundTripper for external metrics.
func newMockExternalMetricRoundTripper(wantMetricName string, wantSelector labels.Selector, response *sd.ListTimeSeriesResponse) http.RoundTripper {
	return &mockExternalMetricRoundTripper{
		wantMetricName: wantMetricName,
		wantSelector:   wantSelector,
		response:       response,
	}
}

// Mock RoundTripper for simulating Stackdriver API calls for external metrics
type mockExternalMetricRoundTripper struct {
	wantMetricName string
	wantSelector   labels.Selector
	response       *sd.ListTimeSeriesResponse
	err            error
}

func (m *mockExternalMetricRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return &http.Response{StatusCode: http.StatusInternalServerError}, m.err
	}

	if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/metricDescriptors/") {
		parts := strings.Split(req.URL.Path, "/")
		if len(parts) == 6 && parts[4] == "metricDescriptors" {
			requestedMetricName := parts[5]
			if requestedMetricName == strings.ReplaceAll(m.wantMetricName, "external.googleapis.com/", "") {
				descriptorResponse := &sd.MetricDescriptor{
					Name:        fmt.Sprintf("projects/%s/metricDescriptors/%s", testProjectID, requestedMetricName),
					Type:        m.wantMetricName,
					MetricKind:  "GAUGE",
					ValueType:   "INT64",
					Description: "A test metric",
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
				resp.Header.Set("Content-Type", "application/json")
				respBody, err := descriptorResponse.MarshalJSON()
				if err != nil {
					return nil, err
				}
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
				return resp, nil
			} else {
				return &http.Response{StatusCode: http.StatusNotFound}, nil
			}
		}
		return &http.Response{StatusCode: http.StatusBadRequest}, nil
	}

	if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/timeSeries") {
		filter := req.URL.Query().Get("filter")
		wantMetricFilter := `metric.type = "` + strings.TrimPrefix(m.wantMetricName, "external.googleapis.com/") + `"`
		wantProjectFilter := `resource.labels.project_id = "test-project"`
		if strings.Contains(filter, wantMetricFilter) && strings.Contains(filter, wantProjectFilter) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "application/json")

			respBody, err := m.response.MarshalJSON()
			if err != nil {
				return nil, err
			}
			resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
			return resp, nil
		} else {
			return &http.Response{StatusCode: http.StatusBadRequest}, fmt.Errorf("unexpected filter")
		}
	}

	return &http.Response{StatusCode: http.StatusNotImplemented}, nil
}

func compareExternalMetricValue(a, b external_metrics.ExternalMetricValue) bool {
	if a.MetricName != b.MetricName {
		return false
	}
	if a.Value.Cmp(b.Value) != 0 {
		return false
	}
	return reflect.DeepEqual(a.MetricLabels, b.MetricLabels)
}
