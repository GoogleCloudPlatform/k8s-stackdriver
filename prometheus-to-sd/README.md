# Overview

prometheus-to-sd is a simple component that can scrape metrics stored in
[prometheus text format](https://prometheus.io/docs/instrumenting/exposition_formats/)
from one or multiple components and push them to the Stackdriver. Main requirement:
k8s cluster should run on GCE or GKE.

## Container Image

Look at the following link: https://gcr.io/google-containers/prometheus-to-sd and pick the latest image.

# Usage

For scraping metrics from the component it's name, host, port and metrics should passed
through the flag `source` in the next format:
`component-name:http://host:port?whitelisted=a,b,c`. If whitelisted part is
omitted, then all metrics that are scraped from the component will be pushed
to the Stackdriver.

## Custom metrics

To be able to push custom metrics to the Stackdriver flag `stackdriver-prefix=custom.googleapis.com`
has to be specified. In such case metric `bar` from the component
`foo` is going to be pushed to the Stackdriver as `custom.googleapis.com/foo/bar`.

## Metrics autodiscovery

If metric descriptors already exist on the Stackdriver (created manually or by different component)
then autodiscovery feature could be used. In such case prometheus-to-sd will push metrics for
which metric descriptors are available on the Stackdriver. To use this feature a flag
`auto-whitelist-metrics=true` has to be passed.

## Resource descriptor

Each pushed metric includes [monitored resource
descriptor](https://cloud.google.com/logging/docs/api/v2/resource-list#resource-types). Fields, such as
`project_id`, `cluster_name`, `instance_id` and `zone` are filled automatically by
the prometheus-to-sd. Values of the `namespace_id` and `pod_id` can be passed to
the component through the additional flags or omitted. `container_name` is
always empty for now. Field `zone` is overridable via flag.

## Scrape interval vs. export interval

There are two flags: `scrape-interval` and `export-interval` that allow
specifying how often metrics are read from the sources and how often they are
exported to Stackdriver, respectively. By default both are set to 1m. The
scrapes can be more frequent than exports, however, to achieve grater precision
for metrics being exported. For example, if metrics are exported once every
minute and a container dies between scrapes, up to 1 minutes of metrics can be
lost. Frequent scrapes mitigate that, at the cost of elevated resource usage.

## Deployment

Example of [deployment](https://github.com/GoogleCloudPlatform/k8s-stackdriver/blob/master/prometheus-to-sd/kubernetes/prometheus-to-sd-kube-state-metrics.yaml)
used to monitor
[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) component, that is used to collect
different metrics about the state of k8s cluster.

## Container monitoring agents

Container monitoring agents, such as [cAdvisor](https://github.com/google/cadvisor#cadvisor), 
collect metrics about containers other than itself.  Monitoring agents use prometheus
labels for the namespace, pod, and container to differentiate metrics from different
containers.  To use these prometheus labels as the monitored resource labels in
stackdriver, include `namespaceIdLabel`, `podIdLabel`, and `containerNameLabel` in the 
`source` flag in this format: 
`component-name:http://host:port?whitelisted=a,b,c&namespaceIdLabel=d&podIdLabel=e&containerNameLabel=f`.
Note that prometheus labels used for the monitored resource are not included as labels in stackdriver.

For example, if prom-to-sd scraped the prometheus metric: 
`container_cpu_usage_seconds{d="my-namespace",e="abc123",f="my-app",g="production"} 1.02030405e+09`
It would be displayed in stackdriver with the namespace, `my-namespace`, the pod id, `abc123`, and the container name, `my-app`.  The only stackdriver label for this metric would be `g="production"`.

## Alternatives

Google develops **prometheus-to-sd** primarily for Google Kubernetes Engine to collect metrics from system services in order to support Kubernetes users. We designed the tool to be lean when deployed as a sidecar in your pod. It's intended to support only the metrics the Kubernetes team at Google needs; you can use it to monitor your applications, but the feature set is limited.

Google develops [**stackdriver-prometheus**](https://github.com/Stackdriver/stackdriver-prometheus) primarily for Stackdriver users and gives support to Stackdriver users. We designed the user experience to meet the expectations of Prometheus users and to make it easy to run with Prometheus server. stackdriver-prometheus is intended to monitor all your applications, Kubernetes and beyond.
