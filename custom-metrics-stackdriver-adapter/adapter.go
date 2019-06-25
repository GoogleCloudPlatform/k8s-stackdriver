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

package main

import (
	"flag"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	stackdriver "google.golang.org/api/monitoring/v3"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/logs"
	"k8s.io/klog"

	adapter "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/provider"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
)

// StackdriverAdapter is an adapter for Stackdriver
type StackdriverAdapter struct {
	basecmd.AdapterBase
}

type stackdriverAdapterServerOptions struct {
	// UseNewResourceModel is a flag that indicates whether new Stackdriver resource model should be
	// used
	UseNewResourceModel bool
	// EnableCustomMetricsAPI switches on sample apiserver for Custom Metrics API
	EnableCustomMetricsAPI bool
	// EnableExternalMetricsAPI switches on sample apiserver for External Metrics API
	EnableExternalMetricsAPI bool
}

func (a *StackdriverAdapter) makeProviderOrDie(o *stackdriverAdapterServerOptions) provider.MetricsProvider {
	config, err := a.ClientConfig()
	if err != nil {
		klog.Fatalf("unable to construct client config: %v", err)
	}

	client, err := coreclient.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to construct client: %v", err)
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		klog.Fatalf("unable to construct discovery REST mapper: %v", err)
	}

	tokenSource, err := google.DefaultTokenSource(oauth2.NoContext, "https://www.googleapis.com/auth/monitoring.read")
	if err != nil {
		klog.Fatalf("unable to use default token source: %v", err)
	}
	oauthClient := oauth2.NewClient(oauth2.NoContext, tokenSource)
	stackdriverService, err := stackdriver.New(oauthClient)
	if err != nil {
		klog.Fatalf("Failed to create Stackdriver client: %v", err)
	}

	return adapter.NewStackdriverProvider(client, mapper, stackdriverService, 5*time.Minute, time.Minute, o.UseNewResourceModel)
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	cmd := &StackdriverAdapter{
		basecmd.AdapterBase{
			Name: "custom-metrics-stackdriver-adapter",
		},
	}
	flags := cmd.Flags()

	flags.AddGoFlagSet(flag.CommandLine) // make sure we get the klog flags

	serverOptions := stackdriverAdapterServerOptions{
		UseNewResourceModel:      false,
		EnableCustomMetricsAPI:   true,
		EnableExternalMetricsAPI: true,
	}

	flags.BoolVar(&serverOptions.UseNewResourceModel, "use-new-resource-model", serverOptions.UseNewResourceModel,
		"whether to use new Stackdriver resource model")
	flags.BoolVar(&serverOptions.EnableCustomMetricsAPI, "enable-custom-metrics-api", serverOptions.EnableCustomMetricsAPI,
		"whether to enable Custom Metrics API")
	flags.BoolVar(&serverOptions.EnableExternalMetricsAPI, "enable-external-metrics-api", serverOptions.EnableExternalMetricsAPI,
		"whether to enable External Metrics API")

	flags.Parse(os.Args)

	metricsProvider := cmd.makeProviderOrDie(&serverOptions)
	if serverOptions.EnableCustomMetricsAPI {
		cmd.WithCustomMetrics(metricsProvider)
	}
	if serverOptions.EnableExternalMetricsAPI {
		cmd.WithExternalMetrics(metricsProvider)
	}

	if err := cmd.Run(wait.NeverStop); err != nil {
		klog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}
