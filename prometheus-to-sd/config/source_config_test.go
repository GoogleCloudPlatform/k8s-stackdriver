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

package config

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
)

var (
	emptyWhitelistedLabelsMap = make(map[string]map[string]bool)
)

func TestNewSourceConfig(t *testing.T) {
	podConfig := NewPodConfig("podId", "namespaceId", "", "", "")
	emptyPodConfig := NewPodConfig("", "", "", "", "")
	authConfig := AuthConfig{Token: "token"}
	correct := [...]struct {
		component            string
		protocol             string
		host                 string
		port                 string
		path                 string
		whitelisted          string
		podConfig            PodConfig
		whitelistedLabelsMap map[string]map[string]bool
		output               SourceConfig
	}{
		{"testComponent", "https", "localhost", "1234", defaultMetricsPath, "a,b,c,d", podConfig, emptyWhitelistedLabelsMap,
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "https",
				Host:                 "localhost",
				Port:                 1234,
				Path:                 defaultMetricsPath,
				AuthConfig:           authConfig,
				Whitelisted:          []string{"a", "b", "c", "d"},
				PodConfig:            podConfig,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},

		{"testComponent", "http", "localhost", "1234", "/status/prometheus", "", emptyPodConfig, emptyWhitelistedLabelsMap,
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "localhost",
				Port:                 1234,
				Path:                 "/status/prometheus",
				AuthConfig:           authConfig,
				Whitelisted:          nil,
				PodConfig:            emptyPodConfig,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{"testComponent", "http", "localhost", "1234", "/", "", emptyPodConfig, emptyWhitelistedLabelsMap,
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "localhost",
				Port:                 1234,
				Path:                 "/metrics",
				AuthConfig:           authConfig,
				Whitelisted:          nil,
				PodConfig:            emptyPodConfig,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{"testComponent", "http", "localhost", "1234", "", "", emptyPodConfig, emptyWhitelistedLabelsMap,
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "localhost",
				Port:                 1234,
				Path:                 "/metrics",
				AuthConfig:           authConfig,
				Whitelisted:          nil,
				PodConfig:            emptyPodConfig,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{"testComponent", "http", "localhost", "1234", defaultMetricsPath, "", emptyPodConfig,
			map[string]map[string]bool{"containerNameLabel": {"prometheus-to-sd": true}},
			SourceConfig{
				Component:   "testComponent",
				Protocol:    "http",
				Host:        "localhost",
				Port:        1234,
				Path:        "/metrics",
				AuthConfig:  authConfig,
				Whitelisted: nil,
				PodConfig:   emptyPodConfig,
				WhitelistedLabelsMap: map[string]map[string]bool{
					"containerNameLabel": {"prometheus-to-sd": true},
				},
				CustomLabels: map[string]string{},
			},
		},
	}

	for _, c := range correct {
		res, err := newSourceConfig(c.component, c.protocol, c.host, c.port, c.path, authConfig, c.whitelisted, "", c.podConfig, c.whitelistedLabelsMap, "", make(map[string]string))
		if assert.NoError(t, err) {
			assert.Equal(t, c.output, *res)
		}
	}
}

func TestParseSourceConfig(t *testing.T) {
	podId := "podId"
	namespaceId := "namespaceId"
	tokenAuthConfig := AuthConfig{Token: "token"}
	userAuthConfig := AuthConfig{Username: "user", Password: "password"}
	correct := [...]struct {
		in     flags.Uri
		output SourceConfig
	}{
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "https",
					Host:     "hostname:1234",
					Path:     defaultMetricsPath,
					RawQuery: "whitelisted=a,b,c,d&authToken=token",
				},
			},
			SourceConfig{

				Component:            "testComponent",
				Protocol:             "https",
				Host:                 "hostname",
				Port:                 1234,
				Path:                 defaultMetricsPath,
				AuthConfig:           tokenAuthConfig,
				Whitelisted:          []string{"a", "b", "c", "d"},
				PodConfig:            NewPodConfig(podId, namespaceId, "", "", ""),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "hostname:1234",
					Path:     "/status/prometheus",
					RawQuery: "whitelisted=a,b,c,d&authUsername=user&authPassword=password",
				},
			},
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "hostname",
				Port:                 1234,
				Path:                 "/status/prometheus",
				AuthConfig:           userAuthConfig,
				Whitelisted:          []string{"a", "b", "c", "d"},
				PodConfig:            NewPodConfig(podId, namespaceId, "", "", ""),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "localhost:8080",
					Path:     defaultMetricsPath,
					RawQuery: "metricsPrefix=container.googleapis.com/newPrefix",
				},
			},
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "localhost",
				Port:                 8080,
				Path:                 defaultMetricsPath,
				MetricsPrefix:        "container.googleapis.com/newPrefix",
				PodConfig:            NewPodConfig(podId, namespaceId, "", "", ""),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "hostname:1234",
					Path:     defaultMetricsPath,
					RawQuery: "whitelistedLabels=containerNameLabel:testContainer|podIdLabel:pod1",
				},
			},
			SourceConfig{
				Component: "testComponent",
				Protocol:  "http",
				Host:      "hostname",
				Port:      1234,
				Path:      defaultMetricsPath,
				WhitelistedLabelsMap: map[string]map[string]bool{
					"containerNameLabel": {"testContainer": true},
					"podIdLabel":         {"pod1": true},
				},
				PodConfig:    NewPodConfig(podId, namespaceId, "", "", ""),
				CustomLabels: map[string]string{},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "hostname:1234",
					Path:     defaultMetricsPath,
					RawQuery: "whitelistedLabels=containerNameLabel:testContainer1,testContainer2",
				},
			},
			SourceConfig{
				Component: "testComponent",
				Protocol:  "http",
				Host:      "hostname",
				Port:      1234,
				Path:      defaultMetricsPath,
				WhitelistedLabelsMap: map[string]map[string]bool{
					"containerNameLabel": {"testContainer1": true, "testContainer2": true},
				},
				PodConfig:    NewPodConfig(podId, namespaceId, "", "", ""),
				CustomLabels: map[string]string{},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "hostname:1234",
					Path:     defaultMetricsPath,
					RawQuery: "customResourceType=quux&customLabels[foo]=bar&customLabels[bar]=baz",
				},
			},
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "hostname",
				Port:                 1234,
				Path:                 defaultMetricsPath,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				PodConfig:            NewPodConfig(podId, namespaceId, "", "", ""),
				CustomResourceType:   "quux",
				CustomLabels: map[string]string{
					"foo": "bar",
					"bar": "baz",
				},
			},
		},
		{
			flags.Uri{
				Key: "testComponent",
				Val: url.URL{
					Scheme:   "http",
					Host:     "hostname:1234",
					Path:     defaultMetricsPath,
					RawQuery: "customResourceType=quux&customLabels[foo]=&customLabels[bar]",
				},
			},
			SourceConfig{
				Component:            "testComponent",
				Protocol:             "http",
				Host:                 "hostname",
				Port:                 1234,
				Path:                 defaultMetricsPath,
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				PodConfig:            NewPodConfig(podId, namespaceId, "", "", ""),
				CustomResourceType:   "quux",
				CustomLabels: map[string]string{
					"foo": "",
					"bar": "",
				},
			},
		},
	}

	for _, c := range correct {
		res, err := parseSourceConfig(c.in, podId, namespaceId)
		if assert.NoError(t, err) {
			assert.Equal(t, c.output, *res)
		}
	}

	incorrect := [...]flags.Uri{
		{
			Key: "incorrectHost",
			Val: url.URL{
				Scheme:   "http",
				Host:     "hostname[:1234",
				RawQuery: "whitelisted=a,b,c,d",
			},
		},
		{
			Key: "noPort",
			Val: url.URL{
				Scheme:   "http",
				Host:     "hostname",
				RawQuery: "whitelisted=a,b,c,d",
			},
		},
		{
			Key: "unsupportedWhitelistedLabel",
			Val: url.URL{
				Scheme:   "http",
				Host:     "hostname:1234",
				Path:     defaultMetricsPath,
				RawQuery: "whitelistedLabels=unsupportedLabel:val1",
			},
		},
	}

	for _, c := range incorrect {
		_, err := parseSourceConfig(c, podId, namespaceId)
		assert.Error(t, err)
	}
}
