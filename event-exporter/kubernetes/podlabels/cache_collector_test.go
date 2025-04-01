package podlabels

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type mockPodOptions struct {
	Labels         map[string]string
	OwnerReference metav1.OwnerReference
}

func makePod(o mockPodOptions) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Labels = o.Labels
	pod.OwnerReferences = []metav1.OwnerReference{o.OwnerReference}
	return pod
}

func TestGetLabelsFromPod(t *testing.T) {
	testCases := []struct {
		description string
		pod         *corev1.Pod
		wantLabels  map[string]string
	}{
		{
			description: "pod without valid owner type should not get any label",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"k8s-super-app": "baz",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "CustomerType",
					Name: "foo-agent",
				},
			}),
			wantLabels: nil,
		},
		{
			description: "correct daemonset owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"k8s-super-app": "baz",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "DaemonSet",
					Name: "foo-agent",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "DaemonSet",
				"logging.gke.io/top_level_controller_name": "foo-agent",
			},
		},
		{
			description: "correct deployment owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"pod-template-hash": "56ccbfc78f",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "ReplicaSet",
					Name: "foo-56ccbfc78f",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "Deployment",
				"logging.gke.io/top_level_controller_name": "foo",
			},
		},
		{
			description: "replicaset not owned by deployment should not get owner label",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"pod-template-hash": "56ccbfc78f",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "ReplicaSet",
					Name: "foo",
				},
			}),
			wantLabels: nil,
		},
		{
			description: "correct statefulset owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"k8s-super-app": "baz",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "StatefulSet",
					Name: "foo",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "StatefulSet",
				"logging.gke.io/top_level_controller_name": "foo",
			},
		},
		{
			description: "correct cronjob owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"k8s-super-app": "baz",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "Job",
					Name: "build-foo-28352157",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "CronJob",
				"logging.gke.io/top_level_controller_name": "build-foo",
			},
		},
		{
			description: "correct job owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"k8s-super-app": "baz",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "Job",
					Name: "build-foo-123",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "Job",
				"logging.gke.io/top_level_controller_name": "build-foo-123",
			},
		},
		{
			description: "correct jobset owner label returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"jobset.sigs.k8s.io/jobset-name": "training-sample",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "Job",
					Name: "training-sample-0",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "JobSet",
				"logging.gke.io/top_level_controller_name": "training-sample",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			gotLabels := getLabelsFromPod(tc.pod)
			if len(tc.wantLabels) != len(gotLabels) {
				t.Errorf("getLabelsFromPod() got unexpected labels %v, want %v", gotLabels, tc.wantLabels)
			}
			for k, v := range tc.wantLabels {
				if gotLabels[k] != v {
					t.Errorf("getLabelsFromPod() got unexpected value for %q, got %q, want %q", k, gotLabels[k], v)
				}
			}
		})
	}
}

func intValueOfMetric(c prometheus.Collector) int {
	return int(math.Round(testutil.ToFloat64(c)))
}

func TestGetLabelsCacheOperations(t *testing.T) {
	fakeclient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-agent-12abc",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{metav1.OwnerReference{
					Kind: "DaemonSet",
					Name: "my-agent",
				}},
			}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pod",
				Namespace: "default",
			}})
	collector, err := NewCollector(fakeclient, nil, 3, 3, 10*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("NewCollector() failed with %v", err)
	}
	labels := collector.GetLabels("default", "my-agent-12abc") // query miss +1, empty label query miss +1, get pod OK +1, cache add +1
	if len(labels) != 2 {
		t.Errorf("GetLabels() #1 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("non-existing-namespace", "non-existing-pod") // query miss +1, empty label query miss +1, get pod NotFound +1, empty label cache add +1
	if labels != nil {
		t.Errorf("GetLabels() #2 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-pod") // query miss +1, empty label query miss +1, get pod OK +1, empty label cache add +1
	if labels != nil {
		t.Errorf("GetLabels() #3 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-pod") // query miss +1, empty label query hit +1
	if labels != nil {
		t.Errorf("GetLabels() #4 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-agent-12abc") // query hit +1
	if len(labels) != 2 {
		t.Errorf("GetLabels() #5 returned unexpected labels %v", labels)
	}
	collector.emptyLabelPodCacheTTL = 0
	labels = collector.GetLabels("non-existing-namespace", "non-existing-pod") // query miss +1, empty label query expire +1, get pod NotFound +1, empty label cache add +1
	if labels != nil {
		t.Errorf("GetLabels() #4 returned unexpected labels %v", labels)
	}

	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "add"})); count != 1 {
		t.Errorf("At this point, cacheOpsCount with operation=add should be 1, but got %d", count)
	}
	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "queryhit"})); count != 1 {
		t.Errorf("At this point, cacheOpsCount with operation=queryhit should be 1, but got %d", count)
	}
	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "querymiss"})); count != 5 {
		t.Errorf("At this point, cacheOpsCount with operation=querymiss should be 5, but got %d", count)
	}
	if count := intValueOfMetric(podGetCount.With(prometheus.Labels{"status": "OK"})); count != 2 {
		t.Errorf("At this point, podGetCount with status=OK should be 2, but got %d", count)
	}
	if count := intValueOfMetric(podGetCount.With(prometheus.Labels{"status": "NotFound"})); count != 2 {
		t.Errorf("At this point, podGetCount with status=NotFound should be 2, but got %d", count)
	}
	if count := intValueOfMetric(noLabelPodCacheOpsCount.With(prometheus.Labels{"operation": "add"})); count != 3 {
		t.Errorf("At this point, noLabelPodCacheOpsCount with operation=add should be 3, but got %d", count)
	}
	if count := intValueOfMetric(noLabelPodCacheOpsCount.With(prometheus.Labels{"operation": "queryhit"})); count != 1 {
		t.Errorf("At this point, noLabelPodCacheOpsCount with operation=queryhit should be 1, but got %d", count)
	}
	if count := intValueOfMetric(noLabelPodCacheOpsCount.With(prometheus.Labels{"operation": "querymiss"})); count != 3 {
		t.Errorf("At this point, noLabelPodCacheOpsCount with operation=querymiss should be 3, but got %d", count)
	}
	if count := intValueOfMetric(noLabelPodCacheOpsCount.With(prometheus.Labels{"operation": "expire"})); count != 1 {
		t.Errorf("At this point, noLabelPodCacheOpsCount with operation=expire should be 1, but got %d", count)
	}
}
