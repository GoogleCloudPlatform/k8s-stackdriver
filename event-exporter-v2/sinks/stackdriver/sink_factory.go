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
	"flag"
	"fmt"
	"time"

	"golang.org/x/net/context"
	sd "google.golang.org/api/logging/v2"
	"google.golang.org/api/option"

	"k8s.io/utils/clock"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter-v2/kubernetes/podlabels"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-exporter-v2/sinks"
)

// SdSinkFactory is the Stackdriver sink factory that creates new Stackdriver sinks.
type SdSinkFactory struct {
	flagSet              *flag.FlagSet
	flushDelay           *time.Duration
	maxBufferSize        *int
	maxConcurrency       *int
	resourceModelVersion *string
	endpoint             *string
	universeDomain       *string
}

// NewSdSinkFactory creates a new Stackdriver sink factory.
func NewSdSinkFactory() *SdSinkFactory {
	fs := flag.NewFlagSet("stackdriver", flag.ContinueOnError)
	return &SdSinkFactory{
		flagSet: fs,
		flushDelay: fs.Duration("flush-delay", defaultFlushDelay, "Delay after receiving "+
			"the first event in batch before sending the request to Stackdriver, if batch"+
			"doesn't get sent before"),
		maxBufferSize: fs.Int("max-buffer-size", defaultMaxBufferSize, "Maximum number of events "+
			"in the request to Stackdriver"),
		maxConcurrency: fs.Int("max-concurrency", defaultMaxConcurrency, "Maximum number of "+
			"concurrent requests to Stackdriver"),
		resourceModelVersion: fs.String("stackdriver-resource-model", "", "Stackdriver resource model "+
			"to be used for exports"),
		endpoint:       fs.String("endpoint", defaultEndpoint, "Base path for Stackdriver API"),
		universeDomain: fs.String("universeDomain", defaultUniverseDomain, "The domain of the universe."),
	}
}

// CreateNew creates a new Stackdriver sink.
func (f *SdSinkFactory) CreateNew(opts []string, podLabelCollector podlabels.PodLabelCollector) (sinks.Sink, error) {
	return f.CreateNewWithOwnerChecker(opts, podLabelCollector, nil)
}

// CreateNewWithOwnerChecker creates a new Stackdriver sink with optional hash-based
// event ownership checking for multi-pod deployments.
func (f *SdSinkFactory) CreateNewWithOwnerChecker(opts []string, podLabelCollector podlabels.PodLabelCollector, ownerChecker EventOwnerChecker) (sinks.Sink, error) {
	err := f.flagSet.Parse(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink opts: %v", err)
	}

	config, err := f.createSinkConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build sink config: %v", err)
	}

	resourceModelFactory, err := f.createMonitoredResourceFactory()
	if err != nil {
		return nil, fmt.Errorf("failed to create stackdriver monitored resource factory: %v", err)
	}

	ctx := context.Background()
	service, err := sd.NewService(ctx, option.WithEndpoint(config.Endpoint), option.WithUniverseDomain(config.UniverseDomain))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Stackdriver service: %v", err)
	}
	writer := newSdWriter(service)

	clk := clock.RealClock{}

	return newSdSink(writer, clk, config, resourceModelFactory, podLabelCollector, ownerChecker), nil
}

func (f *SdSinkFactory) createMonitoredResourceFactory() (*monitoredResourceFactory, error) {
	resourceModelConfig, err := newMonitoredResourceFactoryConfig(*f.resourceModelVersion)
	if err != nil {
		return nil, err
	}
	return newMonitoredResourceFactory(resourceModelConfig), nil
}

func (f *SdSinkFactory) createSinkConfig() (*sdSinkConfig, error) {
	config, err := newGceSdSinkConfig()
	if err != nil {
		return nil, err
	}

	config.FlushDelay = *f.flushDelay
	config.MaxBufferSize = *f.maxBufferSize
	config.MaxConcurrency = *f.maxConcurrency
	config.Endpoint = *f.endpoint
	config.UniverseDomain = *f.universeDomain
	return config, nil
}
