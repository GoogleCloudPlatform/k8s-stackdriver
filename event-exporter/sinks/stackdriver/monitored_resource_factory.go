package stackdriver

import (
	sd "google.golang.org/api/logging/v2"

	corev1 "k8s.io/api/core/v1"
)

type resourceModelVersion string

const (
	newTypes = resourceModelVersion("new")
	oldTypes = resourceModelVersion("old")
)

const (
	//sd.MonitoredResource Old types:
	gkeCluster = "gke_cluster"

	//sd.MonitoredResource New types:
	k8sCluster = "k8s_cluster"
	k8sNode    = "k8s_node"
	k8sPod     = "k8s_pod"

	//sd.MonitoredResource Labels:=
	clusterName   = "cluster_name"
	location      = "location"
	projectID     = "project_id"
	podName       = "pod_name"
	nodeName      = "node_name"
	namespaceName = "namespace_name"

	//corev1.Event InvolvedObject.Kind:
	pod  = "Pod"
	node = "Node"
)

// trustedNodeEventNamespaces enumerates the namespaces from which Node-scoped
// events are accepted for node attribution. These are system namespaces that
// are not writable by ordinary workloads under standard RBAC. Events from
// other namespaces fall back to the default cluster-scoped resource.
var trustedNodeEventNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-node-lease": true,
}

// Constructs monitored resources.
type monitoredResourceFactory struct {
	defaultResource *sd.MonitoredResource
	resourceModel   resourceModelVersion
	// Common labels shared by all monitored resources.
	commonLabels map[string]string
}

func newMonitoredResourceFactory(config *monitoredResourceFactoryConfig) *monitoredResourceFactory {
	labels := commonLabels(config)

	resource := &sd.MonitoredResource{
		Labels: labels,
	}

	if config.resourceModel == oldTypes {
		resource.Type = gkeCluster
	} else {
		resource.Type = k8sCluster
	}

	factory := &monitoredResourceFactory{
		defaultResource: resource,
		resourceModel:   config.resourceModel,
		commonLabels:    labels,
	}

	return factory
}

func (f *monitoredResourceFactory) resourceFromEvent(event *corev1.Event) *sd.MonitoredResource {
	if f.resourceModel == oldTypes {
		return f.defaultResource
	}

	switch event.InvolvedObject.Kind {
	case pod:
		// The event's own metadata.namespace is the RBAC-enforced source
		// of truth for where the event was created. Only emit a pod-scoped
		// resource when the involved object's namespace agrees with it.
		if event.Namespace != "" && event.Namespace == event.InvolvedObject.Namespace {
			return f.buildPodMonitoredResource(event)
		}
		return f.defaultResource
	case node:
		// Nodes are cluster-scoped, so there is no per-event namespace to
		// cross-check. Restrict node attribution to events originating in
		// namespaces where node-scoped events are expected.
		if trustedNodeEventNamespaces[event.Namespace] {
			return f.buildNodeMonitoredResource(event)
		}
		return f.defaultResource
	default:
		return f.defaultResource
	}
}

func (f *monitoredResourceFactory) buildPodMonitoredResource(event *corev1.Event) *sd.MonitoredResource {
	labels := copyMap(f.commonLabels)
	labels[podName] = event.InvolvedObject.Name
	labels[namespaceName] = event.Namespace

	return &sd.MonitoredResource{
		Type:   k8sPod,
		Labels: labels,
	}
}

func (f *monitoredResourceFactory) buildNodeMonitoredResource(event *corev1.Event) *sd.MonitoredResource {
	labels := copyMap(f.commonLabels)
	labels[nodeName] = event.InvolvedObject.Name

	return &sd.MonitoredResource{
		Type:   k8sNode,
		Labels: labels,
	}
}

// copyMap returns a copy of an input map.
func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func commonLabels(config *monitoredResourceFactoryConfig) map[string]string {
	labels := make(map[string]string)
	labels[clusterName] = config.clusterName
	labels[location] = config.location
	labels[projectID] = config.projectID
	return labels
}
