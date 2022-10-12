package utils

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog"
)

type Schema struct {
	resourceType string
	metricType   string
	project      string
	cluster      string
	location     string
	namespace    string
	pods         string
}

const (
	PodType        = "k8s_pod"
	PrometheusType = "prometheus_target"
	LegacyType     = "<not_allowed>"
)

var (
	PodSchema = &Schema{
		resourceType: "resource.type",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster_name",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace_name",
		pods:         "resource.labels.pod_name",
	}
	LegacyPodSchema = &Schema{
		resourceType: "",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster_name",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace_name",
		pods:         "resource.labels.pod_id",
	}
	PrometheusSchema = &Schema{
		resourceType: "resource.type",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace",
		pods:         "metric.labels.pod",
	}
	SchemaTypes = map[string]string{
		"pod":        PodType,
		"prometheus": PrometheusType,
		"legacy":     LegacyType,
	}
)

type FilterBuilder struct {
	schema  *Schema
	filters []string
}

// initialize with resource type and whether to use legacy model
// For legacy pod, use NewFilterBuilder(AnyType, false)
func NewFilterBuilder(resourceType string) *FilterBuilder {
	var schema *Schema
	switch resourceType {
	case PrometheusType:
		schema = PrometheusSchema
	case LegacyType:
		schema = LegacyPodSchema
	default:
		schema = PodSchema
	}
	filters := []string{}
	// in legacy resource model, it doesn't use resource.type
	if resourceType != LegacyType && schema.resourceType != "" {
		filters = append(filters, fmt.Sprintf("%s = %q", schema.resourceType, resourceType))
	}
	return &FilterBuilder{
		schema:  schema,
		filters: filters,
	}
}

func (fb *FilterBuilder) WithMetricType(metricType string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.metricType, metricType))
	return fb
}

// filter by project id
func (fb *FilterBuilder) WithProject(project string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.project, project))
	return fb
}

// filter by cluster name
func (fb *FilterBuilder) WithCluster(cluster string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.cluster, cluster))
	return fb
}

// filter by location
func (fb *FilterBuilder) WithLocation(location string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.location, location))
	return fb
}

// used in legacy model
func (fb *FilterBuilder) WithContainer() *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.container_name = %q", ""))
	return fb
}

func (fb *FilterBuilder) WithNamespace(namespace string) *FilterBuilder {
	if namespace != "" {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.namespace, namespace))
	}
	return fb
}

func (fb *FilterBuilder) WithPods(pods []string) *FilterBuilder {
	if len(pods) == 0 {
		klog.Fatalf("createFilterForPods called with empty list of pod names")
	} else if len(pods) == 1 {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = %s", fb.schema.pods, pods[0]))
	} else {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = one_of(%s)", fb.schema.pods, strings.Join(pods, ",")))
	}

	return fb
}

func (fb *FilterBuilder) Build() string {
	// sort for testing purpose
	sort.Strings(fb.filters)
	query := strings.Join(fb.filters, " AND ")
	klog.Infof("Query with filter(s): %q", query)
	return query
}
