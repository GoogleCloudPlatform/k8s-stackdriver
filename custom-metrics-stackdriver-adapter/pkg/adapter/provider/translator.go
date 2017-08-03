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

	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api"

	// Install registers the API group and adds types to a scheme.
	_ "k8s.io/client-go/pkg/api/install"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
)

// Translator is a structure used to translate between Custom Metrics API and Stackdriver API
type Translator struct {
	service   *stackdriver.Service
	config    *config.GceConfig
	reqWindow time.Duration
	clock     clock
}

// GetSDReqForPod returns Stackdriver request for query for a single pod.
func (t *Translator) GetSDReqForPod(pod runtime.Object, metricName string) (*stackdriver.ProjectsTimeSeriesListCall, error) {
	objMeta := pod.(metav1.ObjectMetaAccessor).GetObjectMeta()
	resourceID := fmt.Sprintf("%s", objMeta.GetUID())

	project := fmt.Sprintf("projects/%s", t.config.Project)
	endTime := t.clock.Now()
	startTime := endTime.Add(-t.reqWindow)
	filter, err := t.createFilter([]string{fmt.Sprintf("%q", resourceID)}, t.config.Project, t.config.Zone, t.config.Cluster, metricName)
	if err != nil {
		return nil, err
	}
	return t.service.Projects.TimeSeries.List(project).Filter(filter).
		IntervalStartTime(startTime.Format(time.RFC3339)).
		IntervalEndTime(endTime.Format(time.RFC3339)).
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(t.reqWindow.Seconds()))), nil
}

// GetRespForPod returns translates Stackdriver response to a Custom Metric associated with a single
// pod.
func (t *Translator) GetRespForPod(response *stackdriver.ListTimeSeriesResponse, groupResource schema.GroupResource, metricName string, namespace string, name string) (*custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, namespace, response)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("Expected exactly one value for pod %s in namespace %s, but received %v values", name, namespace, len(values))
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
	return nil, fmt.Errorf("Illegal state")
}

func (t *Translator) createFilter(podIDs []string, project string, zone string, clusterName string, metricName string) (string, error) {
	// project_id, cluster_name and pod_id together identify a pod unambiguously
	projectFilter := fmt.Sprintf("resource.label.project_id = %q", project)
	zoneFilter := fmt.Sprintf("resource.label.zone = %q", zone)
	clusterFilter := fmt.Sprintf("resource.label.cluster_name = %q", strings.TrimSpace(clusterName))
	// container_name is set to empty string for pod metrics
	containerFilter := "resource.label.container_name = \"\""
	metricFilter := fmt.Sprintf("metric.type = \"%s/%s\"", t.config.MetricsPrefix, metricName)

	var nameFilter string
	if len(podIDs) == 0 {
		return "", fmt.Errorf("No pods matched for metric %s", metricName)
	} else if len(podIDs) == 1 {
		nameFilter = fmt.Sprintf("resource.label.pod_id = %s", podIDs[0])
	} else {
		nameFilter = fmt.Sprintf("resource.label.pod_id = one_of(%s)", strings.Join(podIDs, ","))
	}
	return fmt.Sprintf("(%s) AND (%s) AND (%s) AND (%s) AND (%s) AND (%s)", metricFilter, projectFilter, clusterFilter, zoneFilter, nameFilter, containerFilter), nil
}

func (t *Translator) getMetricValuesFromResponse(groupResource schema.GroupResource, namespace string, response *stackdriver.ListTimeSeriesResponse) (map[string]resource.Quantity, error) {
	group, err := api.Registry.Group(groupResource.Group)
	if err != nil {
		return nil, err
	}
	_, err = api.Registry.RESTMapper().KindFor(groupResource.WithVersion(group.GroupVersion.Version))
	if err != nil {
		return nil, err
	}

	if len(response.TimeSeries) < 1 {
		return nil, fmt.Errorf("Expected at least one time series from Stackdriver, but received %v", len(response.TimeSeries))
	}
	metricValues := make(map[string]resource.Quantity)
	// Find time series with specified labels matching
	// Stackdriver API doesn't allow complex label filtering (i.e. "label1 = x AND (label2 = y OR label2 = z)"),
	// therefore only part of the filters is passed and remaining filtering is done here.
	for _, series := range response.TimeSeries {
		if len(series.Points) != 1 {
			return nil, fmt.Errorf("Expected exactly one Point in TimeSeries from Stackdriver, but received %v", len(series.Points))
		}
		value := *series.Points[0].Value
		name := series.Resource.Labels["pod_id"]
		switch {
		case value.Int64Value != nil:
			metricValues[name] = *resource.NewQuantity(*value.Int64Value, resource.DecimalSI)
		case value.DoubleValue != nil:
			metricValues[name] = *resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI)
		default:
			return nil, fmt.Errorf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value)
		}
	}
	return metricValues, nil
}
func (t *Translator) metricFor(value resource.Quantity, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	group, err := api.Registry.Group(groupResource.Group)
	if err != nil {
		return nil, err
	}
	kind, err := api.Registry.RESTMapper().KindFor(groupResource.WithVersion(group.GroupVersion.Version))
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: api.ObjectReference{
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
