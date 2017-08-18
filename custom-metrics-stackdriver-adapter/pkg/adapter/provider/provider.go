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
	"time"

	"github.com/golang/glog"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	// Install registers the API group and adds types to a scheme.
	_ "k8s.io/client-go/pkg/api/install"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
)

// TODO(kawych):
// * Support metrics for pods identified by selector - in progress.
// * Use discovery REST mapper instead of api.Registry
// * Implement ListAllMetrics - in progress.
// * Support metrics for objects other than pod, e.i. root-scoped - depends on SD resource types.
// * Support long responses from Stackdriver (pagination).
// * Return Kubernetes API - compatible errors defined in
// "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider/errors"

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (c realClock) Now() time.Time {
	return time.Now()
}

// StackdriverProvider is a provider of custom metrics from Stackdriver.
type StackdriverProvider struct {
	apiserverClient    rest.Interface
	stackdriverService *stackdriver.Service
	config             *config.GceConfig
	rateInterval       time.Duration
	translator         *Translator
}

// NewStackdriverProvider creates a StackdriverProvider
func NewStackdriverProvider(apiserverClient rest.Interface, stackdriverService *stackdriver.Service, rateInterval time.Duration) provider.CustomMetricsProvider {
	gceConf, err := config.GetGceConfig("custom.googleapis.com")
	if err != nil {
		glog.Fatalf("Failed to retrieve GCE config: %v", err)
	}

	return &StackdriverProvider{
		apiserverClient:    apiserverClient,
		stackdriverService: stackdriverService,
		config:             gceConf,
		rateInterval:       rateInterval,
		translator: &Translator{
			service:   stackdriverService,
			config:    gceConf,
			reqWindow: rateInterval,
			clock:     realClock{},
		},
	}
}

// GetRootScopedMetricByName queries Stackdriver for metrics identified by name and not associated
// with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("Non namespaced metrics not supported (metric name: %s)", metricName)
}

// GetRootScopedMetricBySelector queries Stackdriver for metrics identified by selector and not
// associated with any namespace. Current implementation doesn't support root scoped metrics.
func (p *StackdriverProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("Non namespaced metrics not supported (metric name: %s)", metricName)
}

// GetNamespacedMetricByName queries Stackdriver for metrics identified by name and associated
// with a namespace.
func (p *StackdriverProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	matchingObject, err := p.apiserverClient.Get().Namespace(namespace).Resource(groupResource.Resource).Name(name).Do().Get()
	if err != nil {
		return nil, err
	}
	stackdriverRequest, err := p.translator.GetSDReqForPod(matchingObject, metricName)
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
// TODO(kawych): implement this
func (p *StackdriverProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("Selectors not supported")
}

// ListAllMetrics returns all custom metrics available from Stackdriver.
// TODO(kawych): implement this
func (p *StackdriverProvider) ListAllMetrics() []provider.MetricInfo {
	return []provider.MetricInfo{}
}
