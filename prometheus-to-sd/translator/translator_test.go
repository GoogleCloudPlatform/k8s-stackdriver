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
	"context"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genproto/googleapis/api/label"
	"google.golang.org/genproto/googleapis/api/metric"
	v3 "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

type ByMetricTypeReversed []*v3.TimeSeries

func (ts ByMetricTypeReversed) Len() int {
	return len(ts)
}

func (ts ByMetricTypeReversed) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

func (ts ByMetricTypeReversed) Less(i, j int) bool {
	return ts[i].Metric.Type > ts[j].Metric.Type
}

const epsilon = float64(0.001)

var commonConfig = &config.CommonConfig{
	GceConfig: &config.GceConfig{
		Project:  "test-proj",
		Zone:     "us-central1-f",
		Cluster:  "test-cluster",
		Instance: "kubernetes-master.c.test-proj.internal",
	},
	SourceConfig: &config.SourceConfig{
		PodConfig:     config.NewPodConfig("machine", "", "", "", "", "", "", ""),
		Component:     "testcomponent",
		MetricsPrefix: "container.googleapis.com/master",
	},
}

var metricTypeGauge = dto.MetricType_GAUGE
var metricTypeCounter = dto.MetricType_COUNTER
var metricTypeHistogram = dto.MetricType_HISTOGRAM
var metricTypeUntyped = dto.MetricType_UNTYPED

var testMetricName = "test_name"
var booleanMetricName = "boolean_metric"
var floatMetricName = "float_metric"
var intSummaryMetricName = "int_summary_metric"
var floatSummaryMetricName = "float_summary_metric"
var testMetricHistogram = "test_histogram"
var unrelatedMetric = "unrelated_metric"
var testMetricDescription = "Description 1"
var testMetricHistogramDescription = "Description 2"
var untypedMetricName = "untyped_metric"
var testLabelName = "labelName"
var testLabelValue1 = "labelValue1"
var testLabelValue2 = "labelValue2"

var now = time.Now()
var startTime = time.Unix(1234567890, 0)
var startTimePB = timestamppb.New(startTime)

var metricsResponse = &PrometheusResponse{rawResponse: []byte(`
# TYPE test_name counter
test_name{labelName="labelValue1"} 42.0
test_name{labelName="labelValue2"} 106.0
test_name{labelName="labelValue3"} 136.0
# TYPE boolean_metric gauge
boolean_metric{labelName="falseValue"} 0.00001
boolean_metric{labelName="trueValue"} 1.2
# TYPE float_metric counter
float_metric 123.17
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890.0
# TYPE unrelated_metric gauge
unrelated_metric 23.0
# TYPE test_histogram histogram
test_histogram_bucket{le="1.0"} 1
test_histogram_bucket{le="3.0"} 4
test_histogram_bucket{le="5.0"} 4
test_histogram_bucket{le="+Inf"} 5
test_histogram_sum 13.0
test_histogram_count 5
# TYPE untyped_metric untyped
untyped_metric 98.6
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}

var metrics = map[string]*dto.MetricFamily{
	testMetricName: {
		Name: &testMetricName,
		Type: &metricTypeCounter,
		Help: &testMetricDescription,
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("labelValue1"),
					},
				},
				Counter: &dto.Counter{Value: floatPtr(42.0)},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("labelValue2"),
					},
				},
				Counter: &dto.Counter{Value: floatPtr(106.0)},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("labelValue3"),
					},
				},
				Counter: &dto.Counter{Value: floatPtr(136.0)},
			},
		},
	},
	booleanMetricName: {
		Name: stringPtr(booleanMetricName),
		Type: &metricTypeGauge,
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("falseValue"),
					},
				},
				Gauge: &dto.Gauge{Value: floatPtr(0.00001)},
			},
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("trueValue"),
					},
				},
				Gauge: &dto.Gauge{Value: floatPtr(1.2)},
			},
		},
	},
	floatMetricName: {
		Name: stringPtr(floatMetricName),
		Type: &metricTypeCounter,
		Metric: []*dto.Metric{
			{
				Counter: &dto.Counter{Value: floatPtr(123.17)},
			},
		},
	},
	processStartTimeMetric: {
		Name: stringPtr(processStartTimeMetric),
		Type: &metricTypeGauge,
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: floatPtr(1234567890.0)},
			},
		},
	},
	unrelatedMetric: {
		Name: &unrelatedMetric,
		Type: &metricTypeGauge,
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: floatPtr(23.0)},
			},
		},
	},
	testMetricHistogram: {
		Name: &testMetricHistogram,
		Type: &metricTypeHistogram,
		Help: &testMetricHistogramDescription,
		Metric: []*dto.Metric{
			{
				Histogram: &dto.Histogram{
					SampleCount: intPtr(5),
					SampleSum:   floatPtr(13),
					Bucket: []*dto.Bucket{
						{
							CumulativeCount: intPtr(1),
							UpperBound:      floatPtr(1),
						},
						{
							CumulativeCount: intPtr(4),
							UpperBound:      floatPtr(3),
						},
						{
							CumulativeCount: intPtr(4),
							UpperBound:      floatPtr(5),
						},
						{
							CumulativeCount: intPtr(5),
							UpperBound:      floatPtr(math.Inf(1)),
						},
					},
				},
			},
		},
	},
	untypedMetricName: {
		Name: &untypedMetricName,
		Type: &metricTypeUntyped,
		Metric: []*dto.Metric{
			{
				Untyped: &dto.Untyped{
					Value: floatPtr(98.6),
				},
			},
		},
	},
}

var metricDescriptors = map[string]*metric.MetricDescriptor{
	testMetricName: {
		Type:        "container.googleapis.com/master/testcomponent/test_name",
		Description: testMetricDescription,
		MetricKind:  metric.MetricDescriptor_CUMULATIVE,
		ValueType:   metric.MetricDescriptor_INT64,
		Labels: []*label.LabelDescriptor{
			{
				Key: "labelName",
			},
		},
	},
	booleanMetricName: {
		Type:       "container.googleapis.com/master/testcomponent/boolean_metric",
		MetricKind: metric.MetricDescriptor_GAUGE,
		ValueType:  metric.MetricDescriptor_BOOL,
		Labels: []*label.LabelDescriptor{
			{
				Key: "labelName",
			},
		},
	},
	floatMetricName: {
		Type:       "container.googleapis.com/master/testcomponent/float_metric",
		MetricKind: metric.MetricDescriptor_CUMULATIVE,
		ValueType:  metric.MetricDescriptor_DOUBLE,
	},
	processStartTimeMetric: {
		Type:       "container.googleapis.com/master/testcomponent/process_start_time_seconds",
		MetricKind: metric.MetricDescriptor_GAUGE,
		ValueType:  metric.MetricDescriptor_INT64,
	},
	unrelatedMetric: {
		Type:       "container.googleapis.com/master/testcomponent/unrelated_metric",
		MetricKind: metric.MetricDescriptor_GAUGE,
		ValueType:  metric.MetricDescriptor_INT64,
	},
	testMetricHistogram: {
		Type:        "container.googleapis.com/master/testcomponent/test_histogram",
		Description: testMetricHistogramDescription,
		MetricKind:  metric.MetricDescriptor_CUMULATIVE,
		ValueType:   metric.MetricDescriptor_DISTRIBUTION,
	},
	floatSummaryMetricName + "_sum": {
		Type:       "container.googleapis.com/master/testcomponent/float_summary_metric_sum",
		MetricKind: metric.MetricDescriptor_CUMULATIVE,
		ValueType:  metric.MetricDescriptor_DOUBLE,
	},
	intSummaryMetricName + "_sum": {
		Type:       "container.googleapis.com/master/testcomponent/int_summary_metric_sum",
		MetricKind: metric.MetricDescriptor_CUMULATIVE,
		ValueType:  metric.MetricDescriptor_INT64,
	},
	untypedMetricName: {
		Type:       "container.googleapis.com/master/testcomponent/untyped_metric",
		MetricKind: metric.MetricDescriptor_GAUGE,
		ValueType:  metric.MetricDescriptor_DOUBLE,
	},
}

func TestGetMonitoredResourceFromLabels(t *testing.T) {
	testCases := []struct {
		name           string
		config         *config.CommonConfig
		labels         []*dto.LabelPair
		expectedType   string
		expectedLabels map[string]string
	}{
		{
			"Ensure that default returns gke_container.",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{
					Project:    "test-project",
					Zone:       "us-east1-a",
					Cluster:    "test-cluster",
					Instance:   "test-instance",
					InstanceId: "123",
				},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "", ""),
				},
				MonitoredResourceLabels: map[string]string{},
			},
			nil,
			"gke_container",
			map[string]string{
				"project_id":   "test-project",
				"zone":         "us-east1-a",
				"cluster_name": "test-cluster",
				// To note: in legacy "gke_container" type, "instance_id" label referes to instance name.
				"instance_id":    "test-instance",
				"namespace_id":   "",
				"pod_id":         "",
				"container_name": "",
			},
		},
		{
			"Ensure that k8s resources with 'machine' pod label return k8s_node.",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("machine", "", "", "", "", "", "", ""),
				},
				MonitoredResourceTypePrefix: "k8s_",
				MonitoredResourceLabels: map[string]string{
					"project_id":   "test-project",
					"location":     "us-west1",
					"cluster_name": "test-cluster",
					"node_name":    "test-node",
				},
			},
			nil,
			"k8s_node",
			map[string]string{
				"project_id":   "test-project",
				"location":     "us-west1",
				"cluster_name": "test-cluster",
				"node_name":    "test-node",
			},
		},
		{
			"Ensure that k8s resources with 'machine' pod label return k8s_node, and can get default from GCE config..",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{
					Project:         "test-project",
					Zone:            "us-east1-a",
					Cluster:         "test-cluster",
					ClusterLocation: "test-location",
					Instance:        "test-instance",
					InstanceId:      "123",
				},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("machine", "", "", "", "", "", "", ""),
				},
				MonitoredResourceTypePrefix: "k8s_",
				MonitoredResourceLabels:     map[string]string{},
			},
			nil,
			"k8s_node",
			map[string]string{
				"project_id":   "test-project",
				"location":     "test-location",
				"cluster_name": "test-cluster",
				"node_name":    "test-instance",
			},
		},
		{
			"Ensure that k8s resources without container labels return k8s_pod.",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{
					Project:    "test-project",
					Zone:       "us-east1-a",
					Cluster:    "test-cluster",
					Instance:   "test-instance",
					InstanceId: "123",
				},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("test-pod", "test-namespace", "", "", "", "", "", ""),
				},
				MonitoredResourceTypePrefix: "k8s_",
				MonitoredResourceLabels: map[string]string{
					"project_id":   "test-project",
					"location":     "us-west1",
					"cluster_name": "test-cluster",
				},
			},
			nil,
			"k8s_pod",
			map[string]string{
				"project_id":     "test-project",
				"location":       "us-west1",
				"cluster_name":   "test-cluster",
				"namespace_name": "test-namespace",
				"pod_name":       "test-pod",
			},
		},
		{
			"Ensure that k8s resources with all labels return k8s_container.",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("test-pod", "test-namespace", "", "", "containerNameLabel", "", "", ""),
				},
				MonitoredResourceTypePrefix: "k8s_",
				MonitoredResourceLabels: map[string]string{
					"project_id":   "test-project",
					"location":     "us-west1",
					"cluster_name": "test-cluster",
				},
			},
			[]*dto.LabelPair{
				{
					Name:  stringPtr("containerNameLabel"),
					Value: stringPtr("test-container"),
				},
			},
			"k8s_container",
			map[string]string{
				"project_id":     "test-project",
				"location":       "us-west1",
				"cluster_name":   "test-cluster",
				"namespace_name": "test-namespace",
				"pod_name":       "test-pod",
				"container_name": "test-container",
			},
		},
		{
			"Ensure that other resources with 'machine' pod label return node type.",
			&config.CommonConfig{
				GceConfig: &config.GceConfig{
					Project:    "default-project",
					Zone:       "us-east1-a",
					Cluster:    "test-cluster",
					Instance:   "default-instance",
					InstanceId: "123",
				},
				SourceConfig: &config.SourceConfig{
					PodConfig: config.NewPodConfig("machine", "", "", "", "", "", "", ""),
				},
				MonitoredResourceTypePrefix: "other_prefix_",
				MonitoredResourceLabels: map[string]string{
					"location":         "us-west1",
					"cluster_name":     "test-cluster",
					"additional_label": "foo",
				},
			},
			nil,
			"other_prefix_node",
			map[string]string{
				"project_id":       "default-project",
				"location":         "us-west1",
				"cluster_name":     "test-cluster",
				"instance_id":      "123",
				"additional_label": "foo",
			},
		},
		{
			"Allow source config resource type override",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig: &config.GceConfig{
					Project:    "default-project",
					Zone:       "us-east1-a",
					Cluster:    "test-cluster",
					Instance:   "default-instance",
					InstanceId: "123",
				},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "resource_foo",
					CustomLabels: map[string]string{
						"foo": "bar",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "", ""),
				},
			},
			nil,
			"resource_foo",
			map[string]string{
				"foo": "bar",
			},
		},
		{
			"Ensure source config resource type override can default",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig: &config.GceConfig{
					Project:         "default-project",
					Zone:            "us-east1-a",
					Cluster:         "test-cluster",
					ClusterLocation: "test-location",
					Instance:        "default-instance",
					InstanceId:      "123",
				},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "resource_foo",
					CustomLabels: map[string]string{
						"foo":          "bar",
						"baz":          "",
						"project_id":   "",
						"cluster_name": "",
						"location":     "",
						"instance_id":  "",
						"node_name":    "",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "", ""),
				},
			},
			nil,
			"resource_foo",
			map[string]string{
				"foo":          "bar",
				"baz":          "",
				"project_id":   "default-project",
				"cluster_name": "test-cluster",
				"location":     "test-location",
				"instance_id":  "123",
				"node_name":    "default-instance",
			},
		},
		{
			"Add tenant UID label to custom monitored resource",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig: &config.GceConfig{
					Project:         "default-project",
					Zone:            "us-east1-a",
					Cluster:         "test-cluster",
					ClusterLocation: "test-location",
					Instance:        "default-instance",
					InstanceId:      "123",
				},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "resource_foo",
					CustomLabels: map[string]string{
						"project_id":   "",
						"cluster_name": "",
						"location":     "",
						"instance_id":  "",
						"node_name":    "",
						"tenant_uid":   "old-tenant-uid",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "tenantUIDLabel", "", ""),
				},
			},
			[]*dto.LabelPair{
				{
					Name:  stringPtr("tenantUIDLabel"),
					Value: stringPtr("tenant_uid"),
				},
			},
			"resource_foo",
			map[string]string{
				"project_id":   "default-project",
				"cluster_name": "test-cluster",
				"location":     "test-location",
				"instance_id":  "123",
				"node_name":    "default-instance",
				"tenant_uid":   "tenant_uid",
			},
		},
		{
			"Add entity type label to custom monitored resource via dynamic mapping",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig:               &config.GceConfig{},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "resource_foo",
					CustomLabels: map[string]string{
						"entity_type": "placeholder",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "entityTypeLabel", ""),
				},
			},
			[]*dto.LabelPair{
				{
					Name:  stringPtr("entityTypeLabel"),
					Value: stringPtr("actual-type"),
				},
			},
			"resource_foo",
			map[string]string{
				"entity_type": "actual-type",
			},
		},
		{
			"Add entity name label to custom monitored resource via dynamic mapping",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig:               &config.GceConfig{},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "resource_foo",
					CustomLabels: map[string]string{
						"entity_name": "placeholder",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "", "entityNameLabel"),
				},
			},
			[]*dto.LabelPair{
				{
					Name:  stringPtr("entityNameLabel"),
					Value: stringPtr("actual-name"),
				},
			},
			"resource_foo",
			map[string]string{
				"entity_name": "actual-name",
			},
		},
		{
			"Ensure entity labels remain empty if metric label is missing (No default logic)",
			&config.CommonConfig{
				MonitoredResourceLabels: map[string]string{},
				GceConfig: &config.GceConfig{
					Project: "default-project",
				},
				SourceConfig: &config.SourceConfig{
					CustomResourceType: "internal_multitenant_gke_container",
					CustomLabels: map[string]string{
						"project_id":  "",
						"entity_name": "",
					},
					PodConfig: config.NewPodConfig("", "", "", "", "", "", "", "entityNameLabel"),
				},
			},
			[]*dto.LabelPair{
				{Name: stringPtr("wrong_label"), Value: stringPtr("my-app")},
			},
			"internal_multitenant_gke_container",
			map[string]string{
				"project_id":  "default-project",
				"entity_name": "",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalResourceLabelsInConfig := make(map[string]string)
			for k, v := range tc.config.MonitoredResourceLabels {
				originalResourceLabelsInConfig[k] = v
			}
			monitoredResource := getMonitoredResourceFromLabels(tc.config, tc.labels)
			assert.Equal(t, tc.expectedType, monitoredResource.Type)
			assert.Equal(t, tc.expectedLabels, monitoredResource.Labels)
			assert.Equal(t, originalResourceLabelsInConfig, tc.config.MonitoredResourceLabels)
			if val, ok := tc.config.SourceConfig.CustomLabels["tenant_uid"]; ok {
				assert.NotEqual(t, tc.expectedLabels["tenant_uid"], val)
			}
		})
	}
}

func TestTranslatePrometheusToStackdriver(t *testing.T) {
	ctx := context.Background()
	cache := buildCacheForTesting()

	tsb := NewTimeSeriesBuilder(CommonConfigWithMetrics([]string{testMetricName, testMetricHistogram, booleanMetricName, floatMetricName}), cache)
	tsb.Update(metricsResponse, now)
	ts, timestamp, err := tsb.Build(ctx)
	assert.Equal(t, timestamp, now)

	assert.Equal(t, err, nil)

	assert.Equal(t, 7, len(ts))
	// TranslatePrometheusToStackdriver uses maps to represent data, so order of output is randomized.
	sort.Sort(ByMetricTypeReversed(ts))

	// First three int values.
	for i := 0; i <= 2; i++ {
		m := ts[i]
		assert.Equal(t, "gke_container", m.Resource.Type)
		assert.Equal(t, "container.googleapis.com/master/testcomponent/test_name", m.Metric.Type)
		assert.Equal(t, metric.MetricDescriptor_INT64, m.ValueType)
		assert.Equal(t, metric.MetricDescriptor_CUMULATIVE, m.MetricKind)

		assert.Equal(t, 1, len(m.Points))
		assert.Equal(t, startTimePB, m.Points[0].Interval.StartTime)

		labels := m.Metric.Labels
		assert.Equal(t, 1, len(labels))

		if labels["labelName"] == "labelValue1" {
			assert.Equal(t, int64(42), m.Points[0].Value.GetInt64Value())
		} else if labels["labelName"] == "labelValue2" {
			assert.Equal(t, int64(106), m.Points[0].Value.GetInt64Value())
		} else if labels["labelName"] == "labelValue3" {
			assert.Equal(t, int64(136), m.Points[0].Value.GetInt64Value())
		} else {
			t.Errorf("Wrong label labelName value %s", labels["labelName"])
		}
	}

	// Histogram
	m := ts[3]
	assert.Equal(t, "gke_container", m.Resource.Type)
	assert.Equal(t, "container.googleapis.com/master/testcomponent/test_histogram", m.Metric.Type)
	assert.Equal(t, metric.MetricDescriptor_DISTRIBUTION, m.ValueType)
	assert.Equal(t, metric.MetricDescriptor_CUMULATIVE, m.MetricKind)
	assert.Equal(t, 1, len(m.Points))

	p := m.Points[0]

	dist := p.Value.GetDistributionValue()
	assert.NotNil(t, dist)
	assert.Equal(t, int64(5), dist.Count)
	assert.InEpsilon(t, 2.6, dist.Mean, epsilon)
	assert.InEpsilon(t, 11.25, dist.SumOfSquaredDeviation, epsilon)

	bounds := dist.BucketOptions.GetExplicitBuckets().GetBounds()
	assert.Equal(t, 3, len(bounds))
	assert.InEpsilon(t, 1, bounds[0], epsilon)
	assert.InEpsilon(t, 3, bounds[1], epsilon)
	assert.InEpsilon(t, 5, bounds[2], epsilon)

	counts := dist.BucketCounts
	assert.Equal(t, 4, len(counts))
	assert.Equal(t, int64(1), counts[0])
	assert.Equal(t, int64(3), counts[1])
	assert.Equal(t, int64(0), counts[2])
	assert.Equal(t, int64(1), counts[3])

	// Then float value.
	m = ts[4]
	assert.Equal(t, "gke_container", m.Resource.Type)
	assert.Equal(t, "container.googleapis.com/master/testcomponent/float_metric", m.Metric.Type)
	assert.Equal(t, metric.MetricDescriptor_DOUBLE, m.ValueType)
	assert.Equal(t, metric.MetricDescriptor_CUMULATIVE, m.MetricKind)
	assert.InEpsilon(t, 123.17, m.Points[0].Value.GetDoubleValue(), epsilon)
	assert.Equal(t, 1, len(m.Points))
	assert.Equal(t, startTimePB, m.Points[0].Interval.StartTime)

	// Then two boolean values.
	for i := 5; i <= 6; i++ {
		m := ts[i]
		assert.Equal(t, "gke_container", m.Resource.Type)
		assert.Equal(t, "container.googleapis.com/master/testcomponent/boolean_metric", m.Metric.Type)
		assert.Equal(t, metric.MetricDescriptor_BOOL, m.ValueType)
		assert.Equal(t, metric.MetricDescriptor_GAUGE, m.MetricKind)

		labels := m.Metric.Labels
		assert.Equal(t, 1, len(labels))
		if labels["labelName"] == "falseValue" {
			assert.Equal(t, false, m.Points[0].Value.GetBoolValue())
		} else if labels["labelName"] == "trueValue" {
			assert.Equal(t, true, m.Points[0].Value.GetBoolValue())
		} else {
			t.Errorf("Wrong label labelName value %s", labels["labelName"])
		}
	}
}

func TestTranslatePrometheusToStackdriverWithLabelFiltering(t *testing.T) {
	cache := buildCacheForTesting()

	whitelistedLabelsMap := map[string]map[string]bool{testLabelName: {testLabelValue1: true, testLabelValue2: true}}
	commonConfigWithFiltering := &config.CommonConfig{
		GceConfig: &config.GceConfig{
			Project:  "test-proj",
			Zone:     "us-central1-f",
			Cluster:  "test-cluster",
			Instance: "kubernetes-master.c.test-proj.internal",
		},
		SourceConfig: &config.SourceConfig{
			PodConfig:            config.NewPodConfig("machine", "", "", "", "", "", "", ""),
			Component:            "testcomponent",
			MetricsPrefix:        "container.googleapis.com/master",
			Whitelisted:          []string{testMetricName, testMetricHistogram, booleanMetricName, floatMetricName},
			WhitelistedLabelsMap: whitelistedLabelsMap,
		},
	}

	tsb := NewTimeSeriesBuilder(commonConfigWithFiltering, cache)
	tsb.Update(metricsResponse, now)
	ts, timestamp, err := tsb.Build(context.Background())

	assert.Equal(t, timestamp, now)
	assert.Equal(t, err, nil)
	assert.Equal(t, 2, len(ts))

	// TranslatePrometheusToStackdriver uses maps to represent data, so order of output is randomized.
	sort.Sort(ByMetricTypeReversed(ts))

	// First two int values.
	for i := 0; i <= 1; i++ {
		m := ts[i]
		assert.Equal(t, "gke_container", m.Resource.Type)
		assert.Equal(t, "container.googleapis.com/master/testcomponent/test_name", m.Metric.Type)
		assert.Equal(t, metric.MetricDescriptor_INT64, m.ValueType)
		assert.Equal(t, metric.MetricDescriptor_CUMULATIVE, m.MetricKind)

		assert.Equal(t, 1, len(m.Points))
		assert.Equal(t, startTimePB, m.Points[0].Interval.StartTime)

		labels := m.Metric.Labels
		assert.Equal(t, 1, len(labels))

		if labels["labelName"] == "labelValue1" {
			assert.Equal(t, int64(42), m.Points[0].Value.GetInt64Value())
		} else if labels["labelName"] == "labelValue2" {
			assert.Equal(t, int64(106), m.Points[0].Value.GetInt64Value())
		} else {
			t.Errorf("Wrong label labelName value %s", labels["labelName"])
		}
	}
}

func TestTranslateSummary(t *testing.T) {
	var intSummaryMetricsResponse = &PrometheusResponse{rawResponse: []byte(`
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890
# TYPE int_summary_metric summary
int_summary_metric{quantile="0.5"} 4
int_summary_metric{quantile="0.9"} 8
int_summary_metric{quantile="0.99"} 8
int_summary_metric_sum 42
int_summary_metric_count 101010
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}
	var floatSummaryMetricsResponse = &PrometheusResponse{rawResponse: []byte(`
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890
# TYPE float_summary_metric summary
float_summary_metric{quantile="0.5"} 4.12
float_summary_metric{quantile="0.9"} 8.123
float_summary_metric{quantile="0.99"} 8.123
float_summary_metric_sum 0.42
float_summary_metric_count 50
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}
	var labelIntSummaryMetricsResponse = &PrometheusResponse{rawResponse: []byte(`
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890
# TYPE int_summary_metric summary
int_summary_metric{quantile="0.5",label="l1"} 1
int_summary_metric{quantile="0.5",label="l2"} 2
int_summary_metric{quantile="0.9",label="l1"} 3
int_summary_metric{quantile="0.9",label="l2"} 4
int_summary_metric{quantile="0.99",label="l1"} 5
int_summary_metric{quantile="0.99",label="l2"} 6
int_summary_metric_sum{label="l1"} 7
int_summary_metric_sum{label="l2"} 8
int_summary_metric_count{label="l1"} 9
int_summary_metric_count{label="l2"} 10
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}

	type summaryTest struct {
		description        string
		prometheusResponse *PrometheusResponse
		summaryMetricName  string
		expectedTimeSeries []*v3.TimeSeries
	}

	end := time.Now()
	start := time.Unix(1234567890, 0)

	sts := []summaryTest{
		{
			description:        "Test summary metrics which accumulate integer values",
			prometheusResponse: intSummaryMetricsResponse,
			summaryMetricName:  intSummaryMetricName,
			expectedTimeSeries: []*v3.TimeSeries{
				{
					Metric: &metric.Metric{
						Labels: map[string]string{},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_sum",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(42, start, end),
					},
				},
				{
					Metric: &metric.Metric{
						Labels: map[string]string{},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_count",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(101010, start, end),
					},
				},
			},
		},
		{
			description:        "Test summary metrics which accumulate float values",
			prometheusResponse: floatSummaryMetricsResponse,
			summaryMetricName:  floatSummaryMetricName,
			expectedTimeSeries: []*v3.TimeSeries{
				{
					Metric: &metric.Metric{
						Labels: map[string]string{},
						Type:   "container.googleapis.com/master/testcomponent/float_summary_metric_sum",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createDoublePoint(0.42, start, end),
					},
				},
				{
					Metric: &metric.Metric{
						Labels: map[string]string{},
						Type:   "container.googleapis.com/master/testcomponent/float_summary_metric_count",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(50, start, end),
					},
				},
			},
		},
		{
			description:        "Test summary metrics which are labeled",
			prometheusResponse: labelIntSummaryMetricsResponse,
			summaryMetricName:  intSummaryMetricName,
			expectedTimeSeries: []*v3.TimeSeries{
				{
					Metric: &metric.Metric{
						Labels: map[string]string{"label": "l1"},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_sum",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(7, start, end),
					},
				},
				{
					Metric: &metric.Metric{
						Labels: map[string]string{"label": "l2"},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_sum",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(8, start, end),
					},
				},
				{
					Metric: &metric.Metric{
						Labels: map[string]string{"label": "l1"},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_count",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(9, start, end),
					},
				},
				{
					Metric: &metric.Metric{
						Labels: map[string]string{"label": "l2"},
						Type:   "container.googleapis.com/master/testcomponent/int_summary_metric_count",
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Points: []*v3.Point{
						createIntPoint(10, start, end),
					},
				},
			},
		},
	}

	for _, tt := range sts {
		t.Run(tt.description, func(t *testing.T) {
			cache := buildCacheForTesting()

			tsb := NewTimeSeriesBuilder(CommonConfigWithMetrics([]string{tt.summaryMetricName + "_sum", tt.summaryMetricName + "_count"}), cache)
			tsb.Update(tt.prometheusResponse, end)
			tss, timestamp, err := tsb.Build(context.Background())
			assert.Equal(t, timestamp, end)

			sort.Sort(ByMetricTypeReversed(tss))
			if err != nil {
				t.Errorf("Should not have an error parsing summary metric %v", err)
			}
			if len(tss) != len(tt.expectedTimeSeries) {
				t.Errorf("Got %d items, Want: %d", len(tss), len(tt.expectedTimeSeries))
			}

			for i, ts := range tss {
				expectedTs := tt.expectedTimeSeries[i]
				if expectedTs.MetricKind != ts.MetricKind {
					t.Errorf("Got %v, expecting %v", ts.MetricKind, expectedTs.MetricKind)
				}
				if !reflect.DeepEqual(expectedTs.Metric, ts.Metric) {
					t.Errorf("Got %v, wanted %v as our metric", ts.Metric, expectedTs.Metric)
				}
				if !reflect.DeepEqual(ts.Points, expectedTs.Points) {
					t.Errorf("Got %v, wanted %v as our points", ts.Points, expectedTs.Points)
				}
			}
		})
	}
}

func createInterval(start time.Time, end time.Time) *v3.TimeInterval {
	return &v3.TimeInterval{
		StartTime: timestamppb.New(start.UTC()),
		EndTime:   timestamppb.New(end.UTC()),
	}
}

func createIntValue(num int) *v3.TypedValue {
	n := int64(num)
	return &v3.TypedValue{
		Value: &v3.TypedValue_Int64Value{Int64Value: n},
	}
}

func createDoubleValue(double float64) *v3.TypedValue {
	d := float64(double)
	return &v3.TypedValue{
		Value: &v3.TypedValue_DoubleValue{DoubleValue: d},
	}
}

func createPoint(value *v3.TypedValue, valueTypeString string, start time.Time, end time.Time) *v3.Point {
	return &v3.Point{
		Interval: createInterval(start, end),
		Value:    value,
	}
}

func createIntPoint(num int, start time.Time, end time.Time) *v3.Point {
	return createPoint(createIntValue(num), "Int64Value", start, end)
}

func createDoublePoint(d float64, start time.Time, end time.Time) *v3.Point {
	return createPoint(createDoubleValue(d), "DoubleValue", start, end)
}

func TestUpdateScrapes(t *testing.T) {
	tsb := NewTimeSeriesBuilder(CommonConfigWithMetrics([]string{testMetricName, floatMetricName}), buildCacheForTesting())
	scrape := &PrometheusResponse{rawResponse: []byte(`
# TYPE test_name counter
test_name{labelName="labelValue1"} 42.0
test_name{labelName="labelValue2"} 106.0
# TYPE float_metric counter
float_metric 123.17
# TYPE test_name counter
process_start_time_seconds 1234567890.0
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}
	tsb.Update(scrape, now)
	scrape = &PrometheusResponse{rawResponse: []byte(`
# TYPE test_name counter
test_name{labelName="labelValue1"} 42.0
test_name{labelName="labelValue2"} 601.0
# TYPE process_start_time_seconds gauge
process_start_time_seconds 1234567890.0
`), header: http.Header{"Content-Type": []string{"text/plain; version=0.0.4; charset=UTF-8"}}}
	tsb.Update(scrape, now)
	ts, timestamp, err := tsb.Build(context.Background())
	assert.Equal(t, timestamp, now)
	assert.Equal(t, err, nil)
	assert.Equal(t, 2, len(ts))
	// TranslatePrometheusToStackdriver uses maps to represent data, so order of output is randomized.
	sort.Sort(ByMetricTypeReversed(ts))

	for i := 0; i <= 1; i++ {
		m := ts[i]
		assert.Equal(t, "gke_container", m.Resource.Type)
		assert.Equal(t, "container.googleapis.com/master/testcomponent/test_name", m.Metric.Type)
		assert.Equal(t, metric.MetricDescriptor_INT64, m.ValueType)
		assert.Equal(t, metric.MetricDescriptor_CUMULATIVE, m.MetricKind)

		assert.Equal(t, 1, len(m.Points))
		assert.Equal(t, startTimePB, m.Points[0].Interval.StartTime)

		labels := m.Metric.Labels
		assert.Equal(t, 1, len(labels))

		if labels["labelName"] == "labelValue1" {
			// This one stays stale
			assert.Equal(t, int64(42), m.Points[0].Value.GetInt64Value())
		} else if labels["labelName"] == "labelValue2" {
			// This one gets updated
			assert.Equal(t, int64(601), m.Points[0].Value.GetInt64Value())
		} else {
			t.Errorf("Wrong label labelName value %s", labels["labelName"])
		}
	}
}

func TestMetricFamilyToMetricDescriptor(t *testing.T) {
	for metricName, metric := range metrics {
		metricDescriptor := MetricFamilyToMetricDescriptor(commonConfig, metric, getOriginalDescriptor(metricName))
		expectedMetricDescriptor := metricDescriptors[metricName]
		assert.Equal(t, metricDescriptor, expectedMetricDescriptor)
	}
}

func TestMetricFamilyToMetricDescriptorFiltersResourceLabels(t *testing.T) {
	// Define a config where "tenant_uid" is a resource label.
	// We use the 6th argument (tenantUIDLabel) which is known to be a label name.
	testConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{
			PodConfig:     config.NewPodConfig("", "", "", "", "", "tenant_uid", "", ""),
			MetricsPrefix: "container.googleapis.com/master",
		},
	}

	metricName := "test_filtering"
	metricFamily := &dto.MetricFamily{
		Name: &metricName,
		Type: &metricTypeCounter,
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("tenant_uid"),
						Value: stringPtr("tenant-123"),
					},
					{
						Name:  stringPtr("labelName"),
						Value: stringPtr("labelValue1"),
					},
				},
				Counter: &dto.Counter{Value: floatPtr(1.0)},
			},
		},
	}

	descriptor := MetricFamilyToMetricDescriptor(testConfig, metricFamily, nil)

	// Expecting only "labelName" because "tenant_uid" is a resource label.
	assert.Equal(t, 1, len(descriptor.Labels))
	assert.Equal(t, "labelName", descriptor.Labels[0].Key)
}

func TestOmitComponentName(t *testing.T) {
	var normalMetric1 = "metric1"
	var metricWithSomePrefix = "some_prefix_metric2"
	var metricWithComponentPrefix = "testcomponent_metric"
	var metricWithIncorrectComponentPrefix = "testcomponentmetric"

	var metricFamiliesForWhitelistTest = map[string]*dto.MetricFamily{
		normalMetric1: {
			Name: stringPtr(normalMetric1),
		},
		metricWithSomePrefix: {
			Name: stringPtr(metricWithSomePrefix),
		},
		metricWithComponentPrefix: {
			Name: stringPtr(metricWithComponentPrefix),
		},
		metricWithIncorrectComponentPrefix: {
			Name: stringPtr(metricWithIncorrectComponentPrefix),
		},
	}
	processedMetrics := OmitComponentName(metricFamiliesForWhitelistTest, "testcomponent")
	for k, v := range processedMetrics {
		assert.False(t, strings.HasPrefix(k, "testcomponent_"))
		assert.False(t, strings.HasPrefix(*v.Name, "testcomponent_"))
	}
}

func TestBuildWithoutUpdate(t *testing.T) {
	cache := buildCacheForTesting()

	tsb := NewTimeSeriesBuilder(CommonConfigWithMetrics([]string{testMetricName, testMetricHistogram, booleanMetricName, floatMetricName}), cache)
	ts, _, err := tsb.Build(context.Background())

	assert.Equal(t, err, nil)
	assert.Equal(t, 0, len(ts))
}

func CommonConfigWithMetrics(whitelisted []string) *config.CommonConfig {
	commonConfigCopy := *commonConfig
	commonConfigCopy.SourceConfig.Whitelisted = whitelisted
	return &commonConfigCopy
}

func buildCacheForTesting() *MetricDescriptorCache {
	cache := NewMetricDescriptorCache(nil, commonConfig)
	cache.descriptors[booleanMetricName] = metricDescriptors[booleanMetricName]
	cache.descriptors[floatMetricName] = metricDescriptors[floatMetricName]
	cache.descriptors[unrelatedMetric] = metricDescriptors[unrelatedMetric]
	cache.descriptors[intSummaryMetricName+"_sum"] = metricDescriptors[intSummaryMetricName+"_sum"]
	cache.descriptors[floatSummaryMetricName+"_sum"] = metricDescriptors[floatSummaryMetricName+"_sum"]
	cache.descriptors[untypedMetricName] = metricDescriptors[untypedMetricName]

	return cache
}

func getOriginalDescriptor(metric string) *metric.MetricDescriptor {
	// For testing reason we provide metric descriptor only for boolean_metric and float_metric.
	if metric == booleanMetricName || metric == floatMetricName {
		return metricDescriptors[metric]
	}
	return nil
}

func floatPtr(val float64) *float64 {
	ptr := val
	return &ptr
}

func intPtr(val uint64) *uint64 {
	ptr := val
	return &ptr
}

func stringPtr(val string) *string {
	ptr := val
	return &ptr
}

func TestDowncaseMetricNames(t *testing.T) {
	var normalMetric1 = "metric1"
	var metricWithComponentPrefix = "testComponent_metric"
	var metricWithIncorrectComponentPrefix = "testComponentMetric"

	var metricFamiliesForWhitelistTest = map[string]*dto.MetricFamily{
		normalMetric1: {
			Name: stringPtr(normalMetric1),
		},
		metricWithComponentPrefix: {
			Name: stringPtr(metricWithComponentPrefix),
		},
		metricWithIncorrectComponentPrefix: {
			Name: stringPtr(metricWithIncorrectComponentPrefix),
		},
	}
	tests := []struct {
		name         string
		inputMetrics map[string]*dto.MetricFamily
	}{
		{"Test downcase names",
			metricFamiliesForWhitelistTest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DowncaseMetricNames(tt.inputMetrics)
			for k, v := range got {
				if k != strings.ToLower(k) {
					t.Errorf("metric name key is not properly downcased, got %s, want %s", k, strings.ToLower(k))
				}

				if v.GetName() != strings.ToLower(v.GetName()) {
					t.Errorf("metric name is not properly downcased, got %s, want %s", v.GetName(), strings.ToLower(v.GetName()))
				}
			}
		})
	}
}
