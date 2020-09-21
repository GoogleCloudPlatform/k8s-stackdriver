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

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func TestTranslator_GetSDReqForPods_Single(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", labels.Everything(), "default")
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForPods_SingleWithMetricSelector(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", metricSelector, "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.labels.custom = \"test\" " +
			"AND metric.type = \"my/custom/metric\" " +
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForPods_SingleWithInvalidMetricSelector(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("resource.labels.type=container")
	_, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", metricSelector, "default")
	if err == nil {
		t.Error("No translation error")
	}
}

func TestTranslator_GetSDReqForPods_Multiple(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", labels.Everything(), "default")
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForPods_MultipleWithMetricSelctor(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", metricSelector, "default")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.labels.custom = \"test\" " +
			"AND metric.type = \"my/custom/metric\" " +
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForContainers_Single(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", labels.Everything(), "default")
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
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForContainers_SingleWithEmptyNamespace(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", labels.Everything(), AllNamespaces)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.pod_name = \"my-pod-name\" " +
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForContainers_OldResourceModel(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	_, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", labels.Everything(), "default")
	if err == nil {
		t.Errorf("OldResourceModel should not work with GetSDReqForContainers")
	}
}

func TestTranslator_GetSDReqForContainers_SingleWithMetricSelector(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", metricSelector, "default")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.labels.custom = \"test\" " +
			"AND metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.namespace_name = \"default\" " +
			"AND resource.labels.pod_name = \"my-pod-name\" " +
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForContainers_SingleWithInvalidMetricSelector(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("resource.labels.type=container")
	_, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod}}, metricName, "GAUGE", metricSelector, "default")
	if err == nil {
		t.Error("No translation error")
	}
}

func TestTranslator_GetSDReqForContainers_Multiple(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", labels.Everything(), "default")
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
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForContainers_MultipleEmptyNamespace(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", labels.Everything(), AllNamespaces)
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\") " +
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForContainers_MultipleWithMetricSelctor(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
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
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := translator.GetSDReqForContainers(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", metricSelector, "default")
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.labels.custom = \"test\" " +
			"AND metric.type = \"my/custom/metric\" " +
			"AND resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.labels.namespace_name = \"default\" " +
			"AND resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\") " +
			"AND resource.type = \"k8s_container\"").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForNodes(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id-1",
			Name:        "my-node-name-1",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForNodes(&v1.NodeList{Items: []v1.Node{node}}, metricName, "GAUGE", labels.Everything())
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForNodes_withMetricSelector(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id-1",
			Name:        "my-node-name-1",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := translator.GetSDReqForNodes(&v1.NodeList{Items: []v1.Node{node}}, metricName, "GAUGE", metricSelector)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("metric.labels.custom = \"test\" " +
			"AND metric.type = \"my/custom/metric\" " +
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetSDReqForPods_legacyResourceModel(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
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
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName, "GAUGE", labels.Everything(), "default")
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
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_ListMetricDescriptors(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	request := translator.ListMetricDescriptors(false)
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.type = one_of(\"k8s_pod\",\"k8s_node\")")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_ListMetricDescriptors_containerMetrics(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	request := translator.ListMetricDescriptors(true)
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.location = \"my-zone\" " +
			"AND resource.type = one_of(\"k8s_pod\",\"k8s_node\",\"k8s_container\")")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_ListMetricDescriptors_legacyResourceType(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), false)
	request := translator.ListMetricDescriptors(false)
	expectedRequest := sdService.Projects.MetricDescriptors.List("projects/my-project").
		Filter("resource.labels.project_id = \"my-project\" " +
			"AND resource.labels.cluster_name = \"my-cluster\" " +
			"AND resource.labels.container_name = \"\" AND resource.labels.pod_id != \"\" " +
			"AND resource.labels.pod_id != \"machine\"")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
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
		t.Errorf("Unexpected result. Expected \n%v,\n received: \n%v", *expectedRequest, *request)
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
		t.Errorf("Unexpected result. Expected \n%v,\n received: \n%v", *expectedRequest, *request)
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
		t.Errorf("Unexpected result. Expected \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_GetExternalMetricRequest_InvalidLabel(t *testing.T) {
	translator, _ := newFakeTranslatorForExternalMetrics(2*time.Minute, time.Minute, "my-project", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC))
	_, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", labels.SelectorFromSet(labels.Set{
		"arbitrary-label": "foo",
	}))
	expectedError := NewLabelNotAllowedError("arbitrary-label")
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
