/*
Copyright 2026 Google Inc.

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

// Package local provides a sink that writes exported events as JSON lines to
// stdout. It is meant for testing the export pipeline (e.g. in a kind
// cluster) without Stackdriver access; each line is prefixed with
// EXPORTED_EVENT so it can be extracted from the pod logs.
package local

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/podlabels"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/sinks"
)

const exportedEventPrefix = "EXPORTED_EVENT"

type exportedEvent struct {
	Action          string            `json:"action"`
	EventUID        string            `json:"eventUID"`
	EventNamespace  string            `json:"eventNamespace"`
	EventName       string            `json:"eventName"`
	ResourceVersion string            `json:"resourceVersion"`
	Count           int32             `json:"count"`
	Reason          string            `json:"reason"`
	InvolvedKind    string            `json:"involvedKind"`
	InvolvedName    string            `json:"involvedName"`
	InvolvedUID     string            `json:"involvedUID"`
	PodLabels       map[string]string `json:"podLabels,omitempty"`
}

type localSink struct {
	podLabelCollector podlabels.PodLabelCollector

	mu sync.Mutex
}

// NewFactory creates a factory for the local testing sink.
func NewFactory() sinks.SinkFactory {
	return &localSinkFactory{}
}

type localSinkFactory struct{}

func (f *localSinkFactory) CreateNew(opts []string, podLabelCollector podlabels.PodLabelCollector) (sinks.Sink, error) {
	return &localSink{
		podLabelCollector: podLabelCollector,
	}, nil
}

func (s *localSink) OnAdd(event *corev1.Event) {
	s.export("ADD", event)
}

func (s *localSink) OnUpdate(_ *corev1.Event, newEvent *corev1.Event) {
	s.export("UPDATE", newEvent)
}

func (s *localSink) OnDelete(*corev1.Event) {
	// Deletions are not exported, matching the Stackdriver sink.
}

func (s *localSink) OnList(*corev1.EventList) {
	glog.Info("Local sink received list, started watching")
}

func (s *localSink) Run(stopCh <-chan struct{}) {
	glog.Info("Starting local sink")
	<-stopCh
	glog.Info("Local sink received stop signal")
}

func (s *localSink) export(action string, event *corev1.Event) {
	exported := exportedEvent{
		Action:          action,
		EventUID:        string(event.UID),
		EventNamespace:  event.Namespace,
		EventName:       event.Name,
		ResourceVersion: event.ResourceVersion,
		Count:           event.Count,
		Reason:          event.Reason,
		InvolvedKind:    event.InvolvedObject.Kind,
		InvolvedName:    event.InvolvedObject.Name,
		InvolvedUID:     string(event.InvolvedObject.UID),
	}
	// Enrich pod events with owner labels the same way the Stackdriver
	// sink does, so label lookup can be tested end to end.
	if event.InvolvedObject.Kind == "Pod" && s.podLabelCollector != nil {
		exported.PodLabels = s.podLabelCollector.GetLabels(event.InvolvedObject.Namespace, event.InvolvedObject.Name)
	}
	line, err := json.Marshal(exported)
	if err != nil {
		glog.Warningf("Failed to marshal exported event %+v: %v", exported, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Printf("%s %s\n", exportedEventPrefix, line)
}
