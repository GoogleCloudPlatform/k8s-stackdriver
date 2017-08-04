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
)

// defaultAPIServer exposes nested objects for testability.
type defaultAPIServer struct {
	http.Handler
	container *restful.Container
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

func (p *fakeProvider) GetNamespacedEventsByName(namespace, name string) (*types.EventValue, error) {
	return &types.EventValue{}, nil
}

func (p *fakeProvider) ListAllEvents() (*types.EventValueList, error) {
	return &types.EventValueList{}, nil
}

func TestEventsAPI(t *testing.T) {
	type T struct {
		Method string
		Path   string
		Status int
	}

	group := "v1events/v1alpha1"
	cases := map[string]T{
		"GET list of events ":           {"GET", prefix + "/" + group + "/namespaces/default/events", http.StatusOK},
		"GET list of events (singular)": {"GET", prefix + "/" + group + "/namespaces/default/event", http.StatusInternalServerError},

		"GET event by name": {"GET", prefix + "/" + group + "/namespaces/default/events/foo", http.StatusOK},
		"GET evento":        {"GET", prefix + "/" + group + "/namespaces/default/evento/foo", http.StatusInternalServerError},

		"GET no namespace":            {"GET", prefix + "/" + group + "/namespace/default/events/foo", http.StatusNotFound},
		"GET wrong prefix (singular)": {"GET", "//apis/v1event/v1alpha1/", http.StatusNotFound},
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
			t.Errorf("request: %s", request)
			t.Errorf("Expected %d for %s (%s), Got %#v -- %s", v.Status, v.Method, k, response, bodyPart)
		}
	}

}
