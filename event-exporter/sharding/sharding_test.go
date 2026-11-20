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

package sharding

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestNewValidation(t *testing.T) {
	testCases := []struct {
		shardID     int
		totalShards int
		wantErr     bool
	}{
		{0, 1, false},
		{2, 3, false},
		{0, 0, true},
		{-1, 3, true},
		{3, 3, true},
	}
	for _, tc := range testCases {
		_, err := New(tc.shardID, tc.totalShards)
		if gotErr := err != nil; gotErr != tc.wantErr {
			t.Errorf("New(%d, %d) error = %v, wantErr %v", tc.shardID, tc.totalShards, err, tc.wantErr)
		}
	}
}

func TestEveryUIDOwnedByExactlyOneShard(t *testing.T) {
	const totalShards = 3
	sharders := make([]*Sharder, totalShards)
	for i := range sharders {
		s, err := New(i, totalShards)
		if err != nil {
			t.Fatalf("New(%d, %d) failed: %v", i, totalShards, err)
		}
		sharders[i] = s
	}

	ownedPerShard := make([]int, totalShards)
	for i := 0; i < 1000; i++ {
		uid := types.UID(fmt.Sprintf("uid-%d", i))
		owners := 0
		for shard, s := range sharders {
			if s.Owns(uid) {
				owners++
				ownedPerShard[shard]++
			}
		}
		if owners != 1 {
			t.Errorf("UID %q owned by %d shards, want exactly 1", uid, owners)
		}
	}

	// FNV-1a should spread UIDs roughly evenly; guard against a degenerate
	// distribution rather than asserting exact counts.
	for shard, count := range ownedPerShard {
		if count < 200 {
			t.Errorf("shard %d owns only %d of 1000 UIDs, distribution is too skewed", shard, count)
		}
	}
}

func TestDisabledSharderOwnsEverything(t *testing.T) {
	s, err := New(0, 1)
	if err != nil {
		t.Fatalf("New(0, 1) failed: %v", err)
	}
	if s.Enabled() {
		t.Error("Sharder with a single shard should not be enabled")
	}
	if !s.Owns("any-uid") || !s.Owns("") {
		t.Error("Sharder with a single shard should own every UID")
	}

	var nilSharder *Sharder
	if nilSharder.Enabled() {
		t.Error("nil Sharder should not be enabled")
	}
	if !nilSharder.Owns("any-uid") {
		t.Error("nil Sharder should own every UID")
	}
}

func TestShardIDFromHostname(t *testing.T) {
	testCases := []struct {
		hostname string
		want     int
		wantErr  bool
	}{
		{"event-exporter-0", 0, false},
		{"event-exporter-12", 12, false},
		{"event-exporter-v0.4.1-5", 5, false},
		{"event-exporter", 0, true},
		{"", 0, true},
	}
	for _, tc := range testCases {
		got, err := shardIDFromHostname(tc.hostname)
		if gotErr := err != nil; gotErr != tc.wantErr {
			t.Errorf("shardIDFromHostname(%q) error = %v, wantErr %v", tc.hostname, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("shardIDFromHostname(%q) = %d, want %d", tc.hostname, got, tc.want)
		}
	}
}
