# Custom Metrics with Stackdriver

This is a minimal example showing how to export custom metrics directly to Stackdriver
in a k8s cluster running on GCE or GKE. This example assumes that you already have
a [custom metrics setup] in your cluster.

## Stackdriver dummy exporter

A simple sd-dummy-exporter container exports a metric of constant value 
to Stackdriver in a loop. The metric name and value can be passed in via flags.
Pod id, pod name and namespace are passed to the container via downward API (see
[custom-metrics-sd deployment] for how it's done).

Stackdriver dummy exporter can export metrics for **new Stackdriver resource model**,
**legacy Stackdriver resource model** or both, specified by flags:
`--use-new-resource-model` and `--use-old-resource-model`. The deployment used in this
example exports metrics for both resource models at the same time.

## Horizontal Pod Autoscaling object

In the example, there is a [Horizontal Pod Autoscaler object] that can be used to scale the deployment on 
the exported metric. It is configured with a target value lower than the default metric value. This will
cause the deployment to scale up to 5 replicas over the course of ~15 minutes. 
If you do not see a successful scale up event for a longer period of time, there is 
likely an issue with your setup. See [Troubleshooting](#troubleshoooting) section for possible reasons.

## Deployment

The example can be used to test your custom metrics setup using Stackdriver. See the [custom metrics setup] instructions for more details.

Running 
```
kubectl create -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/examples/direct-to-sd/custom-metrics-sd.yaml
```
will create a custom-metric-sd deployment containing sd_dummy_exporter container.
Additionally it will create the HPA object to tell Kubernetes to scale
the deployment based on the exported metric. Default maximum number of pods is 5.

## Troubleshooting

There are several places where the metric flow can get stuck, so when troubleshooting
make sure to check them one by one.

1. Check that your metrics are exported to Stackdriver.

Go to the [Stackdriver UI](https://app.google.stackdriver.com/) and log into your account. 
Check the Resources > Metrics Explorer page and search for custom/<metric-name>. If you 
can't find the metric or there is no data, sd-dummy-exporter is not pushing your metrics to Stackdriver.

Make sure that:
* Your metric is correctly labeled with your GCP project id, zone or location and cluster name
* Your metric's
  * `resource_type` is one of: `k8s_pod`, `k8s_node` for **new resource model**
    or `gke_container` for **legacy resource model**.
  * name starts with ```custom.googleapis.com/``` prefix
* There are no errors when writing your metrics to Stackdriver.
```
kubectl logs custom-metric-sd
```
Search for "Failed to write time series data" in the logs

See [sd-dummy-exporter code] for an example of correctly labeled and exported metric.

2. Check that custom-metrics-stackdriver-adapter is able to read the metric from Stackdriver

Run kubeproxy 
```
kubectl proxy -p 8001
```
And check the available metrics
```
curl http://localhost:8001/apis/custom.metrics.k8s.io/v1beta1/
```
You should see a similar list:
```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "custom.metrics.k8s.io/v1beta1",
  "resources": [
    {
      "name": "*/<metric-name>",
      "singularName": "",
      "namespaced": true,
      "kind": "MetricValueList",
      "verbs": [
        "get"
      ]
    },
  ]
}
```
Note that the metric-name here does not contain the ```custom.googleapis.com/``` prefix.

If you do not see your metric in the list, custom-metrics-stackdriver-adapter
can't find the metric in Stackdriver.

Make sure that:
* Your `metricType = DOUBLE` or `INT64`
* Your metric name does not contain slashes after the ```custom.googleapis.com/``` prefix
See [sd-dummy-exporter code] for an example of correctly labeled and exported metric.

3. Check that metric timeseries is available for Horizontal Pod Autoscaler Controller

If you did find your metric in 2. but still see no scale up, check that the metric
values are available from custom-metrics-stackdriver-adapter.

Run kubeproxy 
```
kubectl proxy -p 8001
```
And check the metric
```
curl http://localhost:8001/apis/custom.metrics.k8s.io/v1beta1/namespaces/<namespace-name>/pods/*/<metric-name>
```

This should return a MetricValueList object. If you get a 404, the metric timeseries is not available from
custom-metrics-stackdriver-adapter.

Make sure that:
* your metric is correctly labeled with pod details (id, name and namespace).
For reference on correctly labeling your metric see [sd-dummy-exporter code] 
and [custom-metrics-sd deployment] for how to pass pod details to the sd-dummy-exporter
via downward API.

4. Check that the HPA object is configured correctly

Make sure that:
* the HPA config has the right scaling target
```yaml
scaleTargetRef:
    apiVersion: apps/v1beta1
    kind: Deployment
    name: <deployment-name>
```
* you specify a correct metric to scale on
```yaml
metrics:
  - type: Pods
    pods:
      metricName: <metric-name>
      targetAverageValue: <target-value>
```
Note that the metric-name here does not contain the ```custom.googleapis.com/``` prefix.

[Custom metrics - Stackdriver adapter]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter
[custom metrics setup]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter#configure-cluster
[Horizontal Pod Autoscaler object]:
https://github.com/kubernetes/community/blob/master/contributors/design-proposals/autoscaling/horizontal-pod-autoscaler.md#horizontalpodautoscaler-object
[sd-dummy-exporter code]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter/examples/direct-to-sd/sd_dummy_exporter.go
[custom-metrics-sd deployment]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter/examples/direct-to-sd/custom-metrics-sd.yaml
