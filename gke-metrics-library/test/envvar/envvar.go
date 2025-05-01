// Package envvar provides helpers and defaults for setting (and unsetting)
// environment variables in tests.
package envvar

import (
	"testing"
)

// TestVariable that can be set in a test.
type TestVariable struct {
	Key   string
	Value string
}

// Setup environment variables for the test and unset any environment variables set this way.
func Setup(t testing.TB, envs ...TestVariable) {
	for _, e := range envs {
		t.Setenv(e.Key, e.Value)
	}
}

// DefaultMetricsTestEnvironmentVariables used across most metrics related tests.
var DefaultMetricsTestEnvironmentVariables = []TestVariable{
	{Key: "PROJECT_NUMBER", Value: "000000000"},
	{Key: "LOCATION", Value: "location"},
	{Key: "CLUSTER_NAME", Value: "cluster"},
	{Key: "POD_NAMESPACE", Value: "namespace"},
	{Key: "POD_NAME", Value: "pod"},
	{Key: "CONTAINER_NAME", Value: "container"},
}
