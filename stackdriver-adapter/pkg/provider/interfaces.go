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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/stackdriver-adapter/pkg/types"
)

type EventInfo struct {
	GroupResource          schema.GroupResource
	Namespaced             bool
	Event                  string
}

type EventsProvider interface {
	GetNamespacedEventsByName( namespace, eventName string) (*types.EventValue, error)
}
