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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// GetPodMetrics implements the api.MetricsProvider interface. It translate data from getPodMetrics to the new api.
func (p *CoreProvider) GetPodMetrics(pods ...*metav1.PartialObjectMetadata) ([]metrics.PodMetrics, error) {
	resMetrics := make([]metrics.PodMetrics, 0, len(pods))

	if len(pods) == 0 {
		return resMetrics, nil
	}

	podNames := []apitypes.NamespacedName{}
	for _, pod := range pods {
		podNames = append(podNames, apitypes.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		})
	}

	timeInfos, containerMetrics, err := p.getPodMetrics(podNames...)
	if err != nil {
		return []metrics.PodMetrics{}, err
	}

	for i, pod := range pods {
		podMetric := metrics.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              pod.Name,
				Namespace:         pod.Namespace,
				Labels:            pod.Labels,
				CreationTimestamp: metav1.Now(),
			},
			Timestamp: metav1.NewTime(timeInfos[i].Timestamp),
			Window:    metav1.Duration{Duration: timeInfos[i].Window},
		}

		podMetric.Containers = containerMetrics[i]

		resMetrics = append(resMetrics, podMetric)
	}

	return resMetrics, nil
}

// GetNodeMetrics implements the api.MetricsProvider interface. It translate data from getNodeMetrics to the new api.
func (p *CoreProvider) GetNodeMetrics(nodes ...*corev1.Node) ([]metrics.NodeMetrics, error) {
	resMetrics := make([]metrics.NodeMetrics, 0, len(nodes))
	if len(nodes) == 0 {
		return resMetrics, nil
	}

	nodeNames := []string{}
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	timeInfos, resourceList, err := p.getNodeMetrics(nodeNames...)
	if err != nil {
		return []metrics.NodeMetrics{}, err
	}

	for i, node := range nodes {
		resMetrics = append(resMetrics, metrics.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              node.Name,
				Labels:            node.Labels,
				CreationTimestamp: metav1.Now(),
			},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resourceList[i][corev1.ResourceCPU],
				corev1.ResourceMemory: resourceList[i][corev1.ResourceMemory],
			},
			Timestamp: metav1.NewTime(timeInfos[i].Timestamp),
			Window:    metav1.Duration{Duration: timeInfos[i].Window},
		})
	}
	return resMetrics, nil
}

//	Wrapping this method with the new GetPodMetrics for easily re-using the old unit test and a quick merge for the vulnerability fix. In the long run it's still better to directly updating this with the new API without another layer of wrap.
//
// If metrics from i-th pod are not present, ContainerMetrics[i] will be nil and TimeInfo[i] will be default TimeInfo value.
func (p *CoreProvider) getPodMetrics(pods ...apitypes.NamespacedName) ([]api.TimeInfo, [][]metrics.ContainerMetrics, error) {
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

//	Wrapping this method with the new GetPodMetrics for easily re-using the old unit test and a quick merge for the vulnerability fix. In the long run it's still better to directly updating this with the new API without another layer of wrap.
//
// If metrics from i-th node are not present, ResourceList[i] will be nil and TimeInfo[i] will be default TimeInfo value.
func (p *CoreProvider) getNodeMetrics(nodes ...string) ([]api.TimeInfo, []corev1.ResourceList, error) {
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

// GetPodMetricsOldAPI use old parameter lists. Making it public for testing purpose.
// We're wrapping this method with the new GetPodMetrics for easily re-using the old unit test and a quick merge for the vulnerability fix.
// In the long run it's still better to directly updating this with the new API without another layer of wrap.
func (p *CoreProvider) GetPodMetricsOldAPI(pods ...apitypes.NamespacedName) ([]api.TimeInfo, [][]metrics.ContainerMetrics, error) {
	return p.getPodMetrics(pods...)
}

// GetNodeMetricsOldAPI use old parameter lists. Making it public for testing purpose.
func (p *CoreProvider) GetNodeMetricsOldAPI(nodes ...string) ([]api.TimeInfo, []corev1.ResourceList, error) {
	return p.getNodeMetrics(nodes...)
}
