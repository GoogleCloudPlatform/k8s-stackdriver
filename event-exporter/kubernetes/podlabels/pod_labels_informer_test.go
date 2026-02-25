package podlabels

import (
	"context"
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
		{
			description: "correct jobset owner, restart attempt and uid labels returned for pod",
			pod: makePod(mockPodOptions{
				Labels: map[string]string{
					"jobset.sigs.k8s.io/jobset-name":     "training-sample",
					"jobset.sigs.k8s.io/restart-attempt": "5",
					"jobset.sigs.k8s.io/jobset-uid":      "fake-uid",
				},
				OwnerReference: metav1.OwnerReference{
					Kind: "Job",
					Name: "training-sample-0",
				},
			}),
			wantLabels: map[string]string{
				"logging.gke.io/top_level_controller_type": "JobSet",
				"logging.gke.io/top_level_controller_name": "training-sample",
				"jobset.sigs.k8s.io/restart-attempt":       "5",
				"jobset.sigs.k8s.io/jobset-uid":            "fake-uid",
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
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "DaemonSet",
					Name: "my-agent",
				}},
			}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pod",
				Namespace: "default",
			}})
	factory := NewPodLabelsSharedInformerFactory(fakeclient, nil)
	collector := factory.NewPodLabelsSharedInformer()
	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Run(stopCh)

	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "add"})); count != 2 {
		t.Errorf("At this point, cacheOpsCount with operation=add should be 2, but got %d", count)
	}

	labels := collector.GetLabels("default", "my-agent-12abc") // query hit +1
	if len(labels) != 2 {
		t.Errorf("GetLabels() #1 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("non-existing-namespace", "non-existing-pod") // query miss +1
	if labels != nil {
		t.Errorf("GetLabels() #2 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-pod") // query hit +1
	if labels != nil {
		t.Errorf("GetLabels() #3 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-pod") // query hit +1
	if labels != nil {
		t.Errorf("GetLabels() #4 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("default", "my-agent-12abc") // query hit +1
	if len(labels) != 2 {
		t.Errorf("GetLabels() #5 returned unexpected labels %v", labels)
	}
	labels = collector.GetLabels("non-existing-namespace", "non-existing-pod") // query miss +1
	if labels != nil {
		t.Errorf("GetLabels() #4 returned unexpected labels %v", labels)
	}

	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "querymiss"})); count != 2 {
		t.Errorf("At this point, cacheOpsCount with operation=querymiss should be 2, but got %d", count)
	}

	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "DaemonSet",
				Name: "some-name",
			}},
		},
	}

	ctx := context.TODO()
	_, err := fakeclient.CoreV1().Pods("default").Create(ctx, newPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unexpected error creating pod: %v", err)
	}

	time.Sleep(time.Second)

	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "add"})); count != 3 {
		t.Errorf("At this point, cacheOpsCount with operation=add should be 3, but got %d", count)
	}

	labels = collector.GetLabels("default", "test-pod") // query hit +1
	if len(labels) != 2 {
		t.Errorf("GetLabels() #1 returned unexpected labels %v", labels)
	}
	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "queryhit"})); count != 5 {
		t.Errorf("At this point, cacheOpsCount with operation=queryhit should be 5, but got %d", count)
	}

	err = fakeclient.CoreV1().Pods("default").Delete(ctx, "test-pod", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("unexpected error deleting pod: %v", err)
	}

	time.Sleep(time.Second)

	if count := intValueOfMetric(cacheOpsCount.With(prometheus.Labels{"operation": "evict"})); count != 1 {
		t.Errorf("At this point, cacheOpsCount with operation=evict should be 1, but got %d", count)
	}
}
