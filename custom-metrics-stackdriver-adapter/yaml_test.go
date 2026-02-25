package main

import (
	"os"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDeploymentTolerations(t *testing.T) {
	files := []string{
		"deploy/production/adapter.yaml",
		"deploy/production/adapter_new_resource_model.yaml",
		"adapter-beta.yaml",
		"deploy/staging/adapter_new_resource_model.yaml",
		"deploy/staging/adapter_old_resource_model.yaml",
		"deploy/test/adapter_new_resource_model_with_core_metrics.yaml",
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("Failed to read file %s: %v", file, err)
			}

			// Split documents by "---"
			docs := strings.Split(string(content), "\n---")
			foundDeployment := false

			for _, doc := range docs {
				if strings.TrimSpace(doc) == "" {
					continue
				}

				// Decode the YAML into a Kubernetes object
				obj, _, err := decode([]byte(doc), nil, nil)
				if err != nil {
					// Skip objects that fail to decode (e.g., parse errors or unknown kinds for client-go)
					continue
				}

				// Check if the object is a Deployment
				if deployment, ok := obj.(*appsv1.Deployment); ok {
					foundDeployment = true
					checkTolerations(t, deployment, file)
				}
			}

			if !foundDeployment {
				t.Errorf("No Deployment found in file %s", file)
			}
		})
	}
}

func checkTolerations(t *testing.T, deployment *appsv1.Deployment, file string) {
	tolerations := deployment.Spec.Template.Spec.Tolerations
	gpuFound := false
	tpuFound := false

	for _, tol := range tolerations {
		if tol.Key == "nvidia.com/gpu" && tol.Operator == "Exists" && tol.Effect == "NoSchedule" {
			gpuFound = true
		}
		if tol.Key == "cloud.google.com/gke-tpu" && tol.Operator == "Exists" && tol.Effect == "NoSchedule" {
			tpuFound = true
		}
	}

	if !gpuFound {
		t.Errorf("Deployment %s in file %s is missing GPU toleration", deployment.Name, file)
	}
	if !tpuFound {
		t.Errorf("Deployment %s in file %s is missing TPU toleration", deployment.Name, file)
	}
}
