package dynamicmapper

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
)

// RegeneratingDiscoveryRESTMapper is a RESTMapper which Regenerates its cache of mappings periodically.
// It functions by recreating a normal discovery RESTMapper at the specified interval.
// We don't refresh automatically on cache misses, since we get called on every label, plenty of which will
// be unrelated to Kubernetes resources.
type RegeneratingDiscoveryRESTMapper struct {
	discoveryClient   discovery.DiscoveryInterface
	versionInterfaces meta.VersionInterfacesFunc

	refreshInterval time.Duration

	mu sync.RWMutex

	delegate meta.RESTMapper
}

// NewRESTMapper creates RegeneratingDiscoveryRESTMapper.
func NewRESTMapper(discoveryClient discovery.DiscoveryInterface, versionInterfaces meta.VersionInterfacesFunc, refreshInterval time.Duration) (*RegeneratingDiscoveryRESTMapper, error) {
	mapper := &RegeneratingDiscoveryRESTMapper{
		discoveryClient:   discoveryClient,
		versionInterfaces: versionInterfaces,
		refreshInterval:   refreshInterval,
	}
	if err := mapper.RegenerateMappings(); err != nil {
		return nil, fmt.Errorf("unable to populate initial set of REST mappings: %v", err)
	}

	return mapper, nil
}

// RunUntil runs the mapping refresher until the given stop channel is closed.
func (m *RegeneratingDiscoveryRESTMapper) RunUntil(stop <-chan struct{}) {
	go wait.Until(func() {
		if err := m.RegenerateMappings(); err != nil {
			glog.Errorf("error regenerating REST mappings from discovery: %v", err)
		}
	}, m.refreshInterval, stop)
}

// RegenerateMappings regenerates cached mappings.
func (m *RegeneratingDiscoveryRESTMapper) RegenerateMappings() error {
	resources, err := discovery.GetAPIGroupResources(m.discoveryClient)
	if err != nil {
		return err
	}
	newDelegate := discovery.NewRESTMapper(resources, m.versionInterfaces)

	// don't lock until we're ready to replace
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delegate = newDelegate

	return nil
}

// KindFor takes a partial resource and returns back the single match.
// It returns an error if there are multiple matches.
func (m *RegeneratingDiscoveryRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.KindFor(resource)
}

// KindsFor takes a partial resource and returns back the list of
// potential kinds in priority order.
func (m *RegeneratingDiscoveryRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.KindsFor(resource)

}

// ResourceFor takes a partial resource and returns back the single
// match. It returns an error if there are multiple matches.
func (m *RegeneratingDiscoveryRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.ResourceFor(input)

}

// ResourcesFor takes a partial resource and returns back the list of
// potential resource in priority order.
func (m *RegeneratingDiscoveryRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.ResourcesFor(input)

}

// RESTMapping identifies a preferred resource mapping for the
// provided group kind.
func (m *RegeneratingDiscoveryRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.RESTMapping(gk, versions...)

}

// RESTMappings returns the RESTMappings for the provided group kind
// in a rough internal preferred order. If no kind is found, it will
// return a NoResourceMatchError.
func (m *RegeneratingDiscoveryRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.RESTMappings(gk, versions...)
}

// ResourceSingularizer converts a resource name from plural to
// singular (e.g., from pods to pod).
func (m *RegeneratingDiscoveryRESTMapper) ResourceSingularizer(resource string) (singular string, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.delegate.ResourceSingularizer(resource)
}
