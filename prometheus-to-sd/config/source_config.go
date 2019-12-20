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
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang/glog"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
)

// SourceConfig contains data specific for scraping one component.
type SourceConfig struct {
	Component            string
	Protocol             string
	Host                 string
	Port                 uint
	Path                 string
	AuthConfig           AuthConfig
	Whitelisted          []string
	WhitelistedLabelsMap map[string]map[string]bool
	PodConfig            PodConfig
	MetricsPrefix        string
	CustomResourceType   string
	CustomLabels         map[string]string
}

const defaultMetricsPath = "/metrics"

var validWhitelistedLabels = map[string]bool{"containerNameLabel": true, "namespaceIdLabel": true, "podIdLabel": true}

// newSourceConfig creates a new SourceConfig based on string representation of fields.
func newSourceConfig(component, protocol, host, port, path string, auth AuthConfig, whitelisted, metricsPrefix string, podConfig PodConfig, whitelistedLabelsMap map[string]map[string]bool, customResourceType string, customLabels map[string]string) (*SourceConfig, error) {
	if port == "" {
		return nil, fmt.Errorf("No port provided.")
	}
	if path == "" || path == "/" {
		path = defaultMetricsPath
	}

	portNum, err := strconv.ParseUint(port, 10, 32)
	if err != nil {
		return nil, err
	}

	if len(protocol) == 0 {
		if portNum == 443 {
			protocol = "https"
		} else {
			protocol = "http"
		}
	}

	var whitelistedList []string
	if whitelisted != "" {
		whitelistedList = strings.Split(whitelisted, ",")
	}

	return &SourceConfig{
		Component:            component,
		Protocol:             protocol,
		Host:                 host,
		Port:                 uint(portNum),
		Path:                 path,
		AuthConfig:           auth,
		Whitelisted:          whitelistedList,
		WhitelistedLabelsMap: whitelistedLabelsMap,
		PodConfig:            podConfig,
		MetricsPrefix:        metricsPrefix,
		CustomResourceType:   customResourceType,
		CustomLabels:         customLabels,
	}, nil
}

// parseSourceConfig creates a new SourceConfig based on the provided flags.Uri instance.
func parseSourceConfig(uri flags.Uri, podId, namespaceId string) (*SourceConfig, error) {
	host, port, err := net.SplitHostPort(uri.Val.Host)
	if err != nil {
		return nil, err
	}

	component := uri.Key
	values := uri.Val.Query()
	protocol := uri.Val.Scheme
	path := uri.Val.Path
	whitelisted := values.Get("whitelisted")
	podIdLabel := values.Get("podIdLabel")
	namespaceIdLabel := values.Get("namespaceIdLabel")
	containerNameLabel := values.Get("containerNameLabel")
	metricsPrefix := values.Get("metricsPrefix")
	customResource := values.Get("customResourceType")
	customLabels := getMap(values, "customLabels")
	auth, err := parseAuthConfig(uri.Val)
	if err != nil {
		return nil, err
	}
	podConfig := NewPodConfig(podId, namespaceId, podIdLabel, namespaceIdLabel, containerNameLabel)

	whitelistedLabelsMap, err := parseWhitelistedLabels(values.Get("whitelistedLabels"))
	if err != nil {
		return nil, err
	}
	return newSourceConfig(component, protocol, host, port, path, *auth, whitelisted, metricsPrefix, podConfig, whitelistedLabelsMap, customResource, customLabels)
}

func getMap(v url.Values, name string) map[string]string {
	n := len(name)
	m := make(map[string]string)
	for k, vals := range v {
		if len(k) <= n {
			continue
		}
		if k[:n] == name && k[n] == '[' && k[len(k)-1] == ']' {
			if len(vals) > 1 {
				glog.Warningf("Ignoring multiple values of %v", k)
			}
			var val string
			if len(vals) == 1 {
				val = vals[0]
			}
			m[k[n+1:len(k)-1]] = val
		}
	}
	return m
}

// UpdateWhitelistedMetrics sets passed list as a list of whitelisted metrics.
func (config *SourceConfig) UpdateWhitelistedMetrics(list []string) {
	config.Whitelisted = list
}

// SourceConfigsFromFlags creates a slice of SourceConfig's base on the provided flags.
func SourceConfigsFromFlags(source flags.Uris, podId *string, namespaceId *string, defaultMetricsPrefix string) []*SourceConfig {
	var sourceConfigs []*SourceConfig
	for _, c := range source {
		if sourceConfig, err := parseSourceConfig(c, *podId, *namespaceId); err != nil {
			glog.Fatalf("Error while parsing source config flag %v: %v", c, err)
		} else {
			if sourceConfig.MetricsPrefix == "" {
				sourceConfig.MetricsPrefix = defaultMetricsPrefix
			}
			sourceConfigs = append(sourceConfigs, sourceConfig)
		}
	}
	return sourceConfigs
}

// parseWhitelistedLabels extracts the labels and their corresponding whitelisted values that will be filtered for.
func parseWhitelistedLabels(whitelistedLabels string) (map[string]map[string]bool, error) {
	// whitelistedLabels  is of the format whitelistedLabel1:val11,val12,etc.|whitelistedLabel2:val21
	labelsMap := make(map[string]map[string]bool)
	// URL.Query().Get() will return "" if whitelistedLabels is not specified
	if whitelistedLabels == "" {
		return labelsMap, nil
	}
	labelVals := strings.Split(whitelistedLabels, "|")
	for _, labelVal := range labelVals {
		labelAndValueParts := strings.Split(labelVal, ":")
		if len(labelAndValueParts) != 2 {
			return nil, fmt.Errorf("Incorrectly formatted whitelisted label and values: %v", labelVal)
		}
		labelKey := labelAndValueParts[0]
		if validWhitelistedLabels[labelKey] {
			labelsMap[labelKey] = make(map[string]bool)
			for _, val := range strings.Split(labelAndValueParts[1], ",") {
				labelsMap[labelAndValueParts[0]][val] = true
			}
		} else {
			return nil, fmt.Errorf("Filtering against label %v is unsupported. Only containerNameLabel, namespaceIdLabel, and podIdLabel are supported.", labelKey)
		}
	}
	return labelsMap, nil
}
