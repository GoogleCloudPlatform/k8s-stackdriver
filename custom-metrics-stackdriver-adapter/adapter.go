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
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	stackdriver "google.golang.org/api/monitoring/v3"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	customexternalmetrics "sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/translator"
	generatedopenapi "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/api/generated/openapi"
	gceconfig "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/config"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/logs"
	"k8s.io/klog"
	"sigs.k8s.io/metrics-server/pkg/api"

	corev1 "k8s.io/api/core/v1"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"

	coreadapter "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/coreprovider"
	adapter "github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter/pkg/adapter/provider"
)

const (
	defaultExternalMetricsCacheSize = 300
	// This is only metric info and likely to be much smaller than separate metrics of each kind above
	defaultMetricKindCacheSize = 25
)

// StackdriverAdapter is an adapter for Stackdriver
type StackdriverAdapter struct {
	basecmd.AdapterBase
}

type stackdriverAdapterServerOptions struct {
	// UseNewResourceModel is a flag that indicates whether new Stackdriver resource model should be used
	UseNewResourceModel bool
	// EnableCustomMetricsAPI switches on sample apiserver for Custom Metrics API
	EnableCustomMetricsAPI bool
	// EnableExternalMetricsAPI switches on sample apiserver for External Metrics API
	EnableExternalMetricsAPI bool
	// FallbackForContainerMetrics provides metrics from container when metric is not present in pod
	FallbackForContainerMetrics bool
	// EnableCoreMetricsAPI provides core metrics. Experimental, do not use.
	EnableCoreMetricsAPI bool
	// MetricsAddress is endpoint and port on which Prometheus metrics server should be enabled.
	MetricsAddress string
	// StackdriverEndpoint to change default Stackdriver endpoint (useful in sandbox).
	StackdriverEndpoint string
	// EnableDistributionSupport is a flag that indicates whether or not to allow distributions can
	// be used (with special reducer labels) in the adapter
	EnableDistributionSupport bool
	// ListFullCustomMetrics is a flag that whether list all pod custom metrics during api discovery.
	// Default = false, which only list 1 metric. Enabling this back would increase memory usage.
	ListFullCustomMetrics bool
	// ExternalMetricsCacheTTL specifies the cache expiration time for external metrics.
	ExternalMetricsCacheTTL time.Duration
	// ExternalMetricsCacheSize specifies the maximum number of stored entries in the external metric cache.
	ExternalMetricsCacheSize int
	// MetricKindCacheTTL specifies the cache expiration time for metric kind information.
	MetricKindCacheTTL time.Duration
	// MetricKindCacheSize specifies the maximum number of stored entries in the metric kind cache.
	MetricKindCacheSize int
}

func (sa *StackdriverAdapter) makeProviderOrDie(o *stackdriverAdapterServerOptions, rateInterval time.Duration, alignmentPeriod time.Duration) (provider.MetricsProvider, *translator.Translator) {
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
	if o.StackdriverEndpoint != "" {
		if !validateUrl(o.StackdriverEndpoint) {
			klog.Fatalf("Provided StackdriverEndpoint %v is not correct url", o.StackdriverEndpoint)
		}
		stackdriverService.BasePath = o.StackdriverEndpoint
	}

	gceConf, err := gceconfig.GetGceConfig()
	if err != nil {
		klog.Fatalf("Failed to retrieve GCE config: %v", err)
	}
	conf, err := sa.Config()
	if err != nil {
		klog.Fatalf("Unable to get StackdriverAdapter apiserver config %v", err)
	}
	conf.GenericConfig.EnableMetrics = true

	translator := translator.NewTranslator(stackdriverService, gceConf, rateInterval, alignmentPeriod, mapper, o.UseNewResourceModel, o.EnableDistributionSupport, o.MetricKindCacheSize, o.MetricKindCacheTTL)

	// If ListFullCustomMetrics is false, it returns one resource during api discovery `kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta2"` to reduce memory usage.
	customMetricsListCache := listStackdriverCustomMetrics(translator, o.ListFullCustomMetrics, o.FallbackForContainerMetrics)
	return adapter.NewStackdriverProvider(client, mapper, gceConf, stackdriverService, translator, rateInterval, o.UseNewResourceModel, o.FallbackForContainerMetrics, customMetricsListCache, o.ExternalMetricsCacheTTL, o.ExternalMetricsCacheSize), translator
}

func listStackdriverCustomMetrics(translator *translator.Translator, listFullCustomMetrics bool, fallbackForContainerMetrics bool) []provider.CustomMetricInfo {
	stackdriverRequest := translator.ListMetricDescriptors(fallbackForContainerMetrics)
	response, err := stackdriverRequest.Do()
	if err != nil {
		klog.Fatalf("Failed request to stackdriver api: %s", err)
	}
	customMetricsListCache := translator.GetMetricsFromSDDescriptorsResp(response)
	if !listFullCustomMetrics && len(customMetricsListCache) > 0 {
		customMetricsListCache = customMetricsListCache[0:1]
	}
	return customMetricsListCache
}

func (sa *StackdriverAdapter) withCoreMetrics(translator *translator.Translator) error {
	provider := coreadapter.NewCoreProvider(translator)
	informers, err := sa.Informers()
	if err != nil {
		return err
	}

	server, err := sa.Server()
	if err != nil {
		return err
	}

	podInformer, nil := informers.ForResource(corev1.SchemeGroupVersion.WithResource("pods"))
	if err != nil {
		return err
	}

	nodes := informers.Core().V1().Nodes()
	if err := api.Install(provider, podInformer.Lister(), nodes.Lister(), server.GenericAPIServer, []labels.Requirement{}); err != nil {
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

	if cmd.OpenAPIConfig == nil {
		cmd.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(api.Scheme, customexternalmetrics.Scheme))
		cmd.OpenAPIConfig.Info.Title = "custom-metrics-stackdriver-adapter"
		cmd.OpenAPIConfig.Info.Version = "1.0.0"
	}

	flags := cmd.Flags()

	flags.AddGoFlagSet(flag.CommandLine) // make sure we get the klog flags

	serverOptions := stackdriverAdapterServerOptions{
		UseNewResourceModel:         false,
		EnableCustomMetricsAPI:      true,
		EnableExternalMetricsAPI:    true,
		FallbackForContainerMetrics: false,
		EnableCoreMetricsAPI:        false,
		EnableDistributionSupport:   false,
		ListFullCustomMetrics:       false,
		ExternalMetricsCacheSize:    defaultExternalMetricsCacheSize,
		MetricKindCacheSize:         defaultMetricKindCacheSize,
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
	flags.BoolVar(&serverOptions.ListFullCustomMetrics, "list-full-custom-metrics", serverOptions.ListFullCustomMetrics,
		"whether to supporting list full custom metrics. This is a featuragate to list full custom metrics back, which should keep as false to return only 1 metric. Otherwise, it would have high memory usage issue.")
	flags.StringVar(&serverOptions.MetricsAddress, "metrics-address", "",
		"Endpoint with port on which Prometheus metrics server should be enabled. Example: localhost:8080. If there is no flag, Prometheus metric server is disabled and monitoring metrics are not collected.")
	flags.StringVar(&serverOptions.StackdriverEndpoint, "stackdriver-endpoint", "",
		"Stackdriver Endpoint used by adapter. Default is https://monitoring.googleapis.com/")
	flags.BoolVar(&serverOptions.EnableDistributionSupport, "enable-distribution-support", serverOptions.EnableDistributionSupport,
		"enables support for scaling based on distribution values")
	flags.DurationVar(&serverOptions.ExternalMetricsCacheTTL, "external-metric-cache-ttl", serverOptions.ExternalMetricsCacheTTL,
		"The duration (e.g., 1m, 5s) for which external metric values are cached.")
	flags.IntVar(&serverOptions.ExternalMetricsCacheSize, "external-metric-cache-size", serverOptions.ExternalMetricsCacheSize,
		"The maximum number of entries in the external metric cache.")
	flags.DurationVar(&serverOptions.MetricKindCacheTTL, "metric-kind-cache-ttl", serverOptions.MetricKindCacheTTL,
		"The duration (e.g., 1m, 5s) for which metric kind info calls are cached.")
	flags.IntVar(&serverOptions.MetricKindCacheSize, "metric-kind-cache-size", serverOptions.MetricKindCacheSize,
		"The maximum number of entries in the metric kind cache. Defaults to 0 (off). For clusters using a lot of stackdriver based auto-scaling; adjusting this value to equal the separate metric types used will help reduce internal api rate limits")

	flags.Parse(os.Args)

	klog.Info("serverOptions: ", serverOptions)
	if !serverOptions.UseNewResourceModel && serverOptions.FallbackForContainerMetrics {
		klog.Fatalf("Container metrics work only with new resource model")
	}
	if !serverOptions.UseNewResourceModel && serverOptions.EnableCoreMetricsAPI {
		klog.Fatalf("Core metrics work only with new resource model")
	}
	if serverOptions.ListFullCustomMetrics {
		klog.Infof("ListFullCustomMetrics is enabled, which would increase memory usage a lot. Please keep it as false, unless have to.")
	} else {
		klog.Infof("ListFullCustomMetrics is disabled, which would only list 1 metric resource to reduce memory usage. Add --list-full-custom-metrics to list full metric resources for debugging.")
	}

	// TODO(holubwicz): move duration config to server options
	metricsProvider, translator := cmd.makeProviderOrDie(&serverOptions, 5*time.Minute, 1*time.Minute)
	if serverOptions.EnableCustomMetricsAPI {
		cmd.WithCustomMetrics(metricsProvider)
	}
	if serverOptions.EnableExternalMetricsAPI {
		cmd.WithExternalMetrics(metricsProvider)
	}
	if serverOptions.EnableCoreMetricsAPI {
		if err := cmd.withCoreMetrics(translator); err != nil {
			klog.Fatalf("unable to install resource metrics API: %v", err)
		}
	}
	if serverOptions.MetricsAddress != "" {
		go runPrometheusMetricsServer(serverOptions.MetricsAddress)
	}
	if err := cmd.Run(wait.NeverStop); err != nil {
		klog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}

func runPrometheusMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(addr, nil)
	klog.Fatalf("Failed server: %s", err)
}

// validateUrl returns true if url is correct
func validateUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
