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
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"golang.org/x/oauth2/google"
	monitoring "google.golang.org/api/monitoring/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	clusterName = flag.String("cluster-name", "", "the name of the cluster where to launch the testing environment."+
		" If empty, a random one will be used, and -cluster-create must be used")
	integration = flag.Bool("integration", false, "whether to run integration tests")
)

func getProjectId() string {
	return "prometheus-to-sd"
}

func getClusterZone() string { return "us-central1-a" }

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

// GetPodInstanceId returns the instance id used in Stackdriver Monitored Resources for the given namespace and pod name.
func getPodInstanceId(namespaceName string, podName string) string {
	pod, err := GetClientset().CoreV1().Pods(namespaceName).Get(podName, metav1.GetOptions{})
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
	var value int64 = 0
	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.InitialInterval = 10 * time.Second
	err := backoff.Retry(
		func() error {
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
				return backoff.Permanent(err)
			}
			log.Printf("ListTimeSeriesResponse: %v", response)
			if len(response.TimeSeries) != 1 {
				return errors.New(fmt.Sprintf("Expected 1 time series, got %v", response.TimeSeries))
			}
			timeSeries := response.TimeSeries[0]
			if len(timeSeries.Points) != 1 {
				return errors.New(fmt.Sprintf("Expected 1 point, got %v", timeSeries))
			}
			value = ValueAsInt64(timeSeries.Points[0].Value)
			return nil
		}, backoffPolicy)
	return value, err
}

func execKubectl(args ...string) error {
	// TODO(jkohen): log stdout and stderr
	kubectlPath := "kubectl" // Assume in PATH
	cmd := exec.Command(kubectlPath, args...)
	human := strings.Join(cmd.Args, " ")
	log.Printf("Running command: %s", human)
	return cmd.Run()
}

func TestE2E(t *testing.T) {
	if !*integration {
		t.Skip("skipping integration test: disabled")
	}
	if *clusterName == "" {
		t.Fatalf("the cluster name must not be empty")
	}
	log.Printf("Cluster name: %s", *clusterName)
	namespaceName := fmt.Sprintf("e2e-%x", rand.Uint64())
	log.Printf("Namespace name: %s", namespaceName)
	if err := execKubectl("create", "namespace", namespaceName); err != nil {
		t.Fatalf("Failed to run kubectl: %v", err)
	}
	if err := execKubectl("apply", "--namespace", namespaceName, "-f", "e2e.yaml"); err != nil {
		t.Fatalf("Failed to run kubectl: %v", err)
	}
	defer func() {
		if err := execKubectl("delete", "namespace", namespaceName); err != nil {
			t.Fatalf("Failed to run kubectl: %v", err)
		}
	}()
	k8sInstanceId := getPodInstanceId(namespaceName, "echo")
	t.Run("gke_container", func(t *testing.T) {
		client, err := google.DefaultClient(
			context.Background(), monitoring.MonitoringReadScope)
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
					"cluster_name":   *clusterName,
					"namespace_id":   namespaceName,
					"instance_id":    k8sInstanceId,
					"pod_id":         "echo",
					"container_name": "",
					"zone":           getClusterZone(),
				},
			}, &monitoring.Metric{
				Type: "custom.googleapis.com/web-echo/process_start_time_seconds",
			})
		if err != nil {
			t.Fatalf("Failed to fetch metric: %v", err)
		}
		log.Printf("Got value: %v", value)
	})
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
