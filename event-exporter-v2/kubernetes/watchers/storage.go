/*
Copyright 2017 Google Inc.

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

package watchers

import (
	"time"

	"k8s.io/client-go/tools/cache"
)

// StorageType defines what storage should be used as a cache for the watcher.
type StorageType int

const (
	// SimpleStorage storage type indicates thread-safe map as backing storage.
	SimpleStorage StorageType = iota
	// TTLStorage storage type indicates storage with expiration. When this
	// type of storage is used, TTL should be specified.
	TTLStorage
	// DeltaFIFOStorage storage type indicates a producer-consumer queue that
	// stores a slice of event deltas (Deltas)** for each object key,
	// typically used for the Reflector to produce and a consumer to Pop.
	DeltaFIFOStorage
)

// WatcherStoreConfig represents the configuration of the storage backing the watcher.
type WatcherStoreConfig struct {
	KeyFunc     cache.KeyFunc
	Handler     cache.ResourceEventHandler
	StorageType StorageType
	StorageTTL  time.Duration
}

type watcherStore struct {
	cache.ReflectorStore

	handler cache.ResourceEventHandler
}

func (s *watcherStore) Add(obj interface{}) error {
	s.handler.OnAdd(obj, false)
	return nil
}

func (s *watcherStore) Update(obj interface{}) error {
	s.handler.OnUpdate(nil, obj)
	return nil
}

func (s *watcherStore) Delete(obj interface{}) error {
	s.handler.OnDelete(obj)
	return nil
}

func newWatcherStore(config *WatcherStoreConfig) *watcherStore {
	var cacheStorage cache.ReflectorStore
	switch config.StorageType {
	case TTLStorage:
		cacheStorage = cache.NewTTLStore(config.KeyFunc, config.StorageTTL)
	case DeltaFIFOStorage:
		cacheStorage = cache.NewDeltaFIFOWithOptions(cache.DeltaFIFOOptions{KeyFunction: config.KeyFunc})
	case SimpleStorage:
	default:
		cacheStorage = cache.NewStore(config.KeyFunc)
	}

	return &watcherStore{
		ReflectorStore: cacheStorage,
		handler:        config.Handler,
	}
}
