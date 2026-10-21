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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// shardingHandler forwards only the events owned by this shard to the
// delegate handler. Events are sharded by the involved object's UID: for pod
// events that is the pod UID, so an event is always handled by the same shard
// that caches the pod's metadata for owner label lookup.
type shardingHandler struct {
	delegate EventHandler
	owns     func(types.UID) bool
}

// NewShardingHandler wraps delegate so that only events whose involved
// object UID satisfies owns are passed through.
func NewShardingHandler(delegate EventHandler, owns func(types.UID) bool) EventHandler {
	return &shardingHandler{
		delegate: delegate,
		owns:     owns,
	}
}

func (s *shardingHandler) OnAdd(event *corev1.Event) {
	if !s.owns(event.InvolvedObject.UID) {
		recordShardingFilteredEvent()
		return
	}
	s.delegate.OnAdd(event)
}

func (s *shardingHandler) OnUpdate(oldEvent *corev1.Event, newEvent *corev1.Event) {
	if !s.owns(newEvent.InvolvedObject.UID) {
		recordShardingFilteredEvent()
		return
	}
	s.delegate.OnUpdate(oldEvent, newEvent)
}

func (s *shardingHandler) OnDelete(event *corev1.Event) {
	if !s.owns(event.InvolvedObject.UID) {
		recordShardingFilteredEvent()
		return
	}
	s.delegate.OnDelete(event)
}
