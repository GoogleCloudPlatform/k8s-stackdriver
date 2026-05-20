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

package controller

import (
	"net/http"
	"testing"
)

func TestNewClient(t *testing.T) {
	testCases := []struct {
		name        string
		host        string
		port        uint
		expectedURL string
	}{
		{
			name:        "IPv4",
			host:        "127.0.0.1",
			port:        10250,
			expectedURL: "http://127.0.0.1:10250/metrics",
		},
		{
			name:        "IPv6",
			host:        "2001:db8::1",
			port:        10250,
			expectedURL: "http://[2001:db8::1]:10250/metrics",
		},
		{
			name:        "Hostname",
			host:        "localhost",
			port:        8080,
			expectedURL: "http://localhost:8080/metrics",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(tc.host, tc.port, http.DefaultClient)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			if c.metricsURL.String() != tc.expectedURL {
				t.Errorf("Expected URL %q, got %q", tc.expectedURL, c.metricsURL.String())
			}
		})
	}
}
