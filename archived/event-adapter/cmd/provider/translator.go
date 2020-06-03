/*
Copyright 2017 The Kubernetes Authors.

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

package provider

import (
	"encoding/json"
	"fmt"
	sd "google.golang.org/api/logging/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "k8s.io/client-go/pkg/api/v1"
)

type eventTime struct {
	Timestamp metav1.Time `json:"timestamp,omitempty"`
}

func extractEvent(e *sd.LogEntry) (*api.Event, error) {
	a, err := e.JsonPayload.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("Cannot marshall the answer: %v", err)
	}
	var event api.Event
	err = json.Unmarshal(a, &event)
	if err != nil {
		return nil, fmt.Errorf("Cannot unmarshall the answer to an event: %v", err)
	}
	return &event, nil
}

func extractTime(e *sd.LogEntry) (*eventTime, error) {
	a, err := e.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("Cannot marshall the time: %v", err)
	}
	var time eventTime
	err = json.Unmarshal(a, &time)
	if err != nil {
		return nil, fmt.Errorf("Cannot unmarshall the time of the event: %v", err)
	}
	return &time, nil
}

func translateToEventList(response *sd.ListLogEntriesResponse, eventsList *api.EventList, eventIndex map[string]int, maxListLength int) (*api.EventList, map[string]int, error) {
	for _, value := range response.Entries {
		event := value
		newEvent, err := extractEvent(event)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to parse event : %v", err)
		}

		time, err := extractTime(event)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to parse timestamp : %v", err)
		}
		timestamp := time.Timestamp

		if ind, ok := eventIndex[newEvent.Name]; ok {
			item := &eventsList.Items[ind]
			if item.FirstTimestamp.After(timestamp.Time) {
				item.FirstTimestamp = timestamp
			}
			if item.LastTimestamp.Before(timestamp) {
				item.LastTimestamp = timestamp
			}
			item.Count++
		} else {
			newEvent.Count = 1
			newEvent.FirstTimestamp = timestamp
			newEvent.LastTimestamp = timestamp
			eventsList.Items = append(eventsList.Items, *newEvent)
			eventIndex[newEvent.Name] = len(eventsList.Items) - 1
		}
		if len(eventsList.Items) >= maxListLength {
			break
		}
	}
	return eventsList, eventIndex, nil
}
