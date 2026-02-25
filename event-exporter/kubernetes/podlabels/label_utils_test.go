package podlabels

import (
	"testing"
)

func TestStripUnixTimeSuffix(t *testing.T) {
	testCases := []struct {
		description     string
		jobName         string
		wantCronJob     bool
		wantCronJobName string
	}{
		{
			description:     "valid job name owned by cronjob",
			jobName:         "hello-world-32345678",
			wantCronJob:     true,
			wantCronJobName: "hello-world",
		},
		{
			description: "invalid timestamp, must be above lower bound 20000000",
			jobName:     "hello-world-12345678",
			wantCronJob: false,
		},
		{
			description: "invalid job name, missing timestamp suffix",
			jobName:     "foo",
			wantCronJob: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			cronJobName, ok := stripUnixTimeSuffix(tc.jobName)
			if tc.wantCronJob != ok {
				t.Errorf("stripUnixTimeSuffix(%q) returned incorrect cronjob decision flag, want %t, got %t", tc.jobName, tc.wantCronJob, ok)
			}
			if ok && tc.wantCronJobName != cronJobName {
				t.Errorf("stripUnixTimeSuffix(%q) returned incorrect cronjob name, want %s, got %s", tc.jobName, tc.wantCronJobName, cronJobName)
			}
		})
	}
}
