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
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
	"github.com/golang/glog"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

// Translator is a structure used to translate between Custom Metrics API and Stackdriver API
type Translator struct {
	service             *stackdriver.Service
	config              *config.GceConfig
	reqWindow           time.Duration
	clock               clock
	mapper              apimeta.RESTMapper
	useNewResourceModel bool
}

// GetSDReqForPods returns Stackdriver request for query for multiple pods.
// podList is required to be no longer than 100 items. This is enforced by limitation of "one_of()"
// operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForPods(podList *v1.PodList, metricName string, namespace string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(podList.Items) == 0 {
		return nil, apierr.NewBadRequest("No objects matched to provided selector")
	}
	if len(podList.Items) > 100 {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForPods called with %v pod list, but allowed limit is 100 pods", len(podList.Items)))
	}
	var filter string
	if t.useNewResourceModel {
		resourceNames := getResourceNames(podList)
		filter = joinFilters(
			t.filterForMetric(t.config.MetricsPrefix+"/"+metricName),
			t.filterForCluster(),
			t.filterForPods(resourceNames, namespace),
			t.filterForAnyPod())
	} else {
		resourceIDs := getResourceIDs(podList)
		filter = joinFilters(
			t.filterForMetric(t.config.MetricsPrefix+"/"+metricName),
			t.legacyFilterForCluster(),
			t.legacyFilterForPods(resourceIDs))
	}
	return t.createListTimeseriesRequest(filter), nil
}

// GetRespForPod returns translates Stackdriver response to a Custom Metric associated with a single
// pod.
func (t *Translator) GetRespForPod(response *stackdriver.ListTimeSeriesResponse, groupResource schema.GroupResource, metricName string, namespace string, name string) (*custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, namespace, response, metricName)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, apierr.NewInternalError(fmt.Errorf("Expected exactly one value for pod %s in namespace %s, but received %v values", name, namespace, len(values)))
	}
	// Since len(values) = 1, this loop will execute only once.
	for _, value := range values {
		metricValue, err := t.metricFor(value, groupResource, namespace, name, metricName)
		if err != nil {
			return nil, err
		}
		return metricValue, nil
	}
	// This code is unreacheable
	return nil, apierr.NewInternalError(fmt.Errorf("Illegal state"))
}

// GetRespForPods translates Stackdriver response to a Custom Metric associated
// with multiple pods.
func (t *Translator) GetRespForPods(response *stackdriver.ListTimeSeriesResponse, podList *v1.PodList, groupResource schema.GroupResource, metricName string, namespace string) ([]custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, namespace, response, metricName)
	if err != nil {
		return nil, err
	}
	return t.metricsFor(values, groupResource, metricName, podList)
}

// ListMetricDescriptors returns Stackdriver request for all custom metrics descriptors.
func (t *Translator) ListMetricDescriptors() *stackdriver.ProjectsMetricDescriptorsListCall {
	var filter string
	if t.useNewResourceModel {
		filter = joinFilters(t.filterForMetricPrefix(), t.filterForCluster(), t.filterForAnyPod())
	} else {
		filter = joinFilters(t.filterForMetricPrefix(), t.legacyFilterForCluster(), t.legacyFilterForAnyPod())
	}
	return t.service.Projects.MetricDescriptors.
		List(fmt.Sprintf("projects/%s", t.config.Project)).
		Filter(filter)
}

// GetMetricsFromSDDescriptorsResp returns an array of MetricInfo for all metric descriptors
// returned by Stackdriver API that satisfy the requirements:
// - metricKind is "GAUGE"
// - valueType is "INT64" or "DOUBLE"
// - metric name doesn't contain "/" character after "custom.googleapis.com/" prefix
func (t *Translator) GetMetricsFromSDDescriptorsResp(response *stackdriver.ListMetricDescriptorsResponse) []provider.MetricInfo {
	metrics := []provider.MetricInfo{}
	for _, descriptor := range response.MetricDescriptors {
		if descriptor.MetricKind == "GAUGE" &&
			(descriptor.ValueType == "INT64" || descriptor.ValueType == "DOUBLE") &&
			!strings.Contains(strings.TrimPrefix(descriptor.Type, t.config.MetricsPrefix+"/"), "/") {
			metrics = append(metrics, provider.MetricInfo{
				GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
				Metric:        strings.TrimPrefix(descriptor.Type, t.config.MetricsPrefix+"/"),
				Namespaced:    true,
			})
		}
	}
	return metrics
}

func getResourceNames(list *v1.PodList) []string {
	resourceNames := []string{}
	for _, item := range list.Items {
		resourceNames = append(resourceNames, fmt.Sprintf("%q", item.GetName()))
	}
	return resourceNames
}

func getResourceIDs(list *v1.PodList) []string {
	resourceIDs := []string{}
	for _, item := range list.Items {
		resourceIDs = append(resourceIDs, fmt.Sprintf("%q", item.GetUID()))
	}
	return resourceIDs
}

func joinFilters(filters ...string) string {
	return strings.Join(filters, " AND ")
}

func (t *Translator) filterForCluster() string {
	projectFilter := fmt.Sprintf("resource.label.project_id = %q", t.config.Project)
	clusterFilter := fmt.Sprintf("resource.label.cluster_name = %q", t.config.Cluster)
	locationFilter := fmt.Sprintf("resource.label.location = %q", t.config.Location)
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, locationFilter)
}

func (t *Translator) filterForMetricPrefix() string {
	return fmt.Sprintf("metric.type = starts_with(\"%s/\")", t.config.MetricsPrefix)
}

func (t *Translator) filterForMetric(metricName string) string {
	return fmt.Sprintf("metric.type = %q", metricName)
}

func (t *Translator) filterForAnyPod() string {
	return "resource.type = \"k8s_pod\""
}

func (t *Translator) filterForPods(podNames []string, namespace string) string {
	if len(podNames) == 0 {
		glog.Fatalf("createFilterForIDs called with empty list of pod IDs")
	} else if len(podNames) == 1 {
		return fmt.Sprintf("resource.label.namespace_name = %q AND resource.label.pod_name = %s", namespace, podNames[0])
	}
	return fmt.Sprintf("resource.label.namespace_name = %q AND resource.label.pod_name = one_of(%s)", namespace, strings.Join(podNames, ","))
}

func (t *Translator) legacyFilterForCluster() string {
	projectFilter := fmt.Sprintf("resource.label.project_id = %q", t.config.Project)
	// Skip location, since it may be set incorrectly by Heapster for old resource model
	clusterFilter := fmt.Sprintf("resource.label.cluster_name = %q", t.config.Cluster)
	containerFilter := "resource.label.container_name = \"\""
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, containerFilter)
}

func (t *Translator) legacyFilterForAnyPod() string {
	return "resource.label.pod_id != \"\" AND resource.label.pod_id != \"machine\""
}

func (t *Translator) legacyFilterForPods(podIDs []string) string {
	if len(podIDs) == 0 {
		glog.Fatalf("createFilterForIDs called with empty list of pod IDs")
	} else if len(podIDs) == 1 {
		return fmt.Sprintf("resource.label.pod_id = %s", podIDs[0])
	}
	return fmt.Sprintf("resource.label.pod_id = one_of(%s)", strings.Join(podIDs, ","))
}

func (t *Translator) createListTimeseriesRequest(filter string) *stackdriver.ProjectsTimeSeriesListCall {
	project := fmt.Sprintf("projects/%s", t.config.Project)
	endTime := t.clock.Now()
	startTime := endTime.Add(-t.reqWindow)
	return t.service.Projects.TimeSeries.List(project).Filter(filter).
		IntervalStartTime(startTime.Format(time.RFC3339)).
		IntervalEndTime(endTime.Format(time.RFC3339)).
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(t.reqWindow.Seconds())))
}

func (t *Translator) getMetricValuesFromResponse(groupResource schema.GroupResource, namespace string, response *stackdriver.ListTimeSeriesResponse, metricName string) (map[string]resource.Quantity, error) {
	if len(response.TimeSeries) < 1 {
		return nil, provider.NewMetricNotFoundError(groupResource, metricName)
	}
	metricValues := make(map[string]resource.Quantity)
	// Find time series with specified labels matching
	// Stackdriver API doesn't allow complex label filtering (i.e. "label1 = x AND (label2 = y OR label2 = z)"),
	// therefore only part of the filters is passed and remaining filtering is done here.
	for _, series := range response.TimeSeries {
		if len(series.Points) != 1 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, apierr.NewInternalError(fmt.Errorf("Expected exactly one Point in TimeSeries from Stackdriver, but received %v", len(series.Points)))
		}
		value := *series.Points[0].Value
		name := series.Resource.Labels["pod_id"]
		switch {
		case value.Int64Value != nil:
			metricValues[name] = *resource.NewQuantity(*value.Int64Value, resource.DecimalSI)
		case value.DoubleValue != nil:
			metricValues[name] = *resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI)
		default:
			return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value))
		}
	}
	return metricValues, nil
}

func (t *Translator) metricFor(value resource.Quantity, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	kind, err := t.mapper.KindFor(groupResource.WithVersion(""))
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: custom_metrics.ObjectReference{
			APIVersion: groupResource.Group + "/" + runtime.APIVersionInternal,
			Kind:       kind.Kind,
			Name:       name,
			Namespace:  namespace,
		},
		MetricName: metricName,
		Timestamp:  metav1.Time{t.clock.Now()},
		Value:      value,
	}, nil
}

func (t *Translator) metricsFor(values map[string]resource.Quantity, groupResource schema.GroupResource, metricName string, podList *v1.PodList) ([]custom_metrics.MetricValue, error) {
	res := make([]custom_metrics.MetricValue, 0)

	for _, item := range podList.Items {
		if _, ok := values[fmt.Sprintf("%s", item.GetUID())]; !ok {
			glog.V(4).Infof("Metric '%s' not found for pod '%s'", metricName, item.Name)
			continue
		}
		value, err := t.metricFor(values[fmt.Sprintf("%s", item.GetUID())], groupResource, item.GetNamespace(), item.GetName(), metricName)
		if err != nil {
			return nil, err
		}
		res = append(res, *value)
	}

	return res, nil
}
