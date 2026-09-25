# Architecture

`cloud-monitoring-proxy` is a single Go binary that periodically polls a
configured set of metric types from the Cloud Monitoring v3 API
(`projects.timeSeries.list`) and serves the translated series from an in-memory
cache on a Prometheus `/metrics` endpoint.

## Pipeline overview

```
config (fsnotify hot-reload) ──> poll.Manager (one scheduler per metric type)
                                    │  ListTimeSeries(metricType, lookbackWindow)
                                    ▼
                                 translate (naming, labels, kinds, delta-accum, dist→hist)
                                    ▼
                                 cache.Store (in-memory latest-value store + staleness eviction)
                                    ▼
                   ┌────────────────┴─────────────────┐
             export.Gatherer (/metrics)        /api/v1/discovery
```

### Package layout

- `cmd/cloud-monitoring-proxy/`: entrypoint, flag parsing, lifecycle wiring.
- `internal/config/`: YAML schema, validation, preset expansion, `fsnotify` hot-reload.
- `internal/presets/`: embedded YAML metric bundles (`node`, `pod`, `gpu`, `tpu`, `kube-state`).
- `internal/scope/`: project/location/cluster discovery via the GCE metadata server.
- `internal/gcm/`: Cloud Monitoring v3 client wrapper, concurrency semaphore, descriptor cache, and test fake.
- `internal/translate/`: Cloud Monitoring to Prometheus metric/label/value translation and DELTA accumulation.
- `internal/cache/`: interned in-memory series store, per-metric cardinality cap, staleness sweeper.
- `internal/poll/`: per-metric polling schedulers, auto-lookback calculation, rate-limit backoff.
- `internal/export/`: custom `prometheus.Gatherer`, `/metrics`, `/api/v1/discovery`, `/healthz`, `/readyz`.
- `internal/telemetry/`: `gcmproxy_*` self-observability metrics.

## Querying Cloud Monitoring

1. **One metric type per API call**: the Cloud Monitoring `timeSeries.list`
   filter grammar requires a single `metric.type = "..."` equality per request.
   `typePrefix` entries are expanded against the periodically refreshed
   `metricDescriptors.list` cache, and concurrent `ListTimeSeries` calls are
   bounded by `polling.concurrency`.
2. **Automatic lookback window**: `MetricDescriptor.metadata` provides
   `samplePeriod` and `ingestDelay` per metric type. When an entry does not set
   an explicit `lookback`, the proxy queries
   `[now - (ingestDelay + samplePeriod + 30s), now]` (falling back to `5m` if
   descriptor metadata is absent).
3. **Cluster scoping**: on GKE, `projectID`, `location`, and `clusterName` are
   discovered from the GCE metadata server unless overridden in `scope`. When
   `clusterScopeFilter: true` (default), `resource.labels.cluster_name` and
   `resource.labels.location` filters are appended for resource types that
   carry those labels (`k8s_container`, `k8s_pod`, `k8s_node`, `k8s_cluster`,
   `prometheus_target`).

## Translation rules

### Metric names

- `prometheus.googleapis.com/<name>/<type>` → `<name>` (restores the original
  Prometheus metric name for metrics ingested via Google Cloud Managed Service
  for Prometheus).
- All other domains: the first `/` becomes `:`, and any character outside
  `[a-zA-Z0-9_]` becomes `_` (for example,
  `kubernetes.io/container/cpu/core_usage_time` →
  `kubernetes_io:container_cpu_core_usage_time`).
- Post-translation name collisions across distinct Cloud Monitoring metric
  types are rejected so duplicate metric families are never emitted.

### Labels

- Monitored resource labels and metric labels are flattened onto the series;
  if a metric label collides with a resource label, the metric label is
  prefixed with `metric_`.
- `monitored_resource="<resource_type>"` is added to every series to
  disambiguate metric types that span multiple resource types.

### Metric kinds and value types

| Cloud Monitoring kind / valueType | Prometheus type | Behavior |
|---|---|---|
| `GAUGE` (`BOOL`, `INT64`, `DOUBLE`) | Gauge | Latest point in the query window (`BOOL` mapped to `0`/`1`) |
| `CUMULATIVE` (`INT64`, `DOUBLE`) | Counter | Latest point value as-is |
| `DELTA` (`INT64`, `DOUBLE`) | Counter | Re-accumulated in memory: points with `interval.endTime > lastAccumulatedEnd` are added to a per-series running sum |
| `DISTRIBUTION` (`GAUGE`, `CUMULATIVE`, `DELTA`) | Histogram | Converted to cumulative `_bucket{le="..."}` + `+Inf`, `_count`, and `_sum` (`mean × count`); `DELTA` distributions accumulate bucket counts and sums across polls |
| `STRING`, `MONEY` | Dropped | Counted in `gcmproxy_series_dropped_total{reason="value_type"}` |

## Self-observability metrics

All internal metrics use the `gcmproxy_` prefix:

- `gcmproxy_gcm_api_requests_total{method,code}`
- `gcmproxy_billed_series_estimate_total{metric_type}`
- `gcmproxy_poll_duration_seconds{metric_type}`
- `gcmproxy_poll_errors_total{metric_type,reason}`
- `gcmproxy_series_cached{metric_type}`
- `gcmproxy_series_dropped_total{reason}`
- `gcmproxy_delta_bucket_layout_resets_total{metric_type}`
- `gcmproxy_sweep_evicted_total{kind}`
- `gcmproxy_data_age_seconds{metric_type}`
- `gcmproxy_config_load_success_total`, `gcmproxy_config_load_errors_total`,
  `gcmproxy_config_stale`, `gcmproxy_config_last_reload_timestamp_seconds`
- `gcmproxy_build_info{version}`
