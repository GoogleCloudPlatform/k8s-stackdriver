package config

import (
	"net/url"
	"testing"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMapToSourceConfig(t *testing.T) {
	podName := "pod-name"
	podNamespace := "pod-namespace"
	emptyAuthConfig := AuthConfig{}
	tokenAuthConfig := AuthConfig{Token: "token"}
	userAuthConfig := AuthConfig{Username: "user", Password: "password"}
	emptyWhitelistedLabelsMap := make(map[string]map[string]bool)

	testcases := []struct {
		componentName string
		url           url.URL
		ip            string
		podName       string
		podNamespace  string
		want          SourceConfig
	}{
		{
			componentName: "kube-proxy",
			url:           url.URL{Scheme: "https", Host: ":8080"},
			ip:            "10.25.78.143",
			podName:       podName,
			podNamespace:  podNamespace,
			want: SourceConfig{
				Component:            "kube-proxy",
				Protocol:             "https",
				Host:                 "10.25.78.143",
				Port:                 uint(8080),
				Path:                 defaultMetricsPath,
				AuthConfig:           emptyAuthConfig,
				PodConfig:            NewPodConfig(podName, podNamespace, "", "", ""),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			componentName: "fluentd",
			url:           url.URL{Scheme: "http", Host: ":80", RawQuery: "whitelisted=metric1,metric2&authToken=token"},
			ip:            "very_important_ip",
			podName:       podName,
			podNamespace:  podNamespace,
			want: SourceConfig{
				Component:            "fluentd",
				Protocol:             "http",
				Host:                 "very_important_ip",
				Port:                 uint(80),
				Path:                 defaultMetricsPath,
				AuthConfig:           tokenAuthConfig,
				Whitelisted:          []string{"metric1", "metric2"},
				PodConfig:            NewPodConfig(podName, podNamespace, "", "", ""),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			componentName: "cadvisor",
			url:           url.URL{Scheme: "http", Host: ":8080", RawQuery: "whitelisted=metric1,metric2&podIdLabel=pod-id&namespaceIdLabel=namespace-id&containerNamelabel=container-name&authUsername=user&authPassword=password"},
			ip:            "very_important_ip",
			podName:       podName,
			podNamespace:  podNamespace,
			want: SourceConfig{
				Component:            "cadvisor",
				Protocol:             "http",
				Host:                 "very_important_ip",
				Port:                 uint(8080),
				Path:                 defaultMetricsPath,
				AuthConfig:           userAuthConfig,
				Whitelisted:          []string{"metric1", "metric2"},
				PodConfig:            NewPodConfig(podName, podNamespace, "pod-id", "namespace-id", "container-name"),
				WhitelistedLabelsMap: emptyWhitelistedLabelsMap,
				CustomLabels:         map[string]string{},
			},
		},
		{
			componentName: "cadvisor",
			url:           url.URL{Host: ":8080", RawQuery: "whitelisted=metric1,metric2&podIdLabel=pod-id&namespaceIdLabel=namespace-id&containerNamelabel=id&whitelistedLabels=containerNameLabel:/system.slice/node-problem-detector.service"},
			ip:            "very_important_ip",
			podName:       podName,
			podNamespace:  podNamespace,
			want: SourceConfig{
				Component:   "cadvisor",
				Protocol:    "http",
				Host:        "very_important_ip",
				Port:        uint(8080),
				Path:        defaultMetricsPath,
				Whitelisted: []string{"metric1", "metric2"},
				PodConfig:   NewPodConfig(podName, podNamespace, "pod-id", "namespace-id", "id"),
				WhitelistedLabelsMap: map[string]map[string]bool{
					"containerNameLabel": {"/system.slice/node-problem-detector.service": true},
				},
				CustomLabels: map[string]string{},
			},
		},
	}
	for _, testcase := range testcases {
		output, err := mapToSourceConfig(testcase.componentName, testcase.url, testcase.ip, testcase.podName, testcase.podNamespace)
		if assert.NoError(t, err) {
			assert.Equal(t, testcase.want, *output)
		}
	}
}

func TestCreateLabelSelector(t *testing.T) {
	testcases := []struct {
		sources  map[string]url.URL
		nodeName string
		want     v1.ListOptions
	}{
		{
			sources:  map[string]url.URL{},
			nodeName: "node",
			want: v1.ListOptions{
				FieldSelector: "spec.nodeName=node",
				LabelSelector: "k8s-app in ()",
			},
		},
		{
			sources: map[string]url.URL{"key": {}},
			want: v1.ListOptions{
				FieldSelector: "spec.nodeName=",
				LabelSelector: "k8s-app in (key,)",
			},
		},
		{
			sources:  map[string]url.URL{"key1": {}, "key2": {}},
			nodeName: "node",
			want: v1.ListOptions{
				FieldSelector: "spec.nodeName=node",
				LabelSelector: "k8s-app in (key1,key2,)",
			},
		},
	}
	for _, testcase := range testcases {
		output := createOptionsForPodSelection(testcase.nodeName, testcase.sources)
		assert.Equal(t, testcase.want, output)
	}
}

func TestValidateSources(t *testing.T) {
	testcases := []struct {
		sources   flags.Uris
		want      map[string]url.URL
		wantError bool
	}{
		{
			sources: flags.Uris{
				{},
			},
			want:      map[string]url.URL{},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name"},
			},
			want:      map[string]url.URL{},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name", Val: url.URL{Scheme: "http", Host: ":80"}},
			},
			want: map[string]url.URL{
				"component-name": {Scheme: "http", Host: ":80"},
			},
			wantError: false,
		},
		{
			sources: flags.Uris{
				{Key: "component-name", Val: url.URL{Scheme: "http", Host: "hostname:80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":80"}},
				{Key: "component-name2", Val: url.URL{Scheme: "http", Host: "hostname:80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: "10.67.86.43:80"}},
				{Key: "component-name2", Val: url.URL{Scheme: "http", Host: ":80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":80"}},
				{Key: "component-name2", Val: url.URL{Scheme: "http", Host: ":81"}},
			},
			want: map[string]url.URL{
				"component-name1": {Scheme: "http", Host: ":80"},
				"component-name2": {Scheme: "http", Host: ":81"},
			},
			wantError: false,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":80"}},
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":81"}},
			},
			want:      map[string]url.URL{},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":80"}},
				{Key: "component-name1", Val: url.URL{Scheme: "http", Host: ":80"}},
			},
			want:      map[string]url.URL{},
			wantError: true,
		},
	}
	for _, test := range testcases {
		sourceMap, err := validateSources(test.sources)
		if test.wantError {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, test.want, sourceMap)
		}
	}
}
