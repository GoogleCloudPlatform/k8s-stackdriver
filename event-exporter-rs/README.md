# GKE `event-exporter-rs`

Pure-Rust, `#![forbid(unsafe_code)]`, bounded-memory Kubernetes Event exporter for GKE worker nodes (`linux/amd64` and `linux/arm64` on `gcr.io/distroless/static`).

## Key Properties

- **Bounded Pod Owner Lookup (`PodOwnerTable`)**: Streams `meta.k8s.io/v1 PartialObjectMetadata` (`WatchList` with automatic paginated `List` fallback) into a compact 24-byte/slot SwissTable (`1,000,000` pods in `~55–65 MiB` RSS vs. `>1.2 GiB` in Go `client-go` `Reflector`).
- **Zero `DeltaFIFO` Event Retention**: Streams Kubernetes Event Watch frames line-by-line into a byte-bounded (`16 MiB` default) micro-batching queue with backpressure, recovering missed events across `410 Gone` etcd compactions via `resourceVersion` watermarks.
- **Horizontal Sharding**: Supports single-replica Deployment (`--shard-mode=none`) and StatefulSet sharding (`--shard-mode=server` KEP-5866 with automatic `--shard-mode=client` Jump Consistent Hash co-partitioning fallback).
- **Dual Export Sinks**:
  - `--sink=stackdriver` (default): Cloud Logging v2 `POST <endpoint>/v2/entries:write` (gzipped HTTP/2 JSON, supporting `--universe-domain` for Sovereign/TPC/GDU clusters).
  - `--sink=otlp`: OpenTelemetry `LogsService/Export` (`telemetry.googleapis.com:443` gRPC or OTLP/HTTP `/v1/logs` protobuf) preserving `k8s_pod`, `k8s_node`, and `k8s_cluster` `MonitoredResource` mappings and LOrIS Prometheus metrics (`:8080/metrics`).
- **Zero Google-Internal Dependencies**: 100% pure safe Rust (`tokio`, `reqwest` + `rustls`/`ring`, `tonic`/`prost`, `serde_json`), statically linked onto `gcr.io/distroless/static:latest`.

## Build & Test

```bash
make test
make lint
make build
```
