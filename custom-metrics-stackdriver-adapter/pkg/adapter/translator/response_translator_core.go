package translator

import (
	"fmt"
	"time"

	stackdriver "google.golang.org/api/monitoring/v3"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/metrics-server/pkg/api"
)

// PodResult is struct for constructing the result from all received responses
type PodResult struct {
	ContainerMetric map[string]map[string]resource.Quantity
	TimeInfo        map[string]api.TimeInfo
	*Translator
}

// NodeResult is struct for constructing the result from all received responses
type NodeResult struct {
	NodeMetric map[string]resource.Quantity
	TimeInfo   map[string]api.TimeInfo
	*Translator
}

// NewPodResult creates a PodResult
func NewPodResult(t *Translator) *PodResult {
	return &PodResult{
		make(map[string]map[string]resource.Quantity),
		make(map[string]api.TimeInfo),
		t,
	}
}

// NewNodeResult creates a NodeResult
func NewNodeResult(t *Translator) *NodeResult {
	return &NodeResult{
		make(map[string]resource.Quantity),
		make(map[string]api.TimeInfo),
		t,
	}
}

// AddCoreContainerMetricFromResponse for each pod adds to map entries from response about metric and TimeInfo.
func (r *PodResult) AddCoreContainerMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) error {
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// TODO(holubowicz): consider changing request window for core metrics
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]

		metricValue, err := getQuantityValue(point)
		if err != nil {
			return err
		}

		podKey, err := r.metricKey(series)
		if err != nil {
			return err
		}
		_, ok := r.ContainerMetric[podKey]
		if !ok {
			r.ContainerMetric[podKey] = make(map[string]resource.Quantity, 0)

			intervalEndTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
			if err != nil {
				return err
			}
			r.TimeInfo[podKey] = api.TimeInfo{Timestamp: intervalEndTime, Window: r.alignmentPeriod}
		}

		containerName, ok := series.Resource.Labels["container_name"]
		if !ok {
			return apierr.NewInternalError(fmt.Errorf("Container name is not present."))
		}
		_, ok = r.ContainerMetric[podKey][containerName]
		if ok {
			return apierr.NewInternalError(fmt.Errorf("The same container appered two time in the response."))
		}
		r.ContainerMetric[podKey][containerName] = *metricValue
	}
	return nil
}

// GetCoreContainerMetricFromResponse for each pod extracts map from container name to value of metric and TimeInfo.
func (t *Translator) GetCoreContainerMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) (map[string]map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	r := NewPodResult(t)
	err := r.AddCoreContainerMetricFromResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return r.ContainerMetric, r.TimeInfo, nil
}

// AddCoreNodeMetricFromResponse for each node adds to map entries from response about metric and TimeInfo.
func (r *NodeResult) AddCoreNodeMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) error {
	for _, series := range response.TimeSeries {
		if len(series.Points) <= 0 {
			// This shouldn't happen with correct query to Stackdriver
			return apierr.NewInternalError(fmt.Errorf("Empty time series returned from Stackdriver"))
		}
		// Points in a time series are returned in reverse time order
		point := *series.Points[0]
		metricValue, err := getQuantityValue(point)
		if err != nil {
			return err
		}

		nodeName := series.Resource.Labels["node_name"]
		_, ok := r.NodeMetric[nodeName]
		if ok {
			return apierr.NewInternalError(fmt.Errorf("The same node appered two time in the response."))
		}
		r.NodeMetric[nodeName] = *metricValue

		intervalEndTime, err := time.Parse(time.RFC3339, point.Interval.EndTime)
		if err != nil {
			return err
		}
		r.TimeInfo[nodeName] = api.TimeInfo{Timestamp: intervalEndTime, Window: r.alignmentPeriod}
	}
	return nil
}

// GetCoreNodeMetricFromResponse for each node extracts value of metric and TimeInfo.
func (t *Translator) GetCoreNodeMetricFromResponse(response *stackdriver.ListTimeSeriesResponse) (map[string]resource.Quantity, map[string]api.TimeInfo, error) {
	r := NewNodeResult(t)
	err := r.AddCoreNodeMetricFromResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return r.NodeMetric, r.TimeInfo, nil
}

// getQuantityValue converts a Stackdriver metric to Metrics API one
func getQuantityValue(p stackdriver.Point) (*resource.Quantity, error) {
	switch {
	case p.Value.Int64Value != nil:
		return resource.NewQuantity(*p.Value.Int64Value, resource.DecimalSI), nil
	case p.Value.DoubleValue != nil:
		return resource.NewScaledQuantity(int64(*p.Value.DoubleValue*1000*1000), -6), nil
	default:
		return nil, apierr.NewBadRequest(fmt.Sprintf("Expected metric of type DoubleValue or Int64Value, but received TypedValue: %v", p.Value))
	}
}
