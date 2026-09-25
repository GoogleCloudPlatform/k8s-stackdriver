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

package translate

import (
	"github.com/prometheus/prometheus/model/labels"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
)

// MonitoredResourceLabel disambiguates metric types that map to multiple
// monitored-resource types (GMP does the same).
const MonitoredResourceLabel = "monitored_resource"

// buildLabels flattens resource labels + metric labels onto one sorted
// label set. Resource labels win the plain name; a metric label whose
// sanitized key collides gets a "metric_" prefix (GMP rule). The
// monitored-resource type is always present under MonitoredResourceLabel.
func buildLabels(ts *monitoringpb.TimeSeries) labels.Labels {
	res := ts.GetResource()
	b := labels.NewBuilder(labels.EmptyLabels())
	b.Set(MonitoredResourceLabel, res.GetType())

	taken := make(map[string]struct{}, len(res.GetLabels())+1)
	taken[MonitoredResourceLabel] = struct{}{}
	for k, v := range res.GetLabels() {
		sk := SanitizeLabelName(k)
		b.Set(sk, v)
		taken[sk] = struct{}{}
	}
	for k, v := range ts.GetMetric().GetLabels() {
		sk := SanitizeLabelName(k)
		if _, clash := taken[sk]; clash {
			sk = "metric_" + sk
		}
		b.Set(sk, v)
	}
	return b.Labels()
}
