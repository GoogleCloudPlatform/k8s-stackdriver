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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/metrics"
	"sigs.k8s.io/metrics-server/pkg/api"
)

// CoreProvider is a provider of core metrics.
type CoreProvider struct{}

// Check if CoreProvider implements api.MetricsGetter interface.
var _ api.MetricsGetter = &CoreProvider{}

// NewCoreProvider creates a CoreProvider.
func NewCoreProvider() *CoreProvider {
	return &CoreProvider{}
}

// GetContainerMetrics implements the api.MetricsProvider interface.
func (p *CoreProvider) GetContainerMetrics(pods ...apitypes.NamespacedName) ([]api.TimeInfo, [][]metrics.ContainerMetrics, error) {
	resTimes := make([]api.TimeInfo, len(pods))
	resMetrics := make([][]metrics.ContainerMetrics, len(pods))

	for i := range pods {
		// TODO(holubowicz): return real values
		resMetrics[i] = []metrics.ContainerMetrics{
			{Name: "cont1", Usage: buildResList(1000.0, 1024*1024*1024)},
			{Name: "cont2", Usage: buildResList(1500.0, 1024*1024*1024)},
		}
		resTimes[i] = api.TimeInfo{Timestamp: time.Now(), Window: 1 * time.Minute}
	}

	return resTimes, resMetrics, nil
}

// GetNodeMetrics implements the api.MetricsProvider interface.
func (p *CoreProvider) GetNodeMetrics(nodes ...string) ([]api.TimeInfo, []corev1.ResourceList, error) {
	resTimes := make([]api.TimeInfo, len(nodes))
	resMetrics := make([]corev1.ResourceList, len(nodes))

	for i := range nodes {
		// TODO(holubowicz): return real values
		resMetrics[i] = buildResList(0.05, 42*1024*1024)
		resTimes[i] = api.TimeInfo{Timestamp: time.Now(), Window: 1 * time.Minute}
	}

	return resTimes, resMetrics, nil
}

func buildResList(cpu, memory float64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(cpu*1000.0), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewMilliQuantity(int64(memory*1000.0), resource.BinarySI),
	}
}
