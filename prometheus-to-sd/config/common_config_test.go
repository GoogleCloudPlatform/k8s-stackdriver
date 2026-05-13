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

package config

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidTokenFile(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{"/var/run/secrets/token", true},
		{"/var/run/secrets/sub/token", true},
		{"/etc/secrets/my-token", true},
		{"/etc/prometheus/token", true},
		{"/etc/passwd", false},
		{"/var/run/secrets/../../etc/passwd", false},
		{"relative/path", false},
		{"", false},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.valid, isValidTokenFile(tc.path), "path: %s", tc.path)
	}
}

func TestParseAuthConfigInvalidTokenFile(t *testing.T) {
	u, _ := url.Parse("http://localhost?authTokenFile=/etc/passwd")
	_, err := parseAuthConfig(*u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authTokenFile path")
}
