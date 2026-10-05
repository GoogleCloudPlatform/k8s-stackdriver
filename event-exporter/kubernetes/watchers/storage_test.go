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

package watchers

import (
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestWatcherStoreReplaceDoesNotPopulateDeltaFIFO(t *testing.T) {
	store := newWatcherStore(&WatcherStoreConfig{
		KeyFunc:     cache.MetaNamespaceKeyFunc,
		Handler:     cache.ResourceEventHandlerFuncs{},
		StorageType: DeltaFIFOStorage,
	})

	err := store.Replace([]interface{}{
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "event",
			},
		},
	}, "1")
	if err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	queue, ok := store.ReflectorStore.(cache.Queue)
	if !ok {
		t.Fatal("DeltaFIFO store does not implement cache.Queue")
	}

	popped := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := queue.Pop(func(interface{}, bool) error {
			popped <- struct{}{}
			return nil
		})
		done <- err
	}()

	select {
	case <-popped:
		t.Fatal("Replace added an item to the queue")
	case <-time.After(50 * time.Millisecond):
	}

	queue.Close()
	if err := <-done; !errors.Is(err, cache.ErrFIFOClosed) {
		t.Fatalf("Pop returned error %v, want %v", err, cache.ErrFIFOClosed)
	}
}
