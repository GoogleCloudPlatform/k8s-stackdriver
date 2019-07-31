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

package stackdriver

import (
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/compute/metadata"
)

const (
	defaultFlushDelay     = 5 * time.Second
	defaultMaxBufferSize  = 100
	defaultMaxConcurrency = 10
	defaultEndpoint       = ""

	eventsLogName = "events"
)

type sdSinkConfig struct {
	FlushDelay     time.Duration
	MaxBufferSize  int
	MaxConcurrency int
	LogName        string
	Endpoint       string
}

func newGceSdSinkConfig() (*sdSinkConfig, error) {
	if !metadata.OnGCE() {
		return nil, errors.New("not running on GCE, which is not supported for Stackdriver sink")
	}

	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil, fmt.Errorf("failed to get project id: %v", err)
	}

	logName := fmt.Sprintf("projects/%s/logs/%s", projectID, eventsLogName)

	return &sdSinkConfig{
		FlushDelay:     defaultFlushDelay,
		MaxBufferSize:  defaultMaxBufferSize,
		MaxConcurrency: defaultMaxConcurrency,
		LogName:        logName,
		Endpoint:       defaultEndpoint,
	}, nil
}
