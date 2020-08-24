/*
Copyright 2017 The Kubernetes Authors.

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

package provider

import (
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	sd "google.golang.org/api/monitoring/v3"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
)

var (
	podKind schema.ObjectKind
)

type fakeClock struct {
	time time.Time
}

func (f fakeClock) Now() time.Time {
	return f.time
}

func newFakeTranslator(reqWindow, alignmentPeriod time.Duration, project, cluster, location string, currentTime time.Time, useNewResourceModel bool) (*Translator, *sd.Service) {
	sdService, err := sd.New(http.DefaultClient)
	if err != nil {
		klog.Fatal("Unexpected error creating stackdriver Service client")
	}
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Pod"), meta.RESTScopeNamespace)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("Node"), meta.RESTScopeRoot)
	return &Translator{
		service:         sdService,
		reqWindow:       reqWindow,
		alignmentPeriod: alignmentPeriod,
		config: &config.GceConfig{
			Project:  project,
			Cluster:  cluster,
			Location: location,
		},
		clock:               fakeClock{currentTime},
		mapper:              restMapper,
		useNewResourceModel: useNewResourceModel,
	}, sdService
}

// newFakeTranslatorForExternalMetrics returns a simplified translator, where only the fields used
// for External Metrics API need to be specified. Other fields are initialized to zeros.
func newFakeTranslatorForExternalMetrics(reqWindow, alignmentPeriod time.Duration, project string, currentTime time.Time) (*Translator, *sd.Service) {
	return newFakeTranslator(reqWindow, alignmentPeriod, project, "", "", currentTime, false)
}
