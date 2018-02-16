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

	"github.com/golang/glog"
	stackdriver "google.golang.org/api/monitoring/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
	"k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
)

// TODO(kawych):
// * Support metrics for objects other than pod, e.i. root-scoped - depends on SD resource types.
// * Support long responses from Stackdriver (pagination).

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (c realClock) Now() time.Time {
	return time.Now()
}

// StackdriverProvider is a provider of custom metrics from Stackdriver.
type StackdriverProvider struct {
	kubeClient          *corev1.CoreV1Client
	stackdriverService  *stackdriver.Service
	config              *config.GceConfig
	rateInterval        time.Duration
	translator          *Translator
	useNewResourceModel bool
}

// NewStackdriverProvider creates a StackdriverProvider
func NewStackdriverProvider(kubeClient *corev1.CoreV1Client, mapper apimeta.RESTMapper, stackdriverService *stackdriver.Service, rateInterval time.Duration, useNewResourceModel bool) provider.CustomMetricsProvider {
	gceConf, err := config.GetGceConfig("custom.googleapis.com")
	if err != nil {
		glog.Fatalf("Failed to retrieve GCE config: %v", err)
	}

	return &StackdriverProvider{
		kubeClient:         kubeClient,
		stackdriverService: stackdriverService,
		config:             gceConf,
		rateInterval:       rateInterval,
		translator: &Translator{
			service:             stackdriverService,
			config:              gceConf,
			reqWindow:           rateInterval,
			clock:               realClock{},
			mapper:              mapper,
			useNewResourceModel: useNewResourceModel,
		},
	}
}

// GetRootScopedMetricByName queries Stackdriver for metrics identified by name and not associated
// with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, provider.NewOperationNotSupportedError("Get root scoped metric by name")
}

// GetRootScopedMetricBySelector queries Stackdriver for metrics identified by selector and not
// associated with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, provider.NewOperationNotSupportedError("Get root scoped metric by selector")
}

// GetNamespacedMetricByName queries Stackdriver for metrics identified by name and associated
// with a namespace.
func (p *StackdriverProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	matchingPod, err := p.kubeClient.Pods(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	stackdriverRequest, err := p.translator.GetSDReqForPods(&v1.PodList{Items: []v1.Pod{*matchingPod}}, metricName, namespace)
	if err != nil {
		return nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, err
	}
	return p.translator.GetRespForPod(stackdriverResponse, groupResource, metricName, namespace, name)
}

// GetNamespacedMetricBySelector queries Stackdriver for metrics identified by selector and associated
// with a namespace.
func (p *StackdriverProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	matchingPods, err := p.kubeClient.Pods(namespace).List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := []custom_metrics.MetricValue{}
	for i := 0; i < len(matchingPods.Items); i += 100 {
		stackdriverRequest, err := p.translator.GetSDReqForPods(&v1.PodList{Items: matchingPods.Items[i:min(i+100, len(matchingPods.Items))]}, metricName, namespace)
		if err != nil {
			return nil, err
		}
		stackdriverResponse, err := stackdriverRequest.Do()
		if err != nil {
			return nil, err
		}
		slice, err := p.translator.GetRespForPods(stackdriverResponse, matchingPods, groupResource, metricName, namespace)
		if err != nil {
			return nil, err
		}
		result = append(result, slice...)
	}
	return &custom_metrics.MetricValueList{Items: result}, nil
}

// ListAllMetrics returns all custom metrics available from Stackdriver.
// List only pod metrics
func (p *StackdriverProvider) ListAllMetrics() []provider.MetricInfo {
	metrics := []provider.MetricInfo{}
	stackdriverRequest := p.translator.ListMetricDescriptors()
	response, err := stackdriverRequest.Do()
	if err != nil {
		glog.Errorf("Failed request to stackdriver api: %s", err)
		return metrics
	}
	return p.translator.GetMetricsFromSDDescriptorsResp(response)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
