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

package installer

import (
	"fmt"
	"github.com/emicklei/go-restful"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apimachinery"
	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/endpoints/request"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	restorage "github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/registry"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/types"
	api "k8s.io/client-go/pkg/api/v1"
)

// defaultAPIServer exposes nested objects for testability.
type defaultAPIServer struct {
	http.Handler
	container *restful.Container
}

type NotFoundError struct {
	event     string
	namespace string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("Event %s not found in namespace %s", e.event, e.namespace)
}

func (e NotFoundError) Status() metav1.Status {
	return metav1.Status{
		Code: 404,
	}
}

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	Scheme               = runtime.NewScheme()
	Codecs               = serializer.NewCodecFactory(Scheme)
	prefix               = genericapiserver.APIGroupPrefix
	groupVersion         schema.GroupVersion
	groupMeta            *apimachinery.GroupMeta
)

func init() {
	types.Install(groupFactoryRegistry, registry, Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)

	groupMeta = registry.GroupOrDie(types.GroupName)
	groupVersion = groupMeta.GroupVersion
}

func extractBodyString(response *http.Response) (string, error) {
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(body), err
}
func handle(prov provider.EventsProvider) http.Handler {
	container := restful.NewContainer()
	container.Router(restful.CurlyRouter{})
	mux := container.ServeMux
	resourceStorage := restorage.NewREST(prov)
	group := EventsAPIGroupVersion{
		DynamicStorage: resourceStorage,
		APIGroupVersion: &genericapi.APIGroupVersion{
			Root:         prefix,
			GroupVersion: groupVersion,

			ParameterCodec:  metav1.ParameterCodec,
			Serializer:      Codecs,
			Creater:         Scheme,
			Convertor:       Scheme,
			UnsafeConvertor: runtime.UnsafeObjectConvertor(Scheme),
			Copier:          Scheme,
			Typer:           Scheme,
			Linker:          groupMeta.SelfLinker,
			Mapper:          groupMeta.RESTMapper,

			Context:                request.NewRequestContextMapper(),
			OptionsExternalVersion: &schema.GroupVersion{Version: "v1"},

			ResourceLister: provider.NewResourceLister(prov),
		},
	}
	if err := group.InstallREST(container); err != nil {
		panic(fmt.Sprintf("unable to install container %s: %v", group.GroupVersion, err))
	}
	return &defaultAPIServer{mux, container}
}

type fakeProvider struct{}

func (p *fakeProvider) GetNamespacedEventsByName(namespace, name string) (*api.Event, error) {
	if namespace == "default" && name == "existing_event" {
		return &api.Event{}, nil
	}

	err := NotFoundError{
		event:     name,
		namespace: namespace,
	}

	if namespace == "default" && name == "not_existing_event" {
		return nil, err
	}

	if namespace == "foo" && name == "foo" {
		return nil, err
	}

	return nil, fmt.Errorf("Namespace : %s, event name: %s", namespace, name)
}

func (p *fakeProvider) ListAllEventsByNamespace(namespace string) (*api.EventList, error) {
	return &api.EventList{}, nil
}

func (p *fakeProvider) ListAllEvents() (*api.EventList, error) {
	return &api.EventList{}, nil
}

func (p *fakeProvider) CreateNewEvent(namespace string) (*api.Event, error) {
	return &api.Event{}, nil
}

func TestEventsAPI(t *testing.T) {
	type T struct {
		Method string
		Path   string
		Status int
	}

	group := "v1events/v1alpha1"
	cases := map[string]T{
		"GET list of events ":           {"GET", prefix + "/" + group + "/namespaces/foo/events", http.StatusOK},
		"GET list of events wrong path": {"GET", prefix + "/" + group + "/namespaces/default/foo", http.StatusNotFound},

		"GET event by name":                    {"GET", prefix + "/" + group + "/namespaces/default/events/existing_event", http.StatusOK},
		"GET not existing event by name":       {"GET", prefix + "/" + group + "/namespaces/default/events/not_existing_event", http.StatusNotFound},
		"GET not existing event and namespace": {"GET", prefix + "/" + group + "/namespaces/foo/events/foo", http.StatusNotFound},
		"GET event by name wrong path":         {"GET", prefix + "/" + group + "/namespaces/default/eve/foo", http.StatusNotFound},

		"GET event with too long path":  {"GET", prefix + "/" + group + "/namespaces/default/events/foo/foo", http.StatusNotFound},
		"GET event only with namespace": {"GET", prefix + "/" + group + "/namespaces/default", http.StatusNotFound},

		"GET event with any namespaces": {"GET", prefix + "/" + group + "/events/foo", http.StatusNotFound},

		"GET no namespace": {"GET", prefix + "/" + group + "/foo/default/events/foo", http.StatusNotFound},
		"GET wrong prefix": {"GET", "//apis/v1foo/v1alpha1/", http.StatusNotFound},

		"GET list all events":           {"GET", prefix + "/" + group + "/events", http.StatusOK},
		"GET list all events with typo": {"GET", prefix + "/" + group + "/foo", http.StatusNotFound},

		"POST create a new event":                   {"POST", prefix + "/" + group + "/namespaces/default/events", http.StatusOK},
		"POST create a new event without namespace": {"POST", prefix + "/" + group + "/events", http.StatusMethodNotAllowed},
		"POST create a new event giving name":       {"POST", prefix + "/" + group + "/namespaces/default/events/foo", http.StatusMethodNotAllowed},
		"POST prefix":                               {"POST", prefix + "/" + group, http.StatusMethodNotAllowed},

		"POST no namespace": {"POST", prefix + "/" + group + "/foo/default/events", http.StatusNotFound},
		"POST no events":    {"POST", prefix + "/" + group + "/namespaces/default/foo", http.StatusNotFound},
		"POST wrong prefix": {"POST", "//apis/v1foo/v1alpha1/", http.StatusNotFound},
	}

	prov := &fakeProvider{}
	server := httptest.NewServer(handle(prov))
	defer server.Close()
	client := http.Client{}

	for k, v := range cases {

		request, err := http.NewRequest(v.Method, server.URL+v.Path, nil)

		if err != nil {
			t.Fatalf("unexpected error (%s): %v", k, err)
		}

		response, err := client.Do(request)
		if err != nil {
			t.Errorf("unexpected error (%s): %v", k, err)
			continue
		}

		if response.StatusCode != v.Status {
			body, err := extractBodyString(response)
			bodyPart := body
			if err != nil {
				bodyPart = fmt.Sprintf("[error extracting body: %v]", err)
			}
			t.Errorf("request: %v", request)
			t.Errorf("Expected %d for %s (%s), Got %#v -- %s", v.Status, v.Method, k, response, bodyPart)
		}
	}

}
