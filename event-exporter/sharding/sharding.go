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
	"hash/fnv"
	"os"
	"regexp"
	"strconv"

	"k8s.io/apimachinery/pkg/types"
)

// Sharder deterministically assigns objects to one of totalShards shards by
// hashing their UID. All replicas must run with the same totalShards so that
// every object is owned by exactly one replica.
type Sharder struct {
	shardID     uint32
	totalShards uint32
}

// New creates a Sharder for the shard shardID out of totalShards.
func New(shardID, totalShards int) (*Sharder, error) {
	if totalShards < 1 {
		return nil, fmt.Errorf("total shards must be at least 1, got %d", totalShards)
	}
	if shardID < 0 || shardID >= totalShards {
		return nil, fmt.Errorf("shard ID must be in [0, %d), got %d", totalShards, shardID)
	}
	return &Sharder{
		shardID:     uint32(shardID),
		totalShards: uint32(totalShards),
	}, nil
}

// NewFromFlags creates a Sharder from the command line flag values. A shardID
// of -1 derives the shard ID from the ordinal suffix of the pod hostname,
// which works out of the box for StatefulSet replicas.
func NewFromFlags(shardID, totalShards int) (*Sharder, error) {
	if shardID == -1 {
		if totalShards == 1 {
			shardID = 0
		} else {
			hostname, err := os.Hostname()
			if err != nil {
				return nil, fmt.Errorf("failed to get hostname to derive shard ID: %v", err)
			}
			shardID, err = shardIDFromHostname(hostname)
			if err != nil {
				return nil, err
			}
		}
	}
	return New(shardID, totalShards)
}

// Enabled reports whether sharding is active, i.e. there is more than one
// shard. A nil Sharder behaves as a single shard that owns everything.
func (s *Sharder) Enabled() bool {
	return s != nil && s.totalShards > 1
}

// ShardID returns the ID of this shard.
func (s *Sharder) ShardID() int {
	return int(s.shardID)
}

// TotalShards returns the total number of shards.
func (s *Sharder) TotalShards() int {
	return int(s.totalShards)
}

// Owns reports whether the object with the given UID belongs to this shard.
// An empty UID deterministically maps to one fixed shard, so replicas never
// disagree on ownership.
func (s *Sharder) Owns(uid types.UID) bool {
	if !s.Enabled() {
		return true
	}
	h := fnv.New32a()
	h.Write([]byte(uid))
	return h.Sum32()%s.totalShards == s.shardID
}

var hostnameOrdinalMatcher = regexp.MustCompile(`-([0-9]+)$`)

// shardIDFromHostname extracts the shard ID from a StatefulSet pod hostname
// of the form <statefulset-name>-<ordinal>.
func shardIDFromHostname(hostname string) (int, error) {
	m := hostnameOrdinalMatcher.FindStringSubmatch(hostname)
	if m == nil {
		return 0, fmt.Errorf("hostname %q has no ordinal suffix to derive shard ID from; set shard-id explicitly", hostname)
	}
	return strconv.Atoi(m[1])
}
