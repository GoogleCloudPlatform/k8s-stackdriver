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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	stackdriver "google.golang.org/api/monitoring/v3"
	v1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
)

// TODO(kawych):
// * Support long responses from Stackdriver (pagination).

const (
	nodeResource = "nodes"
	podResource  = "pods"
)

// StackdriverProvider is a provider of custom metrics from Stackdriver.
type StackdriverProvider struct {
	kubeClient                  *corev1.CoreV1Client
	stackdriverService          *stackdriver.Service
	config                      *config.GceConfig
	rateInterval                time.Duration
	translator                  *translator.Translator
	useNewResourceModel         bool
	metricsCache                []provider.CustomMetricInfo
	fallbackForContainerMetrics bool
	externalMetricsCache        *externalMetricsCache
}

// NewStackdriverProvider creates a StackdriverProvider
func NewStackdriverProvider(kubeClient *corev1.CoreV1Client, mapper apimeta.RESTMapper, gceConf *config.GceConfig, stackdriverService *stackdriver.Service, translator *translator.Translator, rateInterval time.Duration, useNewResourceModel bool, fallbackForContainerMetrics bool, customMetricsListCache []provider.CustomMetricInfo, externalMetricCacheTTL time.Duration, externalMetricCacheSize int) provider.MetricsProvider {
	p := &StackdriverProvider{
		kubeClient:                  kubeClient,
		stackdriverService:          stackdriverService,
		config:                      gceConf,
		rateInterval:                rateInterval,
		translator:                  translator,
		useNewResourceModel:         useNewResourceModel,
		fallbackForContainerMetrics: fallbackForContainerMetrics,
		metricsCache:                customMetricsListCache,
	}

	if externalMetricCacheTTL > 0 {
		p.externalMetricsCache = newExternalMetricsCache(externalMetricCacheSize, externalMetricCacheTTL)
		klog.Infof("Started stackdriver provider with cache size: %d, cache TTL: %v", externalMetricCacheSize, externalMetricCacheTTL)
	} else {
		klog.Info("Stackdriver provider external metric cache is disabled. To enable the cache, ensure that --external-metric-cache-ttl is provided with non-zero value")
	}

	return p
}

// GetMetricByName fetches a particular metric for a particular object.
// The namespace will be empty if the metric is root-scoped.
func (p *StackdriverProvider) GetMetricByName(ctx context.Context, name types.NamespacedName, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	if name.Namespace == "" {
		return p.getRootScopedMetricByName(info.GroupResource, name.Name, info.Metric, metricSelector)
	}
	return p.getNamespacedMetricByName(info.GroupResource, name.Namespace, name.Name, info.Metric, metricSelector)
}

// GetMetricBySelector fetches a particular metric for a set of objects matching
// the given label selector. The namespace will be empty if the metric is root-scoped.
func (p *StackdriverProvider) GetMetricBySelector(ctx context.Context, namespace string, selector labels.Selector, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValueList, error) {
	if namespace == "" {
		return p.getRootScopedMetricBySelector(info.GroupResource, selector, info.Metric, metricSelector)
	}
	return p.getNamespacedMetricBySelector(info.GroupResource, namespace, selector, info.Metric, metricSelector)
}

// getRootScopedMetricByName queries Stackdriver for metrics identified by name and not associated
// with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) getRootScopedMetricByName(groupResource schema.GroupResource, name string, escapedMetricName string, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	if !p.useNewResourceModel {
		return nil, NewOperationNotSupportedError("Get root scoped metric by name")
	}
	if groupResource.Resource != nodeResource {
		return nil, NewOperationNotSupportedError(fmt.Sprintf("Get root scoped metric by name for resource %q", groupResource.Resource))
	}
	matchingNode, err := p.kubeClient.Nodes().Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	metricName := getCustomMetricName(escapedMetricName)
	metricKind, metricValueType, err := p.translator.GetMetricKind(metricName, metricSelector)
	if err != nil {
		return nil, err
	}
	stackdriverRequest, err := translator.NewQueryBuilder(p.translator, metricName).
		WithNodes(&v1.NodeList{Items: []v1.Node{*matchingNode}}).
		WithMetricKind(metricKind).
		WithMetricValueType(metricValueType).
		WithMetricSelector(metricSelector).
		Build()
	if err != nil {
		return nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, err
	}
	return p.translator.GetRespForSingleObject(stackdriverResponse, groupResource, escapedMetricName, metricSelector, "", name)
}

// getRootScopedMetricBySelector queries Stackdriver for metrics identified by selector and not
// associated with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) getRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, escapedMetricName string, metricSelector labels.Selector) (*custom_metrics.MetricValueList, error) {
	if !p.useNewResourceModel {
		return nil, NewOperationNotSupportedError("Get root scoped metric by selector")
	}
	if groupResource.Resource != nodeResource {
		return nil, NewOperationNotSupportedError(fmt.Sprintf("Get root scoped metric by selector for resource %q", groupResource.Resource))
	}
	matchingNodes, err := p.kubeClient.Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	metricName := getCustomMetricName(escapedMetricName)
	metricKind, metricValueType, err := p.translator.GetMetricKind(metricName, metricSelector)
	if err != nil {
		return nil, err
	}
	result := []custom_metrics.MetricValue{}
	for i := 0; i < len(matchingNodes.Items); i += translator.MaxNumOfArgsInOneOfFilter {
		sliceSegmentEnd := min(i+translator.MaxNumOfArgsInOneOfFilter, len(matchingNodes.Items))
		nodesSlice := &v1.NodeList{Items: matchingNodes.Items[i:sliceSegmentEnd]}
		stackdriverRequest, err := translator.NewQueryBuilder(p.translator, metricName).
			WithNodes(nodesSlice).
			WithMetricKind(metricKind).
			WithMetricValueType(metricValueType).
			WithMetricSelector(metricSelector).
			Build()
		if err != nil {
			return nil, err
		}
		stackdriverResponse, err := stackdriverRequest.Do()
		if err != nil {
			return nil, err
		}
		slice, err := p.translator.GetRespForMultipleObjects(stackdriverResponse, p.translator.GetNodeItems(matchingNodes), groupResource, escapedMetricName, metricSelector)
		if err != nil {
			return nil, err
		}
		result = append(result, slice...)
	}
	return &custom_metrics.MetricValueList{Items: result}, nil
}

// GetNamespacedMetricByName queries Stackdriver for metrics identified by name and associated
// with a namespace.
func (p *StackdriverProvider) getNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, escapedMetricName string, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	if groupResource.Resource != podResource {
		return nil, NewOperationNotSupportedError(fmt.Sprintf("Get namespaced metric by name for resource %q", groupResource.Resource))
	}

	matchingPod, err := p.kubeClient.Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	metricName := getCustomMetricName(escapedMetricName)
	metricKind, metricValueType, err := p.translator.GetMetricKind(metricName, metricSelector)
	if err != nil {
		return nil, err
	}
	queryBuilder := translator.NewQueryBuilder(p.translator, metricName)

	pods := &v1.PodList{Items: []v1.Pod{*matchingPod}}
	stackdriverRequest, err := queryBuilder.
		WithPods(pods).
		WithMetricKind(metricKind).
		WithMetricValueType(metricValueType).
		WithMetricSelector(metricSelector).
		WithNamespace(namespace).
		Build()

	if err != nil {
		return nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, err
	}

	if p.fallbackForContainerMetrics && len(stackdriverResponse.TimeSeries) == 0 {
		stackdriverRequest, err := queryBuilder.
			AsContainerType().
			WithPods(pods).
			WithMetricKind(metricKind).
			WithMetricValueType(metricValueType).
			WithMetricSelector(metricSelector).
			WithNamespace(namespace).
			Build()
		if err != nil {
			return nil, err
		}
		stackdriverResponse, err = stackdriverRequest.Do()
		if err != nil {
			return nil, err
		}

		err = p.translator.CheckMetricUniquenessForPod(stackdriverResponse, escapedMetricName)
		if err != nil {
			return nil, err
		}
	}
	return p.translator.GetRespForSingleObject(stackdriverResponse, groupResource, escapedMetricName, metricSelector, namespace, name)
}

// GetNamespacedMetricBySelector queries Stackdriver for metrics identified by selector and associated
// with a namespace.
func (p *StackdriverProvider) getNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, labelSelector labels.Selector, escapedMetricName string, metricSelector labels.Selector) (*custom_metrics.MetricValueList, error) {
	if groupResource.Resource != podResource {
		return nil, NewOperationNotSupportedError(fmt.Sprintf("Get namespaced metric by selector for resource %q", groupResource.Resource))
	}
	matchingPods, err := p.kubeClient.Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}
	metricName := getCustomMetricName(escapedMetricName)
	klog.V(4).Infof("Querying for metric: %s", metricName)

	metricKind, metricValueType, err := p.translator.GetMetricKind(metricName, metricSelector)
	if err != nil {
		return nil, err
	}

	queryBuilder := translator.NewQueryBuilder(p.translator, metricName).
		WithMetricKind(metricKind).
		WithMetricSelector(metricSelector).
		WithMetricValueType(metricValueType).
		WithNamespace(namespace)

	result := []custom_metrics.MetricValue{}
	for i := 0; i < len(matchingPods.Items); i += translator.MaxNumOfArgsInOneOfFilter {
		sliceSegmentEnd := min(i+translator.MaxNumOfArgsInOneOfFilter, len(matchingPods.Items))
		podsSlice := &v1.PodList{Items: matchingPods.Items[i:sliceSegmentEnd]}
		stackdriverRequest, err := queryBuilder.WithPods(podsSlice).Build()
		if err != nil {
			return nil, err
		}
		stackdriverResponse, err := stackdriverRequest.Do()
		if err != nil {
			return nil, err
		}
		slice, err := p.translator.GetRespForMultipleObjects(stackdriverResponse, p.translator.GetPodItems(matchingPods), groupResource, escapedMetricName, metricSelector)
		if err != nil {
			return nil, err
		}
		result = append(result, slice...)
	}

	if p.fallbackForContainerMetrics && len(result) == 0 {
		for i := 0; i < len(matchingPods.Items); i += translator.MaxNumOfArgsInOneOfFilter {
			sliceSegmentEnd := min(i+translator.MaxNumOfArgsInOneOfFilter, len(matchingPods.Items))
			podsSlice := &v1.PodList{Items: matchingPods.Items[i:sliceSegmentEnd]}
			stackdriverRequest, err := translator.NewQueryBuilder(p.translator, metricName).
				AsContainerType().
				WithPods(podsSlice).
				WithMetricKind(metricKind).
				WithMetricValueType(metricValueType).
				WithMetricSelector(metricSelector).
				WithNamespace(namespace).
				Build()
			if err != nil {
				return nil, err
			}
			stackdriverResponse, err := stackdriverRequest.Do()
			if err != nil {
				return nil, err
			}
			err = p.translator.CheckMetricUniquenessForPod(stackdriverResponse, escapedMetricName)
			if err != nil {
				return nil, err
			}
			slice, err := p.translator.GetRespForMultipleObjects(stackdriverResponse, p.translator.GetPodItems(matchingPods), groupResource, escapedMetricName, metricSelector)
			if err != nil {
				return nil, err
			}
			result = append(result, slice...)
		}
	}

	return &custom_metrics.MetricValueList{Items: result}, nil
}

// ListAllMetrics returns one custom metric to reduce memory usage, when  ListFullCustomMetrics is false (by default),
// Else, it returns all custom metrics available from Stackdriver.
func (p *StackdriverProvider) ListAllMetrics() []provider.CustomMetricInfo {
	return p.metricsCache
}

// GetExternalMetric queries Stackdriver for external metrics.
func (p *StackdriverProvider) GetExternalMetric(ctx context.Context, namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	if p.externalMetricsCache != nil {
		key := cacheKey{
			namespace:      namespace,
			metricSelector: metricSelector.String(),
			info:           info,
		}
		if cachedValue, ok := p.externalMetricsCache.get(key); ok {
			return cachedValue, nil
		}
	}

	// Proceed to do a fresh fetch for metrics since one of the following is true at this point
	// a) externalMetricCache is disabled
	// b) the key was never added to the cache
	// c) the key was in the cache, but its corrupt or its TTL has expired
	metricNameEscaped := info.Metric
	metricName := getExternalMetricName(metricNameEscaped)
	metricKind, metricValueType, err := p.translator.GetMetricKind(metricName, metricSelector)
	if err != nil {
		return nil, err
	}
	stackdriverRequest, err := p.translator.GetExternalMetricRequest(metricName, metricKind, metricValueType, metricSelector)
	if err != nil {
		return nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, err
	}
	externalMetricItems, err := p.translator.GetRespForExternalMetric(stackdriverResponse, metricNameEscaped)
	if err != nil {
		return nil, err
	}

	resp := &external_metrics.ExternalMetricValueList{
		Items: externalMetricItems,
	}

	if p.externalMetricsCache != nil {
		key := cacheKey{
			namespace:      namespace,
			metricSelector: metricSelector.String(),
			info:           info,
		}
		p.externalMetricsCache.add(key, resp)
	}

	return resp, nil
}

// ListAllExternalMetrics returns a list of available external metrics.
// Not implemented (currently returns a list of one fake metric).
func (p *StackdriverProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	return []provider.ExternalMetricInfo{
		{
			Metric: "externalmetrics",
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getExternalMetricName(metricName string) string {
	return strings.Replace(metricName, "|", "/", -1)
}

func getCustomMetricName(metricName string) string {
	if strings.Contains(metricName, "|") {
		return getExternalMetricName(metricName)
	}
	return "custom.googleapis.com/" + metricName
}
