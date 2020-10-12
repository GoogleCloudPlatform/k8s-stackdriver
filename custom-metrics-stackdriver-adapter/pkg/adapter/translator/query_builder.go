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
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	stackdriver "google.golang.org/api/monitoring/v3"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog"
)

var (
	// allowedExternalMetricsLabelPrefixes and allowedExternalMetricsFullLabelNames specify all metric labels allowed for querying
	// External Metrics API.
	allowedExternalMetricsLabelPrefixes  = []string{"metric.labels", "resource.labels", "metadata.system_labels", "metadata.user_labels"}
	allowedExternalMetricsFullLabelNames = []string{"resource.type"}
	// allowedCustomMetricsLabelPrefixes and allowedCustomMetricsFullLabelNames specify all metric labels allowed for querying
	allowedCustomMetricsLabelPrefixes  = []string{"metric.labels"}
	allowedCustomMetricsFullLabelNames = []string{}
)

const (
	// AllNamespaces is constant to indicate that there is no namespace filter in query
	AllNamespaces = ""
	// MaxNumOfArgsInOneOfFilter is the maximum value of one_of() function allowed in Stackdriver Filters
	MaxNumOfArgsInOneOfFilter = 100
)

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (c realClock) Now() time.Time {
	return time.Now()
}

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

// NewTranslator creates a Translator
func NewTranslator(service *stackdriver.Service, gceConf *config.GceConfig, rateInterval time.Duration, alignmentPeriod time.Duration, mapper apimeta.RESTMapper, useNewResourceModel bool) *Translator {
	return &Translator{
		service:             service,
		config:              gceConf,
		reqWindow:           rateInterval,
		alignmentPeriod:     alignmentPeriod,
		clock:               realClock{},
		mapper:              mapper,
		useNewResourceModel: useNewResourceModel,
	}
}

// GetSDReqForPods returns Stackdriver request for query for multiple pods.
// podList is required to be no longer than MaxNumOfArgsInOneOfFilter items. This is enforced by limitation of
// "one_of()" operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForPods(podList *v1.PodList, metricName string, metricKind string, metricSelector labels.Selector, namespace string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(podList.Items) == 0 {
		return nil, apierr.NewBadRequest("No objects matched provided selector")
	}
	if len(podList.Items) > MaxNumOfArgsInOneOfFilter {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForPods called with %v pod list, but allowed limit is %v pods", len(podList.Items), MaxNumOfArgsInOneOfFilter))
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
	if metricSelector.Empty() {
		return t.createListTimeseriesRequest(filter, metricKind), nil
	}
	filterForSelector, err := t.filterForSelector(metricSelector, allowedCustomMetricsLabelPrefixes, allowedCustomMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequest(joinFilters(filterForSelector, filter), metricKind), nil
}

// GetSDReqForContainers returns Stackdriver request for container resource from multiple pods.
// podList is required to be no longer than MaxNumOfArgsInOneOfFilter items. This is enforced by limitation of
// "one_of()" operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForContainers(podList *v1.PodList, metricName string, metricKind string, metricSelector labels.Selector, namespace string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	resourceNames := getPodNames(podList)
	return t.GetSDReqForContainersWithNames(resourceNames, metricName, metricKind, metricSelector, namespace)
}

// GetSDReqForContainersWithNames instead of PodList takes array of Pod names as first argument.
// Request for PodList is expensive and not always necessary so it's better to use this method.
// Assumes that resourceNames are double-quoted string.
func (t *Translator) GetSDReqForContainersWithNames(resourceNames []string, metricName string, metricKind string, metricSelector labels.Selector, namespace string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(resourceNames) == 0 {
		return nil, apierr.NewBadRequest("No objects matched provided selector")
	}
	if len(resourceNames) > MaxNumOfArgsInOneOfFilter {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForContainers called with %v pod list, but allowed limit is %v pods", len(resourceNames), MaxNumOfArgsInOneOfFilter))
	}
	if !t.useNewResourceModel {
		return nil, apierr.NewInternalError(fmt.Errorf("Illegal state! Container metrics works only with new resource model"))
	}
	filter := joinFilters(
		t.filterForMetric(metricName),
		t.filterForCluster(),
		t.filterForPods(resourceNames, namespace),
		t.filterForAnyContainer())
	if metricSelector.Empty() {
		return t.createListTimeseriesRequest(filter, metricKind), nil
	}
	filterForSelector, err := t.filterForSelector(metricSelector, allowedCustomMetricsLabelPrefixes, allowedCustomMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequest(joinFilters(filterForSelector, filter), metricKind), nil
}

// GetSDReqForNodes returns Stackdriver request for query for multiple nodes.
// nodeList is required to be no longer than MaxNumOfArgsInOneOfFilter items. This is enforced by limitation of
// "one_of()" operator in Stackdriver filters, see documentation:
// https://cloud.google.com/monitoring/api/v3/filters
func (t *Translator) GetSDReqForNodes(nodeList *v1.NodeList, metricName string, metricKind string, metricSelector labels.Selector) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	resourceNames := getNodeNames(nodeList)
	return t.GetSDReqForNodesWithNames(resourceNames, metricName, metricKind, metricSelector)
}

// GetSDReqForNodesWithNames instead of NodeList takes array of Node names as first argument.
// Request for NodeList could be expensive and not always necessary so it's better to use this method.
// Assumes that resourceNames are double-quoted string.
func (t *Translator) GetSDReqForNodesWithNames(resourceNames []string, metricName string, metricKind string, metricSelector labels.Selector) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if len(resourceNames) == 0 {
		return nil, apierr.NewBadRequest("No objects matched provided selector")
	}
	if len(resourceNames) > MaxNumOfArgsInOneOfFilter {
		return nil, apierr.NewInternalError(fmt.Errorf("GetSDReqForNodes called with %v node list, but allowed limit is %v nodes", len(resourceNames), MaxNumOfArgsInOneOfFilter))
	}
	var filter string
	if !t.useNewResourceModel {
		return nil, NewOperationNotSupportedError("Root scoped metrics are not supported without new Stackdriver resource model enabled")
	}
	filter = joinFilters(
		t.filterForMetric(metricName),
		t.filterForCluster(),
		t.filterForNodes(resourceNames),
		t.filterForAnyNode())
	if metricSelector.Empty() {
		return t.createListTimeseriesRequest(filter, metricKind), nil
	}
	filterForSelector, err := t.filterForSelector(metricSelector, allowedCustomMetricsLabelPrefixes, allowedCustomMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequest(joinFilters(filterForSelector, filter), metricKind), nil
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
	filterForSelector, err := t.filterForSelector(metricSelector, allowedExternalMetricsLabelPrefixes, allowedExternalMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequestProject(joinFilters(filterForMetric, filterForSelector), metricKind, metricProject), nil
}

// ListMetricDescriptors returns Stackdriver request for all custom metrics descriptors.
func (t *Translator) ListMetricDescriptors(fallbackForContainerMetrics bool) *stackdriver.ProjectsMetricDescriptorsListCall {
	var filter string
	if t.useNewResourceModel {
		filter = joinFilters(t.filterForCluster(), t.filterForAnyResource(fallbackForContainerMetrics))
	} else {
		filter = joinFilters(t.legacyFilterForCluster(), t.legacyFilterForAnyPod())
	}
	return t.service.Projects.MetricDescriptors.
		List(fmt.Sprintf("projects/%s", t.config.Project)).
		Filter(filter)
}

// GetMetricKind returns metricKind for metric metricName, obtained from Stackdriver Monitoring API.
func (t *Translator) GetMetricKind(metricName string, metricSelector labels.Selector) (string, error) {
	metricProj := t.config.Project
	requirements, selectable := metricSelector.Requirements()
	if !selectable {
		return "", apierr.NewBadRequest(fmt.Sprintf("Label selector is impossible to match: %s", metricSelector))
	}
	for _, req := range requirements {
		if req.Key() == "resource.labels.project_id" {
			if req.Operator() == selection.Equals || req.Operator() == selection.DoubleEquals {
				metricProj = req.Values().List()[0]
				break
			}
			return "", NewLabelNotAllowedError(fmt.Sprintf("Project selector must use '=' or '==': You used %s", req.Operator()))
		}
	}
	response, err := t.service.Projects.MetricDescriptors.Get(fmt.Sprintf("projects/%s/metricDescriptors/%s", metricProj, metricName)).Do()
	if err != nil {
		return "", NewNoSuchMetricError(metricName, err)
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
			return "", NewLabelNotAllowedError(fmt.Sprintf("Project selector must use '=' or '==': You used %s", req.Operator()))
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

func isAllowedLabelName(labelName string, allowedLabelPrefixes []string, allowedFullLabelNames []string) bool {
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

func splitMetricLabel(labelName string, allowedLabelPrefixes []string) (string, string, error) {
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

func (t *Translator) filterForAnyContainer() string {
	return "resource.type = \"k8s_container\""
}

func (t *Translator) filterForAnyResource(fallbackForContainerMetrics bool) string {
	if fallbackForContainerMetrics {
		return "resource.type = one_of(\"k8s_pod\",\"k8s_node\",\"k8s_container\")"
	}
	return "resource.type = one_of(\"k8s_pod\",\"k8s_node\")"
}

// The namespace string can be empty. If so, all namespaces are allowed.
func (t *Translator) filterForPods(podNames []string, namespace string) string {
	if len(podNames) == 0 {
		klog.Fatalf("createFilterForPods called with empty list of pod names")
	} else if len(podNames) == 1 {
		if namespace == AllNamespaces {
			return fmt.Sprintf("resource.labels.pod_name = %s", podNames[0])
		}
		return fmt.Sprintf("resource.labels.namespace_name = %q AND resource.labels.pod_name = %s", namespace, podNames[0])
	}
	if namespace == AllNamespaces {
		return fmt.Sprintf("resource.labels.pod_name = one_of(%s)", strings.Join(podNames, ","))
	}
	return fmt.Sprintf("resource.labels.namespace_name = %q AND resource.labels.pod_name = one_of(%s)", namespace, strings.Join(podNames, ","))
}

func (t *Translator) filterForNodes(nodeNames []string) string {
	if len(nodeNames) == 0 {
		klog.Fatalf("createFilterForNodes called with empty list of node names")
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
		klog.Fatalf("createFilterForIDs called with empty list of pod IDs")
	} else if len(podIDs) == 1 {
		return fmt.Sprintf("resource.labels.pod_id = %s", podIDs[0])
	}
	return fmt.Sprintf("resource.labels.pod_id = one_of(%s)", strings.Join(podIDs, ","))
}

func (t *Translator) filterForSelector(metricSelector labels.Selector, allowedLabelPrefixes []string, allowedFullLabelNames []string) (string, error) {
	requirements, selectable := metricSelector.Requirements()
	if !selectable {
		return "", apierr.NewBadRequest(fmt.Sprintf("Label selector is impossible to match: %s", metricSelector))
	}
	filters := []string{}
	for _, req := range requirements {
		l := req.Values().List()
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				filters = append(filters, fmt.Sprintf("%s = %q", req.Key(), l[0]))
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.NotEquals:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				filters = append(filters, fmt.Sprintf("%s != %q", req.Key(), l[0]))
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.In:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				if len(l) == 1 {
					filters = append(filters, fmt.Sprintf("%s = %s", req.Key(), l[0]))
				} else {
					filters = append(filters, fmt.Sprintf("%s = one_of(%s)", req.Key(), strings.Join(quoteAll(l), ",")))
				}
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.NotIn:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				if len(l) == 1 {
					filters = append(filters, fmt.Sprintf("%s != %s", req.Key(), l[0]))
				} else {
					filters = append(filters, fmt.Sprintf("NOT %s = one_of(%s)", req.Key(), strings.Join(quoteAll(l), ",")))
				}
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.Exists:
			prefix, suffix, err := splitMetricLabel(req.Key(), allowedLabelPrefixes)
			if err == nil {
				filters = append(filters, fmt.Sprintf("%s : %s", prefix, suffix))
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.DoesNotExist:
			// DoesNotExist is not allowed due to Stackdriver filtering syntax limitation
			return "", apierr.NewBadRequest("Label selector with operator DoesNotExist is not allowed")
		case selection.GreaterThan:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				value, err := strconv.ParseInt(l[0], 10, 64)
				if err != nil {
					return "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", l[0]))
				}
				filters = append(filters, fmt.Sprintf("%s > %v", req.Key(), value))
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		case selection.LessThan:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				value, err := strconv.ParseInt(l[0], 10, 64)
				if err != nil {
					return "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", l[0]))
				}
				filters = append(filters, fmt.Sprintf("%s < %v", req.Key(), value))
			} else {
				return "", NewLabelNotAllowedError(req.Key())
			}
		default:
			return "", NewOperationNotSupportedError(fmt.Sprintf("Selector with operator %q", req.Operator()))
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
		aligner = "ALIGN_RATE" // Calculates integral of metric on segment and divide it by segment length.
		alignmentPeriod = t.alignmentPeriod
	}
	return t.service.Projects.TimeSeries.List(project).Filter(filter).
		IntervalStartTime(startTime.Format(time.RFC3339)).
		IntervalEndTime(endTime.Format(time.RFC3339)).
		AggregationPerSeriesAligner(aligner).
		AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(alignmentPeriod.Seconds())))
}

// GetPodItems returns list Pod Objects
func (t *Translator) GetPodItems(list *v1.PodList) []metav1.ObjectMeta {
	items := []metav1.ObjectMeta{}
	for _, item := range list.Items {
		items = append(items, item.ObjectMeta)
	}
	return items
}

// GetNodeItems returns list Node Objects
func (t *Translator) GetNodeItems(list *v1.NodeList) []metav1.ObjectMeta {
	items := []metav1.ObjectMeta{}
	for _, item := range list.Items {
		items = append(items, item.ObjectMeta)
	}
	return items
}
