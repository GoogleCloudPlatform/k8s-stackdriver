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

	corev1 "k8s.io/api/core/v1"
)

type fakeEventHandler struct {
	onAddFunc    func(*corev1.Event)
	onUpdateFunc func(*corev1.Event, *corev1.Event)
	onDeleteFunc func(*corev1.Event)
}

func (c *fakeEventHandler) OnAdd(event *corev1.Event) {
	if c.onAddFunc != nil {
		c.onAddFunc(event)
	}
}

func (c *fakeEventHandler) OnUpdate(oldEvent, newEvent *corev1.Event) {
	if c.onUpdateFunc != nil {
		c.onUpdateFunc(oldEvent, newEvent)
	}
}

func (c *fakeEventHandler) OnDelete(event *corev1.Event) {
	if c.onDeleteFunc != nil {
		c.onDeleteFunc(event)
	}
}

func TestEventWatchHandlerAdd(t *testing.T) {
	testCases := []struct {
		desc     string
		obj      interface{}
		expected bool
	}{
		{
			"obj=nil",
			nil,
			false,
		},
		{
			"obj=non-event",
			42,
			false,
		},
		{
			"obj=event",
			&corev1.Event{},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			isTriggered := false
			fakeHandler := &fakeEventHandler{
				onAddFunc: func(*corev1.Event) { isTriggered = true },
			}

			c := newEventHandlerWrapper(fakeHandler)
			c.OnAdd(tc.obj)

			if isTriggered != tc.expected {
				t.Fatalf("Add is triggered = %v, expected %v", isTriggered, tc.expected)
			}
		})
	}
}

func TestEventWatchHandlerUpdate(t *testing.T) {
	testCases := []struct {
		desc     string
		oldObj   interface{}
		newObj   interface{}
		expected bool
	}{
		{
			"oldObj=nil,newObj=event",
			nil,
			&corev1.Event{},
			true,
		},
		{
			"oldObj=non-event,newObj=event",
			42,
			&corev1.Event{},
			false,
		},
		{
			"oldObj=event,newObj=nil",
			&corev1.Event{},
			nil,
			false,
		},
		{
			"oldObj=event,newObj=non-event",
			&corev1.Event{},
			42,
			false,
		},
		{
			"oldObj=event,newObj=event",
			&corev1.Event{},
			&corev1.Event{},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			isTriggered := false
			fakeHandler := &fakeEventHandler{
				onUpdateFunc: func(*corev1.Event, *corev1.Event) { isTriggered = true },
			}

			c := newEventHandlerWrapper(fakeHandler)
			c.OnUpdate(tc.oldObj, tc.newObj)

			if isTriggered != tc.expected {
				t.Fatalf("Update is triggered = %v, expected %v", isTriggered, tc.expected)
			}
		})
	}
}

func TestEventWatchHandlerDelete(t *testing.T) {
	testCases := []struct {
		desc     string
		obj      interface{}
		expected bool
	}{
		{
			"obj=nil",
			nil,
			false,
		},
		{
			"obj=non-event",
			42,
			false,
		},
		{
			"obj=event",
			&corev1.Event{},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			isTriggered := false
			fakeHandler := &fakeEventHandler{
				onDeleteFunc: func(*corev1.Event) { isTriggered = true },
			}

			c := newEventHandlerWrapper(fakeHandler)
			c.OnDelete(tc.obj)

			if isTriggered != tc.expected {
				t.Fatalf("Delete is triggered = %v, expected %v", isTriggered, tc.expected)
			}
		})
	}
}
