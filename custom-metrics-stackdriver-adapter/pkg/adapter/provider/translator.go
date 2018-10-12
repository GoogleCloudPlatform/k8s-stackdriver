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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"strconv"
)

var (
	// allowedLabelPrefixes and allowedFullLabelNames specify all metric labels allowed for querying
	// External Metrics API.
	allowedLabelPrefixes  = []string{"metric.labels", "resource.labels", "metadata.system_labels", "metadata.user_labels"}
	allowedFullLabelNames = []string{"resource.type"}
)

const (
	// oneOfMax is the maximum value of one_of() function allowed in Stackdriver Filters
	oneOfMax     = 100
	nodeResource = "nodes"
	podResource  = "pods"
)

// Translator is a structure used to translate between Custom Metrics API and Stackdriver API
type Translator struct {
	service             *stackdriver.Service
	config              *config.GceConfig
	reqWindow           time.Duration
	alignmentPeriod     time.Duration
	clock               clock
	mapper              apimeta.RESTMapper
	useNewResourceModel bool
}

// GetSDReqForPods returns Stackdriver request for query for multiple pods.
// podList is required to be no longer than oneOfMax items. This is enforced by limitation of
// "one_of()" operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForPods(podList *v1.PodList, metricName string, metricKind, namespace string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(podList.Items) == 0 {
		return nil, apierr.NewBadRequest("No objects matched provided selector")
	}
	if len(podList.Items) > oneOfMax {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForPods called with %v pod list, but allowed limit is %v pods", len(podList.Items), oneOfMax))
	}
	var filter string
	if t.useNewResourceModel {
		resourceNames := getPodNames(podList)
		filter = joinFilters(
			t.filterForMetric(metricName),
			t.filterForCluster(),
			t.filterForPods(resourceNames, namespace),
			t.filterForAnyPod())
	} else {
		resourceIDs := getResourceIDs(podList)
		filter = joinFilters(
			t.filterForMetric(metricName),
			t.legacyFilterForCluster(),
			t.legacyFilterForPods(resourceIDs))
	}
	return t.createListTimeseriesRequest(filter, metricKind), nil
}

// GetSDReqForNodes returns Stackdriver request for query for multiple nodes.
// nodeList is required to be no longer than oneOfMax items. This is enforced by limitation of
// "one_of()" operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForNodes(nodeList *v1.NodeList, metricName string, metricKind string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(nodeList.Items) == 0 {
		return nil, apierr.NewBadRequest("No objects matched provided selector")
	}
	if len(nodeList.Items) > oneOfMax {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForNodes called with %v node list, but allowed limit is %v nodes", len(nodeList.Items), oneOfMax))
	}
	var filter string
	if !t.useNewResourceModel {
		return nil, provider.NewOperationNotSupportedError("Root scoped metrics are not supported without new Stackdriver resource model enabled")
	}
	resourceNames := getNodeNames(nodeList)
	filter = joinFilters(
		t.filterForMetric(metricName),
		t.filterForCluster(),
		t.filterForNodes(resourceNames),
		t.filterForAnyNode())
	return t.createListTimeseriesRequest(filter, metricKind), nil
}

// GetExternalMetricRequest returns Stackdriver request for query for external metric.
func (t *Translator) GetExternalMetricRequest(metricName string, metricKind string, metricSelector labels.Selector) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	metricProject, err := t.GetExternalMetricProject(metricSelector)
	if err != nil {
		return nil, err
	}

	filterForMetric := t.filterForMetric(metricName)
	if metricSelector.Empty() {
		return t.createListTimeseriesRequest(filterForMetric, metricKind), nil
	}
	filterForSelector, err := t.filterForSelector(metricSelector)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequestProject(joinFilters(filterForMetric, filterForSelector), metricKind, metricProject), nil
}

// GetRespForSingleObject returns translates Stackdriver response to a Custom Metric associated with
// a single object.
func (t *Translator) GetRespForSingleObject(response *stackdriver.ListTimeSeriesResponse, groupResource schema.GroupResource, metricName string, namespace string, name string) (*custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, response, metricName)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, provider.NewMetricNotFoundForError(groupResource, metricName, name)
	}
	if len(values) > 1 {
		return nil, apierr.NewInternalError(fmt.Errorf("Expected exactly one value for resource %q in namespace %q, but received %v values", name, namespace, len(values)))
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

// GetRespForMultipleObjects translates Stackdriver response to a Custom Metric associated
// with multiple pods.
func (t *Translator) GetRespForMultipleObjects(response *stackdriver.ListTimeSeriesResponse, list []metav1.ObjectMeta, groupResource schema.GroupResource, metricName string) ([]custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, response, metricName)
	if err != nil {
		return nil, err
	}
	return t.metricsFor(values, groupResource, metricName, list)
}

// GetRespForExternalMetric translates Stackdriver response to list of External Metrics
func (t *Translator) GetRespForExternalMetric(response *stackdriver.ListTimeSeriesResponse, metricName string) ([]external_metrics.ExternalMetricValue, error) {
	metricValues := []external_metrics.ExternalMetricValue{}
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := series.Points[0]
		endTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
		if err != nil {
			return nil, apierr.NewInternalError(fmt.Errorf("Timeseries from Stackdriver has incorrect end time: %s", point.Interval.EndTime))
		}
		metricValue := external_metrics.ExternalMetricValue{
			Timestamp:    metav1.NewTime(endTime),
			MetricName:   metricName,
			MetricLabels: t.getMetricLabels(series),
		}
		value := *point.Value
		switch {
		case value.Int64Value != nil:
			metricValue.Value = *resource.NewQuantity(*value.Int64Value, resource.DecimalSI)
		case value.DoubleValue != nil:
			metricValue.Value = *resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI)
		default:
			return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value))
		}
		metricValues = append(metricValues, metricValue)
	}
	return metricValues, nil
}

// ListMetricDescriptors returns Stackdriver request for all custom metrics descriptors.
func (t *Translator) ListMetricDescriptors() *stackdriver.ProjectsMetricDescriptorsListCall {
	var filter string
	if t.useNewResourceModel {
		filter = joinFilters(t.filterForCluster(), t.filterForAnyResource())
	} else {
		filter = joinFilters(t.legacyFilterForCluster(), t.legacyFilterForAnyPod())
	}
	return t.service.Projects.MetricDescriptors.
		List(fmt.Sprintf("projects/%s", t.config.Project)).
		Filter(filter)
}

// GetMetricsFromSDDescriptorsResp returns an array of MetricInfo for all metric descriptors
// returned by Stackdriver API that satisfy the requirements:
// - valueType is "INT64" or "DOUBLE"
// - metric name doesn't contain "/" character after "custom.googleapis.com/" prefix
func (t *Translator) GetMetricsFromSDDescriptorsResp(response *stackdriver.ListMetricDescriptorsResponse) []provider.CustomMetricInfo {
	metrics := []provider.CustomMetricInfo{}
	for _, descriptor := range response.MetricDescriptors {
		if descriptor.ValueType == "INT64" || descriptor.ValueType == "DOUBLE" {
			metrics = append(metrics, provider.CustomMetricInfo{
				GroupResource: schema.GroupResource{Group: "", Resource: "*"},
				Metric:        escapeMetric(descriptor.Type),
				Namespaced:    true,
			})
		}
	}
	return metrics
}

// GetMetricKind returns metricKind for metric metricName, obtained from Stackdriver Monitoring API.
func (t *Translator) GetMetricKind(metricName string) (string, error) {
	response, err := t.service.Projects.MetricDescriptors.Get(fmt.Sprintf("projects/%s/metricDescriptors/%s", t.config.Project, metricName)).Do()
	if err != nil {
		return "", provider.NewNoSuchMetricError(metricName, err)
	}
	return response.MetricKind, nil
}

// GetExternalMetricProject If the metric has "resource.labels.project_id" as a selector, then use a different project
func (t *Translator) GetExternalMetricProject(metricSelector labels.Selector) (string, error) {
	requirements, _ := metricSelector.Requirements()
	for _, req := range requirements {
		if req.Key() == "resource.labels.project_id" {
			if req.Operator() == selection.Equals || req.Operator() == selection.DoubleEquals {
				return req.Values().List()[0], nil
			}
			return "", provider.NewLabelNotAllowedError(fmt.Sprintf("Project selector must use '=' or '==': You used %s", req.Operator()))
		}
	}
	return t.config.Project, nil
}

func getPodNames(list *v1.PodList) []string {
	resourceNames := []string{}
	for _, item := range list.Items {
		resourceNames = append(resourceNames, fmt.Sprintf("%q", item.GetName()))
	}
	return resourceNames
}

func getNodeNames(list *v1.NodeList) []string {
	resourceNames := []string{}
	for _, item := range list.Items {
		resourceNames = append(resourceNames, fmt.Sprintf("%q", item.GetName()))
	}
	return resourceNames
}

func isAllowedLabelName(labelName string) bool {
	for _, prefix := range allowedLabelPrefixes {
		if strings.HasPrefix(labelName, prefix+".") {
			return true
		}
	}
	for _, name := range allowedFullLabelNames {
		if labelName == name {
			return true
		}
	}
	return false
}

func splitMetricLabel(labelName string) (string, string, error) {
	for _, prefix := range allowedLabelPrefixes {
		if strings.HasPrefix(labelName, prefix+".") {
			return prefix, strings.TrimPrefix(labelName, prefix+"."), nil
		}
	}
	return "", "", apierr.NewBadRequest(fmt.Sprintf("Label name: %s is not allowed.", labelName))
}

func getResourceIDs(list *v1.PodList) []string {
	resourceIDs := []string{}
	for _, item := range list.Items {
		resourceIDs = append(resourceIDs, fmt.Sprintf("%q", item.GetUID()))
	}
	return resourceIDs
}

func quoteAll(list []string) []string {
	result := []string{}
	for _, item := range list {
		result = append(result, fmt.Sprintf("%q", item))
	}
	return result
}

func joinFilters(filters ...string) string {
	return strings.Join(filters, " AND ")
}

func (t *Translator) filterForCluster() string {
	projectFilter := fmt.Sprintf("resource.labels.project_id = %q", t.config.Project)
	clusterFilter := fmt.Sprintf("resource.labels.cluster_name = %q", t.config.Cluster)
	locationFilter := fmt.Sprintf("resource.labels.location = %q", t.config.Location)
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, locationFilter)
}

func (t *Translator) filterForMetric(metricName string) string {
	return fmt.Sprintf("metric.type = %q", metricName)
}

func (t *Translator) filterForAnyPod() string {
	return "resource.type = \"k8s_pod\""
}

func (t *Translator) filterForAnyNode() string {
	return "resource.type = \"k8s_node\""
}

func (t *Translator) filterForAnyResource() string {
	return "resource.type = one_of(\"k8s_pod\",\"k8s_node\")"
}

func (t *Translator) filterForPods(podNames []string, namespace string) string {
	if len(podNames) == 0 {
		glog.Fatalf("createFilterForPods called with empty list of pod names")
	} else if len(podNames) == 1 {
		return fmt.Sprintf("resource.labels.namespace_name = %q AND resource.labels.pod_name = %s", namespace, podNames[0])
	}
	return fmt.Sprintf("resource.labels.namespace_name = %q AND resource.labels.pod_name = one_of(%s)", namespace, strings.Join(podNames, ","))
}

func (t *Translator) filterForNodes(nodeNames []string) string {
	if len(nodeNames) == 0 {
		glog.Fatalf("createFilterForNodes called with empty list of node names")
	} else if len(nodeNames) == 1 {
		return fmt.Sprintf("resource.labels.node_name = %s", nodeNames[0])
	}
	return fmt.Sprintf("resource.labels.node_name = one_of(%s)", strings.Join(nodeNames, ","))
}

func (t *Translator) legacyFilterForCluster() string {
	projectFilter := fmt.Sprintf("resource.labels.project_id = %q", t.config.Project)
	// Skip location, since it may be set incorrectly by Heapster for old resource model
	clusterFilter := fmt.Sprintf("resource.labels.cluster_name = %q", t.config.Cluster)
	containerFilter := "resource.labels.container_name = \"\""
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, containerFilter)
}

func (t *Translator) legacyFilterForAnyPod() string {
	return "resource.labels.pod_id != \"\" AND resource.labels.pod_id != \"machine\""
}

func (t *Translator) legacyFilterForPods(podIDs []string) string {
	if len(podIDs) == 0 {
		glog.Fatalf("createFilterForIDs called with empty list of pod IDs")
	} else if len(podIDs) == 1 {
		return fmt.Sprintf("resource.labels.pod_id = %s", podIDs[0])
	}
	return fmt.Sprintf("resource.labels.pod_id = one_of(%s)", strings.Join(podIDs, ","))
}

func (t *Translator) filterForSelector(metricSelector labels.Selector) (string, error) {
	requirements, selectable := metricSelector.Requirements()
	if !selectable {
		return "", apierr.NewBadRequest(fmt.Sprintf("Label selector is impossible to match: %s", metricSelector))
	}
	filters := []string{}
	for _, req := range requirements {
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals:
			if isAllowedLabelName(req.Key()) {
				filters = append(filters, fmt.Sprintf("%s = %q", req.Key(), req.Values().List()[0]))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.NotEquals:
			if isAllowedLabelName(req.Key()) {
				filters = append(filters, fmt.Sprintf("%s != %q", req.Key(), req.Values().List()[0]))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.In:
			if isAllowedLabelName(req.Key()) {
				filters = append(filters, fmt.Sprintf("%s = one_of(%s)", req.Key(), strings.Join(quoteAll(req.Values().List()), ",")))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.NotIn:
			if isAllowedLabelName(req.Key()) {
				filters = append(filters, fmt.Sprintf("NOT %s = one_of(%s)", req.Key(), strings.Join(quoteAll(req.Values().List()), ",")))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.Exists:
			prefix, suffix, err := splitMetricLabel(req.Key())
			if err == nil {
				filters = append(filters, fmt.Sprintf("%s : %s", prefix, suffix))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.DoesNotExist:
			// DoesNotExist is not allowed due to Stackdriver filtering syntax limitation
			return "", apierr.NewBadRequest("Label selector with operator DoesNotExist is not allowed")
		case selection.GreaterThan:
			if isAllowedLabelName(req.Key()) {
				value, err := strconv.ParseInt(req.Values().List()[0], 10, 64)
				if err != nil {
					return "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", req.Values().List()[0]))
				}
				filters = append(filters, fmt.Sprintf("%s > %v", req.Key(), value))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		case selection.LessThan:
			if isAllowedLabelName(req.Key()) {
				value, err := strconv.ParseInt(req.Values().List()[0], 10, 64)
				if err != nil {
					return "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", req.Values().List()[0]))
				}
				filters = append(filters, fmt.Sprintf("%s < %v", req.Key(), value))
			} else {
				return "", provider.NewLabelNotAllowedError(req.Key())
			}
		default:
			return "", provider.NewOperationNotSupportedError(fmt.Sprintf("Selector with operator %q", req.Operator()))
		}
	}
	return strings.Join(filters, " AND "), nil
}

func (t *Translator) getMetricLabels(series *stackdriver.TimeSeries) map[string]string {
	metricLabels := map[string]string{}
	for label, value := range series.Metric.Labels {
		metricLabels["metric.labels."+label] = value
	}
	metricLabels["resource.type"] = series.Resource.Type
	for label, value := range series.Resource.Labels {
		metricLabels["resource.labels."+label] = value
	}
	return metricLabels
}

func (t *Translator) createListTimeseriesRequest(filter string, metricKind string) *stackdriver.ProjectsTimeSeriesListCall {
	return t.createListTimeseriesRequestProject(filter, metricKind, t.config.Project)
}

func (t *Translator) createListTimeseriesRequestProject(filter string, metricKind string, metricProject string) *stackdriver.ProjectsTimeSeriesListCall {
	project := fmt.Sprintf("projects/%s", metricProject)
	endTime := t.clock.Now()
	startTime := endTime.Add(-t.reqWindow)
	// use "ALIGN_NEXT_OLDER" by default, i.e. for metricKind "GAUGE"
	aligner := "ALIGN_NEXT_OLDER"
	alignmentPeriod := t.reqWindow
	if metricKind == "DELTA" || metricKind == "CUMULATIVE" {
		aligner = "ALIGN_RATE"
		alignmentPeriod = t.alignmentPeriod
	}
	return t.service.Projects.TimeSeries.List(project).Filter(filter).
		IntervalStartTime(startTime.Format(time.RFC3339)).
		IntervalEndTime(endTime.Format(time.RFC3339)).
		AggregationPerSeriesAligner(aligner).
		AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(alignmentPeriod.Seconds())))
}

func (t *Translator) getMetricValuesFromResponse(groupResource schema.GroupResource, response *stackdriver.ListTimeSeriesResponse, metricName string) (map[string]resource.Quantity, error) {
	metricValues := make(map[string]resource.Quantity)
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]
		value := point.Value
		name, err := t.metricKey(series)
		if err != nil {
			return nil, err
		}

		currentQuantity, ok := metricValues[name]
		if !ok {
			currentQuantity = *resource.NewQuantity(0, resource.DecimalSI)
		}

		switch {
		case value.Int64Value != nil:
			currentQuantity.Add(*resource.NewQuantity(*value.Int64Value, resource.DecimalSI))
			metricValues[name] = currentQuantity
		case value.DoubleValue != nil:
			currentQuantity.Add(*resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI))
			metricValues[name] = currentQuantity
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
		// TODO(kawych): metric timestamp should be retrieved from Stackdriver response instead
		Timestamp: metav1.Time{t.clock.Now()},
		Value:     value,
	}, nil
}

func (t *Translator) metricsFor(values map[string]resource.Quantity, groupResource schema.GroupResource, metricName string, list []metav1.ObjectMeta) ([]custom_metrics.MetricValue, error) {
	res := make([]custom_metrics.MetricValue, 0)

	for _, item := range list {
		if _, ok := values[t.resourceKey(item)]; !ok {
			glog.V(4).Infof("Metric '%s' not found for pod '%s'", metricName, item.GetName())
			continue
		}
		value, err := t.metricFor(values[t.resourceKey(item)], groupResource, item.GetNamespace(), item.GetName(), metricName)
		if err != nil {
			return nil, err
		}
		res = append(res, *value)
	}

	return res, nil
}

func (t *Translator) getPodItems(list *v1.PodList) []metav1.ObjectMeta {
	items := []metav1.ObjectMeta{}
	for _, item := range list.Items {
		items = append(items, item.ObjectMeta)
	}
	return items
}

func (t *Translator) getNodeItems(list *v1.NodeList) []metav1.ObjectMeta {
	items := []metav1.ObjectMeta{}
	for _, item := range list.Items {
		items = append(items, item.ObjectMeta)
	}
	return items
}

func (t *Translator) resourceKey(object metav1.ObjectMeta) string {
	if t.useNewResourceModel {
		return object.GetNamespace() + ":" + object.GetName()
	}
	return fmt.Sprintf("%s", object.GetUID())
}

func (t *Translator) metricKey(timeSeries *stackdriver.TimeSeries) (string, error) {
	if t.useNewResourceModel {
		switch timeSeries.Resource.Type {
		case "k8s_pod":
			return timeSeries.Resource.Labels["namespace_name"] + ":" + timeSeries.Resource.Labels["pod_name"], nil
		case "k8s_node":
			return ":" + timeSeries.Resource.Labels["node_name"], nil
		}
	} else {
		return timeSeries.Resource.Labels["pod_id"], nil
	}
	return "", apierr.NewInternalError(fmt.Errorf("Stackdriver returned incorrect resource type %q", timeSeries.Resource.Type))
}

func escapeMetric(metricName string) string {
	return strings.Replace(metricName, "/", "|", -1)
}
