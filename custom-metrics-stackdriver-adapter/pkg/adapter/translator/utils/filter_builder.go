package utils

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog"
)

type FilterBuilder struct {
	useLegacyModel bool
	filters        []string
}

// initialize with resource type and whether to use legacy model
func NewFilterBuilder(resourceType string, useLegacyModel bool) *FilterBuilder {
	filters := []string{}
	// in legacy model, it doesn't use resource.type
	if resourceType != "" && !useLegacyModel {
		filters = append(filters, fmt.Sprintf("resource.type = %q", resourceType))
	}
	return &FilterBuilder{
		useLegacyModel: useLegacyModel,
		filters:        filters,
	}
}

func (fb *FilterBuilder) WithMetricType(metricType string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("metric.type = %q", metricType))
	return fb
}

// filter by project id
func (fb *FilterBuilder) WithProject(project string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.project_id = %q", project))
	return fb
}

// filter by cluster name
func (fb *FilterBuilder) WithCluster(cluster string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.cluster_name = %q", cluster))
	return fb
}

// filter by location
func (fb *FilterBuilder) WithLocation(location string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.location = %q", location))
	return fb
}

// used in legacy model
func (fb *FilterBuilder) WithContainer() *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.container_name = %q", ""))
	return fb
}

func (fb *FilterBuilder) WithNamespace(namespace string) *FilterBuilder {
	if namespace != "" {
		fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.namespace_name = %q", namespace))
	}
	return fb
}

func (fb *FilterBuilder) WithPods(pods []string) *FilterBuilder {
	var fieldName string
	if fb.useLegacyModel {
		fieldName = "resource.labels.pod_id"
	} else {
		fieldName = "resource.labels.pod_name"
	}

	if len(pods) == 0 {
		klog.Fatalf("createFilterForPods called with empty list of pod names")
	} else if len(pods) == 1 {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = %s", fieldName, pods[0]))
	} else {
		fb.filters = append(fb.filters, fmt.Sprintf("%s = one_of(%s)", fieldName, strings.Join(pods, ",")))
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
