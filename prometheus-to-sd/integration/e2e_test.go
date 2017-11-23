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

package integration

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2/google"
	monitoring "google.golang.org/api/monitoring/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // Load the GCP plugin (only required to authenticate against GKE clusters).
	"k8s.io/client-go/tools/clientcmd"
)

var integration = flag.Bool("integration", false, "run integration tests")

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getProjectId() string {
	return "prometheus-to-sd"
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
func getKubernetesInstanceId() string {
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
	log.Printf("Found pod nodename: %v", pod.Spec.NodeName)
	return fmt.Sprintf("%s.c.%s.internal", pod.Spec.NodeName, getProjectId())
}

func buildFilter(selector string, labels map[string]string) string {
	s := make([]string, len(labels))
	for k, v := range labels {
		s = append(s, fmt.Sprintf("%s.labels.%s=\"%s\"", selector, k, v))
	}
	return strings.Join(s, " ")
}

func queryStackdriverMetric(service *monitoring.Service, resource *monitoring.MonitoredResource, metric *monitoring.Metric) (int64, error) {
	request := service.Projects.TimeSeries.
		List(fmt.Sprintf("projects/%s", getProjectId())).
		Filter(fmt.Sprintf("resource.type=\"%s\" metric.type=\"%s\" %s %s", resource.Type, metric.Type,
			buildFilter("resource", resource.Labels), buildFilter("metric", metric.Labels))).
		AggregationAlignmentPeriod("300s").
		AggregationPerSeriesAligner("ALIGN_NEXT_OLDER").
		IntervalEndTime(time.Now().Format(time.RFC3339))
	log.Printf("ListTimeSeriesRequest: %v", request)
	response, err := request.Do()
	if err != nil {
		return 0, err
	}
	log.Printf("ListTimeSeriesResponse: %v", response)
	if len(response.TimeSeries) != 1 {
		return 0, errors.New(fmt.Sprintf("Expected 1 time series, got %v", response.TimeSeries))
	}
	timeSeries := response.TimeSeries[0]
	if len(timeSeries.Points) != 1 {
		return 0, errors.New(fmt.Sprintf("Expected 1 point, got %v", timeSeries))
	}
	return ValueAsInt64(timeSeries.Points[0].Value), nil
}

func TestE2E(t *testing.T) {
	if !*integration {
		t.Skip("skipping integration test: disabled")
	}
	// TODO(jkohen): add code to start and turn down a cluster
	k8sInstanceId := getKubernetesInstanceId()
	client, err := google.DefaultClient(
		context.Background(), "https://www.googleapis.com/auth/monitoring.read")
	if err != nil {
		t.Fatalf("Failed to get Google OAuth2 credentials: %v", err)
	}
	stackdriverService, err := monitoring.New(client)
	if err != nil {
		t.Fatalf("Failed to create Stackdriver client: %v", err)
	}
	log.Printf("Successfully created Stackdriver client")
	value, err := queryStackdriverMetric(
		stackdriverService,
		&monitoring.MonitoredResource{
			Type: "gke_container",
			Labels: map[string]string{
				"project_id":     getProjectId(),
				"cluster_name":   "quickstart-cluster-1",
				"namespace_id":   "default",
				"instance_id":    k8sInstanceId,
				"pod_id":         "echo",
				"container_name": "",
				"zone":           "us-central1-a",
			},
		}, &monitoring.Metric{
			Type: "custom.googleapis.com/web-echo/process_start_time_seconds",
		})
	if err != nil {
		t.Fatalf("Failed to fetch metric: %v", err)
	}
	log.Printf("Got value: %v", value)
}
