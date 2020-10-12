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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/metrics-server/pkg/api"
)

func TestTranslator_GetRespForNodes_Aggregation(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
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
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
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
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
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
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
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

func TestTranslator_GetCoreNodeMetricFromResponse_SingleRAM(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value int64 = 101
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval1.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreNodeMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]resource.Quantity{"my-node-name": *resource.NewQuantity(value, resource.DecimalSI)}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetric), len(metrics))
	}
	expectedTimeInfo := map[string]api.TimeInfo{"my-node-name": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected %v timeInfo, received %v", len(expectedTimeInfo), len(timeInfo))
	}
}
func TestTranslator_GetCoreNodeMetricFromResponse_SingleCPU(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value float64 = 0.000001
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval1.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "CUMULATIVE",
				ValueType:  "DOUBLE",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval1, Value: &sd.TypedValue{DoubleValue: &value}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreNodeMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]resource.Quantity{"my-node-name": *resource.NewScaledQuantity(int64(value*1000*1000), -6)}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetric), len(metrics))
	}
	expectedTimeInfo := map[string]api.TimeInfo{"my-node-name": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected %v timeInfo, received %v", len(expectedTimeInfo), len(timeInfo))
	}
}

func TestTranslator_GetCoreNodeMetricFromResponse_MultiplePoints(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 1
	var value2 int64 = 2
	timeInterval2 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime2, err := time.Parse(time.RFC3339, timeInterval2.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points: []*sd.Point{{Interval: timeInterval2, Value: &sd.TypedValue{Int64Value: &value2}},
					{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreNodeMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]resource.Quantity{"my-node-name": *resource.NewQuantity(value2, resource.DecimalSI)}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetric), len(metrics))
	}
	expectedTimeInfo := map[string]api.TimeInfo{"my-node-name": {endTime2, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected %v timeInfo, received %v", len(expectedTimeInfo), len(timeInfo))
	}
}

func TestTranslator_GetCoreNodeMetricFromResponse_Empty(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{},
	}

	metrics, timeInfo, err := translator.GetCoreNodeMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	if len(metrics) != 0 {
		t.Errorf("Unexpected result. Expected 0 metrics, received %v", len(metrics))
	}
	if len(timeInfo) != 0 {
		t.Errorf("Unexpected result. Expected 0 timeInfo, received %v", len(timeInfo))
	}
}

func TestTranslator_GetCoreNodeMetricFromResponse_Multiple(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 101
	var value2 float64 = 101
	timeInterval := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name2"}},
				MetricKind: "CUMULATIVE",
				ValueType:  "DOUBLE",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{DoubleValue: &value2}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreNodeMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]resource.Quantity{"my-node-name1": *resource.NewQuantity(value1, resource.DecimalSI), "my-node-name2": *resource.NewScaledQuantity(int64(value2*1000*1000), -6)}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetric), len(metrics))
	}
	expectedTimeInfo := map[string]api.TimeInfo{"my-node-name1": {endTime, translator.alignmentPeriod}, "my-node-name2": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected %v timeInfo, received %v", len(expectedTimeInfo), len(timeInfo))
	}
}

func TestTranslator_GetCoreNodeMetricFromResponse_MultipleSame(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 101
	var value2 float64 = 101.1

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name1"}},
				MetricKind: "CUMULATIVE",
				ValueType:  "DOUBLE",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:02:00Z"}, Value: &sd.TypedValue{DoubleValue: &value2}}},
			},
		},
	}

	_, _, err := translator.GetCoreNodeMetricFromResponse(response)
	expectedError := apierr.NewInternalError(fmt.Errorf("The same node appered two time in the response."))
	if err == nil {
		t.Errorf("Translation should return error.")
	} else if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %s, received: %s", expectedError, err)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_SingleRAM(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value int64 = 101
	timeInterval := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{Int64Value: &value}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"container1": *resource.NewQuantity(value, resource.DecimalSI)}}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metrics)
	}
	expectedTimeInfo := map[string]api.TimeInfo{
		"namespace1:pod1": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTimeInfo, timeInfo)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_SingleCPU(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value float64 = 0.1
	timeInterval := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "CUMULATIVE",
				ValueType:  "DOUBLE",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{DoubleValue: &value}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"container1": *resource.NewScaledQuantity(int64(value*1000*1000), -6)}}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metrics)
	}
	expectedTimeInfo := map[string]api.TimeInfo{
		"namespace1:pod1": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTimeInfo, timeInfo)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_MultipleContainers(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 101
	var value2 int64 = 102
	var value3 int64 = 103
	timeInterval := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container2", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{Int64Value: &value2}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container3", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval, Value: &sd.TypedValue{Int64Value: &value3}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"container1": *resource.NewQuantity(value1, resource.DecimalSI), "container2": *resource.NewQuantity(value2, resource.DecimalSI), "container3": *resource.NewQuantity(value3, resource.DecimalSI)},
	}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metrics)
	}
	expectedTimeInfo := map[string]api.TimeInfo{
		"namespace1:pod1": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTimeInfo, timeInfo)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_MultiplePods(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 101
	var value2 int64 = 102
	var value3 int64 = 103
	var value4 int64 = 104
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	endTime1, err := time.Parse(time.RFC3339, timeInterval1.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}
	timeInterval2 := &sd.TimeInterval{StartTime: "2017-01-02T14:01:00Z", EndTime: "2017-01-02T14:02:00Z"}
	endTime2, err := time.Parse(time.RFC3339, timeInterval2.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}
	timeInterval3 := &sd.TimeInterval{StartTime: "2017-01-02T15:01:00Z", EndTime: "2017-01-02T15:02:00Z"}
	endTime3, err := time.Parse(time.RFC3339, timeInterval3.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace2", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval2, Value: &sd.TypedValue{Int64Value: &value2}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod2"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval3, Value: &sd.TypedValue{Int64Value: &value3}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container2", "namespace_name": "namespace1", "pod_name": "pod2"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval3, Value: &sd.TypedValue{Int64Value: &value4}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"container1": *resource.NewQuantity(value1, resource.DecimalSI)},
		"namespace2:pod1": {"container1": *resource.NewQuantity(value2, resource.DecimalSI)},
		"namespace1:pod2": {"container1": *resource.NewQuantity(value3, resource.DecimalSI), "container2": *resource.NewQuantity(value4, resource.DecimalSI)},
	}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metrics)
	}
	expectedTimeInfo := map[string]api.TimeInfo{
		"namespace1:pod1": {endTime1, translator.alignmentPeriod},
		"namespace2:pod1": {endTime2, translator.alignmentPeriod},
		"namespace1:pod2": {endTime3, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTimeInfo, timeInfo)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_SameContainers(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value1 int64 = 101
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "CUMULATIVE",
				ValueType:  "DOUBLE",
				Metric:     nil,
				Points:     []*sd.Point{{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
		},
	}

	_, _, err := translator.GetCoreContainerMetricFromResponse(response)
	expectedError := apierr.NewInternalError(fmt.Errorf("The same container appered two time in the response."))
	if err == nil {
		t.Errorf("Translation should return error.")
	} else if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %s, received: %s", expectedError, err)
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_Empty(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	if len(metrics) != 0 {
		t.Errorf("Unexpected result. Expected 0 metrics, received %v", len(metrics))
	}
	if len(timeInfo) != 0 {
		t.Errorf("Unexpected result. Expected 0 metrics, received %v", len(metrics))
	}
}

func TestTranslator_GetCoreContainerMetricFromResponse_MultiplePoints(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	var value2 int64 = 101
	timeInterval2 := &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"}
	var value1 int64 = 101
	timeInterval1 := &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-01T13:02:00Z"}
	endTime, err := time.Parse(time.RFC3339, timeInterval2.EndTime)
	if err != nil {
		t.Errorf("Can't parse EndTime: %s", err)
	}

	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource: &sd.MonitoredResource{Type: "k8s_container",
					Labels: map[string]string{"container_name": "container1", "namespace_name": "namespace1", "pod_name": "pod1"}},
				MetricKind: "GAUGE",
				ValueType:  "INT64",
				Metric:     nil,
				Points: []*sd.Point{{Interval: timeInterval2, Value: &sd.TypedValue{Int64Value: &value2}},
					{Interval: timeInterval1, Value: &sd.TypedValue{Int64Value: &value1}}},
			},
		},
	}

	metrics, timeInfo, err := translator.GetCoreContainerMetricFromResponse(response)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}

	expectedMetric := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"container1": *resource.NewQuantity(value2, resource.DecimalSI)}}
	if !reflect.DeepEqual(metrics, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metrics)
	}
	expectedTimeInfo := map[string]api.TimeInfo{
		"namespace1:pod1": {endTime, translator.alignmentPeriod}}
	if !reflect.DeepEqual(timeInfo, expectedTimeInfo) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTimeInfo, timeInfo)
	}
}
