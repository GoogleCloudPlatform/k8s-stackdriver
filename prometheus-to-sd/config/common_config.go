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
	"io/ioutil"
	"net/url"
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// PodConfig can identify metric and resource information for pods.
type PodConfig interface {
	// IsMetricLabel returns true if the label name should be added as a metric label
	IsMetricLabel(labelName string) bool

	// GetPodInfo returns the information required to identify the pod.
	GetPodInfo(labels []*dto.LabelPair) (containerName, podId, namespaceId, tenantUID string)
}

// NewPodConfig returns a PodConfig which uses for the provided pod, namespace and container label values,
// if found, and falls back to the podId and namespaceId.
func NewPodConfig(podId, namespaceId, podIdLabel, namespaceIdLabel, containerNameLabel, tenantUIDLabel string) PodConfig {
	return &podConfigImpl{
		podId:              podId,
		namespaceId:        namespaceId,
		podIdLabel:         podIdLabel,
		namespaceIdLabel:   namespaceIdLabel,
		containerNameLabel: containerNameLabel,
		tenantUIDLabel:     tenantUIDLabel,
	}
}

type podConfigImpl struct {
	podId              string
	namespaceId        string
	podIdLabel         string
	namespaceIdLabel   string
	containerNameLabel string
	tenantUIDLabel     string
}

func (p *podConfigImpl) IsMetricLabel(labelName string) bool {
	return labelName != p.podIdLabel && labelName != p.containerNameLabel && labelName != p.namespaceIdLabel && labelName != p.tenantUIDLabel
}

func (p *podConfigImpl) GetPodInfo(labels []*dto.LabelPair) (containerName, podId, namespaceId, tenantUID string) {
	containerName, podId, namespaceId, tenantUID = "", p.podId, p.namespaceId, ""
	for _, label := range labels {
		if label.GetName() == p.containerNameLabel && label.GetValue() != "" {
			containerName = label.GetValue()
		} else if label.GetName() == p.podIdLabel && label.GetValue() != "" {
			podId = label.GetValue()
		} else if label.GetName() == p.namespaceIdLabel && label.GetValue() != "" {
			namespaceId = label.GetValue()
		} else if label.GetName() == p.tenantUIDLabel && label.GetValue() != "" {
			tenantUID = label.GetValue()
		}
	}
	return containerName, podId, namespaceId, tenantUID
}

// CommonConfig contains all required information about environment in which
// prometheus-to-sd running and which component is monitored.
type CommonConfig struct {
	GceConfig                   *GceConfig
	SourceConfig                *SourceConfig
	OmitComponentName           bool
	DowncaseMetricNames         bool
	MonitoredResourceLabels     map[string]string
	MonitoredResourceTypePrefix string
}

// AuthConfig contains authentication data for making requests to components.
type AuthConfig struct {
	Username string
	Password string
	Token    string
}

func parseAuthConfig(url url.URL) (*AuthConfig, error) {
	values := url.Query()
	authToken := values.Get("authToken")
	authTokenFile := values.Get("authTokenFile")
	authUsername := values.Get("authUsername")
	authPassword := values.Get("authPassword")

	if len(authToken) == 0 && len(authTokenFile) > 0 {
		buff, err := ioutil.ReadFile(authTokenFile)
		if err != nil {
			return nil, err
		}
		authToken = strings.Trim(string(buff), " \n")
	}

	return &AuthConfig{
		Username: authUsername,
		Password: authPassword,
		Token:    authToken,
	}, nil
}
