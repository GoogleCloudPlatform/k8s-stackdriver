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
	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/provider"
	"k8s.io/metrics/pkg/apis/custom_metrics/install"
)

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	scheme               = runtime.NewScheme()
	// Codecs is a codec factory
	Codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	install.Install(groupFactoryRegistry, registry, scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// Config contains a configuration for the api server.
type Config struct {
	GenericConfig *genericapiserver.Config
}

// CustomMetricsAdapterServer contains state for a Kubernetes cluster master/api server.
type CustomMetricsAdapterServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
	Provider         provider.CustomMetricsProvider
}

// CompletedConfig is a configuration for the api server with all required fields set to valid data.
type CompletedConfig struct {
	*Config
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *Config) Complete() CompletedConfig {
	c.GenericConfig.Complete()

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{c}
}

// SkipComplete provides a way to construct a server instance without config completion.
func (c *Config) SkipComplete() CompletedConfig {
	return CompletedConfig{c}
}

// New returns a new instance of CustomMetricsAdapterServer from the given config.
func (c CompletedConfig) New(cmProvider provider.CustomMetricsProvider) (*CustomMetricsAdapterServer, error) {
	genericServer, err := c.Config.GenericConfig.SkipComplete().New(genericapiserver.EmptyDelegate) // completion is done in Complete, no need for a second time
	if err != nil {
		return nil, err
	}

	s := &CustomMetricsAdapterServer{
		GenericAPIServer: genericServer,
		Provider:         cmProvider,
	}

	if err := s.InstallCustomMetricsAPI(); err != nil {
		return nil, err
	}

	return s, nil
}
