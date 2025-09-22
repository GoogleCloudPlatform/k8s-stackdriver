# Fetching Metrics from a Different Google Cloud Project

This adapter supports fetching metrics from a Google Cloud project different from the one where the adapter is running. This is useful in scenarios where metrics are stored in a central project, but the Horizontal Pod Autoscaler (HPA) is defined in a cluster running in a different project.

## Configuration

To fetch a metric from a different project, you need to specify the target project ID using the label `resource.labels.metrics_host_project_id` within the metric selector in your HPA definition. This applies to all supported metric types: `Pods`, `Object`, and `External`.

The label should be added to the `matchLabels` section of the `metric.selector` .

### Examples

Below are examples for each metric type:

**1. Pods Metric:**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: pods-metric-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metric:
        name: my-pod-metric
        selector:
          matchLabels:
            "resource.labels.metrics_host_project_id": "my-metrics-project"
      target:
        type: AverageValue
        averageValue: 10
```

**2. Object Metric:**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: object-metric-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Object
    object:
      metric:
        name: my-object-metric
        selector:
          matchLabels:
            "resource.labels.metrics_host_project_id": "my-metrics-project"
      describedObject:
        apiVersion: v1
        kind: Service
        name: my-service
      target:
        type: Value
        value: 100
```

**3. External Metric:**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: external-metric-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: External
    external:
      metric:
        name: my-external-metric
        selector:
          matchLabels:
            "resource.labels.metrics_host_project_id": "my-metrics-project"
      target:
        type: Value
        value: 10
```

### Backward Compatibility for External Metrics

For external metrics, the adapter also supports the legacy label `resource.labels.project_id` for backward compatibility. However, it is recommended to use the new label `resource.labels.metrics_host_project_id` for clarity and consistency across all metric types.

If both `resource.labels.metrics_host_project_id` and `resource.labels.project_id` are present for an External metric, the value from `resource.labels.metrics_host_project_id` will be used.

## Permissions

Ensure that the service account used by the Custom Metrics Stackdriver Adapter has the necessary permissions (e.g., `monitoring.viewer`) to read metrics from the target project (`my-metrics-project` in the examples above).
