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

package apiserver

import (
	specificapi "github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/apiserver/installer"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	storage "github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/registry"
	"k8s.io/apimachinery/pkg/apimachinery"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

// InstallEventsAPI registers the api server in Kube Aggregator
func (s *EventsAdapterServer) InstallEventsAPI() error {

	groupMeta := registry.GroupOrDie("v1events")

	preferredVersionForDiscovery := metav1.GroupVersionForDiscovery{
		GroupVersion: groupMeta.GroupVersion.String(),
		Version:      groupMeta.GroupVersion.Version,
	}
	groupVersion := metav1.GroupVersionForDiscovery{
		GroupVersion: groupMeta.GroupVersion.String(),
		Version:      groupMeta.GroupVersion.Version,
	}
	apiGroup := metav1.APIGroup{
		Name:             groupMeta.GroupVersion.String(),
		Versions:         []metav1.GroupVersionForDiscovery{groupVersion},
		PreferredVersion: preferredVersionForDiscovery,
	}

	evAPI := s.evAPI(groupMeta, &groupMeta.GroupVersion)

	if err := evAPI.InstallREST(s.GenericAPIServer.HandlerContainer.Container); err != nil {
		return err
	}

	path := genericapiserver.APIGroupPrefix + "/" + groupMeta.GroupVersion.Group
	s.GenericAPIServer.HandlerContainer.Add(genericapi.NewGroupWebService(s.GenericAPIServer.Serializer, path, apiGroup))

	return nil
}

func (s *EventsAdapterServer) evAPI(groupMeta *apimachinery.GroupMeta, groupVersion *schema.GroupVersion) *specificapi.EventsAPIGroupVersion {
	resourceStorage := storage.NewREST(s.Provider)

	return &specificapi.EventsAPIGroupVersion{
		DynamicStorage: resourceStorage,
		APIGroupVersion: &genericapi.APIGroupVersion{
			Root:         genericapiserver.APIGroupPrefix,
			GroupVersion: *groupVersion,

			ParameterCodec:  metav1.ParameterCodec,
			Serializer:      Codecs,
			Creater:         Scheme,
			Convertor:       Scheme,
			UnsafeConvertor: runtime.UnsafeObjectConvertor(Scheme),
			Copier:          Scheme,
			Typer:           Scheme,
			Linker:          groupMeta.SelfLinker,
			Mapper:          groupMeta.RESTMapper,

			Context:                s.GenericAPIServer.RequestContextMapper(),
			MinRequestTimeout:      s.GenericAPIServer.MinRequestTimeout(),
			OptionsExternalVersion: &schema.GroupVersion{Version: "v1"},

			ResourceLister: provider.NewResourceLister(s.Provider),
		},
	}
}
