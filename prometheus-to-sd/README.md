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
always empty for now.

## Deployment

Example of [deployment](https://github.com/GoogleCloudPlatform/k8s-stackdriver/blob/master/prometheus-to-sd/kubernetes/prometheus-to-sd-kube-state-metrics.yaml)
used to monitor
[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) component, that is used to collect
different metrics about the state of k8s cluster.
