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

package events

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/watchers"
)

type mockEventInterface struct {
	corev1client.EventInterface
	listFunc  func(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error)
	watchFunc func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

func (m *mockEventInterface) List(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, opts)
	}
	return &corev1.EventList{}, nil
}

func (m *mockEventInterface) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	if m.watchFunc != nil {
		return m.watchFunc(ctx, opts)
	}
	return watch.NewFake(), nil
}

type mockCoreV1 struct {
	corev1client.CoreV1Interface
	events corev1client.EventInterface
}

func (m *mockCoreV1) Events(namespace string) corev1client.EventInterface {
	return m.events
}

func (m *mockCoreV1) RESTClient() rest.Interface {
	return nil
}

type mockKubeClient struct {
	kubernetes.Interface
	coreV1 corev1client.CoreV1Interface
}

func (m *mockKubeClient) CoreV1() corev1client.CoreV1Interface {
	return m.coreV1
}

func TestEventWatcherFieldAndLabelSelectors(t *testing.T) {
	testCases := []struct {
		desc               string
		fieldSelector      string
		labelSelector      string
		expectedFieldParam string
		expectedLabelParam string
	}{
		{
			desc:               "no selectors",
			fieldSelector:      "",
			labelSelector:      "",
			expectedFieldParam: "",
			expectedLabelParam: "",
		},
		{
			desc:               "field selector only",
			fieldSelector:      "type=Warning,involvedObject.kind=Pod",
			labelSelector:      "",
			expectedFieldParam: "involvedObject.kind=Pod,type=Warning", // sorted by fields.ParseSelector
			expectedLabelParam: "",
		},
		{
			desc:               "both field and label selectors",
			fieldSelector:      "type=Normal",
			labelSelector:      "app=test",
			expectedFieldParam: "type=Normal",
			expectedLabelParam: "app=test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			parsedField, err := fields.ParseSelector(tc.fieldSelector)
			if err != nil {
				t.Fatalf("Failed to parse field selector: %v", err)
			}
			parsedLabel, err := labels.Parse(tc.labelSelector)
			if err != nil {
				t.Fatalf("Failed to parse label selector: %v", err)
			}

			var capturedListOptions metav1.ListOptions
			var capturedWatchOptions metav1.ListOptions

			mockEvents := &mockEventInterface{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error) {
					capturedListOptions = opts
					return &corev1.EventList{}, nil
				},
				watchFunc: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
					capturedWatchOptions = opts
					return watch.NewFake(), nil
				},
			}

			client := &mockKubeClient{
				coreV1: &mockCoreV1{
					events: mockEvents,
				},
			}

			config := &EventWatcherConfig{
				OnList:             func(*corev1.EventList) {},
				ResyncPeriod:       time.Minute,
				Handler:            &fakeEventHandler{},
				EventLabelSelector: parsedLabel,
				EventFieldSelector: parsedField,
				StorageType:        watchers.SimpleStorage,
			}

			lw := createEventListerWatcher(client, config)

			// Test ListFunc
			_, err = lw.List(metav1.ListOptions{})
			if err != nil {
				t.Fatalf("ListFunc failed: %v", err)
			}
			if capturedListOptions.FieldSelector != tc.expectedFieldParam {
				t.Errorf("ListFunc FieldSelector = %q, want %q", capturedListOptions.FieldSelector, tc.expectedFieldParam)
			}
			if capturedListOptions.LabelSelector != tc.expectedLabelParam {
				t.Errorf("ListFunc LabelSelector = %q, want %q", capturedListOptions.LabelSelector, tc.expectedLabelParam)
			}

			// Test WatchFunc
			_, err = lw.Watch(metav1.ListOptions{})
			if err != nil {
				t.Fatalf("WatchFunc failed: %v", err)
			}
			if capturedWatchOptions.FieldSelector != tc.expectedFieldParam {
				t.Errorf("WatchFunc FieldSelector = %q, want %q", capturedWatchOptions.FieldSelector, tc.expectedFieldParam)
			}
			if capturedWatchOptions.LabelSelector != tc.expectedLabelParam {
				t.Errorf("WatchFunc LabelSelector = %q, want %q", capturedWatchOptions.LabelSelector, tc.expectedLabelParam)
			}
		})
	}
}

func TestStreamingListEventsFieldSelector(t *testing.T) {
	parsedField, err := fields.ParseSelector("type=Warning")
	if err != nil {
		t.Fatalf("Failed to parse field selector: %v", err)
	}

	var capturedOptions metav1.ListOptions
	fakeWatcher := watch.NewFake()

	mockEvents := &mockEventInterface{
		watchFunc: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			capturedOptions = opts
			go func() {
				time.Sleep(10 * time.Millisecond)
				bookmarkEvent := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "123",
						Annotations: map[string]string{
							"k8s.io/initial-events-end": "true",
						},
					},
				}
				fakeWatcher.Action(watch.Bookmark, bookmarkEvent)
			}()
			return fakeWatcher, nil
		},
	}

	client := &mockKubeClient{
		coreV1: &mockCoreV1{
			events: mockEvents,
		},
	}

	config := &EventWatcherConfig{
		OnList:             func(*corev1.EventList) {},
		EventLabelSelector: labels.Everything(),
		EventFieldSelector: parsedField,
		Handler:            &fakeEventHandler{},
	}

	_, err = streamingListEvents(client, config, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("streamingListEvents failed: %v", err)
	}

	if capturedOptions.FieldSelector != "type=Warning" {
		t.Errorf("streamingListEvents FieldSelector = %q, want %q", capturedOptions.FieldSelector, "type=Warning")
	}
}
