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

package main

import (
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/fields"
)

func TestStartPprofServer(t *testing.T) {
	if s := startPprofServer(""); s != nil {
		t.Errorf("startPprofServer(\"\") expected nil, got %v", s)
	}

	server := startPprofServer("11123")
	if server == nil {
		t.Fatalf("startPprofServer(\"11123\") returned nil")
	}
	defer server.Close()

	if server.Addr != ":11123" {
		t.Errorf("server.Addr = %q, want \":11123\"", server.Addr)
	}

	endpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
	}

	for _, endpoint := range endpoints {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			t.Fatalf("Failed to create request for %s: %v", endpoint, err)
		}
		_, pattern := server.Handler.(*http.ServeMux).Handler(req)
		if pattern == "" {
			t.Errorf("pprof endpoint %q matched empty pattern on server handler", endpoint)
		}
	}
}

func TestEventFieldSelectorFlag(t *testing.T) {
	testCases := []struct {
		desc        string
		selector    string
		expectError bool
		expectedStr string
	}{
		{
			desc:        "empty selector",
			selector:    "",
			expectError: false,
			expectedStr: "",
		},
		{
			desc:        "valid field selector",
			selector:    "type=Warning,involvedObject.kind=Pod",
			expectError: false,
			expectedStr: "involvedObject.kind=Pod,type=Warning",
		},
		{
			desc:        "invalid field selector operator",
			selector:    "type in (Warning)",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			fs, err := fields.ParseSelector(tc.selector)
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error parsing selector %q, got nil", tc.selector)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error parsing selector %q: %v", tc.selector, err)
				}
				if fs.String() != tc.expectedStr {
					t.Errorf("fs.String() = %q, want %q", fs.String(), tc.expectedStr)
				}
			}
		})
	}
}
