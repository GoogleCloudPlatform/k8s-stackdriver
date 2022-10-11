package utils

import (
	"fmt"
	"strings"

	"k8s.io/klog"
)

type FilterBuilder struct {
	filters []string
}

func NewFilterBuilder(resourceType string) *FilterBuilder {
	return &FilterBuilder{
		filters: []string{fmt.Sprintf("resource.type = %q", resourceType)},
	}
}

func (fb *FilterBuilder) WithMetricType(metricType string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("metric.type = %q", metricType))
	return fb
}

func (fb *FilterBuilder) WithProject(project string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.project_id = %q", project))
	return fb
}

func (fb *FilterBuilder) WithCluster(cluster string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.cluster_name = %q", cluster))
	return fb
}

func (fb *FilterBuilder) WithLocation(location string) *FilterBuilder {
	fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.location = %q", location))
	return fb
}

func (fb *FilterBuilder) WithNamespace(namespace string) *FilterBuilder {
	if namespace != "" {
		fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.namespace_name = %q", namespace))
	}
	return fb
}

func (fb *FilterBuilder) WithPods(pods []string) *FilterBuilder {
	if len(pods) == 0 {
		klog.Fatalf("createFilterForPods called with empty list of pod names")
	} else if len(pods) == 1 {
		fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.pod_name = %s", pods[0]))
	} else {
		fb.filters = append(fb.filters, fmt.Sprintf("resource.labels.pod_name = one_of(%s)", strings.Join(pods, ",")))
	}

	return fb
}

func (fb *FilterBuilder) Build() string {
	query := strings.Join(fb.filters, " AND ")
	klog.Infof("Query with filter(s): %q", query)
	return query
}
