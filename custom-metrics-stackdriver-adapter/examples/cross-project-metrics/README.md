# Custom Metrics with Stackdriver

This is a minimal example showing how to use the stackdriver adapter to get metrics from
a different project than the one your cluster is running in. By default, the adapter
uses the credentials of the node machine. There are three approaches for granting permissions
to your other project:


1. Grant the default service account for your project access to `roles/monitoring.viewer` in
   your other project.

1. Create your GKE cluster with a service account, and grant the service account access to 
   `roles/monitoring.viewer` in your other project

1. (Recommended) Create a service a service account for the stackdriver-adapter, and mount
   the credentials into the pod. This is the method this example implements.


This example assumes you already have custom metrics set up in your cluster

## Create a service account with credentials

For this example, you will need to create a service account. See [Creating a service account](https://cloud.google.com/iam/docs/creating-managing-service-accounts)
for how to do that.


Next, you will need to create a key for that services account. See [Creating service account keys](https://cloud.google.com/iam/docs/creating-managing-service-account-keys#iam-service-account-keys-create-console) for how to do that.


Copy the key to this directory. The filename should be `key.json`


Add `roles/monitoring.viewer` to your service account in all projects that you want it to grab metrics from.

## Creating the kubernetes secret

Run the following command:

```
kubectl create secret generic custom-metrics-stackdriver-adapter --from-file=./key.json -n custom-metrics
```


## Apply the modified adapter

Run the following command:

```
kubectl apply -f modified-adapter.yml
```


## Picking a metric to scale on

Open `cross-project-hpa.yml`. You'll notice that the metrics are based on the number of undelivered messages on a
pubsub subscription called `my-subscription` in a different project called `my-other-project`. Modify these selectors
to match your environment and save the file. 


## Apply the deployment and hpa

Run the following commands:

```
kubectl apply -f deployment-to-be-scaled.yml
kubectl apply -f cross-project-hpa.yml
```


## Check that it's working

Run the following command:

```
kubectl describe hpa dummy
```

You should see that the hpa is able to detect the metric in your other project, and is scaling your deployment accordingly.


## Troubleshooting

There are several places where cross project autoscaling can get stuck, so when troubleshooting
make sure to check them one by one.

1. Check that your service account has the right permissions. It needs `roles/monitoring.viewer`


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
