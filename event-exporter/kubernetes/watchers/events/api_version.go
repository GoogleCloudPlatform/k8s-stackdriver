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

import "fmt"

// EventsAPIVersion defines the Events API group/version to watch.
type EventsAPIVersion string

const (
	AutoEventsAPIVersion     EventsAPIVersion = "auto"
	CoreV1EventsAPIVersion   EventsAPIVersion = "core/v1"
	EventsV1EventsAPIVersion EventsAPIVersion = "events.k8s.io/v1"
)

func ParseEventsAPIVersion(value string) (EventsAPIVersion, error) {
	switch EventsAPIVersion(value) {
	case AutoEventsAPIVersion, CoreV1EventsAPIVersion, EventsV1EventsAPIVersion:
		return EventsAPIVersion(value), nil
	default:
		return "", fmt.Errorf("unsupported events API version: %s", value)
	}
}
