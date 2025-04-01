package stackdriver

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	sd "google.golang.org/api/logging/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clock "k8s.io/utils/clock/testing"
)

type mockPodLabelCollector struct{}

func (pl *mockPodLabelCollector) GetLabels(namespace, pod string) map[string]string {
	return map[string]string{
		"logging.gke.io/top_level_controller_type": "Deployment",
		"logging.gke.io/top_level_controller_name": "test_deployment_name",
	}
}

func TestFromEvent(t *testing.T) {
	newTypesConfig := factoryConfig(newTypes)
	monitoredResourceFactory := newMonitoredResourceFactory(newTypesConfig)
	involvedNodeObject := corev1.ObjectReference{Kind: node, Name: "test_node_name"}
	wantedNodeMonitoredResource := &sd.MonitoredResource{
		Type: k8sNode,
		Labels: map[string]string{
			clusterName: newTypesConfig.clusterName,
			location:    newTypesConfig.location,
			projectID:   newTypesConfig.projectID,
			nodeName:    "test_node_name",
		},
	}
	involvedPodObject := corev1.ObjectReference{Kind: pod, Name: "test_pod_name", Namespace: "test_namespace"}

	lastTimestamp := metav1.NewTime(time.Now())
	lastObservedTime := metav1.NewMicroTime(time.Now())

	tests := []struct {
		desc   string
		event  *corev1.Event
		wanted *sd.LogEntry
	}{
		{
			desc: "core/v1 event API",
			event: &corev1.Event{
				Type:           "Warning",
				InvolvedObject: involvedNodeObject,
				LastTimestamp:  lastTimestamp,
			},
			wanted: &sd.LogEntry{
				Timestamp: lastTimestamp.Format(time.RFC3339Nano),
				Resource:  wantedNodeMonitoredResource,
				Severity:  "WARNING",
			},
		},
		{
			desc: "events/v1 event API",
			event: &corev1.Event{
				Type:           "Warning",
				InvolvedObject: involvedNodeObject,
				Series: &corev1.EventSeries{
					Count:            1,
					LastObservedTime: lastObservedTime,
				},
			},
			wanted: &sd.LogEntry{
				Timestamp: lastObservedTime.Format(time.RFC3339Nano),
				Resource:  wantedNodeMonitoredResource,
				Severity:  "WARNING",
			},
		},
		{
			desc: "Timestamp not set",
			event: &corev1.Event{
				Type:           "Warning",
				InvolvedObject: involvedNodeObject,
			},
			wanted: &sd.LogEntry{
				Resource: wantedNodeMonitoredResource,
				Severity: "WARNING",
			},
		},
		{
			desc: "Event type is not set",
			event: &corev1.Event{
				InvolvedObject: involvedNodeObject,
				LastTimestamp:  lastTimestamp,
			},
			wanted: &sd.LogEntry{
				Timestamp: lastTimestamp.Format(time.RFC3339Nano),
				Resource:  wantedNodeMonitoredResource,
				Severity:  "INFO",
			},
		},
		{
			desc: "Event type is not warning",
			event: &corev1.Event{
				Type:           "Normal",
				InvolvedObject: involvedNodeObject,
				LastTimestamp:  lastTimestamp,
			},
			wanted: &sd.LogEntry{
				Timestamp: lastTimestamp.Format(time.RFC3339Nano),
				Resource:  wantedNodeMonitoredResource,
				Severity:  "INFO",
			},
		},
		{
			desc: "k8s pod event with pod labels",
			event: &corev1.Event{
				Type:           "Normal",
				InvolvedObject: involvedPodObject,
				LastTimestamp:  lastTimestamp,
			},
			wanted: &sd.LogEntry{
				Timestamp: lastTimestamp.Format(time.RFC3339Nano),
				Resource: &sd.MonitoredResource{
					Type: k8sPod,
					Labels: map[string]string{
						clusterName:   newTypesConfig.clusterName,
						location:      newTypesConfig.location,
						projectID:     newTypesConfig.projectID,
						podName:       "test_pod_name",
						namespaceName: "test_namespace",
					},
				},
				Severity: "INFO",
				Labels: map[string]string{
					"logging.gke.io/top_level_controller_type": "Deployment",
					"logging.gke.io/top_level_controller_name": "test_deployment_name",
				},
			},
		},
	}

	for _, test := range tests {
		factory := newSdLogEntryFactory(clock.NewFakeClock(time.Time{}), monitoredResourceFactory, &mockPodLabelCollector{})
		got := factory.FromEvent(test.event)
		if diff := compareLogEntries(got, test.wanted); diff != "" {
			t.Errorf("Unexpected log entry from event %v, (-want +got): %s", test.event, diff)
		}
	}
}

func compareLogEntries(got, want *sd.LogEntry) string {
	var ignore = map[string]bool{
		"JsonPayload": true,
	}
	cmpIgnoreSomeFields := cmp.FilterPath(
		func(p cmp.Path) bool {
			return ignore[p.String()]
		},
		cmp.Ignore())
	return cmp.Diff(got, want, cmpIgnoreSomeFields)
}
