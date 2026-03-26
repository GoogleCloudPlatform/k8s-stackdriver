/*
Copyright 2024 Google Inc.

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

package peerdiscovery

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractIPs(t *testing.T) {
	ep := &corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.3"},
					{IP: "10.0.0.1"},
				},
			},
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.2"},
				},
			},
		},
	}
	ips := extractIPs(ep)
	if len(ips) != 3 {
		t.Fatalf("expected 3 IPs, got %d", len(ips))
	}
}

func TestExtractIPsEmpty(t *testing.T) {
	ep := &corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{},
	}
	ips := extractIPs(ep)
	if len(ips) != 0 {
		t.Fatalf("expected 0 IPs, got %d", len(ips))
	}
}

func TestIsOwnerGracePeriod(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.1")
	// No peers known — should return true for all events (grace period).
	if !pd.IsOwner("default/my-event") {
		t.Error("expected IsOwner to return true during grace period (no peers)")
	}
}

func TestIsOwnerNotInPeerList(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.99")
	// Set peers but our IP is not in the list.
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		myIndex:   -1,
	})
	// Grace period: pod not in peer list — should process all events.
	if !pd.IsOwner("default/my-event") {
		t.Error("expected IsOwner to return true when pod not in peer list")
	}
}

func TestIsOwnerDeterministic(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.2")
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		myIndex:   1,
	})

	key := "kube-system/some-event.abc123"
	result1 := pd.IsOwner(key)
	result2 := pd.IsOwner(key)
	if result1 != result2 {
		t.Error("IsOwner should be deterministic for the same key")
	}
}

func TestIsOwnerDistribution(t *testing.T) {
	pds := make([]*PeerDiscovery, 3)
	ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	for i, ip := range ips {
		pds[i] = New(nil, "svc", "ns", ip)
		pds[i].peers.Store(&peerState{
			sortedIPs: ips,
			myIndex:   i,
		})
	}

	// Generate many event keys and verify each is owned by exactly one pod.
	for i := 0; i < 1000; i++ {
		key := "default/event-" + string(rune('a'+i%26)) + string(rune('0'+i%10))
		ownerCount := 0
		for _, pd := range pds {
			if pd.IsOwner(key) {
				ownerCount++
			}
		}
		if ownerCount != 1 {
			t.Fatalf("event key %q owned by %d pods, expected exactly 1", key, ownerCount)
		}
	}
}

func TestIsOwnerSinglePod(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.1")
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1"},
		myIndex:   0,
	})
	// Single pod should own all events.
	for i := 0; i < 100; i++ {
		key := "default/event-" + string(rune('a'+i%26))
		if !pd.IsOwner(key) {
			t.Fatalf("single pod should own all events, but didn't own %q", key)
		}
	}
}

func TestOnEndpointsUpdate(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.2")
	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.3"},
					{IP: "10.0.0.1"},
					{IP: "10.0.0.2"},
				},
			},
		},
	}
	pd.onEndpointsUpdate(ep)

	state := pd.peers.Load().(*peerState)
	if len(state.sortedIPs) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(state.sortedIPs))
	}
	// Verify sorted order.
	for i := 1; i < len(state.sortedIPs); i++ {
		if state.sortedIPs[i-1] >= state.sortedIPs[i] {
			t.Fatalf("peer list not sorted: %v", state.sortedIPs)
		}
	}
	if state.myIndex != 1 { // 10.0.0.2 should be at index 1 after sorting
		t.Fatalf("expected myIndex=1, got %d", state.myIndex)
	}
}

func TestOnEndpointsDelete(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.1")
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1", "10.0.0.2"},
		myIndex:   0,
	})
	pd.onEndpointsDelete()
	state := pd.peers.Load().(*peerState)
	if len(state.sortedIPs) != 0 {
		t.Fatalf("expected 0 peers after delete, got %d", len(state.sortedIPs))
	}
	if state.myIndex != -1 {
		t.Fatalf("expected myIndex=-1 after delete, got %d", state.myIndex)
	}
}

func TestPeerCount(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.1")
	if pd.PeerCount() != 0 {
		t.Fatalf("expected 0 peers initially, got %d", pd.PeerCount())
	}
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1", "10.0.0.2"},
		myIndex:   0,
	})
	if pd.PeerCount() != 2 {
		t.Fatalf("expected 2 peers, got %d", pd.PeerCount())
	}
}

func TestReady(t *testing.T) {
	pd := New(nil, "svc", "ns", "10.0.0.1")
	if pd.Ready() {
		t.Error("expected not ready initially")
	}
	pd.peers.Store(&peerState{
		sortedIPs: []string{"10.0.0.1", "10.0.0.2"},
		myIndex:   0,
	})
	if !pd.Ready() {
		t.Error("expected ready after peer list set with valid index")
	}
}

func TestRebalanceCoverage(t *testing.T) {
	// Simulate scale from 2 to 3 pods.
	// Verify that every event is owned by at least one pod in both configurations.
	ips2 := []string{"10.0.0.1", "10.0.0.2"}
	ips3 := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}

	keys := make([]string, 500)
	for i := range keys {
		keys[i] = "default/event-" + string(rune(i))
	}

	// With 2 pods, every key must be owned by exactly one.
	for _, key := range keys {
		pd0 := New(nil, "svc", "ns", "10.0.0.1")
		pd0.peers.Store(&peerState{sortedIPs: ips2, myIndex: 0})
		pd1 := New(nil, "svc", "ns", "10.0.0.2")
		pd1.peers.Store(&peerState{sortedIPs: ips2, myIndex: 1})

		count := 0
		if pd0.IsOwner(key) {
			count++
		}
		if pd1.IsOwner(key) {
			count++
		}
		if count != 1 {
			t.Fatalf("key %q owned by %d pods with 2 peers", key, count)
		}
	}

	// With 3 pods, every key must be owned by exactly one.
	for _, key := range keys {
		pds := make([]*PeerDiscovery, 3)
		for i, ip := range ips3 {
			pds[i] = New(nil, "svc", "ns", ip)
			pds[i].peers.Store(&peerState{sortedIPs: ips3, myIndex: i})
		}
		count := 0
		for _, pd := range pds {
			if pd.IsOwner(key) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("key %q owned by %d pods with 3 peers", key, count)
		}
	}
}
