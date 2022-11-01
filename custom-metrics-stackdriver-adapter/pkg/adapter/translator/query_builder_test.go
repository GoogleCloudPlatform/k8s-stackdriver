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
	"sort"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func TestQueryBuilder_nil_translator(t *testing.T) {
	_, err := NewQueryBuilder(nil, "my-metric-name").Build()
	if err == nil {
		t.Error("Expected nil translation error, but found nil")
	}
}

func TestQueryBuilder_Both_Pods_PodNames_Provided(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	_, err := NewQueryBuilder(translator, "my-metric-name").
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithPodNames([]string{"pod-1", "pod-2"}).
		Build()
	if err == nil {
		t.Error("Expected pods and podNames mutually exclusive error, but found nil")
	}
}

func TestTranslator_QueryBuilder_pod_Single(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"resource.type = \"k8s_pod\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_prometheus_Single(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo"
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"resource.type = \"prometheus_target\"",
		"metric.type = \"prometheus.googleapis.com/foo\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace = \"default\"",
		"metric.labels.pod = \"my-pod-name\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_pod_SingleWithMetricSelector(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"resource.type = \"k8s_pod\"",
		"metric.type = \"my/custom/metric\"",
		"metric.labels.custom = \"test\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_prometheus_SingleWithMetricSelector(t *testing.T) {
	translator, sdService :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	metricSelector, _ := labels.Parse("metric.labels.custom=test")
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"resource.type = \"prometheus_target\"",
		"metric.type = \"prometheus.googleapis.com/foo/gauge\"",
		"metric.labels.custom = \"test\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace = \"default\"",
		"metric.labels.pod = \"my-pod-name\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("\nUnexpected result.\nExpect: \n%v,\nActual: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_pod_SingleWithInvalidMetricSelector(t *testing.T) {
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
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err == nil {
		t.Error("No translation error")
	}
}

func TestTranslator_QueryBuilder_prometheus_SingleWithInvalidMetricSelector(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	metricSelector, _ := labels.Parse("resource.labels.type=container")
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err == nil {
		t.Error("No translation error")
	}
}

func TestTranslator_QueryBuilder_pod_Multiple(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"k8s_pod\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_prometheus_Multiple(t *testing.T) {
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
	metricName := "prometheus.googleapis.com/foo/gauge"
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"prometheus.googleapis.com/foo/gauge\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace = \"default\"",
		"metric.labels.pod = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"prometheus_target\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("\nUnexpected result.\nExpect: %v,\nActual: %v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_pod_MultipleWithMetricSelctor(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"k8s_pod\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_Container_Single(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_Container_SingleWithEmptyNamespace(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace(AllNamespaces).
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_Container_OldResourceModel(t *testing.T) {
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
	_, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err == nil {
		t.Errorf("OldResourceModel should not work with container type query")
	}
}

func TestTranslator_QueryBuilder_Container_SingleWithMetricSelector(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_Container_SingleWithInvalidMetricSelector(t *testing.T) {
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
	_, err := NewQueryBuilder(translator, metricName).WithPods(&v1.PodList{Items: []v1.Pod{pod}}).WithMetricKind("GAUGE").WithMetricValueType("INT64").WithMetricSelector(metricSelector).WithNamespace("default").Build()
	if err == nil {
		t.Error("No translation error")
	}
}

func TestTranslator_QueryBuilder_Container_Multiple(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_Container_MultipleEmptyNamespace(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace(AllNamespaces).
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_Container_MultipleWithMetricSelctor(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = one_of(\"my-pod-name-1\",\"my-pod-name-2\")",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestQueryBuilder_Node(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithNodes(&v1.NodeList{Items: []v1.Node{node}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.node_name = monitoring.regex.full_match(\"^(my-node-name-1)(:\\\\d+)*\")",
		"resource.type = \"k8s_node\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestQueryBuilder_Multiple_Nodes(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithNodes(&v1.NodeList{Items: []v1.Node{node, node}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.node_name = monitoring.regex.full_match(\"^(my-node-name-1|my-node-name-1)(:\\\\d+)*\")",
		"resource.type = \"k8s_node\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestQueryBuilder_Node_withMetricSelector(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).WithNodes(&v1.NodeList{Items: []v1.Node{node}}).WithMetricKind("GAUGE").WithMetricValueType("INT64").WithMetricSelector(metricSelector).Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.node_name = monitoring.regex.full_match(\"^(my-node-name-1)(:\\\\d+)*\")",
		"resource.type = \"k8s_node\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("120s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", *expectedRequest, *request)
	}
}

func TestTranslator_QueryBuilder_pod_legacyResourceModel(t *testing.T) {
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
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}).
		WithMetricKind("GAUGE").
		WithMetricValueType("INT64").
		WithMetricSelector(labels.Everything()).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Fatalf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.container_name = \"\"",
		"resource.labels.pod_id = one_of(\"my-pod-id-1\",\"my-pod-id-2\")",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
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
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", "INT64", labels.NewSelector())
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
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "CUMULATIVE", "INT64", labels.NewSelector().Add(*req1, *req2, *req3, *req4, *req5))
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
	request, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "CUMULATIVE", "INT64", labels.NewSelector().Add(*req1, *req2))
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
	_, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", "INT64", labels.SelectorFromSet(labels.Set{
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
	_, err := translator.GetExternalMetricRequest("custom.googleapis.com/my/metric/name", "GAUGE", "INT64", labels.NewSelector().Add(*req1, *req2, *req3, *req4))
	expectedError := errors.NewBadRequest("Label selector with operator DoesNotExist is not allowed")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_QueryBuilder_pod_Single_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	selector, _ := labels.Parse("reducer=REDUCE_PERCENTILE_50")
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_pod\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_50").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_promethus_Single_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	selector, _ := labels.Parse("reducer=REDUCE_PERCENTILE_50")
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"prometheus.googleapis.com/foo/gauge\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace = \"default\"",
		"metric.labels.pod = \"my-pod-name\"",
		"resource.type = \"prometheus_target\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_50").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_pod_NoSupportDistributions(t *testing.T) {
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
	req, _ := labels.NewRequirement("reducer", selection.Equals, []string{"REDUCE_PERCENTILE_50"})
	selector := labels.NewSelector().Add(*req)
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	if err == nil {
		t.Errorf("Expected error, as distributions should not be suppoted; was suceessful")
	}
}

func TestTranslator_QueryBuilder_prometheus_NoSupportDistributions(t *testing.T) {
	translator, _ :=
		NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	req, _ := labels.NewRequirement("reducer", selection.Equals, []string{"REDUCE_PERCENTILE_50"})
	selector := labels.NewSelector().Add(*req)
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	if err == nil {
		t.Errorf("Expected error, as distributions should not be suppoted; was suceessful")
	}
}

func TestTranslator_QueryBuilder_pod_BadPercentile(t *testing.T) {
	translator, _ :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	selector, _ := labels.Parse("reducer=PERCENTILE_52")
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	expectedError := NewLabelNotAllowedError("Specified reducer is not supported: PERCENTILE_52")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_QueryBuilder_prometheus_BadPercentile(t *testing.T) {
	translator, _ :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	selector, _ := labels.Parse("reducer=PERCENTILE_52")
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	expectedError := NewLabelNotAllowedError("Specified reducer is not supported: PERCENTILE_52")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_QueryBuilder_pod_TooManyPercentiles(t *testing.T) {
	translator, _ :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	selector, _ := labels.Parse("reducer in (PERCENTILE_50,PERCENTILE_99)")
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("INT64").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	expectedError := NewLabelNotAllowedError("Reducer must use '=' or '==': You used in")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_QueryBuilder_prometheus_TooManyPercentiles(t *testing.T) {
	translator, _ :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "prometheus.googleapis.com/foo/gauge"
	selector, _ := labels.Parse("reducer in (PERCENTILE_50,PERCENTILE_99)")
	_, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("INT64").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	expectedError := NewLabelNotAllowedError("Reducer must use '=' or '==': You used in")
	if *err.(*errors.StatusError) != *expectedError {
		t.Errorf("Expected status error: %s, but received: %s", expectedError, err)
	}
}

func TestTranslator_QueryBuilder_pod_SingleWithMetricSelector_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test,reducer=REDUCE_PERCENTILE_99")
	request, err := NewQueryBuilder(translator, metricName).
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_pod\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_99").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_QueryBuilder_Container_Single_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	selector, _ := labels.Parse("reducer=REDUCE_PERCENTILE_50")
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(selector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_50").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForContainer_SingleWithMetricSelector_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
			Name:        "my-pod-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test,reducer=REDUCE_PERCENTILE_99")
	request, err := NewQueryBuilder(translator, metricName).
		AsContainerType().
		WithPods(&v1.PodList{Items: []v1.Pod{pod}}).
		WithMetricKind("DELTA").
		WithMetricValueType("DISTRIBUTION").
		WithMetricSelector(metricSelector).
		WithNamespace("default").
		Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.namespace_name = \"default\"",
		"resource.labels.pod_name = \"my-pod-name\"",
		"resource.type = \"k8s_container\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_99").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestQueryBuilder_Node_Single_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id",
			Name:        "my-node-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("reducer=REDUCE_PERCENTILE_95")
	request, err := NewQueryBuilder(translator, metricName).WithNodes(&v1.NodeList{Items: []v1.Node{node}}).WithMetricKind("DELTA").WithMetricValueType("DISTRIBUTION").WithMetricSelector(metricSelector).Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.node_name = monitoring.regex.full_match(\"^(my-node-name)(:\\\\d+)*\")",
		"resource.type = \"k8s_node\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_95").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}

func TestQueryBuilder_Node_SingleWithMetricSelector_Distribution(t *testing.T) {
	translator, sdService :=
		newFakeTranslatorForDistributions(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ClusterName: "my-cluster",
			UID:         "my-node-id",
			Name:        "my-node-name",
		},
	}
	metricName := "my/custom/metric"
	metricSelector, _ := labels.Parse("metric.labels.custom=test,reducer=REDUCE_PERCENTILE_95")
	request, err := NewQueryBuilder(translator, metricName).WithNodes(&v1.NodeList{Items: []v1.Node{node}}).WithMetricKind("DELTA").WithMetricValueType("DISTRIBUTION").WithMetricSelector(metricSelector).Build()
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	filters := []string{
		"metric.labels.custom = \"test\"",
		"metric.type = \"my/custom/metric\"",
		"resource.labels.project_id = \"my-project\"",
		"resource.labels.cluster_name = \"my-cluster\"",
		"resource.labels.location = \"my-zone\"",
		"resource.labels.node_name = monitoring.regex.full_match(\"^(my-node-name)(:\\\\d+)*\")",
		"resource.type = \"k8s_node\"",
	}
	sort.Strings(filters)
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter(strings.Join(filters, " AND ")).
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:02:00Z").
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationCrossSeriesReducer("REDUCE_PERCENTILE_95").
		AggregationAlignmentPeriod("60s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedRequest, request)
	}
}
