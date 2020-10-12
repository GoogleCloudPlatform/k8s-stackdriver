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

package server

import (
	"fmt"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/apiserver"
	"io"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"net"
)

// EventsAdapterServerOptions stores a configuration for events adapter
type EventsAdapterServerOptions struct {
	// genericoptions.ReccomendedOptions - EtcdOptions
	SecureServing  *genericoptions.SecureServingOptions
	Authentication *genericoptions.DelegatingAuthenticationOptions
	Authorization  *genericoptions.DelegatingAuthorizationOptions
	Features       *genericoptions.FeatureOptions

	StdOut io.Writer
	StdErr io.Writer
}

// NewEventsAdapterServerOptions creates a EventsAdapterServerOptions for provided output interface
func NewEventsAdapterServerOptions(out, errOut io.Writer) *EventsAdapterServerOptions {
	o := &EventsAdapterServerOptions{
		SecureServing:  genericoptions.NewSecureServingOptions(),
		Authentication: genericoptions.NewDelegatingAuthenticationOptions(),
		Authorization:  genericoptions.NewDelegatingAuthorizationOptions(),
		Features:       genericoptions.NewFeatureOptions(),

		StdOut: out,
		StdErr: errOut,
	}

	return o
}

// Validate validates EventsAdapterServerOptions. Currently all fields are correctly set in
// NewsAdapterServerOptions, so this is a no-op.
func (o EventsAdapterServerOptions) Validate(args []string) error {
	return nil
}

// Complete fills in any fields not set that are required to have valid data. Currently all fields
// are set by NewEventsAdapterServerOptions, so this is a no-op.
func (o *EventsAdapterServerOptions) Complete() error {
	return nil
}

// Config returns apiserver.Config object from EventsAdapterServerOptions.
func (o EventsAdapterServerOptions) Config() (*apiserver.Config, error) {
	// TODO have a "real" external address (have an AdvertiseAddress?)
	if err := o.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	serverConfig := genericapiserver.NewConfig(apiserver.Codecs)
	if err := o.SecureServing.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	if err := o.Authentication.ApplyTo(serverConfig); err != nil {
		return nil, err
	}
	if err := o.Authorization.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	// TODO: we can't currently serve swagger because we don't have a good way to dynamically update it
	// serverConfig.SwaggerConfig = genericapiserver.DefaultSwaggerConfig()

	config := &apiserver.Config{
		GenericConfig: serverConfig,
	}
	return config, nil
}
