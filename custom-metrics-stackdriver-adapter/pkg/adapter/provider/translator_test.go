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
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type fakeClock struct {
	time time.Time
}

func (f fakeClock) Now() time.Time {
	return f.time
}

func TestTranslator_GetSDReqForPods_Single(t *testing.T) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		t.Errorf("Unexpected error creating stackdriver Service client")
		return
	}
	translator := Translator{
		service:   sdService,
		reqWindow: 5 * time.Minute,
		config: &config.GceConfig{
			MetricsPrefix: "custom.googleapis.com",
			Project:       "my-project",
			Cluster:       "my-cluster",
			Zone:          "my-zone",
		},
		clock: fakeClock{time.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC)},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name",
			Labels:      map[string]string{},
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod}}, metricName)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("(metric.type = \"custom.googleapis.com/my/custom/metric\") " +
			"AND (resource.label.project_id = \"my-project\") " +
			"AND (resource.label.cluster_name = \"my-cluster\") " +
			"AND (resource.label.zone = \"my-zone\") " +
			"AND (resource.label.pod_id = \"my-pod-id\") " +
			"AND (resource.label.container_name = \"\")").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:05:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("300s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", expectedRequest, request)
	}
}

func TestTranslator_GetSDReqForPods_Multiple(t *testing.T) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		t.Errorf("Unexpected error creating stackdriver Service client")
		return
	}
	translator := Translator{
		service:   sdService,
		reqWindow: 5 * time.Minute,
		config: &config.GceConfig{
			MetricsPrefix: "custom.googleapis.com",
			Project:       "my-project",
			Cluster:       "my-cluster",
			Zone:          "my-zone",
		},
		clock: fakeClock{time.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC)},
	}
	pod1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name",
			Labels:      map[string]string{},
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	pod2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name-2",
			Labels:      map[string]string{},
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id-2",
		},
	}
	metricName := "my/custom/metric"
	request, err := translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{pod1, pod2}}, metricName)
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("(metric.type = \"custom.googleapis.com/my/custom/metric\") " +
			"AND (resource.label.project_id = \"my-project\") " +
			"AND (resource.label.cluster_name = \"my-cluster\") " +
			"AND (resource.label.zone = \"my-zone\") " +
			"AND (resource.label.pod_id = one_of(\"my-pod-id\",\"my-pod-id-2\")) " +
			"AND (resource.label.container_name = \"\")").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:05:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("300s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedRequest, *request)
	}
}

func TestTranslator_GetRespForPod(t *testing.T) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		t.Errorf("Couldn't create Stackdriver Service client")
		return
	}
	translator := Translator{
		service:   sdService,
		reqWindow: 5 * time.Minute,
		config: &config.GceConfig{
			MetricsPrefix: "custom.googleapis.com",
			Project:       "my-project",
			Cluster:       "my-cluster",
			Zone:          "my-zone",
		},
		clock: fakeClock{time.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC)},
	}
	var metricValue int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:05:00Z"}, Value: &sd.TypedValue{Int64Value: &metricValue}}},
		}},
	}
	metric, err := translator.GetRespForPod(response, schema.GroupResource{Resource: "Pod", Group: ""}, "my/custom/metric", "my-namespace", "my-pod-name")
	if err != nil {
		t.Errorf("Translation error: %s", err)
		return
	}
	expectedMetric := &custom_metrics.MetricValue{
		Value:           *resource.NewQuantity(151, resource.DecimalSI),
		Timestamp:       metav1.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC),
		DescribedObject: api.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
		MetricName:      "my/custom/metric",
	}
	if !reflect.DeepEqual(*metric, *expectedMetric) {
		t.Errorf("Expected: \n%s,\n received: \n%s", expectedMetric, metric)
	}
}

func TestTranslator_GetRespForPods(t *testing.T) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		t.Errorf("Unexpected error creating stackdriver Service client")
		return
	}
	translator := Translator{
		service:   sdService,
		reqWindow: 5 * time.Minute,
		config: &config.GceConfig{
			MetricsPrefix: "custom.googleapis.com",
			Project:       "my-project",
			Cluster:       "my-cluster",
			Zone:          "my-zone",
		},
		clock: fakeClock{time.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC)},
	}
	var val int64 = 151
	response := &sd.ListTimeSeriesResponse{
		TimeSeries: []*sd.TimeSeries{{
			Resource:   &sd.MonitoredResource{Type: "gke_container", Labels: map[string]string{"pod_id": "my-pod-id"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     &sd.Metric{Type: "custom.googleapis.com/my/custom/metric"},
			Points:     []*sd.Point{{Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:00:00Z", EndTime: "2017-01-02T13:05:00Z"}, Value: &sd.TypedValue{Int64Value: &val}}},
		}},
	}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod-name",
			Labels:      map[string]string{},
			Namespace:   "my-namespace",
			ClusterName: "my-cluster",
			UID:         "my-pod-id",
		},
	}
	metrics, err := translator.GetRespForPods(response, &v1.PodList{Items: []v1.Pod{pod}}, schema.GroupResource{Resource: "Pod"}, "my/custom/metric", "my-namespace")
	if err != nil {
		t.Errorf("Translation error: %s", err)
	}
	expectedMetrics := &custom_metrics.MetricValueList{
		Items: []custom_metrics.MetricValue{
			{
				Value:           *resource.NewQuantity(151, resource.DecimalSI),
				Timestamp:       metav1.Date(2017, 1, 2, 13, 5, 0, 0, time.UTC),
				DescribedObject: api.ObjectReference{Name: "my-pod-name", APIVersion: "/__internal", Kind: "Pod", Namespace: "my-namespace"},
				MetricName:      "my/custom/metric",
			},
		},
	}
	if !reflect.DeepEqual(*metrics, *expectedMetrics) {
		t.Errorf("Unexpected result. Expected: \n%s,\n received: \n%s", *expectedMetrics, *metrics)
	}
}
