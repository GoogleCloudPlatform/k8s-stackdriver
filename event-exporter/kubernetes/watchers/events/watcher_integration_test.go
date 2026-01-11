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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/watchers"
)

type recordingHandler struct {
	addCh    chan *corev1.Event
	updateCh chan *corev1.Event
	deleteCh chan *corev1.Event
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{
		addCh:    make(chan *corev1.Event, 1),
		updateCh: make(chan *corev1.Event, 1),
		deleteCh: make(chan *corev1.Event, 1),
	}
}

func (h *recordingHandler) OnAdd(event *corev1.Event) {
	h.addCh <- event
}

func (h *recordingHandler) OnUpdate(_, newEvent *corev1.Event) {
	h.updateCh <- newEvent
}

func (h *recordingHandler) OnDelete(event *corev1.Event) {
	h.deleteCh <- event
}

func TestWatcherProcessesEventsV1Objects(t *testing.T) {
	handler := newRecordingHandler()
	fakeWatch := watch.NewFakeWithChanSize(5, false)
	listCalled := make(chan struct{}, 1)

	lw := &cache.ListWatch{
		ListFunc: func(_ metav1.ListOptions) (runtime.Object, error) {
			listCalled <- struct{}{}
			return &eventsv1.EventList{}, nil
		},
		WatchFunc: func(_ metav1.ListOptions) (watch.Interface, error) {
			return fakeWatch, nil
		},
	}

	w := watchers.NewWatcher(&watchers.WatcherConfig{
		ListerWatcher: lw,
		ExpectedType:  &eventsv1.Event{},
		StoreConfig: &watchers.WatcherStoreConfig{
			KeyFunc:     cache.DeletionHandlingMetaNamespaceKeyFunc,
			Handler:     newEventHandlerWrapper(handler),
			StorageType: watchers.DeltaFIFOStorage,
		},
		ResyncPeriod: 0,
	})

	stopCh := make(chan struct{})
	defer close(stopCh)
	defer fakeWatch.Stop()

	go w.Run(stopCh)

	select {
	case <-listCalled:
	case <-time.After(2 * time.Second):
		t.Fatalf("list call did not happen")
	}

	event := &eventsv1.Event{
		Regarding: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
		},
		Reason:    "Scheduled",
		Note:      "pod scheduled",
		EventTime: metav1.NewMicroTime(time.Now()),
	}

	fakeWatch.Add(event)
	select {
	case got := <-handler.addCh:
		if got.InvolvedObject.Name != "test-pod" {
			t.Fatalf("InvolvedObject.Name = %s, expected %s", got.InvolvedObject.Name, "test-pod")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("add event not received")
	}

	fakeWatch.Modify(event)
	select {
	case <-handler.updateCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("update event not received")
	}

	fakeWatch.Delete(event)
	select {
	case <-handler.deleteCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("delete event not received")
	}
}
