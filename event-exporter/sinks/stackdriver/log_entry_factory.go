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
	go_json "encoding/json"
	"time"

	"github.com/golang/glog"
	sd "google.golang.org/api/logging/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	// fieldBlacklist is a list of fields that should be excluded from the
	// json object sent to Stackdriver.
	fieldBlacklist = []string{
		// Is unnecessary, because it's demuxed already
		"count",
		// Timestamp is in the logEntry's metadata
		"lastTimestamp",
		// Not relevant because of demuxing
		"firstTimestamp",
	}
)

type sdLogEntryFactory struct {
	clock           clock.Clock
	encoder         runtime.Encoder
	resourceFactory *monitoredResourceFactory
}

func newSdLogEntryFactory(clock clock.Clock, resourceFactory *monitoredResourceFactory) *sdLogEntryFactory {
	return &sdLogEntryFactory{
		clock:           clock,
		encoder:         newEncoder(),
		resourceFactory: resourceFactory,
	}
}

func (f *sdLogEntryFactory) FromEvent(event *corev1.Event) *sd.LogEntry {
	payload, err := f.serializeEvent(event)
	if err != nil {
		glog.Warningf("Failed to encode event %+v: %v", event, err)
	}

	resource := f.resourceFactory.resourceFromEvent(event)

	return &sd.LogEntry{
		JsonPayload: payload,
		Severity:    f.detectSeverity(event),
		Timestamp:   event.LastTimestamp.Format(time.RFC3339Nano),
		Resource:    resource,
	}
}

func (f *sdLogEntryFactory) FromMessage(msg string) *sd.LogEntry {
	return &sd.LogEntry{
		TextPayload: msg,
		Severity:    "WARNING",
		Timestamp:   f.clock.Now().Format(time.RFC3339Nano),
	}
}

func (f *sdLogEntryFactory) detectSeverity(event *corev1.Event) string {
	if event.Type == "Warning" {
		return "WARNING"
	}
	return "INFO"
}

func (f *sdLogEntryFactory) serializeEvent(event *corev1.Event) ([]byte, error) {
	bytes, err := runtime.Encode(f.encoder, event)
	if err != nil {
		return nil, err
	}

	var obj map[string]interface{}
	err = go_json.Unmarshal(bytes, &obj)
	if err != nil {
		return nil, err
	}

	for _, field := range fieldBlacklist {
		delete(obj, field)
	}

	return go_json.Marshal(obj)
}

func newEncoder() runtime.Encoder {
	jsonSerializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{})
	return scheme.Codecs.EncoderForVersion(jsonSerializer, corev1.SchemeGroupVersion)
}
