package config

import (
	"net/url"
	"testing"

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
