package config

import (
	"net/url"
	"testing"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMapToSourceConfig(t *testing.T) {
	testcases := []struct {
		componentName string
		url           url.URL
		ip            string
		podConfig     PodConfig
		want          SourceConfig
	}{
		{
			componentName: "kube-proxy",
			url:           url.URL{Host: ":8080"},
			ip:            "10.25.78.143",
			podConfig: PodConfig{
				NamespaceId: "kube-system",
				PodId:       "something",
			},
			want: SourceConfig{
				Component: "kube-proxy",
				Host:      "10.25.78.143",
				Port:      uint(8080),
				Path:      defaultMetricsPath,
				PodConfig: PodConfig{
					NamespaceId: "kube-system",
					PodId:       "something",
				},
			},
		},
		{
			componentName: "fluentd",
			url:           url.URL{Host: ":80", RawQuery: "whitelisted=metric1,metric2"},
			ip:            "very_important_ip",
			podConfig: PodConfig{
				NamespaceId: "kube-system",
				PodId:       "something_else",
			},
			want: SourceConfig{
				Component:   "fluentd",
				Host:        "very_important_ip",
				Port:        uint(80),
				Path:        defaultMetricsPath,
				Whitelisted: []string{"metric1", "metric2"},
				PodConfig: PodConfig{
					NamespaceId: "kube-system",
					PodId:       "something_else",
				},
			},
		},
	}
	for _, testcase := range testcases {
		output, err := mapToSourceConfig(testcase.componentName, testcase.url, testcase.ip, testcase.podConfig)
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
				{Key: "component-name", Val: url.URL{Host: ":80"}},
			},
			want: map[string]url.URL{
				"component-name": {Host: ":80"},
			},
			wantError: false,
		},
		{
			sources: flags.Uris{
				{Key: "component-name", Val: url.URL{Host: "hostname:80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Host: ":80"}},
				{Key: "component-name2", Val: url.URL{Host: "hostname:80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Host: "10.67.86.43:80"}},
				{Key: "component-name2", Val: url.URL{Host: ":80"}},
			},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Host: ":80"}},
				{Key: "component-name2", Val: url.URL{Host: ":81"}},
			},
			want: map[string]url.URL{
				"component-name1": {Host: ":80"},
				"component-name2": {Host: ":81"},
			},
			wantError: false,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Host: ":80"}},
				{Key: "component-name1", Val: url.URL{Host: ":81"}},
			},
			want:      map[string]url.URL{},
			wantError: true,
		},
		{
			sources: flags.Uris{
				{Key: "component-name1", Val: url.URL{Host: ":80"}},
				{Key: "component-name1", Val: url.URL{Host: ":80"}},
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
