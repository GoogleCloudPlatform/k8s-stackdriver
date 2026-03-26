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
	"sort"
	"sync/atomic"
	"time"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// PeerDiscovery watches the Endpoints of a headless Service to discover
// peer pods and determine event ownership via consistent hashing.
type PeerDiscovery struct {
	client      kubernetes.Interface
	serviceName string
	namespace   string
	podIP       string

	// peers stores a *peerState atomically for lock-free reads in the hot path.
	peers atomic.Value // *peerState

	informerFactory informers.SharedInformerFactory
}

type peerState struct {
	sortedIPs []string
	myIndex   int // -1 if not found
}

// New creates a new PeerDiscovery instance.
// podIP is this pod's IP (from Downward API POD_IP env var).
// serviceName is the headless Service name.
// namespace is the namespace of the headless Service and this pod.
func New(client kubernetes.Interface, serviceName, namespace, podIP string) *PeerDiscovery {
	pd := &PeerDiscovery{
		client:      client,
		serviceName: serviceName,
		namespace:   namespace,
		podIP:       podIP,
	}
	// Initial state: no peers known, pod not found — grace period active.
	pd.peers.Store(&peerState{sortedIPs: nil, myIndex: -1})
	return pd
}

// IsOwner returns true if this pod should process the event with the given key.
// The key should be "namespace/name" of the event.
// During startup grace period (pod not yet in Endpoints), returns true for all
// events to prevent gaps.
func (pd *PeerDiscovery) IsOwner(eventKey string) bool {
	state := pd.peers.Load().(*peerState)

	// Grace period: if we have no peers or we're not in the peer list yet,
	// process all events to prevent gaps.
	if len(state.sortedIPs) == 0 || state.myIndex == -1 {
		return true
	}

	h := xxhash.Sum64String(eventKey)
	owner := int(h % uint64(len(state.sortedIPs)))
	return owner == state.myIndex
}

// PeerCount returns the current number of known peers.
func (pd *PeerDiscovery) PeerCount() int {
	state := pd.peers.Load().(*peerState)
	return len(state.sortedIPs)
}

// Ready returns true when the pod has discovered itself in the Endpoints.
func (pd *PeerDiscovery) Ready() bool {
	state := pd.peers.Load().(*peerState)
	return len(state.sortedIPs) > 0 && state.myIndex >= 0
}

// Run starts the Endpoints informer and blocks until stopCh is closed.
func (pd *PeerDiscovery) Run(stopCh <-chan struct{}) {
	pd.informerFactory = informers.NewSharedInformerFactoryWithOptions(
		pd.client,
		0, // no resync — watch-based updates only
		informers.WithNamespace(pd.namespace),
	)

	endpointsInformer := pd.informerFactory.Core().V1().Endpoints().Informer()
	endpointsInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			ep, ok := obj.(*corev1.Endpoints)
			if !ok {
				return false
			}
			return ep.Name == pd.serviceName
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { pd.onEndpointsUpdate(obj) },
			UpdateFunc: func(_, obj interface{}) { pd.onEndpointsUpdate(obj) },
			DeleteFunc: func(_ interface{}) { pd.onEndpointsDelete() },
		},
	})

	pd.informerFactory.Start(stopCh)

	// Wait for cache sync with timeout.
	if !cache.WaitForCacheSync(stopCh, endpointsInformer.HasSynced) {
		glog.Errorf("Failed to sync Endpoints informer cache")
		return
	}

	glog.Infof("PeerDiscovery started: watching Endpoints %s/%s, my IP: %s",
		pd.namespace, pd.serviceName, pd.podIP)

	// Block until stop.
	<-stopCh
	glog.Info("PeerDiscovery stopped")
}

// WaitForSync blocks until the Endpoints informer has synced at least once,
// or the timeout expires. Returns true if synced.
func (pd *PeerDiscovery) WaitForSync(timeout time.Duration) bool {
	deadline := time.After(timeout)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			return false
		case <-tick.C:
			if pd.Ready() {
				return true
			}
		}
	}
}

func (pd *PeerDiscovery) onEndpointsUpdate(obj interface{}) {
	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}

	ips := extractIPs(ep)
	sort.Strings(ips)

	myIndex := -1
	for i, ip := range ips {
		if ip == pd.podIP {
			myIndex = i
			break
		}
	}

	pd.peers.Store(&peerState{
		sortedIPs: ips,
		myIndex:   myIndex,
	})

	peerCount.Set(float64(len(ips)))
	peerUpdatesTotal.Inc()

	glog.V(2).Infof("PeerDiscovery updated: %d peers, myIndex=%d, peers=%v", len(ips), myIndex, ips)
}

func (pd *PeerDiscovery) onEndpointsDelete() {
	pd.peers.Store(&peerState{sortedIPs: nil, myIndex: -1})
	peerCount.Set(0)
	peerUpdatesTotal.Inc()
	glog.Warning("PeerDiscovery: Endpoints deleted, entering grace period (processing all events)")
}

// extractIPs returns all ready endpoint IPs from an Endpoints object.
func extractIPs(ep *corev1.Endpoints) []string {
	var ips []string
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			if addr.IP != "" {
				ips = append(ips, addr.IP)
			}
		}
	}
	return ips
}
