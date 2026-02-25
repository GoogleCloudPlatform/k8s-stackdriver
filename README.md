## Google Cloud Operations integration for GKE

> :exclamation: **Most tools in this repository are not meant for end-users.**
> It contains source code for tools that are pre-installed in the GKE clusters.
> An exception to this is the [Custom Metrics - Stackdriver Adapter](custom-metrics-stackdriver-adapter),
> which is meant for [end-user consumption][stackdriverAdapter].

[Google Cloud Operations suite][cloudOperationsSite] (fka Stackdriver) provides advanced 
monitoring and logging solution that will allow you to get more
insights into your Kubernetes clusters. If you are a Google
Kubernetes Engine (GKE) user, you get [integration][k8sMonitoring]
with Cloud Monitoring and Logging out of the box.

[k8sMonitoring]: https://cloud.google.com/kubernetes-engine-monitoring
[cloudOperationsSite]: https://cloud.google.com/products/operations 
[stackdriverAdapter]: https://cloud.google.com/kubernetes-engine/docs/tutorials/autoscaling-metrics
