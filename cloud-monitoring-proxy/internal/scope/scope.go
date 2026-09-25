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

// Package scope resolves which project/location/cluster the proxy serves.
// Values come from config when set, otherwise from the GCE metadata server
// (GKE publishes cluster-name and cluster-location as instance attributes).
package scope

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/compute/metadata"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/config"
)

// Scope is the fully resolved query scope.
type Scope struct {
	ProjectID   string
	Location    string
	ClusterName string
	// ClusterScopeFilter mirrors config: when true, every query for a
	// resource type carrying cluster labels is restricted to this cluster.
	ClusterScopeFilter bool
}

// Discover resolves the scope. Explicit config always wins; the metadata
// server fills gaps. Outside GCP with an incomplete config it returns an
// actionable error rather than guessing.
func Discover(ctx context.Context, cfg config.Scope) (Scope, error) {
	s := Scope{
		ProjectID:          cfg.ProjectID,
		Location:           cfg.Location,
		ClusterName:        cfg.ClusterName,
		ClusterScopeFilter: cfg.ClusterScoped(),
	}

	needProject := s.ProjectID == ""
	needCluster := s.ClusterScopeFilter && (s.Location == "" || s.ClusterName == "")
	if !needProject && !needCluster {
		return s, nil
	}

	// The client honors GCE_METADATA_HOST (which tests point at a fake).
	mc := metadata.NewClient(&http.Client{Timeout: 3 * time.Second})
	if needProject {
		v, err := mc.ProjectIDWithContext(ctx)
		if err != nil {
			return Scope{}, fmt.Errorf("scope.projectID is not set and the metadata server is unavailable "+
				"(set scope.projectID in the config when running outside GCP): %w", err)
		}
		s.ProjectID = v
	}
	if needCluster {
		if s.Location == "" {
			v, err := mc.InstanceAttributeValueWithContext(ctx, "cluster-location")
			if err != nil {
				return Scope{}, clusterAttrErr("cluster-location", "scope.location", err)
			}
			s.Location = v
		}
		if s.ClusterName == "" {
			v, err := mc.InstanceAttributeValueWithContext(ctx, "cluster-name")
			if err != nil {
				return Scope{}, clusterAttrErr("cluster-name", "scope.clusterName", err)
			}
			s.ClusterName = v
		}
	}
	return s, nil
}

func clusterAttrErr(attr, field string, err error) error {
	return fmt.Errorf("cluster scoping is enabled but the %s metadata attribute is unavailable "+
		"(set %s in the config, or set scope.clusterScopeFilter: false to serve unscoped): %w",
		attr, field, err)
}
