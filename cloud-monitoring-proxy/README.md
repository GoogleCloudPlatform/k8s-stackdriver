# Cloud Monitoring Proxy (Experimental)

> [!CAUTION]
> **Experimental — Not an officially supported Google product.**
>
> `cloud-monitoring-proxy` is an experimental, feature-scoped open-source
> utility provided **as-is** under the Apache 2.0 license, with **no SLA, no
> support commitments, no official container or Helm chart releases, and no
> guarantee of maintenance or backward compatibility**. Configuration syntax,
> flags, HTTP endpoints, and exported metric names may change or be removed at
> any time. Review the [Experimental Status and Limitations](#experimental-status-and-limitations)
> and [API Read Cost](#api-read-cost) sections carefully before deploying in
> any environment.

`cloud-monitoring-proxy` is a small in-cluster service that polls an explicit,
user-configured list of metric types from Google Cloud Monitoring and
re-exposes them on a standard Prometheus `/metrics` HTTP endpoint. It allows
in-cluster Prometheus-compatible collectors (Prometheus, OpenTelemetry
Collector, or third-party agents) to scrape selected GKE system, accelerator
(GPU/TPU), and kube-state metrics from Cloud Monitoring using Kubernetes
Workload Identity Federation.

```
Cloud Monitoring ──ListTimeSeries──> cloud-monitoring-proxy (poll + cache) ──/metrics──> in-cluster scraper
     (GCM)           (default 60s)            (single replica)
```

## Experimental status and limitations

This project is strictly a **poll-and-scrape read adapter** with important
architectural and operational constraints:

1. **Minutes-level freshness (not real-time)**: Cloud Monitoring metrics are
   sampled periodically (typically every 60s) and have an ingestion delay of up
   to several minutes depending on the metric family. Scraped values reflect the
   latest point available in Cloud Monitoring when polled, not the instant of
   the scrape. Per-metric staleness is exposed on
   `gcmproxy_data_age_seconds{metric_type=...}`.
2. **Timestamp behavior and `rate()` windows**: By default
   (`server.emitTimestamps: false`), samples are served **without** explicit
   timestamps so standard Prometheus scrapers do not reject delayed Cloud
   Monitoring points as too old (`out-of-bounds`). Because the scraper assigns
   its own scrape timestamp, rate calculations are shifted by the ingestion
   delay. Setting `server.emitTimestamps: true` emits original Cloud Monitoring
   timestamps, which requires a scraper/storage backend configured to accept
   delayed samples.
3. **Cloud Monitoring API read cost**: Every poll executes
   `projects.timeSeries.list` calls (one per configured metric type), which are
   billed by Cloud Monitoring per time series returned. High-cardinality metric
   selections or frequent polling can generate substantial Cloud Monitoring API
   charges. **The proxy does not enforce budget caps or cost guardrails.**
4. **Single replica only (no high availability or sharding)**: Each replica
   polls Cloud Monitoring independently. Running `N` replicas multiplies Cloud
   Monitoring API read volume and cost by `N` without sharding the workload.
5. **In-memory state only**: Both the series cache and the `DELTA`-to-counter
   accumulators are held strictly in memory. A pod restart requires one poll
   cycle to refill the cache and resets `DELTA`-accumulated counters to zero
   (handled as a standard counter reset by PromQL `rate()`/`increase()`).
6. **Single-cluster scope by default**: By default
   (`scope.clusterScopeFilter: true`), queries are restricted to the local GKE
   cluster (`cluster_name` and `location`) and resource types without cluster
   labels (such as `gce_instance`) are skipped. Setting
   `clusterScopeFilter: false` allows querying project-wide metrics, which
   exposes those project-wide metrics to any pod that can reach `/metrics`.
7. **Scrape-only (`/metrics`)**: Only pull-based Prometheus exposition is
   implemented. Push protocols (such as Prometheus Remote Write or OTLP push)
   are not supported.
8. **Unauthenticated HTTP endpoints**: `/metrics` and `/api/v1/discovery` are
   served over plain HTTP without authentication. On multi-tenant clusters,
   restrict ingress using `NetworkPolicy` (`networkPolicy.enabled=true` in the
   Helm chart or [`deploy/manifests/networkpolicy-optional.yaml`](deploy/manifests/networkpolicy-optional.yaml)).
9. **Supported value types**: Only `BOOL`, `INT64`, `DOUBLE`, and `DISTRIBUTION`
   metrics are translated. `STRING` and `MONEY` metrics are dropped.

## Building and installing

No pre-built container images or hosted Helm charts are published. Build the
container image from source into your own registry and install using the
included Helm chart or raw manifests.

```bash
git clone https://github.com/GoogleCloudPlatform/k8s-stackdriver.git
cd k8s-stackdriver/cloud-monitoring-proxy

# Build the image with Cloud Build (or docker build / ko):
gcloud builds submit --tag REGION-docker.pkg.dev/PROJECT/REPO/cloud-monitoring-proxy:v0.1.0 .
```

### Quickstart on GKE (Helm + Workload Identity Federation)

Prerequisites: a GKE cluster with Workload Identity Federation enabled, `helm`,
`kubectl`, and `gcloud`.

```bash
# 1. Install the chart from the local tree
helm install cmproxy deploy/chart/cloud-monitoring-proxy \
  --namespace cmproxy --create-namespace \
  --set image.repository=REGION-docker.pkg.dev/PROJECT/REPO/cloud-monitoring-proxy \
  --set image.tag=v0.1.0

# 2. Grant roles/monitoring.viewer to the Kubernetes ServiceAccount
#    (note: the principal path uses the project NUMBER; the pool uses project ID)
PROJECT_ID=$(gcloud config get-value project)
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format='value(projectNumber)')
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --role roles/monitoring.viewer \
  --member "principal://iam.googleapis.com/projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/$PROJECT_ID.svc.id.goog/subject/ns/cmproxy/sa/cmproxy-cloud-monitoring-proxy" \
  --condition=None

# 3. Verify readiness and inspect exposed metrics
kubectl -n cmproxy port-forward svc/cmproxy-cloud-monitoring-proxy 9090:9090 &
curl -s localhost:9090/readyz
curl -s localhost:9090/metrics | grep kubernetes_io: | head
curl -s "localhost:9090/api/v1/discovery?prefix=kubernetes.io/node/" | head -30
```

Raw Kubernetes manifests are also available under [`deploy/manifests/`](deploy/manifests/).

## Configuration

Configuration is loaded from YAML and hot-reloaded on file/ConfigMap changes:

```yaml
scope:
  # Auto-discovered from the GCE metadata server on GKE; restricts queries
  # to the local cluster unless set to false:
  clusterScopeFilter: true

metrics:
  - preset: node               # curated bundles: node, pod, gpu, tpu, kube-state
  - preset: pod
  - type: kubernetes.io/container/restart_count
    interval: 5m               # per-metric poll interval (minimum 60s)
    filter: 'resource.labels.namespace_name = "prod"'
  - typePrefix: custom.googleapis.com/myapp/
    lookback: 10m              # override automatic ingestDelay+samplePeriod window

server:
  listen: ":9090"
  emitTimestamps: false

polling:
  defaultInterval: 60s
  concurrency: 10

limits:
  maxSeriesPerMetric: 200000
  staleAfter: 5m
```

- Full configuration reference: [`docs/configuration.md`](docs/configuration.md)
- Curated presets (`node`, `pod`, `gpu`, `tpu`, `kube-state`): [`docs/presets.md`](docs/presets.md)
- Example configurations: [`examples/`](examples/)
- Architecture and translation details: [`docs/architecture.md`](docs/architecture.md)

### Metric naming and type mapping

Metric names follow the Google Cloud Managed Service for Prometheus (GMP)
convention so PromQL queries written for Cloud Monitoring's PromQL surface use
the same metric and label names:

| Cloud Monitoring metric type | Prometheus name on `/metrics` |
|---|---|
| `kubernetes.io/container/cpu/core_usage_time` | `kubernetes_io:container_cpu_core_usage_time` |
| `compute.googleapis.com/instance/cpu/utilization` | `compute_googleapis_com:instance_cpu_utilization` |
| `prometheus.googleapis.com/kube_pod_status_phase/gauge` | `kube_pod_status_phase` |
| `prometheus.googleapis.com/DCGM_FI_DEV_GPU_UTIL/gauge` | `DCGM_FI_DEV_GPU_UTIL` |

- `GAUGE` → Prometheus `gauge`
- `CUMULATIVE` → Prometheus `counter`
- `DELTA` → Prometheus `counter` (re-accumulated in memory across poll cycles)
- `DISTRIBUTION` → Prometheus classic `histogram` (`_bucket`, `_sum`, `_count`)

## API read cost

Cloud Monitoring bills API reads (`projects.timeSeries.list`) **per time series
returned**. Refer to the official [Cloud Monitoring pricing documentation](https://cloud.google.com/stackdriver/pricing)
for current rates and free-tier allotments.

> `monthly series reads ≈ (series returned per poll) × (polls per month)`

At a `60s` polling interval (~43,200 polls/month), each 1,000 active time series
generates roughly **43.2 million series reads per month** (~8.6 million/month at
a `5m` interval).

To control API read volume:

- Enable only the presets or individual `type` entries you need.
- Increase `interval` (e.g., `5m` or `15m`) for slower-moving metrics.
- Use per-entry `filter` expressions to restrict resource or metric labels.
- Keep `replicaCount: 1`.
- Monitor `gcmproxy_billed_series_estimate_total` and compare against Cloud
  Monitoring's `monitoring.googleapis.com/billing/time_series_billed_for_queries_count`.

## HTTP endpoints

| Path | Description |
|---|---|
| `/metrics` | Prometheus exposition endpoint (plus `gcmproxy_*` self-metrics) |
| `/api/v1/discovery?prefix=` | Read-only JSON listing of metric descriptors in the project (type, translated Prometheus name, kind, value type, sample period, ingest delay) |
| `/healthz`, `/readyz` | Liveness and readiness probes (`/readyz` succeeds once the initial poll cycle completes) |

## Related projects

- [`prometheus-community/stackdriver_exporter`](https://github.com/prometheus-community/stackdriver_exporter)
  is a general-purpose Prometheus exporter for Cloud Monitoring across GCP
  projects and services. `cloud-monitoring-proxy` focuses specifically on
  in-cluster GKE usage with automatic cluster scoping, background polling into
  an in-memory cache (decoupling scrape latency from Cloud Monitoring API calls),
  automatic per-metric lookback windows from descriptor metadata, GMP-compatible
  metric naming, and in-memory `DELTA` counter accumulation.

## Development

```bash
make build
make test
make lint
```

See [`e2e/README.md`](e2e/README.md) for running the live GKE end-to-end
verification suite.

## License

Apache 2.0 — see the repository root [`LICENSE`](../LICENSE).
