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
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/metrics"
	"sigs.k8s.io/metrics-server/pkg/api"
)

type fakeCoreClient struct {
	contArg     []string
	contCpuRes1 map[string]map[string]resource.Quantity
	contCpuRes2 map[string]api.TimeInfo
	contRamRes1 map[string]map[string]resource.Quantity
	contRamRes2 map[string]api.TimeInfo
	nodeArg     []string
	nodeCpuRes1 map[string]resource.Quantity
	nodeCpuRes2 map[string]api.TimeInfo
	nodeRamRes1 map[string]resource.Quantity
	nodeRamRes2 map[string]api.TimeInfo
	t           *testing.T
}

func (c *fakeCoreClient) getContainerCPU(resourceNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	if !reflect.DeepEqual(resourceNames, c.contArg) {
		c.t.Errorf("Unexpected argument. Expected: \n%v,\n received: \n%v", c.contArg, resourceNames)
	}
	return c.contCpuRes1, c.contCpuRes2, nil
}
func (c *fakeCoreClient) getContainerRAM(resourceNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	if !reflect.DeepEqual(resourceNames, c.contArg) {
		c.t.Errorf("Unexpected argument. Expected: \n%v,\n received: \n%v", c.contArg, resourceNames)
	}
	return c.contRamRes1, c.contRamRes2, nil
}
func (c *fakeCoreClient) getNodeCPU(resourceNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	if !reflect.DeepEqual(resourceNames, c.nodeArg) {
		c.t.Errorf("Unexpected argument. Expected: \n%v,\n received: \n%v", c.nodeArg, resourceNames)
	}
	return c.nodeCpuRes1, c.nodeCpuRes2, nil
}
func (c *fakeCoreClient) getNodeRAM(resourceNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	if !reflect.DeepEqual(resourceNames, c.nodeArg) {
		c.t.Errorf("Unexpected argument. Expected: \n%v,\n received: \n%v", c.nodeArg, resourceNames)
	}
	return c.nodeRamRes1, c.nodeRamRes2, nil
}

func podClient(pods []string, contCpuRes1 map[string]map[string]resource.Quantity, contCpuRes2 map[string]api.TimeInfo, contRamRes1 map[string]map[string]resource.Quantity, contRamRes2 map[string]api.TimeInfo, t *testing.T) *fakeCoreClient {
	return &fakeCoreClient{pods, contCpuRes1, contCpuRes2, contRamRes1, contRamRes2, nil, nil, nil, nil, nil, t}
}

func nodeClient(nodes []string, nodeCpuRes1 map[string]resource.Quantity, nodeCpuRes2 map[string]api.TimeInfo, nodeRamRes1 map[string]resource.Quantity, nodeRamRes2 map[string]api.TimeInfo, t *testing.T) *fakeCoreClient {
	return &fakeCoreClient{nil, nil, nil, nil, nil, nodes, nodeCpuRes1, nodeCpuRes2, nodeRamRes1, nodeRamRes2, t}
}

func TestCoreprovider_GetPodMetricsOldAPI_Single(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace1", "pod1"}}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(1000)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromInt(1000)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
	}

	var provider = CoreProvider{podClient([]string{doubleQuote("pod1")}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}
	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}

	expectedMetric := []metrics.ContainerMetrics{{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod1"]["cont1"], "memory": contRamRes["namespace1:pod1"]["cont1"]}}}
	if !reflect.DeepEqual(metric[0], expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metric[0])
	}

	if !reflect.DeepEqual(timeInfo[0], timeRes["namespace1:pod1"]) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", time1, timeInfo[0])
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_Many(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace2", "pod1"}, {"namespace1", "pod1"}, {"namespace1", "pod2"}}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	time2, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:10Z")
	time3, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:20Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(0.1), "cont2": fromDouble(0.2)},
		"namespace1:pod2": {"cont1": fromDouble(0.3), "cont2": fromDouble(0.4)},
		"namespace2:pod1": {"cont1": fromDouble(0.5), "cont2": fromDouble(0.6)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromInt(1000), "cont2": fromInt(1001)},
		"namespace1:pod2": {"cont1": fromInt(1002), "cont2": fromInt(1003)},
		"namespace2:pod1": {"cont1": fromInt(1004), "cont2": fromInt(1005)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
		"namespace1:pod2": {time2, time.Minute},
		"namespace2:pod1": {time3, time.Minute},
	}
	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name), doubleQuote(pods[1].Name), doubleQuote(pods[2].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedMetrics := [][]metrics.ContainerMetrics{
		{{"cont2", corev1.ResourceList{"cpu": contCpuRes["namespace2:pod1"]["cont2"], "memory": contRamRes["namespace2:pod1"]["cont2"]}},
			{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace2:pod1"]["cont1"], "memory": contRamRes["namespace2:pod1"]["cont1"]}}},
		{{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod1"]["cont1"], "memory": contRamRes["namespace1:pod1"]["cont1"]}},
			{"cont2", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod1"]["cont2"], "memory": contRamRes["namespace1:pod1"]["cont2"]}}},
		{{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod2"]["cont1"], "memory": contRamRes["namespace1:pod2"]["cont1"]}},
			{"cont2", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod2"]["cont2"], "memory": contRamRes["namespace1:pod2"]["cont2"]}}},
	}
	timeExpected := []api.TimeInfo{
		{time3, time.Minute},
		{time1, time.Minute},
		{time2, time.Minute},
	}

	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}
	for i := range pods {
		if !reflect.DeepEqual(metric[i], expectedMetrics[i]) && !reflect.DeepEqual(metric[i], []metrics.ContainerMetrics{expectedMetrics[i][1], expectedMetrics[i][0]}) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metric[i])
		}

		if !reflect.DeepEqual(timeInfo[i], timeExpected[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", timeExpected[i], timeInfo[i])
		}
	}
}
func TestCoreprovider_GetPodMetricsOldAPI_MissingContainer(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace1", "pod1"}}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(0.1), "cont3": fromDouble(0.3)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromInt(11), "cont2": fromInt(12)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
	}
	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedMetrics := [][]metrics.ContainerMetrics{
		{{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace1:pod1"]["cont1"], "memory": contRamRes["namespace1:pod1"]["cont1"]}}},
	}
	timeExpected := []api.TimeInfo{
		{time1, time.Minute},
	}
	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}

	for i := range pods {
		if !reflect.DeepEqual(metric[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metric[i])
		}

		if !reflect.DeepEqual(timeInfo[i], timeExpected[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", timeExpected[i], timeInfo[i])
		}
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_MissingPodInfo(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace1", "pod1"}, {"namespace1", "pod2"}, {"namespace2", "pod1"}}

	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	time2, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:10Z")
	time3, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:20Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(0.1)},
		"namespace2:pod1": {"cont1": fromDouble(0.5)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod2": {"cont1": fromInt(1002)},
		"namespace2:pod1": {"cont1": fromInt(1004)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
		"namespace1:pod2": {time2, time.Minute},
		"namespace2:pod1": {time3, time.Minute},
	}
	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name), doubleQuote(pods[1].Name), doubleQuote(pods[2].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedMetrics := [][]metrics.ContainerMetrics{
		nil,
		nil,
		{{"cont1", corev1.ResourceList{"cpu": contCpuRes["namespace2:pod1"]["cont1"], "memory": contRamRes["namespace2:pod1"]["cont1"]}}},
	}
	timeExpected := []api.TimeInfo{
		{time1, time.Minute},
		{time2, time.Minute},
		{time3, time.Minute},
	}

	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}

	for i := range pods {
		if !reflect.DeepEqual(metric[i], expectedMetrics[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics[i], metric[i])
		}

		if expectedMetrics[i] != nil && !reflect.DeepEqual(timeInfo[i], timeExpected[i]) {
			t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", timeExpected[i], timeInfo[i])
		}
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_MissingPod(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace1", "pod1"}}

	contCpuRes := map[string]map[string]resource.Quantity{}
	contRamRes := map[string]map[string]resource.Quantity{}
	timeRes := map[string]api.TimeInfo{}

	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedMetrics := [][]metrics.ContainerMetrics{
		nil,
	}
	if !reflect.DeepEqual(metric, expectedMetrics) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics, metric)
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_EmptyResult(t *testing.T) {
	pods := []apitypes.NamespacedName{{"namespace1", "pod1"}}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont3": fromDouble(0.3)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont2": fromInt(12)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
	}
	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedMetrics := [][]metrics.ContainerMetrics{
		nil,
	}

	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}

	if !reflect.DeepEqual(metric, expectedMetrics) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics, metric)
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_Empty(t *testing.T) {
	pods := []apitypes.NamespacedName{}

	contCpuRes := map[string]map[string]resource.Quantity{}
	contRamRes := map[string]map[string]resource.Quantity{}
	timeRes := map[string]api.TimeInfo{}

	var provider = CoreProvider{podClient([]string{}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}
}

func TestCoreprovider_GetPodMetricsOldAPI_AdditionalInfo(t *testing.T) {
	var pods []apitypes.NamespacedName
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(1000)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromInt(1000)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time1, time.Minute},
	}

	var provider = CoreProvider{podClient([]string{}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetPodMetricsOldAPI(pods...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}
	if len(pods) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(metric))
	}
	if len(pods) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(pods), len(timeInfo))
	}
}

func TestCoreprovider_GetNodeMetricsOldAPI_Single(t *testing.T) {
	nodes := []string{"node1"}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	nodeCpuRes := map[string]resource.Quantity{
		"node1": fromDouble(1000),
	}
	nodeRamRes := map[string]resource.Quantity{
		"node1": fromInt(1000),
	}
	timeRes := map[string]api.TimeInfo{
		"node1": {time1, time.Minute},
	}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes, timeRes, nodeRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetNodeMetricsOldAPI(nodes...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	if len(nodes) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	expectedMetric := []corev1.ResourceList{{"cpu": nodeCpuRes["node1"], "memory": nodeRamRes["node1"]}}
	if !reflect.DeepEqual(metric, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metric)
	}

	if !reflect.DeepEqual(timeInfo[0], timeRes["node1"]) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", timeInfo[0], timeRes["node1"])
	}
}
func TestCoreprovider_GetNodeMetricsOldAPI_Many(t *testing.T) {
	nodes := []string{"node3", "node1", "node2"}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	time2, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:10Z")
	time3, _ := time.Parse(time.RFC3339, "2018-01-02T13:01:20Z")

	nodeCpuRes := map[string]resource.Quantity{
		"node1": fromDouble(1000),
		"node2": fromDouble(1001),
		"node3": fromDouble(1002),
	}
	nodeRamRes := map[string]resource.Quantity{
		"node1": fromInt(10000),
		"node2": fromInt(100000),
		"node3": fromInt(1000000),
	}
	timeRes := map[string]api.TimeInfo{
		"node1": {time1, time.Minute},
		"node2": {time2, time.Minute},
		"node3": {time3, time.Minute},
	}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes, timeRes, nodeRamRes, timeRes, t)}
	timeInfo, metric, err := provider.GetNodeMetricsOldAPI(nodes...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	if len(nodes) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	expectedMetric := []corev1.ResourceList{
		{"cpu": nodeCpuRes["node3"], "memory": nodeRamRes["node3"]},
		{"cpu": nodeCpuRes["node1"], "memory": nodeRamRes["node1"]},
		{"cpu": nodeCpuRes["node2"], "memory": nodeRamRes["node2"]}}
	expectedTime := []api.TimeInfo{
		{time3, time.Minute},
		{time1, time.Minute},
		{time2, time.Minute},
	}

	if !reflect.DeepEqual(metric, expectedMetric) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetric, metric)
	}

	if !reflect.DeepEqual(timeInfo, expectedTime) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedTime, timeInfo)
	}
}

func TestCoreprovider_GetNodeMetricsOldAPI_MissingInfo(t *testing.T) {
	nodes := []string{"node1"}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")

	nodeCpuRes1 := map[string]resource.Quantity{
		"node1": fromDouble(1000),
	}
	nodeRamRes1 := map[string]resource.Quantity{
		"node2": fromInt(10001),
	}
	timeRes := map[string]api.TimeInfo{
		"node1": {time1, time.Minute},
		"node2": {time1, time.Minute},
	}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes1, timeRes, nodeRamRes1, timeRes, t)}
	timeInfo, metric, err := provider.GetNodeMetricsOldAPI(nodes...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	if len(nodes) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	expectedMetrics := []corev1.ResourceList{nil}
	if !reflect.DeepEqual(metric, expectedMetrics) {
		t.Errorf("Unexpected result. Expected: \n%v,\n received: \n%v", expectedMetrics, metric)
	}
}

func TestCoreprovider_GetNodeMetricsOldAPI_Empty(t *testing.T) {
	nodes := []string{}
	nodeCpuRes1 := map[string]resource.Quantity{}
	nodeRamRes1 := map[string]resource.Quantity{}
	timeRes := map[string]api.TimeInfo{}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes1, timeRes, nodeRamRes1, timeRes, t)}
	timeInfo, metric, err := provider.GetNodeMetricsOldAPI(nodes...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	if len(nodes) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
}

func TestCoreprovider_GetNodeMetricsOldAPI_AdditionalInfo(t *testing.T) {
	nodes := []string{}
	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	nodeCpuRes1 := map[string]resource.Quantity{
		"node1": fromDouble(1000),
	}
	nodeRamRes1 := map[string]resource.Quantity{
		"node1": fromInt(10001),
	}
	timeRes := map[string]api.TimeInfo{
		"node1": {time1, time.Minute},
	}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes1, timeRes, nodeRamRes1, timeRes, t)}
	timeInfo, metric, err := provider.GetNodeMetricsOldAPI(nodes...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(metric) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
	if len(nodes) != len(timeInfo) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(timeInfo))
	}
}

func fromInt(value int64) resource.Quantity {
	return *resource.NewQuantity(value, resource.DecimalSI)
}
func fromDouble(value float64) resource.Quantity {
	return *resource.NewScaledQuantity(int64(value*1000*1000), -6)
}

func doubleQuote(s string) string {
	return fmt.Sprintf("%q", s)
}

func TestCoreprovider_GetNodeMetrics_Many(t *testing.T) {
	nodeObjects := []*corev1.Node{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
	}
	nodes := []string{}
	for _, n := range nodeObjects {
		nodes = append(nodes, n.Name)
	}

	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	time2, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:10Z")
	time3, _ := time.Parse(time.RFC3339, "2018-01-02T13:01:20Z")

	nodeCpuRes := map[string]resource.Quantity{
		"node1": fromDouble(1000),
		"node2": fromDouble(1001),
		"node3": fromDouble(1002),
	}
	nodeRamRes := map[string]resource.Quantity{
		"node1": fromInt(10000),
		"node2": fromInt(100000),
		"node3": fromInt(1000000),
	}
	timeRes := map[string]api.TimeInfo{
		"node1": {time1, time.Minute},
		"node2": {time2, time.Minute},
		"node3": {time3, time.Minute},
	}
	var provider = CoreProvider{nodeClient(nodes, nodeCpuRes, timeRes, nodeRamRes, timeRes, t)}
	nodeMetrics, err := provider.GetNodeMetrics(nodeObjects...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	if len(nodes) != len(nodeMetrics) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(nodes), len(nodeMetrics))
	}

	expectedNodeMetrics := []metrics.NodeMetrics{
		metrics.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node3",
				CreationTimestamp: metav1.Now(),
			},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    fromDouble(1002),
				corev1.ResourceMemory: fromInt(1000000),
			},
			Timestamp: metav1.NewTime(time3),
			Window:    metav1.Duration{time.Minute},
		},
		metrics.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node2",
				CreationTimestamp: metav1.Now(),
			},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    fromDouble(1001),
				corev1.ResourceMemory: fromInt(100000),
			},
			Timestamp: metav1.NewTime(time2),
			Window:    metav1.Duration{time.Minute},
		},
		metrics.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node1",
				CreationTimestamp: metav1.Now(),
			},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    fromDouble(1000),
				corev1.ResourceMemory: fromInt(10000),
			},
			Timestamp: metav1.NewTime(time1),
			Window:    metav1.Duration{time.Minute},
		},
	}

	if diff := cmp.Diff(expectedNodeMetrics, nodeMetrics, cmpopts.SortMaps(func(a, b string) bool { return a < b }), cmpopts.IgnoreFields(metrics.NodeMetrics{}, "ObjectMeta", "CreationTimestamp")); diff != "" {
		t.Errorf("Has a diff, (-want, +got): %s", diff)
	}

}

func TestCoreprovider_GetPodMetrics_Many(t *testing.T) {
	podObjects := []*metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace1", Name: "pod1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace2", Name: "pod2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace3", Name: "pod3"}},
	}

	pods := []apitypes.NamespacedName{}
	for _, p := range podObjects {
		pods = append(pods, apitypes.NamespacedName{Namespace: p.Namespace, Name: p.Name})
	}

	time1, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:00Z")
	time2, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:10Z")
	time3, _ := time.Parse(time.RFC3339, "2017-01-02T13:01:20Z")

	contCpuRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromDouble(0.3), "cont2": fromDouble(0.4)},
		"namespace2:pod2": {"cont3": fromDouble(0.1), "cont4": fromDouble(0.2)},
		"namespace3:pod3": {"cont5": fromDouble(0.5), "cont6": fromDouble(0.6)},
	}
	contRamRes := map[string]map[string]resource.Quantity{
		"namespace1:pod1": {"cont1": fromInt(1002), "cont2": fromInt(1003)},
		"namespace2:pod2": {"cont3": fromInt(1000), "cont4": fromInt(1001)},
		"namespace3:pod3": {"cont5": fromInt(1004), "cont6": fromInt(1005)},
	}
	timeRes := map[string]api.TimeInfo{
		"namespace1:pod1": {time2, time.Minute},
		"namespace2:pod2": {time1, time.Minute},
		"namespace3:pod3": {time3, time.Minute},
	}
	var provider = CoreProvider{podClient([]string{doubleQuote(pods[0].Name), doubleQuote(pods[1].Name), doubleQuote(pods[2].Name)}, contCpuRes, timeRes, contRamRes, timeRes, t)}
	podMetrics, err := provider.GetPodMetrics(podObjects...)
	if err != nil {
		t.Fatalf("Provider error: %s", err)
	}

	expectedPodMetrics := []metrics.PodMetrics{

		metrics.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod1",
				Namespace:         "namespace1",
				CreationTimestamp: metav1.Now(),
			},
			Timestamp: metav1.NewTime(time2),
			Window:    metav1.Duration{Duration: time.Minute},
			Containers: []metrics.ContainerMetrics{
				{
					Name: "cont1",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.3),
						corev1.ResourceMemory: fromInt(1002),
					},
				},
				{
					Name: "cont2",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.4),
						corev1.ResourceMemory: fromInt(1003),
					},
				},
			},
		},

		metrics.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod2",
				Namespace:         "namespace2",
				CreationTimestamp: metav1.Now(),
			},
			Timestamp: metav1.NewTime(time1),
			Window:    metav1.Duration{Duration: time.Minute},
			Containers: []metrics.ContainerMetrics{
				{
					Name: "cont3",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.1),
						corev1.ResourceMemory: fromInt(1000),
					},
				},
				{
					Name: "cont4",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.2),
						corev1.ResourceMemory: fromInt(1001),
					},
				},
			},
		},

		metrics.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod3",
				Namespace:         "namespace3",
				CreationTimestamp: metav1.Now(),
			},
			Timestamp: metav1.NewTime(time3),
			Window:    metav1.Duration{Duration: time.Minute},
			Containers: []metrics.ContainerMetrics{
				{
					Name: "cont5",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.5),
						corev1.ResourceMemory: fromInt(1004),
					},
				},
				{
					Name: "cont6",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    fromDouble(0.6),
						corev1.ResourceMemory: fromInt(1005),
					},
				},
			},
		},
	}

	if len(expectedPodMetrics) != len(podMetrics) {
		t.Fatalf("Unexpected result. Expected len: \n%v,\n received: \n%v", len(expectedPodMetrics), len(podMetrics))
	}

	if diff := cmp.Diff(expectedPodMetrics, podMetrics, cmpopts.SortSlices(func(a, b metrics.ContainerMetrics) bool { return a.Name < b.Name }), cmpopts.IgnoreFields(metrics.PodMetrics{}, "ObjectMeta", "CreationTimestamp")); diff != "" {
		t.Errorf("Has a diff, (-want, +got): %s. Want: %v, got: %v.", diff, expectedPodMetrics, podMetrics)
	}

}
