package coreprovider

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	sd "google.golang.org/api/monitoring/v3"
	stackdriver "google.golang.org/api/monitoring/v3"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/metrics-server/pkg/api"
)

var podtests = []struct {
	numOfPods       int
	numOfContainers int
}{
	{
		numOfPods:       1,
		numOfContainers: 1,
	},
	{
		numOfPods:       1,
		numOfContainers: 2,
	},
	{
		numOfPods:       10,
		numOfContainers: 1,
	},
	{
		numOfPods:       99,
		numOfContainers: 1,
	},
	{
		numOfPods:       100,
		numOfContainers: 1,
	},
	{
		numOfPods:       101,
		numOfContainers: 1,
	},
	{
		numOfPods:       199,
		numOfContainers: 1,
	},
	{
		numOfPods:       200,
		numOfContainers: 1,
	},
	{
		numOfPods:       201,
		numOfContainers: 1,
	},
	{
		numOfPods:       534,
		numOfContainers: 1,
	},
	{
		numOfPods:       99,
		numOfContainers: 5,
	},
	{
		numOfPods:       100,
		numOfContainers: 7,
	},
	{
		numOfPods:       201,
		numOfContainers: 3,
	},
}

var nodetests = []struct {
	numOfNodes int
}{
	{
		numOfNodes: 1,
	},
	{
		numOfNodes: 2,
	},
	{
		numOfNodes: 15,
	},
	{
		numOfNodes: 299,
	},
	{
		numOfNodes: 300,
	},
	{
		numOfNodes: 301,
	},
	{
		numOfNodes: 132,
	},
	{
		numOfNodes: 834,
	},
}

func setupForPods(metric string, numOfPods int, numOfContainers int) ([]responseEntry, []string) {
	possibleResponses := make([]responseEntry, 0)
	allPods := make([]string, 0)

	for podId := 1; podId <= numOfPods; podId++ {
		for containerId := 1; containerId <= numOfContainers; containerId++ {
			var entry responseEntry
			if metric == "CPU" {
				entry = getContainerGuageResponseEntry(podId, containerId)
			} else {
				entry = getContainerCumulativeResponseEntry(podId, containerId)
			}
			possibleResponses = append(possibleResponses, entry)
			if containerId == 1 {
				allPods = append(allPods, entry.resourceName)
			}
		}
	}
	return possibleResponses, allPods
}

func TestCoreprovider_getContainerMetric(t *testing.T) {
	tr, _ :=
		translator.NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)

	for _, metric := range []string{"CPU", "Memory"} {
		for _, tc := range podtests {
			t.Run(fmt.Sprintf("%s%+v", metric, tc), func(t *testing.T) {
				possibleResponses, allPods := setupForPods(metric, tc.numOfPods, tc.numOfContainers)
				requests := &requestedResources{make(map[string]int), 0}
				client := newFakeClient(tr, possibleResponses, requests, t)

				var metricResult map[string]map[string]resource.Quantity
				var timeResult map[string]api.TimeInfo
				var err error
				if metric == "CPU" {
					metricResult, timeResult, err = client.getContainerCPU(allPods)
				} else {
					metricResult, timeResult, err = client.getContainerRAM(allPods)
				}
				if err != nil {
					t.Fatalf("%s", err)
				}

				numberOfMinimumRequests := (len(allPods) + translator.MaxNumOfArgsInOneOfFilter - 1) / translator.MaxNumOfArgsInOneOfFilter
				if numberOfMinimumRequests != requests.countRequests { // check if client done minimum possible number of requests
					t.Errorf("Called %v Stackdriver requests, when %v request should be called", requests.countRequests, numberOfMinimumRequests)
				}
				for _, pod := range allPods {
					if requests.cntResourceRequests[pod] != 1 { // check every pod is requested once
						t.Errorf("Number of requests for pod %v is %v when it should be 1", pod, requests.cntResourceRequests[pod])
					}
				}

				for _, entry := range possibleResponses { // check if every container is included in response
					namespaceName := entry.response.Resource.Labels["namespace_name"]
					containerName := entry.response.Resource.Labels["container_name"]
					key := namespaceName + ":" + entry.resourceName

					if _, ok := metricResult[key]; ok == false {
						t.Fatalf("No response for %v metric", *entry.response.Resource)
					}
					if _, ok := metricResult[key][containerName]; ok == false {
						t.Errorf("No response for %v metric", *entry.response.Resource)
					}

					if _, ok := timeResult[key]; ok == false {
						t.Errorf("No response for %v metric", *entry.response.Resource)
					}
				}
			})
		}
	}
}

func setupForNodes(metric string, numOfNodes int) ([]responseEntry, []string) {
	possibleResponses := make([]responseEntry, 0)
	allNodes := make([]string, 0)

	for nodeId := 1; nodeId <= numOfNodes; nodeId++ {
		var entry responseEntry
		if metric == "CPU" {
			entry = getNodeGuageResponseEntry(nodeId)
		} else {
			entry = getNodeCumulativeResponseEntry(nodeId)
		}
		possibleResponses = append(possibleResponses, entry)
		allNodes = append(allNodes, entry.resourceName)
	}
	return possibleResponses, allNodes
}

func TestCoreprovider_getNodeMetric(t *testing.T) {
	tr, _ :=
		translator.NewFakeTranslator(2*time.Minute, time.Minute, "my-project", "my-cluster", "my-zone", time.Date(2017, 1, 2, 13, 2, 0, 0, time.UTC), true)

	for _, metric := range []string{"CPU", "Memory"} {
		for _, tc := range nodetests {
			t.Run(fmt.Sprintf("%s%+v", metric, tc), func(t *testing.T) {
				requests := &requestedResources{make(map[string]int), 0}
				possibleResponses, allNodes := setupForNodes(metric, tc.numOfNodes)
				client := newFakeClient(tr, possibleResponses, requests, t)

				var metricResult map[string]resource.Quantity
				var timeResult map[string]api.TimeInfo
				var err error
				if metric == "CPU" {
					metricResult, timeResult, err = client.getNodeCPU(allNodes)
				} else {
					metricResult, timeResult, err = client.getNodeRAM(allNodes)
				}

				numberOfMinimumRequests := (len(allNodes) + translator.MaxNumOfArgsInOneOfFilter - 1) / translator.MaxNumOfArgsInOneOfFilter
				if numberOfMinimumRequests != requests.countRequests { // check if client done minimum possible number of requests
					t.Errorf("Called %v Stackdriver requests, when %v request should be called", requests.countRequests, numberOfMinimumRequests)
				}
				for _, node := range allNodes { // check every node is requested once
					if requests.cntResourceRequests[node] != 1 {
						t.Errorf("Number of requests for node %v is %v when it should be 1", node, requests.cntResourceRequests[node])
					}
				}

				if err != nil {
					t.Fatalf("%s", err)
				}
				for _, entry := range possibleResponses { // check if every node is included in response
					if _, ok := metricResult[entry.resourceName]; ok == false {
						t.Errorf("No response for %v metric", *entry.response.Resource)
					}
					if _, ok := timeResult[entry.resourceName]; ok == false {
						t.Errorf("No response for %v metric", *entry.response.Resource)
					}
				}
			})
		}
	}
}

type requestedResources struct {
	cntResourceRequests map[string]int
	countRequests       int
}

type fakeDo struct {
	possibleResponses []responseEntry
	requests          *requestedResources
	t                 *testing.T
}

func (d *fakeDo) do(stackdriverRequest *stackdriver.ProjectsTimeSeriesListCall) (*stackdriver.ListTimeSeriesResponse, error) {
	requestString := fmt.Sprintf("%v", stackdriverRequest)
	requestedResources := make(map[string]int)
	d.requests.countRequests++

	responseList := make([]*sd.TimeSeries, 0)
	for _, entry := range d.possibleResponses {
		occurrences := strings.Count(requestString, entry.resourceName)
		if occurrences > 1 { // check pod/node is in the request no more than once
			d.t.Fatalf("pod %v occure more than one time in request", entry.resourceName)
		}
		if occurrences == 1 {
			responseList = append(responseList, entry.response)

			requestedResources[entry.resourceName]++
			if requestedResources[entry.resourceName] == 1 {
				d.requests.cntResourceRequests[entry.resourceName]++
			}
		}
	}
	if len(requestedResources) > translator.MaxNumOfArgsInOneOfFilter { // check if maximum number of resources in one_of is <= 100
		d.t.Fatalf("Adpter requested about %v resources in one_of when %v is the limit", len(requestedResources), translator.MaxNumOfArgsInOneOfFilter)
	}
	return &sd.ListTimeSeriesResponse{TimeSeries: responseList}, nil
}

type responseEntry struct {
	resourceName string
	response     *sd.TimeSeries
}

func newFakeClient(translator *translator.Translator, possibleResponses []responseEntry, requests *requestedResources, t *testing.T) *stackdriverCoreClient {
	return &stackdriverCoreClient{
		translator: translator,
		doRequest: &fakeDo{
			possibleResponses,
			requests,
			t,
		},
	}
}

func getContainerGuageResponseEntry(podId int, containerId int) responseEntry {
	metricValue := int64(0)
	podName := fmt.Sprintf("pod%v_", podId)
	return responseEntry{podName,
		&sd.TimeSeries{
			Resource:   &sd.MonitoredResource{Type: "k8s_pod", Labels: map[string]string{"container_name": fmt.Sprintf("container%v_", containerId), "pod_name": podName, "namespace_name": "all"}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     nil,
			Points: []*sd.Point{{
				Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"},
				Value:    &sd.TypedValue{Int64Value: &metricValue}}},
		},
	}
}
func getContainerCumulativeResponseEntry(podId int, containerId int) responseEntry {
	metricValue := float64(0)
	podName := fmt.Sprintf("pod%v_", podId)
	return responseEntry{podName,
		&sd.TimeSeries{
			Resource:   &sd.MonitoredResource{Type: "k8s_pod", Labels: map[string]string{"container_name": fmt.Sprintf("container%v_", containerId), "pod_name": podName, "namespace_name": "all"}},
			MetricKind: "CUMULATIVE",
			ValueType:  "DOUBLE",
			Metric:     nil,
			Points: []*sd.Point{{
				Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"},
				Value:    &sd.TypedValue{DoubleValue: &metricValue}}},
		},
	}
}
func getNodeGuageResponseEntry(nodeId int) responseEntry {
	metricValue := int64(0)
	nodeName := fmt.Sprintf("node%v_", nodeId)
	return responseEntry{nodeName,
		&sd.TimeSeries{
			Resource:   &sd.MonitoredResource{Type: "k8s_pod", Labels: map[string]string{"node_name": nodeName}},
			MetricKind: "GAUGE",
			ValueType:  "INT64",
			Metric:     nil,
			Points: []*sd.Point{{
				Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"},
				Value:    &sd.TypedValue{Int64Value: &metricValue}}},
		},
	}
}
func getNodeCumulativeResponseEntry(nodeId int) responseEntry {
	metricValue := float64(0)
	nodeName := fmt.Sprintf("node%v_", nodeId)
	return responseEntry{nodeName,
		&sd.TimeSeries{
			Resource:   &sd.MonitoredResource{Type: "k8s_pod", Labels: map[string]string{"node_name": nodeName}},
			MetricKind: "CUMULATIVE",
			ValueType:  "DOUBLE",
			Metric:     nil,
			Points: []*sd.Point{{
				Interval: &sd.TimeInterval{StartTime: "2017-01-02T13:01:00Z", EndTime: "2017-01-02T13:02:00Z"},
				Value:    &sd.TypedValue{DoubleValue: &metricValue}}},
		},
	}
}
