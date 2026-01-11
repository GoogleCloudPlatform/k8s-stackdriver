/*
Copyright 2017 Google Inc.

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

package events

import (
	"github.com/golang/glog"

	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

func resolveEventsAPIVersion(client kubernetes.Interface, requested EventsAPIVersion) EventsAPIVersion {
	if requested != AutoEventsAPIVersion {
		return requested
	}
	if client == nil || !supportsEventsV1(client.Discovery()) {
		glog.Infof("events.k8s.io/v1 not available, falling back to core/v1")
		return CoreV1EventsAPIVersion
	}
	return EventsV1EventsAPIVersion
}

func supportsEventsV1(discoveryClient discovery.DiscoveryInterface) bool {
	if discoveryClient == nil {
		return false
	}

	_, err := discoveryClient.ServerResourcesForGroupVersion(eventsv1.SchemeGroupVersion.String())
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	if groups, ok := discovery.GroupDiscoveryFailedErrorGroups(err); ok {
		if gvErr, exists := groups[eventsv1.SchemeGroupVersion]; exists && apierrors.IsNotFound(gvErr) {
			return false
		}
	}

	return true
}
