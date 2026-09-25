# Preset bundles

Presets are curated sets of Cloud Monitoring metric types you can reference
by name in the config instead of listing metric types by hand:

```yaml
metrics:
  - preset: node
  - preset: gpu
    interval: 60s
```

A preset entry expands at config load; `interval`, `lookback`, and `filter`
on the entry apply to every expanded metric. An explicit `type` entry later
in the list overrides the preset's settings for that one metric.

To see what a preset would serve (names, kinds, ingest delays) on a running
proxy: `curl localhost:9090/api/v1/discovery?prefix=kubernetes.io/node/`.

| Preset | Covers | Requires (GKE `--monitoring`) |
|---|---|---|
| `node` | Node CPU, memory, network, ephemeral storage, PIDs, conditions, interruptions | `SYSTEM` (default-on) |
| `pod` | Pod network/volumes + container CPU/memory vs requests/limits, restarts, uptime | `SYSTEM` (default-on) |
| `gpu` | GKE system accelerator metrics + the DCGM package (utilization, framebuffer, temps, power, NVLink/PCIe profiling) | `SYSTEM` for the system set; `SYSTEM,DCGM` for DCGM (GKE ≥1.30.1-gke.1204000, managed GPU drivers) |
| `tpu` | TPU duty cycle, HBM memory, tensorcore/bandwidth utilization, interruption/recovery, multislice network+compute latencies | `SYSTEM` (default-on); TPU slice node pools, JAX ≥0.4.14, workload port 8431 |
| `kube-state` | Pod phases, deployment/statefulset/daemonset replica health, HPA, PVC phases | `POD,DEPLOYMENT,STATEFULSET,DAEMONSET,HPA,STORAGE` |

Exact contents live in [`internal/presets/bundles/`](../internal/presets/bundles/).

Notes:

- `prometheus.googleapis.com/*` types (DCGM, kube-state) follow the public
  GKE documentation; their descriptors appear in a project only once the
  corresponding package writes data.
- GPU system accelerator metrics are not collected under GPU time-sharing
  or MPS.
- Cloud Monitoring bills API reads per time series returned (see
  [Cloud Monitoring pricing](https://cloud.google.com/stackdriver/pricing)).
  Narrow presets, `filter` expressions, and longer per-entry `interval`
  values reduce read volume.
