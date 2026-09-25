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

// Package cache is the in-memory latest-value series store between the
// poller and the exposition layer. No persistence: GCM is the durable
// store, and a restart refills the cache within one poll cycle.
package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// Series is the latest state of one time series.
type Series struct {
	Labels labels.Labels
	Hash   uint64
	Value  float64
	Hist   *translate.HistogramValue
	// PointEnd is the GCM timestamp of the value (drives data-age and the
	// optional honest-timestamp exposition mode).
	PointEnd time.Time
	// LastSeen is the poll time this series last appeared; the sweeper
	// evicts series unseen for limits.staleAfter.
	LastSeen time.Time
}

// Family is one output metric family. It is owned by exactly one GCM
// metric type (post-conversion name collisions are rejected).
type Family struct {
	Name       string
	Help       string
	Kind       translate.Kind
	MetricType string
	Series     map[uint64]*Series
}

// ApplyStats reports what one Apply call did.
type ApplyStats struct {
	Added, Updated int
	// DroppedLimit counts new series rejected by maxSeriesPerMetric.
	DroppedLimit int
	// CollisionRejected is set when the update's family name is already
	// owned by a different metric type; the whole update was dropped.
	CollisionRejected bool
}

// internEntry shares one packed label set across families (the same
// container/node label set typically appears in dozens of metric types).
type internEntry struct {
	lbls labels.Labels
	refs int
}

// Store is safe for one writer (the poll pipeline) plus concurrent readers.
type Store struct {
	mu       sync.RWMutex
	families map[string]*Family
	intern   map[uint64]*internEntry
}

func NewStore() *Store {
	return &Store{
		families: map[string]*Family{},
		intern:   map[uint64]*internEntry{},
	}
}

// Apply merges one metric type's translated poll result. maxSeries bounds
// the family's series count (existing series always update; only new
// series are dropped at the limit).
func (s *Store) Apply(metricType string, upd translate.FamilyUpdate, maxSeries int, now time.Time) ApplyStats {
	var st ApplyStats
	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.families[upd.Name]
	if ok && f.MetricType != metricType {
		st.CollisionRejected = true
		return st
	}
	if !ok {
		f = &Family{Name: upd.Name, MetricType: metricType, Series: map[uint64]*Series{}}
		s.families[upd.Name] = f
	}
	f.Help = upd.Help
	f.Kind = upd.Kind

	for _, sm := range upd.Samples {
		h := sm.Labels.Hash()
		if existing, ok := f.Series[h]; ok {
			existing.Value = sm.Value
			existing.Hist = sm.Hist
			existing.PointEnd = sm.PointEnd
			existing.LastSeen = now
			st.Updated++
			continue
		}
		if len(f.Series) >= maxSeries {
			st.DroppedLimit++
			continue
		}
		f.Series[h] = &Series{
			Labels:   s.internLabels(h, sm.Labels),
			Hash:     h,
			Value:    sm.Value,
			Hist:     sm.Hist,
			PointEnd: sm.PointEnd,
			LastSeen: now,
		}
		st.Added++
	}
	return st
}

// Sweep evicts series unseen since cutoff and removes emptied families.
func (s *Store) Sweep(cutoff time.Time) (evictedSeries, evictedFamilies int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, f := range s.families {
		for h, sr := range f.Series {
			if sr.LastSeen.Before(cutoff) {
				s.releaseLabels(h)
				delete(f.Series, h)
				evictedSeries++
			}
		}
		if len(f.Series) == 0 {
			delete(s.families, name)
			evictedFamilies++
		}
	}
	return evictedSeries, evictedFamilies
}

// Read calls fn with all families sorted by name, under the read lock.
// fn must not retain or mutate anything it is handed.
func (s *Store) Read(fn func(families []*Family)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sorted := make([]*Family, 0, len(s.families))
	for _, f := range s.families {
		sorted = append(sorted, f)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	fn(sorted)
}

// SortedSeries returns the family's series sorted by label set, for
// deterministic exposition output.
func (f *Family) SortedSeries() []*Series {
	out := make([]*Series, 0, len(f.Series))
	for _, sr := range f.Series {
		out = append(out, sr)
	}
	sort.Slice(out, func(i, j int) bool {
		return labels.Compare(out[i].Labels, out[j].Labels) < 0
	})
	return out
}

// Stats returns total family and series counts.
func (s *Store) Stats() (families, series int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.families {
		series += len(f.Series)
	}
	return len(s.families), series
}

func (s *Store) internLabels(h uint64, l labels.Labels) labels.Labels {
	if e, ok := s.intern[h]; ok {
		e.refs++
		return e.lbls
	}
	s.intern[h] = &internEntry{lbls: l, refs: 1}
	return l
}

func (s *Store) releaseLabels(h uint64) {
	if e, ok := s.intern[h]; ok {
		if e.refs--; e.refs <= 0 {
			delete(s.intern, h)
		}
	}
}

// internSize is exported to tests.
func (s *Store) internSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.intern)
}
