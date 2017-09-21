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

package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	gce "cloud.google.com/go/compute/metadata"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	v3 "google.golang.org/api/monitoring/v3"
	"strings"
)

// SD Dummy Exporter is a testing utility that exports a metric of constant value to Stackdriver
// in a loop. Metric name and value can be specified with flags 'metric-name' and 'metric-value'.
// SD Dummy Exporter assumes that it runs as a pod in GCE or GKE cluster, and the pod id is passed
// to it with 'pod-id' flag (which can be passed to a pod via Downward API).
func main() {
	// Gather pod information
	podIdFlag := flag.String("pod-id", "", "a string")
	metricNameFlag := flag.String("metric-name", "foo", "a string")
	metricValueFlag := flag.Int64("metric-value", 0, "an int")
	flag.Parse()
	projectId, _ := gce.ProjectID()
	zone, _ := gce.Zone()
	clusterName, _ := gce.InstanceAttributeValue("cluster-name")
	clusterName = strings.TrimSpace(clusterName)
	containerName := ""
	podId := *podIdFlag
	metricName := *metricNameFlag
	metricValue := *metricValueFlag

	oauthClient := oauth2.NewClient(context.Background(), google.ComputeTokenSource(""))
	stackdriverService, err := v3.New(oauthClient)
	if err != nil {
		log.Print("error: %s", err)
		return
	}

	for {
		// Prepare an individual data point
		dataPoint := &v3.Point{
			Interval: &v3.TimeInterval{
				EndTime: time.Now().Format(time.RFC3339),
			},
			Value: &v3.TypedValue{
				Int64Value: &metricValue,
			},
		}
		// Write time series data.
		request := &v3.CreateTimeSeriesRequest{
			TimeSeries: []*v3.TimeSeries{
				{
					Metric: &v3.Metric{
						Type: "custom.googleapis.com/" + metricName,
					},
					Resource: &v3.MonitoredResource{
						Type: "gke_container",
						Labels: map[string]string{
							"project_id":     projectId,
							"zone":           zone,
							"cluster_name":   clusterName,
							"container_name": containerName,
							"pod_id":         podId,
							// namespace_id and instance_id don't matter
							"namespace_id": "default",
							"instance_id":  "",
						},
					},
					Points: []*v3.Point{
						dataPoint,
					},
				},
			},
		}
		_, err := stackdriverService.Projects.TimeSeries.Create(fmt.Sprintf("projects/%s", projectId), request).Do()
		if err != nil {
			log.Printf("Failed to write time series data: %v\n", err)
		} else {
			log.Printf("Finished writing time series with value: %v\n", metricValue)
		}
		time.Sleep(5000 * time.Millisecond)
	}
}
