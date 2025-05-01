// Package envvar contains functions to interact with the OS environment variables.
package envvar

import (
	"fmt"
	"os"
)

// ErrEmpty indicates an environment variable that was either not set or set to an empty value.
type ErrEmpty struct {
	key string
}

func (e ErrEmpty) Error() string {
	return fmt.Sprintf("environment variable %q must be set to a non-empty value", e.key)
}

// NewErrEmpty creates a new ErrEmpty for the environment variable named by the key.
func NewErrEmpty(key string) error {
	return ErrEmpty{key: key}
}

// LookupEnv retrieves the value of the environment variable named by the key.
// It returns an error if the environment variable value is empty or if no variable is found with
// that name.
func LookupEnv(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", NewErrEmpty(key)
	}
	return value, nil
}

// LookupEnvList retrieves the value of the environment variables named by the keys
// and returns them as a map.
// It returns an error if the environment variable value is empty or if no variable
// is found with that name.
func LookupEnvList(keys []string) (map[string]string, error) {
	envVars := make(map[string]string)
	for _, key := range keys {
		value, err := LookupEnv(key)
		if err != nil {
			return nil, err
		}
		envVars[key] = value
	}
	return envVars, nil
}

// LookupEnvOrDefault retrieves the value of the environment variable or returns the specified
// default if the variable is unset or empty.
func LookupEnvOrDefault(key string, defaultValue string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return defaultValue
	}
	return value
}
