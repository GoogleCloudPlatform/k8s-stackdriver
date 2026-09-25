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
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const goodA = "metrics:\n  - type: a.googleapis.com/x\n"
const goodB = "metrics:\n  - type: b.googleapis.com/y\n"
const bad = "metrics:\n  - type: a\n    preset: node\n"

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startWatch(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.Watch(ctx); err != nil {
			t.Errorf("Watch: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); <-done })
}

func TestStoreReloadGoodAndBad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(goodA), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var notified atomic.Int32
	s.Subscribe(func(*Config) { notified.Add(1) })
	startWatch(t, s)
	time.Sleep(150 * time.Millisecond) // let the watcher arm

	// Good reload: new content becomes visible, subscriber fires.
	if err := os.WriteFile(path, []byte(goodB), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "good reload", func() bool {
		return s.Current().Metrics[0].Type == "b.googleapis.com/y"
	})
	waitFor(t, "subscriber", func() bool { return notified.Load() >= 1 })
	if s.Stale() {
		t.Error("Stale() = true after successful reload")
	}

	// Bad reload: old config kept, error counted, stale set.
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bad reload counted", func() bool { return s.LoadErrors() == 1 })
	if got := s.Current().Metrics[0].Type; got != "b.googleapis.com/y" {
		t.Errorf("bad reload replaced config: type = %q", got)
	}
	if !s.Stale() {
		t.Error("Stale() = false after failed reload")
	}

	// Recovery: good content clears stale.
	if err := os.WriteFile(path, []byte(goodA), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "recovery", func() bool { return !s.Stale() })
}

// TestStoreConfigMapSymlinkSwap emulates kubelet's ConfigMap update dance:
// config.yaml -> ..data/config.yaml, and updates atomically swap the
// ..data symlink to a new timestamped directory.
func TestStoreConfigMapSymlinkSwap(t *testing.T) {
	dir := t.TempDir()

	write := func(version, content string) string {
		vdir := filepath.Join(dir, version)
		if err := os.Mkdir(vdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(vdir, "config.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return version
	}

	v1 := write("..2026_09_01_v1", goodA)
	if err := os.Symlink(v1, filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..data", "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}

	s, err := NewStore(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	startWatch(t, s)
	time.Sleep(150 * time.Millisecond)

	// Kubelet-style swap: new data dir, new symlink, atomic rename over ..data.
	v2 := write("..2026_09_01_v2", goodB)
	if err := os.Symlink(v2, filepath.Join(dir, "..data_tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "..data_tmp"), filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "symlink-swap reload", func() bool {
		return s.Current().Metrics[0].Type == "b.googleapis.com/y"
	})
}

// TestUnchangedContentDoesNotNotify: directory events for identical file
// content (kubelet touches, sibling files) must not churn subscribers.
func TestUnchangedContentDoesNotNotify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(goodA), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var notified atomic.Int32
	s.Subscribe(func(*Config) { notified.Add(1) })
	startWatch(t, s)
	time.Sleep(150 * time.Millisecond)

	// Rewrite identical content and create an unrelated sibling file.
	if err := os.WriteFile(path, []byte(goodA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sibling.log"), []byte("noise"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if n := notified.Load(); n != 0 {
		t.Errorf("subscribers notified %d times for unchanged content", n)
	}

	// A real change still notifies.
	if err := os.WriteFile(path, []byte(goodB), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "real change notification", func() bool { return notified.Load() >= 1 })
}
