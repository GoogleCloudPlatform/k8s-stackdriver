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
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/watchers"
)

const (
	// Since events live in the apiserver only for 1 hour, we have to remove
	// old objects to avoid memory leaks. If TTL is exactly 1 hour, race
	// can occur in case of the event being updated right before the end of
	// the hour, since it takes some time to deliver this event via watch.
	// 2 hours ought to be enough for anybody.
	eventStorageTTL = 2 * time.Hour

	// Large clusters can have up to 1M events. Fetching them using default
	// 500 page requires 2000 requests and is not able to finish before
	// continuation token will expire.
	// Value 10000 translates to ~100 requests that each takes 0.5s-1s,
	// so in total listing should take ~1m, which is still below 2.5m-5m
	// token expiration time.
	eventWatchListPageSize = 10000
)

// OnListFunc represent an action on the initial list of object received
// from the Kubernetes API server before starting watching for the updates.
type OnListFunc func(*corev1.EventList)

// EventWatcherConfig represents the configuration for the watcher that
// only watches the events resource.
type EventWatcherConfig struct {
	// Note, that this action will be executed on each List request, of which
	// there can be many, e.g. because of network problems. Note also, that
	// items in the List response WILL NOT trigger OnAdd method in handler,
	// instead Store contents will be completely replaced.
	OnList                       OnListFunc
	ResyncPeriod                 time.Duration
	Handler                      EventHandler
	EventLabelSelector           labels.Selector
	ListerWatcherOptionsLimit    int64
	ListerWatcherEnableStreaming bool
	StorageType                  watchers.StorageType
}

// NewEventWatcher create a new watcher that only watches the events resource.
func NewEventWatcher(client kubernetes.Interface, config *EventWatcherConfig) watchers.Watcher {
	return watchers.NewWatcher(&watchers.WatcherConfig{
		// List and watch events in all namespaces.
		ListerWatcher: &cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				if config.ListerWatcherEnableStreaming {
					return streamingListEvents(client, config, options)
				} else {
					if config.ListerWatcherOptionsLimit > 0 {
						options.Limit = config.ListerWatcherOptionsLimit
					}
					options.LabelSelector = config.EventLabelSelector.String()
					list, err := client.CoreV1().Events(meta_v1.NamespaceAll).List(context.TODO(), options)
					if err == nil {
						config.OnList(list)
					}
					return list, err
				}
			},

			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				options.LabelSelector = config.EventLabelSelector.String()
				return client.CoreV1().Events(meta_v1.NamespaceAll).Watch(context.TODO(), options)
			},
		},
		ExpectedType: &corev1.Event{},
		StoreConfig: &watchers.WatcherStoreConfig{
			KeyFunc:     cache.DeletionHandlingMetaNamespaceKeyFunc,
			Handler:     newEventHandlerWrapper(config.Handler),
			StorageType: config.StorageType,
			StorageTTL:  eventStorageTTL,
		},
		ResyncPeriod:      config.ResyncPeriod,
		WatchListPageSize: eventWatchListPageSize,
	})
}

// streamingListEvents uses Streaming List (SendInitialEvents=true) to avoid buffering.
// This allows us to process initial events incrementally.
func streamingListEvents(client kubernetes.Interface, config *EventWatcherConfig, options meta_v1.ListOptions) (runtime.Object, error) {
	sendInitialEvents := true
	options.SendInitialEvents = &sendInitialEvents
	options.ResourceVersionMatch = meta_v1.ResourceVersionMatchNotOlderThan
	options.Watch = true
	options.LabelSelector = config.EventLabelSelector.String()
	options.AllowWatchBookmarks = true

	// Perform the streaming list (actually a Watch)
	watcher, err := client.CoreV1().Events(meta_v1.NamespaceAll).Watch(context.TODO(), options)
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	// Call OnList once to start the sink (it just logs "Started watching")
	config.OnList(&corev1.EventList{})

	lastRV := ""
	for event := range watcher.ResultChan() {
		if meta, ok := event.Object.(meta_v1.Object); ok {
			lastRV = meta.GetResourceVersion()
		}

		switch event.Type {
		case watch.Added:
			if e, ok := event.Object.(*corev1.Event); ok {
				// Manually pass to handler since we bypass Reflector's store
				config.Handler.OnAdd(e)
			}
		case watch.Bookmark:
			// Check for the annotation that signals the initial list is done.
			if m, ok := event.Object.(meta_v1.Object); ok {
				if val, ok := m.GetAnnotations()["k8s.io/initial-events-end"]; ok && val == "true" {
					// Stop the watcher to close the channel and break the loop
					watcher.Stop()
				}
			}
		case watch.Error:
			// If we get an error, Reflector will retry ListFunc anyway.
			// We can return the error here to trigger that retry.
			if status, ok := event.Object.(*meta_v1.Status); ok {
				return nil, errors.New(status.Message)
			}
		}
	}

	// Return an empty list with the correct ResourceVersion.
	// Reflector will then start Watching from this version.
	return &corev1.EventList{
		ListMeta: meta_v1.ListMeta{
			ResourceVersion: lastRV,
		},
		Items: []corev1.Event{},
	}, nil
}
