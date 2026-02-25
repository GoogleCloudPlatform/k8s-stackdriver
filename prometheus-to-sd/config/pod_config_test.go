/*
Copyright 2018 Google Inc.

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

package config

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestIsMetricLabel(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		config *podConfigImpl
		label  string
		want   bool
	}{
		{
			desc:   "empty",
			config: &podConfigImpl{},
			label:  "foo",
			want:   true,
		},
		{
			desc: "containerNameLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			label: "foo",
			want:  false,
		},
		{
			desc: "podIdLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			label: "bar",
			want:  false,
		},
		{
			desc: "namespaceIdLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			label: "abc",
			want:  false,
		},
		{
			desc: "tenantUIDLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
				tenantUIDLabel:     "def",
			},
			label: "def",
			want:  false,
		},
		{
			desc: "entityTypeLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
				tenantUIDLabel:     "def",
				entityTypeLabel:    "ghi",
			},
			label: "ghi",
			want:  false,
		},
		{
			desc: "entityNameLabel matches",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
				tenantUIDLabel:     "def",
				entityTypeLabel:    "ghi",
				entityNameLabel:    "jkl",
			},
			label: "jkl",
			want:  false,
		},
		{
			desc: "none match",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
				tenantUIDLabel:     "def",
			},
			label: "xyz",
			want:  true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			actual := tc.config.IsMetricLabel(tc.label)
			if actual != tc.want {
				t.Fatalf("Unexpected result; got %v, want %v", actual, tc.want)
			}
		})
	}
}

func TestGetPodInfo(t *testing.T) {
	container, containerLabel := "container", "cLabel"
	pod, podLabel := "pod", "pLabel"
	namespace, namespaceLabel := "namespace", "nLabel"
	tenantUID, tenantUIDLabel := "tenantUID", "tLabel"
	entityType, entityTypeLabel := "entityType", "eTLabel"
	entityName, entityNameLabel := "entityName", "eNLabel"
	other, otherLabel := "other", "olabel"
	labels := []*dto.LabelPair{
		{
			Name:  &containerLabel,
			Value: &container,
		},
		{
			Name:  &podLabel,
			Value: &pod,
		},
		{
			Name:  &otherLabel,
			Value: &other,
		},
		{
			Name:  &namespaceLabel,
			Value: &namespace,
		},
		{
			Name:  &tenantUIDLabel,
			Value: &tenantUID,
		},
		{
			Name:  &entityTypeLabel,
			Value: &entityType,
		},
		{
			Name:  &entityNameLabel,
			Value: &entityName,
		},
	}
	for _, tc := range []struct {
		desc              string
		config            *podConfigImpl
		wantContainerName string
		wantPodId         string
		wantNamespaceId   string
		wantTenantUID     string
		wantEntityType    string
		wantEntityName    string
	}{
		{
			desc:              "empty",
			config:            &podConfigImpl{},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "not matching labels",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "matching labels",
			config: &podConfigImpl{
				containerNameLabel: containerLabel,
				podIdLabel:         podLabel,
				namespaceIdLabel:   namespaceLabel,
			},
			wantContainerName: container,
			wantPodId:         pod,
			wantNamespaceId:   namespace,
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "matching labels w/ podId and namespaceId specified",
			config: &podConfigImpl{
				podId:              "podid",
				namespaceId:        "namespaceid",
				containerNameLabel: containerLabel,
				podIdLabel:         podLabel,
				namespaceIdLabel:   namespaceLabel,
			},
			wantContainerName: container,
			wantPodId:         pod,
			wantNamespaceId:   namespace,
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "not matching labels w/ podId and namespaceId specified",
			config: &podConfigImpl{
				podId:              "podid",
				namespaceId:        "namespaceid",
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			wantContainerName: "",
			wantPodId:         "podid",
			wantNamespaceId:   "namespaceid",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "some matching labels w/ podId and namespaceId specified",
			config: &podConfigImpl{
				podId:              "podid",
				namespaceId:        "namespaceid",
				containerNameLabel: containerLabel,
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
			},
			wantContainerName: container,
			wantPodId:         "podid",
			wantNamespaceId:   "namespaceid",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "podId and namespaceId specified",
			config: &podConfigImpl{
				podId:       "podid",
				namespaceId: "namespaceid",
			},
			wantContainerName: "",
			wantPodId:         "podid",
			wantNamespaceId:   "namespaceid",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "tenant UID specified",
			config: &podConfigImpl{
				tenantUIDLabel: tenantUIDLabel,
			},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     tenantUID,
			wantEntityType:    "",
			wantEntityName:    "",
		},
		{
			desc: "entity type specified",
			config: &podConfigImpl{
				entityTypeLabel: entityTypeLabel,
			},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     "",
			wantEntityType:    entityType,
			wantEntityName:    "",
		},
		{
			desc: "entity name specified",
			config: &podConfigImpl{
				entityNameLabel: entityNameLabel,
			},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     "",
			wantEntityType:    "",
			wantEntityName:    entityName,
		},
		{
			desc: "all custom labels specified",
			config: &podConfigImpl{
				tenantUIDLabel:  tenantUIDLabel,
				entityTypeLabel: entityTypeLabel,
				entityNameLabel: entityNameLabel,
			},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
			wantTenantUID:     tenantUID,
			wantEntityType:    entityType,
			wantEntityName:    entityName,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			container, pod, namespace, tenantUID, entityType, entityName := tc.config.GetPodInfo(labels)
			if container != tc.wantContainerName {
				t.Errorf("Unexpected containerName; got %q, want %q", container, tc.wantContainerName)
			}
			if pod != tc.wantPodId {
				t.Errorf("Unexpected podId; got %q, want %q", pod, tc.wantPodId)
			}
			if namespace != tc.wantNamespaceId {
				t.Errorf("Unexpected namespaceId; got %q, want %q", namespace, tc.wantNamespaceId)
			}
			if tenantUID != tc.wantTenantUID {
				t.Errorf("Unexpected tenantUID; got %q, want %q", tenantUID, tc.wantTenantUID)
			}
			if entityType != tc.wantEntityType {
				t.Errorf("Unexpected entityType; got %q, want %q", entityType, tc.wantEntityType)
			}
			if entityName != tc.wantEntityName {
				t.Errorf("Unexpected entityName; got %q, want %q", entityName, tc.wantEntityName)
			}
		})
	}
}
