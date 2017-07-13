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

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/wait"
	api_v1 "k8s.io/client-go/pkg/api/v1"
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

func TestMaxConcurrency(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	config := &sdSinkConfig{
		Resource:       nil,
		FlushDelay:     100 * time.Millisecond,
		LogName:        "logname",
		MaxConcurrency: 10,
		MaxBufferSize:  10,
	}
	q := make(chan struct{}, config.MaxConcurrency+1)
	w := &fakeSdWriter{
		writeFunc: func([]*sd.LogEntry, string, *sd.MonitoredResource) int {
			q <- struct{}{}
			<-done
			return 0
		},
	}
	s := newSdSink(w, clock.NewFakeClock(time.Time{}), config)
	go s.Run(done)

	for i := 0; i < config.MaxConcurrency*(config.MaxBufferSize+2); i++ {
		s.OnAdd(&api_v1.Event{})
	}

	wait.Poll(100*time.Millisecond, 1*time.Second, func() (bool, error) {
		return len(q) == config.MaxConcurrency, nil
	})
	if len(q) != config.MaxConcurrency {
		t.Fatalf("Write called %d times, expected %d", len(q), config.MaxConcurrency)
	}
}

func TestBatchTimeout(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	config := &sdSinkConfig{
		Resource:       nil,
		FlushDelay:     100 * time.Millisecond,
		LogName:        "logname",
		MaxConcurrency: 10,
		MaxBufferSize:  10,
	}
	q := make(chan struct{}, config.MaxConcurrency+1)
	w := &fakeSdWriter{
		writeFunc: func([]*sd.LogEntry, string, *sd.MonitoredResource) int {
			q <- struct{}{}
			return 0
		},
	}
	s := newSdSink(w, clock.NewFakeClock(time.Time{}), config)
	go s.Run(done)

	s.OnAdd(&api_v1.Event{})
	wait.Poll(100*time.Millisecond, 1*time.Second, func() (bool, error) {
		return len(q) == 1, nil
	})

	if len(q) != 1 {
		t.Fatalf("Write called %d times, expected 1", len(q))
	}
}

func TestBatchSizeLimit(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	config := &sdSinkConfig{
		Resource:       nil,
		FlushDelay:     1 * time.Minute,
		LogName:        "logname",
		MaxConcurrency: 10,
		MaxBufferSize:  10,
	}
	q := make(chan struct{}, config.MaxConcurrency+1)
	w := &fakeSdWriter{
		writeFunc: func([]*sd.LogEntry, string, *sd.MonitoredResource) int {
			q <- struct{}{}
			return 0
		},
	}
	s := newSdSink(w, clock.NewFakeClock(time.Time{}), config)
	go s.Run(done)

	for i := 0; i < 15; i++ {
		s.OnAdd(&api_v1.Event{})
	}

	wait.Poll(100*time.Millisecond, 1*time.Second, func() (bool, error) {
		return len(q) == 1, nil
	})

	if len(q) != 1 {
		t.Fatalf("Write called %d times, expected 1", len(q))
	}
}

func TestInitialList(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	config := &sdSinkConfig{
		Resource:       nil,
		FlushDelay:     100 * time.Millisecond,
		LogName:        "logname",
		MaxConcurrency: 10,
		MaxBufferSize:  1,
	}
	q := make(chan struct{}, config.MaxConcurrency+1)
	w := &fakeSdWriter{
		writeFunc: func([]*sd.LogEntry, string, *sd.MonitoredResource) int {
			q <- struct{}{}
			return 0
		},
	}
	s := newSdSink(w, clock.NewFakeClock(time.Time{}), config)
	go s.Run(done)

	s.OnList(&api_v1.EventList{})

	wait.Poll(100*time.Millisecond, 1*time.Second, func() (bool, error) {
		return len(q) == 1, nil
	})
	if len(q) != 1 {
		t.Fatalf("Write called %d times, expected 1", len(q))
	}

	s.OnList(&api_v1.EventList{})

	time.Sleep(2 * config.FlushDelay)
	if len(q) != 1 {
		t.Fatalf("Write called %d times, expected 1", len(q))
	}
}
