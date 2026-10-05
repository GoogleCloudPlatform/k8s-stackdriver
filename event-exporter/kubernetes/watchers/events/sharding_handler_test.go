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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func eventForUID(uid types.UID) *corev1.Event {
	return &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			UID: uid,
		},
	}
}

func TestShardingHandlerFiltersByInvolvedObjectUID(t *testing.T) {
	const ownedUID = types.UID("owned-uid")
	owns := func(uid types.UID) bool { return uid == ownedUID }

	testCases := []struct {
		desc     string
		uid      types.UID
		expected bool
	}{
		{"owned event is forwarded", ownedUID, true},
		{"foreign event is dropped", "foreign-uid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			addTriggered := false
			updateTriggered := false
			deleteTriggered := false
			handler := NewShardingHandler(&fakeEventHandler{
				onAddFunc:    func(*corev1.Event) { addTriggered = true },
				onUpdateFunc: func(*corev1.Event, *corev1.Event) { updateTriggered = true },
				onDeleteFunc: func(*corev1.Event) { deleteTriggered = true },
			}, owns)

			handler.OnAdd(eventForUID(tc.uid))
			handler.OnUpdate(nil, eventForUID(tc.uid))
			handler.OnDelete(eventForUID(tc.uid))

			if addTriggered != tc.expected {
				t.Errorf("Add is triggered = %v, expected %v", addTriggered, tc.expected)
			}
			if updateTriggered != tc.expected {
				t.Errorf("Update is triggered = %v, expected %v", updateTriggered, tc.expected)
			}
			if deleteTriggered != tc.expected {
				t.Errorf("Delete is triggered = %v, expected %v", deleteTriggered, tc.expected)
			}
		})
	}
}
