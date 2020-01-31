/*
Copyright 2017 Google Inc.

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

package kubelet

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	v3 "google.golang.org/api/monitoring/v3"
	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/stats"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm/monitor"
)

var (
	// Container metrics

	containerUptimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "kubernetes.io/container/uptime",
	}

	containerCpuCoreUsageTimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "kubernetes.io/container/cpu/core_usage_time",
	}

	containerMemTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/container/memory/limit_bytes",
	}

	containerMemUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/container/memory/used_bytes",
	}

	containerPageFaultsMD = &metricMetadata{
		MetricKind: "DELTA",
		ValueType:  "INT64",
		Name:       "kubernetes.io/container/memory/page_fault_kind",
	}

	containerEphemeralstorageUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/container/ephemeral_storage/used_bytes",
	}

	// Node metrics

	nodeCpuCoreUsageTimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "kubernetes.io/node/cpu/core_usage_time",
	}

	nodeMemTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/node/total_bytes",
	}

	nodeMemUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/node/used_bytes",
	}

	nodeEphemeralstorageTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/node/ephemeral_storage/total_bytes",
	}

	nodeEphemeralstorageUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "kubernetes.io/node/ephemeral_storage/used_bytes",
	}

	// Legacy metrics.

	legacyUsageTimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/cpu/usage_time",
	}
	legacyDiskTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/disk/bytes_total",
	}
	legacyDiskUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/disk/bytes_used",
	}
	legacyMemTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/bytes_total",
	}
	legacyMemUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/bytes_used",
	}
	legacyPageFaultsMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/page_fault_count",
	}
	legacyUptimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/uptime",
	}

	memUsedNonEvictableLabels = map[string]string{"memory_type": "non-evictable"}
	memUsedEvictableLabels    = map[string]string{"memory_type": "evictable"}
	minorPageFaultLabels      = map[string]string{"fault_type": "minor"}
	majorPageFaultLabels      = map[string]string{"fault_type": "major"}
	noLabels                  = map[string]string{}
)

type metricMetadata struct {
	MetricKind, ValueType, Name string
}

// Translator contains the required information to perform translations from
// kubelet summarys to GCM's GKE metrics.
type Translator struct {
	zone, project, cluster, instanceID, metricPrefix string
	resolution                                       time.Duration
	useOldResourceModel                              bool
}

// NewTranslator creates a new Translator with the given fields.
func NewTranslator(zone, project, cluster, instanceID, metricPrefix string, resolution time.Duration) *Translator {
	return &Translator{
		zone:                zone,
		project:             project,
		cluster:             cluster,
		instanceID:          instanceID,
		metricPrefix:        metricPrefix,
		resolution:          resolution,
		useOldResourceModel: metricPrefix == "",
	}
}

// Translate translates a summary to its TimeSeries.
func (t *Translator) Translate(summary *stats.Summary) (*v3.CreateTimeSeriesRequest, error) {
	var ts []*v3.TimeSeries

	nodeTs, err := t.translateNode(summary.Node)
	if err != nil {
		return nil, err
	}
	podsTs, err := t.translateContainers(summary.Pods)
	if err != nil {
		return nil, err
	}

	ts = append(ts, nodeTs...)
	ts = append(ts, podsTs...)
	return &v3.CreateTimeSeriesRequest{TimeSeries: ts}, nil
}

func (t *Translator) translateNode(node stats.NodeStats) ([]*v3.TimeSeries, error) {
	var (
		timeSeries, memTS, fsTS, cpuTS []*v3.TimeSeries
		tsFactory	*timeSeriesFactory
		err error
	)

	if t.useOldResourceModel {
		tsFactory = newTimeSeriesFactory(t.getMonitoredResource(map[string]string{"pod": "machine"}), t.resolution)
	} else {
		tsFactory = newTimeSeriesFactory(t.getMonitoredResource(map[string]string{"node": node.NodeName}), t.resolution)
	}

	// Uptime. This is embedded: there's no nil check.
	now := time.Now()
	uptimePoint := &v3.Point{
		Interval: &v3.TimeInterval{
			EndTime:   now.Format(time.RFC3339),
			StartTime: node.StartTime.Time.Format(time.RFC3339),
		},
		Value: &v3.TypedValue{
			DoubleValue: monitor.Float64Ptr(float64(time.Since(node.StartTime.Time).Seconds())),
		},
	}
	metric := containerUptimeMD
	if t.useOldResourceModel {
		metric = legacyUptimeMD
	}
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, metric, uptimePoint))

	// Memory stats.
	if t.useOldResourceModel {
		memTS, err = legacyTranslateMemory(node.Memory, tsFactory, node.StartTime.Time)
	} else {
		memTS, err = nodeTranslateMemory(node.Memory, tsFactory, node.StartTime.Time)
	}
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, memTS...)

	// File-system stats.
	if t.useOldResourceModel {
		fsTS, err = legacyTranslateFS("/", node.Fs, tsFactory, node.StartTime.Time)
	} else {
		fsTS, err = nodeTranslateFS("/", node.Fs, tsFactory, node.StartTime.Time)
	}
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, fsTS...)

	// CPU stats.
	if t.useOldResourceModel {
		cpuTS, err = legacyTranslateCPU(node.CPU, tsFactory, node.StartTime.Time)
	} else {
		cpuTS, err = nodeTranslateCPU(node.CPU, tsFactory, node.StartTime.Time)
	}
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, cpuTS...)

	// System containers
	for _, container := range node.SystemContainers {
		// For system containers:
		// * There won't be duplication;
		// * There aren't pod id and namespace;
		// * There is no fs stats.
		// Pod ID and namespace for system containers are empty.
		containerSeries, err := t.translateContainer("", "", container, false /* requireFsStats */)
		if err != nil {
			glog.Warningf("Failed to translate system container stats for %q: %v", container.Name, err)
			continue
		}
		timeSeries = append(timeSeries, containerSeries...)
	}

	return timeSeries, nil
}

func (t *Translator) translateContainers(pods []stats.PodStats) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries
	for _, pod := range pods {
		metricsSeen := make(map[string]time.Time)
		metrics := make(map[string][]*v3.TimeSeries)
		namespace := pod.PodRef.Namespace
		podID := pod.PodRef.Name
		// There can be duplicate data points for containers, so only
		// take the latest one.
		for _, container := range pod.Containers {
			containerName := container.Name
			// Check for duplicates
			if container.StartTime.Time.Before(metricsSeen[containerName]) || container.StartTime.Time.Equal(metricsSeen[containerName]) {
				continue
			}
			metricsSeen[containerName] = container.StartTime.Time

			containerSeries, err := t.translateContainer(podID, namespace, container, true /* requireFsStats */)
			if err != nil {
				glog.Warningf("Failed to translate container stats for container %q in pod %q(%q): %v",
					containerName, podID, namespace, err)
				continue
			}
			metrics[containerName] = containerSeries
		}

		// Flatten the deduplicated metrics.
		for _, containerSeries := range metrics {
			timeSeries = append(timeSeries, containerSeries...)
		}
	}
	return timeSeries, nil
}

func (t *Translator) translateContainer(podID, namespace string, container stats.ContainerStats, requireFsStats bool) ([]*v3.TimeSeries, error) {
	var (
		containerSeries, memTS, rootfsTS, logfsTS, cpuTS []*v3.TimeSeries
		err error
		containerName   = container.Name
		containerLabels = map[string]string{
			"namespace": 	namespace,
			"pod":			podID,
			"container":	containerName,
		}
	)

	tsFactory := newTimeSeriesFactory(t.getMonitoredResource(containerLabels), t.resolution)

	// Uptime. This is embedded: there's no nil check.
	now := time.Now()
	uptimePoint := &v3.Point{
		Interval: &v3.TimeInterval{
			EndTime:   now.Format(time.RFC3339),
			StartTime: container.StartTime.Time.Format(time.RFC3339),
		},
		Value: &v3.TypedValue{
			DoubleValue:     monitor.Float64Ptr(float64(time.Since(container.StartTime.Time).Seconds())),
			ForceSendFields: []string{"DoubleValue"},
		},
	}
	metric := containerUptimeMD
	if t.useOldResourceModel {
		metric = legacyUptimeMD
	}
	containerSeries = append(containerSeries, tsFactory.newTimeSeries(noLabels, metric, uptimePoint))

	// Memory stats.
	if t.useOldResourceModel {
		memTS, err = legacyTranslateMemory(container.Memory, tsFactory, container.StartTime.Time)
	} else {
		memTS, err = containerTranslateMemory(container.Memory, tsFactory, container.StartTime.Time)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to translate memory stats: %v", err)
	}
	containerSeries = append(containerSeries, memTS...)

	// File-system stats.
	if t.useOldResourceModel {
		rootfsTS, err = legacyTranslateFS("/", container.Rootfs, tsFactory, container.StartTime.Time)
	} else {
		rootfsTS, err = containerTranslateFS("/", container.Rootfs, container.Logs, tsFactory, container.StartTime.Time)
	}
	if err != nil {
		if requireFsStats {
			return nil, fmt.Errorf("failed to translate rootfs stats: %v", err)
		}
	} else {
		containerSeries = append(containerSeries, rootfsTS...)
	}

	if t.useOldResourceModel {
		logfsTS, err = legacyTranslateFS("logs", container.Logs, tsFactory, container.StartTime.Time)
		if err != nil {
			if requireFsStats {
				return nil, fmt.Errorf("failed to translate log stats: %v", err)
			}
		} else {
			containerSeries = append(containerSeries, logfsTS...)
		}
	}

	// CPU stats.
	if t.useOldResourceModel {
		cpuTS, err = legacyTranslateCPU(container.CPU, tsFactory, container.StartTime.Time)
	} else {
		cpuTS, err = containerTranslateCPU(container.CPU, tsFactory, container.StartTime.Time)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to translate cpu stats: %v", err)
	}
	containerSeries = append(containerSeries, cpuTS...)

	return containerSeries, nil
}

func legacyTranslateCPU(cpu *stats.CPUStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	return translateCPU(cpu, tsFactory, startTime, map[string]*metricMetadata{"usage_time": legacyUsageTimeMD})
}

func nodeTranslateCPU(cpu *stats.CPUStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	return translateCPU(cpu, tsFactory, startTime, map[string]*metricMetadata{"usage_time": nodeCpuCoreUsageTimeMD})
}

func containerTranslateCPU(cpu *stats.CPUStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	return translateCPU(cpu, tsFactory, startTime, map[string]*metricMetadata{"usage_time": containerCpuCoreUsageTimeMD})
}

// translateCPU creates all the TimeSeries for a give CPUStat.
func translateCPU(cpu *stats.CPUStats, tsFactory *timeSeriesFactory, startTime time.Time, metrics map[string]*metricMetadata) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	// First check that all required information is present.
	if cpu == nil {
		return nil, fmt.Errorf("CPU information missing.")
	}
	if cpu.UsageCoreNanoSeconds == nil {
		return nil, fmt.Errorf("UsageCoreNanoSeconds missing from CPUStats %v", cpu)
	}

	// Total CPU utilization for all time. Convert from nanosec to sec.
	cpuTotalPoint := tsFactory.newPoint(&v3.TypedValue{
		DoubleValue:     monitor.Float64Ptr(float64(*cpu.UsageCoreNanoSeconds) / float64(1000*1000*1000)),
		ForceSendFields: []string{"DoubleValue"},
	}, startTime, cpu.Time.Time, metrics["usage_time"].MetricKind)

	timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, metrics["usage_time"], cpuTotalPoint))
	return timeSeries, nil
}

func legacyTranslateFS(volume string, fs *stats.FsStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	legacyFSMetrics := map[string]*metricMetadata{
		"disk_total": legacyDiskTotalMD,
		"disk_used": legacyDiskUsedMD,
	}
	return translateFS(volume, fs, tsFactory, startTime, legacyFSMetrics)
}

func nodeTranslateFS(volume string, fs *stats.FsStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	nodeFSMetrics := map[string]*metricMetadata{
		"disk_total": nodeEphemeralstorageUsedMD,
		"disk_used": nodeEphemeralstorageTotalMD,
	}
	return translateFS(volume, fs, tsFactory, startTime, nodeFSMetrics)
}

func containerTranslateFS(volume string, rootfs *stats.FsStats, logs *stats.FsStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	containerFSMetrics := map[string]*metricMetadata{
		"disk_used": containerEphemeralstorageUsedMD,
	}

	var combinedUsage uint64
	if rootfs != nil {
		combinedUsage = *rootfs.UsedBytes
	}
	if logs != nil {
		combinedUsage = combinedUsage + *logs.UsedBytes
	}
	
	combinedStats := &stats.FsStats{
		UsedBytes: &combinedUsage,
	}
	if rootfs == nil && logs == nil {
		combinedStats = nil
	}
	return translateFS(volume, combinedStats, tsFactory, startTime, containerFSMetrics)
}

// translateFS creates all the TimeSeries for a given FsStats and volume name.
func translateFS(volume string, fs *stats.FsStats, tsFactory *timeSeriesFactory, startTime time.Time, metrics map[string]*metricMetadata) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	if fs == nil {
		return nil, fmt.Errorf("File-system information missing.")
	}

	// For some reason the Kubelet doesn't return when this sample is from,
	// so we'll use now.
	now := time.Now()

	resourceLabels := map[string]string{"device_name": volume}
	if diskTotalMD, found := metrics["disk_total"]; found {
		if fs.CapacityBytes == nil {
			return nil, fmt.Errorf("CapacityBytes is missing from FsStats %v", fs)
		}
		// Total disk available.
		diskTotalPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*fs.CapacityBytes)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, now, diskTotalMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(resourceLabels, diskTotalMD, diskTotalPoint))
	}

	if diskUsedMD, found := metrics["disk_used"]; found {
		if fs.UsedBytes == nil {
			return nil, fmt.Errorf("UsedBytes is missing from FsStats %v", fs)
		}
		// Total disk used.
		diskUsedPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*fs.UsedBytes)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, now, diskUsedMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(resourceLabels, diskUsedMD, diskUsedPoint))
	}
	return timeSeries, nil
}

func legacyTranslateMemory(memory *stats.MemoryStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	legacyMemoryMetrics := map[string]*metricMetadata{
		"mem_total": legacyMemTotalMD,
		"mem_used": legacyMemUsedMD,
		"page_faults": legacyPageFaultsMD,
	}
	return translateMemory(memory, tsFactory, startTime, legacyMemoryMetrics)
}

func nodeTranslateMemory(memory *stats.MemoryStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	nodeMemoryMetrics := map[string]*metricMetadata{
		"mem_total": nodeMemTotalMD,
		"mem_used": nodeMemUsedMD,
	}
	return translateMemory(memory, tsFactory, startTime, nodeMemoryMetrics)
}

func containerTranslateMemory(memory *stats.MemoryStats, tsFactory *timeSeriesFactory, startTime time.Time) ([]*v3.TimeSeries, error) {
	containerMemoryMetrics := map[string]*metricMetadata{
		"mem_total": containerMemTotalMD,
		"mem_used": containerMemUsedMD,
		"page_faults": containerPageFaultsMD,
	}
	return translateMemory(memory, tsFactory, startTime, containerMemoryMetrics)
}

// translateMemory creates all the TimeSeries for a given MemoryStats.
func translateMemory(memory *stats.MemoryStats, tsFactory *timeSeriesFactory, startTime time.Time, metrics map[string]*metricMetadata) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	if memory == nil {
		return nil, fmt.Errorf("Memory information missing.")
	}

	if pageFaultsMD, found := metrics["page_faults"]; found {
		if memory.MajorPageFaults == nil {
			return nil, fmt.Errorf("MajorPageFaults missing in MemoryStats %v", memory)
		}
		if memory.PageFaults == nil {
			return nil, fmt.Errorf("PageFaults missing in MemoryStats %v", memory)
		}
		// Major page faults.
		majorPFPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*memory.MajorPageFaults)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, memory.Time.Time, pageFaultsMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(majorPageFaultLabels, pageFaultsMD, majorPFPoint))
		// Minor page faults.
		minorPFPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*memory.PageFaults - *memory.MajorPageFaults)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, memory.Time.Time, pageFaultsMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(minorPageFaultLabels, pageFaultsMD, minorPFPoint))
	}

	if memUsedMD, found := metrics["mem_used"]; found {
		if memory.WorkingSetBytes == nil {
			return nil, fmt.Errorf("WorkingSetBytes information missing in MemoryStats %v", memory)
		}
		if memory.UsageBytes == nil {
			return nil, fmt.Errorf("UsageBytes information missing in MemoryStats %v", memory)
		}

		// Non-evictable memory.
		nonEvictMemPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*memory.WorkingSetBytes)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, memory.Time.Time, memUsedMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(memUsedNonEvictableLabels, memUsedMD, nonEvictMemPoint))
		// Evictable memory.
		evictMemPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*memory.UsageBytes - *memory.WorkingSetBytes)),
			ForceSendFields: []string{"Int64Value"},
		}, startTime, memory.Time.Time, memUsedMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(memUsedEvictableLabels, memUsedMD, evictMemPoint))
	}

	if memTotalMD, found := metrics["mem_total"]; found {
		// Available memory. This may or may not be present, so don't fail if it's absent.
		if memory.AvailableBytes != nil {
			availableMemPoint := tsFactory.newPoint(&v3.TypedValue{
				Int64Value:      monitor.Int64Ptr(int64(*memory.AvailableBytes)),
				ForceSendFields: []string{"Int64Value"},
			}, startTime, memory.Time.Time, memTotalMD.MetricKind)
			timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, memTotalMD, availableMemPoint))
		}
	}
	return timeSeries, nil
}

func (t *Translator) getMonitoredResource(labels map[string]string) *v3.MonitoredResource {
	resourceLabels := map[string]string{
		"project_id":	t.project,
		"cluster_name":	t.cluster,
	}

	if t.useOldResourceModel {
		resourceLabels["zone"] = t.zone
		resourceLabels["instance_id"] = t.instanceID
		resourceLabels["namespace_id"] = labels["namespace"]
		resourceLabels["pod_id"] = labels["pod"]
		resourceLabels["container_name"] = labels["container"]
		return &v3.MonitoredResource{
			Type:	"gke_container",
			Labels: resourceLabels,
		}
	}

	resourceLabels["location"] = t.zone

	if _, found := labels["node"]; found {
		resourceLabels["node_name"] = labels["node"]
		return &v3.MonitoredResource{
			Type:   t.metricPrefix + "node",
			Labels: resourceLabels,
		}
	}

	resourceLabels["namespace_id"] = labels["namespace"]
	resourceLabels["pod_id"] = labels["pod"]
	resourceLabels["container_name"] = labels["container"]

	return &v3.MonitoredResource{
		Type:   t.metricPrefix + "container",
		Labels: resourceLabels,
	}
}

type timeSeriesFactory struct {
	resolution        time.Duration
	monitoredResource *v3.MonitoredResource
}

func newTimeSeriesFactory(monitoredResource *v3.MonitoredResource, resolution time.Duration) *timeSeriesFactory {
	return &timeSeriesFactory{
		resolution:        resolution,
		monitoredResource: monitoredResource,
	}
}

func (t *timeSeriesFactory) newPoint(val *v3.TypedValue, collectionStartTime time.Time, sampleTime time.Time, metricKind string) *v3.Point {
	if metricKind == "GAUGE" {
		collectionStartTime = sampleTime
	}
	return &v3.Point{
		Interval: &v3.TimeInterval{
			EndTime:   sampleTime.Format(time.RFC3339),
			StartTime: collectionStartTime.Format(time.RFC3339),
		},
		Value: val,
	}
}

func (t *timeSeriesFactory) newTimeSeries(metricLabels map[string]string, metadata *metricMetadata, point *v3.Point) *v3.TimeSeries {
	return &v3.TimeSeries{
		Metric: &v3.Metric{
			Labels: metricLabels,
			Type:   metadata.Name,
		},
		MetricKind: metadata.MetricKind,
		ValueType:  metadata.ValueType,
		Resource:   t.monitoredResource,
		Points:     []*v3.Point{point},
	}
}
