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

package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/translator"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsPrefix = flag.String("stackdriver-prefix", "container.googleapis.com/master",
		"Prefix that is appended to every metric.")
	autoWhitelistMetrics = flag.Bool("auto-whitelist-metrics", false,
		"If component has no whitelisted metrics, prometheus-to-sd will fetch them from Stackdriver.")
	metricDescriptorsResolution = flag.Duration("metric-descriptors-resolution", 10*time.Minute,
		"The resolution at which prometheus-to-sd will scrape metric descriptors from Stackdriver.")
	apioverride = flag.String("api-override", "",
		"The stackdriver API endpoint to override the default one used (which is prod).")
	source = flags.Uris{}
	podId  = flag.String("pod-id", "machine",
		"Name of the pod in which monitored component is running.")
	namespaceId = flag.String("namespace-id", "",
		"Namespace name of the pod in which monitored component is running.")
	zoneOverride = flag.String("zone-override", "",
		"Name of the zone to override the default one (in which component is running).")
	omitComponentName = flag.Bool("omit-component-name", true,
		"If metric name starts with the component name then this substring is removed to keep metric name shorter.")
	debugPort      = flag.Uint("port", 6061, "Port on which debug information is exposed.")
	dynamicSources = flags.Uris{}
	scrapeInterval = flag.Duration("scrape-interval", 60*time.Second,
		"The interval between metric scrapes. If there are multiple scrapes between two exports, the last present value is exported, even when missing from last scraping.")
	exportInterval = flag.Duration("export-interval", 60*time.Second,
		"The interval between metric exports. Can't be lower than --scrape-interval.")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Var(&source, "source", "source(s) to watch in [component-name]:http://host:port/path?whitelisted=a,b,c&podIdLabel=d&namespaceIdLabel=e&containerNameLabel=f format")
	flag.Var(&dynamicSources, "dynamic-source",
		`dynamic source(s) to watch in format: "[component-name]:http://:port/path?whitelisted=metric1,metric2&podIdLabel=label1&namespaceIdLabel=label2&containerNameLabel=label3". Dynamic sources are components (on the same node) discovered dynamically using the kubernetes api.`,
	)

	defer glog.Flush()
	flag.Parse()

	gceConf, err := config.GetGceConfig(*metricsPrefix, *zoneOverride)
	if err != nil {
		glog.Fatalf("Failed to get GCE config: %v", err)
	}
	glog.Infof("GCE config: %+v", gceConf)

	sourceConfigs := getSourceConfigs(gceConf)
	glog.Infof("Built the following source configs: %v", sourceConfigs)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		glog.Error(http.ListenAndServe(fmt.Sprintf(":%d", *debugPort), nil))
	}()

	client := oauth2.NewClient(context.Background(), google.ComputeTokenSource(""))
	stackdriverService, err := v3.New(client)
	if *apioverride != "" {
		stackdriverService.BasePath = *apioverride
	}
	if err != nil {
		glog.Fatalf("Failed to create Stackdriver client: %v", err)
	}
	glog.V(4).Infof("Successfully created Stackdriver client")

	if len(sourceConfigs) == 0 {
		glog.Fatalf("No sources defined. Please specify at least one --source flag.")
	}

	if *scrapeInterval > *exportInterval {
		glog.Fatalf("--scrape-interval cannot be bigger than --export-interval")
	}

	for _, sourceConfig := range sourceConfigs {
		glog.V(4).Infof("Starting goroutine for %+v", sourceConfig)

		// Pass sourceConfig as a parameter to avoid using the last sourceConfig by all goroutines.
		go readAndPushDataToStackdriver(stackdriverService, gceConf, sourceConfig)
	}

	// As worker goroutines work forever, block main thread as well.
	<-make(chan int)
}

func getSourceConfigs(gceConfig *config.GceConfig) []config.SourceConfig {
	glog.Info("Taking source configs from flags")
	staticSourceConfigs := config.SourceConfigsFromFlags(source, podId, namespaceId)
	glog.Info("Taking source configs from kubernetes api server")
	dynamicSourceConfigs, err := config.SourceConfigsFromDynamicSources(gceConfig, []flags.Uri(dynamicSources))
	if err != nil {
		glog.Fatalf(err.Error())
	}
	return append(staticSourceConfigs, dynamicSourceConfigs...)
}

func readAndPushDataToStackdriver(stackdriverService *v3.Service, gceConf *config.GceConfig, sourceConfig config.SourceConfig) {
	glog.Infof("Running prometheus-to-sd, monitored target is %s %v:%v", sourceConfig.Component, sourceConfig.Host, sourceConfig.Port)
	commonConfig := &config.CommonConfig{
		GceConfig:         gceConf,
		PodConfig:         sourceConfig.PodConfig,
		ComponentName:     sourceConfig.Component,
		OmitComponentName: *omitComponentName,
	}
	metricDescriptorCache := translator.NewMetricDescriptorCache(stackdriverService, commonConfig, sourceConfig.Component)
	signal := time.After(0)
	useWhitelistedMetricsAutodiscovery := *autoWhitelistMetrics && len(sourceConfig.Whitelisted) == 0
	timeSeriesBuilder := translator.NewTimeSeriesBuilder(commonConfig, &sourceConfig, metricDescriptorCache)
	exportTicker := time.Tick(*exportInterval)

	for range time.Tick(*scrapeInterval) {
		// Possibly exporting as a first thing, since errors down the
		// road will jump to next iteration of the loop.
		select {
		case <-exportTicker:
			ts, err := timeSeriesBuilder.Build()
			if err != nil {
				glog.Errorf("Could not build time series for component %v: %v", sourceConfig.Component, err)
			} else {
				translator.SendToStackdriver(stackdriverService, commonConfig, ts)
			}
		default:
		}

		// Mark cache at the beginning of each iteration as stale. Cache is considered refreshed only if during
		// current iteration there was successful call to Refresh function.
		metricDescriptorCache.MarkStale()
		glog.V(4).Infof("Scraping metrics of component %v", sourceConfig.Component)
		select {
		case <-signal:
			glog.V(4).Infof("Updating metrics cache for component %v", sourceConfig.Component)
			metricDescriptorCache.Refresh()
			if useWhitelistedMetricsAutodiscovery {
				sourceConfig.UpdateWhitelistedMetrics(metricDescriptorCache.GetMetricNames())
				glog.V(2).Infof("Autodiscovered whitelisted metrics for component %v: %v", commonConfig.ComponentName, sourceConfig.Whitelisted)
			}
			signal = time.After(*metricDescriptorsResolution)
		default:
		}
		if useWhitelistedMetricsAutodiscovery && len(sourceConfig.Whitelisted) == 0 {
			glog.V(4).Infof("Skipping %v component as there are no metric to expose.", sourceConfig.Component)
			continue
		}
		metrics, err := translator.GetPrometheusMetrics(&sourceConfig)
		if err != nil {
			glog.V(2).Infof("Error while getting Prometheus metrics %v for component %v", err, sourceConfig.Component)
			continue
		}
		timeSeriesBuilder.Update(metrics)
	}
}
