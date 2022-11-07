package utils

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Helper struct for testing syntax sugar
type filterBuilderChecker struct {
	actual   FilterBuilder
	expected FilterBuilder
}

// Take actual value
func expectFilterBuilder(actual FilterBuilder) filterBuilderChecker {
	return filterBuilderChecker{actual: actual}
}

// Take expected value
func (c filterBuilderChecker) toEqual(expected FilterBuilder) filterBuilderChecker {
	c.expected = expected
	return c
}

// compare actual and expected value, then report it with test suite
func (c filterBuilderChecker) report(t *testing.T) {
	errors := []string{}

	if !reflect.DeepEqual(c.actual.schema, c.expected.schema) {
		errors = append(errors, fmt.Sprintf("\nschema\nExpect: %v\nActual: %v\n", c.expected.schema, c.actual.schema))
	}

	if !reflect.DeepEqual(c.actual.filters, c.expected.filters) {
		errors = append(errors, fmt.Sprintf("\nfilters\nExpect: %v\nActual: %v\n", c.expected.filters, c.actual.filters))
	}
	if !(len(errors) == 0) {
		t.Error(strings.Join(errors, "---"))
	}
}

func TestNewFilterBuilder_default(t *testing.T) {
	actual := NewFilterBuilder("random")
	expected := FilterBuilder{schema: PodSchema, filters: []string{"resource.type = \"random\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestNewFilterBuilder_pod(t *testing.T) {
	actual := NewFilterBuilder(SchemaTypes["pod"])
	expected := FilterBuilder{schema: PodSchema, filters: []string{"resource.type = \"k8s_pod\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestNewFilterBuilder_legacy(t *testing.T) {
	actual := NewFilterBuilder(SchemaTypes["legacy"])
	expected := FilterBuilder{schema: LegacyPodSchema, filters: []string{}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestNewFilterBuilder_prometheus(t *testing.T) {
	actual := NewFilterBuilder(SchemaTypes["prometheus"])
	expected := FilterBuilder{schema: PrometheusSchema, filters: []string{"resource.type = \"prometheus_target\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithMetricType_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	metricType := "random_type"
	actual = actual.WithMetricType(metricType)

	expected := FilterBuilder{schema: schema, filters: []string{"metric.type = \"random_type\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithMetricType_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	metricType := "random_type"
	actual = actual.WithMetricType(metricType)

	expected := FilterBuilder{schema: schema, filters: []string{"metric.type = \"random_type\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithProject_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	project := "random_project"
	actual = actual.WithProject(project)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.project_id = \"random_project\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithProject_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	project := "random_project"
	actual = actual.WithProject(project)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.project_id = \"random_project\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithCluster_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	cluster := "random_cluster"
	actual = actual.WithCluster(cluster)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.cluster_name = \"random_cluster\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithCluster_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	cluster := "random_cluster"
	actual = actual.WithCluster(cluster)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.cluster = \"random_cluster\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithLocation_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	location := "random_location"
	actual = actual.WithLocation(location)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.location = \"random_location\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithLocation_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	location := "random_location"
	actual = actual.WithLocation(location)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.location = \"random_location\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithContainer(t *testing.T) {
	actual := FilterBuilder{filters: []string{}}
	actual = actual.WithContainer()

	expected := FilterBuilder{filters: []string{"resource.labels.container_name = \"\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNamespace_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	namespace := "random_namespace"
	actual = actual.WithNamespace(namespace)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.namespace_name = \"random_namespace\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNamespace_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	namespace := "random_namespace"
	actual = actual.WithNamespace(namespace)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.namespace = \"random_namespace\""}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Single_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := []string{"pod"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.pod_name = pod"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Single_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := []string{"pod"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{"metric.labels.pod = pod"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Single_legacy(t *testing.T) {
	schma := LegacyPodSchema
	actual := FilterBuilder{schema: schma, filters: []string{}}
	pods := []string{"pod"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schma, filters: []string{"resource.labels.pod_id = pod"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Multiple_pod(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := []string{"pod1", "pod2"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.pod_name = one_of(pod1,pod2)"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_101_pods(t *testing.T) {
	schema := PodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := make([]string, 101)
	for i := range pods {
		pods[i] = "pod"
	}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Multiple_prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := []string{"pod1", "pod2"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{"metric.labels.pod = one_of(pod1,pod2)"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithPods_Multiple_legacy(t *testing.T) {
	schema := LegacyPodSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	pods := []string{"pod1", "pod2"}
	actual = actual.WithPods(pods)

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.pod_id = one_of(pod1,pod2)"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNodes_One_Node(t *testing.T) {
	schema := NodeSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	nodes := []string{"node"}
	actual = actual.WithNodes((nodes))

	expected := FilterBuilder{schema: schema, filters: []string{"resource.labels.node_name = monitoring.regex.full_match(\"^(node)$\")"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNodes_101_Nodes(t *testing.T) {
	schema := NodeSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	nodes := make([]string, 101)
	for i := range nodes {
		nodes[i] = "node"
	}
	actual = actual.WithNodes((nodes))

	expected := FilterBuilder{schema: schema, filters: []string{fmt.Sprintf("resource.labels.node_name = monitoring.regex.full_match(\"^(%s)$\")", strings.Join(nodes, "|"))}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNodes_One_Prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	nodes := []string{"node"}
	actual = actual.WithNodes((nodes))

	expected := FilterBuilder{schema: schema, filters: []string{"metric.labels.node = monitoring.regex.full_match(\"^(node)$\")"}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_WithNodes_101_Prometheus(t *testing.T) {
	schema := PrometheusSchema
	actual := FilterBuilder{schema: schema, filters: []string{}}
	nodes := make([]string, 101)
	for i := range nodes {
		nodes[i] = "node"
	}
	actual = actual.WithNodes((nodes))

	expected := FilterBuilder{schema: schema, filters: []string{fmt.Sprintf("metric.labels.node = monitoring.regex.full_match(\"^(%s)$\")", strings.Join(nodes, "|"))}}
	expectFilterBuilder(actual).toEqual(expected).report(t)
}

func TestFilterBuilder_Build(t *testing.T) {
	actual := (FilterBuilder{filters: []string{"d", "f", "e", "a", "c", "b"}}).Build()
	expected := "a AND b AND c AND d AND e AND f"
	if actual != expected {
		t.Errorf("\nQuery\nExpect: %v\nActual: %v\n", expected, actual)
	}
}
