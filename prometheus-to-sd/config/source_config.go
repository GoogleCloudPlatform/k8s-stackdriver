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
	Component   string
	Host        string
	Port        uint
	Path        string
	Whitelisted []string
	PodConfig   PodConfig
}

// PodConfig describes the pod in which the monitored component is running.
type PodConfig struct {
	PodId       string
	NamespaceId string
}

const defaultMetricsPath = "/metrics"

// newSourceConfig creates a new SourceConfig based on string representation of fields.
func newSourceConfig(component string, host string, port string, path string, whitelisted string, podConfig PodConfig) (*SourceConfig, error) {
	if port == "" {
		return nil, fmt.Errorf("No port provided.")
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
		Component:   component,
		Host:        host,
		Port:        uint(portNum),
		Path:        path,
		Whitelisted: whitelistedList,
		PodConfig:   podConfig,
	}, nil
}

// parseSourceConfig creates a new SourceConfig based on the provided flags.Uri instance.
func parseSourceConfig(uri flags.Uri, podConfig PodConfig) (*SourceConfig, error) {
	host, port, err := net.SplitHostPort(uri.Val.Host)
	if err != nil {
		return nil, err
	}

	component := uri.Key
	values := uri.Val.Query()
	path := uri.Val.Path
	whitelisted := values.Get("whitelisted")

	return newSourceConfig(component, host, port, path, whitelisted, podConfig)
}

// UpdateWhitelistedMetrics sets passed list as a list of whitelisted metrics.
func (config *SourceConfig) UpdateWhitelistedMetrics(list []string) {
	config.Whitelisted = list
}

// SourceConfigsFromFlags creates a slice of SourceConfig's base on the provided flags.
func SourceConfigsFromFlags(source flags.Uris, component *string, host *string, port *uint, whitelisted *string, podId *string, namespaceId *string) []SourceConfig {
	podConfig := PodConfig{
		PodId:       *podId,
		NamespaceId: *namespaceId,
	}
	var sourceConfigs []SourceConfig
	for _, c := range source {
		if sourceConfig, err := parseSourceConfig(c, podConfig); err != nil {
			glog.Fatalf("Error while parsing source config flag %v: %v", c, err)
		} else {
			sourceConfigs = append(sourceConfigs, *sourceConfig)
		}
	}

	if len(source) == 0 && *component != "" {
		glog.Warningf("--component, --host, --port and --whitelisted flags are deprecated. Please use --source instead.")
		portStr := strconv.FormatUint(uint64(*port), 10)

		if sourceConfig, err := newSourceConfig(*component, *host, portStr, defaultMetricsPath, *whitelisted, podConfig); err != nil {
			glog.Fatalf("Error while parsing --component flag: %v", err)
		} else {
			glog.Infof("Created a new source instance from --component flag: %+v", sourceConfig)
			sourceConfigs = append(sourceConfigs, *sourceConfig)
		}
	}
	return sourceConfigs
}
