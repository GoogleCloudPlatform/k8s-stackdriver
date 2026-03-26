package stackdriver

import (
	"cloud.google.com/go/compute/metadata"
	"fmt"
	"github.com/golang/glog"
	"strings"
)

// Provides configurations to the `monitoredResourceFactory`.
type monitoredResourceFactoryConfig struct {
	resourceModel resourceModelVersion
	clusterName   string
	location      string
	projectID     string
}

func newMonitoredResourceFactoryConfig(resourceModelVersion string) (*monitoredResourceFactoryConfig, error) {
	clusterName, err := metadata.InstanceAttributeValue("cluster-name")
	if err != nil {
		glog.Warningf("'cluster-name' label is not specified on the VM, defaulting to the empty value")
		clusterName = ""
	}
	clusterName = strings.TrimSpace(clusterName)

	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil, fmt.Errorf("failed to get project id: %v", err)
	}

	location, err := metadata.InstanceAttributeValue("cluster-location")
	location = strings.TrimSpace(location)
	if err != nil || location == "" {
		glog.Warningf("Failed to retrieve cluster location, falling back to local zone: %s", err)
		location, err = metadata.Zone()
		if err != nil {
			return nil, fmt.Errorf("error while getting cluster location: %v", err)
		}
	}

	sdResourceModel := getResourceModelVersion(resourceModelVersion)
	return &monitoredResourceFactoryConfig{
		resourceModel: sdResourceModel,
		clusterName:   clusterName,
		location:      location,
		projectID:     projectID,
	}, nil
}

func getResourceModelVersion(model string) resourceModelVersion {
	if resourceModelVersion(model) == newTypes {
		return newTypes
	}
	return oldTypes
}
