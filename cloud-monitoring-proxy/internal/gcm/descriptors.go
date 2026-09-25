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
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/genproto/googleapis/api/metric"
)

// DescriptorCache holds the project's metric descriptors: the source of
// metric kind/valueType (needed to translate), samplePeriod/ingestDelay
// (auto-lookback), monitoredResourceTypes (cluster scoping), and the data
// behind /api/v1/discovery and typePrefix expansion.
type DescriptorCache struct {
	client Client

	mu     sync.RWMutex
	byType map[string]*metric.MetricDescriptor
	sorted []*metric.MetricDescriptor
}

func NewDescriptorCache(client Client) *DescriptorCache {
	return &DescriptorCache{client: client, byType: map[string]*metric.MetricDescriptor{}}
}

// Refresh reloads all descriptors in the project. The full set (a few
// thousand entries) is small, is what /api/v1/discovery serves, and
// descriptor list calls are not billed per series.
func (d *DescriptorCache) Refresh(ctx context.Context) error {
	descs, err := d.client.ListDescriptors(ctx, "")
	if err != nil {
		return fmt.Errorf("refreshing descriptor cache: %w", err)
	}
	byType := make(map[string]*metric.MetricDescriptor, len(descs))
	for _, md := range descs {
		byType[md.GetType()] = md
	}
	sorted := make([]*metric.MetricDescriptor, len(descs))
	copy(sorted, descs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GetType() < sorted[j].GetType() })

	d.mu.Lock()
	d.byType = byType
	d.sorted = sorted
	d.mu.Unlock()
	return nil
}

// StartRefreshing refreshes every interval until ctx is done. Failures are
// logged and retried at the next tick (the previous cache keeps serving).
func (d *DescriptorCache) StartRefreshing(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := d.Refresh(ctx); err != nil {
				slog.Error("descriptor cache refresh failed; keeping previous", "error", err)
			}
		}
	}
}

// Get returns the descriptor for an exact metric type.
func (d *DescriptorCache) Get(metricType string) (*metric.MetricDescriptor, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	md, ok := d.byType[metricType]
	return md, ok
}

// ExpandPrefix returns all known metric types with the given prefix, sorted.
func (d *DescriptorCache) ExpandPrefix(prefix string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []string
	for _, md := range d.sorted {
		if strings.HasPrefix(md.GetType(), prefix) {
			out = append(out, md.GetType())
		}
	}
	return out
}

// All returns every cached descriptor, sorted by type. Callers must not
// mutate the returned slice or descriptors.
func (d *DescriptorCache) All() []*metric.MetricDescriptor {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sorted
}

// Len returns the number of cached descriptors.
func (d *DescriptorCache) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.sorted)
}
