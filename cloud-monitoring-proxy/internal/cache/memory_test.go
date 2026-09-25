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

package cache

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

// TestMemoryFootprint loads 20 families × 10k shared label sets (200k
// series, the realistic shape: many metric types over the same
// pods/containers) and reports bytes/series extrapolated to 3M series.
// Skipped under -race (memory accounting is distorted) and -short.
func TestMemoryFootprint(t *testing.T) {
	if raceEnabled {
		t.Skip("memory accounting distorted under -race")
	}
	if testing.Short() {
		t.Skip("short mode")
	}

	const nFamilies, nSets = 20, 10000
	lsets := make([]labels.Labels, nSets)
	for i := range lsets {
		lsets[i] = labels.FromStrings(
			"monitored_resource", "k8s_container",
			"project_id", "example-project",
			"location", "us-central1",
			"cluster_name", "e2e-cluster",
			"namespace_name", fmt.Sprintf("namespace-%d", i%50),
			"pod_name", fmt.Sprintf("workload-abcdef-%06d", i),
			"container_name", "main",
		)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	s := NewStore()
	now := time.Now()
	for f := 0; f < nFamilies; f++ {
		samples := make([]translate.Sample, nSets)
		for i := range samples {
			samples[i] = translate.Sample{Labels: lsets[i], Value: float64(i), PointEnd: now}
		}
		s.Apply(fmt.Sprintf("kubernetes.io/container/metric_%d", f),
			translate.FamilyUpdate{Name: fmt.Sprintf("kubernetes_io:container_metric_%d", f), Kind: translate.KindGauge, Samples: samples},
			nSets+1, now)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	fams, series := s.Stats()
	perSeries := float64(after.HeapAlloc-before.HeapAlloc) / float64(series)
	at3M := perSeries * 3_000_000 / (1 << 20)
	t.Logf("families=%d series=%d heap=%.1f MiB per-series=%.0f B extrapolated@3M=%.0f MiB",
		fams, series, float64(after.HeapAlloc-before.HeapAlloc)/(1<<20), perSeries, at3M)
	if at3M > 500 {
		t.Errorf("extrapolated footprint %.0f MiB at 3M series exceeds the 500 MiB design target", at3M)
	}
	runtime.KeepAlive(s)
}

func BenchmarkApply10k(b *testing.B) {
	lsets := make([]labels.Labels, 10000)
	for i := range lsets {
		lsets[i] = labels.FromStrings("pod_name", fmt.Sprintf("pod-%06d", i), "namespace_name", "default")
	}
	samples := make([]translate.Sample, len(lsets))
	now := time.Now()
	for i := range samples {
		samples[i] = translate.Sample{Labels: lsets[i], Value: float64(i), PointEnd: now}
	}
	s := NewStore()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Apply("m.io/a", translate.FamilyUpdate{Name: "a", Kind: translate.KindGauge, Samples: samples}, len(samples)+1, now.Add(time.Duration(i)))
	}
}
