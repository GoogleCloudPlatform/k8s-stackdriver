package coreprovider

import (
	translator "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog"
	"sigs.k8s.io/metrics-server/pkg/api"
)

const (
	metricKindCPU = "CUMULATIVE"
	metricKindRAM = "GUAGE"

	containerCPUMetricName = "kubernetes.io/container/cpu/core_usage_time"
	containerRAMMetricName = "kubernetes.io/container/memory/used_bytes"
	nodeCPUMetricName      = "kubernetes.io/node/cpu/core_usage_time"
	nodeRAMMetricName      = "kubernetes.io/node/memory/used_bytes"
)

type stackdriverCoreClient struct {
	translator *translator.Translator
}

func newClient(translator *translator.Translator) *stackdriverCoreClient {
	return &stackdriverCoreClient{translator}
}

// TODO(holubowicz): handle a case when len(resourceNames) > oneOfMax
func (p *stackdriverCoreClient) getContainerCPU(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	stackdriverRequest, err := p.translator.GetSDReqForContainersWithNames(podsNames, containerCPUMetricName, metricKindCPU, labels.Everything(), translator.AllNamespaces)
	if err != nil {
		return nil, nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, nil, err
	}
	return p.translator.GetCoreContainerMetricFromResponse(stackdriverResponse)
}

func (p *stackdriverCoreClient) getContainerRAM(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	stackdriverRequest, err := p.translator.GetSDReqForContainersWithNames(podsNames, containerRAMMetricName, metricKindRAM, ramNonEvictableLabel(), translator.AllNamespaces)
	if err != nil {
		return nil, nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, nil, err
	}
	return p.translator.GetCoreContainerMetricFromResponse(stackdriverResponse)
}

func (p *stackdriverCoreClient) getNodeCPU(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	stackdriverRequest, err := p.translator.GetSDReqForNodesWithNames(nodesNames, nodeCPUMetricName, metricKindCPU, labels.Everything())
	if err != nil {
		return nil, nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, nil, err
	}
	return p.translator.GetCoreNodeMetricFromResponse(stackdriverResponse)
}

func (p *stackdriverCoreClient) getNodeRAM(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	stackdriverRequest, err := p.translator.GetSDReqForNodesWithNames(nodesNames, nodeRAMMetricName, metricKindRAM, ramNonEvictableLabel())
	if err != nil {
		return nil, nil, err
	}
	stackdriverResponse, err := stackdriverRequest.Do()
	if err != nil {
		return nil, nil, err
	}
	return p.translator.GetCoreNodeMetricFromResponse(stackdriverResponse)
}

func ramNonEvictableLabel() labels.Selector {
	ramMetricLabels := labels.Everything()
	req, err := labels.NewRequirement("metric.labels.memory_type", selection.Equals, []string{"non-evictable"})
	if err != nil {
		klog.Fatalf("Internal error. Requirement build failed. This shouldn't happen.")
	}
	return ramMetricLabels.Add(*req)
}
