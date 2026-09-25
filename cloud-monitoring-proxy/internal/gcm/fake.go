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

package gcm

import (
	"context"
	"strings"
	"sync"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/genproto/googleapis/api/metric"
)

// Fake is an in-memory Client for tests (and e2e local mode).
type Fake struct {
	mu sync.Mutex

	Descriptors []*metric.MetricDescriptor
	// SeriesByType maps metric type -> series returned for it.
	SeriesByType map[string][]*monitoringpb.TimeSeries
	// Err, when set, fails every call (simulates outage/quota).
	Err error

	// SeriesCalls records every ListSeries query, in order.
	SeriesCalls []SeriesQuery
	// DescriptorCalls records every ListDescriptors filter.
	DescriptorCalls []string
}

var _ Client = (*Fake)(nil)

func (f *Fake) ListDescriptors(_ context.Context, filter string) ([]*metric.MetricDescriptor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.DescriptorCalls = append(f.DescriptorCalls, filter)
	if f.Err != nil {
		return nil, f.Err
	}
	// Support the one filter form the proxy uses on descriptors:
	// metric.type = starts_with("prefix"), and "" for all.
	if filter == "" {
		return f.Descriptors, nil
	}
	prefix, ok := parseStartsWith(filter)
	if !ok {
		return f.Descriptors, nil
	}
	var out []*metric.MetricDescriptor
	for _, d := range f.Descriptors {
		if strings.HasPrefix(d.GetType(), prefix) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *Fake) ListSeries(_ context.Context, q SeriesQuery) ([]*monitoringpb.TimeSeries, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SeriesCalls = append(f.SeriesCalls, q)
	if f.Err != nil {
		return nil, f.Err
	}
	return f.SeriesByType[q.MetricType], nil
}

func (f *Fake) Close() error { return nil }

// SetSeries replaces the series for a metric type (goroutine-safe).
func (f *Fake) SetSeries(metricType string, series []*monitoringpb.TimeSeries) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SeriesByType == nil {
		f.SeriesByType = map[string][]*monitoringpb.TimeSeries{}
	}
	f.SeriesByType[metricType] = series
}

// SetErr sets the injected error (goroutine-safe).
func (f *Fake) SetErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Err = err
}

// NumSeriesCalls returns how many ListSeries calls were made.
func (f *Fake) NumSeriesCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.SeriesCalls)
}

func parseStartsWith(filter string) (string, bool) {
	const pre = `metric.type = starts_with("`
	const suf = `")`
	if strings.HasPrefix(filter, pre) && strings.HasSuffix(filter, suf) {
		return strings.TrimSuffix(strings.TrimPrefix(filter, pre), suf), true
	}
	return "", false
}
