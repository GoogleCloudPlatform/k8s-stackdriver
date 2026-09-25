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
	"os"
	"path/filepath"
	"testing"
)

// Every config in examples/ must parse and validate.
func TestExampleConfigsParse(t *testing.T) {
	matches, err := filepath.Glob("../../examples/*.yaml")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no example configs found: %v", err)
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := Parse(raw)
			if err != nil {
				t.Fatalf("does not parse: %v", err)
			}
			if len(cfg.Metrics) == 0 {
				t.Error("example resolves to zero metrics")
			}
		})
	}
}
