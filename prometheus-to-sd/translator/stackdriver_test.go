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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

func TestGetMetricType(t *testing.T) {
	testCases := []struct {
		config   *config.CommonConfig
		metric   string
		expected string
	}{
		{
			&config.CommonConfig{
				SourceConfig: &config.SourceConfig{
					MetricsPrefix: "container.googleapis.com/master",
					Component:     "component",
				},
			},
			"name",
			"container.googleapis.com/master/component/name",
		},
		{
			&config.CommonConfig{
				SourceConfig: &config.SourceConfig{
					MetricsPrefix: "container.googleapis.com",
					Component:     "",
				},
			},
			"name",
			"container.googleapis.com/name",
		},
	}
	for _, tc := range testCases {
		assert.Equal(t, tc.expected, getMetricType(tc.config, tc.metric))
	}
}

func TestParseMetricType(t *testing.T) {
	testConfig := &config.CommonConfig{
		SourceConfig: &config.SourceConfig{MetricsPrefix: "container.googleapis.com/master"},
	}
	testCases := []struct {
		metricType string
		correct    bool
		component  string
		metricName string
	}{
		{
			metricType: "container.googleapis.com/master/component/name",
			correct:    true,
			component:  "component",
			metricName: "name",
		},
		{
			metricType: "container.googleapis.com/master/name",
			correct:    true,
			component:  "",
			metricName: "name",
		},
		{
			metricType: "container.googleapis.com/master",
			correct:    false,
		},
		{
			metricType: "incorrect.prefix.com/component/name",
			correct:    false,
		},
		{
			metricType: "container.googleapis.com/component/name/subname",
			correct:    false,
		},
	}

	for _, tc := range testCases {
		component, metricName, err := parseMetricType(testConfig, tc.metricType)
		if tc.correct {
			assert.NoError(t, err, tc.metricType)
			assert.Equal(t, tc.component, component, tc.metricType)
			assert.Equal(t, tc.metricName, metricName, tc.metricType)
		} else {
			assert.Error(t, err, tc.metricType)
		}
	}
}
