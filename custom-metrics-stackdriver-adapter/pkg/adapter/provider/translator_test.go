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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/apis/custom_metrics"
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
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/custom/metric\" " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.location = \"my-zone\" " +
			"AND resource.label.namespace_name = \"default\" " +
			"AND resource.label.pod_name = \"my-pod-name\" " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:01:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForPods_Multiple(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), true)
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
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/custom/metric\" " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.location = \"my-zone\" " +
			"AND resource.label.namespace_name = \"default\" " +
			"AND resource.label.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\") " +
			"AND resource.type = \"k8s_pod\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:01:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForNodes(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id-1",
			Name:        "my-node-name-1",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForNodes(&v1.NodeList{Items: []v1.Node{node}}, metricName)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/custom/metric\" " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.location = \"my-zone\" " +
			"AND resource.label.node_name = \"my-node-name-1\" " +
			"AND resource.type = \"k8s_node\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:01:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForPods_legacyResourceModel(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), false)
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
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"custom.googleapis.com/my/custom/metric\" " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.container_name = \"\" " +
			"AND resource.label.pod_id = one_of(\"my-pod-id-1\",\"my-pod-id-2\")").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:01:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetRespForNodes_Aggregation(t *testing.T) {
	translator, _ :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), true)
	var valPart1 float64 = 101
	var valPart2 float64 = 50
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "DOUBLE",
				Metric: &sd.Metric{
					Type:   "custom.googleapis.com/my/custom/metric",
					Labels: map[string]string{"irrelevant_label": "value1"},
				},
				Points: []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{DoubleValue: &valPart1}}},
			},
			{
				Resource:   &sd.MonitoredResource{Type: "k8s_node", Labels: map[string]string{"node_name": "my-node-name"}},
				MetricKind: "GAUGE",
				ValueType:  "DOUBLE",
				Metric: &sd.Metric{
					Type:   "custom.googleapis.com/my/custom/metric",
					Labels: map[string]string{"irrelevant_label": "value2"},
				},
				Points: []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{DoubleValue: &valPart2}}},
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
	metrics, err := translator.GetRespForMultipleObjects(response, translator.getNodeItems(&v1.NodeList{Items: []v1.Node{node}}), schema.GroupResource{Resource: "Node"}, "my/custom/metric")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewMilliQuantity(151000, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC),
			DescribedObject: custom_metrics.ObjectReference{Name: "my-node-name", APIVersion: "/__internal", Kind: "Node"},
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

func TestTranslator_GetRespForPod_legacyResourceType(t *testing.T) {
	translator, _ :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), false)
	var metricValue int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{Int64Value: &metricValue}}},
		}},
	}
	metric, err := translator.GetRespForSingleObject(response, schema.GroupResource{Resource: "Pod", Group: ""}, "my/custom/metric", "my-namespace", "my-pod-name")
	if err != nil {
		t.Errorf("Translation error: %s", err)
		return
	}
	expectedMetric := &custom_metrics.MetricValue{
		Value:           *resource.NewQuantity(151, resource.DecimalSI),
		Timestamp:       metav1.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC),
		DescribedObject: custom_metrics.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
		MetricName:      "my/custom/metric",
	}
	if !reflect.DeepEqual(*metric, *expectedMetric) {
		t.Errorf("Expected: \n%s,\n received: \n%s", expectedMetric, metric)
	}
}

func TestTranslator_GetRespForPods_legacyResourceType(t *testing.T) {
	translator, _ :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), false)
	var val int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:01:00Z"}, Value: &sd.TypedValue{Int64Value: &val}}},
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
		t.Errorf("Translation error: %s", err)
	}
	expectedMetrics := []custom_metrics.MetricValue{
		{
			Value:           *resource.NewQuantity(151, resource.DecimalSI),
			Timestamp:       metav1.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC),
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
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), true)
	request := translator.ListMetricDescriptors()
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("metric.type = starts_with(\"custom.googleapis.com/\") " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.location = \"my-zone\" " +
			"AND resource.type = one_of(\"k8s_pod\",\"k8s_node\")")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_ListMetricDescriptors_legacyResourceType(t *testing.T) {
	translator, sdService :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), false)
	request := translator.ListMetricDescriptors()
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("metric.type = starts_with(\"custom.googleapis.com/\") " +
			"AND resource.label.project_id = \"my-project\" " +
			"AND resource.label.cluster_name = \"my-cluster\" " +
			"AND resource.label.container_name = \"\" AND resource.label.pod_id != \"\" " +
			"AND resource.label.pod_id != \"machine\"")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetMetricsFromSDDescriptorsResp(t *testing.T) {
	translator, _ :=
		newFakeTranslator(time.Minute, "custom.googleapis.com", "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 1, 0, 0, time.UTC), false)
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
	expectedMetricInfo := []provider.MetricInfo{
		{Metric: "qps-int", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
		{Metric: "qps-double", GroupResource: schema.GroupResource{Group: "", Resource: "*"}, Namespaced: true},
	}
	if !reflect.DeepEqual(metricInfo, expectedMetricInfo) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedMetricInfo, metricInfo)
	}
}

func newFakeTranslator(reqWindow time.Duration, metricPrefix, project, cluster, location string, currentTime time.Time, useNewResourceModel bool) (*Translator, *sd.Service) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		glog.Fatal("Unexpected error creating stackdriver Service client")
	}
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{}, meta.InterfacesForUnstructured)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Pod"), meta.RESTScopeNamespace)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Node"), meta.RESTScopeRoot)
	return &Translator{
		service:   sdService,
		reqWindow: reqWindow,
		config: &config.GceConfig{
			MetricsPrefix: metricPrefix,
			Project:       project,
			Cluster:       cluster,
			Location:      location,
		},
		clock:               fakeClock{currentTime},
		mapper:              restMapper,
		useNewResourceModel: useNewResourceModel,
	}, sdService
}
