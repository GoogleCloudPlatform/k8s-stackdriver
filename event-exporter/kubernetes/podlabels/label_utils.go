package podlabels

import (
	"regexp"
)

const (
	ownerTypeKeyName   = "logging.gke.io/top_level_controller_type"
	ownerNameKeyName   = "logging.gke.io/top_level_controller_name"
	jobSetNameLabelKey = "jobset.sigs.k8s.io/jobset-name"
)

// matches suffixes containing number between 20000000 to 59999999
// These 2 numbers are chosen because the convenience of regex matching:
// 20000000: Thu Jan 10 2008 21:20:00 GMT+0000  in minutes since Unix Epoch
// 59999999: Sat Jan 29 2084 15:59:00 GMT+0000  in minutes since Unix Epoch
var generatedUnixTimeSuffixMatcher = regexp.MustCompile("^-[2-5][0-9]{7}$")

// stripUnixTimeSuffix removes the time suffix added by kubernetes when generating job names from cronjob
// If a suffix is detected and removed, it returns the remaining string and true;
// otherwise it returns empty string and false.
// It should match the kubernetes behavior:
// https://github.com/kubernetes/kubernetes/blob/v1.30.1/pkg/controller/cronjob/cronjob_controllerv2.go#L662
func stripUnixTimeSuffix(name string) (string, bool) {
	if len(name) < 10 {
		return "", false
	}
	if generatedUnixTimeSuffixMatcher.MatchString(name[len(name)-9:]) {
		return name[:len(name)-9], true
	}
	return "", false
}
