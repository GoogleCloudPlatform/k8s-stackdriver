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

package types

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

type EventValueList struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []EventValue `json:"items" protobuf:"bytes,2,rep,name=items"`
}


type EventValue struct {
	metav1.TypeMeta `json:",inline"`

	// a reference to the described object
	DescribedObject v1.ObjectReference `json:"describedObject" protobuf:"bytes,1,name=describedObject"`

	// the name of the event
	EventName string `json:"eventName" protobuf:"bytes,2,name=eventName"`

	// indicates the time at which the events were produced
	Timestamp metav1.Time `json:"timestamp" protobuf:"bytes,3,name=timestamp"`

	WindowSeconds *int64 `json:"window,omitempty" protobuf:"bytes,4,opt,name=windowSeconds"`

	Value resource.Quantity `json:"value" protobuf:"bytes,5,name=value"`
}

const AllObjects = "*"

