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
	"encoding/json"
	"reflect"
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	stats "k8s.io/kubelet/pkg/apis/stats/v1alpha1"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm/monitor"
)

const (
	summaryJSON = `{
    "node": {
        "cpu": {
            "time": "2016-06-09T23:23:43Z",
            "usageCoreNanoSeconds": 10000000000,
            "usageNanoCores": 1000000000
        },
        "fs": {
            "availableBytes": 6000,
            "capacityBytes": 10000,
            "usedBytes": 4000
        },
        "memory": {
            "majorPageFaults": 6,
            "pageFaults": 10,
            "rssBytes": 2900,
            "time": "2016-06-09T23:23:43Z",
            "usageBytes": 2800,
            "workingSetBytes": 2700
        },
        "network": {
            "rxBytes": 1000,
            "rxErrors": 0,
            "time": "2016-06-09T23:23:43Z",
            "txBytes": 5000,
            "txErrors": 0
        },
        "nodeName": "gke-365122390874-ce73d81691de44a798a4",
        "startTime": "2016-06-08T00:25:37Z",
        "systemContainers": [
            {
                "cpu": {
                    "time": "2016-06-09T23:23:45Z",
                    "usageCoreNanoSeconds": 10000000000,
                    "usageNanoCores": 1000000000
                },
                "memory": {
                    "majorPageFaults": 5,
                    "pageFaults": 10,
                    "rssBytes": 2900,
                    "time": "2016-06-09T23:23:45Z",
                    "usageBytes": 2800,
                    "workingSetBytes": 2700
                },
                "name": "misc",
                "startTime": "2016-06-08T00:26:41Z",
                "userDefinedMetrics": null
            }
        ]
    },
    "pods": [
        {
            "containers": [
                {
                    "cpu": {
                        "time": "2016-06-09T23:23:51Z",
                        "usageCoreNanoSeconds": 10000000000,
                        "usageNanoCores": 1000000000
                    },
                    "logs": {
                        "availableBytes": 5000,
                        "capacityBytes": 8000,
                        "usedBytes": 3000
                    },
                    "memory": {
                        "majorPageFaults": 6,
                        "pageFaults": 10,
                        "rssBytes": 2900,
                        "time": "2016-06-09T23:23:51Z",
                        "usageBytes": 2800,
                        "workingSetBytes": 2700
                    },
                    "name": "test-container",
                    "rootfs": {
                        "availableBytes": 6000,
                        "capacityBytes": 10000,
                        "usedBytes": 4000
                    },
                    "startTime": "2016-06-08T00:27:48Z",
                    "userDefinedMetrics": null
                },
                {
                    "cpu": {
                        "time": "2016-06-09T23:23:50Z",
                        "usageCoreNanoSeconds": 1127596874,
                        "usageNanoCores": 0
                    },
                    "logs": {
                        "availableBytes": 6214086656,
                        "capacityBytes": 10432602112,
                        "usedBytes": 16384
                    },
                    "memory": {
                        "majorPageFaults": 0,
                        "pageFaults": 21866,
                        "rssBytes": 0,
                        "time": "2016-06-09T23:23:50Z",
                        "usageBytes": 192512,
                        "workingSetBytes": 131072
                    },
                    "name": "fluentd-cloud-logging",
                    "rootfs": {
                        "availableBytes": 6214086656,
                        "capacityBytes": 10432602112,
                        "usedBytes": 28672
                    },
                    "startTime": "2016-06-08T00:27:19Z",
                    "userDefinedMetrics": null
                }
            ],
            "network": {
                "rxBytes": 538477070,
                "rxErrors": 0,
                "time": "2016-06-09T23:23:43Z",
                "txBytes": 2969251391,
                "txErrors": 0
            },
            "podRef": {
                "name": "test-pod",
                "namespace": "kube-system",
                "uid": "e336ead99236b6eac0ce68e5336c86a0"
            },
            "startTime": "2016-06-08T00:27:47Z"
        }
    ]
}`

	incompleteContainerMemStatJSON = `{
    "podRef": {
        "name": "auditproxy-gke-12345678-1248-332a-vm",
        "namespace": "kube-system",
        "uid": "43780f1ce6e2171bc9e70cae3118ab6b"
    },
    "startTime": "2025-04-06T11:28:17Z",
    "containers": [
        {
            "name": "auditproxy",
            "startTime": "2025-04-23T15:17:45Z",
            "cpu": {
                "time": "2025-04-29T15:24:11Z",
                "usageNanoCores": 12352231,
                "usageCoreNanoSeconds": 4106274919000
            },
            "memory": {
                "time": "2025-04-29T15:24:11Z",
                "workingSetBytes": 76451840
            },
            "rootfs": {
                "time": "2025-04-29T15:24:04Z",
                "availableBytes": 4546961408,
                "capacityBytes": 16656896000,
                "usedBytes": 57344,
                "inodesFree": 963008,
                "inodes": 1036320,
                "inodesUsed": 18
            },
            "logs": {
                "time": "2025-04-29T15:24:11Z",
                "availableBytes": 4546961408,
                "capacityBytes": 16656896000,
                "usedBytes": 44961792,
                "inodesFree": 963008,
                "inodes": 1036320,
                "inodesUsed": 9
            },
            "swap": {
                "time": "2025-04-29T15:24:11Z",
                "swapAvailableBytes": 0,
                "swapUsageBytes": 0
            }
        }
    ],
    "cpu": {
        "time": "2025-04-29T15:23:59Z",
        "usageNanoCores": 7444062,
        "usageCoreNanoSeconds": 15900193844000
    },
    "memory": {
        "time": "2025-04-29T15:23:59Z",
        "usageBytes": 91631616,
        "workingSetBytes": 88707072,
        "rssBytes": 70098944,
        "pageFaults": 63466572,
        "majorPageFaults": 2461
    },
    "network": {
        "time": "2025-04-29T15:24:07Z",
        "name": "eth0",
        "rxBytes": 208635268392,
        "rxErrors": 0,
        "txBytes": 939343805472,
        "txErrors": 0,
        "interfaces": [
            {
                "name": "eth0",
                "rxBytes": 208635268392,
                "rxErrors": 0,
                "txBytes": 939343805472,
                "txErrors": 0
            }
        ]
    },
    "ephemeral-storage": {
        "time": "2025-04-29T15:24:11Z",
        "availableBytes": 4546961408,
        "capacityBytes": 16656896000,
        "usedBytes": 45023232,
        "inodesFree": 963008,
        "inodes": 1036320,
        "inodesUsed": 28
    },
    "swap": {
        "time": "2025-04-29T15:23:59Z",
        "swapUsageBytes": 0
    }
}`
)

// TestTranslator
func TestTranslator(t *testing.T) {
	testCases := []struct {
		Summary, Zone, Project, Cluster, ClusterLocation, Instance, InstanceID, NodeName, SchemaPrefix string
		monitoredResourceLabels                                                                        map[string]string
		Resolution                                                                                     time.Duration
		ExpectedTSCount                                                                                int
	}{
		{
			Zone:                    "us-central1-f",
			Project:                 "test-project",
			Cluster:                 "unit-test-clus",
			ClusterLocation:         "test-location",
			Instance:                "this-instance",
			InstanceID:              "id",
			SchemaPrefix:            "",
			monitoredResourceLabels: map[string]string{},
			Resolution:              time.Second * time.Duration(10),
			Summary:                 summaryJSON,
			ExpectedTSCount:         34,
		},
		{
			Zone:                    "us-central1-f",
			Project:                 "test-project",
			Cluster:                 "unit-test-clus",
			ClusterLocation:         "test-location",
			Instance:                "this-instance",
			InstanceID:              "id",
			SchemaPrefix:            "k8s_",
			monitoredResourceLabels: map[string]string{},
			Resolution:              time.Second * time.Duration(10),
			Summary:                 summaryJSON,
			ExpectedTSCount:         25,
		},
	}

	for i, tc := range testCases {
		summary := &stats.Summary{}
		if err := json.Unmarshal([]byte(tc.Summary), summary); err != nil {
			t.Errorf("Failed to unmarshal test case %d with data %s, err: %v", i, tc.Summary, err)
		}

		translator := NewTranslator(tc.Zone, tc.Project, tc.Cluster, tc.ClusterLocation, tc.Instance, tc.InstanceID, tc.SchemaPrefix, tc.monitoredResourceLabels, tc.Resolution)
		tsReq, err := translator.Translate(summary)
		if err != nil {
			t.Errorf("Failed to translate to GCM in test case %d. Summary: %v, Err: %s", i, tc.Summary, err)
		}

		if tc.ExpectedTSCount != len(tsReq.TimeSeries) {
			t.Errorf("Expected %d TimeSeries, got %d", tc.ExpectedTSCount, len(tsReq.TimeSeries))
		}
	}
}

func TestTranslator_NilFields(t *testing.T) {
	summary := &stats.Summary{
		Node: stats.NodeStats{
			StartTime: metav1.NewTime(time.Now()),
			Memory: &stats.MemoryStats{
				Time:            metav1.NewTime(time.Now().Add(time.Second)),
				WorkingSetBytes: monitor.Uint64Ptr(100),
				PageFaults:      monitor.Uint64Ptr(10),
				MajorPageFaults: nil, // This would cause panic in translateMemory
			},
			Fs: &stats.FsStats{
				CapacityBytes: monitor.Uint64Ptr(1000),
				UsedBytes:     monitor.Uint64Ptr(500),
			},
			CPU: &stats.CPUStats{
				Time:                 metav1.NewTime(time.Now().Add(time.Second)),
				UsageCoreNanoSeconds: monitor.Uint64Ptr(100),
			},
		},
		Pods: []stats.PodStats{
			{
				PodRef: stats.PodReference{Name: "test-pod", Namespace: "test-ns"},
				Containers: []stats.ContainerStats{
					{
						Name:      "test-container",
						StartTime: metav1.NewTime(time.Now()),
						CPU: &stats.CPUStats{
							Time:                 metav1.NewTime(time.Now().Add(time.Second)),
							UsageCoreNanoSeconds: monitor.Uint64Ptr(100),
						},
						Memory: &stats.MemoryStats{
							Time:            metav1.NewTime(time.Now().Add(time.Second)),
							WorkingSetBytes: monitor.Uint64Ptr(100),
						},
						Rootfs: &stats.FsStats{
							UsedBytes: nil, // This would cause panic in containerTranslateFS
						},
						Logs: &stats.FsStats{
							UsedBytes: nil, // This would cause panic in containerTranslateFS
						},
					},
				},
			},
		},
	}

	translator := NewTranslator("zone", "project", "cluster", "location", "instance", "id", "k8s_", map[string]string{}, time.Minute)
	_, err := translator.Translate(summary)
	if err != nil {
		t.Errorf("Translate failed: %v", err)
	}
}

func TestTranslateContainers(t *testing.T) {
	aliceContainer := *getContainerStats(false)
	bobContainer := *getContainerStats(false)
	noMemStatsContainer := *getContainerStats(false)
	noMemStatsContainer.Memory = nil
	noCPUStatsContainer := *getContainerStats(false)
	noCPUStatsContainer.CPU = nil
	noLogStatsContainer := *getContainerStats(false)
	noLogStatsContainer.Logs = nil
	noRootfsStatsContainer := *getContainerStats(false)
	noRootfsStatsContainer.Rootfs = nil
	badTimestampOnCumulativeMetricsContrainer := *getContainerStats(false)
	badTimestampOnCumulativeMetricsContrainer.Memory.Time = badTimestampOnCumulativeMetricsContrainer.StartTime
	badTimestampOnCumulativeMetricsContrainer.CPU.Time = badTimestampOnCumulativeMetricsContrainer.StartTime
	legacyTsPerContainer := 11
	tsPerContainer := 9
	inCompleteContainerMemPodStat := &stats.PodStats{}
	if err := json.Unmarshal([]byte(incompleteContainerMemStatJSON), inCompleteContainerMemPodStat); err != nil {
		t.Errorf("Failed to unmarshal incompleteContainerMemStatJSON, err: %v", err)
	}
	testCases := []struct {
		name                  string
		ExpectedLegacyTSCount int
		ExpectedTSCount       int
		pods                  []stats.PodStats
	}{
		{
			name:                  "empty",
			ExpectedLegacyTSCount: 0,
			ExpectedTSCount:       0,
			pods:                  []stats.PodStats{},
		},
		{
			name:                  "pod without container",
			ExpectedLegacyTSCount: 0,
			ExpectedTSCount:       0,
			pods: []stats.PodStats{
				getPodStats(),
			},
		},
		{
			name:                  "single pod with one container",
			ExpectedLegacyTSCount: legacyTsPerContainer,
			ExpectedTSCount:       tsPerContainer,
			pods: []stats.PodStats{
				getPodStats(aliceContainer),
			},
		},
		{
			name:                  "single pod with one container without usageNanoCores",
			ExpectedLegacyTSCount: legacyTsPerContainer,
			ExpectedTSCount:       tsPerContainer,
			pods: []stats.PodStats{
				getPodStats(*getContainerStats(true)),
			},
		},
		{
			name:                  "single pod with two containers",
			ExpectedLegacyTSCount: legacyTsPerContainer * 2,
			ExpectedTSCount:       tsPerContainer * 2,
			pods: []stats.PodStats{
				getPodStats(aliceContainer, bobContainer),
			},
		},
		{
			name:                  "single pod with similar container",
			ExpectedLegacyTSCount: legacyTsPerContainer,
			ExpectedTSCount:       tsPerContainer,
			pods: []stats.PodStats{
				getPodStats(aliceContainer, aliceContainer),
			},
		},
		{
			name:                  "two pods with one container each",
			ExpectedLegacyTSCount: legacyTsPerContainer * 2,
			ExpectedTSCount:       tsPerContainer * 2,
			pods: []stats.PodStats{
				getPodStats(aliceContainer),
				getPodStats(bobContainer),
			},
		},
		{
			name:                  "two pods with similar container",
			ExpectedLegacyTSCount: legacyTsPerContainer * 2,
			ExpectedTSCount:       tsPerContainer * 2,
			pods: []stats.PodStats{
				getPodStats(aliceContainer),
				getPodStats(aliceContainer),
			},
		},
		{
			name:                  "single pod with empty stats container",
			ExpectedLegacyTSCount: legacyTsPerContainer,
			ExpectedTSCount:       tsPerContainer * 3,
			pods: []stats.PodStats{
				getPodStats(
					aliceContainer,
					noMemStatsContainer,
					noCPUStatsContainer,
					noLogStatsContainer,
					noRootfsStatsContainer,
				),
			},
		},
		{
			name:                  "bad timestamp for cumulative metrics",
			ExpectedLegacyTSCount: legacyTsPerContainer - 3,
			ExpectedTSCount:       tsPerContainer - 3,
			pods: []stats.PodStats{
				getPodStats(
					badTimestampOnCumulativeMetricsContrainer,
				),
			},
		},
		{
			name:                  "inCompleteContainerMemPodStat missing memory usageBytes pageFaults, and majorPageFaults",
			ExpectedLegacyTSCount: 7,
			ExpectedTSCount:       6,
			pods:                  []stats.PodStats{*inCompleteContainerMemPodStat},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			legacyTranslator := NewTranslator("us-central1-f", "test-project", "unit-test-clus", "test-location", "this-instance", "id", "", map[string]string{}, time.Second)
			ts, err := legacyTranslator.translateContainers(tc.pods)
			if err != nil {
				t.Errorf("Failed to translate to GCM. Pods: %v, Err: %s", tc.pods, err)
			}
			if tc.ExpectedLegacyTSCount != len(ts) {
				t.Errorf("Expected %d TimeSeries, got %d", tc.ExpectedLegacyTSCount, len(ts))
			}

			translator := NewTranslator("us-central1-f", "test-project", "unit-test-clus", "test-location", "this-instance", "id", "k8s_", map[string]string{}, time.Second)
			ts, err = translator.translateContainers(tc.pods)
			if err != nil {
				t.Errorf("Failed to translate to GCM. Pods: %v, Err: %s", tc.pods, err)
			}
			if tc.ExpectedTSCount != len(ts) {
				t.Errorf("Expected %d TimeSeries, got %d", tc.ExpectedTSCount, len(ts))
			}
		})
	}
}

func TestTranslatePodAndContainerMemoryMetrics(t *testing.T) {
	podStat := &stats.PodStats{}
	if err := json.Unmarshal([]byte(incompleteContainerMemStatJSON), podStat); err != nil {
		t.Fatalf("json.Unmarshal(incompleteContainerMemStatJSON) failed: %v", err)
	}
	podStat.Containers[0].Memory.RSSBytes = monitor.Uint64Ptr(65432100)

	wantContainerTime := podStat.Containers[0].Memory.Time.Format(time.RFC3339)
	wantPodTime := podStat.Memory.Time.Format(time.RFC3339)

	testCases := []struct {
		name                string
		schemaPrefix        string
		extraLabels         map[string]string
		wantPodLabels       map[string]string
		wantContainerLabels map[string]string
	}{
		{
			name:         "k8s_ prefix skips instance_id",
			schemaPrefix: "k8s_",
			extraLabels:  map[string]string{},
			wantPodLabels: map[string]string{
				"project_id":     "test-project",
				"cluster_name":   "unit-test-clus",
				"location":       "test-location",
				"namespace_name": "kube-system",
				"pod_name":       "auditproxy-gke-12345678-1248-332a-vm",
			},
			wantContainerLabels: map[string]string{
				"project_id":     "test-project",
				"cluster_name":   "unit-test-clus",
				"location":       "test-location",
				"namespace_name": "kube-system",
				"pod_name":       "auditproxy-gke-12345678-1248-332a-vm",
				"container_name": "auditproxy",
			},
		},
		{
			name:         "custom_ prefix includes instance_id and extra labels",
			schemaPrefix: "custom_",
			extraLabels: map[string]string{
				"cluster_hash":        "hash-123",
				"consumer_project_id": "consumer-proj",
			},
			wantPodLabels: map[string]string{
				"project_id":          "test-project",
				"cluster_name":        "unit-test-clus",
				"location":            "test-location",
				"instance_id":         "instance-id-1",
				"cluster_hash":        "hash-123",
				"consumer_project_id": "consumer-proj",
				"namespace_name":      "kube-system",
				"pod_name":            "auditproxy-gke-12345678-1248-332a-vm",
			},
			wantContainerLabels: map[string]string{
				"project_id":          "test-project",
				"cluster_name":        "unit-test-clus",
				"location":            "test-location",
				"instance_id":         "instance-id-1",
				"cluster_hash":        "hash-123",
				"consumer_project_id": "consumer-proj",
				"namespace_name":      "kube-system",
				"pod_name":            "auditproxy-gke-12345678-1248-332a-vm",
				"container_name":      "auditproxy",
			},
		},
	}

	type metricSummary struct {
		value        int64
		metricKind   string
		valueType    string
		startTime    string
		endTime      string
		resourceType string
		labels       map[string]string
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			translator := NewTranslator("us-central1-f", "test-project", "unit-test-clus", "test-location", "", "instance-id-1", tc.schemaPrefix, tc.extraLabels, time.Minute)
			tsList, err := translator.translateContainers([]stats.PodStats{*podStat})
			if err != nil {
				t.Fatalf("translateContainers(%v) returned unexpected error: %v", podStat.PodRef.Name, err)
			}

			got := make(map[string]metricSummary)
			for _, ts := range tsList {
				switch ts.Metric.Type {
				case "kubernetes.io/container/memory/rss",
					"kubernetes.io/pod/memory/rss",
					"kubernetes.io/pod/memory/working_set":
					got[ts.Metric.Type] = metricSummary{
						value:        *ts.Points[0].Value.Int64Value,
						metricKind:   ts.MetricKind,
						valueType:    ts.ValueType,
						startTime:    ts.Points[0].Interval.StartTime,
						endTime:      ts.Points[0].Interval.EndTime,
						resourceType: ts.Resource.Type,
						labels:       ts.Resource.Labels,
					}
				}
			}

			want := map[string]metricSummary{
				"kubernetes.io/container/memory/rss": {
					value:        65432100,
					metricKind:   "GAUGE",
					valueType:    "INT64",
					startTime:    wantContainerTime,
					endTime:      wantContainerTime,
					resourceType: tc.schemaPrefix + "container",
					labels:       tc.wantContainerLabels,
				},
				"kubernetes.io/pod/memory/rss": {
					value:        70098944,
					metricKind:   "GAUGE",
					valueType:    "INT64",
					startTime:    wantPodTime,
					endTime:      wantPodTime,
					resourceType: tc.schemaPrefix + "pod",
					labels:       tc.wantPodLabels,
				},
				"kubernetes.io/pod/memory/working_set": {
					value:        88707072,
					metricKind:   "GAUGE",
					valueType:    "INT64",
					startTime:    wantPodTime,
					endTime:      wantPodTime,
					resourceType: tc.schemaPrefix + "pod",
					labels:       tc.wantPodLabels,
				},
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("translateContainers(%q) memory metrics = %+v, want %+v", podStat.PodRef.Name, got, want)
			}
		})
	}
}

func getPodStats(containers ...stats.ContainerStats) stats.PodStats {
	return stats.PodStats{
		PodRef:      stats.PodReference{Name: "test-pod", Namespace: "test-namespace", UID: "UID_test-pod"},
		StartTime:   metav1.NewTime(time.Now()),
		Containers:  containers,
		Network:     getNetworkStats(),
		VolumeStats: []stats.VolumeStats{*getVolumeStats()},
	}
}

func getContainerStats(skipUsageNanoCores bool) *stats.ContainerStats {
	f := fuzz.New().NilChance(0)
	v := &stats.ContainerStats{}
	f.Fuzz(v)
	if skipUsageNanoCores {
		v.CPU.UsageNanoCores = nil
	}
	if v.Memory.Time.Time.Before(v.StartTime.Time) {
		v.Memory.Time, v.StartTime = v.StartTime, v.Memory.Time
	}
	if v.CPU.Time.Time.Before(v.StartTime.Time) {
		v.CPU.Time, v.StartTime = v.StartTime, v.CPU.Time
	}
	return v
}

func getVolumeStats() *stats.VolumeStats {
	f := fuzz.New().NilChance(0)
	v := &stats.VolumeStats{}
	f.Fuzz(v)
	return v
}

func getNetworkStats() *stats.NetworkStats {
	f := fuzz.New().NilChance(0)
	v := &stats.NetworkStats{}
	f.Fuzz(v)
	return v
}
