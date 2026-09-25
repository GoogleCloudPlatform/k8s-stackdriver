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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/translate"
)

var t0 = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)

func upd(name string, samples ...translate.Sample) translate.FamilyUpdate {
	return translate.FamilyUpdate{Name: name, Help: "h", Kind: translate.KindGauge, Samples: samples}
}

func sample(v float64, kv ...string) translate.Sample {
	return translate.Sample{Labels: labels.FromStrings(kv...), Value: v, PointEnd: t0}
}

func TestApplyInsertUpdate(t *testing.T) {
	s := NewStore()
	st := s.Apply("m.io/a", upd("a", sample(1, "x", "1"), sample(2, "x", "2")), 100, t0)
	if st.Added != 2 || st.Updated != 0 {
		t.Fatalf("first apply: %+v", st)
	}
	st = s.Apply("m.io/a", upd("a", sample(5, "x", "1")), 100, t0.Add(time.Minute))
	if st.Added != 0 || st.Updated != 1 {
		t.Fatalf("second apply: %+v", st)
	}
	s.Read(func(fams []*Family) {
		if len(fams) != 1 || len(fams[0].Series) != 2 {
			t.Fatalf("families: %+v", fams)
		}
		for _, sr := range fams[0].SortedSeries() {
			if sr.Labels.Get("x") == "1" {
				if sr.Value != 5 || !sr.LastSeen.Equal(t0.Add(time.Minute)) {
					t.Errorf("update not applied: %+v", sr)
				}
			} else if !sr.LastSeen.Equal(t0) {
				t.Errorf("untouched series LastSeen changed: %+v", sr)
			}
		}
	})
}

func TestSeriesLimit(t *testing.T) {
	s := NewStore()
	st := s.Apply("m.io/a", upd("a", sample(1, "x", "1"), sample(2, "x", "2"), sample(3, "x", "3")), 2, t0)
	if st.Added != 2 || st.DroppedLimit != 1 {
		t.Fatalf("limit not enforced: %+v", st)
	}
	// Existing series still update at the limit.
	st = s.Apply("m.io/a", upd("a", sample(9, "x", "1"), sample(4, "x", "4")), 2, t0)
	if st.Updated != 1 || st.DroppedLimit != 1 {
		t.Fatalf("update-at-limit: %+v", st)
	}
}

func TestCollisionRejected(t *testing.T) {
	s := NewStore()
	s.Apply("m.io/a", upd("same_name", sample(1, "x", "1")), 10, t0)
	st := s.Apply("m.io/b", upd("same_name", sample(2, "x", "2")), 10, t0)
	if !st.CollisionRejected || st.Added != 0 {
		t.Fatalf("collision not rejected: %+v", st)
	}
	if _, series := s.Stats(); series != 1 {
		t.Errorf("collision update leaked series: %d", series)
	}
}

func TestSweepAndInternRefcount(t *testing.T) {
	s := NewStore()
	shared := sample(1, "pod", "p1", "namespace", "default")
	// Same label set in two families → one intern entry, two refs.
	s.Apply("m.io/a", upd("a", shared), 10, t0)
	s.Apply("m.io/b", upd("b", shared), 10, t0)
	if got := s.internSize(); got != 1 {
		t.Fatalf("internSize = %d, want 1 (shared)", got)
	}

	// Age out only family a's series by re-applying b later.
	s.Apply("m.io/b", upd("b", shared), 10, t0.Add(10*time.Minute))
	es, ef := s.Sweep(t0.Add(5 * time.Minute))
	if es != 1 || ef != 1 {
		t.Fatalf("sweep = %d series, %d families; want 1,1", es, ef)
	}
	if got := s.internSize(); got != 1 {
		t.Errorf("intern entry dropped while still referenced: %d", got)
	}
	// Evict the rest: intern pool must empty.
	if es, ef = s.Sweep(t0.Add(time.Hour)); es != 1 || ef != 1 {
		t.Fatalf("final sweep = %d,%d", es, ef)
	}
	if got := s.internSize(); got != 0 {
		t.Errorf("intern pool leaked: %d", got)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			s.Apply("m.io/a", upd("a", sample(float64(i), "i", fmt.Sprint(i%100))), 1000, t0.Add(time.Duration(i)))
			if i%10 == 0 {
				s.Sweep(t0)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			s.Read(func(fams []*Family) {
				for _, f := range fams {
					f.SortedSeries()
				}
			})
			s.Stats()
		}
	}()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
