package metrics

import (
	"fmt"

	gce "cloud.google.com/go/compute/metadata"
	"gke-internal/gke-metrics/gke-metrics-library/envvar"
)

// NewK8sContainerFromEnvVars creates a k8s container resource from the environment variables.
// You must have set the environment variables before calling this function:
//   - CLUSTER_PROJECT
//   - CLUSTER_LOCATION
//   - CLUSTER_NAME
//   - POD_NAMESPACE
//   - POD_NAME
//   - CONTAINER_NAME
//
// Otherwise this function returns an error.
func NewK8sContainerFromEnvVars() (Resource, error) {
	envVars, err := envvar.LookupEnvList([]string{
		"CLUSTER_PROJECT",
		"CLUSTER_LOCATION",
		"CLUSTER_NAME",
		"POD_NAMESPACE",
		"POD_NAME",
		"CONTAINER_NAME"})
	if err != nil {
		return Resource{}, err
	}

	return NewK8sContainer(
		envVars["CLUSTER_PROJECT"],
		envVars["CLUSTER_LOCATION"],
		envVars["CLUSTER_NAME"],
		envVars["POD_NAMESPACE"],
		envVars["POD_NAME"],
		envVars["CONTAINER_NAME"],
	), nil
}

// NewK8sClusterFromEnvVars creates a k8s cluster resource from the environment variables.
// You must have set the environment variables before calling this function:
//   - CLUSTER_PROJECT
//   - CLUSTER_LOCATION
//   - CLUSTER_NAME
//
// Otherwise this function returns an error.
func NewK8sClusterFromEnvVars() (Resource, error) {
	envvars, err := envvar.LookupEnvList([]string{
		"CLUSTER_PROJECT",
		"CLUSTER_LOCATION",
		"CLUSTER_NAME"})
	if err != nil {
		return Resource{}, err
	}

	return NewK8sCluster(
		envvars["CLUSTER_PROJECT"],
		envvars["CLUSTER_LOCATION"],
		envvars["CLUSTER_NAME"],
	), nil
}

// NewInternalGkeMasterContainerFromEnvVars creates a InternalGkeMasterContainer
// resource from the environment variables.
// You must have set the environment variables before calling this function:
//   - HOSTED_MASTER_PROJECT
//   - CONSUMER_PROJECT
//   - CLUSTER_LOCATION
//   - CLUSTER_HASH
//   - CLUSTER_NAME
//   - POD_NAME
//   - CONTAINER_NAME
//   - POD_NAMESPACE
//
// It will retrieve the KCP instance ID from the GCE Metadata API.
//
// Otherwise this function returns an error.
func NewInternalGkeMasterContainerFromEnvVars() (Resource, error) {
	envVars, err := envvar.LookupEnvList([]string{
		"HOSTED_MASTER_PROJECT",
		"CONSUMER_PROJECT",
		"CLUSTER_LOCATION",
		"CLUSTER_HASH",
		"CLUSTER_NAME",
		"POD_NAME",
		"CONTAINER_NAME",
		"POD_NAMESPACE",
	})
	if err != nil {
		return Resource{}, err
	}

	instanceID, err := instanceIDFromGCE()
	if err != nil {
		return Resource{}, err
	}

	return NewInternalGkeMasterContainer(
		envVars["HOSTED_MASTER_PROJECT"],
		envVars["CONSUMER_PROJECT"],
		envVars["CLUSTER_LOCATION"],
		envVars["CLUSTER_HASH"],
		envVars["CLUSTER_NAME"],
		envVars["POD_NAME"],
		envVars["CONTAINER_NAME"],
		instanceID,
		envVars["POD_NAMESPACE"],
	), nil
}

// NewInternalGkeMasterNodeFromEnvVars creates a InternalGkeMasterNode
// resource from the environment variables.
// You must have set the environment variables before calling this function:
//   - HOSTED_MASTER_PROJECT
//   - CONSUMER_PROJECT
//   - CLUSTER_LOCATION
//   - CLUSTER_HASH
//   - CLUSTER_NAME
//
// It will retrieve the KCP instance ID from the GCE Metadata API.
//
// Otherwise this function returns an error.
func NewInternalGkeMasterNodeFromEnvVars() (Resource, error) {
	envVars, err := envvar.LookupEnvList([]string{
		"HOSTED_MASTER_PROJECT",
		"CONSUMER_PROJECT",
		"CLUSTER_LOCATION",
		"CLUSTER_HASH",
		"CLUSTER_NAME",
	})
	if err != nil {
		return Resource{}, err
	}

	instanceID, err := instanceIDFromGCE()
	if err != nil {
		return Resource{}, err
	}

	return NewInternalGkeMasterNode(
		envVars["HOSTED_MASTER_PROJECT"],
		envVars["CONSUMER_PROJECT"],
		envVars["CLUSTER_LOCATION"],
		envVars["CLUSTER_HASH"],
		envVars["CLUSTER_NAME"],
		instanceID,
	), nil
}

// NewK8sNodeFromEnvVars creates a K8sNode
// resource from the environment variables.
// You must have set the environment variables before calling this function:
//   - CLUSTER_PROJECT
//   - CLUSTER_LOCATION
//   - CLUSTER_NAME
//   - NODE_NAME
//
// Otherwise this function returns an error.
func NewK8sNodeFromEnvVars() (Resource, error) {
	envvars, err := envvar.LookupEnvList([]string{
		"CLUSTER_PROJECT",
		"CLUSTER_LOCATION",
		"CLUSTER_NAME",
		"NODE_NAME"})
	if err != nil {
		return Resource{}, err
	}

	return NewK8sNode(
		envvars["CLUSTER_PROJECT"],
		envvars["CLUSTER_LOCATION"],
		envvars["CLUSTER_NAME"],
		envvars["NODE_NAME"],
	), nil
}

var instanceIDFromGCE = func() (string, error) {
	if !gce.OnGCE() {
		return "", fmt.Errorf("not running on GCE")
	}
	return gce.InstanceID()
}
