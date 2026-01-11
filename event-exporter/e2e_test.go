//go:build e2e
// +build e2e

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

package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/watchers"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter/kubernetes/watchers/events"
)

const (
	e2eLabelKey        = "event-exporter-e2e"
	coreLabelValue     = "core-v1"
	eventsV1LabelValue = "events-v1"
	defaultEventCount  = 200
	defaultSeriesStep  = 10
	defaultE2ETimeout  = 2 * time.Minute
	defaultNamespace   = "event-exporter-e2e"
)

type recordingSink struct {
	addCount    atomic.Int64
	updateCount atomic.Int64
	deleteCount atomic.Int64

	listOnce sync.Once
	listCh   chan struct{}

	mu      sync.Mutex
	samples []*corev1.Event
}

func newRecordingSink() *recordingSink {
	return &recordingSink{
		listCh: make(chan struct{}),
	}
}

func (s *recordingSink) OnList(*corev1.EventList) {
	s.listOnce.Do(func() { close(s.listCh) })
}

func (s *recordingSink) OnAdd(event *corev1.Event) {
	s.addCount.Add(1)
	s.recordSample(event)
}

func (s *recordingSink) OnUpdate(_, newEvent *corev1.Event) {
	s.updateCount.Add(1)
	s.recordSample(newEvent)
}

func (s *recordingSink) OnDelete(event *corev1.Event) {
	s.deleteCount.Add(1)
}

func (s *recordingSink) Run(stopCh <-chan struct{}) {
	<-stopCh
}

func (s *recordingSink) recordSample(event *corev1.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) < 3 && event != nil {
		s.samples = append(s.samples, event)
	}
}

func (s *recordingSink) totalProcessed() int64 {
	return s.addCount.Load() + s.updateCount.Load()
}

func TestE2EEventsExport(t *testing.T) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set; skipping e2e")
	}

	eventCount := envInt("E2E_EVENT_COUNT", defaultEventCount)
	seriesStep := envInt("E2E_SERIES_STEP", defaultSeriesStep)
	timeout := envDuration("E2E_TIMEOUT", defaultE2ETimeout)
	namespace := envString("E2E_NAMESPACE", defaultNamespace)

	ctx := context.Background()
	client, err := newClient(kubeconfig)
	if err != nil {
		t.Fatalf("failed to build kube client: %v", err)
	}

	if err := ensureNamespace(ctx, client, namespace); err != nil {
		t.Fatalf("failed to ensure namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	})

	runCase := func(t *testing.T, apiVersion events.EventsAPIVersion, labelValue string, generator func(context.Context, kubernetes.Interface, string, int, int) (int, int, error)) {
		t.Helper()
		sink := newRecordingSink()
		stopCh := make(chan struct{})
		defer close(stopCh)

		selector := labels.SelectorFromSet(labels.Set{e2eLabelKey: labelValue})
		exporter := newEventExporter(client, sink, 0, selector, 0, watchers.DeltaFIFOStorage, apiVersion)
		go exporter.Run(stopCh)

		waitForList(t, sink.listCh, timeout)

		start := time.Now()
		expected, occurrences, err := generator(ctx, client, namespace, eventCount, seriesStep)
		if err != nil {
			t.Fatalf("failed to generate events: %v", err)
		}

		waitForCount(t, sink, int64(expected), timeout)
		elapsed := time.Since(start)
		throughput := float64(occurrences) / elapsed.Seconds()

		verifySamples(t, sink.samples, labelValue)
		t.Logf("%s: processed=%d updates=%d occurrences=%d elapsed=%s throughput=%.2f occ/s", apiVersion, sink.totalProcessed(), sink.updateCount.Load(), occurrences, elapsed, throughput)
	}

	t.Run("core-v1", func(t *testing.T) {
		runCase(t, events.CoreV1EventsAPIVersion, coreLabelValue, generateCoreV1Events)
	})

	t.Run("events-v1", func(t *testing.T) {
		runCase(t, events.EventsV1EventsAPIVersion, eventsV1LabelValue, generateEventsV1Series)
	})
}

func verifySamples(t *testing.T, samples []*corev1.Event, labelValue string) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("no events observed for label %s", labelValue)
	}
	for _, event := range samples {
		if event == nil {
			t.Fatalf("nil event observed")
		}
		if event.InvolvedObject.Name == "" {
			t.Fatalf("missing involved object name")
		}
		if event.Reason == "" {
			t.Fatalf("missing event reason")
		}
		if event.Message == "" {
			t.Fatalf("missing event message")
		}
	}
}

func waitForList(t *testing.T, listCh <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-listCh:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for initial list")
	}
}

func waitForCount(t *testing.T, sink *recordingSink, expected int64, timeout time.Duration) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		if sink.totalProcessed() >= expected {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for %d events, got %d", expected, sink.totalProcessed())
		}
	}
}

func newClient(kubeconfig string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	config.ContentType = "application/vnd.kubernetes.protobuf"
	return kubernetes.NewForConfig(config)
}

func ensureNamespace(ctx context.Context, client kubernetes.Interface, name string) error {
	_, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	return err
}

func generateCoreV1Events(ctx context.Context, client kubernetes.Interface, namespace string, count int, _ int) (int, int, error) {
	for i := 0; i < count; i++ {
		now := time.Now()
		event := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "e2e-core-",
				Namespace:    namespace,
				Labels:       map[string]string{e2eLabelKey: coreLabelValue},
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "e2e-pod",
				Namespace: namespace,
			},
			Reason:         "E2ECore",
			Message:        "core/v1 event",
			Type:           corev1.EventTypeNormal,
			FirstTimestamp: metav1.NewTime(now),
			LastTimestamp:  metav1.NewTime(now),
		}
		if _, err := client.CoreV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
			return 0, 0, err
		}
	}
	return count, count, nil
}

func generateEventsV1Series(ctx context.Context, client kubernetes.Interface, namespace string, count int, seriesStep int) (int, int, error) {
	if seriesStep < 1 {
		seriesStep = 1
	}
	now := time.Now()
	event := &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("e2e-events-v1-%d", time.Now().UnixNano()),
			Namespace: namespace,
			Labels:    map[string]string{e2eLabelKey: eventsV1LabelValue},
		},
		EventTime:           metav1.NewMicroTime(now),
		ReportingController: "event-exporter-e2e",
		ReportingInstance:   "event-exporter-e2e",
		Action:              "E2E",
		Reason:              "E2EV1",
		Note:                "events.k8s.io/v1 event",
		Type:                corev1.EventTypeNormal,
		Regarding: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "e2e-pod",
			Namespace: namespace,
		},
	}

	created, err := client.EventsV1().Events(namespace).Create(ctx, event, metav1.CreateOptions{})
	if err != nil {
		return 0, 0, err
	}

	updates := 0
	for i := 2; i <= count; i++ {
		if i%seriesStep != 0 {
			continue
		}
		if created.Series == nil {
			created.Series = &eventsv1.EventSeries{}
		}
		created.Series.Count = int32(i)
		created.Series.LastObservedTime = metav1.NewMicroTime(time.Now())
		updated, updateErr := client.EventsV1().Events(namespace).Update(ctx, created, metav1.UpdateOptions{})
		if updateErr != nil {
			return 0, 0, updateErr
		}
		created = updated
		updates++
	}

	expected := 1 + updates
	return expected, count, nil
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envString(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
