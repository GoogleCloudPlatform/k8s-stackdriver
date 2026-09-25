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

// Package presets holds curated, embedded bundles of Cloud Monitoring
// metric types that config entries can reference by name
// (e.g. `- preset: node`).
package presets

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed bundles/*.yaml
var bundlesFS embed.FS

// Bundle is one named preset: a curated list of metric types.
type Bundle struct {
	// Description explains what the bundle covers.
	Description string `yaml:"description"`
	// Requires names the GKE monitoring package (or other precondition)
	// that must be enabled for these metrics to exist.
	Requires string `yaml:"requires"`
	// Types are exact Cloud Monitoring metric types.
	Types []string `yaml:"types"`
	// TypePrefixes are expanded against the live descriptor cache.
	TypePrefixes []string `yaml:"typePrefixes"`
}

var load = sync.OnceValues(func() (map[string]Bundle, error) {
	entries, err := bundlesFS.ReadDir("bundles")
	if err != nil {
		return nil, fmt.Errorf("reading embedded bundles: %w", err)
	}
	out := make(map[string]Bundle, len(entries))
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".yaml")
		raw, err := bundlesFS.ReadFile("bundles/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading bundle %s: %w", e.Name(), err)
		}
		var b Bundle
		if err := yaml.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("parsing bundle %s: %w", e.Name(), err)
		}
		if len(b.Types) == 0 && len(b.TypePrefixes) == 0 {
			return nil, fmt.Errorf("bundle %s has no types or typePrefixes", e.Name())
		}
		out[name] = b
	}
	return out, nil
})

// Get returns the named bundle.
func Get(name string) (Bundle, bool) {
	all, err := load()
	if err != nil {
		panic(fmt.Sprintf("embedded preset bundles are invalid: %v", err))
	}
	b, ok := all[name]
	return b, ok
}

// Names returns all bundle names, sorted.
func Names() []string {
	all, err := load()
	if err != nil {
		panic(fmt.Sprintf("embedded preset bundles are invalid: %v", err))
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
