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
	"fmt"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/client-go/pkg/api"
)

//TODO(erocchi) Use the Kubernetes AIP - compatible errors, see example definitions here: https://github.com/kubernetes-incubator/custom-metrics-apiserver/blob/master/pkg/provider/errors.go

type sdService stackdriver.Service

// StackdriverProvider implements EventsProvider
type StackdriverProvider struct {
	//TODO(erocchi) Define fields for this struct. It should contain what will be needed to communicate with Stackdriver.
}

//TODO(erocchi) add implementation to communicate with Stackdriver

// NewStackdriverProvider creates a new Provider with standard settings
func NewStackdriverProvider() provider.EventsProvider {
	return &StackdriverProvider{}
}

// GetNamespacedEventsByName gets the event with the given namespace and name
func (p *StackdriverProvider) GetNamespacedEventsByName(namespace, eventName string) (*api.Event, error) {
	return nil, fmt.Errorf("GetNamespacedEventsByName is not implemented yet.")
}

// ListAllEventsByNamespace gets all the events with the given namespace
func (p *StackdriverProvider) ListAllEventsByNamespace(namespace string) (*api.EventList, error) {
	return nil, fmt.Errorf("ListAllEventsByNamespace is not implemented yet.")
}

// ListAllEvents gets all the events
func (p *StackdriverProvider) ListAllEvents() (*api.EventList, error) {
	return nil, fmt.Errorf("ListAllEvents is not implemented yet.")
}

// CreateNewEvent creates a new event in the given namespace
func (p *StackdriverProvider) CreateNewEvent(namespace string) (*api.Event, error) {
	return nil, fmt.Errorf("CreateNewEvent is not implemented yet.")
}
