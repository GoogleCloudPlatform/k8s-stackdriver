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

package translator

import (
	"reflect"
	"testing"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

func TestTranslator_GetRespForNodes_Aggregation(t *testing.T) {
	translator, _ :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var valPart1 float64 = 101
	var valPart2 float64 = 50
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "DOUBLE",
				Metric: &sd.Metric{
					Type:   "my|custom|metric",
					Labels: map[string]string{"irrelevant_label": "value1"},
				},
				Points: []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{DoubleValue: &valPart1}}},
			},
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "DOUBLE",
				Metric: &sd.Metric{
					Type:   "my|custom|metric",
					Labels: map[string]string{"irrelevant_label": "value2"},
				},
				Points: []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{DoubleValue: &valPart2}}},
			},
		},
	}
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-node-name",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	metrics, err := translator.GetRespForMultipleObjects(response, translator.GetNodeItems(&v1.NodeList{Items: []v1.Node{node}}), schema.GroupResource{Resource: "Node"}, "my|custom|metric", metricSelector)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedMetricSelector, _ := metav1.ParseToLabelSelector("metric.labels.custom=test")
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewMilliQuantity(151000, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-node-name", APIVersion: "/__internal", Kind: "Node"},
			Metric: custom_metrics.MetricIdentifier{
				Name:     "my|custom|metric",
				Selector: expectedMetricSelector,
			},
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metrics[i])
		}
	}
}

func TestTranslator_GetRespForPod_legacyResourceType(t *testing.T) {
	translator, _ :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	var metricValue int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &metricValue}}},
		}},
	}
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	metric, err := translator.GetRespForSingleObject(response, schema.GroupResource{Resource: "Pod", Group: ""}, "my/custom/metric", metricSelector, "my-namespace", "my-pod-name")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetricSelector, _ := metav1.ParseToLabelSelector("metric.labels.custom=test")
	expectedMetric := &custom_metrics.MetricValue{
		Value:           *resource.NewQuantity(151, resource.DecimalSI),
		Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
		DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
		Metric: custom_metrics.MetricIdentifier{
			Name:     "my/custom/metric",
			Selector: expectedMetricSelector,
		},
	}
	if !reflect.DeepEqual(*metric, *expectedMetric) {
		t.Errorf("Expected: \n%v,\n received: \n%v", expectedMetric, metric)
	}
}

func TestTranslator_GetRespForPods_legacyResourceType(t *testing.T) {
	translator, _ :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	var val int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &val}}},
		}},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name",
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	metrics, err := translator.GetRespForMultipleObjects(response, translator.GetPodItems(&v1.PodList{Items: []v1.Pod{pod}}), schema.GroupResource{Resource: "Pod"}, "my/custom/metric", metricSelector)
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetricSelector, _ := metav1.ParseToLabelSelector("metric.labels.custom=test")
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
			Metric: custom_metrics.MetricIdentifier{
				Name:     "my/custom/metric",
				Selector: expectedMetricSelector,
			},
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metrics[i])
		}
	}
}

func TestTranslator_GetRespForCustomMetric_MultiplePoints(t *testing.T) {
	translator, _ :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	var val1 int64 = 151
	var val2 int64 = 117
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points: []*sd.Point{
				{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &val1}},
				{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{Int64Value: &val2}},
			},
		}},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name",
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	metrics, err := translator.GetRespForMultipleObjects(response, translator.GetPodItems(&v1.PodList{Items: []v1.Pod{pod}}), schema.GroupResource{Resource: "Pod"}, "my/custom/metric", metricSelector)
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetricSelector, _ := metav1.ParseToLabelSelector("metric.labels.custom=test")
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
			Metric: custom_metrics.MetricIdentifier{
				Name:     "my/custom/metric",
				Selector: expectedMetricSelector,
			},
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metrics[i])
		}
	}
}

func TestTranslator_GetMetricsFromSDDescriptorsResp(t *testing.T) {
	translator, _ :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	response := &sd.ListMetricDescriptorsResponse{
		MetricDescriptors: []*sd.MetricDescriptor{
			{Type: "custom.googleapis.com/qps-int", MetricKind: "GAUGE", ValueType: "INT64"},
			{Type: "custom.googleapis.com/qps-double", MetricKind: "GAUGE", ValueType: "DOUBLE"},
			{Type: "custom.googleapis.com/qps-string", MetricKind: "GAUGE", ValueType: "STRING"},
			{Type: "custom.googleapis.com/qps-bool", MetricKind: "GAUGE", ValueType: "BOOL"},
			{Type: "custom.googleapis.com/qps-dist", MetricKind: "GAUGE", ValueType: "DISTRIBUTION"},
			{Type: "custom.googleapis.com/qps-money", MetricKind: "GAUGE", ValueType: "MONEY"},
			{Type: "custom.googleapis.com/qps-delta", MetricKind: "DELTA", ValueType: "INT64"},
			{Type: "custom.googleapis.com/qps-cum", MetricKind: "CUMULATIVE", ValueType: "INT64"},
			{Type: "custom.googleapis.com/q/p/s", MetricKind: "GAUGE", ValueType: "INT64"},
		},
	}
	metricInfo := translator.GetMetricsFromSDDescriptorsResp(response)
	expectedMetricInfo := []provider.CustomMetricInfo{
		{Metric: "custom.googleapis.com|qps-int", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
		{Metric: "custom.googleapis.com|qps-double", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
		{Metric: "custom.googleapis.com|qps-delta", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
		{Metric: "custom.googleapis.com|qps-cum", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
		{Metric: "custom.googleapis.com|q|p|s", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
	}
	if !reflect.DeepEqual(metricInfo, expectedMetricInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetricInfo, metricInfo)
	}
}

func TestTranslator_GetRespForExternalMetric(t *testing.T) {
	translator, _ := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	var val int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric", Labels: map[string]string{"foo": "bar"}},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &val}}},
		}},
	}
	metrics, err := translator.GetRespForExternalMetric(response, "my/custom/metric")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetrics := []external_metrics.ExternalMetricValue{
		{
			Value:      *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:  metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			MetricName: "my/custom/metric",
			MetricLabels: map[string]string{
				"resource.type":          "gke_container",
				"resource.labels.pod_id": "my-pod-id",
				"metric.labels.foo":      "bar",
			},
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metrics[i])
		}
	}
}

func TestTranslator_GetRespForExternalMetric_MultiplePoints(t *testing.T) {
	translator, _ := newFakeTranslatorForExternalMetrics(3*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 3, 0, 0, time.UTC))
	var val1 int64 = 151
	var val2 int64 = 117
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "CUMULATIVE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric", Labels: map[string]string{"foo": "bar"}},
			Points: []*sd.Point{
				{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &val1}},
				{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{Int64Value: &val2}},
			},
		}},
	}
	metrics, err := translator.GetRespForExternalMetric(response, "my/custom/metric")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetrics := []external_metrics.ExternalMetricValue{
		{
			Value:      *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:  metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			MetricName: "my/custom/metric",
			MetricLabels: map[string]string{
				"resource.type":          "gke_container",
				"resource.labels.pod_id": "my-pod-id",
				"metric.labels.foo":      "bar",
			},
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metrics[i])
		}
	}
}
