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

type monitoredResourceFactory struct {
	defaultResource *sd.MonitoredResource
	resourceModel   resourceModelVersion
	config          *monitoredResourceFactoryConfig
	labels          map[string]string
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
		config:          config,
		labels:          labels,
	}

	return factory
}
func commonLabels(config *monitoredResourceFactoryConfig) map[string]string {
	labels := make(map[string]string)
	labels[clusterName] = config.clusterName
	labels[location] = config.location
	labels[projectID] = config.projectID
	return labels
}

func (f *monitoredResourceFactory) resourceFromEvent(event *corev1.Event) *sd.MonitoredResource {
	if f.resourceModel == oldTypes {
		return f.defaultMonitoredResource()
	}

	var monitoredResource *sd.MonitoredResource

	switch event.InvolvedObject.Kind {
	case pod:
		monitoredResource = f.buildPodMonitoredResource(event)
	case node:
		monitoredResource = f.buildNodeMonitoredResource(event)
	default:
		monitoredResource = f.defaultMonitoredResource()
	}
	return monitoredResource
}

func (f *monitoredResourceFactory) defaultMonitoredResource() *sd.MonitoredResource {
	return f.defaultResource
}

func (f *monitoredResourceFactory) buildPodMonitoredResource(event *corev1.Event) *sd.MonitoredResource {
	labels := commonLabels(f.config)
	labels[podName] = event.InvolvedObject.Name
	labels[namespaceName] = event.InvolvedObject.Namespace

	return &sd.MonitoredResource{
		Type:   k8sPod,
		Labels: labels,
	}
}

func (f *monitoredResourceFactory) buildNodeMonitoredResource(event *corev1.Event) *sd.MonitoredResource {
	labels := commonLabels(f.config)
	labels[nodeName] = event.InvolvedObject.Name

	return &sd.MonitoredResource{
		Type:   k8sNode,
		Labels: labels,
	}
}
