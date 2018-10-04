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
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
	"github.com/golang/glog"
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

var (
	podKind schema.ObjectKind
)

type fakeClock struct {
	time time.Time
}

func (f fakeClock) Now() time.Time {
	return f.time
}

func TestTranslator_GetSDReqForPods_Single(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.namespace_name = \"default\" " +
			"AND resource.labels.pod_name = \"my-pod-name\" " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForPods_Multiple(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id-1",
			Name:        "my-pod-name-1",
		},
	}
	pod2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id-2",
			Name:        "my-pod-name-2",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", "default")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.namespace_name = \"default\" " +
			"AND resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\") " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForNodes(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id-1",
			Name:        "my-node-name-1",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForNodes(&v1.NodeList{Items: []v1.Node{node}}, metricName, "GAUGE")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.node_name = \"my-node-name-1\" " +
			"AND resource.type = \"k8s_node\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForPods_legacyResourceModel(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	pod1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id-1",
		},
	}
	pod2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id-2",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", "default")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.container_name = \"\" " +
			"AND resource.labels.pod_id = one_of(\"my-pod-id-1\",\"my-pod-id-2\")").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

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
	metrics, err := translator.GetRespForMultipleObjects(response, translator.getNodeItems(&v1.NodeList{Items: []v1.Node{node}}), schema.GroupResource{Resource: "Node"}, "my|custom|metric")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewMilliQuantity(151000, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-node-name", APIVersion: "/__internal", Kind: "Node"},
			MetricName:      "my|custom|metric",
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetrics[i], metrics[i])
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
	metric, err := translator.GetRespForSingleObject(response, schema.GroupResource{Resource: "Pod", Group: ""}, "my/custom/metric", "my-namespace", "my-pod-name")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetric := &custom_metrics.MetricValue{
		Value:           *resource.NewQuantity(151, resource.DecimalSI),
		Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
		DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
		MetricName:      "my/custom/metric",
	}
	if !reflect.DeepEqual(*metric, *expectedMetric) {
		t.Errorf("Expected: \n%s,\n received: \n%s", expectedMetric, metric)
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
	metrics, err := translator.GetRespForMultipleObjects(response, translator.getPodItems(&v1.PodList{Items: []v1.Pod{pod}}), schema.GroupResource{Resource: "Pod"}, "my/custom/metric")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
			MetricName:      "my/custom/metric",
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetrics[i], metrics[i])
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
	metrics, err := translator.GetRespForMultipleObjects(response, translator.getPodItems(&v1.PodList{Items: []v1.Pod{pod}}), schema.GroupResource{Resource: "Pod"}, "my/custom/metric")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
			MetricName:      "my/custom/metric",
		},
	}
	if len(metrics) != len(expectedMetrics) {
		t.Errorf("Unexpected result. Expected %v metrics, received %v", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetrics[i], metrics[i])
		}
	}
}

func TestTranslator_ListMetricDescriptors(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	request := translator.ListMetricDescriptors()
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.type = one_of(\"k8s_pod\",\"k8s_node\")")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_ListMetricDescriptors_legacyResourceType(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	request := translator.ListMetricDescriptors()
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.container_name = \"\" AND resource.labels.pod_id != \"\" " +
			"AND resource.labels.pod_id != \"machine\"")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
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
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetricInfo, metricInfo)
	}
}

func TestTranslator_GetExternalMetricRequest_NoSelector(t *testing.T) {
	translator, sdService := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", labels.NewSelector())
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/metric/name\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetExternalMetricRequest_CorrectSelector_Cumulative(t *testing.T) {
	translator, sdService := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	req1, _ := labels.NewRequirement("resource.type", selection.Equals, []string{"k8s_pod"})
	req2, _ := labels.NewRequirement("resource.labels.project_id", selection.Equals, []string{"my-project"})
	req3, _ := labels.NewRequirement("resource.labels.pod_name", selection.Exists, []string{})
	req4, _ := labels.NewRequirement("resource.labels.namespace_name", selection.NotIn, []string{"default", "kube-system"})
	req5, _ := labels.NewRequirement("metric.labels.my_label", selection.GreaterThan, []string{"86"})
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "CUMULATIVE", labels.NewSelector().Add(*req1, *req2, *req3, *req4, *req5))
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/metric/name\" " +
			"AND metric.labels.my_label > 86 " +
			"AND NOT resource.labels.namespace_name = one_of(\"default\",\"kube-system\") " +
			"AND resource.labels : pod_name " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_RATE").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetExternalMetricRequest_DifferentProject(t *testing.T) {
	translator, sdService := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	req1, _ := labels.NewRequirement("resource.type", selection.Equals, []string{"k8s_pod"})
	req2, _ := labels.NewRequirement("resource.labels.project_id", selection.Equals, []string{"other-project"})
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "CUMULATIVE", labels.NewSelector().Add(*req1, *req2))
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/other-project").
		Filter("metric.type = \"custom.googleapis.com/my/metric/name\" " +
			"AND resource.labels.project_id = \"other-project\" " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_RATE").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetExternalMetricRequest_InvalidLabel(t *testing.T) {
	translator, _ := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	_, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", labels.SelectorFromSet(labels.Set{
		"arbitrary-label": "foo",
	}))
	expectedError := provider.NewLabelNotAllowedError("arbitrary-label")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_GetExternalMetricRequest_OneInvalidRequirement(t *testing.T) {
	translator, _ := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	req1, _ := labels.NewRequirement("resource.type", selection.Equals, []string{"k8s_pod"})
	req2, _ := labels.NewRequirement("resource.labels.pod_name", selection.Exists, []string{})
	req3, _ := labels.NewRequirement("resource.labels.namespace_name", selection.NotIn, []string{"default", "kube-system"})
	req4, _ := labels.NewRequirement("metric.labels.my_label", selection.DoesNotExist, []string{})
	_, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", labels.NewSelector().Add(*req1, *req2, *req3, *req4))
	expectedError := errors.NewBadRequest("Label selector with operator DoesNotExist is not allowed")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
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
		t.Errorf("Unexpected result. Expected %s metrics, received %s", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetrics[i], metrics[i])
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
		t.Errorf("Unexpected result. Expected %s metrics, received %s", len(expectedMetrics), len(metrics))
	}
	for i := range metrics {
		if !reflect.DeepEqual(metrics[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetrics[i], metrics[i])
		}
	}
}

func newFakeTranslator(reqWindow, alignmentPeriod time.Duration, project, cluster, location string, currentTime time.Time, useNewResourceModel bool) (*Translator, *sd.Service) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		glog.Fatal("Unexpected error creating stackdriver Service client")
	}
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Pod"), meta.RESTScopeNamespace)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Node"), meta.RESTScopeRoot)
	return &Translator{
		service:         sdService,
		reqWindow:       reqWindow,
		alignmentPeriod: alignmentPeriod,
		config: &config.GceConfig{
			Project:  project,
			Cluster:  cluster,
			Location: location,
		},
		clock:               fakeClock{currentTime},
		mapper:              restMapper,
		useNewResourceModel: useNewResourceModel,
	}, sdService
}

// newFakeTranslatorForExternalMetrics returns a simplified translator, where only the fields used
// for External Metrics API need to be specified. Other fields are initialized to zeros.
func newFakeTranslatorForExternalMetrics(reqWindow, alignmentPeriod time.Duration, project string, currentTime time.Time) (*Translator, *sd.Service) {
	return newFakeTranslator(reqWindow, alignmentPeriod, project, "", "", currentTime, false)
}
