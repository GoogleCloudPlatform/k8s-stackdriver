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

package coreprovider

import (
	"fmt"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"k8s.io/metrics/pkg/apis/metrics"
	"sigs.k8s.io/metrics-server/pkg/api"
)

// CoreProvider is a provider of core metrics.
type CoreProvider struct {
	client coreClientInterface
}

// Check if CoreProvider implements api.MetricsGetter interface.
var _ api.MetricsGetter = &CoreProvider{}

// NewCoreProvider creates a CoreProvider
func NewCoreProvider(translator *translator.Translator) *CoreProvider {
	return &CoreProvider{client: newClient(translator)}
}

type coreClientInterface interface {
	getContainerCPU(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error)
	getContainerRAM(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error)
	getNodeCPU(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error)
	getNodeRAM(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error)
}

// GetContainerMetrics implements the api.MetricsProvider interface.
// If metrics from i-th pod are not present, ContainerMetrics[i] will be nil and TimeInfo[i] will be default TimeInfo value.
func (p *CoreProvider) GetContainerMetrics(pods ...apitypes.NamespacedName) ([]api.TimeInfo, [][]metrics.ContainerMetrics, error) {
	timeInfo := make([]api.TimeInfo, len(pods))
	coreMetrics := make([][]metrics.ContainerMetrics, len(pods))
	resourceNames := make([]string, len(pods))
	for i, pod := range pods {
		resourceNames[i] = fmt.Sprintf("%q", pod.Name)
	}

	cpuMetrics, cpuTimeInfo, err := p.client.getContainerCPU(resourceNames)
	if err != nil {
		return nil, nil, err
	}
	ramMetrics, _, err := p.client.getContainerRAM(resourceNames)
	if err != nil {
		return nil, nil, err
	}

	for i := range pods {
		podKey := pods[i].Namespace + ":" + pods[i].Name

		cpuContainers, ok := cpuMetrics[podKey]
		if !ok {
			klog.V(4).Infof("Metric cpu not found for pod '%s'", podKey)
			continue
		}
		ramContainers, ok := ramMetrics[podKey]
		if !ok {
			klog.V(4).Infof("Metric ram not found for pod '%s'", podKey)
			continue
		}

		coreMetrics[i] = make([]metrics.ContainerMetrics, 0)
		for container, cpu := range cpuContainers {
			ram, ok := ramContainers[container]
			if !ok { // cpu and ram should be present in the container
				continue
			}

			coreMetrics[i] = append(coreMetrics[i], metrics.ContainerMetrics{Name: container, Usage: corev1.ResourceList{
				corev1.ResourceCPU:    cpu,
				corev1.ResourceMemory: ram,
			}})

			timeInfo[i], ok = cpuTimeInfo[podKey] // TODO(holubowicz): query about the same time segment in cpu and ram (now it can be slightly different)
			if !ok {
				return nil, nil, apierr.NewInternalError(fmt.Errorf("TimeInfo should be set for every pod with metrics"))
			}
		}
		if len(coreMetrics[i]) == 0 {
			coreMetrics[i] = nil
		}
	}

	return timeInfo, coreMetrics, nil
}

// GetNodeMetrics implements the api.MetricsProvider interface.
// If metrics from i-th node are not present, ResourceList[i] will be nil and TimeInfo[i] will be default TimeInfo value.
func (p *CoreProvider) GetNodeMetrics(nodes ...string) ([]api.TimeInfo, []corev1.ResourceList, error) {
	timeInfo := make([]api.TimeInfo, len(nodes))
	coreMetrics := make([]corev1.ResourceList, len(nodes))
	resourceNames := make([]string, len(nodes))

	for i := range nodes {
		resourceNames[i] = fmt.Sprintf("%q", nodes[i])
	}

	cpuMetrics, cpuTimeInfo, err := p.client.getNodeCPU(nodes)
	if err != nil {
		return nil, nil, err
	}
	ramMetrics, _, err := p.client.getNodeRAM(nodes)
	if err != nil {
		return nil, nil, err
	}

	for i, name := range nodes {
		cpu, ok := cpuMetrics[name]
		if !ok {
			klog.V(4).Infof("Metric cpu not found for node '%s'", name)
			continue
		}
		ram, ok := ramMetrics[name]
		if !ok {
			klog.V(4).Infof("Metric ram not found for node '%s'", name)
			continue
		}
		coreMetrics[i] = corev1.ResourceList{
			corev1.ResourceCPU:    cpu,
			corev1.ResourceMemory: ram,
		}
		timeInfo[i] = cpuTimeInfo[name]
		if !ok {
			return nil, nil, apierr.NewInternalError(fmt.Errorf("TimeInfo should be set for every node with metrics"))
		}
	}
	return timeInfo, coreMetrics, nil
}
