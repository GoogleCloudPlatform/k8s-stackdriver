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

package config

import (
	"fmt"

	gce "cloud.google.com/go/compute/metadata"
	"github.com/golang/glog"
	"strings"
)

// GceConfig aggregates all GCE related configuration parameters.
type GceConfig struct {
	Project  string
	Location string
	Cluster  string
	Instance string
}

// GetGceConfig builds GceConfig based on the provided prefix and metadata server available on GCE.
func GetGceConfig() (*GceConfig, error) {
	if !gce.OnGCE() {
		return nil, fmt.Errorf("Not running on GCE.")
	}

	project, err := gce.ProjectID()
	if err != nil {
		return nil, fmt.Errorf("error while getting project id: %v", err)
	}

	location, err := gce.InstanceAttributeValue("cluster-location")
	if err != nil {
		glog.Warningf("Failed to retrieve cluster location, falling back to local zone: %s", err)
		location, err = gce.Zone()
		if err != nil {
			return nil, fmt.Errorf("error while getting cluster location: %v", err)
		}
	}

	cluster, err := gce.InstanceAttributeValue("cluster-name")
	if err != nil {
		return nil, fmt.Errorf("error while getting cluster name: %v", err)
	}

	instance, err := gce.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error while getting instance hostname: %v", err)
	}

	return &GceConfig{
		Project: project,
		// Trim trailing '\n' from parameters obtained from instance attributes
		Location: strings.TrimSpace(location),
		Cluster:  strings.TrimSpace(cluster),
		Instance: instance,
	}, nil
}
