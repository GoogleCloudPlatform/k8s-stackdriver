//go:build live

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

// Live smoke tests against a real project. Run from a machine with ADC:
//
//	GCM_PROXY_TEST_PROJECT=<your-project> go test -tags live ./internal/gcm -run TestLive -v
package gcm

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveDescriptors(t *testing.T) {
	project := os.Getenv("GCM_PROXY_TEST_PROJECT")
	if project == "" {
		t.Skip("GCM_PROXY_TEST_PROJECT not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := New(ctx, project, 4, Hooks{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Close() }()

	descs, err := c.ListDescriptors(ctx, `metric.type = starts_with("kubernetes.io/node/")`)
	if err != nil {
		t.Fatalf("ListDescriptors: %v", err)
	}
	if len(descs) == 0 {
		t.Fatal("no kubernetes.io/node/ descriptors found")
	}
	withMeta := 0
	for _, d := range descs {
		if d.GetMetadata().GetSamplePeriod().AsDuration() > 0 {
			withMeta++
		}
	}
	t.Logf("descriptors: %d (with samplePeriod metadata: %d); first: %s kind=%s value=%s sample=%s delay=%s",
		len(descs), withMeta,
		descs[0].GetType(), descs[0].GetMetricKind(), descs[0].GetValueType(),
		descs[0].GetMetadata().GetSamplePeriod().AsDuration(),
		descs[0].GetMetadata().GetIngestDelay().AsDuration())
	if withMeta == 0 {
		t.Error("no descriptor carried samplePeriod metadata — auto-lookback would be inert")
	}

	// DescriptorCache full refresh against the live project.
	dc := NewDescriptorCache(c)
	if err := dc.Refresh(ctx); err != nil {
		t.Fatalf("cache Refresh: %v", err)
	}
	t.Logf("full descriptor cache: %d entries", dc.Len())
	if dc.Len() < 100 {
		t.Errorf("suspiciously few descriptors in project: %d", dc.Len())
	}
}
