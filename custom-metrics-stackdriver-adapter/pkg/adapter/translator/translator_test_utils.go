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

package translator

import (
	"net/http"
	"time"

	sd "google.golang.org/api/monitoring/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
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

// NewFakeTranslator creates a Translator for testing purposes
func NewFakeTranslator(reqWindow, alignmentPeriod time.Duration, project, cluster, location string, currentTime time.Time, useNewResourceModel bool) (*Translator, *sd.Service) {
	return newFakeTranslator(reqWindow, alignmentPeriod, project, cluster, location, currentTime, useNewResourceModel, false /* do not support distributions */)
}

// newFakeTranslatorForExternalMetrics returns a simplified translator, where only the fields used
// for External Metrics API need to be specified. Other fields are initialized to zeros.
func newFakeTranslatorForExternalMetrics(reqWindow, alignmentPeriod time.Duration, project string, currentTime time.Time) (*Translator, *sd.Service) {
	return newFakeTranslator(reqWindow, alignmentPeriod, project, "", "", currentTime, false /* don't use new resouce model */, false /* don't support distributions */)
}

// NewFakeTranslatorForExternalMetricsWithSDService returns a simplified translator with the provided stackdriver service.
func NewFakeTranslatorForExternalMetricsWithSDService(reqWindow, alignmentPeriod time.Duration, project string, currentTime time.Time, sdService *sd.Service) *Translator {
	t, _ := newFakeTranslator(reqWindow, alignmentPeriod, project, "", "", currentTime, false /* don't use new resouce model */, false /* don't support distributions */)
	t.service = sdService
	return t
}

// newFakeTranslatorForDistributions creates a Translator that supports Distributions for testing purposes.
// This
func newFakeTranslatorForDistributions(reqWindow, alignmentPeriod time.Duration, project, cluster, location string, currentTime time.Time, useNewResourceModel bool) (*Translator, *sd.Service) {
	return newFakeTranslator(reqWindow, alignmentPeriod, project, cluster, location, currentTime, useNewResourceModel, true /* support distributions */)
}

func newFakeTranslator(reqWindow, alignmentPeriod time.Duration, project, cluster, location string, currentTime time.Time, useNewResourceModel, supportDistributions bool) (*Translator, *sd.Service) {
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
		clock:                fakeClock{currentTime},
		mapper:               restMapper,
		useNewResourceModel:  useNewResourceModel,
		supportDistributions: supportDistributions,
	}, sdService
}
