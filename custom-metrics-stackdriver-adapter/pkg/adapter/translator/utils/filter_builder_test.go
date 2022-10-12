package utils

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Helper struct for testing syntax sugar
type FilterBuilderChecker struct {
	actual   *FilterBuilder
	expected *FilterBuilder
}

// Take actual value
func ExpectFilterBuilder(actual *FilterBuilder) *FilterBuilderChecker {
	return &FilterBuilderChecker{actual: actual}
}

// Take expected value
func (c *FilterBuilderChecker) ToEqual(expected *FilterBuilder) *FilterBuilderChecker {
	c.expected = expected
	return c
}

// compare actual and expected value, then report it with test suite
func (c *FilterBuilderChecker) Report(t *testing.T) {
	errors := []string{}
	if c.actual.useLegacyModel != c.expected.useLegacyModel {
		errors = append(errors, fmt.Sprintf("\nuseLegacyModel\nExpect: %v\nActual: %v\n", c.expected.useLegacyModel, c.actual.useLegacyModel))
	}
	if !reflect.DeepEqual(c.actual.filters, c.expected.filters) {
		errors = append(errors, fmt.Sprintf("\nfilters\nExpect: %v\nActual: %v\n", c.expected.filters, c.actual.filters))
	}
	if !(len(errors) == 0) {
		t.Error(strings.Join(errors, "---"))
	}
}

func TestNewFilterBuilder(t *testing.T) {
	actual := NewFilterBuilder("k8s_pod", false)
	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.type = \"k8s_pod\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestNewFilterBuilder_legacy(t *testing.T) {
	actual := NewFilterBuilder("", true)
	expected := &FilterBuilder{useLegacyModel: true, filters: []string{}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestNewFilterBuilder_emptyResourceType(t *testing.T) {
	actual := NewFilterBuilder("", false)
	expected := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithMetricType(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	metricType := "random_type"
	actual.WithMetricType(metricType)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"metric.type = \"random_type\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithProject(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	project := "random_project"
	actual.WithProject(project)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.project_id = \"random_project\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithCluster(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	cluster := "random_cluster"
	actual.WithCluster(cluster)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.cluster_name = \"random_cluster\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithLocation(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	location := "random_location"
	actual.WithLocation(location)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.location = \"random_location\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithContainer(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	actual.WithContainer()

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.container_name = \"\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithNamespace(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	namespace := "random_namespace"
	actual.WithNamespace(namespace)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.namespace_name = \"random_namespace\""}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithPods_Single(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	pods := []string{"pod"}
	actual.WithPods(pods)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.pod_name = pod"}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithPods_Single_legacy(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: true, filters: []string{}}
	pods := []string{"pod"}
	actual.WithPods(pods)

	expected := &FilterBuilder{useLegacyModel: true, filters: []string{"resource.labels.pod_id = pod"}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithPods_Multiple(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: false, filters: []string{}}
	pods := []string{"pod1", "pod2"}
	actual.WithPods(pods)

	expected := &FilterBuilder{useLegacyModel: false, filters: []string{"resource.labels.pod_name = one_of(pod1,pod2)"}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_WithPods_Multiple_legacy(t *testing.T) {
	actual := &FilterBuilder{useLegacyModel: true, filters: []string{}}
	pods := []string{"pod1", "pod2"}
	actual.WithPods(pods)

	expected := &FilterBuilder{useLegacyModel: true, filters: []string{"resource.labels.pod_id = one_of(pod1,pod2)"}}
	ExpectFilterBuilder(actual).ToEqual(expected).Report(t)
}

func TestFilterBuilder_Build(t *testing.T) {
	actual := (&FilterBuilder{useLegacyModel: false, filters: []string{"d", "f", "e", "a", "c", "b"}}).Build()
	expected := "a AND b AND c AND d AND e AND f"
	if actual != expected {
		t.Errorf("\nQuery\nExpect: %v\nActual: %v\n", expected, actual)
	}
}
