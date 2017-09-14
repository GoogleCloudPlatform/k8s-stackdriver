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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/handlers"
)

type eventsResourceLister struct {
	provider EventsProvider
}

// NewResourceLister creates a new APIResourceLister for provided EventsProvider
func NewResourceLister(provider EventsProvider) handlers.APIResourceLister {
	return &eventsResourceLister{
		provider: provider,
	}
}

// ListAPIResources lists the supported API paths
func (l *eventsResourceLister) ListAPIResources() []metav1.APIResource {
	resources := make([]metav1.APIResource, 3)
	resources[0] = metav1.APIResource{
		Name:       "namespaces/{namespace}/events/{eventName}",
		Namespaced: true,
		Kind:       "Event",
		Verbs:      metav1.Verbs{"get"},
	}

	resources[1] = metav1.APIResource{
		Name:       "namespaces/{namespace}/events",
		Namespaced: true,
		Kind:       "EventList",
		Verbs:      metav1.Verbs{"get", "post"},
	}

	resources[2] = metav1.APIResource{
		Name:       "events",
		Namespaced: true,
		Kind:       "EventList",
		Verbs:      metav1.Verbs{"get"},
	}

	return resources
}
