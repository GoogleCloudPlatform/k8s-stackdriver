package utils

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog"
)

// Schema defines criteria supported by FilterBuilder.
type Schema struct {
	resourceType string
	metricType   string
	project      string
	cluster      string
	location     string
	namespace    string
	pods         string
	nodes        string
}

const (
	PodSchemaKey        = "pod"               // PodSchemaKey is the key to use pod type filter schema.
	ContainerSchemaKey  = "container"         // ContainerSchemaKey is the key to use container type filter schema.
	PrometheusSchemaKey = "prometheus"        // PrometheusSchemaKey is the key to use prometheus type filter schema.
	NodeSchemaKey       = "node"              // NodeSchemaKey is the key to use node type filter schema
	LegacySchemaKey     = "legacy"            // LegacySchemaKey is the key to use legacy pod type filter schema.
	PodType             = "k8s_pod"           // PodType is the resource value for pod type. (also used in the query)
	ContainerType       = "k8s_container"     // ContainerType is the resource value for container type. (also used in the query)
	NodeType            = "k8s_node"          // NodeType is the resource value for node type. (also used in the query)
	PrometheusType      = "prometheus_target" // PrometheusType is the resource value for prometheus type. (also used in the query)
	LegacyType          = "<not_allowed>"     // LegacyType is the resource value for legacy type. (NOT used in the query)
)

var (
	// PodSchema is the predefined schema for building pod type queries.
	PodSchema = &Schema{
		resourceType: "resource.type",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster_name",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace_name",
		pods:         "resource.labels.pod_name",
	}
	// ContainerSchema is the predefined schema for building container type queries,
	// and it uses the same schema as pod type.
	ContainerSchema = PodSchema
	// LegacyPodSchema is the predefined schema for building legacy pod type queries.
	LegacyPodSchema = &Schema{
		resourceType: "",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster_name",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace_name",
		pods:         "resource.labels.pod_id",
	}
	// NodeSchema is the predefined schema for building node type queries.
	NodeSchema = &Schema{
		resourceType: "resource.type",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster_name",
		location:     "resource.labels.location",
		namespace:    "resource.labels.namespace_name",
		nodes:        "resource.labels.node_name",
	}
	// PrometheusSchema is the predefined schema for building prometheus type queries.
	PrometheusSchema = &Schema{
		resourceType: "resource.type",
		metricType:   "metric.type",
		project:      "resource.labels.project_id",
		cluster:      "resource.labels.cluster",
		location:     "resource.labels.location",
		nodes:        "resource.labels.instance",
		pods:         "metric.labels.pod",
	}
	// SchemaTypes is a collection of all FilterBuilder supported resource types for external uses.
	SchemaTypes = map[string]string{
		PodSchemaKey:        PodType,
		ContainerSchemaKey:  ContainerType,
		PrometheusSchemaKey: PrometheusType,
		NodeSchemaKey:       NodeType,
		LegacySchemaKey:     LegacyType,
	}
)

// FilterBuilder composes criteria into a string which can be used in TimeSeries queries.
type FilterBuilder struct {
	schema  *Schema
	filters []string
}

// NewFilterBuilder is the initializer for FilterBuilder.
//
// Parameter:
//   - resourceType defines the query schema to use
//
// Example:
//
//	// To initialize with a filter "resource.type = \"k8s_pod\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey])
func NewFilterBuilder(resourceType string) FilterBuilder {
	var schema *Schema
	switch resourceType {
	case PodType:
		schema = PodSchema
	case ContainerType:
		schema = ContainerSchema
	case PrometheusType:
		schema = PrometheusSchema
	case NodeType:
		schema = NodeSchema
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
	return FilterBuilder{
		schema:  schema,
		filters: filters,
	}
}

// WithMetricType adds a filter for metric type.
//
// Example:
//
//	// To add "resource.type = \"custom.googleapis.com/foo\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithMetricType("custom.googleapis.com/foo")
func (fb FilterBuilder) WithMetricType(metricType string) FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.metricType, metricType))
	return fb
}

// WithProject adds a filter for project id.
//
// Example:
//
//	// To add "resource.labels.project_id = \"my-project\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithProject("my-project")
func (fb FilterBuilder) WithProject(project string) FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.project, project))
	return fb
}

// WithCluster adds a filter for cluster name.
//
// Example:
//
//	// To add "resource.labels.cluster_name = \"my-cluster\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithCluster("my-cluster")
func (fb FilterBuilder) WithCluster(cluster string) FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.cluster, cluster))
	return fb
}

// WithLocation adds a filter for location.
//
// Example:
//
//	// To add "resource.labels.location = \"my-location\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithLocation("my-location")
func (fb FilterBuilder) WithLocation(location string) FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.location, location))
	return fb
}

// WithContainer adds a filter for container. (used ONLY in the legacy model)
//
// Example:
//
//	// To add "resource.labels.container_name = \"my-container\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithContainer("my-container")
func (fb FilterBuilder) WithContainer() FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.container_name = %q", ""))
	return fb
}

// WithNamespace adds a filter for namespace.
// (note: empty namespace will be ignored)
//
// Example:
//
//	// To add "resource.labels.namespace_name = \"my-namepace\""
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithNamespace("my-namespace")
func (fb FilterBuilder) WithNamespace(namespace string) FilterBuilder {
	if namespace != "" {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = %q", fb.schema.namespace, namespace))
	}
	return fb
}

// WithPods adds a filter for pods.
//
// Number of pods should NOT be more than 100 according to "https://cloud.google.com/monitoring/api/v3/filters#comparisons"
//
// Example:
//
//  1. To add "resource.labels.pod_name = my-pod"
//     filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithPods([]string{"my-pod"})
//
//  2. To add "resource.lables.pod_name = one_of(my-pod-1,my-pod-2)"
//     filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithPods([]string{"my-pod-1", "my-pod-2"})
func (fb FilterBuilder) WithPods(pods []string) FilterBuilder {
	if len(pods) > 100 {
		klog.Warningf("FilterBuilder tries to build with more than 100 pods, thus the pod filter is ignored")
		return fb
	}

	switch len(pods) {
	case 0:
		klog.Warningf("FilterBuilder tries to build with empty pod, thus the pod filter is ignored")
	case 1:
		fb.filters = append(fb.filters, fmt.Sprintf("%s = %s", fb.schema.pods, pods[0]))
	default:
		// one_of(<string>,...,<string>) for up to 100 strings
		fb.filters = append(fb.filters, fmt.Sprintf("%s = one_of(%s)", fb.schema.pods, strings.Join(pods, ",")))
	}

	return fb
}

// WithNodes adds a filter for nodes.
//
// Note:
//
//	for prometheus metrics, their instance information consist of a node name and a port number
//	(such as "gke-test--default-pool-cee13989-qsky:8080"), but when you use this method, you only
//	need to provide target node names (such as "gke-test--default-pool-cee13989-qsky"), because we
//	have "(:\\d+)*" at the end of the regex to ignore the port number part.
//
// Example:
//
//	// To filter for instances gke-test--default-pool-cee13989-0i75 and
//	// gke-test--default-pool-cee13989-qsky:8080 with the query
//	// resource.labels.instance = monitoring.regex.full_match("^" +
//	// 	"(gke-test--default-pool-cee13989-0i75|gke-test--default-pool-cee13989-qsky)" +
//	// 	"(:\\d+)*"
//	// )
//	filterBuilder := NewFilterBuilder(SchemaTypes[PodSchemaKey]).WithNodes([]string{
//		"gke-test--default-pool-cee13989-0i75",
//		"gke-test--default-pool-cee13989-qsky",
//	})
func (fb FilterBuilder) WithNodes(nodes []string) FilterBuilder {
	regex := fmt.Sprintf("^(%s)(:\\d+)*", strings.Join(nodes, "|"))
	fb.filters = append(fb.filters, fmt.Sprintf("%s = monitoring.regex.full_match(%q)", fb.schema.nodes, regex))
	return fb
}

// Build is the last step for FilterBuilder
//
// it combines all filter criteria with AND
func (fb FilterBuilder) Build() string {
	// sort for testing purpose
	sort.Strings(fb.filters)
	query := strings.Join(fb.filters, " AND ")
	klog.Infof("Query with filter(s): %q", query)
	return query
}
