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

// Package config implements the proxy's YAML configuration: schema,
// defaults, validation, preset expansion, and hot-reload (see store.go).
// Schema reference: docs/configuration.md.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/presets"
)

// MinInterval is the floor for polling intervals: GCM's finest resolution
// for most metrics is 60s, so polling faster is pure cost.
const MinInterval = 60 * time.Second

type Config struct {
	Scope   Scope    `yaml:"scope"`
	Metrics []Metric `yaml:"metrics"`
	Server  Server   `yaml:"server"`
	Polling Polling  `yaml:"polling"`
	Limits  Limits   `yaml:"limits"`
}

type Scope struct {
	ProjectID   string `yaml:"projectID"`
	Location    string `yaml:"location"`
	ClusterName string `yaml:"clusterName"`
	// ClusterScopeFilter (default true) ANDs cluster_name/location resource
	// label filters onto every query whose resource type carries them.
	ClusterScopeFilter *bool `yaml:"clusterScopeFilter"`
}

// ClusterScoped reports whether queries must be restricted to this cluster.
func (s Scope) ClusterScoped() bool {
	return s.ClusterScopeFilter == nil || *s.ClusterScopeFilter
}

// Metric is one entry in the metrics list. Exactly one of Preset, Type, or
// TypePrefix must be set. When two entries resolve to the same metric type,
// the later entry wins (lets explicit entries override preset members).
type Metric struct {
	Preset     string `yaml:"preset"`
	Type       string `yaml:"type"`
	TypePrefix string `yaml:"typePrefix"`

	// Interval overrides polling.defaultInterval (0 = use default).
	Interval Duration `yaml:"interval"`
	// Lookback overrides the auto window derived from the metric
	// descriptor's ingestDelay+samplePeriod (0 = auto).
	Lookback Duration `yaml:"lookback"`
	// Filter is ANDed onto the GCM query filter.
	Filter string `yaml:"filter"`
	// Rename overrides the converted Prometheus name (Type entries only).
	Rename string `yaml:"rename"`
}

type Server struct {
	Listen         string `yaml:"listen"`
	EmitTimestamps bool   `yaml:"emitTimestamps"`
}

type Polling struct {
	DefaultInterval   Duration `yaml:"defaultInterval"`
	Concurrency       int      `yaml:"concurrency"`
	PageSize          int32    `yaml:"pageSize"`
	DescriptorRefresh Duration `yaml:"descriptorRefresh"`
}

type Limits struct {
	MaxSeriesPerMetric int      `yaml:"maxSeriesPerMetric"`
	StaleAfter         Duration `yaml:"staleAfter"`
}

// Load reads, parses (strict), defaults, preset-expands, and validates the
// config at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return Parse(raw)
}

// Parse is Load for in-memory bytes.
func Parse(raw []byte) (*Config, error) {
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := cfg.expandPresets(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = ":9090"
	}
	if c.Polling.DefaultInterval == 0 {
		c.Polling.DefaultInterval = Duration(MinInterval)
	}
	if c.Polling.Concurrency == 0 {
		c.Polling.Concurrency = 10
	}
	if c.Polling.PageSize == 0 {
		c.Polling.PageSize = 10000
	}
	if c.Polling.DescriptorRefresh == 0 {
		c.Polling.DescriptorRefresh = Duration(30 * time.Minute)
	}
	if c.Limits.MaxSeriesPerMetric == 0 {
		c.Limits.MaxSeriesPerMetric = 200000
	}
	if c.Limits.StaleAfter == 0 {
		c.Limits.StaleAfter = Duration(5 * time.Minute)
	}
}

func (c *Config) validate() error {
	var errs []error
	for i, m := range c.Metrics {
		set := 0
		for _, v := range []string{m.Preset, m.Type, m.TypePrefix} {
			if v != "" {
				set++
			}
		}
		if set != 1 {
			errs = append(errs, fmt.Errorf("metrics[%d]: exactly one of preset, type, typePrefix must be set", i))
			continue
		}
		if m.Interval != 0 && m.Interval.Std() < MinInterval {
			errs = append(errs, fmt.Errorf("metrics[%d]: interval %s is below the %s floor", i, m.Interval, MinInterval))
		}
		if m.Rename != "" && m.Type == "" {
			errs = append(errs, fmt.Errorf("metrics[%d]: rename requires an exact type entry", i))
		}
		if m.Preset != "" {
			if _, ok := presets.Get(m.Preset); !ok {
				errs = append(errs, fmt.Errorf("metrics[%d]: unknown preset %q (available: %s)",
					i, m.Preset, strings.Join(presets.Names(), ", ")))
			}
		}
	}
	if c.Polling.DefaultInterval.Std() < MinInterval {
		errs = append(errs, fmt.Errorf("polling.defaultInterval %s is below the %s floor", c.Polling.DefaultInterval, MinInterval))
	}
	if c.Polling.Concurrency < 1 {
		errs = append(errs, errors.New("polling.concurrency must be >= 1"))
	}
	if c.Polling.PageSize < 1 || c.Polling.PageSize > 100000 {
		errs = append(errs, errors.New("polling.pageSize must be in [1, 100000]"))
	}
	if c.Polling.DescriptorRefresh.Std() < time.Minute {
		errs = append(errs, errors.New("polling.descriptorRefresh must be >= 1m"))
	}
	if c.Limits.MaxSeriesPerMetric < 1 {
		errs = append(errs, errors.New("limits.maxSeriesPerMetric must be >= 1"))
	}
	if c.Limits.StaleAfter.Std() < MinInterval {
		errs = append(errs, fmt.Errorf("limits.staleAfter %s is below the %s floor", c.Limits.StaleAfter, MinInterval))
	}
	return errors.Join(errs...)
}

// expandPresets replaces preset entries with their bundle members
// (inheriting the entry's interval/lookback/filter) and deduplicates by
// resolved type/typePrefix, later entries winning.
func (c *Config) expandPresets() error {
	var out []Metric
	for _, m := range c.Metrics {
		if m.Preset == "" {
			out = append(out, m)
			continue
		}
		b, ok := presets.Get(m.Preset)
		if !ok {
			return fmt.Errorf("unknown preset %q", m.Preset) // validate() catches this first
		}
		for _, t := range b.Types {
			out = append(out, Metric{Type: t, Interval: m.Interval, Lookback: m.Lookback, Filter: m.Filter})
		}
		for _, p := range b.TypePrefixes {
			out = append(out, Metric{TypePrefix: p, Interval: m.Interval, Lookback: m.Lookback, Filter: m.Filter})
		}
	}
	seen := make(map[string]int, len(out)) // key -> index in deduped
	var deduped []Metric
	for _, m := range out {
		key := "t:" + m.Type
		if m.TypePrefix != "" {
			key = "p:" + m.TypePrefix
		}
		if idx, dup := seen[key]; dup {
			deduped[idx] = m // later wins
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, m)
	}
	c.Metrics = deduped
	return nil
}
