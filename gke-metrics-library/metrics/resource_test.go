package metrics

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	testProjectName           = "test-project-name"
	testProjectNumber         = "test-project-number"
	testConsumerProjectNumber = "test-consumer-project-number"
	testLocation              = "test-location"
	testCluster               = "test-cluster"
	testClusterHash           = "test-cluster-hash"
	testNamespace             = "test-namespace"
	testPod                   = "test-pod"
	testContainer             = "test-container"
	testInstanceID            = "test-instance-id"
	testNode                  = "test-node"
	testWorkerID              = "test-worker-id"
	testControllerAPIGroup    = "test-controller-api-group"
	testControllerKind        = "test-controller-kind"
	testController            = "test-controller"
)

func TestNewResource(t *testing.T) {
	schema := "test-schema"
	labels := map[string]string{
		"project_id": testProjectName,
	}
	want := Resource{
		schema: Schema(schema),
		labels: labels,
	}
	got := NewResource(schema, labels)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewResource() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewK8sNode(t *testing.T) {
	want := Resource{
		schema: k8sNode,
		labels: map[string]string{
			"project_id":   testProjectName,
			"location":     testLocation,
			"cluster_name": testCluster,
			"node_name":    testNode,
		},
	}
	got := NewK8sNode(testProjectName, testLocation, testCluster, testNode)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewK8sNode() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewK8sPod(t *testing.T) {
	want := Resource{
		schema: k8sPod,
		labels: map[string]string{
			"project_id":     testProjectName,
			"location":       testLocation,
			"cluster_name":   testCluster,
			"namespace_name": testNamespace,
			"pod_name":       testPod,
		},
	}
	got := NewK8sPod(testProjectName, testLocation, testCluster, testNamespace, testPod)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewK8sPod() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewK8sContainer(t *testing.T) {
	want := Resource{
		schema: k8sContainer,
		labels: map[string]string{
			"project_id":     testProjectName,
			"location":       testLocation,
			"cluster_name":   testCluster,
			"namespace_name": testNamespace,
			"pod_name":       testPod,
			"container_name": testContainer,
		},
	}
	got := NewK8sContainer(testProjectName, testLocation, testCluster, testNamespace, testPod, testContainer)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewK8sContainer() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewK8sCluster(t *testing.T) {
	want := Resource{
		schema: k8sCluster,
		labels: map[string]string{
			"project_id":   testProjectName,
			"location":     testLocation,
			"cluster_name": testCluster,
		},
	}
	got := NewK8sCluster(testProjectName, testLocation, testCluster)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewK8sCluster() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewInternalGkeMasterContainer(t *testing.T) {
	want := Resource{
		schema: internalGkeMasterContainer,
		labels: map[string]string{
			"project_id":          testProjectNumber,
			"consumer_project_id": testConsumerProjectNumber,
			"location":            testLocation,
			"cluster_hash":        testClusterHash,
			"cluster_name":        testCluster,
			"pod_name":            testPod,
			"container_name":      testContainer,
			"instance_id":         testInstanceID,
			"namespace_name":      testNamespace,
		},
	}
	got := NewInternalGkeMasterContainer(testProjectNumber, testConsumerProjectNumber, testLocation, testClusterHash, testCluster, testPod, testContainer, testInstanceID, testNamespace)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewInternalGkeMasterContainer() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewInternalGkeMasterNode(t *testing.T) {
	want := Resource{
		schema: internalGkeMasterNode,
		labels: map[string]string{
			"project_id":          testProjectNumber,
			"consumer_project_id": testConsumerProjectNumber,
			"location":            testLocation,
			"cluster_hash":        testClusterHash,
			"cluster_name":        testCluster,
			"instance_id":         testInstanceID,
		},
	}
	got := NewInternalGkeMasterNode(testProjectNumber, testConsumerProjectNumber, testLocation, testClusterHash, testCluster, testInstanceID)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewInternalGkeMasterNode() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewGCETPUWorkerResource(t *testing.T) {
	want := Resource{
		schema: gceTPUWorker,
		labels: map[string]string{
			"resource_container": testProjectName,
			"location":           testLocation,
			"worker_id":          testWorkerID,
		},
	}
	got := NewGCETPUWorkerResource(testProjectName, testLocation, testWorkerID)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewGCETPUWorkerResource() returned diff (-want, +got):\n%s", diff)
	}
}

func TestNewK8sScaleResource(t *testing.T) {
	want := Resource{
		schema: k8sScale,
		labels: map[string]string{
			"project_id":                testProjectName,
			"location":                  testLocation,
			"cluster_name":              testCluster,
			"namespace_name":            testNamespace,
			"controller_api_group_name": testControllerAPIGroup,
			"controller_kind":           testControllerKind,
			"controller_name":           testController,
		},
	}
	got := NewK8sScaleResource(testProjectName, testLocation, testCluster, testNamespace, testControllerAPIGroup, testControllerKind, testController)
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(Resource{})); diff != "" {
		t.Errorf("NewK8sScaleResource() returned diff (-want, +got):\n%s", diff)
	}
}

func TestProjectNumber(t *testing.T) {
	testCases := []struct {
		desc     string
		resource Resource
		want     string
	}{
		{
			desc: "ok",
			resource: Resource{
				labels: map[string]string{"project_id": testProjectName},
			},
			want: testProjectName,
		},
		{
			desc: "ok for tpu worker",
			resource: Resource{
				schema: gceTPUWorker,
				labels: map[string]string{"resource_container": testProjectName},
			},
			want: testProjectName,
		},
		{
			desc: "ok for fastbind gateway container",
			resource: Resource{
				schema: FastbindGatewayContainer,
				labels: map[string]string{"resource_container": testProjectName},
			},
			want: testProjectName,
		},
		{
			desc:     "no project_id label",
			resource: Resource{labels: map[string]string{}},
			want:     "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.resource.ProjectNumber()
			if got != tc.want {
				t.Errorf("ProjectID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConsumerProjectNumber(t *testing.T) {
	testConsumerProjectNumber := "test-consumer-project-number"
	testCases := []struct {
		desc     string
		resource Resource
		want     string
	}{
		{
			desc: "ok",
			resource: Resource{
				labels: map[string]string{"consumer_project_id": testConsumerProjectNumber},
			},
			want: testConsumerProjectNumber,
		},
		{
			desc:     "no consumer_project_id label",
			resource: Resource{labels: map[string]string{}},
			want:     "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tc.resource.ConsumerProjectNumber()
			if got != tc.want {
				t.Errorf("ConsumerProjectNumber() = %q, want %q", got, tc.want)
			}
		})
	}
}
