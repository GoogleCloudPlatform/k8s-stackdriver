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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2/google"
	monitoring "google.golang.org/api/monitoring/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // Load the GCP plugin (only required to authenticate against GKE clusters).
	"k8s.io/client-go/tools/clientcmd"
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

// Returns the value from a TypedValue as an int64. Floats are returned after
// casting, other types are returned as zero.
func ValueAsInt64(value *monitoring.TypedValue) int64 {
	if value == nil {
		return 0
	}
	switch {
	case value.Int64Value != nil:
		return *value.Int64Value
	case value.DoubleValue != nil:
		return int64(*value.DoubleValue)
	default:
		return 0
	}
}

// TODO(jkohen): Move to its own module (kubernetes.go?).
func getTestInstanceId() string {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	// Use the current context in kubeconfig.
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	const namespace = "default"
	pod, err := clientset.CoreV1().Pods(namespace).Get("echo", metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}
	glog.V(1).Infof("Found pod nodename: %v", pod.Spec.NodeName)
	return pod.Spec.NodeName
}

func queryStackdriverMetric(service *monitoring.Service, resource *monitoring.MonitoredResource, metric *monitoring.Metric) (int64, error) {
	request := service.Projects.TimeSeries.
		List("projects/prometheus-to-sd").
		Filter(fmt.Sprintf("resource.type=\"%v\" metric.type=\"%v\"", resource.Type, metric.Type)).
		AggregationAlignmentPeriod("300s").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		IntervalEndTime(time.Now().Format(time.RFC3339))
	glog.V(2).Infof("ListTimeSeriesRequest: %v", request)
	response, err := request.Do()
	if err != nil {
		return 0, err
	}
	glog.V(2).Infof("ListTimeSeriesResponse: %v", response)
	if len(response.TimeSeries) != 1 {
		return 0, errors.New(fmt.Sprintf("Expected 1 time series, got %v", response.TimeSeries))
	}
	timeSeries := response.TimeSeries[0]
	if len(timeSeries.Points) != 1 {
		return 0, errors.New(fmt.Sprintf("Expected 1 point, got %v", timeSeries))
	}
	return ValueAsInt64(timeSeries.Points[0].Value), nil
}

func main() {
	flag.Parse()
	testInstanceId := getTestInstanceId()
	client, err := google.DefaultClient(
		context.Background(), "https://www.googleapis.com/auth/monitoring.read")
	if err != nil {
		glog.Fatalf("Failed to get Google OAuth2 credentials: %v", err)
	}
	stackdriverService, err := monitoring.New(client)
	if err != nil {
		glog.Fatalf("Failed to create Stackdriver client: %v", err)
	}
	glog.V(4).Infof("Successfully created Stackdriver client")
	value, err := queryStackdriverMetric(
		stackdriverService,
		&monitoring.MonitoredResource{
			Type: "gke_container",
			Labels: map[string]string{
				"cluster_name": "quickstart-cluster-1",
				"namespace_id": "default",
				"instance_id":  testInstanceId,
				"pod_id":       "echo",
				"container":    "",
				"location":     "us-central1-a",
			},
		}, &monitoring.Metric{
			Type: "custom.googleapis.com/web-echo/process_start_time_seconds",
		})
	if err != nil {
		glog.Fatalf("Failed to fetch metric: %v", err)
	}
	glog.Infof("Got value: %v", value)
}
