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

package registry

import (
	"fmt"
	specificinstaller "github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/apiserver/installer/context"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/runtime"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	api "k8s.io/client-go/pkg/api/v1"
)

// REST is a wrapper for EventsProvider that provides implementation for Storage and Lister interfaces
type REST struct {
	evProvider provider.EventsProvider
}

var _ rest.Storage = &REST{}
var _ rest.Lister = &REST{}

// NewREST creates a new REST for the given EventsProvider
func NewREST(evProvider provider.EventsProvider) *REST {
	return &REST{
		evProvider: evProvider,
	}
}

// New implements Storage
func (r *REST) New() runtime.Object {
	return &api.Event{}
}

// NewList implement Lister
func (r *REST) NewList() runtime.Object {
	return &api.EventList{}
}

// List selects the events that match to the selector
func (r *REST) List(ctx genericapirequest.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {

	namespace := genericapirequest.NamespaceValue(ctx)
	if namespace == "" {
		return r.evProvider.ListAllEvents()
	}

	eventName, methodSingleEvent, ok := specificinstaller.RequestInformationFrom(ctx)
	methodListEvents, list := specificinstaller.RequestListInformationFrom(ctx)

	if methodListEvents == "" && methodSingleEvent == "" {
		return nil, fmt.Errorf("Unable to serve the request: forgiven method")
	}

	if list {
		return r.listEventsAPI(namespace, methodListEvents)
	}

	if ok {
		return r.singleEventAPI(namespace, eventName, methodSingleEvent)
	}

	return nil, fmt.Errorf("Unable to serve the request")
}

func (r *REST) singleEventAPI(namespace, name, method string) (runtime.Object, error) {
	if method == "GET" {
		return r.evProvider.GetNamespacedEventsByName(namespace, name)
	}
	return nil, fmt.Errorf("Method %s not allowed", method)
}

func (r *REST) listEventsAPI(namespace, method string) (runtime.Object, error) {
	switch method {
	case "GET":
		return r.evProvider.ListAllEventsByNamespace(namespace)
	case "POST":
		return r.evProvider.CreateNewEvent(namespace)
	}
	return nil, fmt.Errorf("Method %s not allowed", method)
}
