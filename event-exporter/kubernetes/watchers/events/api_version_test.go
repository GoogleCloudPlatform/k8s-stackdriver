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
	"testing"

	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseEventsAPIVersion(t *testing.T) {
	testCases := []struct {
		value   string
		wantErr bool
	}{
		{value: string(AutoEventsAPIVersion), wantErr: false},
		{value: string(CoreV1EventsAPIVersion), wantErr: false},
		{value: string(EventsV1EventsAPIVersion), wantErr: false},
		{value: "invalid", wantErr: true},
	}

	for _, tc := range testCases {
		_, err := ParseEventsAPIVersion(tc.value)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q", tc.value)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.value, err)
		}
	}
}

func TestResolveEventsAPIVersion(t *testing.T) {
	tests := []struct {
		desc        string
		resources   []string
		requested   EventsAPIVersion
		wantVersion EventsAPIVersion
	}{
		{
			desc:        "auto with events v1 support",
			resources:   []string{eventsv1.SchemeGroupVersion.String()},
			requested:   AutoEventsAPIVersion,
			wantVersion: EventsV1EventsAPIVersion,
		},
		{
			desc:        "auto without events v1 support",
			resources:   nil,
			requested:   AutoEventsAPIVersion,
			wantVersion: CoreV1EventsAPIVersion,
		},
		{
			desc:        "explicit events v1",
			resources:   nil,
			requested:   EventsV1EventsAPIVersion,
			wantVersion: EventsV1EventsAPIVersion,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			discoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
			for _, gv := range tc.resources {
				discoveryClient.Resources = append(discoveryClient.Resources, &metav1.APIResourceList{
					GroupVersion: gv,
					APIResources: []metav1.APIResource{{Name: "events"}},
				})
			}

			got := resolveEventsAPIVersion(client, tc.requested)
			if got != tc.wantVersion {
				t.Fatalf("version = %s, expected %s", got, tc.wantVersion)
			}
		})
	}
}
