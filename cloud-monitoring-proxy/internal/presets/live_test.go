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

// Live validation: every kubernetes.io/* metric type in every bundle must
// exist as a real descriptor. prometheus.googleapis.com/* entries are
// skipped here (those descriptors exist only after the package writes
// data) and are verified by the e2e suite instead.
//
//	GCM_PROXY_TEST_PROJECT=<your-project> go test -tags live ./internal/presets -v
package presets

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/cloud-monitoring-proxy/internal/gcm"
)

func TestLiveBundleTypesExist(t *testing.T) {
	project := os.Getenv("GCM_PROXY_TEST_PROJECT")
	if project == "" {
		t.Skip("GCM_PROXY_TEST_PROJECT not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := gcm.New(ctx, project, 4, gcm.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	dc := gcm.NewDescriptorCache(c)
	if err := dc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	checked, skipped := 0, 0
	for _, name := range Names() {
		b, _ := Get(name)
		for _, typ := range b.Types {
			if strings.HasPrefix(typ, "prometheus.googleapis.com/") {
				skipped++
				continue
			}
			if _, ok := dc.Get(typ); !ok {
				t.Errorf("bundle %q: metric type %q does not exist in live project", name, typ)
			}
			checked++
		}
	}
	t.Logf("validated %d types against live descriptors (%d prometheus.googleapis.com entries deferred to e2e)", checked, skipped)
}
