/*
Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package translator

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/golang/glog"
	dto "github.com/prometheus/client_model/go"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

const (
	// Built-in Prometheus metric exporting process start time.
	processStartTimeMetric = "process_start_time_seconds"
)

var supportedMetricTypes = map[dto.MetricType]bool{
	dto.MetricType_COUNTER:   true,
	dto.MetricType_GAUGE:     true,
	dto.MetricType_HISTOGRAM: true,
	dto.MetricType_UNTYPED:   true,
}

const falseValueEpsilon = 0.001

// TimeSeriesBuilder keeps track of incoming prometheus updates and can convert
// last received one to Stackdriver TimeSeries.
type TimeSeriesBuilder struct {
	config *config.CommonConfig
	cache  *MetricDescriptorCache
	batch  *batchWithTimestamp
}

type batchWithTimestamp struct {
	metrics   *PrometheusResponse
	timestamp time.Time
}

// NewTimeSeriesBuilder creates new builder object that keeps intermediate state of metrics.
func NewTimeSeriesBuilder(commonConfig *config.CommonConfig, cache *MetricDescriptorCache) *TimeSeriesBuilder {
	return &TimeSeriesBuilder{
		config: commonConfig,
		cache:  cache,
	}
}

// Update updates the internal state with current batch.
func (t *TimeSeriesBuilder) Update(batch *PrometheusResponse, timestamp time.Time) {
	t.batch = &batchWithTimestamp{batch, timestamp}
}

// Build returns a new TimeSeries array and restarts the internal state.
func (t *TimeSeriesBuilder) Build() ([]*v3.TimeSeries, time.Time, error) {
	var ts []*v3.TimeSeries
	if t.batch == nil {
		return ts, time.Now(), nil
	}
	defer func() { t.batch = nil }()
	metricFamilies, err := t.batch.metrics.Build(t.config, t.cache)
	if err != nil {
		return ts, time.Now(), err
	}
	// Get start time before whitelisting, because process start time
	// metric is likely not to be whitelisted.
	startTime := t.getStartTime(metricFamilies)
	metricFamilies = filterWhitelistedMetrics(metricFamilies, t.config.SourceConfig.Whitelisted)
	metricFamilies = filterWhitelistedLabels(metricFamilies, t.config.SourceConfig.WhitelistedLabelsMap)

	for name, metric := range metricFamilies {
		if t.cache.IsMetricBroken(name) {
			continue
		}
		f, err := translateFamily(t.config, metric, t.batch.timestamp, startTime, t.cache)
		if err != nil {
			glog.Warningf("Error while processing metric %s: %v", name, err)
		} else {
			ts = append(ts, f...)
		}
	}
	return ts, t.batch.timestamp, nil
}

// OmitComponentName removes from the metric names prefix that is equal to component name.
func OmitComponentName(metricFamilies map[string]*dto.MetricFamily, componentName string) map[string]*dto.MetricFamily {
	result := make(map[string]*dto.MetricFamily)
	for metricName, metricFamily := range metricFamilies {
		newMetricName := strings.TrimPrefix(metricName, fmt.Sprintf("%s_", componentName))
		metricFamily.Name = &newMetricName
		result[newMetricName] = metricFamily
	}
	return result
}

// DowncaseMetricNames downcases metric names.
func DowncaseMetricNames(metricFamilies map[string]*dto.MetricFamily) map[string]*dto.MetricFamily {
	result := make(map[string]*dto.MetricFamily)
	for metricName, metricFamily := range metricFamilies {
		newMetricName := strings.ToLower(metricName)
		metricFamily.Name = &newMetricName
		result[newMetricName] = metricFamily
	}
	return result
}

// FlattenSummaryMetricFamilies flattens summary metric families into two counter metrics,
// one for the running sum and count, respectively
func FlattenSummaryMetricFamilies(metricFamilies map[string]*dto.MetricFamily) map[string]*dto.MetricFamily {
	result := make(map[string]*dto.MetricFamily)
	for metricName, family := range metricFamilies {
		switch family.GetType() {
		case dto.MetricType_SUMMARY:
			if len(family.Metric) < 1 {
				glog.V(2).Infof("Summary metric %v does not have metric data associated, ignoring", family.Name)
				continue
			}
			result[metricName+"_sum"] = sumMetricFromSummary(family.GetName(), family.Metric)
			result[metricName+"_count"] = countMetricFromSummary(family.GetName(), family.Metric)
		default:
			result[metricName] = family
		}
	}
	return result
}

// sumMetricFromSummary manipulates a Summary to extract out a specific sum MetricType_COUNTER metric
func sumMetricFromSummary(name string, metrics []*dto.Metric) *dto.MetricFamily {
	n := name + "_sum"
	t := dto.MetricType_COUNTER
	newMetrics := make([]*dto.Metric, 0, len(metrics))
	for _, m := range metrics {
		s := m.Summary.GetSampleSum()
		newMetric := &dto.Metric{
			Label: m.Label,
			Counter: &dto.Counter{
				Value: &s,
			},
		}
		newMetrics = append(newMetrics, newMetric)
	}
	return &dto.MetricFamily{
		Type:   &t,
		Name:   &n,
		Metric: newMetrics,
	}
}

// countMetricFromSummary manipulates a Summary to extract out a specific count MetricType_COUNTER metric
func countMetricFromSummary(name string, metrics []*dto.Metric) *dto.MetricFamily {
	n := name + "_count"
	t := dto.MetricType_COUNTER

	newMetrics := make([]*dto.Metric, 0, len(metrics))
	for _, m := range metrics {
		c := float64(m.Summary.GetSampleCount())
		newMetric := &dto.Metric{
			Label: m.Label,
			Counter: &dto.Counter{
				Value: &c,
			},
		}
		newMetrics = append(newMetrics, newMetric)
	}
	return &dto.MetricFamily{
		Type:   &t,
		Name:   &n,
		Metric: newMetrics,
	}
}

func (t *TimeSeriesBuilder) getStartTime(metrics map[string]*dto.MetricFamily) time.Time {
	// For cumulative metrics we need to know process start time.
	// If the process start time is not specified, assuming it's
	// the unix 1 second, because Stackdriver can't handle
	// unix zero or unix negative number.
	startTime := time.Unix(1, 0)
	if family, found := metrics[processStartTimeMetric]; found && family.GetType() == dto.MetricType_GAUGE && len(family.GetMetric()) == 1 {
		startSec := family.Metric[0].Gauge.Value
		startTime = time.Unix(int64(*startSec), 0)
		glog.V(4).Infof("Monitored process start time: %v", startTime)
	} else {
		glog.Warningf("Metric %s invalid or not defined for component %s. Using %v instead. Cumulative metrics might be inaccurate.", processStartTimeMetric, t.config.SourceConfig.Component, startTime)
	}
	return startTime
}

func filterWhitelistedMetrics(allMetrics map[string]*dto.MetricFamily, whitelisted []string) map[string]*dto.MetricFamily {
	if len(whitelisted) == 0 {
		return allMetrics
	}
	glog.V(4).Infof("Exporting only whitelisted metrics: %v", whitelisted)
	res := map[string]*dto.MetricFamily{}
	for _, w := range whitelisted {
		if family, found := allMetrics[w]; found {
			res[w] = family
		} else {
			glog.V(3).Infof("Whitelisted metric %s not present in Prometheus endpoint.", w)
		}
	}
	return res
}

func filterWhitelistedLabels(allMetrics map[string]*dto.MetricFamily, whitelistedLabelsMap map[string]map[string]bool) map[string]*dto.MetricFamily {
	if len(whitelistedLabelsMap) == 0 {
		return allMetrics
	}
	glog.V(4).Infof("Exporting only whitelisted label values: %v", whitelistedLabelsMap)
	res := map[string]*dto.MetricFamily{}
	for metricName, metricFamily := range allMetrics {
		var filteredMetrics []*dto.Metric
		for _, metric := range metricFamily.Metric {
			labels := metric.GetLabel()
			for _, label := range labels {
				if whitelistedLabelValues, found := whitelistedLabelsMap[*label.Name]; found && whitelistedLabelValues[*label.Value] {
					filteredMetrics = append(filteredMetrics, metric)
				}
			}
		}

		if len(filteredMetrics) > 0 {
			res[metricName] = &dto.MetricFamily{
				Name:   metricFamily.Name,
				Help:   metricFamily.Help,
				Type:   metricFamily.Type,
				Metric: filteredMetrics,
			}
		} else {
			glog.V(3).Infof("Whitelisted label values for metric %s not found in Prometheus endpoint.", metricFamily)
		}
	}
	return res
}

func translateFamily(config *config.CommonConfig,
	family *dto.MetricFamily,
	timestamp time.Time,
	startTime time.Time,
	cache *MetricDescriptorCache) ([]*v3.TimeSeries, error) {

	glog.V(3).Infof("Translating metric family %v from component %v", family.GetName(), config.SourceConfig.Component)
	var ts []*v3.TimeSeries
	if _, found := supportedMetricTypes[family.GetType()]; !found {
		return ts, fmt.Errorf("metric type %v of family %s not supported", family.GetType(), family.GetName())
	}
	for _, metric := range family.GetMetric() {
		t := translateOne(config, family.GetName(), family.GetType(), metric, startTime, timestamp, cache)
		ts = append(ts, t)
		glog.V(4).Infof("%+v\nMetric: %+v, Interval: %+v", *t, *(t.Metric), t.Points[0].Interval)
	}
	return ts, nil
}

// getMetricType creates metric type name base on the metric prefix, component name and metric name.
func getMetricType(config *config.CommonConfig, name string) string {
	if config.SourceConfig.Component == "" {
		return fmt.Sprintf("%s/%s", config.SourceConfig.MetricsPrefix, name)
	}
	return fmt.Sprintf("%s/%s/%s", config.SourceConfig.MetricsPrefix, config.SourceConfig.Component, name)
}

// assumes that mType is Counter, Gauge or Histogram
func translateOne(config *config.CommonConfig,
	name string,
	mType dto.MetricType,
	metric *dto.Metric,
	start time.Time,
	end time.Time,
	cache *MetricDescriptorCache) *v3.TimeSeries {
	interval := &v3.TimeInterval{
		EndTime: end.UTC().Format(time.RFC3339),
	}
	metricKind := extractMetricKind(mType)
	if metricKind == "CUMULATIVE" {
		interval.StartTime = start.UTC().Format(time.RFC3339)
	}
	valueType := extractValueType(mType, cache.getMetricDescriptor(name))
	point := &v3.Point{
		Interval: interval,
		Value: &v3.TypedValue{
			ForceSendFields: []string{},
		},
	}
	setValue(mType, valueType, metric, point)

	mr := getMonitoredResourceFromLabels(config, metric.GetLabel())
	glog.V(4).Infof("MonitoredResource to write: %v", mr)
	return &v3.TimeSeries{
		Metric: &v3.Metric{
			Labels: getMetricLabels(config, metric.GetLabel()),
			Type:   getMetricType(config, name),
		},
		Resource:   getMonitoredResourceFromLabels(config, metric.GetLabel()),
		MetricKind: metricKind,
		ValueType:  valueType,
		Points:     []*v3.Point{point},
	}
}

func setValue(mType dto.MetricType, valueType string, metric *dto.Metric, point *v3.Point) {
	if mType == dto.MetricType_GAUGE {
		setValueBaseOnSimpleType(metric.GetGauge().GetValue(), valueType, point)
	} else if mType == dto.MetricType_HISTOGRAM {
		point.Value.DistributionValue = convertToDistributionValue(metric.GetHistogram())
		point.ForceSendFields = append(point.ForceSendFields, "DistributionValue")
	} else if mType == dto.MetricType_UNTYPED {
		setValueBaseOnSimpleType(metric.GetUntyped().GetValue(), valueType, point)
	} else {
		setValueBaseOnSimpleType(metric.GetCounter().GetValue(), valueType, point)
	}
}

func setValueBaseOnSimpleType(value float64, valueType string, point *v3.Point) {
	if valueType == "INT64" {
		val := int64(value)
		point.Value.Int64Value = &val
		point.ForceSendFields = append(point.ForceSendFields, "Int64Value")
	} else if valueType == "DOUBLE" {
		point.Value.DoubleValue = &value
		point.ForceSendFields = append(point.ForceSendFields, "DoubleValue")
	} else if valueType == "BOOL" {
		var val = math.Abs(value) > falseValueEpsilon
		point.Value.BoolValue = &val
		point.ForceSendFields = append(point.ForceSendFields, "BoolValue")
	} else {
		glog.Errorf("Value type '%s' is not supported yet.", valueType)
	}
}

func convertToDistributionValue(h *dto.Histogram) *v3.Distribution {
	count := int64(h.GetSampleCount())
	mean := float64(0)
	dev := float64(0)
	var bounds []float64
	var values []int64

	if count > 0 {
		mean = h.GetSampleSum() / float64(count)
	}

	prevVal := uint64(0)
	lower := float64(0)
	for _, b := range h.Bucket {
		upper := b.GetUpperBound()
		if !math.IsInf(b.GetUpperBound(), 1) {
			bounds = append(bounds, b.GetUpperBound())
		} else {
			upper = lower
		}
		val := b.GetCumulativeCount() - prevVal
		x := (lower + upper) / float64(2)
		dev += float64(val) * (x - mean) * (x - mean)

		values = append(values, int64(b.GetCumulativeCount()-prevVal))

		lower = b.GetUpperBound()
		prevVal = b.GetCumulativeCount()
	}

	return &v3.Distribution{
		Count:                 count,
		Mean:                  mean,
		SumOfSquaredDeviation: dev,
		BucketOptions: &v3.BucketOptions{
			ExplicitBuckets: &v3.Explicit{
				Bounds: bounds,
			},
		},
		BucketCounts: values,
	}
}

func getMetricLabels(config *config.CommonConfig, labels []*dto.LabelPair) map[string]string {
	metricLabels := map[string]string{}
	for _, label := range labels {
		if config.SourceConfig.PodConfig.IsMetricLabel(label.GetName()) {
			metricLabels[label.GetName()] = label.GetValue()
		}
	}
	return metricLabels
}

// MetricFamilyToMetricDescriptor converts MetricFamily object to the MetricDescriptor. If needed it uses information
// from the previously created metricDescriptor (for example to merge label sets).
func MetricFamilyToMetricDescriptor(config *config.CommonConfig,
	family *dto.MetricFamily, originalDescriptor *v3.MetricDescriptor) *v3.MetricDescriptor {
	return &v3.MetricDescriptor{
		Description: family.GetHelp(),
		Type:        getMetricType(config, family.GetName()),
		MetricKind:  extractMetricKind(family.GetType()),
		ValueType:   extractValueType(family.GetType(), originalDescriptor),
		Labels:      extractAllLabels(family, originalDescriptor),
	}
}

func extractMetricKind(mType dto.MetricType) string {
	if mType == dto.MetricType_COUNTER || mType == dto.MetricType_HISTOGRAM {
		return "CUMULATIVE"
	}
	return "GAUGE"
}

func extractValueType(mType dto.MetricType, originalDescriptor *v3.MetricDescriptor) string {
	// If MetricDescriptor is created already in the Stackdriver use stored value type.
	// This is going to work perfectly for "container.googleapis.com" metrics.
	if originalDescriptor != nil {
		// TODO(loburm): for custom metrics add logic that can figure value type base on the actual values.
		return originalDescriptor.ValueType
	}
	if mType == dto.MetricType_HISTOGRAM {
		return "DISTRIBUTION"
	}
	if mType == dto.MetricType_UNTYPED {
		return "DOUBLE"
	}
	return "INT64"
}

func extractAllLabels(family *dto.MetricFamily, originalDescriptor *v3.MetricDescriptor) []*v3.LabelDescriptor {
	var labels []*v3.LabelDescriptor
	labelSet := make(map[string]bool)
	for _, metric := range family.GetMetric() {
		for _, label := range metric.GetLabel() {
			_, ok := labelSet[label.GetName()]
			if !ok {
				labels = append(labels, &v3.LabelDescriptor{Key: label.GetName()})
				labelSet[label.GetName()] = true
			}
		}
	}
	if originalDescriptor != nil {
		for _, label := range originalDescriptor.Labels {
			_, ok := labelSet[label.Key]
			if !ok {
				labels = append(labels, label)
				labelSet[label.Key] = true
			}
		}
	}
	return labels
}

func createProjectName(config *config.GceConfig) string {
	return fmt.Sprintf("projects/%s", config.Project)
}

func getMonitoredResourceFromLabels(config *config.CommonConfig, labels []*dto.LabelPair) *v3.MonitoredResource {
	if config.SourceConfig.CustomResourceType != "" {
		return getCustomMonitoredResource(config)
	}
	container, pod, namespace := config.SourceConfig.PodConfig.GetPodInfo(labels)
	prefix := config.MonitoredResourceTypePrefix

	if prefix == "" {
		return &v3.MonitoredResource{
			Type: "gke_container",
			Labels: map[string]string{
				"project_id":     config.GceConfig.Project,
				"cluster_name":   config.GceConfig.Cluster,
				"zone":           config.GceConfig.Zone,
				"instance_id":    config.GceConfig.Instance,
				"namespace_id":   namespace,
				"pod_id":         pod,
				"container_name": container,
			},
		}
	}

	resourceLabels := make(map[string]string)
	for k, v := range config.MonitoredResourceLabels {
		resourceLabels[k] = v
	}
	if _, found := resourceLabels["project_id"]; !found {
		resourceLabels["project_id"] = config.GceConfig.Project
	}
	if _, found := resourceLabels["cluster_name"]; !found {
		resourceLabels["cluster_name"] = config.GceConfig.Cluster
	}
	if _, found := resourceLabels["location"]; !found {
		resourceLabels["location"] = config.GceConfig.ClusterLocation
	}

	// When MonitoredResource type is not "k8s_*", default "instance_id" label to GCE instance name.
	if prefix != "k8s_" {
		if _, found := resourceLabels["instance_id"]; !found {
			resourceLabels["instance_id"] = config.GceConfig.InstanceId
		}
	}

	// When namespace and pod are unspecified, it should be written to node type.
	if namespace == "" || pod == "" || pod == "machine" {
		// When MonitoredResource is "k8s_node", default "node_name" label to GCE instance name.
		if prefix == "k8s_" {
			if _, found := resourceLabels["node_name"]; !found {
				resourceLabels["node_name"] = config.GceConfig.Instance
			}
		}
		return &v3.MonitoredResource{
			Type:   prefix + "node",
			Labels: resourceLabels,
		}
	}

	resourceLabels["namespace_name"] = namespace
	resourceLabels["pod_name"] = pod
	if container == "" {
		return &v3.MonitoredResource{
			Type:   prefix + "pod",
			Labels: resourceLabels,
		}
	}

	resourceLabels["container_name"] = container
	return &v3.MonitoredResource{
		Type:   prefix + "container",
		Labels: resourceLabels,
	}
}

func getCustomMonitoredResource(config *config.CommonConfig) *v3.MonitoredResource {
	resourceLabels := config.SourceConfig.CustomLabels
	applyDefaultIfEmpty(resourceLabels, "instance_id", config.GceConfig.InstanceId)
	applyDefaultIfEmpty(resourceLabels, "project_id", config.GceConfig.Project)
	applyDefaultIfEmpty(resourceLabels, "cluster_name", config.GceConfig.Cluster)
	applyDefaultIfEmpty(resourceLabels, "location", config.GceConfig.ClusterLocation)
	applyDefaultIfEmpty(resourceLabels, "node_name", config.GceConfig.Instance)
	return &v3.MonitoredResource{
		Type:   config.SourceConfig.CustomResourceType,
		Labels: resourceLabels,
	}
}

func applyDefaultIfEmpty(resourceLabels map[string]string, key, defaultValue string) {
	if val, found := resourceLabels[key]; found && val == "" {
		resourceLabels[key] = defaultValue
	}
}
