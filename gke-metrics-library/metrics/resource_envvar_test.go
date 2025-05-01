package metrics

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gke-internal/gke-metrics/gke-metrics-library/envvar"
)

type fromEnvVarsTestCase struct {
	desc              string
	initializeEnvVars map[string]string
	wantResource      Resource
	wantErr           error
}

func TestNewK8sContainerFromEnvVars(t *testing.T) {
	testCases := []fromEnvVarsTestCase{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_NAME":     "cluster-name",
				"POD_NAMESPACE":    "kube-system",
				"POD_NAME":         "pod-name",
				"CONTAINER_NAME":   "container-name",
			},
			wantResource: Resource{
				schema: k8sContainer,
				labels: map[string]string{
					"project_id":     "1234567890",
					"location":       "us-central1",
					"cluster_name":   "cluster-name",
					"namespace_name": "kube-system",
					"pod_name":       "pod-name",
					"container_name": "container-name",
				},
			},
		},
		{
			desc: "Error: a mandatory env var is missing",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_NAME":     "cluster-name",
				"POD_NAMESPACE":    "kube-system",
				"CONTAINER_NAME":   "container-name",
			},
			wantErr: envvar.NewErrEmpty("POD_NAME"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}
			got, err := NewK8sContainerFromEnvVars()
			if !errors.Is(tc.wantErr, err) {
				t.Fatalf("NewK8sContainerFromEnvVars() got error %v, want %v", err, tc.wantErr)
			}
			// Skip checking the returned resource if we expect an error in the test case.
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.wantResource, got, cmp.AllowUnexported(Resource{})); diff != "" {
				t.Errorf("NewK8sContainerFromEnvVars() returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewK8sClusterFromEnvVars(t *testing.T) {
	testCases := []fromEnvVarsTestCase{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_NAME":     "cluster-name",
			},
			wantResource: Resource{
				schema: k8sCluster,
				labels: map[string]string{
					"project_id":   "1234567890",
					"location":     "us-central1",
					"cluster_name": "cluster-name",
				},
			},
		},
		{
			desc: "Error: a mandatory env var is missing",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
			},
			wantErr: envvar.NewErrEmpty("CLUSTER_NAME"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}
			got, err := NewK8sClusterFromEnvVars()
			if !errors.Is(tc.wantErr, err) {
				t.Fatalf("NewK8sClusterFromEnvVars() got error %v, want %v", err, tc.wantErr)
			}
			// Skip checking the returned resource if we expect an error in the test case.
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.wantResource, got, cmp.AllowUnexported(Resource{})); diff != "" {
				t.Errorf("NewK8sClusterFromEnvVars() returned an unexpected diff (-want +got): %v", diff)
			}
		})
	}
}

func TestNewInternalGkeMasterContainerFromEnvVars(t *testing.T) {
	testCases := []fromEnvVarsTestCase{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"HOSTED_MASTER_PROJECT": "1234567890",
				"CONSUMER_PROJECT":      "987654321",
				"CLUSTER_LOCATION":      "us-central1",
				"CLUSTER_HASH":          "cluster-hash",
				"CLUSTER_NAME":          "cluster-name",
				"POD_NAME":              "pod-name",
				"CONTAINER_NAME":        "container-name",
				"POD_NAMESPACE":         "kube-system",
			},
			wantResource: Resource{
				schema: internalGkeMasterContainer,
				labels: map[string]string{
					"project_id":          "1234567890",
					"consumer_project_id": "987654321",
					"location":            "us-central1",
					"cluster_hash":        "cluster-hash",
					"cluster_name":        "cluster-name",
					"pod_name":            "pod-name",
					"container_name":      "container-name",
					"instance_id":         "instance-id",
					"namespace_name":      "kube-system",
				},
			},
		},
		{
			desc: "Error when a mandatory env var is missing",
			initializeEnvVars: map[string]string{
				"CONSUMER_PROJECT": "987654321",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_HASH":     "cluster-hash",
				"CLUSTER_NAME":     "cluster-name",
				"POD_NAME":         "pod-name",
				"CONTAINER_NAME":   "container-name",
				"INSTANCE_ID":      "instance-id",
				"POD_NAMESPACE":    "kube-system",
			},
			wantErr: envvar.NewErrEmpty("HOSTED_MASTER_PROJECT"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}
			oldInstanceIDFromGCE := instanceIDFromGCE
			defer func() { instanceIDFromGCE = oldInstanceIDFromGCE }()
			instanceIDFromGCE = func() (string, error) {
				return "instance-id", nil
			}
			got, err := NewInternalGkeMasterContainerFromEnvVars()
			if !errors.Is(tc.wantErr, err) {
				t.Fatalf("NewInternalGkeMasterContainerFromEnvVars() got error %v, want %v", err, tc.wantErr)
			}
			// Skip checking the returned resource if we expect an error in the test case.
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.wantResource, got, cmp.AllowUnexported(Resource{})); diff != "" {
				t.Errorf("NewInternalGkeMasterContainerFromEnvVars() returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewInternalGkeMasterNodeFromEnvVars(t *testing.T) {
	testCases := []fromEnvVarsTestCase{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"HOSTED_MASTER_PROJECT": "1234567890",
				"CONSUMER_PROJECT":      "987654321",
				"CLUSTER_LOCATION":      "us-central1",
				"CLUSTER_HASH":          "cluster-hash",
				"CLUSTER_NAME":          "cluster-name",
			},
			wantResource: Resource{
				schema: internalGkeMasterNode,
				labels: map[string]string{
					"project_id":          "1234567890",
					"consumer_project_id": "987654321",
					"location":            "us-central1",
					"cluster_hash":        "cluster-hash",
					"cluster_name":        "cluster-name",
					"instance_id":         "instance-id",
				},
			},
		},
		{
			desc: "Error when a mandatory env var is missing",
			initializeEnvVars: map[string]string{
				"CONSUMER_PROJECT": "987654321",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_HASH":     "cluster-hash",
				"CLUSTER_NAME":     "cluster-name",
				"INSTANCE_ID":      "instance-id",
			},
			wantErr: envvar.NewErrEmpty("HOSTED_MASTER_PROJECT"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}
			oldInstanceIDFromGCE := instanceIDFromGCE
			defer func() { instanceIDFromGCE = oldInstanceIDFromGCE }()
			instanceIDFromGCE = func() (string, error) {
				return "instance-id", nil
			}
			got, err := NewInternalGkeMasterNodeFromEnvVars()
			if !errors.Is(tc.wantErr, err) {
				t.Fatalf("NewInternalGkeMasterNodeFromEnvVars() got error %v, want %v", err, tc.wantErr)
			}
			// Skip checking the returned resource if we expect an error in the test case.
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.wantResource, got, cmp.AllowUnexported(Resource{})); diff != "" {
				t.Errorf("NewInternalGkeMasterNodeFromEnvVars() returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewInternalGkeMasterContainerFromEnvVarsMissInstanceID(t *testing.T) {
	testCases := []struct {
		desc              string
		initializeEnvVars map[string]string
	}{
		{
			desc: "errors when getting instance ID",
			initializeEnvVars: map[string]string{
				"HOSTED_MASTER_PROJECT": "1234567890",
				"CONSUMER_PROJECT":      "987654321",
				"CLUSTER_LOCATION":      "us-central1",
				"CLUSTER_HASH":          "cluster-hash",
				"CLUSTER_NAME":          "cluster-name",
				"POD_NAME":              "pod-name",
				"CONTAINER_NAME":        "container-name",
				"POD_NAMESPACE":         "kube-system",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}

			oldInstanceIDFromGCE := instanceIDFromGCE
			defer func() { instanceIDFromGCE = oldInstanceIDFromGCE }()
			instanceIDErr := fmt.Errorf("missing instance id")
			instanceIDFromGCE = func() (string, error) {
				return "", instanceIDErr
			}
			_, err := NewInternalGkeMasterContainerFromEnvVars()
			if !errors.Is(instanceIDErr, err) {
				t.Fatalf("NewInternalGkeMasterContainerFromEnvVars() got error %v, want %v", err, instanceIDErr)
			}
		})
	}
}

func TestNewK8sNodeFromEnvVars(t *testing.T) {
	testCases := []fromEnvVarsTestCase{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_NAME":     "cluster-name",
				"NODE_NAME":        "node-name",
			},
			wantResource: Resource{
				schema: k8sNode,
				labels: map[string]string{
					"project_id":   "1234567890",
					"location":     "us-central1",
					"cluster_name": "cluster-name",
					"node_name":    "node-name",
				},
			},
		},
		{
			desc: "Error: a mandatory env var is missing",
			initializeEnvVars: map[string]string{
				"CLUSTER_PROJECT":  "1234567890",
				"CLUSTER_LOCATION": "us-central1",
				"CLUSTER_NAME":     "cluster-name",
			},
			wantErr: envvar.NewErrEmpty("NODE_NAME"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}
			got, err := NewK8sNodeFromEnvVars()
			if !errors.Is(tc.wantErr, err) {
				t.Fatalf("NewK8sNodeFromEnvVars() got error %v, want %v", err, tc.wantErr)
			}
			// Skip checking the returned resource if we expect an error in the test case.
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.wantResource, got, cmp.AllowUnexported(Resource{})); diff != "" {
				t.Errorf("NewK8sNodeFromEnvVars() returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}
