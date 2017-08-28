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
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/types"
	stackdriver "google.golang.org/api/monitoring/v3"
)

type sdService stackdriver.Service

//TODO(erocchi) add integration with Stackdriver

// StackdriverProvider implements EventsProvider
type StackdriverProvider struct {
	//TODO(erocchi) Define fields for this struct. It should contain what will be needed to communicate with Stackdriver.
}

// NewStackdriverProvider creates a new Provider with standard settings
func NewStackdriverProvider() provider.EventsProvider {
	return &StackdriverProvider{}
}

// GetNamespacedEventsByName gets the event with the given name
func (p *StackdriverProvider) GetNamespacedEventsByName(namespace, eventName string) (*types.EventValue, error) {
	return nil, fmt.Errorf("GetNamespacedEventsByName is not implemented yet.")
}

// ListAllEventsByNamespace gets all the events
func (p *StackdriverProvider) ListAllEventsByNamespace(namespace string) (*types.EventValueList, error) {
	return nil, fmt.Errorf("ListAllEventsByNamespace is not implemented yet.")
}
