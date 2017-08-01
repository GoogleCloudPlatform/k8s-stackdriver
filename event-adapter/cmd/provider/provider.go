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
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/provider"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/pkg/types"
	"github.com/golang/glog"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/client-go/rest"
	"time"
)

type sdService stackdriver.Service

// StackdriverProvider provide all the info for Stackdriver
type StackdriverProvider struct {
	restClient rest.Interface

	values map[provider.EventInfo]map[string]int64

	//service Stackdriver

	config *config.GceConfig

	rateInterval time.Duration
}

//TODO type Stackdriver interface {}

// NewStackdriverProvider create a new Provider with standard settings
func NewStackdriverProvider(restClient rest.Interface, stackdriverService *stackdriver.Service, rateInterval time.Duration) provider.EventsProvider {
	gceConf, err := config.GetGceConfig("container.googleapis.com")
	if err != nil {
		glog.Fatalf("Failed to get GCE config: %v", err)
	}

	return &StackdriverProvider{
		restClient:   restClient,
		values:       make(map[provider.EventInfo]map[string]int64),
		config:       gceConf,
		rateInterval: rateInterval,
	}
}

// GetNamespacedEventsByName get the information of the given event
func (p *StackdriverProvider) GetNamespacedEventsByName(namespace, eventName string) (*types.EventValue, error) {
	return nil, fmt.Errorf("Failed to find the vent: (namespace: %s, eventName: %s)", namespace, eventName)
}
