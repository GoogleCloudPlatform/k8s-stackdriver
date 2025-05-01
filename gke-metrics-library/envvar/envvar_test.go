package envvar

import (
	"errors"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLookupEnv(t *testing.T) {
	envVarKey := "TEST_GKE_METRICS_ENV_VAR"
	envVarValue := "NON_EMPTY_VALUE"
	os.Setenv(envVarKey, envVarValue)
	t.Cleanup(func() { os.Unsetenv(envVarKey) })

	gotValue, err := LookupEnv(envVarKey)
	if err != nil {
		t.Fatalf("LookupEnv(%q): %v, want nil error", envVarKey, err)
	}

	if gotValue != envVarValue {
		t.Errorf("LookupEnv(%q): got %q, want %q", envVarKey, gotValue, envVarValue)
	}
}

func TestLookupEnvErrors(t *testing.T) {
	envVarKey := "TEST_GKE_METRICS_ENV_VAR"
	wantErr := ErrEmpty{envVarKey}
	os.Unsetenv(envVarKey)

	// This must fail when the environment variable has not been set.
	_, err := LookupEnv(envVarKey)
	if !errors.Is(err, wantErr) {
		t.Errorf("LookupEnv(%q): %v, want %v", envVarKey, err, wantErr)
	}

	// This must fail when the environment variable has set to an empty string.
	os.Setenv(envVarKey, "")
	_, err = LookupEnv(envVarKey)
	if !errors.Is(err, wantErr) {
		t.Errorf("LookupEnv(%q): %v, want %v", envVarKey, err, wantErr)
	}
}

func TestLookupEnvList(t *testing.T) {
	testCases := []struct {
		desc              string
		initializeEnvVars map[string]string
		lookupEnvVars     []string
		want              map[string]string
		wantErr           error
	}{
		{
			desc: "OK",
			initializeEnvVars: map[string]string{
				"TEST_ENV_VAR_1": "NON_EMPTY_VALUE",
				"TEST_ENV_VAR_2": "NON_EMPTY_VALUE_2",
			},
			lookupEnvVars: []string{"TEST_ENV_VAR_1", "TEST_ENV_VAR_2"},
			want: map[string]string{
				"TEST_ENV_VAR_1": "NON_EMPTY_VALUE",
				"TEST_ENV_VAR_2": "NON_EMPTY_VALUE_2",
			},
		},
		{
			desc: "OK: empty list",
			initializeEnvVars: map[string]string{
				"TEST_ENV_VAR_1": "NON_EMPTY_VALUE",
				"TEST_ENV_VAR_2": "NON_EMPTY_VALUE_2",
			},
			lookupEnvVars: []string{},
			want:          map[string]string{},
		},
		{
			desc:              "Error: tries to lookup env var that is not set.",
			initializeEnvVars: map[string]string{},
			lookupEnvVars:     []string{"TEST_ENV_VAR_1", "TEST_ENV_VAR_2"},
			wantErr:           NewErrEmpty("TEST_ENV_VAR_1"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			for k, v := range tc.initializeEnvVars {
				t.Setenv(k, v)
			}

			got, err := LookupEnvList(tc.lookupEnvVars)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("LookupEnvList(%v): %v, want %v error", tc.lookupEnvVars, err, tc.wantErr)
			}
			// Skip checking the `got` value when an error is expected
			if tc.wantErr != nil {
				return
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("LookupEnvList(%v) returned diff (-want +got):\n%s", tc.lookupEnvVars, diff)
			}
		})
	}
}
