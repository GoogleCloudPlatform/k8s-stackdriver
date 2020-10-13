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
	"context"
	"expvar"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	v3 "google.golang.org/api/monitoring/v3"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/flags"
	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/translator"
)

var (
	metricsPrefix = flag.String("stackdriver-prefix", "container.googleapis.com/master",
		"Prefix that is appended to every metric. Could be rewritten by metricsPrefix in per "+
			"component configuration.")
	autoWhitelistMetrics = flag.Bool("auto-whitelist-metrics", false,
		"If component has no whitelisted metrics, prometheus-to-sd will fetch them from Stackdriver.")
	metricDescriptorsResolution = flag.Duration("metric-descriptors-resolution", 10*time.Minute,
		"The resolution at which prometheus-to-sd will scrape metric descriptors from Stackdriver.")
	apioverride = flag.String("api-override", "",
		"The stackdriver API endpoint to override the default one used (which is prod).")
	source = flags.Uris{}
	podId  = flag.String("pod-id", "machine",
		"Name of the pod in which monitored component is running.")
	nodeOverride = flag.String("node-name", "",
		"Node name to use. If not set, defaults to value from GCE Metadata Server.")
	namespaceId = flag.String("namespace-id", "",
		"Namespace name of the pod in which monitored component is running.")
	zoneOverride = flag.String("zone-override", "",
		"Name of the zone to override the default one (in which component is running).")
	clusterNameOverride = flag.String("cluster-name", "",
		"Cluster name to use. If not set, defaults to value from GCE Metadata Server.")
	clusterLocationOverride = flag.String("cluster-location", "",
		"Cluster location to use. If not set, defaults to value from GCE Metadata Server.")
	projectOverride = flag.String("project-id", "",
		"GCP project to send metrics to. If not set, defaults to value from GCE Metadata Server.")
	monitoredResourceTypePrefix = flag.String("monitored-resource-type-prefix", "", "MonitoredResource type prefix, to be appended by 'container', 'pod', and 'node'.")
	monitoredResourceLabels     = flag.String("monitored-resource-labels", "", "Manually specified MonitoredResource labels. "+
		"It is in URL parameter format, like 'A=B&C=D&E=F'. "+
		"When this field is specified, 'monitored-resource-type-prefix' is also required. "+
		"By default, Prometheus-to-sd will read from GCE metadata server to fetch project id, cluster name, cluster location, and instance id. So these fields are optional in this flag. "+
		"If these values are specified in this flag, they will overwrite the value from GCE metadata server. "+
		"To note: 'namespace-name', 'pod-name', and 'container-name' should not be provided in this flag and they will always be overwritten by values in other command line flags.")
	omitComponentName = flag.Bool("omit-component-name", true,
		"If metric name starts with the component name then this substring is removed to keep metric name shorter.")
	metricsPort    = flag.Uint("port", 6061, "Port on which metrics are exposed.")
	listenAddress  = flag.String("listen-address", "", "Interface on which  metrics are exposed.")
	debugPort      = flag.Uint("debug-port", 16061, "Port on which debug information is exposed.")
	debugAddress   = flag.String("debug-address", "localhost", "Interface on which debug information is exposed.")
	dynamicSources = flags.Uris{}
	scrapeInterval = flag.Duration("scrape-interval", 60*time.Second,
		"The interval between metric scrapes. If there are multiple scrapes between two exports, the last present value is exported, even when missing from last scraping.")
	exportInterval = flag.Duration("export-interval", 60*time.Second,
		"The interval between metric exports. Can't be lower than --scrape-interval.")
	downcaseMetricNames = flag.Bool("downcase-metric-names", false,
		"If enabled, will downcase all metric names.")
	delayedShutdownTimeout = flag.Duration("delayed-shutdown-timeout", 120*time.Second,
		"Time to wait for the shutdown after receiving SIGTERM. 0 value means shutdown immediately, negative value results in ignoring signal."+
			" Default value is 120 seconds.")
	gceTokenURL  = flag.String("gce-token-url", "", "URL to be used to obtain GCE access token")
	gceTokenBody = flag.String("gce-token-body", "", "HTTP request body to be used to obtain GCE access token")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Var(&source, "source", "source(s) to watch in [component-name]:[http|https]://host:port/path?whitelisted=a,b,c&podIdLabel=d&namespaceIdLabel=e&containerNameLabel=f&metricsPrefix=prefix&authToken=token&authUsername=user&authPassword=password format. Can be specified multiple times")
	flag.Var(&dynamicSources, "dynamic-source",
		`dynamic source(s) to watch in format: "[component-name]:[http|https]://:port/path?whitelisted=metric1,metric2&podIdLabel=label1&namespaceIdLabel=label2&containerNameLabel=label3&metricsPrefix=prefix&authToken=token&authUsername=user&authPassword=password". Dynamic sources are components (on the same node) discovered dynamically using the kubernetes api.`,
	)

	defer glog.Flush()
	flag.Parse()

	if *delayedShutdownTimeout < 0 {
		signal.Ignore(syscall.SIGTERM)
	} else {
		sigTermChannel := make(chan os.Signal, 1)
		signal.Notify(sigTermChannel, syscall.SIGTERM)

		go func() {
			<-sigTermChannel
			glog.Infof("SIGTERM has been received, Waiting %s before the shutdown.", delayedShutdownTimeout.String())

			time.Sleep(*delayedShutdownTimeout)
			glog.Info("Shutting down after receiving SIGTERM.")
			os.Exit(0)
		}()
	}

	gceConf, err := config.GetGceConfig(*projectOverride, *clusterNameOverride, *clusterLocationOverride, *zoneOverride, *nodeOverride)
	if err != nil {
		glog.Fatalf("Failed to get GCE config: %v", err)
	}
	glog.Infof("GCE config: %+v", gceConf)

	sourceConfigs := getSourceConfigs(*metricsPrefix, gceConf)
	glog.Infof("Built the following source configs: %v", sourceConfigs)

	monitoredResourceLabels := parseMonitoredResourceLabels(*monitoredResourceLabels)
	if len(monitoredResourceLabels) > 0 {
		if *monitoredResourceTypePrefix == "" {
			glog.Fatalf("When 'monitored-resource-labels' is specified, 'monitored-resource-type-prefix' cannot be empty.")
		}
		glog.Infof("Monitored resource labels: %v", monitoredResourceLabels)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		glog.Error(http.ListenAndServe(fmt.Sprintf("%s:%d", *listenAddress, *metricsPort), nil))
	}()

	go func() {
		glog.Error(http.ListenAndServe(fmt.Sprintf("%s:%d", *debugAddress, *debugPort), expvar.Handler()))
	}()

	var client *http.Client

	if *gceTokenURL != "" {
		client = oauth2.NewClient(context.Background(), config.NewAltTokenSource(*gceTokenURL, *gceTokenBody))
	} else if *projectOverride != "" {
		client, err = google.DefaultClient(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			glog.Fatalf("Error getting default credentials: %v", err)
		}
		glog.Infof("Created a client with the default credentials")
	} else {
		ts, err := google.DefaultTokenSource(context.Background(), "")
		if err != nil {
			glog.Fatalf("Error creating default token source: %v", err)
		}
		client = oauth2.NewClient(context.Background(), ts)
	}

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
		go readAndPushDataToStackdriver(stackdriverService, gceConf, sourceConfig, monitoredResourceLabels, *monitoredResourceTypePrefix)
	}

	// As worker goroutines work forever, block main thread as well.
	<-make(chan int)
}

func getSourceConfigs(defaultMetricsPrefix string, gceConfig *config.GceConfig) []*config.SourceConfig {
	glog.Infof("Taking source configs from flags")
	staticSourceConfigs := config.SourceConfigsFromFlags(source, podId, namespaceId, defaultMetricsPrefix)
	glog.Info("Taking source configs from kubernetes api server")
	dynamicSourceConfigs, err := config.SourceConfigsFromDynamicSources(gceConfig, []flags.Uri(dynamicSources))
	if err != nil {
		glog.Fatalf(err.Error())
	}
	return append(staticSourceConfigs, dynamicSourceConfigs...)
}

func readAndPushDataToStackdriver(stackdriverService *v3.Service, gceConf *config.GceConfig, sourceConfig *config.SourceConfig, monitoredResourceLabels map[string]string, prefix string) {
	glog.Infof("Running prometheus-to-sd, monitored target is %s %s://%v:%v", sourceConfig.Component, sourceConfig.Protocol, sourceConfig.Host, sourceConfig.Port)
	commonConfig := &config.CommonConfig{
		GceConfig:                   gceConf,
		SourceConfig:                sourceConfig,
		OmitComponentName:           *omitComponentName,
		DowncaseMetricNames:         *downcaseMetricNames,
		MonitoredResourceLabels:     monitoredResourceLabels,
		MonitoredResourceTypePrefix: prefix,
	}
	metricDescriptorCache := translator.NewMetricDescriptorCache(stackdriverService, commonConfig)
	signal := time.After(0)
	useWhitelistedMetricsAutodiscovery := *autoWhitelistMetrics && len(sourceConfig.Whitelisted) == 0
	timeSeriesBuilder := translator.NewTimeSeriesBuilder(commonConfig, metricDescriptorCache)
	exportTicker := time.Tick(*exportInterval)

	for range time.Tick(*scrapeInterval) {
		// Possibly exporting as a first thing, since errors down the
		// road will jump to next iteration of the loop.
		select {
		case <-exportTicker:
			ts, scrapeTimestamp, err := timeSeriesBuilder.Build()
			// Mark cache as stale at the first export attempt after each refresh. Cache is considered refreshed only if after
			// previous export there was successful call to Refresh function.
			metricDescriptorCache.MarkStale()
			if err != nil {
				glog.Errorf("Could not build time series for component %v: %v", sourceConfig.Component, err)
			} else {
				translator.SendToStackdriver(stackdriverService, commonConfig, ts, scrapeTimestamp)
			}
		default:
		}

		glog.V(4).Infof("Scraping metrics of component %v", sourceConfig.Component)
		select {
		case <-signal:
			glog.V(4).Infof("Updating metrics cache for component %v", sourceConfig.Component)
			metricDescriptorCache.Refresh()
			if useWhitelistedMetricsAutodiscovery {
				sourceConfig.UpdateWhitelistedMetrics(metricDescriptorCache.GetMetricNames())
				glog.V(2).Infof("Autodiscovered whitelisted metrics for component %v: %v", sourceConfig.Component, sourceConfig.Whitelisted)
			}
			signal = time.After(*metricDescriptorsResolution)
		default:
		}
		if useWhitelistedMetricsAutodiscovery && len(sourceConfig.Whitelisted) == 0 {
			glog.V(4).Infof("Skipping %v component as there are no metric to expose.", sourceConfig.Component)
			continue
		}
		scrapeTimestamp := time.Now()
		metrics, err := translator.GetPrometheusMetrics(sourceConfig)
		if err != nil {
			glog.V(2).Infof("Error while getting Prometheus metrics %v for component %v", err, sourceConfig.Component)
			continue
		}
		timeSeriesBuilder.Update(metrics, scrapeTimestamp)
	}
}

func parseMonitoredResourceLabels(monitoredResourceLabelsStr string) map[string]string {
	labels := make(map[string]string)
	m, err := url.ParseQuery(monitoredResourceLabelsStr)
	if err != nil {
		glog.Fatalf("Error parsing 'monitored-resource-labels' field: '%v', with error message: '%s'.", monitoredResourceLabelsStr, err)
	}
	for k, v := range m {
		if len(v) != 1 {
			glog.Fatalf("Key '%v' in 'monitored-resource-labels' doesn't have exactly one value (it has '%v' now).", k, v)
		}
		labels[k] = v[0]
	}
	return labels
}
