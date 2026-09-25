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

package scope

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/compute/metadata"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
)

// newFakeMetadata serves the three metadata paths the package uses and
// points the real metadata client at itself via GCE_METADATA_HOST.
func newFakeMetadata(t *testing.T, projectID, clusterName, clusterLocation string) {
	t.Helper()
	mux := http.NewServeMux()
	serve := func(path, val string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Metadata-Flavor") != "Google" {
				http.Error(w, "missing Metadata-Flavor", http.StatusForbidden)
				return
			}
			if val == "" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Metadata-Flavor", "Google")
			_, _ = w.Write([]byte(val))
		})
	}
	serve("/computeMetadata/v1/project/project-id", projectID)
	serve("/computeMetadata/v1/instance/attributes/cluster-name", clusterName)
	serve("/computeMetadata/v1/instance/attributes/cluster-location", clusterLocation)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(srv.URL, "http://"))
}

func TestDiscoverAllFromMetadata(t *testing.T) {
	newFakeMetadata(t, "proj-meta", "cluster-meta", "us-central1")
	s, err := Discover(context.Background(), config.Scope{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := Scope{ProjectID: "proj-meta", Location: "us-central1", ClusterName: "cluster-meta", ClusterScopeFilter: true}
	if s != want {
		t.Errorf("Discover = %+v, want %+v", s, want)
	}
}

func TestConfigOverridesWin(t *testing.T) {
	newFakeMetadata(t, "proj-meta", "cluster-meta", "us-central1")
	s, err := Discover(context.Background(), config.Scope{ProjectID: "proj-cfg", ClusterName: "cluster-cfg"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if s.ProjectID != "proj-cfg" || s.ClusterName != "cluster-cfg" {
		t.Errorf("config values did not win: %+v", s)
	}
	if s.Location != "us-central1" {
		t.Errorf("missing field not filled from metadata: %+v", s)
	}
}

func TestFullConfigSkipsMetadata(t *testing.T) {
	// Point at a dead host: Discover must not touch metadata at all.
	t.Setenv("GCE_METADATA_HOST", "127.0.0.1:1")
	s, err := Discover(context.Background(), config.Scope{
		ProjectID: "p", Location: "l", ClusterName: "c",
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if s.ProjectID != "p" || s.Location != "l" || s.ClusterName != "c" {
		t.Errorf("Discover = %+v", s)
	}
}

func TestUnscopedNeedsOnlyProject(t *testing.T) {
	t.Setenv("GCE_METADATA_HOST", "127.0.0.1:1")
	off := false
	s, err := Discover(context.Background(), config.Scope{ProjectID: "p", ClusterScopeFilter: &off})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if s.ClusterScopeFilter {
		t.Error("ClusterScopeFilter = true, want false")
	}
}

func TestMissingClusterAttribute(t *testing.T) {
	newFakeMetadata(t, "proj-meta", "", "us-central1") // cluster-name 404s
	_, err := Discover(context.Background(), config.Scope{})
	if err == nil {
		t.Fatal("Discover succeeded, want error about cluster-name")
	}
	if !strings.Contains(err.Error(), "cluster-name") || !strings.Contains(err.Error(), "clusterScopeFilter") {
		t.Errorf("error not actionable: %v", err)
	}
	var nde metadata.NotDefinedError
	if !errors.As(err, &nde) {
		t.Errorf("error does not wrap metadata.NotDefinedError: %v", err)
	}
}

func TestMetadataUnreachable(t *testing.T) {
	t.Setenv("GCE_METADATA_HOST", "127.0.0.1:1")
	_, err := Discover(context.Background(), config.Scope{})
	if err == nil {
		t.Fatal("Discover succeeded with no config and no metadata")
	}
	// Note: the metadata library caches project-id process-wide, so if an
	// earlier test resolved it against the fake server, the failure here
	// may surface on a cluster attribute instead. Either way the error
	// must name the config field that fixes it.
	if !strings.Contains(err.Error(), "set scope.") {
		t.Errorf("error not actionable: %v", err)
	}
}
