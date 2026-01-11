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

package events

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEventV1ToCoreEvent(t *testing.T) {
	now := time.Now()
	lastObserved := metav1.NewMicroTime(now.Add(10 * time.Second))

	event := &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-1",
			Namespace: "default",
		},
		EventTime:           metav1.NewMicroTime(now),
		Series:              &eventsv1.EventSeries{Count: 3, LastObservedTime: lastObserved},
		ReportingController: "kubelet",
		ReportingInstance:   "node-1",
		Action:              "Pulling",
		Reason:              "Pulled",
		Type:                "Normal",
		Regarding: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
		},
		Related: &corev1.ObjectReference{
			Kind: "Node",
			Name: "node-1",
		},
		Note:                     "pulled image",
		DeprecatedSource:         corev1.EventSource{Component: "kubelet"},
		DeprecatedFirstTimestamp: metav1.NewTime(now.Add(-time.Minute)),
		DeprecatedLastTimestamp:  metav1.NewTime(now),
		DeprecatedCount:          5,
	}

	got := eventV1ToCoreEvent(event)

	if got.Kind != "Event" || got.APIVersion != corev1.SchemeGroupVersion.String() {
		t.Fatalf("TypeMeta = %s/%s, expected %s/%s", got.APIVersion, got.Kind, corev1.SchemeGroupVersion.String(), "Event")
	}
	if got.InvolvedObject.Name != "test-pod" {
		t.Fatalf("InvolvedObject.Name = %s, expected %s", got.InvolvedObject.Name, "test-pod")
	}
	if got.Message != "pulled image" {
		t.Fatalf("Message = %s, expected %s", got.Message, "pulled image")
	}
	if got.Source.Component != "kubelet" {
		t.Fatalf("Source.Component = %s, expected %s", got.Source.Component, "kubelet")
	}
	if got.LastTimestamp.IsZero() {
		t.Fatalf("LastTimestamp should be set from deprecated fields")
	}
	if got.EventTime.IsZero() {
		t.Fatalf("EventTime should be set")
	}
	if got.Series == nil || got.Series.Count != 3 || !got.Series.LastObservedTime.Equal(&lastObserved) {
		t.Fatalf("Series not converted correctly")
	}
	if got.Count != 5 {
		t.Fatalf("Count = %d, expected %d", got.Count, 5)
	}
}

func TestEventV1ToCoreEventSeriesCountFallback(t *testing.T) {
	event := &eventsv1.Event{
		Series: &eventsv1.EventSeries{Count: 7},
	}

	got := eventV1ToCoreEvent(event)
	if got.Count != 7 {
		t.Fatalf("Count = %d, expected %d", got.Count, 7)
	}
}
