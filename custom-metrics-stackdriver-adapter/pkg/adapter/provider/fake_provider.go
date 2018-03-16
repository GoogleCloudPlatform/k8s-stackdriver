/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

var (
	testingMetrics = []externalMetric{
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "bar",
				},
				Value: *resource.NewQuantity(42, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
				Labels: map[string]string{
					"foo": "baz",
				},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "baz",
				},
				Value: *resource.NewQuantity(43, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "other-external-metric",
				Labels: map[string]string{},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName:   "other-external-metric",
				MetricLabels: map[string]string{},
				Value:        *resource.NewQuantity(44, resource.DecimalSI),
			},
		},
	}
)

type fakeProvider struct {
	externalMetrics []externalMetric
}

// NewFakeProvider returns a fake ExternalMetricsProvider, intended to use for testing purpose
func NewFakeProvider() provider.ExternalMetricsProvider {
	return &fakeProvider{
		externalMetrics: testingMetrics,
	}
}

func (p *fakeProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}
	for _, metric := range p.externalMetrics {
		if metric.info.Metric == metricName &&
			metricSelector.Matches(labels.Set(metric.info.Labels)) {
			metricValue := metric.value
			metricValue.Timestamp = metav1.Now()
			matchingMetrics = append(matchingMetrics, metricValue)
		}
	}
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

func (p *fakeProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	externalMetricsInfo := []provider.ExternalMetricInfo{}
	for _, metric := range p.externalMetrics {
		externalMetricsInfo = append(externalMetricsInfo, metric.info)
	}
	return externalMetricsInfo
}
