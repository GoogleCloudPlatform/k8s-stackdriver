# Custom Metrics - Stackdriver Adapter

Custom Metrics - Stackdriver Adapter is an implementation of [Custom Metrics
API] and [External Metrics API] using Stackdriver as a backend. Its purpose is
to enable pod autoscaling based on Stackdriver custom metrics.

## Usage guide

This guide shows how to set up Custom Metrics - Stackdriver Adapter and export
metrics to Stackdriver in a compatible way. Once this is done, you can use
them to scale your application, following [HPA walkthrough].

### Configure cluster

1. Create Kubernetes cluster or use existing one, see [cluster setup].
   Requirements:

   * Kubernetes version 1.8.1 or newer running on GKE or GCE

   * Monitoring scope `monitoring` set up on cluster nodes. **It is enabled by
     default, so you should not have to do anything**. See also [OAuth 2.0 API
     Scopes] to learn more about authentication scopes.

     You can use following commands to verify that the scopes are set correctly:
     - For GKE cluster `<my_cluster>`, use following command:
       ```
       gcloud container clusters describe <my_cluster>
       ```
       For each node pool check the section `oauthScopes` - there should be
       `https://www.googleapis.com/auth/monitoring` scope listed there.
     - For a GCE instance `<my_instance>` use following command:
       ```
       gcloud compute instances describe <my_instance>
       ```
       `https://www.googleapis.com/auth/monitoring` should be listed in the
       `scopes` section.


     To configure set scopes manually, you can use:
     - `--scopes` flag if you are using `gcloud container clusters create`
       command, see [gcloud
       documentation](https://cloud.google.com/sdk/gcloud/reference/container/clusters/create).
     - Environment variable `NODE_SCOPES` if you are using [kube-up.sh script].
       It is enabled by default.
     - To set scopes in existing clusters you can use `gcloud beta compute
       instances set-scopes` command, see [gcloud
       documentation](https://cloud.google.com/sdk/gcloud/reference/beta/compute/instances/set-scopes).
    * On GKE, you need cluster-admin permissions on your cluster. You can grant
      your user account these permissions with following command:
      ```
      kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user $(gcloud config get-value account)
      ```

1. Start *Custom Metrics - Stackdriver Adapter*.

Stackdriver supports two models of Kubernetes resources: **the legacy model**
using monitored resource `gke_container` and **the new model** using different
Kubernetes monitored resources, including for example `k8s_pod`, `k8s_node`. See
[monitored resources documentation] for more details.

* If you use **legacy resource model**:
  ```sh
  kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/deploy/production/adapter.yaml
  ```
* If you use **new resource model**:
  ```sh
  kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-stackdriver/master/custom-metrics-stackdriver-adapter/deploy/production/adapter_new_resource_model.yaml
  ```

### Metrics available from Stackdriver

Custom Metrics - Stackdriver Adapter exposes Stackdriver metrics to Kubernetes
components via two APIs.

1. Any Stackdriver metric can be retrieved via **External Metrics API** with one
   assumption: `metricType = DOUBLE` or `INT64`. For example, this API can be
   used to configure Horizontal Pod Autoscaler to scale deployment based on any
   of [existing metrics from other GCP services].

1. Metrics attached to Kubernetes objects, such as Pod or Node, can be retrieved
   via **Custom Metrics API**. The following section provides more details about
   exporting such metrics.

#### Metric kinds

Stackdriver specifies three metric kinds, all of which are supported by Custom
Metrics - Stackdriver Adapter:
1. `GAUGE` - Each data point represents an instantaneous measurement, for
   example the temperature. The adapter exposes the latest value.
1. `DELTA` - Each data point represents the change in a value over the time
   interval. The adapter exposes *rate* of the metric - the metric change per
   second computed over last 5 minutes.
1. `CUMULATIVE` - Each data point is a value being accumulated over time. The
   adapter exposes *rate* of the metric - the metric change per second computed
   over last 5 minutes.

### Export custom metrics to Stackdriver

To learn how to create your custom metric and write your data to Stackdriver,
follow [Stackdriver custom metrics documentation]. You can also follow
[Prometheus to Stackdriver documentation] to export metrics exposed by your pods
in Prometheus format.

The name of your metric must start with custom.googleapis.com/ prefix followed
by a simple name, as defined in [custom metric naming rules].

You will report your metric against a appropriate monitored resource for Kubernetes
objects. To use **legacy resource model**, use monitored resource `gke_container`.
To use **new resource model**, use one of monitored resources: `k8s_pod` or
`k8s_node` - corresponding to Kubernetes objects `Pod` and `Node`.

1. Define your custom metric by following [Stackdriver custom metrics documentation].
   Your metric descriptor needs to meet following requirements:
   * `metricType = DOUBLE` or `INT64`
1. Export metric from your application. The metric has to be associated with a
   specific pod or node and meet folowing requirements:
   * `resource_type` set accordingly: `gke_container` for **legacy resource
     model** or one of `k8s_pod`, `k8s_node` for **new resource model**. (See
     [monitored resources documentation])
   * All resource labels for your monitored resource set to correct values. In
     particular:
     - `pod_id`, `pod_name`, `namespace_name` can be obtained via downward API.
       Example configuration that passes these values to your application as flags:

       ```yaml
       apiVersion: v1
       kind: Pod
       metadata:
         name: my-pod
       spec:
         containers:
         - image: <my-image>
           command:
           - my-app --pod_id=$(POD_ID) --pod_name=$(POD_NAME) --namespace_name=$(NAMESPACE_NAME)
           env:
           - name: POD_ID
             valueFrom:
               fieldRef:
                 fieldPath: metadata.uid
           - name: POD_NAME
             valueFrom:
               fieldRef:
                 fieldPath: metadata.name
           - name: NAMESPACE_NAME
             valueFrom:
               fieldRef:
                 fieldPath: metadata.namespace
       ```

       Example flag definition in Go:
       ```go
       import "flag"

       podIdFlag := flag.String("pod_id", "", "a string")
       flag.Parse()
       podID := *podIdFlag
       ```

     - For monitored resource `gke_container`, `container_name` should be set to
       `""` to indicate that the metric is associated with a pod, not a
       particular container.
     - `project_id`, `zone`, `location`, `cluster_name` - can be obtained by your
       application from [metadata server]. You can use Google Cloud compute
       metadata client to get these values, example in Go:

       ```go
       import gce "cloud.google.com/go/compute/metadata"

       project_id, err := gce.ProjectID()
       // the zone where your application runs
       // used in legacy resource model
       zone, err := gce.Zone()
       // cluster location can be different than your application zone in
       // clusters spanning across multiple zones
       // used in new resource model
       location, err := gce.InstanceAttributeValue("cluster-location")
       cluster_name, err := gce.InstanceAttributeValue("cluster-name")
       ```
     - `namespace_id` and `instance_id` (for **legacy resource model**) are not
       used by Custom Metrics - Stackdriver Adapter, but still it's recommended
       to set those to the correct values to make them more useful for other use
       cases.

     Example code exporting a metric to Stackdriver, written in Go:

     ```go
     import (
       "context"
       "time"
       "golang.org/x/oauth2"
       "golang.org/x/oauth2/google"
       "google.golang.org/api/monitoring/v3"
     )

     // Create stackdriver client
     authClient := oauth2.NewClient(context.Background(), google.ComputeTokenSource(""))
     stackdriverService, err := v3.New(oauthClient)
     if err != nil {
       return
     }

     // Define metric time series filling in all required fields
     request := &v3.CreateTimeSeriesRequest{
       TimeSeries: []*v3.TimeSeries{
         {
           Metric: &v3.Metric{
             Type: "custom.googleapis.com/" + <your metric name>,
           },
           Resource: &v3.MonitoredResource{
             Type: "k8s_pod",
             Labels: map[string]string{
               "project_id":     <your project ID>,
               "location":       <your cluster location>,
               "cluster_name":   <your cluster name>,
               "namespace_name": <your namespace>,
               "pod_name":       <your pod name>,
             },
           },
           Points: []*v3.Point{
             &v3.Point{
               Interval: &v3.TimeInterval{
                 EndTime: time.Now().Format(time.RFC3339),
               },
               Value: &v3.TypedValue{
                 Int64Value: <your metric value>,
               },
             }
           },
         },
       },
     }
     stackdriverService.Projects.TimeSeries.Create("projects/<your project ID>", request).Do()
     ```

### Examples

To test your custom metrics setup or see a reference on how to push your metrics
to Stackdriver, check out our examples:
* application that exports custom metric directly to Stackdriver filling in all
  required labels: [direct-example]
* application that exports custom metrics to Stackdriver using [Prometheus text format] 
  and [Prometheus to Stackdriver] adapter: [prometheus-to-sd-example]

[Custom Metrics API]:
https://github.com/kubernetes/metrics/tree/master/pkg/apis/custom_metrics
[External Metrics API]:
https://github.com/kubernetes/metrics/tree/master/pkg/apis/external_metrics
[HPA walkthrough]:
https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/
[cluster setup]: https://kubernetes.io/docs/setup/
[Stackdriver custom metrics documentation]:
https://cloud.google.com/monitoring/custom-metrics/creating-metrics
[Prometheus to stackdriver documentation]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/prometheus-to-sd
[custom metric naming rules]:
https://github.com/kubernetes/community/blob/master/contributors/design-proposals/instrumentation/custom-metrics-api.md#metric-names
[direct-example]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/blob/master/custom-metrics-stackdriver-adapter/examples/direct-to-sd
[prometheus-to-sd-example]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/blob/master/custom-metrics-stackdriver-adapter/examples/prometheus-to-sd
[OAuth 2.0 API Scopes]:
https://developers.google.com/identity/protocols/googlescopes
[kube-up.sh script]:
https://github.com/kubernetes/kubernetes/blob/master/cluster/kube-up.sh
[monitored resources documentation]:
https://cloud.google.com/monitoring/api/resources
[Prometheus to Stackdriver]:
https://github.com/GoogleCloudPlatform/k8s-stackdriver/tree/master/prometheus-to-sd
[Prometheus text format]:
https://prometheus.io/docs/instrumenting/exposition_formats
[existing metrics from other GCP services]:
https://cloud.google.com/monitoring/api/metrics_gcp
