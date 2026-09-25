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

package gcm

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func md(t string) *metric.MetricDescriptor {
	return &metric.MetricDescriptor{Type: t}
}

func TestBuildFilter(t *testing.T) {
	cases := []struct {
		name   string
		typ    string
		extras []string
		want   string
	}{
		{"type only", "kubernetes.io/container/restart_count", nil,
			`metric.type = "kubernetes.io/container/restart_count"`},
		{"with extras", "a/b", []string{`resource.labels.cluster_name = "c"`, ` `, `metric.labels.x = "y"`},
			`metric.type = "a/b" AND (resource.labels.cluster_name = "c") AND (metric.labels.x = "y")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildFilter(tc.typ, tc.extras); got != tc.want {
				t.Errorf("BuildFilter = %s\nwant        %s", got, tc.want)
			}
		})
	}
}

func TestDescriptorCache(t *testing.T) {
	fake := &Fake{Descriptors: []*metric.MetricDescriptor{
		md("kubernetes.io/node/cpu/core_usage_time"),
		md("kubernetes.io/node/memory/used_bytes"),
		md("compute.googleapis.com/instance/cpu/utilization"),
	}}
	dc := NewDescriptorCache(fake)

	if _, ok := dc.Get("kubernetes.io/node/cpu/core_usage_time"); ok {
		t.Fatal("Get succeeded before Refresh")
	}
	if err := dc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, ok := dc.Get("kubernetes.io/node/cpu/core_usage_time"); !ok {
		t.Error("Get(exact) = not found after Refresh")
	}
	if got := dc.ExpandPrefix("kubernetes.io/node/"); len(got) != 2 {
		t.Errorf("ExpandPrefix = %v, want 2 entries", got)
	}
	if got := dc.All(); len(got) != 3 || got[0].GetType() != "compute.googleapis.com/instance/cpu/utilization" {
		t.Errorf("All() not sorted or wrong length: %v", got)
	}

	// Failed refresh keeps the previous cache.
	fake.SetErr(errors.New("boom"))
	if err := dc.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh succeeded despite injected error")
	}
	if dc.Len() != 3 {
		t.Errorf("cache lost data on failed refresh: len = %d", dc.Len())
	}
}

func TestFakeRecordsAndFilters(t *testing.T) {
	fake := &Fake{Descriptors: []*metric.MetricDescriptor{
		md("kubernetes.io/node/cpu/core_usage_time"),
		md("compute.googleapis.com/instance/cpu/utilization"),
	}}
	got, err := fake.ListDescriptors(context.Background(), `metric.type = starts_with("kubernetes.io/")`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GetType() != "kubernetes.io/node/cpu/core_usage_time" {
		t.Errorf("starts_with filtering broken: %v", got)
	}

	q := SeriesQuery{MetricType: "x", Start: time.Unix(0, 0), End: time.Unix(60, 0)}
	if _, err := fake.ListSeries(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if fake.NumSeriesCalls() != 1 || fake.SeriesCalls[0].MetricType != "x" {
		t.Errorf("call recording broken: %+v", fake.SeriesCalls)
	}
}

func TestIsQuotaErr(t *testing.T) {
	if !IsQuotaErr(status.Error(codes.ResourceExhausted, "quota")) {
		t.Error("ResourceExhausted not classified as quota error")
	}
	if IsQuotaErr(status.Error(codes.Unavailable, "down")) {
		t.Error("Unavailable wrongly classified as quota error")
	}
	if IsQuotaErr(nil) {
		t.Error("nil classified as quota error")
	}
}
