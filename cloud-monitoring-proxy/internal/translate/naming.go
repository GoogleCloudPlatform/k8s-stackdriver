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

// Package translate converts Cloud Monitoring descriptors and time series
// into the Prometheus data model: GMP-compatible naming, flattened labels,
// kind mapping with DELTA re-accumulation, and distribution→histogram
// conversion.
package translate

import (
	"strings"
)

// gmpIngestedDomain is the domain GMP stores ingested Prometheus metrics
// under, as prometheus.googleapis.com/<original_name>/<type-suffix>.
const gmpIngestedDomain = "prometheus.googleapis.com/"

// PromName converts a Cloud Monitoring metric type to its Prometheus name
// following the conventions of Google's own PromQL surface:
//
//   - prometheus.googleapis.com/<name>/<suffix> → <name> (the original
//     Prometheus name, restored; the type suffix is dropped)
//   - anything else: first '/' becomes ':', every other character outside
//     [a-zA-Z0-9_] becomes '_'
//     (kubernetes.io/container/cpu/core_usage_time →
//     kubernetes_io:container_cpu_core_usage_time)
func PromName(metricType string) string {
	if orig, ok := strings.CutPrefix(metricType, gmpIngestedDomain); ok {
		// Strip the trailing type suffix (gauge, counter, histogram,
		// summary, unknown, unknown:counter). Original Prometheus names
		// cannot contain '/', so the first remaining slash ends the name.
		if i := strings.IndexByte(orig, '/'); i > 0 {
			return orig[:i]
		}
		return orig
	}
	domain, path, found := strings.Cut(metricType, "/")
	if !found {
		return sanitizeNamePart(metricType)
	}
	return sanitizeNamePart(domain) + ":" + sanitizeNamePart(path)
}

func sanitizeNamePart(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_',
			c >= '0' && c <= '9' && i > 0:
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// SanitizeLabelName maps a GCM label key to a valid Prometheus label name.
// Keys never start with "__" in the output (that prefix is reserved).
func SanitizeLabelName(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_',
			c >= '0' && c <= '9' && i > 0:
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	for strings.HasPrefix(out, "__") {
		out = out[1:]
	}
	return out
}
