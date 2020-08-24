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
	"strings"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	stackdriver "google.golang.org/api/monitoring/v3"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/metrics-server/pkg/api"
)

// GetRespForSingleObject returns translates Stackdriver response to a Custom Metric associated with
// a single object.
func (t *Translator) GetRespForSingleObject(response *stackdriver.ListTimeSeriesResponse, groupResource schema.GroupResource, metricName string, metricSelector labels.Selector, namespace string, name string) (*custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, response, metricName)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, provider.NewMetricNotFoundForError(groupResource, metricName, name)
	}
	if len(values) > 1 {
		return nil, apierr.NewInternalError(fmt.Errorf("Expected exactly one value for resource %q in namespace %q, but received %v values", name, namespace, len(values)))
	}
	// Since len(values) = 1, this loop will execute only once.
	for _, value := range values {
		metricValue, err := t.metricFor(value, groupResource, namespace, name, metricName, metricSelector)
		if err != nil {
			return nil, err
		}
		return metricValue, nil
	}
	// This code is unreacheable
	return nil, apierr.NewInternalError(fmt.Errorf("Illegal state"))
}

// GetRespForMultipleObjects translates Stackdriver response to a Custom Metric associated
// with multiple pods.
func (t *Translator) GetRespForMultipleObjects(response *stackdriver.ListTimeSeriesResponse, list []metav1.ObjectMeta, groupResource schema.GroupResource, metricName string, metricSelector labels.Selector) ([]custom_metrics.MetricValue, error) {
	values, err := t.getMetricValuesFromResponse(groupResource, response, metricName)
	if err != nil {
		return nil, err
	}
	return t.metricsFor(values, groupResource, metricName, metricSelector, list)
}

// GetRespForExternalMetric translates Stackdriver response to list of External Metrics
func (t *Translator) GetRespForExternalMetric(response *stackdriver.ListTimeSeriesResponse, metricName string) ([]external_metrics.ExternalMetricValue, error) {
	metricValues := []external_metrics.ExternalMetricValue{}
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := series.Points[0]
		endTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
		if err != nil {
			return nil, apierr.NewInternalError(fmt.Errorf("Timeseries from Stackdriver has incorrect end time: %s", point.Interval.EndTime))
		}
		metricValue := external_metrics.ExternalMetricValue{
			Timestamp:    metav1.NewTime(endTime),
			MetricName:   metricName,
			MetricLabels: t.getMetricLabels(series),
		}
		value := *point.Value
		switch {
		case value.Int64Value != nil:
			metricValue.Value = *resource.NewQuantity(*value.Int64Value, resource.DecimalSI)
		case value.DoubleValue != nil:
			metricValue.Value = *resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI)
		default:
			return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value))
		}
		metricValues = append(metricValues, metricValue)
	}
	return metricValues, nil
}

// GetMetricsFromSDDescriptorsResp returns an array of MetricInfo for all metric descriptors
// returned by Stackdriver API that satisfy the requirements:
// - valueType is "INT64" or "DOUBLE"
// - metric name doesn't contain "/" character after "custom.googleapis.com/" prefix
func (t *Translator) GetMetricsFromSDDescriptorsResp(response *stackdriver.ListMetricDescriptorsResponse) []provider.CustomMetricInfo {
	metrics := []provider.CustomMetricInfo{}
	for _, descriptor := range response.MetricDescriptors {
		if descriptor.ValueType == "INT64" || descriptor.ValueType == "DOUBLE" {
			metrics = append(metrics, provider.CustomMetricInfo{
				GroupResource: schema.GroupResource{Group: "", Resource: "*"},
				Metric:        escapeMetric(descriptor.Type),
				Namespaced:    true,
			})
		}
	}
	return metrics
}

// GetCoreContainerMetricFromResponse for each pod extracts map from container name to value of metric and TimeInfo.
func (t *Translator) GetCoreContainerMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	containerMetric := make(map[string]map[string]resource.Quantity)
	timeInfo := make(map[string]api.TimeInfo)

	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// TODO(holubowicz): consider changing request window for core metrics
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]
		metricValue, err := getQuantityValue(point)
		if err != nil {
			return nil, nil, err
		}

		podKey, err := t.metricKey(series)
		if err != nil {
			return nil, nil, err
		}
		_, ok := containerMetric[podKey]
		if !ok {
			containerMetric[podKey] = make(map[string]resource.Quantity, 0)

			intervalEndTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
			if err != nil {
				return nil, nil, err
			}
			timeInfo[podKey] = api.TimeInfo{Timestamp: intervalEndTime, Window: t.alignmentPeriod}
		}

		containerName, ok := series.Resource.Labels["container_name"]
		if !ok {
			return nil, nil, apierr.NewInternalError(fmt.Errorf("Container name is not present."))
		}
		_, ok = containerMetric[podKey][containerName]
		if ok {
			return nil, nil, apierr.NewInternalError(fmt.Errorf("The same container appered two time in the response."))
		}
		containerMetric[podKey][containerName] = *metricValue
	}
	return containerMetric, timeInfo, nil
}

// GetCoreNodeMetricFromResponse for each node extracts value of metric and TimeInfo.
func (t *Translator) GetCoreNodeMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	nodeMetric := make(map[string]resource.Quantity)
	timeInfo := make(map[string]api.TimeInfo)

	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]
		metricValue, err := getQuantityValue(point)
		if err != nil {
			return nil, nil, err
		}

		nodeName := series.Resource.Labels["node_name"]
		_, ok := nodeMetric[nodeName]
		if ok {
			return nil, nil, apierr.NewInternalError(fmt.Errorf("The same node appered two time in the response."))
		}
		nodeMetric[nodeName] = *metricValue

		intervalEndTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
		if err != nil {
			return nil, nil, err
		}
		timeInfo[nodeName] = api.TimeInfo{Timestamp: intervalEndTime, Window: t.alignmentPeriod}
	}
	return nodeMetric, timeInfo, nil
}

func getQuantityValue(p stackdriver.Point) (*resource.Quantity, error) {
	switch {
	case p.Value.Int64Value != nil:
		return resource.NewQuantity(*p.Value.Int64Value, resource.DecimalSI), nil
	case p.Value.DoubleValue != nil:
		return resource.NewScaledQuantity(int64(*p.Value.DoubleValue*1000*1000), -6), nil
	default:
		return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", p.Value))
	}
}

// CheckMetricUniquenessForPod checks if each pod has at most one container with given metric
func (t *Translator) CheckMetricUniquenessForPod(response *stackdriver.ListTimeSeriesResponse, metricName string) error {
	metricContainer := make(map[string]string)
	for _, series := range response.TimeSeries {
		name, err := t.metricKey(series)
		if err != nil {
			return err
		}

		containerName, ok := series.Resource.Labels["container_name"]
		if !ok {
			return apierr.NewInternalError(fmt.Errorf("container_name is missing"))
		}
		metricContainerName, ok := metricContainer[name]
		if !ok {
			metricContainer[name] = containerName
		} else {
			if metricContainerName != containerName {
				return apierr.NewBadRequest(fmt.Sprintf("Only one container in pod can have specific metric. Containers %x %x have the same metric %x in pod %x", metricContainerName, containerName, metricName, name))
			}
		}
	}
	return nil
}

func (t *Translator) getMetricValuesFromResponse(groupResource schema.GroupResource, response *stackdriver.ListTimeSeriesResponse, metricName string) (map[string]resource.Quantity, error) {
	metricValues := make(map[string]resource.Quantity)
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return nil, apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]
		value := point.Value
		name, err := t.metricKey(series)
		if err != nil {
			return nil, err
		}

		currentQuantity, ok := metricValues[name]
		if !ok {
			currentQuantity = *resource.NewQuantity(0, resource.DecimalSI)
		}
		switch {
		case value.Int64Value != nil:
			currentQuantity.Add(*resource.NewQuantity(*value.Int64Value, resource.DecimalSI))
			metricValues[name] = currentQuantity
		case value.DoubleValue != nil:
			currentQuantity.Add(*resource.NewMilliQuantity(int64(*value.DoubleValue*1000), resource.DecimalSI))
			metricValues[name] = currentQuantity
		default:
			return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", value))
		}
	}
	return metricValues, nil
}

func (t *Translator) metricFor(value resource.Quantity, groupResource schema.GroupResource, namespace string, name string, metricName string, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	kind, err := t.mapper.KindFor(groupResource.WithVersion(""))
	if err != nil {
		return nil, err
	}
	var metricLabelSelector *metav1.LabelSelector
	if !metricSelector.Empty() {
		metricLabelSelector, err = metav1.ParseToLabelSelector(metricSelector.String())
		if err != nil {
			return nil, err
		}
	}

	return &custom_metrics.MetricValue{
		DescribedObject: custom_metrics.ObjectReference{
			APIVersion: groupResource.Group + "/" + runtime.APIVersionInternal,
			Kind:       kind.Kind,
			Name:       name,
			Namespace:  namespace,
		},
		Metric: custom_metrics.MetricIdentifier{
			Name:     metricName,
			Selector: metricLabelSelector,
		},
		// TODO(kawych): metric timestamp should be retrieved from Stackdriver response instead
		Timestamp: metav1.Time{t.clock.Now()},
		Value:     value,
	}, nil
}

func (t *Translator) metricsFor(values map[string]resource.Quantity, groupResource schema.GroupResource, metricName string, metricSelector labels.Selector, list []metav1.ObjectMeta) ([]custom_metrics.MetricValue, error) {
	res := make([]custom_metrics.MetricValue, 0)

	for _, item := range list {
		if _, ok := values[t.resourceKey(item)]; !ok {
			klog.V(4).Infof("Metric '%s' not found for pod '%s'", metricName, item.GetName())
			continue
		}
		value, err := t.metricFor(values[t.resourceKey(item)], groupResource, item.GetNamespace(), item.GetName(), metricName, metricSelector)
		if err != nil {
			return nil, err
		}
		res = append(res, *value)
	}
	return res, nil
}

func (t *Translator) resourceKey(object metav1.ObjectMeta) string {
	if t.useNewResourceModel {
		return object.GetNamespace() + ":" + object.GetName()
	}
	return fmt.Sprintf("%s", object.GetUID())
}

func (t *Translator) metricKey(timeSeries *stackdriver.TimeSeries) (string, error) {
	if t.useNewResourceModel {
		switch timeSeries.Resource.Type {
		case "k8s_pod":
			return timeSeries.Resource.Labels["namespace_name"] + ":" + timeSeries.Resource.Labels["pod_name"], nil
		case "k8s_container":
			// The same key as pod, because only one container in pod can provide specific metric. Uniqueness is checked in CheckMetricUniquenessForPod.
			return timeSeries.Resource.Labels["namespace_name"] + ":" + timeSeries.Resource.Labels["pod_name"], nil
		case "k8s_node":
			return ":" + timeSeries.Resource.Labels["node_name"], nil
		}
	} else {
		return timeSeries.Resource.Labels["pod_id"], nil
	}
	return "", apierr.NewInternalError(fmt.Errorf("Stackdriver returned incorrect resource type %q", timeSeries.Resource.Type))
}

func escapeMetric(metricName string) string {
	return strings.Replace(metricName, "/", "|", -1)
}
