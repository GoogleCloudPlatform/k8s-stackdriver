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
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/sinks"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/utils"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/watchers"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/watchers/events"
)

type eventExporter struct {
	sink    sinks.Sink
	watcher watchers.Watcher
}

func (e *eventExporter) Run(stopCh <-chan struct{}) {
	utils.RunConcurrentlyUntil(stopCh, e.sink.Run, e.watcher.Run)
}

func newEventExporter(client kubernetes.Interface, sink sinks.Sink, resyncPeriod time.Duration) *eventExporter {
	return &eventExporter{
		sink:    sink,
		watcher: createWatcher(client, sink, resyncPeriod),
	}
}

func createWatcher(client kubernetes.Interface, sink sinks.Sink, resyncPeriod time.Duration) watchers.Watcher {
	return events.NewEventWatcher(client, &events.EventWatcherConfig{
		OnList:       sink.OnList,
		ResyncPeriod: resyncPeriod,
		Handler:      sink,
	})
}
