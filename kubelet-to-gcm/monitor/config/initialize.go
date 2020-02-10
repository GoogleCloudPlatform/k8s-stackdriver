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
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm/monitor"
)

const (
	gceMetaDataEndpoint = "http://169.254.169.254"
	gceMetaDataPrefix   = "/computeMetadata/v1"
)

// NewConfigs returns the SourceConfigs for all monitored endpoints, and
// hits the GCE Metadata server if required.
func NewConfigs(zone, projectID, cluster, clusterLocation, host, instance, schemaPrefix string, monitoredResourceLabels map[string]string, kubeletPort, ctrlPort uint, resolution time.Duration) (*monitor.SourceConfig, *monitor.SourceConfig, error) {
	zone, err := getZone(zone)
	if err != nil {
		return nil, nil, err
	}

	projectID, err = getProjectID(projectID)
	if err != nil {
		return nil, nil, err
	}

	cluster, err = getCluster(cluster)
	if err != nil {
		return nil, nil, err
	}

	clusterLocation, err = getClusterLocation(clusterLocation)
	if err != nil {
		return nil, nil, err
	}

	host, err = getKubeletHost(host)
	if err != nil {
		return nil, nil, err
	}

	instance, err = getInstance(instance)
	if err != nil {
		return nil, nil, err
	}

	instanceID, err := getInstanceID()
	if err != nil {
		return nil, nil, err
	}

	return &monitor.SourceConfig{
			Zone:                    zone,
			Project:                 projectID,
			Cluster:                 cluster,
			ClusterLocation:         clusterLocation,
			Host:                    host,
			Instance:                instance,
			InstanceID:              instanceID,
			SchemaPrefix:            schemaPrefix,
			MonitoredResourceLabels: monitoredResourceLabels,
			Port:                    kubeletPort,
			Resolution:              resolution,
		}, &monitor.SourceConfig{
			Zone:                    zone,
			Project:                 projectID,
			Cluster:                 cluster,
			ClusterLocation:         clusterLocation,
			Host:                    host,
			Instance:                instance,
			InstanceID:              instanceID,
			SchemaPrefix:            schemaPrefix,
			MonitoredResourceLabels: monitoredResourceLabels,
			Port:                    ctrlPort,
			Resolution:              resolution,
		}, nil
}

// metaDataURI returns the full URI for the desired resource
func metaDataURI(resource string) string {
	return gceMetaDataEndpoint + gceMetaDataPrefix + resource
}

// getGCEMetaData hits the instance's MD server.
func getGCEMetaData(uri string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request %q for GCE metadata: %v", uri, err)
	}
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed request %q for GCE metadata: %v", uri, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read body for request %q for GCE metadata: %v", uri, err)
	}
	return body, nil
}

// getZone returns zone if it's given, or gets it from gce if asked.
func getZone(zone string) (string, error) {
	if zone == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/zone"))
		if err != nil {
			return "", fmt.Errorf("Failed to get zone from GCE: %v", err)
		}
		tokens := strings.Split(string(body), "/")
		if len(tokens) < 1 {
			return "", fmt.Errorf("Failed to parse GCE response %q for instance zone.", string(body))
		}
		zone = tokens[len(tokens)-1]
	}
	return zone, nil
}

// getProjectID returns projectID if it's given, or gets it from gce if asked.
func getProjectID(projectID string) (string, error) {
	if projectID == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/project/project-id"))
		if err != nil {
			return "", fmt.Errorf("Failed to get zone from GCE: %v", err)
		}
		projectID = string(body)
	}
	return projectID, nil
}

// getCluster returns the cluster name given, or gets it from gce if asked.
func getCluster(cluster string) (string, error) {
	if cluster == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/attributes/cluster-name"))
		if err != nil {
			return "", fmt.Errorf("Failed to get cluster name from GCE: %v", err)
		}
		cluster = string(body)
	}
	return cluster, nil
}

func getClusterLocation(clusterLocation string) (string, error) {
	if clusterLocation == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/attributes/cluster-location"))
		if err != nil {
			return "", fmt.Errorf("Failed to get cluster location from GCE: %v", err)
		}
		clusterLocation = string(body)
	}
	return clusterLocation, nil
}

// getKubeletHost returns the kubelet host if given, or gets ip of network interface 0 from gce.
func getKubeletHost(kubeletHost string) (string, error) {
	if kubeletHost == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/network-interfaces/0/ip"))
		if err != nil {
			return "", fmt.Errorf("Failed to get instance IP from GCE: %v", err)
		}
		kubeletHost = string(body)
	}
	return kubeletHost, nil
}

// getInstance returns the instance name if given, or instance name from gce.
func getInstance(instance string) (string, error) {
	if instance == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/hostname"))
		if err != nil {
			return "", fmt.Errorf("Failed to get hostname from GCE: %v", err)
		}
		instance = string(body)
	}
	return strings.Split(instance, ".")[0], nil
}

func getInstanceID() (string, error) {
	body, err := getGCEMetaData(metaDataURI("/instance/id"))
	if err != nil {
		return "", fmt.Errorf("Failed to get instance id from GCE: %v", err)
	}
	return string(body), nil
}
