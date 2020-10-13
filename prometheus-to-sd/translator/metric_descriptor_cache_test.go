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
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

var counterType = dto.MetricType_COUNTER

var equalDescriptor = "equal"
var differentDescription = "differentDescription"
var differentLabels = "differentLabels"

var description1 = "Simple description"
var description2 = "Complex description"

var label1 = &v3.LabelDescriptor{Key: "label1"}
var label2 = &v3.LabelDescriptor{Key: "label2"}
var label3 = &v3.LabelDescriptor{Key: "label3"}

var originalDescriptor = v3.MetricDescriptor{
	Name:        equalDescriptor,
	Description: description1,
	MetricKind:  "GAUGE",
	Labels:      []*v3.LabelDescriptor{label1, label2},
}

var otherDescriptors = map[*v3.MetricDescriptor]bool{
	{
		Name:        equalDescriptor,
		Description: description1,
		MetricKind:  "GAUGE",
		Labels:      []*v3.LabelDescriptor{label1, label2},
	}: false,
	{
		Name:        differentDescription,
		Description: description2,
		MetricKind:  "GAUGE",
		Labels:      []*v3.LabelDescriptor{label1, label2},
	}: true,
	{
		Name:        differentLabels,
		Description: description1,
		MetricKind:  "GAUGE",
		Labels:      []*v3.LabelDescriptor{label3},
	}: true,
	{
		Name:        equalDescriptor,
		Description: description1,
		MetricKind:  "CUMULATIVE",
		Labels:      []*v3.LabelDescriptor{label1, label2},
	}: true,
}

func TestDescriptorChanged(t *testing.T) {
	for descriptor, result := range otherDescriptors {
		if result {
			assert.True(t, descriptorChanged(&originalDescriptor, descriptor))
		} else {
			assert.False(t, descriptorChanged(&originalDescriptor, descriptor))
		}
	}
}

func TestValidateMetricDescriptors(t *testing.T) {
	testCases := []struct {
		description  string
		descriptors  []*v3.MetricDescriptor
		metricFamily *dto.MetricFamily
		missing      bool
		broken       bool
	}{
		{
			description: "Metric is broken if new label was added",
			descriptors: []*v3.MetricDescriptor{
				{
					Name:       "descriptor1",
					MetricKind: "CUMULATIVE",
				},
			},
			metricFamily: &dto.MetricFamily{
				Name: stringPtr("descriptor1"),
				Type: &counterType,
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: floatPtr(100.0),
						},
						Label: []*dto.LabelPair{
							{
								Name:  stringPtr("newLabel"),
								Value: stringPtr("newValue"),
							},
						},
					},
				},
			},
			missing: false,
			broken:  true,
		},
		{
			description: "Metric is broken if metric kind was changed",
			descriptors: []*v3.MetricDescriptor{
				{
					Name:       "descriptor1",
					MetricKind: "GAUGE",
				},
			},
			metricFamily: &dto.MetricFamily{
				Name: stringPtr("descriptor1"),
				Type: &counterType,
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: floatPtr(100.0),
						},
					},
				},
			},
			missing: false,
			broken:  true,
		},
		{
			description: "Metric family hasn't changed",
			descriptors: []*v3.MetricDescriptor{
				{
					Name:       "descriptor1",
					MetricKind: "CUMULATIVE",
				},
			},
			metricFamily: &dto.MetricFamily{
				Name: stringPtr("descriptor1"),
				Type: &counterType,
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: floatPtr(100.0),
						},
					},
				},
			},
			missing: false,
			broken:  false,
		},
		{
			description: "Metric is not in the cache",
			descriptors: []*v3.MetricDescriptor{
				{
					Name:       "descriptor1",
					MetricKind: "CUMULATIVE",
				},
			},
			metricFamily: &dto.MetricFamily{
				Name: stringPtr("descriptor2"),
				Type: &counterType,
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: floatPtr(100.0),
						},
					},
				},
			},
			missing: true,
			broken:  false,
		},
		{
			description: "Description change doesn't break metric family",
			descriptors: []*v3.MetricDescriptor{
				{
					Name:        "descriptor1",
					Description: "original description",
					MetricKind:  "CUMULATIVE",
				},
			},
			metricFamily: &dto.MetricFamily{
				Name: stringPtr("descriptor1"),
				Type: &counterType,
				Help: stringPtr("changed description"),
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{
							Value: floatPtr(100.0),
						},
					},
				},
			},
			missing: false,
			broken:  false,
		},
	}

	for _, tc := range testCases {
		// Init cache
		cache := NewMetricDescriptorCache(nil, &config.CommonConfig{
			SourceConfig: &config.SourceConfig{
				Component:     "test-component",
				MetricsPrefix: "container.googleapis.com",
			},
		})
		cache.fresh = true
		var whitelisted []string
		for _, descriptor := range tc.descriptors {
			cache.descriptors[descriptor.Name] = descriptor
			cache.broken[descriptor.Name] = false
			whitelisted = append(whitelisted, descriptor.Name)
			metrics := make(map[string]*dto.MetricFamily)
			metricName := tc.metricFamily.GetName()
			metrics[metricName] = tc.metricFamily

			cache.ValidateMetricDescriptors(metrics, whitelisted)
			res, ok := cache.broken[metricName]

			if tc.missing {
				assert.False(t, ok, fmt.Sprintf("Metric is not expected to be in the cache %s", metricName))
			} else {
				assert.True(t, ok, fmt.Sprintf("Metric was not found in the cache %s", metricName))
				assert.Equal(t, res, tc.broken, fmt.Sprintf("Broken state of metric %s expected to be %v", metricName, tc.broken))
			}
		}
	}
}
