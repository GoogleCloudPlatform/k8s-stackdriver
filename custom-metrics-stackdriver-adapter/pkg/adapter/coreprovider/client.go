package coreprovider

import (
	translator "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	stackdriver "google.golang.org/api/monitoring/v3"
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

// For testing proposes
type doRequestFunction interface {
	do(*stackdriver.ProjectsTimeSeriesListCall) (*stackdriver.ListTimeSeriesResponse, error)
}
type regularDo struct{}

func (d *regularDo) do(stackdriverRequest *stackdriver.ProjectsTimeSeriesListCall) (*stackdriver.ListTimeSeriesResponse, error) {
	return stackdriverRequest.Do()
}

type stackdriverCoreClient struct {
	translator *translator.Translator
	doRequest  doRequestFunction
}

func newClient(translator *translator.Translator) *stackdriverCoreClient {
	return &stackdriverCoreClient{
		translator: translator,
		doRequest:  &regularDo{},
	}
}

func (p *stackdriverCoreClient) getPodMetric(podsNames []string, metricName string, metricKind string, labels labels.Selector) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	numOfRequests := (len(podsNames) + translator.MaxNumOfArgsInOneOfFilter - 1) / translator.MaxNumOfArgsInOneOfFilter // ceil
	r := translator.NewPodResult(p.translator)

	for i := 0; i < numOfRequests; i++ {
		segmentBeg := i * translator.MaxNumOfArgsInOneOfFilter
		segmentEnd := min((i+1)*translator.MaxNumOfArgsInOneOfFilter, len(podsNames))
		stackdriverRequest, err := p.translator.GetSDReqForContainersWithNames(podsNames[segmentBeg:segmentEnd], metricName, metricKind, labels, translator.AllNamespaces)
		if err != nil {
			return nil, nil, err
		}
		response, err := p.doRequest.do(stackdriverRequest) // TODO: make this calls parallel
		if err != nil {
			return nil, nil, err
		}
		err = r.AddCoreContainerMetricFromResponse(response)
		if err != nil {
			return nil, nil, err
		}
	}

	return r.ContainerMetric, r.TimeInfo, nil
}

func (p *stackdriverCoreClient) getContainerCPU(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	return p.getPodMetric(podsNames, containerCPUMetricName, metricKindCPU, labels.Everything())
}

func (p *stackdriverCoreClient) getContainerRAM(podsNames []string) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	return p.getPodMetric(podsNames, containerRAMMetricName, metricKindRAM, ramNonEvictableLabel())
}

func (p *stackdriverCoreClient) getNodeMetric(nodeNames []string, metricName string, metricKind string, labels labels.Selector) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	numOfRequests := (len(nodeNames) + translator.MaxNumOfArgsInOneOfFilter - 1) / translator.MaxNumOfArgsInOneOfFilter // ceil
	r := translator.NewNodeResult(p.translator)

	for i := 0; i < numOfRequests; i++ {
		segmentBeg := i * translator.MaxNumOfArgsInOneOfFilter
		segmentEnd := min((i+1)*translator.MaxNumOfArgsInOneOfFilter, len(nodeNames))
		stackdriverRequest, err := p.translator.GetSDReqForNodesWithNames(nodeNames[segmentBeg:segmentEnd], metricName, metricKind, labels)
		if err != nil {
			return nil, nil, err
		}
		response, err := p.doRequest.do(stackdriverRequest) // TODO: make this calls parallel
		if err != nil {
			return nil, nil, err
		}
		err = r.AddCoreNodeMetricFromResponse(response)
		if err != nil {
			return nil, nil, err
		}
	}

	return r.NodeMetric, r.TimeInfo, nil
}

func (p *stackdriverCoreClient) getNodeCPU(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	return p.getNodeMetric(nodesNames, nodeCPUMetricName, metricKindCPU, labels.Everything())
}

func (p *stackdriverCoreClient) getNodeRAM(nodesNames []string) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	return p.getNodeMetric(nodesNames, nodeRAMMetricName, metricKindRAM, ramNonEvictableLabel())
}

func ramNonEvictableLabel() labels.Selector {
	ramMetricLabels := labels.Everything()
	req, err := labels.NewRequirement("metric.labels.memory_type", selection.Equals, []string{"non-evictable"})
	if err != nil {
		klog.Fatalf("Internal error. Requirement build failed. This shouldn't happen.")
	}
	return ramMetricLabels.Add(*req)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
