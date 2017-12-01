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

// PodConfig describes pod in which current component is running.
type PodConfig struct {
	PodId       string
	NamespaceId string
}

// CommonConfig contains all required information about environment in which
// prometheus-to-sd running and which component is monitored.
type CommonConfig struct {
	GceConfig     *GceConfig
	PodConfig     *PodConfig
	ComponentName string
	// Contains metrics which will be converted from Prometheus
	// Gauge to Stackdriver Cumulative. Temporary workaround
	// which will be removed.
	GaugeToCumulativeWhitelist []string
}
