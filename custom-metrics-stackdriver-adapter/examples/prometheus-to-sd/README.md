# Custom Metrics with Prometheus and Stackdriver

This is a minimal example showing how to export custom metrics using 
[prometheus text format](https://prometheus.io/docs/instrumenting/exposition_formats/) to Stackdriver
in a k8s cluster running on GCE or GKE. This example assumes that you already have
a [custom metrics setup] in your cluster.

## Deploy a sample application

### Option1. Prometheus dummy exporter

A simple prometheus-dummy-exporter component exposes a single Prometheus metric 
with a constant value. The metric name and value can be passed in via flags.
There is also a flag to configure the port on which the metrics are served. 

#### Setup

The example can be used to test your custom metrics setup using Stackdriver. See the [custom metrics setup] instructions for more details.

Running 
```
kubectl create -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd/custom-metrics-prometheus-sd.yaml 
```
will create a custom-metric-prometheus-sd deployment containing prometheus_dummy_exporter container and a sidecar prometheus-to-sd container.
Additionally, it will create the HPA object to tell Kubernetes to scale the deployment based on the exported metric. The default maximum number of
pods is 5.

#### Prometheus to Stackdriver sidecar container

Prometheus Dummy exporter is deployed with a sidecar [Prometheus to Stackdriver] container
configured to scrape exposed metric from the specified port and write it to Stackdriver. 
[Custom metrics - Stackdriver adapter] then reads this metric from Stackdriver 
and makes the metric available for scaling.

#### prometheus-to-sd flags
Note important configuration details for prometheus-to-sd
```yaml	
command:
  - /monitor
  - --source=:http://localhost:8080  # Empty component name. Needed to avoid slashes in the custom metric name. Port number has to match the one configured for prometheus_dummy_exporter.
  - --stackdriver-prefix=custom.googleapis.com   # Prefix needed for custom metrics
  # pod-id and namespace-id are needed for the metrics exported to Stackdriver
  # to be read correctly by custom-metrics-stackdriver-adapter.
  - --pod-id=$(POD_ID)
  - --namespace-id=$(POD_NAMESPACE)
env:
  - name: POD_ID
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.uid
  - name: POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
```


### Option 2. Exporting metrics to Google Cloud Managed Service for Prometheus

Alternatively, you can use [Google Cloud Managed Service for Prometheus](https://cloud.google.com/managed-prometheus) to manage your metrics.

#### Background

If you would like to learn more about Google Cloud Managed Service for Prometheus, you can check out the [documentation](https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-managed) to get yourself familiar with it.

#### Setup

For detailed setup steps, you can refer to the documentation [here](https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-managed#config-mgd-collection).

## Horizontal Pod Autoscaling object

In the example, there is a [Horizontal Pod Autoscaler object](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) configured to scale the deployment on the exported metric. This will
cause the deployment to scale up to 5 replicas for ~15 minutes. If you do not see a successful scale-up event for a longer period, there is likely an issue with your setup. See the [Troubleshooting](#troubleshoooting) section for possible reasons.

- If you want to use metrics from the Prometheus dummy exporter you deployed, you can directly use the metric name in the YAML file, such as `foo` in the example, the stackdriver adapter will interpret it `custom.googleapis.com|foo` by default.
- If you want to use Google Cloud Managed Service for Prometheus metrics, you will need to provide the metric's full name from the [Metrics Explorer](https://console.cloud.google.com/monitoring/metrics-explorer) in Google Cloud Console's monitoring page, in this case, it will be `prometheus.googleapis.com|foo|gauge`.


## Troubleshooting

There are several places where the metric flow can get stuck, so when troubleshooting
make sure to check them one by one.

1. Check that your metrics are exported to Stackdriver.

Go to the [Stackdriver UI](https://app.google.stackdriver.com/) and log into your account. 
Check the Resources > Metrics Explorer page and search for custom/<metric-name>. If you 
can't find the metric or there is no data, prometheus-to-sd is not pushing your metrics to Stackdriver.

Make sure that:
* --source flag is in a corect format:
	* component name should be empty, but there shoud be a leading colon
  * the port number has to match the port on which metrics are exposed in 
* --stackdriver-prefix=custom.googleapis.com is present in prometheus-to-sd flags

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
      "name": "pods/<metric-name>",
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
Alternatively, you can directly test against custom-metric-stackdriver-adapter by running the command:
```sh
kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/prometheus.googleapis.com|foo|gauge" | jq .
```

If you do not see your metric in the list, custom-metrics-stackdriver-adapter
can't find the metric in Stackdriver.

Make sure that:
* Your metric meets following requirements:
  * `metricKind = GAUGE`
  * `metricType = DOUBLE` or `INT64`
* Your metric name does not contain slashes
See [prometheus-dummy-exporter code] for an example metric definition in go.

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
* pod-id and namespace-id are passed in correctly to prometheus-to-sd container. See [example](#prometheus-to-sd-flags)

4. Check that the HPA object is configured correctly

Make sure that:
* the HPA config has the right scaling target
```yaml
scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: <deployment-name>
```
* you specify a correct metric to scale on
```yaml
metrics:
  - type: Pods
    pods:
      metric:
        name: <metric-name>
      target:
        type: AverageValue
        averageValue:  <target-value>
```


[Prometheus to Stackdriver]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/prometheus-to-sd
[Custom metrics - Stackdriver adapter]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter
[custom metrics setup]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter#configure-cluster
[Horizontal Pod Autoscaler object]:
https://github.com/kubernetes/community/blob/master/contributors/design-proposals/autoscaling/horizontal-pod-autoscaler.md#horizontalpodautoscaler-object
[prometheus-dummy-exporter code]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd/prometheus_dummy_exporter.go
