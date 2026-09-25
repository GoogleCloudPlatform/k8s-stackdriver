# Configuration reference

`cloud-monitoring-proxy` reads a single YAML configuration file (specified via
`--config`, default `/etc/cloud-monitoring-proxy/config.yaml`). The parent
directory is watched with `fsnotify` so ConfigMap updates are hot-reloaded
without restarting the pod.

## Full schema

```yaml
scope:
  projectID: ""              # default: GCE metadata server (project/project-id)
  location: ""               # default: GCE metadata server (instance/attributes/cluster-location)
  clusterName: ""            # default: GCE metadata server (instance/attributes/cluster-name)
  clusterScopeFilter: true   # AND resource.labels.cluster_name and location onto
                             # queries whose monitored resource type carries those labels

metrics:
  - preset: node             # curated bundle: node, pod, gpu, tpu, kube-state
  - type: kubernetes.io/container/restart_count
    interval: 5m             # per-metric poll interval override (minimum 60s)
    lookback: 10m            # per-metric query lookback override (default: auto from descriptor metadata)
    filter: 'resource.labels.namespace_name = "prod"'   # extra Cloud Monitoring filter ANDed onto query
    rename: ""               # optional override for the output Prometheus metric family name
  - typePrefix: custom.googleapis.com/myapp/            # expanded against the descriptor cache

server:
  listen: ":9090"            # HTTP listen address
  emitTimestamps: false      # true → emit GCM point endTime on /metrics samples

polling:
  defaultInterval: 60s       # minimum 60s
  concurrency: 10            # max concurrent projects.timeSeries.list calls
  pageSize: 10000            # pageSize per ListTimeSeries request
  descriptorRefresh: 30m     # cadence for refreshing metric descriptors and re-expanding prefixes

limits:
  maxSeriesPerMetric: 200000 # per metric type; excess new series are dropped and counted
  staleAfter: 5m             # evict cached series unseen for longer than this duration
```

## Validation and hot-reload behavior

- **Intervals**: `polling.defaultInterval` and any per-entry `interval` must be
  at least `60s` (the finest sampling period for most Cloud Monitoring metrics).
- **Metric selector**: each entry in `metrics` must set exactly one of `preset`,
  `type`, or `typePrefix`. Unknown preset names fail validation.
- **Deduplication**: when multiple entries expand to the same Cloud Monitoring
  metric type (for example, a `preset` followed by an explicit `type`), the
  later entry in the list wins.
- **Cluster scoping**: when `scope.clusterScopeFilter` is `true` (default),
  metric types whose monitored resource descriptors do not carry `cluster_name`
  and `location` labels (for example, `gce_instance`) are skipped with a warning
  and counted in `gcmproxy_poll_errors_total{reason="unscopable_resource"}` to
  prevent accidental project-wide reads. Set `clusterScopeFilter: false` to
  query unscoped resource types.
- **Reload safety**: if a reloaded configuration file fails parsing or
  validation, the proxy logs the error, increments
  `gcmproxy_config_load_errors_total`, sets `gcmproxy_config_stale 1`, and
  continues serving the last valid configuration.
