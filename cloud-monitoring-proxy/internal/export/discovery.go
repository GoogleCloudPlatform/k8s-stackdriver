/*
Copyright 2026 Google Inc.

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

package export

import (
	"encoding/json"
	"net/http"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// DiscoveryEntry describes one available metric type: everything a
// user needs to build a config entry and to know what the metric will
// be called on /metrics.
type DiscoveryEntry struct {
	Type                   string   `json:"type"`
	PromName               string   `json:"promName"`
	Kind                   string   `json:"kind"`
	ValueType              string   `json:"valueType"`
	Unit                   string   `json:"unit,omitempty"`
	SamplePeriod           string   `json:"samplePeriod,omitempty"`
	IngestDelay            string   `json:"ingestDelay,omitempty"`
	MonitoredResourceTypes []string `json:"monitoredResourceTypes,omitempty"`
	Description            string   `json:"description,omitempty"`
}

type discoveryResponse struct {
	Count   int              `json:"count"`
	Metrics []DiscoveryEntry `json:"metrics"`
}

// DiscoveryHandler serves GET /api/v1/discovery[?prefix=<metric type prefix>]
// from the descriptor cache (read-only; no GCM calls on request).
func DiscoveryHandler(descs *gcm.DescriptorCache) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		all := descs.All()
		entries := make([]DiscoveryEntry, 0, len(all))
		for _, md := range all {
			if prefix != "" {
				if t := md.GetType(); len(t) < len(prefix) || t[:len(prefix)] != prefix {
					continue
				}
			}
			e := DiscoveryEntry{
				Type:                   md.GetType(),
				PromName:               translate.PromName(md.GetType()),
				Kind:                   md.GetMetricKind().String(),
				ValueType:              md.GetValueType().String(),
				Unit:                   md.GetUnit(),
				MonitoredResourceTypes: md.GetMonitoredResourceTypes(),
				Description:            md.GetDescription(),
			}
			if meta := md.GetMetadata(); meta != nil {
				if d := meta.GetSamplePeriod().AsDuration(); d > 0 {
					e.SamplePeriod = d.String()
				}
				if d := meta.GetIngestDelay().AsDuration(); d > 0 {
					e.IngestDelay = d.String()
				}
			}
			entries = append(entries, e)
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(discoveryResponse{Count: len(entries), Metrics: entries}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}
