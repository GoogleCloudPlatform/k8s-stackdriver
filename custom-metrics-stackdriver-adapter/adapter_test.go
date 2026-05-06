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

package main

import (
	"strings"
	"testing"

	generatedopenapi "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/api/generated/openapi"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/kube-openapi/pkg/validation/spec"
	customexternalmetrics "sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	"sigs.k8s.io/metrics-server/pkg/api"
)

var urltests = []struct {
	url    string
	expect bool
}{
	{
		url:    "https://monitoring.googleapis.com",
		expect: true,
	},
	{
		url:    "https://monitoring.googleapis.com/",
		expect: true,
	},
	{
		url:    "http://monitoring.googleapis.com",
		expect: true,
	},
	{
		url:    "http://google.com",
		expect: true,
	},
	{
		url: "http//google.com",
	},
	{
		url: "http//google.com/",
	},
	{
		url: "google.com",
	},
	{
		url: "google.com/",
	},
	{
		url: "google/com",
	},
	{
		url: "http:::/not.valid/a//a??a?b=&&c#hi",
	},
	{
		url: "http://",
	},
}

func TestURLValidator(t *testing.T) {
	for _, tc := range urltests {
		t.Run(tc.url, func(t *testing.T) {
			result := validateUrl(tc.url)
			if result != tc.expect {
				t.Errorf("for url %v got %v, expect %v", tc.url, result, tc.expect)
			}
		})
	}
}

func TestOpenAPIDefinitionsResolution(t *testing.T) {
	namer := openapinamer.NewDefinitionNamer(api.Scheme, customexternalmetrics.Scheme)
	definitions := generatedopenapi.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.Ref{}
	})
	for gvk, typ := range customexternalmetrics.Scheme.AllKnownTypes() {
		if !strings.Contains(gvk.Group, "metrics.k8s.io") || gvk.Version == "__internal" {
			continue
		}
		typeName := typ.PkgPath() + "." + typ.Name()
		resolvedName, _ := getDefinitionName(namer, typeName)
		if _, ok := definitions[resolvedName]; !ok {
			t.Errorf("OpenAPI definition for GVK %v (type name %q resolved to %q) not found in generated definitions", gvk, typeName, resolvedName)
		}
	}
}