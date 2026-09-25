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
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// reloadDebounce coalesces the burst of fsnotify events a ConfigMap update
// (or editor save) produces into one reload.
const reloadDebounce = 100 * time.Millisecond

// Store holds the current Config and hot-reloads it when the file changes.
// It watches the file's directory, not the file itself: kubelet updates
// mounted ConfigMaps by atomically swapping a `..data` symlink, which never
// fires a Write event on the file path.
//
// A failed reload keeps the previous config (Stale reports true and
// LoadErrors counts failures) — the proxy never drops to zero config
// because of a bad edit.
type Store struct {
	path string

	cur      atomic.Pointer[Config]
	stale    atomic.Bool
	loadErrs atomic.Uint64

	mu       sync.Mutex
	subs     []func(*Config)
	lastHash [sha256.Size]byte
}

// NewStore loads path; the initial load must succeed.
func NewStore(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, lastHash: sha256.Sum256(raw)}
	s.cur.Store(cfg)
	return s, nil
}

// Current returns the latest good config.
func (s *Store) Current() *Config { return s.cur.Load() }

// LoadErrors returns how many reloads have failed since start.
func (s *Store) LoadErrors() uint64 { return s.loadErrs.Load() }

// Stale reports whether the most recent reload attempt failed.
func (s *Store) Stale() bool { return s.stale.Load() }

// Subscribe registers fn to run (on the watch goroutine) after each
// successful reload.
func (s *Store) Subscribe(fn func(*Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

// Watch blocks until ctx is done, reloading on file changes.
func (s *Store) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating config watcher: %w", err)
	}
	defer func() { _ = w.Close() }()

	dir := filepath.Dir(s.path)
	if err := w.Add(dir); err != nil {
		return fmt.Errorf("watching %s: %w", dir, err)
	}

	var timer *time.Timer
	fire := make(chan struct{}, 1)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			slog.Warn("config watcher error", "error", err)
		case _, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Any event in the directory may be part of a ConfigMap
			// symlink swap; debounce and reload rather than trying to
			// pattern-match kubelet's rename dance.
			if timer == nil {
				timer = time.AfterFunc(reloadDebounce, func() {
					select {
					case fire <- struct{}{}:
					default:
					}
				})
			} else {
				timer.Reset(reloadDebounce)
			}
		case <-fire:
			timer = nil
			s.reload()
		}
	}
}

func (s *Store) reload() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		s.loadErrs.Add(1)
		s.stale.Store(true)
		slog.Error("config reload failed; keeping previous config", "error", err)
		return
	}
	// Directory watching surfaces unrelated events (sibling files, kubelet
	// touches); identical content must not churn subscribers/schedulers.
	hash := sha256.Sum256(raw)
	s.mu.Lock()
	unchanged := hash == s.lastHash
	s.mu.Unlock()
	if unchanged && !s.stale.Load() {
		return
	}
	cfg, err := Parse(raw)
	if err != nil {
		s.loadErrs.Add(1)
		s.stale.Store(true)
		slog.Error("config reload failed; keeping previous config", "error", err)
		return
	}
	s.mu.Lock()
	s.lastHash = hash
	s.mu.Unlock()
	s.cur.Store(cfg)
	s.stale.Store(false)
	slog.Info("config reloaded", "metrics", len(cfg.Metrics))
	s.mu.Lock()
	subs := slices.Clone(s.subs)
	s.mu.Unlock()
	for _, fn := range subs {
		fn(cfg)
	}
}
