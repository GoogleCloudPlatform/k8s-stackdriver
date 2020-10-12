package stackdriver

import (
	"reflect"
	"testing"

	"bytes"
	"fmt"
	sd "google.golang.org/api/logging/v2"
	corev1 "k8s.io/api/core/v1"
)

func TestMonitoredResourceFromEvent(t *testing.T) {
	newTypesConfig := factoryConfig(newTypes)
	oldTypesConfig := factoryConfig(oldTypes)

	tests := []struct {
		config *monitoredResourceFactoryConfig
		event  *corev1.Event
		wanted *sd.MonitoredResource
	}{
		{
			config: oldTypesConfig,
			wanted: &sd.MonitoredResource{
				Type: gkeCluster,
				Labels: map[string]string{
					clusterName: oldTypesConfig.clusterName,
					location:    oldTypesConfig.location,
					projectID:   oldTypesConfig.projectID,
				},
			},
		},
		{
			config: newTypesConfig,
			event: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: pod, Name: "test_pod_name", Namespace: "test_pod_namespace"},
			},
			wanted: &sd.MonitoredResource{
				Type: k8sPod,
				Labels: map[string]string{
					clusterName:   newTypesConfig.clusterName,
					location:      newTypesConfig.location,
					projectID:     newTypesConfig.projectID,
					podName:       "test_pod_name",
					namespaceName: "test_pod_namespace",
				},
			},
		},
		{
			config: newTypesConfig,
			event: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: node, Name: "test_node_name"},
			},
			wanted: &sd.MonitoredResource{
				Type: k8sNode,
				Labels: map[string]string{
					clusterName: newTypesConfig.clusterName,
					location:    newTypesConfig.location,
					projectID:   newTypesConfig.projectID,
					nodeName:    "test_node_name",
				},
			},
		},
		{
			config: newTypesConfig,
			event: &corev1.Event{
				InvolvedObject: corev1.ObjectReference{Kind: "somethingElse"},
			},
			wanted: &sd.MonitoredResource{
				Type: k8sCluster,
				Labels: map[string]string{
					clusterName: newTypesConfig.clusterName,
					location:    newTypesConfig.location,
					projectID:   newTypesConfig.projectID,
				},
			},
		},
	}

	for _, test := range tests {
		factory := newMonitoredResourceFactory(test.config)
		monitoredResource := factory.resourceFromEvent(test.event)

		if !reflect.DeepEqual(*monitoredResource, *test.wanted) {
			t.Errorf("Wrong monitored resource from event\ngot:\n%swanted:\n%s", stringify(monitoredResource), stringify(test.wanted))
		}
	}
}

func TestDefaultMonitoredResource(t *testing.T) {
	newTypesConfig := factoryConfig(newTypes)
	oldTypesConfig := factoryConfig(oldTypes)

	tests := []struct {
		config *monitoredResourceFactoryConfig
		wanted *sd.MonitoredResource
	}{
		{
			config: oldTypesConfig,
			wanted: &sd.MonitoredResource{
				Type: gkeCluster,
				Labels: map[string]string{
					clusterName: oldTypesConfig.clusterName,
					location:    oldTypesConfig.location,
					projectID:   oldTypesConfig.projectID,
				},
			},
		}, {
			config: newTypesConfig,
			wanted: &sd.MonitoredResource{
				Type: k8sCluster,
				Labels: map[string]string{
					clusterName: newTypesConfig.clusterName,
					location:    newTypesConfig.location,
					projectID:   newTypesConfig.projectID,
				},
			},
		},
	}

	for _, test := range tests {
		factory := newMonitoredResourceFactory(test.config)
		monitoredResource := factory.defaultMonitoredResource()

		if !reflect.DeepEqual(*monitoredResource, *test.wanted) {
			t.Errorf("Wrong monitored resource from event\ngot:\n%swanted:\n%s", stringify(monitoredResource), stringify(test.wanted))

		}
	}
}

func factoryConfig(types resourceModelVersion) *monitoredResourceFactoryConfig {
	cfg := &monitoredResourceFactoryConfig{
		resourceModel: types,
		clusterName:   "test_cluster_name",
		location:      "test_cluster_location",
		projectID:     "test_project_id",
	}
	return cfg
}

func stringify(monitoredResource *sd.MonitoredResource) string {
	var buffer bytes.Buffer

	str := fmt.Sprintf("Type: %s\n\tLabels:\n", monitoredResource.Type)
	buffer.WriteString(str)

	for key, val := range monitoredResource.Labels {
		str := fmt.Sprintf("\t\t%s:%s\n", key, val)
		buffer.WriteString(str)
	}

	return buffer.String()
}
