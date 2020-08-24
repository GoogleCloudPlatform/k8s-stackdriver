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
	"os"
	"time"

	gceconfig "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	stackdriver "google.golang.org/api/monitoring/v3"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/logs"
	"k8s.io/klog"
	"sigs.k8s.io/metrics-server/pkg/api"

	coreadapter "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/coreprovider"
	adapter "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/provider"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
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
	// FallbackForContainerMetrics provides metrics from container when metric is not present in pod
	FallbackForContainerMetrics bool
	// EnableCoreMetricsAPI provides core metrics. Experimental, do not use.
	EnableCoreMetricsAPI bool
}

func (sa *StackdriverAdapter) makeProviderOrDie(o *stackdriverAdapterServerOptions, rateInterval time.Duration, alignmentPeriod time.Duration) provider.MetricsProvider {
	config, err := sa.ClientConfig()
	if err != nil {
		klog.Fatalf("unable to construct client config: %v", err)
	}

	client, err := coreclient.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to construct client: %v", err)
	}

	mapper, err := sa.RESTMapper()
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
	gceConf, err := gceconfig.GetGceConfig()
	if err != nil {
		klog.Fatalf("Failed to retrieve GCE config: %v", err)
	}
	translator := adapter.NewTranslator(stackdriverService, gceConf, rateInterval, alignmentPeriod, realClock{}, mapper, o.UseNewResourceModel)
	return adapter.NewStackdriverProvider(client, mapper, gceConf, stackdriverService, translator, rateInterval, o.UseNewResourceModel, o.FallbackForContainerMetrics)
}

func (sa *StackdriverAdapter) withCoreMetrics() error {
	provider := coreadapter.NewCoreProvider()
	informers, err := sa.Informers()
	if err != nil {
		return err
	}

	server, err := sa.Server()
	if err != nil {
		return err
	}

	if err := api.Install(provider, informers.Core().V1(), server.GenericAPIServer); err != nil {
		return err
	}

	return nil
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
		UseNewResourceModel:         false,
		EnableCustomMetricsAPI:      true,
		EnableExternalMetricsAPI:    true,
		FallbackForContainerMetrics: false,
		EnableCoreMetricsAPI:        false,
	}

	flags.BoolVar(&serverOptions.UseNewResourceModel, "use-new-resource-model", serverOptions.UseNewResourceModel,
		"whether to use new Stackdriver resource model")
	flags.BoolVar(&serverOptions.EnableCustomMetricsAPI, "enable-custom-metrics-api", serverOptions.EnableCustomMetricsAPI,
		"whether to enable Custom Metrics API")
	flags.BoolVar(&serverOptions.EnableExternalMetricsAPI, "enable-external-metrics-api", serverOptions.EnableExternalMetricsAPI,
		"whether to enable External Metrics API")
	flags.BoolVar(&serverOptions.FallbackForContainerMetrics, "fallback-for-container-metrics", serverOptions.FallbackForContainerMetrics,
		"If true, fallbacks to k8s_container resource when given metric is not present on k8s_pod. At most one container with given metric is allowed for each pod.")
	flags.BoolVar(&serverOptions.EnableCoreMetricsAPI, "enable-core-metrics-api", serverOptions.EnableCoreMetricsAPI,
		"Experimental, do not use. Whether to enable Core Metrics API.")

	flags.Parse(os.Args)

	if !serverOptions.UseNewResourceModel && serverOptions.FallbackForContainerMetrics {
		klog.Fatalf("Container metrics work only with new resource model")
	}
	if !serverOptions.UseNewResourceModel && serverOptions.EnableCoreMetricsAPI {
		klog.Fatalf("Core metrics work only with new resource model")
	}

	metricsProvider := cmd.makeProviderOrDie(&serverOptions, 5*time.Minute, 1*time.Minute)
	if serverOptions.EnableCustomMetricsAPI {
		cmd.WithCustomMetrics(metricsProvider)
	}
	if serverOptions.EnableExternalMetricsAPI {
		cmd.WithExternalMetrics(metricsProvider)
	}
	if serverOptions.EnableCoreMetricsAPI {
		if err := cmd.withCoreMetrics(); err != nil {
			klog.Fatalf("unable to install resource metrics API: %v", err)
		}
	}

	if err := cmd.Run(wait.NeverStop); err != nil {
		klog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}
