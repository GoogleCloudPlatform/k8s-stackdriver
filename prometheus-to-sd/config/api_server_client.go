package config

import (
	"errors"
	"fmt"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

const (
	labelForMonitoredPods = "prom-to-sd.googleapis.com=true"
)

// GetConfigsFromApiServer takes pod specifications from the Kubernetes API and maps them to source configs
func GetConfigsFromApiServer(gceConfig *GceConfig, namespace string) ([]SourceConfig, error) {
	podsApi, err := setupKubernetesPodsApi(namespace)
	if err != nil {
		return nil, err
	}
	podResponse, err := podsApi.List(v1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", gceConfig.Instance),
		LabelSelector: labelForMonitoredPods,
	})
	if err != nil {
		return nil, err
	}
	pods := podResponse.Items
	if len(pods) < 0 {
		return nil, errors.New("no pods for monitoring were found")
	}
	return getConfigsFromPods(pods)
}

func setupKubernetesPodsApi(namespace string) (corev1.PodInterface, error) {
	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	return clientSet.CoreV1().Pods(namespace), nil
}

func getConfigsFromPods(pods []core.Pod) ([]SourceConfig, error) {
	var sourceConfigs []SourceConfig
	for _, pod := range pods {
		ip := pod.Status.PodIP
		podConfig := PodConfig{
			PodId:       pod.Name,
			NamespaceId: pod.Namespace,
		}
		port := pod.Annotations["prom-to-sd.googleapis.com/port"]
		whitelisted := pod.Annotations["prom-to-sd.googleapis.com/whitelisted"]
		sourceConfig, err := newSourceConfig(pod.Name, ip, port, defaultMetricsPath, whitelisted, podConfig)
		if err != nil {
			message := fmt.Sprintf("error with pod %s: %v", pod.Name, err)
			return nil, errors.New(message)
		}
		sourceConfigs = append(sourceConfigs, *sourceConfig)
	}
	return sourceConfigs, nil
}
