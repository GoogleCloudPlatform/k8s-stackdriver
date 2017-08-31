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
	gce "cloud.google.com/go/compute/metadata"
	"fmt"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	sd "google.golang.org/api/logging/v2"
	api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"time"
)

//TODO(erocchi) add implementation to write to Stackdriver
//TODO(erocchi) Use the Kubernetes AIP - compatible errors, see example definitions here: https://github.com/kubernetes-incubator/custom-metrics-apiserver/blob/master/pkg/provider/errors.go

// StackdriverProvider implements EventsProvider
type StackdriverProvider struct {
	restClient         rest.Interface
	service            *sd.Service
	maxRetrievedEvents int
	sinceMillis        int64
}

// NewStackdriverProvider creates a new Provider with standard settings
func NewStackdriverProvider(restClient rest.Interface, stackdriverService *sd.Service, maxRetrievedEvents int, sinceMillis int64) provider.EventsProvider {
	return &StackdriverProvider{
		restClient:         restClient,
		service:            stackdriverService,
		maxRetrievedEvents: maxRetrievedEvents,
		sinceMillis:        sinceMillis,
	}
}

// GetNamespacedEventsByName gets the event with the given namespace and name
func (p *StackdriverProvider) GetNamespacedEventsByName(namespace, eventName string) (*api.Event, error) {

	standardFilter, err := standardFilter()
	if err != nil {
		return nil, err
	}
	filter := "(" + standardFilter + " AND jsonPayload.metadata.namespace = \"" + namespace + "\" AND jsonPayload.metadata.name = \"" + eventName + "\")"
	event, err := p.doRequest(filter)
	if err != nil {
		return nil, err
	}

	if len(event.Items) == 0 {
		return nil, fmt.Errorf("Event not found")
	}
	return &event.Items[0], nil
}

// ListAllEventsByNamespace gets all the events with the given namespace
func (p *StackdriverProvider) ListAllEventsByNamespace(namespace string) (*api.EventList, error) {

	standardFilter, err := standardFilter()
	if err != nil {
		return nil, err
	}

	filter := "(" + standardFilter + " AND jsonPayload.metadata.namespace = \"" + namespace + "\")"
	if err != nil {
		return nil, err
	}

	return p.doRequest(filter)
}

// ListAllEvents gets all the events
func (p *StackdriverProvider) ListAllEvents() (*api.EventList, error) {
	filter, err := standardFilter()
	if err != nil {
		return nil, err
	}
	return p.doRequest(filter)
}

func standardFilter() (string, error) {
	projectID, err := gce.ProjectID()

	if err != nil {
		return "", fmt.Errorf("Cannot retrieve projectID. %v", err)
	}

	return "(logName = \"projects/" + projectID + "/logs/events\" AND jsonPayload.kind = \"Event\")", nil

}

// CreateNewEvent creates a new event in the given namespace
func (p *StackdriverProvider) CreateNewEvent(namespace string) (*api.Event, error) {
	return nil, fmt.Errorf("CreateNewEvent is not implemented yet.")
}

func (p *StackdriverProvider) doRequest(filter string) (*api.EventList, error) {
	projectID, err := gce.ProjectID()

	if err != nil {
		return nil, fmt.Errorf("Cannot retrieve projectID. %v", err)
	}

	resource := []string{
		"projects/" + projectID,
	}

	entriesService := sd.NewEntriesService(p.service)
	translator := newEventsTranslator(p.maxRetrievedEvents)

	var timeLimit time.Time
	if p.sinceMillis < 0 {
		timeLimit = time.Now().Add((-3600) * time.Second)
	} else {
		timeLimit = time.Unix(0, p.sinceMillis*1e6)
	}
	timeFilter := "(" + filter + " AND " + "jsonPayload.metadata.creationTimestamp >= \"" + timeLimit.Format("2006-01-02T15:04:02.002Z") + "\")"

	eventsList := &api.EventList{}
	var response *sd.ListLogEntriesResponse

	for redirection := 0; redirection < 1000; redirection++ {
		request := &sd.ListLogEntriesRequest{
			ResourceNames: resource,
			Filter:        timeFilter,
			OrderBy:       "timestamp desc",
		}
		if redirection != 0 {
			request.PageToken = response.NextPageToken
		}
		response, err = entriesService.List(request).Do()

		if err != nil {
			return nil, fmt.Errorf("Impossible to get the answer:  %v", err)
		}

		eventsList, err = translator.translateToEventList(response)

		if err != nil {
			return nil, fmt.Errorf("Impossible to translate the answer:  %v", err)
		}

		if response.NextPageToken == "" || len(eventsList.Items) >= p.maxRetrievedEvents {
			break
		}
	}

	return eventsList, nil
}
