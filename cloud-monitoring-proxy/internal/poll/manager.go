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

// Package poll runs the fetch pipeline: one scheduler per config entry
// polls its metric type(s) from GCM, translates, and applies to the cache.
// Config hot-reloads rewire the schedulers; DELTA accumulation state
// survives rewires.
package poll

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/genproto/googleapis/api/metric"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/cache"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/scope"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// lookbackMargin pads the auto lookback window beyond the descriptor's
// ingestDelay+samplePeriod, absorbing clock skew and late ingestion.
const lookbackMargin = 30 * time.Second

// defaultLookback applies when a descriptor carries no timing metadata.
const defaultLookback = 5 * time.Minute

// Hooks receives pipeline instrumentation; any field may be nil.
type Hooks struct {
	OnPoll      func(metricType string, dur time.Duration, series int, ts translate.Stats, cs cache.ApplyStats)
	OnPollError func(metricType, reason string)
	OnSweep     func(evictedSeries, evictedFamilies, evictedDeltaStates int)
}

func (h Hooks) poll(mt string, d time.Duration, n int, ts translate.Stats, cs cache.ApplyStats) {
	if h.OnPoll != nil {
		h.OnPoll(mt, d, n, ts, cs)
	}
}

func (h Hooks) pollError(mt, reason string) {
	if h.OnPollError != nil {
		h.OnPollError(mt, reason)
	}
}

// typeState serializes translate+apply per metric type (two entries may
// legitimately cover the same type, e.g. a prefix overlapping an exact
// entry) and owns its DELTA accumulator across config rewires.
type typeState struct {
	mu sync.Mutex
	tr *translate.Translator
}

// Manager owns the schedulers. Create with New, drive with Run.
type Manager struct {
	client gcm.Client
	descs  *gcm.DescriptorCache
	store  *cache.Store
	scope  scope.Scope
	hooks  Hooks

	now       func() time.Time
	maxJitter time.Duration

	stMu   sync.Mutex
	states map[string]*typeState

	rewireMu  sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	nMetrics  atomic.Int64
	firstPoll atomic.Bool
}

func New(client gcm.Client, descs *gcm.DescriptorCache, store *cache.Store, sc scope.Scope, hooks Hooks) *Manager {
	m := &Manager{
		client:    client,
		descs:     descs,
		store:     store,
		scope:     sc,
		hooks:     hooks,
		now:       time.Now,
		maxJitter: 3 * time.Second,
		states:    map[string]*typeState{},
	}
	m.nMetrics.Store(-1) // sentinel: not wired yet → not ready
	return m
}

// Ready reports whether at least one poll attempt completed (or the config
// has no metrics at all) — the readiness signal for /readyz.
func (m *Manager) Ready() bool {
	return m.firstPoll.Load() || m.nMetrics.Load() == 0
}

// Run wires the current config, re-wires on every reload, and blocks until
// ctx is done.
func (m *Manager) Run(ctx context.Context, cfgStore *config.Store) {
	m.Rewire(ctx, cfgStore.Current())
	cfgStore.Subscribe(func(c *config.Config) { m.Rewire(ctx, c) })
	<-ctx.Done()
	m.rewireMu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.rewireMu.Unlock()
	m.wg.Wait()
}

// Rewire replaces the scheduler set to match cfg. DELTA state (typeState)
// is retained so counters don't reset on config edits.
func (m *Manager) Rewire(ctx context.Context, cfg *config.Config) {
	m.rewireMu.Lock()
	defer m.rewireMu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.wg.Wait()
	}
	wctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	m.nMetrics.Store(int64(len(cfg.Metrics)))
	for _, entry := range cfg.Metrics {
		m.wg.Add(1)
		go func(e config.Metric) {
			defer m.wg.Done()
			m.runEntry(wctx, e, cfg)
		}(entry)
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runSweeper(wctx, cfg.Limits.StaleAfter.Std())
	}()
	slog.Info("poll schedulers wired", "entries", len(cfg.Metrics))
}

func (m *Manager) runEntry(ctx context.Context, e config.Metric, cfg *config.Config) {
	interval := cfg.Polling.DefaultInterval.Std()
	if e.Interval != 0 {
		interval = e.Interval.Std()
	}
	if m.maxJitter > 0 {
		select {
		case <-time.After(rand.N(m.maxJitter)):
		case <-ctx.Done():
			return
		}
	}
	m.pollEntry(ctx, e, cfg)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.pollEntry(ctx, e, cfg)
		}
	}
}

// pollEntry resolves the entry to concrete metric types (prefix entries
// re-expand every tick, so descriptor refreshes pick up new types) and
// polls each.
func (m *Manager) pollEntry(ctx context.Context, e config.Metric, cfg *config.Config) {
	switch {
	case e.Type != "":
		m.pollType(ctx, e, e.Type, cfg)
	case e.TypePrefix != "":
		types := m.descs.ExpandPrefix(e.TypePrefix)
		if len(types) == 0 {
			m.hooks.pollError(e.TypePrefix, "prefix_no_match")
			return
		}
		for _, t := range types {
			if ctx.Err() != nil {
				return
			}
			m.pollType(ctx, e, t, cfg)
		}
	}
	m.firstPoll.Store(true)
}

func (m *Manager) pollType(ctx context.Context, e config.Metric, metricType string, cfg *config.Config) {
	start := m.now()
	md, ok := m.descs.Get(metricType)
	if !ok {
		m.hooks.pollError(metricType, "no_descriptor")
		return
	}
	extras, scopable := m.scopeFilters(md)
	if !scopable {
		m.hooks.pollError(metricType, "unscopable")
		slog.Warn("metric not scopable to this cluster; skipping (set scope.clusterScopeFilter: false to serve unscoped)",
			"metric", metricType, "resourceTypes", md.GetMonitoredResourceTypes())
		return
	}
	if e.Filter != "" {
		extras = append(extras, e.Filter)
	}

	end := m.now()
	series, err := m.client.ListSeries(ctx, gcm.SeriesQuery{
		MetricType:   metricType,
		ExtraFilters: extras,
		Start:        end.Add(-m.lookback(e, md)),
		End:          end,
		PageSize:     cfg.Polling.PageSize,
	})
	if err != nil {
		reason := "api_error"
		if gcm.IsQuotaErr(err) {
			reason = "quota"
		}
		m.hooks.pollError(metricType, reason)
		slog.Error("poll failed", "metric", metricType, "reason", reason, "error", err)
		return
	}

	rename := ""
	if e.Type != "" {
		rename = e.Rename
	}
	st := m.state(metricType)
	st.mu.Lock()
	fam, tstats := st.tr.Translate(md, series, rename, m.now())
	cstats := m.store.Apply(metricType, fam, cfg.Limits.MaxSeriesPerMetric, m.now())
	st.mu.Unlock()

	if cstats.CollisionRejected {
		m.hooks.pollError(metricType, "name_collision")
		slog.Error("output name already owned by another metric type; dropping update",
			"metric", metricType, "name", fam.Name)
	}
	m.hooks.poll(metricType, m.now().Sub(start), len(series), tstats, cstats)
}

// lookback: explicit override, else ingestDelay + samplePeriod + margin.
func (m *Manager) lookback(e config.Metric, md *metric.MetricDescriptor) time.Duration {
	if e.Lookback != 0 {
		return e.Lookback.Std()
	}
	meta := md.GetMetadata()
	delay := meta.GetIngestDelay().AsDuration()
	period := meta.GetSamplePeriod().AsDuration()
	if delay == 0 && period == 0 {
		return defaultLookback
	}
	return delay + period + lookbackMargin
}

// scopeFilters builds the cluster restriction for the metric's monitored
// resource types. Returns ok=false when scoping is on but no resource type
// carries cluster labels (the metric would leak cross-cluster data).
func (m *Manager) scopeFilters(md *metric.MetricDescriptor) ([]string, bool) {
	if !m.scope.ClusterScopeFilter {
		return nil, true
	}
	var k8s, promTarget bool
	for _, rt := range md.GetMonitoredResourceTypes() {
		switch rt {
		case "k8s_container", "k8s_pod", "k8s_node", "k8s_cluster":
			k8s = true
		case "prometheus_target":
			promTarget = true
		}
	}
	var clauses []string
	if k8s {
		clauses = append(clauses,
			`resource.labels.cluster_name = "`+m.scope.ClusterName+`" AND resource.labels.location = "`+m.scope.Location+`"`)
	}
	if promTarget {
		clauses = append(clauses,
			`resource.labels.cluster = "`+m.scope.ClusterName+`" AND resource.labels.location = "`+m.scope.Location+`"`)
	}
	switch len(clauses) {
	case 0:
		return nil, false
	case 1:
		return clauses, true
	default:
		return []string{"(" + clauses[0] + ") OR (" + clauses[1] + ")"}, true
	}
}

func (m *Manager) state(metricType string) *typeState {
	m.stMu.Lock()
	defer m.stMu.Unlock()
	st, ok := m.states[metricType]
	if !ok {
		st = &typeState{tr: translate.New()}
		m.states[metricType] = st
	}
	return st
}

func (m *Manager) runSweeper(ctx context.Context, staleAfter time.Duration) {
	interval := staleAfter / 2
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.SweepOnce(staleAfter)
		}
	}
}

// SweepOnce evicts cache series and DELTA state unseen for staleAfter.
func (m *Manager) SweepOnce(staleAfter time.Duration) {
	cutoff := m.now().Add(-staleAfter)
	es, ef := m.store.Sweep(cutoff)
	removed := 0
	m.stMu.Lock()
	states := make([]*typeState, 0, len(m.states))
	for _, st := range m.states {
		states = append(states, st)
	}
	m.stMu.Unlock()
	for _, st := range states {
		st.mu.Lock()
		removed += st.tr.SweepStale(cutoff)
		st.mu.Unlock()
	}
	if m.hooks.OnSweep != nil {
		m.hooks.OnSweep(es, ef, removed)
	}
}
