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
			desc: "none match",
			config: &podConfigImpl{
				containerNameLabel: "foo",
				podIdLabel:         "bar",
				namespaceIdLabel:   "abc",
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
	}
	for _, tc := range []struct {
		desc              string
		config            *podConfigImpl
		wantContainerName string
		wantPodId         string
		wantNamespaceId   string
	}{
		{
			desc:              "empty",
			config:            &podConfigImpl{},
			wantContainerName: "",
			wantPodId:         "",
			wantNamespaceId:   "",
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
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			container, pod, namespace := tc.config.GetPodInfo(labels)
			if container != tc.wantContainerName {
				t.Errorf("Unexpected containerName; got %q, want %q", container, tc.wantContainerName)
			}
			if pod != tc.wantPodId {
				t.Errorf("Unexpected podId; got %q, want %q", pod, tc.wantPodId)
			}
			if namespace != tc.wantNamespaceId {
				t.Errorf("Unexpected namespaceId; got %q, want %q", namespace, tc.wantNamespaceId)
			}
		})
	}
}
