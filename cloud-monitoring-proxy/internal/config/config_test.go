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

package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(empty): %v", err)
	}
	if got := cfg.Server.Listen; got != ":9090" {
		t.Errorf("Listen = %q, want :9090", got)
	}
	if got := cfg.Polling.DefaultInterval.Std(); got != 60*time.Second {
		t.Errorf("DefaultInterval = %v, want 60s", got)
	}
	if got := cfg.Polling.Concurrency; got != 10 {
		t.Errorf("Concurrency = %d, want 10", got)
	}
	if got := cfg.Polling.PageSize; got != 10000 {
		t.Errorf("PageSize = %d, want 10000", got)
	}
	if got := cfg.Polling.DescriptorRefresh.Std(); got != 30*time.Minute {
		t.Errorf("DescriptorRefresh = %v, want 30m", got)
	}
	if got := cfg.Limits.MaxSeriesPerMetric; got != 200000 {
		t.Errorf("MaxSeriesPerMetric = %d, want 200000", got)
	}
	if got := cfg.Limits.StaleAfter.Std(); got != 5*time.Minute {
		t.Errorf("StaleAfter = %v, want 5m", got)
	}
	if !cfg.Scope.ClusterScoped() {
		t.Error("ClusterScoped() = false by default, want true")
	}
}

func TestParseFull(t *testing.T) {
	cfg, err := Parse([]byte(`
scope:
  projectID: p
  location: us-central1
  clusterName: c
  clusterScopeFilter: false
metrics:
  - type: kubernetes.io/container/restart_count
    interval: 5m
    lookback: 10m
    filter: 'resource.labels.namespace_name = "prod"'
    rename: restarts
server:
  listen: ":8080"
  emitTimestamps: true
polling:
  defaultInterval: 2m
  concurrency: 4
  pageSize: 500
  descriptorRefresh: 1h
limits:
  maxSeriesPerMetric: 10
  staleAfter: 90s
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Scope.ClusterScoped() {
		t.Error("ClusterScoped() = true, want false (explicitly disabled)")
	}
	m := cfg.Metrics[0]
	if m.Interval.Std() != 5*time.Minute || m.Lookback.Std() != 10*time.Minute || m.Rename != "restarts" {
		t.Errorf("metric entry not parsed as expected: %+v", m)
	}
	if !cfg.Server.EmitTimestamps || cfg.Server.Listen != ":8080" {
		t.Errorf("server not parsed as expected: %+v", cfg.Server)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name, yaml, wantErr string
	}{
		{"none of the three", "metrics:\n  - interval: 60s\n", "exactly one of"},
		{"two of the three", "metrics:\n  - type: a\n    preset: node\n", "exactly one of"},
		{"interval floor", "metrics:\n  - type: a\n    interval: 30s\n", "below the 1m0s floor"},
		{"rename on prefix", "metrics:\n  - typePrefix: a/\n    rename: x\n", "rename requires"},
		{"unknown preset", "metrics:\n  - preset: nope\n", `unknown preset "nope"`},
		{"unknown field", "metrics:\n  - type: a\n    intervall: 60s\n", "field intervall not found"},
		{"bad duration", "metrics:\n  - type: a\n    interval: fast\n", `invalid duration "fast"`},
		{"negative duration", "metrics:\n  - type: a\n    interval: -60s\n", "must not be negative"},
		{"default interval floor", "polling:\n  defaultInterval: 59s\n", "below the 1m0s floor"},
		{"page size", "polling:\n  pageSize: 200000\n", "pageSize"},
		{"stale floor", "limits:\n  staleAfter: 10s\n", "staleAfter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("Parse succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestPresetExpansion(t *testing.T) {
	cfg, err := Parse([]byte(`
metrics:
  - preset: node
    interval: 5m
    filter: 'resource.labels.node_name = "n1"'
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Metrics) == 0 {
		t.Fatal("preset expanded to zero entries")
	}
	for _, m := range cfg.Metrics {
		if m.Preset != "" {
			t.Errorf("entry still has preset set after expansion: %+v", m)
		}
		if m.Type == "" && m.TypePrefix == "" {
			t.Errorf("expanded entry has neither type nor typePrefix: %+v", m)
		}
		if m.Interval.Std() != 5*time.Minute {
			t.Errorf("expanded entry did not inherit interval: %+v", m)
		}
		if m.Filter == "" {
			t.Errorf("expanded entry did not inherit filter: %+v", m)
		}
	}
}

func TestDuplicateLaterWins(t *testing.T) {
	cfg, err := Parse([]byte(`
metrics:
  - preset: node
  - type: kubernetes.io/node/cpu/core_usage_time
    interval: 15m
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var found *Metric
	count := 0
	for i := range cfg.Metrics {
		if cfg.Metrics[i].Type == "kubernetes.io/node/cpu/core_usage_time" {
			count++
			found = &cfg.Metrics[i]
		}
	}
	if count != 1 {
		t.Fatalf("duplicate type appears %d times after dedup, want 1", count)
	}
	if found.Interval.Std() != 15*time.Minute {
		t.Errorf("later entry did not win: interval = %v, want 15m", found.Interval.Std())
	}
}
