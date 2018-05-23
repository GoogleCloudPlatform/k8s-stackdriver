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
	"os"
	"strings"

	gce "cloud.google.com/go/compute/metadata"
)

// GceConfig aggregates all GCE related configuration parameters.
type GceConfig struct {
	Project       string
	Zone          string
	Cluster       string
	Instance      string
	MetricsPrefix string
}

// GetGceConfig builds GceConfig based on the provided prefix and metadata server available on GCE.
func GetGceConfig(metricsPrefix string) (*GceConfig, error) {
	gceConfig := &GceConfig{
		Project:       "",
		Zone:          "",
		Cluster:       "",
		Instance:      "",
		MetricsPrefix: metricsPrefix,
	}

	if gce.OnGCE() {
		project, err := gce.ProjectID()
		if err != nil {
			return nil, fmt.Errorf("error while getting project id: %v", err)
		}

		zone, err := gce.Zone()
		if err != nil {
			return nil, fmt.Errorf("error while getting zone: %v", err)
		}

		cluster, err := gce.InstanceAttributeValue("cluster-name")
		if err != nil {
			return nil, fmt.Errorf("error while getting cluster name: %v", err)
		}

		cluster = strings.TrimSpace(cluster)
		if cluster == "" {
			return nil, fmt.Errorf("cluster-name metadata was empty")
		}

		instance, err := gce.InstanceName()
		if err != nil {
			return nil, fmt.Errorf("error while getting instance name: %v", err)
		}

		gceConfig.Project = project
		gceConfig.Zone = zone
		gceConfig.Cluster = cluster
		gceConfig.Instance = instance
	} else {
		gceConfig.Project = os.Getenv("PROJECT_ID")
		if gceConfig.Project == "" {
			return nil, fmt.Errorf("PROJECT_ID environment variable was empty")
		}

		gceConfig.Zone = os.Getenv("ZONE")
		if gceConfig.Zone == "" {
			return nil, fmt.Errorf("ZONE environment variable was empty")
		}

		gceConfig.Cluster = os.Getenv("CLUSTER_NAME")
		if gceConfig.Cluster == "" {
			return nil, fmt.Errorf("CLUSTER_NAME environment variable was empty")
		}

		gceConfig.Instance = os.Getenv("INSTANCE_NAME")
		if gceConfig.Instance == "" {
			return nil, fmt.Errorf("INSTANCE_NAME environment variable was empty")
		}
	}

	return gceConfig, nil
}
