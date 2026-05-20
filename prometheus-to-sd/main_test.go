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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEndpointTrusted(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"monitoring.googleapis.com", "monitoring.googleapis.com:443"},
		{"monitoring.googleapis.com:443", "monitoring.googleapis.com:443"},
		{"https://monitoring.googleapis.com", "monitoring.googleapis.com:443"},
		{"http://test-monitoring.sandbox.googleapis.com:80", "test-monitoring.sandbox.googleapis.com:80"},
		{"googleapis.com", "googleapis.com:443"},
	}

	for _, tc := range tests {
		res, err := getEndpoint(tc.input)
		if assert.NoError(t, err, "input: %s", tc.input) {
			assert.Equal(t, tc.expected, res, "input: %s", tc.input)
		}
	}
}

func TestGetEndpointUntrusted(t *testing.T) {
	tests := []string{
		"attacker.com",
		"https://attacker.com",
		"monitoring.googleapis.com.attacker.com",
		"https://attacker.com/?foo=.googleapis.com",
		"http://127.0.0.1:8080",
	}

	for _, tc := range tests {
		_, err := getEndpoint(tc)
		assert.Error(t, err, "input: %s", tc)
		assert.Contains(t, err.Error(), "untrusted endpoint")
	}
}
