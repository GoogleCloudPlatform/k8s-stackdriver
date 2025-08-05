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

	stackdriver "google.golang.org/api/monitoring/v3"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator/utils"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
)

var (
	// allowedExternalMetricsLabelPrefixes and allowedExternalMetricsFullLabelNames specify all metric labels allowed for querying
	// External Metrics API.
	allowedExternalMetricsLabelPrefixes  = []string{"metric.labels", "resource.labels", "metadata.system_labels", "metadata.user_labels"}
	allowedExternalMetricsFullLabelNames = []string{"resource.type", "reducer"}
	// allowedCustomMetricsLabelPrefixes and allowedCustomMetricsFullLabelNames specify all metric labels allowed for querying
	allowedCustomMetricsLabelPrefixes  = []string{"metric.labels"}
	allowedCustomMetricsFullLabelNames = []string{"reducer"}
	allowedReducers                    = map[string]bool{
		"REDUCE_NONE":          true,
		"REDUCE_MEAN":          true,
		"REDUCE_MIN":           true,
		"REDUCE_MAX":           true,
		"REDUCE_SUM":           true,
		"REDUCE_STDDEV":        true,
		"REDUCE_COUNT":         true,
		"REDUCE_COUNT_TRUE":    true,
		"REDUCE_COUNT_FALSE":   true,
		"REDUCE_FRACTION_TRUE": true,
		"REDUCE_PERCENTILE_99": true,
		"REDUCE_PERCENTILE_95": true,
		"REDUCE_PERCENTILE_50": true,
		"REDUCE_PERCENTILE_05": true,
	}
)

const (
	// AllNamespaces is constant to indicate that there is no namespace filter in query
	AllNamespaces = ""
	// MaxNumOfArgsInOneOfFilter is the maximum value of one_of() function allowed in Stackdriver Filters
	MaxNumOfArgsInOneOfFilter = 100
	// PrometheusMetricPrefix is the prefix for prometheus metrics
	PrometheusMetricPrefix = "prometheus.googleapis.com"
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
	service              *stackdriver.Service
	config               *config.GceConfig
	reqWindow            time.Duration
	alignmentPeriod      time.Duration
	clock                clock
	mapper               apimeta.RESTMapper
	metricKindCache      *metricKindCache
	useNewResourceModel  bool
	supportDistributions bool
}

// podValues is a helper struct to hold pods values
type podValues struct {
	pods     *v1.PodList
	podNames []string
}

// isPodValuesValid checks if podValues is valid to be used
//
// this only happens when the both pods & podNames are provided,
// for example, when WithPods() and WithPodNames() are both used in QueryBuilder
func (pc podValues) isPodValuesValid() bool {
	return !(len(pc.podNames) > 0 && pc.pods != nil && len(pc.pods.Items) > 0)
}

// isPodValuesEmpty checks if pods container is empty
func (pc podValues) isPodValuesEmpty() bool {
	return len(pc.podNames) == 0 && (pc.pods == nil || len(pc.pods.Items) == 0)
}

// getQuotedPodNames gets quoted pod names from podValues if any provided
func (pc podValues) getQuotedPodNames() []string {
	if pc.pods == nil {
		return pc.podNames
	}
	podNames := make([]string, len(pc.pods.Items))
	for i, item := range pc.pods.Items {
		podNames[i] = fmt.Sprintf("%q", item.GetName())
	}
	return podNames
}

// getPodIDs gets pod ids from podValues if any provided
func (pc podValues) getPodIDs() []string {
	if pc.pods == nil {
		return []string{}
	}
	podIDs := make([]string, len(pc.pods.Items))
	for i, item := range pc.pods.Items {
		podIDs[i] = fmt.Sprintf("%q", item.GetUID())
	}
	return podIDs
}

// nodeValues is a helper struct to hold nodes values
type nodeValues struct {
	nodes     *v1.NodeList
	nodeNames []string
}

// isNodeValuesValid checks if nodeContainer is valid to be used
//
// this only happens when the both nodes & nodeNames are provided,
// for example, when WithNodes() and WithNodeNames() are both used in QueryBuilder
func (nc nodeValues) isNodeValuesValid() bool {
	return !(len(nc.nodeNames) > 0 && nc.nodes != nil && len(nc.nodes.Items) > 0)
}

// isNodeValuesEmpty checks if nodes container is empty
func (nc nodeValues) isNodeValuesEmpty() bool {
	return len(nc.nodeNames) == 0 && (nc.nodes == nil || len(nc.nodes.Items) == 0)
}

// getNodeNames gets node names from nodeContainer if any provided
func (nc nodeValues) getNodeNames() []string {
	if nc.nodes != nil {
		nodeNames := make([]string, len(nc.nodes.Items))
		for i, item := range nc.nodes.Items {
			nodeNames[i] = item.GetName()
		}
		return nodeNames
	}
	return nc.nodeNames
}

// QueryBuilder is a builder for ProjectsTimeSeriesListCall
//
// use NewQueryBuilder() to initialize
type QueryBuilder struct {
	translator           *Translator     // translator provides configurations to filter
	metricName           string          // metricName is the metric name to filter
	metricKind           string          // metricKind is the metric kind to filter
	metricValueType      string          // metricValueType is the metric value type to filter
	metricSelector       labels.Selector // metricSelector is the metric selector to filtere
	namespace            string          // namespace is the namespace to filter (mutually exclusive with nodes)
	pods                 podValues       // pods is the pods to filter (mutually exclusive with nodes)
	nodes                nodeValues      // nodes is the nodes to filter (mutually exclusive with namespace, pods)
	enforceContainerType bool            // enforceContainerType decides whether to enforce using container type filter schema
}

// NewQueryBuilder is the initiator for QueryBuilder
//
// Parameters:
//   - translator, required for configurations.
//   - metricName, required to determine the query schema.
//
// Example:
//
//	queryBuilder := NewQueryBuilder(NewTranslator(...), "custom.googleapis.com/foo")
func NewQueryBuilder(translator *Translator, metricName string) QueryBuilder {
	return QueryBuilder{
		translator: translator,
		metricName: metricName,
	}
}

// WithMetricKind adds a metric kind filter to the QueryBuilder
//
// Example:
//
//	queryBuilder := NewQueryBuilder(translator, metricName).WithMetricKind("GAUGE")
func (qb QueryBuilder) WithMetricKind(metricKind string) QueryBuilder {
	qb.metricKind = metricKind
	return qb
}

// WithMetricValueType adds a metric value type filter to the QueryBuilder
//
// Example:
//
//	queryBuilder := NewQueryBuilder(translator, metricName).WithMetricValueType("INT64")
func (qb QueryBuilder) WithMetricValueType(metricValueType string) QueryBuilder {
	qb.metricValueType = metricValueType
	return qb
}

// WithMetricSelector adds a metric selector filter to the QueryBuilder
//
// Example:
//
//	// labels comes from "k8s.io/apimachinery/pkg/labels"
//	metricSelector, _ := labels.Parse("metric.labels.custom=test")
//	queryBuilder := NewQueryBuilder(translator, metricName).WithMetricSelector(metricSelector)
func (qb QueryBuilder) WithMetricSelector(metricSelector labels.Selector) QueryBuilder {
	qb.metricSelector = metricSelector
	return qb
}

// WithNamespace adds a namespace filter to to the QueryBuilder
//
//   - CANNOT be used with WithNodes
//
// Example:
//
//	queryBuilder := NewQueryBuilder(translator, metricName).WithNamespace("gmp-test")
func (qb QueryBuilder) WithNamespace(namespace string) QueryBuilder {
	qb.namespace = namespace
	return qb
}

// WithPods adds a pod filter to the QueryBuilder (used when namespace is NOT empty)
//
// # ONLY one among WithPods, WithPodNames and WithNodes should be used
//
// Example:
//
//	// v1 comes from "k8s.io/api/core/v1"
//	pod := v1.Pod{
//		ObjectMeta: metav1.ObjectMeta{
//			ClusterName: "my-cluster",
//			UID:         "my-pod-id",
//			Name:        "my-pod-name",
//		},
//	}
//	queryBuilder := NewQueryBuilder(translator, metricName).WithPods(&v1.PodList{Items: []v1.Pod{pod}})
func (qb QueryBuilder) WithPods(pods *v1.PodList) QueryBuilder {
	qb.pods.pods = pods
	return qb
}

// WithPodNames adds a pod filter to the QueryBuilder (used when namespace is NOT empty)
//
// # ONLY one among WithPods, WithPodNames and WithNodes should be used
//
// Example:
//
//	podNames := []string{"pod-1", "pod-2"}
//	queryBuilder := NewQueryBuilder(translator, metricName).WithPodNames(podNames)
func (qb QueryBuilder) WithPodNames(podNames []string) QueryBuilder {
	qb.pods.podNames = podNames
	return qb
}

// WithNodes adds a node filter to the QueryBuilder (used when namespace is empty)
//
//   - ONLY one among WithPods, WithPodNames and WithNodes should be used
//   - CANNOT be used with WithNamespace
//
// Exmaple:
//
//	// v1 comes from "k8s.io/api/core/v1"
//	node := v1.Node{
//		ObjectMeta: metav1.ObjectMeta{
//			ClusterName: "my-cluster",
//			UID:         "my-node-id-1",
//			Name:        "my-node-name-1",
//		},
//	}
//	queryBuilder := NewQueryBuilder(translator, metricName).WithNodes(&v1.NodeList{Items: []v1.Node{node}})
func (qb QueryBuilder) WithNodes(nodes *v1.NodeList) QueryBuilder {
	qb.nodes.nodes = nodes
	return qb
}

func (qb QueryBuilder) WithNodeNames(nodeNames []string) QueryBuilder {
	qb.nodes.nodeNames = nodeNames
	return qb
}

// getResourceNames is an internal helper function to convert pods or nodes to podNames or nodeNames
func (qb QueryBuilder) getResourceNames() []string {
	if qb.translator.useNewResourceModel {
		// new resource model
		if !qb.pods.isPodValuesEmpty() {
			// pods
			return qb.pods.getQuotedPodNames()
		}
		// nodes
		return qb.nodes.getNodeNames()
	}
	// legacy resource model
	return qb.pods.getPodIDs()
}

// AsContainerType enforces to query k8s_container type metrics
//
// it it valid only when useNewResourceModel is true
func (qb QueryBuilder) AsContainerType() QueryBuilder {
	qb.enforceContainerType = true
	return qb
}

// validate is an internal helper function for checking prerequisits before Build
//
// Criteria:
//   - translator has to be provided
//   - only one of nodes or namespace should be set.
//   - only one from podNames, pods, nodes should be set
//   - podList is required to be no longer than MaxNumOfArgsInOneOfFilter items. This is enforced by limitation of
//     "one_of()" operator in Stackdriver filters, see documentation: "https://cloud.google.com/monitoring/api/v3/filters"
//   - metric value type cannot be "DISTRIBUTION" while translator does not support distribution
//   - container type filter schema cannot be used on the legacy resource model
func (qb QueryBuilder) validate() error {
	if qb.translator == nil {
		return apierr.NewInternalError(fmt.Errorf("QueryBuilder tries to build with translator value: nil"))
	}

	if !qb.nodes.isNodeValuesEmpty() {
		// node metric
		if !qb.nodes.isNodeValuesValid() {
			return apierr.NewInternalError(fmt.Errorf("invalid nodes parameter is set to QueryBuilder"))
		}
		if qb.namespace != "" {
			return apierr.NewInternalError(fmt.Errorf("both nodes and namespace are provided, expect only one of them."))
		}
		if !qb.pods.isPodValuesEmpty() {
			return apierr.NewInternalError(fmt.Errorf("both nodes and pods are provided, expect only one of them."))
		}
	} else {
		// pod metric
		if qb.pods.isPodValuesEmpty() {
			return apierr.NewInternalError(fmt.Errorf("no resources are specified for QueryBuilder, expected one of nodes or pods should be used"))
		}
		if !qb.pods.isPodValuesValid() {
			return apierr.NewInternalError(fmt.Errorf("invalid pods parameter is set to QueryBuilder"))
		}
		numPods := len(qb.pods.getQuotedPodNames())
		if numPods > MaxNumOfArgsInOneOfFilter {
			return apierr.NewInternalError(fmt.Errorf("QueryBuilder tries to build with %v pod list, but allowed limit is %v pods", numPods, MaxNumOfArgsInOneOfFilter))
		}
	}

	if qb.metricValueType == "DISTRIBUTION" && !qb.translator.supportDistributions {
		return apierr.NewBadRequest("distributions are not supported")
	}

	if qb.enforceContainerType && !qb.translator.useNewResourceModel {
		return apierr.NewInternalError(fmt.Errorf("illegal state! Container metrics works only with new resource model"))
	}

	return nil
}

// getFilterBuilder is an internal helper function to decide which type of FilterBuilder to use
//
// Prorities:
//  1. if useNewResourceModel is false, then use legacy type FilterBuiler
//  2. if enforceContainerType is true, then use container type FilterBuilder
//  3. if metricName is prefixed with PrometheusMetricPrefix, then use prometheus type FilterBuilder
//  4. if namespace is empty, then use node type FilterBuilder
//  5. By default, use pod type FilterBuilder
func (qb QueryBuilder) getFilterBuilder() utils.FilterBuilder {
	// legacy type FilterBuilder
	if !qb.translator.useNewResourceModel {
		return utils.NewFilterBuilder(utils.SchemaTypes[utils.LegacySchemaKey])
	}

	// container type FilterBuilder
	if qb.enforceContainerType {
		return utils.NewFilterBuilder(utils.SchemaTypes[utils.ContainerSchemaKey])
	}

	// prometheus type FilterBuilder
	if strings.HasPrefix(qb.metricName, PrometheusMetricPrefix) {
		return utils.NewFilterBuilder(utils.SchemaTypes[utils.PrometheusSchemaKey])
	}

	// node type FilterBuilder
	if qb.namespace == "" {
		return utils.NewFilterBuilder(utils.SchemaTypes[utils.NodeSchemaKey])
	}

	// pod type FilterBuilder
	return utils.NewFilterBuilder(utils.SchemaTypes[utils.PodSchemaKey])
}

// composeFilter is an internal helper function to compose filter criteria
// when namespace is NOT empty
func (qb QueryBuilder) composeFilter() string {
	filterBuilder := qb.getFilterBuilder().
		WithMetricType(qb.metricName).
		WithProject(qb.translator.config.Project).
		WithCluster(qb.translator.config.Cluster)

	resourceNames := qb.getResourceNames()
	if qb.translator.useNewResourceModel {
		// new resource model specific filters
		filterBuilder = filterBuilder.WithLocation(qb.translator.config.Location)
		if !qb.nodes.isNodeValuesEmpty() {
			// node metrics
			return filterBuilder.WithNodes(resourceNames).Build()
		}
		// pod metrics
		return filterBuilder.
			WithNamespace(qb.namespace).
			WithPods(resourceNames).
			Build()

	}
	// legacy resource model specific filters
	return filterBuilder.
		WithContainer().
		WithPods(resourceNames).
		Build()
}

// Build is the last step for QueryBuilder which converts itself into a ProjectsTimeSeriesListCall object
//   - has to pass the prerequisits specified in QueryBuilder.validate()
//   - uses the query schema based on useNewResourceModel from translator as well as the metric name.
//   - composes provided filters using the internal tool FilterBuilder
//
// Example:
//
//	projectsTimeSeriesListCall, error = NewQueryBuilder(translator, metricName).Build()
func (qb QueryBuilder) Build() (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if err := qb.validate(); err != nil {
		return nil, err
	}

	filter := qb.composeFilter()

	if qb.metricSelector.Empty() {
		return qb.translator.createListTimeseriesRequest(filter, qb.metricKind, qb.metricValueType, ""), nil
	}

	filterForSelector, reducer, err := qb.translator.filterForSelector(qb.metricSelector, allowedCustomMetricsLabelPrefixes, allowedCustomMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return qb.translator.createListTimeseriesRequest(joinFilters(filterForSelector, filter), qb.metricKind, qb.metricValueType, reducer), nil
}

// NewTranslator creates a Translator
func NewTranslator(service *stackdriver.Service, gceConf *config.GceConfig, rateInterval time.Duration, alignmentPeriod time.Duration, mapper apimeta.RESTMapper, useNewResourceModel, supportDistributions bool, metricKindCacheSize int, metricKindCacheTTL time.Duration) *Translator {
	cache := &metricKindCache{}
	if metricKindCacheTTL > 0 {
		cache = newMetricKindCache(metricKindCacheSize, metricKindCacheTTL)
		klog.Infof("Started stackdriver provider with metric kind cache - size: %d, cache TTL: %v", metricKindCacheSize, metricKindCacheTTL)
	}
	return &Translator{
		service:              service,
		config:               gceConf,
		reqWindow:            rateInterval,
		alignmentPeriod:      alignmentPeriod,
		clock:                realClock{},
		metricKindCache:      cache,
		mapper:               mapper,
		useNewResourceModel:  useNewResourceModel,
		supportDistributions: supportDistributions,
	}
}

// GetExternalMetricRequest returns Stackdriver request for query for external metric.
func (t *Translator) GetExternalMetricRequest(metricName, metricKind, metricValueType string, metricSelector labels.Selector) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	if metricValueType == "DISTRIBUTION" && !t.supportDistributions {
		return nil, apierr.NewBadRequest("Distributions are not supported")
	}
	metricProject, err := t.GetExternalMetricProject(metricSelector)
	if err != nil {
		return nil, err
	}
	filterForMetric := t.filterForMetric(metricName)
	if metricSelector.Empty() {
		return t.createListTimeseriesRequest(filterForMetric, metricKind, metricValueType, ""), nil
	}
	filterForSelector, reducer, err := t.filterForSelector(metricSelector, allowedExternalMetricsLabelPrefixes, allowedExternalMetricsFullLabelNames)
	if err != nil {
		return nil, err
	}
	return t.createListTimeseriesRequestProject(joinFilters(filterForMetric, filterForSelector), metricKind, metricProject, metricValueType, reducer), nil
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
func (t *Translator) GetMetricKind(metricName string, metricSelector labels.Selector) (string, string, error) {
	metricProj := t.config.Project
	cacheKey := metricKindCacheKey{
		project: metricProj,
		name:    metricName,
	}
	if value, ok := t.metricKindCache.get(cacheKey); ok {
		return value.MetricKind, value.ValueType, nil
	}

	requirements, selectable := metricSelector.Requirements()
	if !selectable {
		return "", "", apierr.NewBadRequest(fmt.Sprintf("Label selector is impossible to match: %s", metricSelector))
	}
	for _, req := range requirements {
		if req.Key() == "resource.labels.project_id" {
			if req.Operator() == selection.Equals || req.Operator() == selection.DoubleEquals {
				metricProj = req.Values().List()[0]
				break
			}
			return "", "", NewLabelNotAllowedError(fmt.Sprintf("Project selector must use '=' or '==': You used %s", req.Operator()))
		}
	}
	response, err := t.service.Projects.MetricDescriptors.Get(fmt.Sprintf("projects/%s/metricDescriptors/%s", metricProj, metricName)).Do()
	if err != nil {
		return "", "", NewNoSuchMetricError(metricName, err)
	}
	t.metricKindCache.add(cacheKey, cachedMetricInfo{
		MetricKind: response.MetricKind,
		ValueType:  response.ValueType,
	})
	return response.MetricKind, response.ValueType, nil
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

func quoteAll(list []string) []string {
	result := []string{}
	for _, item := range list {
		result = append(result, fmt.Sprintf("%q", item))
	}
	return result
}

// Deprecated, use FilterBuilder instead
func joinFilters(filters ...string) string {
	nonEmpty := []string{}
	for _, f := range filters {
		if f != "" {
			nonEmpty = append(nonEmpty, f)
		}
	}
	return strings.Join(nonEmpty, " AND ")
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForCluster() string {
	projectFilter := fmt.Sprintf("resource.labels.project_id = %q", t.config.Project)
	clusterFilter := fmt.Sprintf("resource.labels.cluster_name = %q", t.config.Cluster)
	locationFilter := fmt.Sprintf("resource.labels.location = %q", t.config.Location)
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, locationFilter)
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForMetric(metricName string) string {
	return fmt.Sprintf("metric.type = %q", metricName)
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForAnyPod() string {
	return "resource.type = \"k8s_pod\""
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForAnyNode() string {
	return "resource.type = \"k8s_node\""
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForAnyContainer() string {
	return "resource.type = \"k8s_container\""
}

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForAnyResource(fallbackForContainerMetrics bool) string {
	if fallbackForContainerMetrics {
		return "resource.type = one_of(\"k8s_pod\",\"k8s_node\",\"k8s_container\")"
	}
	return "resource.type = one_of(\"k8s_pod\",\"k8s_node\")"
}

// Deprecated, use FilterBuilder instead
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

// Deprecated, use FilterBuilder instead
func (t *Translator) filterForNodes(nodeNames []string) string {
	if len(nodeNames) == 0 {
		klog.Fatalf("createFilterForNodes called with empty list of node names")
	} else if len(nodeNames) == 1 {
		return fmt.Sprintf("resource.labels.node_name = %s", nodeNames[0])
	}
	return fmt.Sprintf("resource.labels.node_name = one_of(%s)", strings.Join(nodeNames, ","))
}

// Deprecated, use FilterBuilder instead
func (t *Translator) legacyFilterForCluster() string {
	projectFilter := fmt.Sprintf("resource.labels.project_id = %q", t.config.Project)
	// Skip location, since it may be set incorrectly by Heapster for old resource model
	clusterFilter := fmt.Sprintf("resource.labels.cluster_name = %q", t.config.Cluster)
	containerFilter := "resource.labels.container_name = \"\""
	return fmt.Sprintf("%s AND %s AND %s", projectFilter, clusterFilter, containerFilter)
}

// Deprecated, use FilterBuilder instead
func (t *Translator) legacyFilterForAnyPod() string {
	return "resource.labels.pod_id != \"\" AND resource.labels.pod_id != \"machine\""
}

// Deprecated, use FilterBuilder instead
func (t *Translator) legacyFilterForPods(podIDs []string) string {
	if len(podIDs) == 0 {
		klog.Fatalf("createFilterForIDs called with empty list of pod IDs")
	} else if len(podIDs) == 1 {
		return fmt.Sprintf("resource.labels.pod_id = %s", podIDs[0])
	}
	return fmt.Sprintf("resource.labels.pod_id = one_of(%s)", strings.Join(podIDs, ","))
}

func (t *Translator) filterForSelector(metricSelector labels.Selector, allowedLabelPrefixes []string, allowedFullLabelNames []string) (string, string, error) {
	requirements, selectable := metricSelector.Requirements()
	if !selectable {
		return "", "", apierr.NewBadRequest(fmt.Sprintf("Label selector is impossible to match: %s", metricSelector))
	}
	filters := []string{}
	var reducer string
	for _, req := range requirements {
		if req.Key() == "reducer" {
			if req.Operator() != selection.Equals && req.Operator() != selection.DoubleEquals {
				return "", "", NewLabelNotAllowedError(fmt.Sprintf("Reducer must use '=' or '==': You used %s", req.Operator()))
			}
			if req.Values().Len() != 1 {
				return "", "", NewLabelNotAllowedError("Reducer must select a single value")
			}
			r, found := req.Values().PopAny()
			if !found {
				return "", "", NewLabelNotAllowedError("Reducer must specify a value")
			}
			if !allowedReducers[r] {
				return "", "", NewLabelNotAllowedError("Specified reducer is not supported: " + r)
			}
			reducer = r
			continue
		}
		l := req.Values().List()
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				filters = append(filters, fmt.Sprintf("%s = %q", req.Key(), l[0]))
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.NotEquals:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				filters = append(filters, fmt.Sprintf("%s != %q", req.Key(), l[0]))
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.In:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				if len(l) == 1 {
					filters = append(filters, fmt.Sprintf("%s = %s", req.Key(), l[0]))
				} else {
					filters = append(filters, fmt.Sprintf("%s = one_of(%s)", req.Key(), strings.Join(quoteAll(l), ",")))
				}
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.NotIn:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				if len(l) == 1 {
					filters = append(filters, fmt.Sprintf("%s != %s", req.Key(), l[0]))
				} else {
					filters = append(filters, fmt.Sprintf("NOT %s = one_of(%s)", req.Key(), strings.Join(quoteAll(l), ",")))
				}
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.Exists:
			prefix, suffix, err := splitMetricLabel(req.Key(), allowedLabelPrefixes)
			if err == nil {
				filters = append(filters, fmt.Sprintf("%s : %s", prefix, suffix))
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.DoesNotExist:
			// DoesNotExist is not allowed due to Stackdriver filtering syntax limitation
			return "", "", apierr.NewBadRequest("Label selector with operator DoesNotExist is not allowed")
		case selection.GreaterThan:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				value, err := strconv.ParseInt(l[0], 10, 64)
				if err != nil {
					return "", "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", l[0]))
				}
				filters = append(filters, fmt.Sprintf("%s > %v", req.Key(), value))
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		case selection.LessThan:
			if isAllowedLabelName(req.Key(), allowedLabelPrefixes, allowedFullLabelNames) {
				value, err := strconv.ParseInt(l[0], 10, 64)
				if err != nil {
					return "", "", apierr.NewInternalError(fmt.Errorf("Unexpected error: value %s could not be parsed to integer", l[0]))
				}
				filters = append(filters, fmt.Sprintf("%s < %v", req.Key(), value))
			} else {
				return "", "", NewLabelNotAllowedError(req.Key())
			}
		default:
			return "", "", NewOperationNotSupportedError(fmt.Sprintf("Selector with operator %q", req.Operator()))
		}
	}
	return strings.Join(filters, " AND "), reducer, nil
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

func (t *Translator) createListTimeseriesRequest(filter, metricKind, metricValueType, reducer string) *stackdriver.ProjectsTimeSeriesListCall {
	return t.createListTimeseriesRequestProject(filter, metricKind, t.config.Project, metricValueType, reducer)
}

func (t *Translator) createListTimeseriesRequestProject(filter, metricKind, metricProject, metricValueType, reducer string) *stackdriver.ProjectsTimeSeriesListCall {
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
	if metricValueType == "DISTRIBUTION" {
		aligner = "ALIGN_DELTA"
	}
	ptslc := t.service.Projects.TimeSeries.List(project).Filter(filter).
		IntervalStartTime(startTime.Format(time.RFC3339)).
		IntervalEndTime(endTime.Format(time.RFC3339)).
		AggregationPerSeriesAligner(aligner).
		AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(alignmentPeriod.Seconds())))
	if reducer != "" {
		ptslc = ptslc.AggregationCrossSeriesReducer(reducer)
	}
	return ptslc
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

func isDistribution(metricSelector labels.Selector) bool {
	requirements, _ := metricSelector.Requirements()
	for _, req := range requirements {
		if req.Key() == "reducer" {
			return true
		}
	}
	return false
}
