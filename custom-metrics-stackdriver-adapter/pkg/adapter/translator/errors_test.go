/*
Copyright 2026 The Kubernetes Authors.

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

package translator

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestNewNoSuchMetricError_SanitizesSensitiveError(t *testing.T) {
	metricName := "custom.googleapis.com/test_metric"
	sensitiveErr := errors.New("googleapi: Error 403: Permission google.navigation... denied, service account test-sa@my-gcp-project.iam.gserviceaccount.com does not have permission")

	statusErr := NewNoSuchMetricError(metricName, sensitiveErr)

	if statusErr == nil {
		t.Fatalf("expected non-nil StatusError")
	}

	if statusErr.ErrStatus.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, statusErr.ErrStatus.Code)
	}

	if strings.Contains(statusErr.ErrStatus.Message, "test-sa@my-gcp-project.iam.gserviceaccount.com") {
		t.Errorf("status message leaks sensitive service account details: %s", statusErr.ErrStatus.Message)
	}
}
