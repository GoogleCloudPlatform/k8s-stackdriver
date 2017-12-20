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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

const (
	maxTimeseriesesPerRequest = 200
)

// SendToStackdriver sends http request to Stackdriver to create the given timeserieses.
func SendToStackdriver(ctx context.Context, service *v3.Service, config *config.CommonConfig, ts []*v3.TimeSeries) {
	if len(ts) == 0 {
		glog.V(3).Infof("No metrics to send to Stackdriver for component %v", config.ComponentName)
		return
	}

	proj := createProjectName(config.GceConfig)

	var wg sync.WaitGroup
	var failedTs uint32
	for i := 0; i < len(ts); i += maxTimeseriesesPerRequest {
		end := i + maxTimeseriesesPerRequest
		if end > len(ts) {
			end = len(ts)
		}
		wg.Add(1)
		go func(begin int, end int) {
			defer wg.Done()
			req := &v3.CreateTimeSeriesRequest{TimeSeries: ts[begin:end]}
			_, err := service.Projects.TimeSeries.Create(proj, req).Context(ctx).Do()
			if err != nil {
				atomic.AddUint32(&failedTs, uint32(end-begin))
				glog.Errorf("Error while sending request to Stackdriver %v", err)
			}
		}(i, end)
	}
	wg.Wait()
	glog.V(4).Infof("Successfully sent %v timeserieses to Stackdriver for component %v", uint32(len(ts))-failedTs, config.ComponentName)
}

func getMetricDescriptors(service *v3.Service, config *config.GceConfig, component string) (map[string]*v3.MetricDescriptor, error) {
	proj := createProjectName(config)
	filter := fmt.Sprintf("metric.type = starts_with(\"%s/%s\")", config.MetricsPrefix, component)
	metrics := make(map[string]*v3.MetricDescriptor)
	fn := func(page *v3.ListMetricDescriptorsResponse) error {
		for _, metricDescriptor := range page.MetricDescriptors {
			if _, metricName, err := parseMetricType(config, metricDescriptor.Type); err == nil {
				metrics[metricName] = metricDescriptor
			} else {
				glog.Warningf("Unable to parse %v: %v", metricDescriptor.Type, err)
			}
		}
		return nil
	}
	err := service.Projects.MetricDescriptors.List(proj).Filter(filter).Pages(nil, fn)
	if err != nil {
		glog.Warningf("Error while fetching metric descriptors for %v: %v", component, err)
	}
	return metrics, err
}

// updateMetricDescriptorInStackdriver writes metric descriptor to the stackdriver.
func updateMetricDescriptorInStackdriver(service *v3.Service, config *config.GceConfig, metricDescriptor *v3.MetricDescriptor) bool {
	glog.V(4).Infof("Updating metric descriptor: %+v", metricDescriptor)

	projectName := createProjectName(config)
	_, err := service.Projects.MetricDescriptors.Create(projectName, metricDescriptor).Do()
	if err != nil {
		glog.Errorf("Error in attempt to update metric descriptor %v", err)
		return false
	}
	return true
}

// parseMetricType extracts component and metricName from Metric.Type (e.g. output of getMetricType).
func parseMetricType(config *config.GceConfig, metricType string) (component, metricName string, err error) {
	if !strings.HasPrefix(metricType, fmt.Sprintf("%s/", config.MetricsPrefix)) {
		return "", "", fmt.Errorf("MetricType is expected to have prefix: %v. Got %v instead.", config.MetricsPrefix, metricType)
	}

	componentMetricName := strings.TrimPrefix(metricType, fmt.Sprintf("%s/", config.MetricsPrefix))
	split := strings.SplitN(componentMetricName, "/", 2)

	if len(split) < 1 || len(split) > 2 {
		return "", "", fmt.Errorf("MetricType should be in format %v/<component>/<name> or %v/<name>. Got %v instead.", config.MetricsPrefix, metricType)
	}
	if len(split) == 1 {
		return "", split[0], nil
	}
	return split[0], split[1], nil
}
