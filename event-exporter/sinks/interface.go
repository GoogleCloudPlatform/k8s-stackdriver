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

package sinks

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/watchers/events"
)

// Sink interface represents a generic sink that is responsible for handling
// actions upon the event objects and filter the initial events list. Note,
// that OnAdd method from the EventHandler interface will only receive
// objects that were added during watching phase, not before. If sink wishes
// to process the latter additions, it should implement additional logic in
// the OnList method.
type Sink interface {
	events.EventHandler

	OnList(*corev1.EventList)

	Run(stopCh <-chan struct{})
}

// SinkFactory creates a new sink, using user-provided parameters.
type SinkFactory interface {
	CreateNew(opts []string) (Sink, error)
}
