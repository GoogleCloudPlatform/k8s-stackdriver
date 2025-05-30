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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/podlabels"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/sinks/stackdriver"
)

var (
	resyncPeriod       = flag.Duration("resync-period", 1*time.Minute, "Reflector resync period")
	sinkOpts           = flag.String("sink-opts", "", "Parameters for configuring sink")
	prometheusEndpoint = flag.String("prometheus-endpoint", ":80", "Endpoint on which to "+
		"expose Prometheus http handler")
	systemNamespaces = flag.String("system-namespaces", "kube-system,gke-connect", "Comma "+
		"separated list of system namespaces to skip the owner label collection")

	enablePodOwnerLabel    = flag.Bool("enable-pod-owner-label", true, "Whether to enable the pod label collector to add pod owner labels to log entries")
	podCacheSize           = flag.Int("pod-label-cache-size", 2048, "When enable-pod-owner-label, the maximum number of pods in the label cache")
	emptyLabelPodCacheSize = flag.Int("pod-empty-label-cache-size", 4096, "When enable-pod-owner-label, the maximum number of cached pods with empty label, to prevent repeated checks")
	emptyLabelPodCacheTTL  = flag.Duration("pod-empty-label-cache-ttl", 2*time.Hour, "When enable-pod-owner-label, for pods with empty label, how long to keep their cache before checking again")
	getPodTimeout          = flag.Duration("pod-get-timeout", 3*time.Second, "When enable-pod-owner-label, the timeout when getting pod labels")
	eventLabelSelector     = flag.String("event-label-selector", "", "Export events only if they match the given label selector. Same syntax as kubectl label")
)

func newSystemStopChannel() chan struct{} {
	ch := make(chan struct{})
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		sig := <-c
		glog.Infof("Received signal %s, terminating", sig.String())

		// Close stop channel to make sure every goroutine will receive stop signal.
		close(ch)
	}()

	return ch
}

func newKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %v", err)
	}
	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}

func main() {
	flag.Set("logtostderr", "true")
	defer glog.Flush()
	flag.Parse()

	client, err := newKubernetesClient()
	if err != nil {
		glog.Fatalf("Failed to initialize kubernetes client: %v", err)
	}

	var podLabelCollector podlabels.PodLabelCollector = nil
	if *enablePodOwnerLabel {
		podLabelCollector, err = podlabels.NewCollector(client, strings.Split(*systemNamespaces, ","), *podCacheSize, *emptyLabelPodCacheSize, *emptyLabelPodCacheTTL, *getPodTimeout)
		if err != nil {
			glog.Fatalf("Failed to initialize pod label collector: %v", err)
		}
	}

	sink, err := stackdriver.NewSdSinkFactory().CreateNew(strings.Split(*sinkOpts, " "), podLabelCollector)
	if err != nil {
		glog.Fatalf("Failed to initialize sink: %v", err)
	}

	parsedLabelSelector, err := labels.Parse(*eventLabelSelector)
	if err != nil {
		glog.Fatalf("Invalid event label selector:%v", err)
	}
	eventExporter := newEventExporter(client, sink, *resyncPeriod, parsedLabelSelector)

	// Expose the Prometheus http endpoint
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		glog.Fatalf("Prometheus monitoring failed: %v", http.ListenAndServe(*prometheusEndpoint, nil))
	}()

	stopCh := newSystemStopChannel()
	eventExporter.Run(stopCh)
}
