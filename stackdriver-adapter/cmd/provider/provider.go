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
	"time"
	"fmt"
	"github.com/golang/glog"
	"k8s.io/client-go/rest"
	_ "k8s.io/client-go/pkg/api/install"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/stackdriver-adapter/pkg/provider"
	"k8s.io/stackdriver-adapter/pkg/config"
	"k8s.io/stackdriver-adapter/pkg/types"
)

type sdService stackdriver.Service

type StackdriverProvider struct {
	restClient rest.Interface

	values map[provider.EventInfo]map[string]int64

	//service Stackdriver

	config *config.GceConfig

	rateInterval time.Duration
}


//TODO type Stackdriver interface {}

func NewStackdriverProvider(restClient rest.Interface, stackdriverService *stackdriver.Service, rateInterval time.Duration) provider.EventsProvider {
	gceConf, err := config.GetGceConfig("container.googleapis.com")
	if err != nil {
		glog.Fatalf("Failed to get GCE config: %v", err)
	}

	return &StackdriverProvider{
		restClient: restClient,
		values: make(map[provider.EventInfo]map[string]int64),
		config: gceConf,
		rateInterval: rateInterval,
	}
}

func (p *StackdriverProvider) GetNamespacedEventsByName( namespace, eventName string) (*types.EventValue, error){
	return nil,fmt.Errorf("Failed to find the vent: (namespace: %s, eventName: %s)", namespace, eventName)
}
