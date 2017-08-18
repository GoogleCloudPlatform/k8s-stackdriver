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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

type fakeObject struct {
	meta metav1.ObjectMeta
	kind schema.ObjectKind
}

func (f fakeObject) GetObjectKind() schema.ObjectKind {
	return f.kind
}

func (f fakeObject) GetObjectMeta() metav1.Object {
	return &f.meta
}

type fakeList struct {
	Items []runtime.Object
	kind  schema.ObjectKind
}

func (f *fakeList) GetObjectKind() schema.ObjectKind {
	return f.kind
}

func (f *fakeList) IsUnstructuredObject() {
}

func (f *fakeList) IsList() bool {
	return true
}

func (f *fakeList) UnstructuredContent() map[string]interface{} {
	return map[string]interface{}{}
}

type fakeClock struct {
	time time.Time
}

func (f fakeClock) Now() time.Time {
	return f.time
}

func TestTranslator_GetSDReqForPod(t *testing.T) {
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
	podMeta := metav1.ObjectMeta{
		// Pod name, namespace and labels are never used. UID is sufficient to identify a pod.
		ClusterName: "my-cluster",
		UID:         "my-pod-id",
	}
	metricName := "my/custom/metric"
	podKind := schema.EmptyObjectKind
	podKind.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "Pod"))
	request, err := translator.GetSDReqForPod(fakeObject{podMeta, podKind}, metricName)
	if err != nil {
		t.Errorf("Translation error: %s", err)
		return
	}
	expectedRequest := sdService.Projects.TimeSeries.List("projects/my-project").
		Filter("(metric.type = \"custom.googleapis.com/my/custom/metric\") " +
			"AND (resource.label.project_id = \"my-project\") " +
			"AND (resource.label.cluster_name = \"my-cluster\") " +
			"AND (resource.label.zone = \"my-zone\") AND (resource.label.pod_id = \"my-pod-id\") " +
			"AND (resource.label.container_name = \"\")").
		IntervalStartTime("2017-01-02T13:00:00Z").
		IntervalEndTime("2017-01-02T13:05:00Z").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod("300s")
	if !reflect.DeepEqual(*request, *expectedRequest) {
		t.Errorf("Expected: \n%s,\n received: \n%s", expectedRequest, request)
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
