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

package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAuthConfig(t *testing.T) {
	testCases := []struct {
		description    string
		url            string
		expectedConfig AuthConfig
		expectError    bool
	}{
		{
			description: "empty auth config",
			url:         "http://localhost:8080",
			expectedConfig: AuthConfig{
				Username: "",
				Password: "",
				Token:    "",
			},
			expectError: false,
		},
		{
			description: "token auth",
			url:         "http://localhost:8080?authToken=test-token",
			expectedConfig: AuthConfig{
				Username: "",
				Password: "",
				Token:    "test-token",
			},
			expectError: false,
		},
		{
			description: "basic auth",
			url:         "http://localhost:8080?authUsername=user&authPassword=pass",
			expectedConfig: AuthConfig{
				Username: "user",
				Password: "pass",
				Token:    "",
			},
			expectError: false,
		},
		{
			description: "both token and basic auth - token takes precedence",
			url:         "http://localhost:8080?authToken=test-token&authUsername=user&authPassword=pass",
			expectedConfig: AuthConfig{
				Username: "user",
				Password: "pass",
				Token:    "test-token",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			parsedURL, err := url.Parse(tc.url)
			assert.NoError(t, err)

			config, err := parseAuthConfig(*parsedURL)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedConfig.Username, config.Username)
				assert.Equal(t, tc.expectedConfig.Password, config.Password)
				assert.Equal(t, tc.expectedConfig.Token, config.Token)
			}
		})
	}
}

func TestParseAuthConfigWithTokenFile(t *testing.T) {
	// Create a temporary token file
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "token.txt")
	tokenContent := "file-based-token"
	err := os.WriteFile(tokenFile, []byte(tokenContent), 0644)
	assert.NoError(t, err)

	testCases := []struct {
		description    string
		url            string
		expectedConfig AuthConfig
		expectError    bool
	}{
		{
			description: "token from file",
			url:         "http://localhost:8080?authTokenFile=" + tokenFile,
			expectedConfig: AuthConfig{
				Username: "",
				Password: "",
				Token:    tokenContent,
			},
			expectError: false,
		},
		{
			description: "token from file with whitespace",
			url:         "http://localhost:8080?authTokenFile=" + tokenFile,
			expectedConfig: AuthConfig{
				Username: "",
				Password: "",
				Token:    tokenContent,
			},
			expectError: false,
		},
		{
			description:    "non-existent token file",
			url:            "http://localhost:8080?authTokenFile=/non/existent/file",
			expectedConfig: AuthConfig{},
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			parsedURL, err := url.Parse(tc.url)
			assert.NoError(t, err)

			config, err := parseAuthConfig(*parsedURL)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedConfig.Username, config.Username)
				assert.Equal(t, tc.expectedConfig.Password, config.Password)
				assert.Equal(t, tc.expectedConfig.Token, config.Token)
			}
		})
	}
}

func TestParseAuthConfigWithWhitespaceInTokenFile(t *testing.T) {
	// Create a temporary token file with whitespace
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "token.txt")
	tokenContent := "  token-with-whitespace  \n"
	err := os.WriteFile(tokenFile, []byte(tokenContent), 0644)
	assert.NoError(t, err)

	parsedURL, err := url.Parse("http://localhost:8080?authTokenFile=" + tokenFile)
	assert.NoError(t, err)

	config, err := parseAuthConfig(*parsedURL)
	assert.NoError(t, err)
	// Should trim whitespace
	assert.Equal(t, "token-with-whitespace", config.Token)
}

func TestParseAuthConfigEmptyTokenFile(t *testing.T) {
	// Create a temporary empty token file
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "empty-token.txt")
	err := os.WriteFile(tokenFile, []byte(""), 0644)
	assert.NoError(t, err)

	parsedURL, err := url.Parse("http://localhost:8080?authTokenFile=" + tokenFile)
	assert.NoError(t, err)

	config, err := parseAuthConfig(*parsedURL)
	assert.NoError(t, err)
	// Empty file should result in empty token
	assert.Equal(t, "", config.Token)
}
