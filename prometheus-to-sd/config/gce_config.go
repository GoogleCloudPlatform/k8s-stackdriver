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
	"strings"

	gce "cloud.google.com/go/compute/metadata"
	"github.com/golang/glog"
)

// GceConfig aggregates all GCE related configuration parameters.
type GceConfig struct {
	Project         string
	Zone            string
	Cluster         string
	ClusterLocation string
	// This is actually instance name.
	Instance   string
	InstanceId string
}

// GetGceConfig builds GceConfig based on the provided prefix and metadata server available on GCE.
func GetGceConfig(project, cluster, clusterLocation, zone, node string) (*GceConfig, error) {
	if project != "" {
		glog.Infof("Using metadata all from flags")
		if cluster == "" {
			glog.Warning("Cluster name was not set. This can be set with --cluster-name")
		}
		if clusterLocation == "" {
			glog.Warning("Cluster location was not set. This can be set with --cluster-location")
		}
		if zone == "" {
			// zone is only used by the older gke_container
			glog.Info("Zone was not set. This can be set with --zone-override")
		}
		if node == "" {
			glog.Warning("Node was not set. This can be set with --node-name")
		}
		return &GceConfig{
			Project:         project,
			Zone:            zone,
			Cluster:         cluster,
			ClusterLocation: clusterLocation,
			Instance:        node,
		}, nil
	}

	if !gce.OnGCE() {
		return nil, fmt.Errorf("Not running on GCE.")
	}

	var err error
	if project == "" {
		project, err = gce.ProjectID()
		if err != nil {
			return nil, fmt.Errorf("error while getting project id: %v", err)
		}
	}

	if cluster == "" {
		cluster, err = gce.InstanceAttributeValue("cluster-name")
		if err != nil {
			return nil, fmt.Errorf("error while getting cluster name: %v", err)
		}
		cluster = strings.TrimSpace(cluster)
		if cluster == "" {
			return nil, fmt.Errorf("cluster-name metadata was empty")
		}
	}

	// instance/name endpoint is not available on the GKE metadata server.
	// Try GCE instance/name endpoint. If error, try instance/hostname.
	// If instance/hostname, remove domain to replicate instance/name.
	if node == "" {
		node, err = gce.InstanceName()
		if err != nil {
			node, err = gce.Hostname()
			if err != nil {
				return nil, fmt.Errorf("error while getting instance (node) name: %v", err)
			}
			node = strings.Split(node, ".")[0]
		}
	}

	instanceId, err := gce.InstanceID()
	if err != nil {
		return nil, fmt.Errorf("error while getting instance id: %v", err)
	}

	if zone == "" {
		zone, err = gce.Zone()
		if err != nil {
			return nil, fmt.Errorf("error while getting zone: %v", err)
		}
	}

	if clusterLocation == "" {
		clusterLocation, err = gce.InstanceAttributeValue("cluster-location")
		if err != nil {
			return nil, fmt.Errorf("error while getting cluster location: %v", err)
		}
		clusterLocation = strings.TrimSpace(clusterLocation)
		if clusterLocation == "" {
			return nil, fmt.Errorf("cluster-location metadata was empty")
		}
	}

	return &GceConfig{
		Project:         project,
		Zone:            zone,
		Cluster:         cluster,
		ClusterLocation: clusterLocation,
		Instance:        node,
		InstanceId:      instanceId,
	}, nil
}
