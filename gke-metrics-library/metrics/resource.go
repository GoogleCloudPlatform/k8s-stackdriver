package metrics

// Schema is a type that defines the metric schema.
type Schema string

const (
	k8sNode                    Schema = "k8s_node"
	k8sPod                     Schema = "k8s_pod"
	k8sContainer               Schema = "k8s_container"
	k8sCluster                 Schema = "k8s_cluster"
	k8sScale                   Schema = "k8s_scale"
	internalGkeMasterContainer Schema = "internal_gke_master_container"
	internalGkeMasterNode      Schema = "internal_gke_master_node"
	gceTPUWorker               Schema = "tpu.googleapis.com/GceTpuWorker"

	// FastbindGatewayContainer schema is meant for use by the fastbind-gateway control plane.
	FastbindGatewayContainer Schema = "fastbindgateway.googleapis.com/Container"
)

// Resource contains the schema and resource labels.
type Resource struct {
	schema Schema
	labels map[string]string
}

// NewResource creates a metrics resource based on the schema and labels provided.
func NewResource(schema string, labels map[string]string) Resource {
	return Resource{
		schema: Schema(schema),
		labels: labels,
	}
}

// NewK8sNode creates a k8s node resource. The "project_id" takes a project number as value, which
// is the requirement in Cloud Monitoring.
func NewK8sNode(projectNumber, location, clusterName, nodeName string) Resource {
	return Resource{
		schema: k8sNode,
		labels: map[string]string{
			"project_id":   projectNumber,
			"location":     location,
			"cluster_name": clusterName,
			"node_name":    nodeName,
		},
	}
}

// NewK8sPod creates a k8s pod resource. The "project_id" takes a project number as value, which is
// the requirement in Cloud Monitoring.
func NewK8sPod(projectNumber, location, clusterName, namespaceName, podName string) Resource {
	return Resource{
		schema: k8sPod,
		labels: map[string]string{
			"project_id":     projectNumber,
			"location":       location,
			"cluster_name":   clusterName,
			"namespace_name": namespaceName,
			"pod_name":       podName,
		},
	}
}

// NewK8sContainer creates a k8s container resource. The "project_id" takes a project number as
// value, which is the requirement in Cloud Monitoring.
func NewK8sContainer(projectNumber, location, clusterName, namespaceName, podName, containerName string) Resource {
	return Resource{
		schema: k8sContainer,
		labels: map[string]string{
			"project_id":     projectNumber,
			"location":       location,
			"cluster_name":   clusterName,
			"namespace_name": namespaceName,
			"pod_name":       podName,
			"container_name": containerName,
		},
	}
}

// NewK8sCluster creates a k8s cluster resource. The "project_id" takes a project number as value,
// which is the requirement in Cloud Monitoring.
func NewK8sCluster(projectNumber, location, clusterName string) Resource {
	return Resource{
		schema: k8sCluster,
		labels: map[string]string{
			"project_id":   projectNumber,
			"location":     location,
			"cluster_name": clusterName,
		},
	}
}

// NewInternalGkeMasterContainer creates an InternalGkeMasterContainer resource.
// The "project_id" takes a project number as value, which is the requirement in Cloud Monitoring.
func NewInternalGkeMasterContainer(projectNumber, consumerProjectNumber, location, clusterHash, clusterName, podName, containerName, instanceID, namespaceName string) Resource {
	return Resource{
		schema: internalGkeMasterContainer,
		labels: map[string]string{
			"project_id":          projectNumber,
			"consumer_project_id": consumerProjectNumber,
			"location":            location,
			"cluster_hash":        clusterHash,
			"cluster_name":        clusterName,
			"pod_name":            podName,
			"container_name":      containerName,
			"instance_id":         instanceID,
			"namespace_name":      namespaceName,
		},
	}
}

// NewInternalGkeMasterNode creates an InternalGkeMasterNode resource.
// The "project_id" takes a project number as value, which is the requirement in Cloud Monitoring.
func NewInternalGkeMasterNode(projectNumber, consumerProjectNumber, location, clusterHash, clusterName, instanceID string) Resource {
	return Resource{
		schema: internalGkeMasterNode,
		labels: map[string]string{
			"project_id":          projectNumber,
			"consumer_project_id": consumerProjectNumber,
			"location":            location,
			"cluster_hash":        clusterHash,
			"cluster_name":        clusterName,
			"instance_id":         instanceID,
		},
	}
}

// NewGCETPUWorkerResource creates a new GCE Tpu Worker resource. The "project_id" takes a project number as
// value, which is the requirement in Cloud Monitoring. See go/cloud-tpu-platform-telemetry-integration-prd
// for more information.
func NewGCETPUWorkerResource(projectNumber, location, workerID string) Resource {
	return Resource{
		schema: gceTPUWorker,
		labels: map[string]string{
			"resource_container": projectNumber,
			"location":           location,
			"worker_id":          workerID,
		},
	}
}

// NewK8sScaleResource creates a new k8s_scale resource.
// See https://cloud.google.com/monitoring/api/resources#tag_k8s_scale for more information.
func NewK8sScaleResource(projectNumber, location, cluster, namespace, controllerAPIGroup, controllerKind, controllerName string) Resource {
	return Resource{
		schema: k8sScale,
		labels: map[string]string{
			"project_id":                projectNumber,
			"location":                  location,
			"cluster_name":              cluster,
			"namespace_name":            namespace,
			"controller_api_group_name": controllerAPIGroup,
			"controller_kind":           controllerKind,
			"controller_name":           controllerName,
		},
	}
}

// ProjectNumber returns the project number.
func (r *Resource) ProjectNumber() string {
	var (
		projectNumber string
		ok            bool
	)
	if r.schema == gceTPUWorker || r.schema == FastbindGatewayContainer {
		projectNumber, ok = r.labels["resource_container"]
	} else {
		projectNumber, ok = r.labels["project_id"]
	}

	if !ok {
		return ""
	}
	return projectNumber
}

// ConsumerProjectNumber returns the consumer project number, if it exists in the schema labels.
// This field is usually populated for the KCP schemas (e.g., InternalGKEMaster.*)
func (r *Resource) ConsumerProjectNumber() string {
	consumerProjectNumber, ok := r.labels["consumer_project_id"]
	if !ok {
		return ""
	}
	return consumerProjectNumber
}
