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
	"time"
	"fmt"
	"strings"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/client-go/pkg/api"
	_ "k8s.io/client-go/pkg/api/install"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/scheme"
	stackdriver "google.golang.org/api/monitoring/v3"

	"k8s.io/k8s-stackdriver-adapter/pkg/provider"
	"k8s.io/k8s-stackdriver-adapter/pkg/config"
)

type sdService stackdriver.Service

type StackdriverProvider struct {
	restClient rest.Interface

	values map[provider.MetricInfo]map[string]int64

	service Stackdriver

	config *config.GceConfig

	rateInterval time.Duration
}

type Stackdriver interface {
	ListPodMetric(request *ListPodMetricRequest) (*stackdriver.ListTimeSeriesResponse, error)
	ListAllMetrics(project string, metricPrefix string) (*stackdriver.ListMetricDescriptorsResponse, error)
}

func NewStackdriverProvider(restClient rest.Interface, stackdriverService *stackdriver.Service, rateInterval time.Duration) provider.CustomMetricsProvider {
	gceConf, err := config.GetGceConfig("container.googleapis.com")
	if err != nil {
		glog.Fatalf("Failed to get GCE config: %v", err)
	}

	return &StackdriverProvider{
		restClient: restClient,
		values: make(map[provider.MetricInfo]map[string]int64),
		service: (*sdService)(stackdriverService),
		config: gceConf,
		rateInterval: rateInterval,
	}
}

func (p *StackdriverProvider) getMetricValuesFromResponse(groupResource schema.GroupResource, namespace string, response *stackdriver.ListTimeSeriesResponse) (map[string]int64, error) {
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
	metricValues := make(map[string]int64)
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
			metricValues[name] = *value.Int64Value
		case value.DoubleValue != nil:
			metricValues[name] = int64(*value.DoubleValue)
		default:
			return nil, fmt.Errorf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value)
		}
	}
	return metricValues, nil
	//return 0, fmt.Errorf("Received %v time series from Stackdriver, but non of them matches filters. Series here: %s, groupResource = %s, namespace = %s", len(response.TimeSeries), response.TimeSeries, groupResource, namespace)
}

func (p*StackdriverProvider) groupByFieldsForResource(namespace string) []string {
	return []string{"resource.label.namespace_id"}
}

func (p *StackdriverProvider) valuesFor(groupResource schema.GroupResource, metricName string, namespace string, podIDs []string) (map[string]int64, error) {
	request := &ListPodMetricRequest{
		project: p.config.Project,
		cluster_name: p.config.Cluster,
		zone: p.config.Zone,
		metricName: metricName,
		interval: p.rateInterval,
		pod_ids: podIDs,
	}
	response, err := p.service.ListPodMetric(request)
	if err != nil {
		return nil, err
	}

	values, err := p.getMetricValuesFromResponse(groupResource, namespace, response)
	if err != nil {
		return nil, err
	}

	info := provider.MetricInfo{
		GroupResource: groupResource,
		Metric: metricName,
		Namespaced: namespace != "",
	}

	p.values[info] = values

	return values, nil
}

func (p *StackdriverProvider) metricFor(value int64, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
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
			APIVersion: groupResource.Group+"/"+runtime.APIVersionInternal,
			Kind: kind.Kind,
			Name: name,
			Namespace: namespace,
		},
		MetricName: metricName,
		Timestamp: metav1.Time{time.Now()},
		Value: *resource.NewMilliQuantity(value * 1000, resource.DecimalSI),
	}, nil
}

func (p *StackdriverProvider) metricsFor(values map[string]int64, groupResource schema.GroupResource, metricName string, list runtime.Object) (*custom_metrics.MetricValueList, error) {
	if !apimeta.IsListType(list) {
		return nil, fmt.Errorf("returned object was not a list")
	}

	res := make([]custom_metrics.MetricValue, 0)

	err := apimeta.EachListItem(list, func(item runtime.Object) error {
		objMeta := item.(metav1.ObjectMetaAccessor).GetObjectMeta()
		if _, ok := values[fmt.Sprintf("%s", objMeta.GetUID())]; !ok {
			return fmt.Errorf("No metric found for object: '%s'", objMeta.GetName())
		}
		value, err := p.metricFor(values[fmt.Sprintf("%s", objMeta.GetUID())], groupResource, objMeta.GetNamespace(), objMeta.GetName(), metricName)
		if err != nil {
			return err
		}
		res = append(res, *value)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *StackdriverProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("Non namespaced metrics not supported")
}

func (p *StackdriverProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("Non namespaced metrics not supported")
}

func (p *StackdriverProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	matchingObjectRaw, err := p.restClient.Get().Namespace(namespace).Resource(groupResource.Resource).Name(name).Do().Get()
	if err != nil {
		return nil, err
	}
	objMeta := matchingObjectRaw.(metav1.ObjectMetaAccessor).GetObjectMeta()
	resourceID := fmt.Sprintf("%s", objMeta.GetUID())

	values, err := p.valuesFor(groupResource, metricName, namespace, []string{fmt.Sprintf("\"%s\"", resourceID)})
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("Expected exactly one value for pod %s in namespace %s, but received %v values", name, namespace, len(values))
	}
	metricValue, err := p.metricFor(values[resourceID], groupResource, namespace, name, metricName)
	if err != nil {
		return nil, err
	}
	return metricValue, nil
}

func (p *StackdriverProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	// TODO: work for objects not in core v1
	matchingObjectsRaw, err := p.restClient.Get().
			Namespace(namespace).
			Resource(groupResource.Resource).
			VersionedParams(&metav1.ListOptions{LabelSelector: selector.String()}, scheme.ParameterCodec).
			Do().
			Get()
	if err != nil {
		return nil, err
	}
	resourceIDs, err := getResourceIDs(matchingObjectsRaw)
	if err != nil {
		return nil, err
	}
	if len(resourceIDs) == 0 {
		return nil, fmt.Errorf("No objects found for selector %s", selector)
	}

	values, err := p.valuesFor(groupResource, metricName, namespace, resourceIDs)
	if err != nil {
		return nil, err
	}
	return p.metricsFor(values, groupResource, metricName, matchingObjectsRaw)
}

// List only pod metrics
func (p *StackdriverProvider) ListAllMetrics() []provider.MetricInfo {
	metrics := []provider.MetricInfo{}
	glog.Infof("listing all metrics, project: %s, cluster: %s, metric prefix: %s", p.config.Project, p.config.Cluster, p.config.MetricsPrefix)
	response, err := p.service.ListAllMetrics(p.config.Project, p.config.MetricsPrefix)
	if err != nil {
		glog.Errorf("Failed request to stackdriver api: %s", err)
		return metrics
	}

	for _, descriptor := range response.MetricDescriptors {
		if descriptor.MetricKind == "GAUGE" && (descriptor.ValueType == "INT64" || descriptor.ValueType == "DOUBLE") {
			resources, err := p.fetchMetric(descriptor.Type)
			if err != nil {
				glog.Errorf("unable to fetch metric %s with error", descriptor.Name, err)
				return metrics
			}
			for _, resource := range resources {
				metrics = append(metrics, provider.MetricInfo{
					GroupResource: schema.GroupResource{Group: "", Resource: resource},
					Metric: descriptor.Type,
					Namespaced: true,
				})
			}
		}
	}

	return metrics
}

// returns one element array or empty array, depending on if metric name was returned
func (p *StackdriverProvider) fetchMetric(metricName string) ([]string, error) {
	request := &ListPodMetricRequest{
		project: p.config.Project,
		cluster_name: p.config.Cluster,
		zone: p.config.Zone,
		pod_ids: nil,
		metricName: metricName,
		interval: p.rateInterval,
	}
	glog.Infof("Fetch metric request: %s", *request)
	response, err := p.service.ListPodMetric(request)
	if err != nil {
		return nil, err
	}

	workForPods := false
	for _, series := range response.TimeSeries {
		if series.Resource.Labels["container_name"] == "" && series.Resource.Labels["pod_id"] != "" && series.Resource.Labels["pod_id"] != "machine" {
			workForPods = true
		}
	}
	if workForPods {
		return []string{"pods"}, nil
	}
	return []string{}, nil
}

func getResourceIDs(list runtime.Object) ([]string, error) {
	resourceIDs := []string{}
	err := apimeta.EachListItem(list, func(item runtime.Object) error {
		objMeta := item.(metav1.ObjectMetaAccessor).GetObjectMeta()
		resourceIDs = append(resourceIDs, fmt.Sprintf("\"%s\"", objMeta.GetUID()))
		return nil
	})
	if err == nil {
		glog.Infof("resource names: ", resourceIDs)
	}
	return resourceIDs, err
}

// STACKDRIVER IO FUNCTIONS
// TODO: extract to anoter file

type ListPodMetricRequest struct {
	project string
	cluster_name string
	zone string
	pod_ids []string
  metricName string
	interval time.Duration
}

func createMetricFilter(request *ListPodMetricRequest) (string, error) {
	// project_id, cluster_name and pod_id together identify a pod unambiguously
	projectFilter := fmt.Sprintf("resource.label.project_id = %s", request.project)
	zoneFilter := fmt.Sprintf("resource.label.zone = %s", request.zone)
	clusterFilter := fmt.Sprintf("resource.label.cluster_name = %s", request.cluster_name)
	// container_name is set to empty string for pod metrics
	containerFilter := "resource.label.container_name = \"\""
	metricFilter := fmt.Sprintf("metric.type = \"%s\"", request.metricName)

	var nameFilter string
	if (request.pod_ids == nil) {
		return fmt.Sprintf("(%s) AND (%s) AND (%s) AND (%s) AND (%s)", metricFilter, containerFilter, clusterFilter, projectFilter, zoneFilter), nil
	} else if len(request.pod_ids) == 0 {
		glog.Infof("no pods matched for metric %s", request.metricName)
		return "", fmt.Errorf("no pods matched")
	} else if len(request.pod_ids) == 1 {
		nameFilter = fmt.Sprintf("resource.label.pod_id = %s", request.pod_ids[0])
	} else {
		nameFilter = fmt.Sprintf("resource.label.pod_id = one_of(%s)", strings.Join(request.pod_ids, ","))
	}
	return fmt.Sprintf("(%s) AND (%s) AND (%s) AND (%s) AND (%s) AND (%s)", metricFilter, nameFilter, containerFilter, clusterFilter, projectFilter, zoneFilter), nil
}

func (s *sdService) ListPodMetric(request *ListPodMetricRequest) (*stackdriver.ListTimeSeriesResponse, error) {
	project := fmt.Sprintf("projects/%s", request.project)
	endTime := time.Now()
	startTime := endTime.Add(-request.interval)
	filter, err := createMetricFilter(request)
	if err != nil {
		return nil, err
	}
	sdRequest := s.Projects.TimeSeries.List(project).Filter(filter).
			IntervalStartTime(startTime.Format("2006-01-02T15:04:05Z")).
			IntervalEndTime(endTime.Format("2006-01-02T15:04:05Z")).
			AggregationPerSeriesAligner("ALIGN_MEAN").
			AggregationAlignmentPeriod(fmt.Sprintf("%vs", int64(request.interval.Seconds())))
	glog.Infof("request following: %s", sdRequest)
	return sdRequest.Do()
}

func (s *sdService) ListAllMetrics(project string, prefix string) (*stackdriver.ListMetricDescriptorsResponse, error) {
	onlyCustom := fmt.Sprintf("metric.type = starts_with(\"%s/\")", prefix)
	return s.Projects.MetricDescriptors.List(fmt.Sprintf("projects/%s", project)).Filter(onlyCustom).Do()
}
