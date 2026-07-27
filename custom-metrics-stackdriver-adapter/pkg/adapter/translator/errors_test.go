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
	"fmt"
	"net/http"
	"strings"
	"testing"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestMetricErrors_QuotesMetricNames(t *testing.T) {
	metricName := "<script>alert('xss')</script>"
	resourceName := "<img src=x onerror=alert(1)>"
	gr := schema.GroupResource{Group: "apps", Resource: "deployments"}

	quotedMetricName := fmt.Sprintf("%q", metricName)
	quotedResourceName := fmt.Sprintf("%q", resourceName)

	testCases := []struct {
		name              string
		err               *apierr.StatusError
		expectedSubstring string
	}{
		{
			name:              "NewNoSuchMetricError",
			err:               NewNoSuchMetricError(metricName, errors.New("not found")),
			expectedSubstring: quotedMetricName,
		},
		{
			name:              "NewMetricNotFoundError",
			err:               NewMetricNotFoundError(gr, metricName),
			expectedSubstring: quotedMetricName,
		},
		{
			name:              "NewMetricNotFoundForError",
			err:               NewMetricNotFoundForError(gr, metricName, resourceName),
			expectedSubstring: quotedMetricName,
		},
		{
			name:              "NewExternalMetricNotFoundError",
			err:               NewExternalMetricNotFoundError(metricName),
			expectedSubstring: quotedMetricName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("expected non-nil StatusError")
			}
			if tc.err.ErrStatus.Code != http.StatusNotFound {
				t.Errorf("expected status code %d, got %d", http.StatusNotFound, tc.err.ErrStatus.Code)
			}
			if !strings.Contains(tc.err.ErrStatus.Message, tc.expectedSubstring) {
				t.Errorf("expected status message to contain quoted string %s, got %q", tc.expectedSubstring, tc.err.ErrStatus.Message)
			}
		})
	}

	// Also verify resourceName is quoted in NewMetricNotFoundForError
	errWithResource := NewMetricNotFoundForError(gr, metricName, resourceName)
	if !strings.Contains(errWithResource.ErrStatus.Message, quotedResourceName) {
		t.Errorf("expected status message to contain quoted resource name %s, got %q", quotedResourceName, errWithResource.ErrStatus.Message)
	}
}
