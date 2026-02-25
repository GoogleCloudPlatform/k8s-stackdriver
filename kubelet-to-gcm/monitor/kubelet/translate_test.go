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
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	stats "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
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
			ExpectedTSCount:         23,
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
	tsPerContainer := 8
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
			ExpectedTSCount:       4,
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
