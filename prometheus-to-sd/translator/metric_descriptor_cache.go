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
	"github.com/golang/glog"
	dto "github.com/prometheus/client_model/go"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

// MetricDescriptorCache is responsible for fetching, creating and updating metric descriptors from the stackdriver.
type MetricDescriptorCache struct {
	descriptors map[string]*v3.MetricDescriptor
	broken      map[string]bool
	service     *v3.Service
	config      *config.CommonConfig
	fresh       bool
}

// NewMetricDescriptorCache creates empty metric descriptor cache for the given component.
func NewMetricDescriptorCache(service *v3.Service, config *config.CommonConfig) *MetricDescriptorCache {
	return &MetricDescriptorCache{
		descriptors: make(map[string]*v3.MetricDescriptor),
		broken:      make(map[string]bool),
		service:     service,
		config:      config,
		fresh:       false,
	}
}

// IsMetricBroken returns true if this metric descriptor assumed to invalid (for examples it has too many labels).
func (cache *MetricDescriptorCache) IsMetricBroken(name string) bool {
	broken, ok := cache.broken[name]
	return ok && broken
}

// GetMetricNames returns a list of all metric names from the cache.
func (cache *MetricDescriptorCache) GetMetricNames() []string {
	keys := make([]string, 0, len(cache.descriptors))
	for k := range cache.descriptors {
		keys = append(keys, k)
	}
	return keys
}

// MarkStale marks all records in the cache as stale until next Refresh() call.
func (cache *MetricDescriptorCache) MarkStale() {
	cache.fresh = false
}

// ValidateMetricDescriptors checks if metric descriptors differs from the values kept in the cache.
// If the value has changed then metric family is marked is broken. Use this method to verify that
// metrics with prefix "container.googleapis.com" haven't changed.
func (cache *MetricDescriptorCache) ValidateMetricDescriptors(metrics map[string]*dto.MetricFamily, whitelisted []string) {
	// Perform cache operation only if cache was recently refreshed. This is done mostly from the optimization point
	// of view, we don't want to check all metric descriptors too often, as they should change rarely.
	if !cache.fresh {
		return
	}
	for _, metricFamily := range metrics {
		if !isMetricWhitelisted(metricFamily.GetName(), whitelisted) {
			continue
		}
		metricDescriptor, ok := cache.descriptors[metricFamily.GetName()]
		if !ok {
			continue
		}
		updatedMetricDescriptor := MetricFamilyToMetricDescriptor(cache.config, metricFamily, metricDescriptor)
		if descriptorLabelSetChanged(metricDescriptor, updatedMetricDescriptor) || descriptorMetricKindChanged(metricDescriptor, updatedMetricDescriptor) {
			cache.broken[metricFamily.GetName()] = true
			metricFamilyDropped.WithLabelValues(cache.config.SourceConfig.Component, metricFamily.GetName()).Set(1.0)
			glog.Warningf("Definition of the metric %s was changed and metric is not going to be pushed", metricFamily.GetName())
		} else {
			metricFamilyDropped.WithLabelValues(cache.config.SourceConfig.Component, metricFamily.GetName()).Set(0.0)
		}
	}
}

// UpdateMetricDescriptors iterates over all metricFamilies and updates metricDescriptors in the Stackdriver if required.
func (cache *MetricDescriptorCache) UpdateMetricDescriptors(metrics map[string]*dto.MetricFamily, whitelisted []string) {
	// Perform cache operation only if cache was recently refreshed. This is done mostly from the optimization point
	// of view, we don't want to check all metric descriptors too often, as they should change rarely.
	if !cache.fresh {
		return
	}
	for _, metricFamily := range metrics {
		if isMetricWhitelisted(metricFamily.GetName(), whitelisted) {
			cache.updateMetricDescriptorIfStale(metricFamily)
		}
	}
}

func isMetricWhitelisted(metric string, whitelisted []string) bool {
	// Empty list means that we want to fetch all metrics.
	if len(whitelisted) == 0 {
		return true
	}
	for _, whitelistedMetric := range whitelisted {
		if whitelistedMetric == metric {
			return true
		}
	}
	return false
}

// updateMetricDescriptorIfStale checks if descriptor created from MetricFamily object differs from the existing one
// and updates if needed.
func (cache *MetricDescriptorCache) updateMetricDescriptorIfStale(metricFamily *dto.MetricFamily) {
	metricDescriptor, ok := cache.descriptors[metricFamily.GetName()]
	updatedMetricDescriptor := MetricFamilyToMetricDescriptor(cache.config, metricFamily, metricDescriptor)
	if !ok || descriptorChanged(metricDescriptor, updatedMetricDescriptor) {
		if updateMetricDescriptorInStackdriver(cache.service, cache.config.GceConfig, updatedMetricDescriptor) {
			cache.descriptors[metricFamily.GetName()] = updatedMetricDescriptor
		} else {
			cache.broken[metricFamily.GetName()] = true
		}
	}
}

func (cache *MetricDescriptorCache) getMetricDescriptor(metric string) *v3.MetricDescriptor {
	value, ok := cache.descriptors[metric]
	if !ok {
		glog.V(4).Infof("Metric %s was not found in the cache for component %v", metric, cache.config.SourceConfig.Component)
	}
	return value
}

func descriptorChanged(original *v3.MetricDescriptor, checked *v3.MetricDescriptor) bool {
	return descriptorDescriptionChanged(original, checked) || descriptorLabelSetChanged(original, checked) || descriptorMetricKindChanged(original, checked)
}

func descriptorDescriptionChanged(original *v3.MetricDescriptor, checked *v3.MetricDescriptor) bool {
	if original.Description != checked.Description {
		glog.V(4).Infof("Description is different, %v != %v", original.Description, checked.Description)
		return true
	}
	return false
}

func descriptorLabelSetChanged(original *v3.MetricDescriptor, checked *v3.MetricDescriptor) bool {
	for _, label := range checked.Labels {
		found := false
		for _, labelFromOriginal := range original.Labels {
			if label.Key == labelFromOriginal.Key {
				found = true
				break
			}
		}
		if !found {
			glog.V(4).Infof("Missing label %v in the original metric descriptor", label)
			return true
		}
	}
	return false
}

func descriptorMetricKindChanged(original *v3.MetricDescriptor, checked *v3.MetricDescriptor) bool {
	if original.MetricKind != checked.MetricKind {
		glog.V(4).Infof("Metric kind is different, %v != %v", original.MetricKind, checked.MetricKind)
		return true
	}
	return false
}

// Refresh function fetches all metric descriptors of all metrics defined for given component with a defined prefix
// and puts them into cache.
func (cache *MetricDescriptorCache) Refresh() {
	metricDescriptors, err := getMetricDescriptors(cache.service, cache.config)
	if err == nil {
		cache.descriptors = metricDescriptors
		cache.broken = make(map[string]bool)
		cache.fresh = true
	}
}
