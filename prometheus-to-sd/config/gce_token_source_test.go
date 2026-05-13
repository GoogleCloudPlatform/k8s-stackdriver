/*
Copyright 2015 The Kubernetes Authors.
Modifications copyright (C) 2020 Google Inc.

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAltTokenSourceToken(t *testing.T) {
	// Create a mock token server
	tokenResponse := struct {
		AccessToken string    `json:"accessToken"`
		ExpireTime  time.Time `json:"expireTime"`
	}{
		AccessToken: "test-access-token",
		ExpireTime:  time.Now().Add(time.Hour),
	}
	responseJSON, _ := json.Marshal(tokenResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		assert.Equal(t, "POST", r.Method)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
	}))
	defer server.Close()

	// Test the internal token method directly to avoid GCE auth requirements
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	// Create a mock oauth client that doesn't require GCE
	ts.oauthClient = &http.Client{}
	ts.throttle = nil // Disable throttling for tests

	oauthToken, err := ts.token()
	assert.NoError(t, err)
	assert.NotNil(t, oauthToken)
	assert.Equal(t, tokenResponse.AccessToken, oauthToken.AccessToken)
	assert.Equal(t, tokenResponse.ExpireTime.UTC(), oauthToken.Expiry.UTC())
}

func TestAltTokenSourceTokenError(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	// Test the internal token method directly
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	ts.oauthClient = &http.Client{}
	ts.throttle = nil

	oauthToken, err := ts.token()
	assert.Error(t, err)
	assert.Nil(t, oauthToken)
}

func TestAltTokenSourceTokenInvalidJSON(t *testing.T) {
	// Create a mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// Test the internal token method directly
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	ts.oauthClient = &http.Client{}
	ts.throttle = nil

	oauthToken, err := ts.token()
	assert.Error(t, err, oauthToken)
}

func TestAltTokenSourceTokenMissingAccessToken(t *testing.T) {
	// Create a mock server that returns JSON without accessToken
	responseJSON, _ := json.Marshal(map[string]interface{}{
		"expireTime": time.Now().Add(time.Hour),
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
	}))
	defer server.Close()

	// Test the internal token method directly
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	ts.oauthClient = &http.Client{}
	ts.throttle = nil

	oauthToken, err := ts.token()
	assert.NoError(t, err)
	assert.NotNil(t, oauthToken)
	// AccessToken should be empty string when missing in response
	assert.Equal(t, "", oauthToken.AccessToken)
}

func TestAltTokenSourceTokenExpiry(t *testing.T) {
	// Create a mock token server with past expiry
	pastExpiry := time.Now().Add(-time.Hour)
	tokenResponse := struct {
		AccessToken string    `json:"accessToken"`
		ExpireTime  time.Time `json:"expireTime"`
	}{
		AccessToken: "test-access-token",
		ExpireTime:  pastExpiry,
	}
	responseJSON, _ := json.Marshal(tokenResponse)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
	}))
	defer server.Close()

	// Test the internal token method directly
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	ts.oauthClient = &http.Client{}
	ts.throttle = nil

	oauthToken, err := ts.token()
	assert.NoError(t, err)
	assert.NotNil(t, oauthToken)
	assert.Equal(t, tokenResponse.AccessToken, oauthToken.AccessToken)
	// Token should be expired
	assert.True(t, oauthToken.Expiry.Before(time.Now()))
}

func TestAltTokenSourceDifferentTokens(t *testing.T) {
	// Create a mock token server that returns different tokens
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		tokenResponse := struct {
			AccessToken string    `json:"accessToken"`
			ExpireTime  time.Time `json:"expireTime"`
		}{
			AccessToken: "test-access-token-" + string(rune('A'+callCount)),
			ExpireTime:  time.Now().Add(time.Hour),
		}
		responseJSON, _ := json.Marshal(tokenResponse)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
	}))
	defer server.Close()

	// Create token source without caching (each call fetches new token)
	ts := &AltTokenSource{
		tokenURL:  server.URL,
		tokenBody: "",
	}
	ts.oauthClient = &http.Client{}

	// Directly call token() to bypass caching
	token1, err := ts.token()
	assert.NoError(t, err)
	assert.NotNil(t, token1)

	token2, err := ts.token()
	assert.NoError(t, err)
	assert.NotNil(t, token2)

	// Tokens should be different since we're calling token() directly
	assert.NotEqual(t, token1.AccessToken, token2.AccessToken)
}
