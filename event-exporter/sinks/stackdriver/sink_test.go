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

package stackdriver

import (
	"testing"
	"time"

	sd "google.golang.org/api/logging/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/wait"
)

type fakeSdWriter struct {
	writeFunc func([]*sd.LogEntry, string, *sd.MonitoredResource) int
}

func (w *fakeSdWriter) Write(entries []*sd.LogEntry, logName string, resource *sd.MonitoredResource) int {
	if w.writeFunc != nil {
		return w.writeFunc(entries, logName, resource)
	}
	return 0
}

const (
	defaultTestFlushDelay     = 10 * time.Millisecond
	defaultTestMaxConcurrency = 10
	defaultTestMaxBufferSize  = 10

	bufferSizeParamName = "buffersize"
	flushDelayParamName = "flushdelay"
	blockingParamName   = "blocking"
)

func TestMaxConcurrency(t *testing.T) {
	c, s, q, done := createSinkWith(map[string]interface{}{
		blockingParamName: true,
	})
	defer close(done)

	for i := 0; i < c.MaxConcurrency*(c.MaxBufferSize+2); i++ {
		s.OnAdd(&corev1.Event{})
	}

	want := c.MaxConcurrency
	got := waitWritesCount(q, want)
	if got != want {
		t.Fatalf("Write called %d times, want %d", got, want)
	}
}

func TestBatchTimeout(t *testing.T) {
	_, s, q, done := createSink()
	defer close(done)

	s.OnAdd(&corev1.Event{})

	want := 1
	got := waitWritesCount(q, want)
	if got != want {
		t.Fatalf("Write called %d times, want %d", got, want)
	}
}

func TestBatchSizeLimit(t *testing.T) {
	_, s, q, done := createSinkWith(map[string]interface{}{
		flushDelayParamName: 1 * time.Hour,
	})
	defer close(done)

	for i := 0; i < 15; i++ {
		s.OnAdd(&corev1.Event{})
	}

	want := 1
	got := waitWritesCount(q, want)
	if got != want {
		t.Fatalf("Write called %d times, want %d", got, want)
	}
}

func TestInitialList(t *testing.T) {
	_, s, q, done := createSinkWith(map[string]interface{}{
		bufferSizeParamName: 1,
	})
	defer close(done)

	s.OnList(&corev1.EventList{})

	want := 1
	got := waitWritesCount(q, want)
	if got != want {
		t.Fatalf("Write called %d times, want %d", got, want)
	}

	s.OnList(&corev1.EventList{})

	got = waitWritesCount(q, want)
	if got != want {
		t.Fatalf("Write called %d times, want %d", got, want)
	}
}

func TestOnUpdate(t *testing.T) {
	tcs := []struct {
		desc      string
		old       *corev1.Event
		new       *corev1.Event
		wantEntry bool
	}{
		{
			"old=nil,new=event",
			nil,
			&corev1.Event{},
			true,
		},
		{
			"old=event,new=event",
			&corev1.Event{},
			&corev1.Event{},
			true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, s, q, done := createSink()
			defer close(done)

			s.OnUpdate(tc.old, tc.new)

			want := 1
			if !tc.wantEntry {
				want = 0
			}
			got := waitWritesCount(q, want)
			if got != want {
				t.Fatalf("Write called %d times, want %d", got, want)
			}
		})
	}
}

func createSink() (*sdSinkConfig, *sdSink, chan struct{}, chan struct{}) {
	return createSinkWith(make(map[string]interface{}))
}

func createSinkWith(params map[string]interface{}) (c *sdSinkConfig, s *sdSink, q chan struct{}, done chan struct{}) {
	done = make(chan struct{})
	c = createConfig(params)
	q = make(chan struct{}, 2*c.MaxConcurrency)
	w := &fakeSdWriter{
		writeFunc: func([]*sd.LogEntry, string, *sd.MonitoredResource) int {
			q <- struct{}{}
			if v, ok := params[blockingParamName]; ok && v.(bool) {
				<-done
			}
			return 0
		},
	}

	monitoredResourceFactory := &monitoredResourceFactory{}
	s = newSdSink(w, clock.NewFakeClock(time.Time{}), c, monitoredResourceFactory)
	go s.Run(done)
	return
}

func createConfig(params map[string]interface{}) *sdSinkConfig {
	c := &sdSinkConfig{
		FlushDelay:     defaultTestFlushDelay,
		LogName:        "logname",
		MaxConcurrency: defaultTestMaxConcurrency,
		MaxBufferSize:  defaultTestMaxBufferSize,
	}

	if v, ok := params[bufferSizeParamName]; ok {
		c.MaxBufferSize = v.(int)
	}

	if v, ok := params[flushDelayParamName]; ok {
		c.FlushDelay = v.(time.Duration)
	}

	return c
}

func waitWritesCount(q chan struct{}, want int) int {
	wait.Poll(10*time.Millisecond, 100*time.Millisecond, func() (bool, error) {
		return len(q) == want, nil
	})
	// Wait for some more time to ensure that the number is not greater.
	time.Sleep(100 * time.Millisecond)
	return len(q)
}
