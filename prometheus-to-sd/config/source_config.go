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
	"strconv"
	"strings"

	"github.com/golang/glog"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
)

// SourceConfig contains data specific for scraping one component.
type SourceConfig struct {
	Component     string
	Host          string
	Port          uint
	Path          string
	Whitelisted   []string
	PodConfig     PodConfig
	MetricsPrefix string
}

const defaultMetricsPath = "/metrics"

// newSourceConfig creates a new SourceConfig based on string representation of fields.
func newSourceConfig(component, host, port, path, whitelisted, metricsPrefix string, podConfig PodConfig) (*SourceConfig, error) {
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

	var whitelistedList []string
	if whitelisted != "" {
		whitelistedList = strings.Split(whitelisted, ",")
	}

	return &SourceConfig{
		Component:     component,
		Host:          host,
		Port:          uint(portNum),
		Path:          path,
		Whitelisted:   whitelistedList,
		PodConfig:     podConfig,
		MetricsPrefix: metricsPrefix,
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
	path := uri.Val.Path
	whitelisted := values.Get("whitelisted")
	podIdLabel := values.Get("podIdLabel")
	namespaceIdLabel := values.Get("namespaceIdLabel")
	containerNamelabel := values.Get("containerNamelabel")
	metricsPrefix := values.Get("metricsPrefix")
	podConfig := NewPodConfig(podId, namespaceId, podIdLabel, namespaceIdLabel, containerNamelabel)

	return newSourceConfig(component, host, port, path, whitelisted, metricsPrefix, podConfig)
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
