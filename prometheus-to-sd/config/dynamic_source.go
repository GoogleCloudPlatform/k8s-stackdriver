package config

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
	"github.com/golang/glog"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	podNamespace       = "kube-system"
	componentNameLabel = "k8s-app"
)

// ChangeType defines the possible types of events.
type ChangeType string

// Possible change types
const (
	Added   ChangeType = "ADDED"
	Deleted ChangeType = "DELETED"
	Unknown ChangeType = "UNKNOWN"
)

// SourceConfigChange describes how one of the dynamic sources have changed.
// Is generated when watch generates a new event.
type SourceConfigChange struct {
	ChangeType   ChangeType
	SourceConfig SourceConfig
}

// CreateDynamicSourcesStream takes source specifications and creates a watch for them.
// Every time a new event comes, it is mapped to source configs and sent through the channel returned by this function
func CreateDynamicSourcesStream(gceConfig *GceConfig, sources []flags.Uri) (<-chan SourceConfigChange, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	sourceMap, err := validateSources(sources)
	if err != nil {
		return nil, err
	}
	kubeApi, err := createKubernetesApiClient()
	if err != nil {
		return nil, err
	}
	podOptions := createOptionsForPodSelection(gceConfig.Instance, sourceMap)
	return getSourceConfigChangeChannel(kubeApi, sourceMap, podOptions)
}

func validateSources(sources flags.Uris) (map[string]url.URL, error) {
	sourceMap := make(map[string]url.URL)
	for _, source := range sources {
		if source.Val.Hostname() != "" {
			return nil, errors.New("hostname should be empty for all dynamic sources")
		}
		if source.Key == "" {
			return nil, errors.New("component name should NOT be empty for any dynamic source")
		}
		if source.Val.Port() == "" {
			return nil, errors.New("port should NOT be empty for any dynamic source")
		}
		sourceMap[source.Key] = source.Val
	}
	if len(sourceMap) != len(sources) {
		return nil, errors.New("source should have unique component names")
	}
	return sourceMap, nil
}

func createKubernetesApiClient() (clientset.Interface, error) {
	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return clientset.NewForConfig(conf)
}

func createOptionsForPodSelection(nodeName string, sources map[string]url.URL) v1.ListOptions {
	var nameList string
	for key := range sources {
		nameList += key + ","
	}
	labelSelector := fmt.Sprintf("%s in (%s)", componentNameLabel, nameList)
	listOptions := v1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
		LabelSelector: labelSelector,
	}
	glog.Infof("Filters for dynamic components: %+v", listOptions)
	return listOptions
}

func getConfigFromPod(pod core.Pod, sources map[string]url.URL) (*SourceConfig, error) {
	componentName := pod.Labels[componentNameLabel]
	source, _ := sources[componentName]
	podConfig := PodConfig{
		PodId:       pod.Name,
		NamespaceId: pod.Namespace,
	}
	return mapToSourceConfig(componentName, source, pod.Status.PodIP, podConfig)
}

func mapToSourceConfig(componentName string, url url.URL, ip string, podConfig PodConfig) (*SourceConfig, error) {
	port := url.Port()
	whitelisted := url.Query().Get("whitelisted")
	return newSourceConfig(componentName, ip, port, url.Path, whitelisted, podConfig)
}

func getSourceConfigChangeChannel(kubeApi clientset.Interface, sources map[string]url.URL, podOptions v1.ListOptions) (<-chan SourceConfigChange, error) {
	watchInterface, err := kubeApi.CoreV1().Pods(podNamespace).Watch(podOptions)
	if err != nil {
		return nil, err
	}
	sourceConfigChannel := make(chan SourceConfigChange)
	go func() {
		eventChannel := watchInterface.ResultChan()
		defer watchInterface.Stop()
		for event := range eventChannel {
			sourceConfigChange, err := mapWatchEventToSourceConfigChange(event, sources)
			if err != nil {
				glog.Warningf("dropping event because %v", err)
			}
			if sourceConfigChange != nil {
				sourceConfigChannel <- *sourceConfigChange
			}
		}
	}()
	return sourceConfigChannel, nil
}

func mapWatchEventToSourceConfigChange(event watch.Event, sources map[string]url.URL) (*SourceConfigChange, error) {
	if event.Type == watch.Error {
		return nil, fmt.Errorf("watch error with event object %+v", event.Object)
	}
	pod, ok := event.Object.(*core.Pod)
	if !ok {
		return nil, fmt.Errorf("not a pod object %+v", event.Object)
	}
	sourceConfig, err := getConfigFromPod(*pod, sources)
	if err != nil {
		return nil, fmt.Errorf("could not map pod %s to source config: %v", pod.Name, err)
	}
	if event.Type != watch.Deleted && pod.Status.PodIP == "" {
		return nil, nil
	}
	sourceConfigChange := SourceConfigChange{
		ChangeType:   getChangeType(event.Type),
		SourceConfig: *sourceConfig,
	}
	return &sourceConfigChange, nil
}

func getChangeType(eType watch.EventType) ChangeType {
	// In some cases ADDED events will not have ip address,
	// so we will take MODIFIED events with ip address as ADDED changes when the pod was scheduled
	if eType == watch.Added || eType == watch.Modified {
		return Added
	}
	if eType == watch.Deleted {
		return Deleted
	}
	return Unknown
}
