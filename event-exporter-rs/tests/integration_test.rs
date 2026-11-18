//! Unit, CLI flag parity, PodOwnerTable JobSet inference, RBAC spoofing guard, sharding, and
//! 1,000,000-pod memory benchmark tests.

#![forbid(unsafe_code)]

use event_exporter_rs::cli::{ExporterConfig, SinkKind};
use event_exporter_rs::enricher::{
    EntryPayload, LogSeverity, MonitoredResourceKind, build_startup_marker_entry,
    enrich_event_object,
};
use event_exporter_rs::metrics::{MetricsRegistry, spawn_metrics_server};
use event_exporter_rs::pod_cache::{
    CompactOwner, ControllerKind, PodKey, PodOwnerTable, parse_partial_pod_object,
};
use event_exporter_rs::shard::{ShardMode, ShardSpec};
use serde_json::{Map, Value, json};
use std::collections::HashSet;
use std::fs;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::sync::Arc;
use std::sync::atomic::{AtomicU8, Ordering};
use std::time::{Duration, Instant, UNIX_EPOCH};

#[test]
fn test_cli_gke_manifest_and_otlp_flags() {
    let args = [
        "-sink-opts=-stackdriver-resource-model=new -endpoint=https://logging.googleapis.com/ -flush-delay=2s -max-buffer-size=250",
        "-prometheus-endpoint=:8080",
        "-enable-pod-owner-label=true",
        "-lister-watcher-options-limit=100",
        "-lister-watcher-enable-streaming=true",
        "-storage-type=DeltaFIFOStorage",
        "-system-namespaces=kube-system,gke-managed-system",
        "-event-label-selector=tenancy.gke.io/access-level!=tenant",
        "--universe-domain=apis-tpclp.goog",
        "--shard-mode=server",
        "--shard-count=4",
        "--shard-index=2",
        "-resync-period=30m",
    ];
    let cfg = ExporterConfig::from_args(args).expect("GKE manifest flags must parse");
    assert_eq!(cfg.sink_kind, SinkKind::CloudLogging);
    assert!(cfg.watch_pods);
    assert!(cfg.enable_watch_list);
    assert_eq!(cfg.list_page_size, 100);
    assert_eq!(cfg.flush_delay, Duration::from_secs(2));
    assert_eq!(cfg.max_batch_entries, 250);
    assert_eq!(cfg.shard_mode, ShardMode::Server);
    assert_eq!(cfg.shard_count, 4);
    assert_eq!(cfg.shard_index, Some(2));
    assert_eq!(
        cfg.event_label_selector.as_deref(),
        Some("tenancy.gke.io/access-level!=tenant")
    );
    assert!(cfg.system_namespaces.contains("kube-system"));
    assert!(cfg.system_namespaces.contains("gke-managed-system"));

    let otlp_cfg = ExporterConfig::from_args([
        "--sink=otlp",
        "--otlp-endpoint=http://127.0.0.1:4318/v1/logs",
        "--shard-count=3",
        "-lister-watcher-options-limit=0",
    ])
    .expect("OTLP and unlimited lister-watcher-options-limit=0 flags must parse");
    assert_eq!(otlp_cfg.sink_kind, SinkKind::Otlp);
    assert_eq!(otlp_cfg.shard_mode, ShardMode::Client);
    assert_eq!(otlp_cfg.list_page_size, 500);
    assert!(ExporterConfig::from_args(["-resync-period=invalid"]).is_err());
}

#[test]
fn test_pod_owner_and_jobset_inference() {
    let mut sys_ns = HashSet::new();
    sys_ns.insert("kube-system".to_string());
    let table = PodOwnerTable::new(sys_ns);
    let metrics = MetricsRegistry::new();

    let cases = [
        (
            json!({
                "metadata": {
                    "name": "web-7d9c8b6f5-abcde",
                    "namespace": "prod",
                    "labels": { "pod-template-hash": "7d9c8b6f5", "app": "web" },
                    "ownerReferences": [{ "kind": "ReplicaSet", "name": "web-7d9c8b6f5" }]
                }
            }),
            "prod",
            "web-7d9c8b6f5-abcde",
            Some((ControllerKind::Deployment, "web", None, None)),
        ),
        (
            json!({
                "metadata": {
                    "name": "fluentbit-xyz",
                    "namespace": "obs",
                    "ownerReferences": [{ "kind": "DaemonSet", "name": "fluentbit-gke" }]
                }
            }),
            "obs",
            "fluentbit-xyz",
            Some((ControllerKind::DaemonSet, "fluentbit-gke", None, None)),
        ),
        (
            json!({
                "metadata": {
                    "name": "etcd-0",
                    "namespace": "obs",
                    "ownerReferences": [{ "kind": "StatefulSet", "name": "etcd" }]
                }
            }),
            "obs",
            "etcd-0",
            Some((ControllerKind::StatefulSet, "etcd", None, None)),
        ),
        (
            json!({
                "metadata": {
                    "name": "nightly-sync-28901234-k9p2m",
                    "namespace": "batch",
                    "ownerReferences": [{ "kind": "Job", "name": "nightly-sync-28901234" }]
                }
            }),
            "batch",
            "nightly-sync-28901234-k9p2m",
            Some((ControllerKind::CronJob, "nightly-sync", None, None)),
        ),
        (
            json!({
                "metadata": {
                    "name": "tpu-train-worker-0-0-xyz",
                    "namespace": "ml",
                    "labels": {
                        "jobset.sigs.k8s.io/jobset-name": "tpu-train",
                        "jobset.sigs.k8s.io/restart-attempt": "3",
                        "jobset.sigs.k8s.io/jobset-uid": "js-uid-999"
                    },
                    "ownerReferences": [{ "kind": "Job", "name": "tpu-train-worker-0" }]
                }
            }),
            "ml",
            "tpu-train-worker-0-0-xyz",
            Some((
                ControllerKind::JobSet,
                "tpu-train",
                Some("3"),
                Some("js-uid-999"),
            )),
        ),
        (
            json!({
                "metadata": {
                    "name": "migrate-db-p4x",
                    "namespace": "batch",
                    "ownerReferences": [{ "kind": "Job", "name": "migrate-db" }]
                }
            }),
            "batch",
            "migrate-db-p4x",
            Some((ControllerKind::Job, "migrate-db", None, None)),
        ),
        (
            json!({
                "metadata": {
                    "name": "coredns-abc",
                    "namespace": "kube-system",
                    "labels": { "pod-template-hash": "abc" },
                    "ownerReferences": [{ "kind": "ReplicaSet", "name": "coredns-abc" }]
                }
            }),
            "kube-system",
            "coredns-abc",
            None,
        ),
    ];

    for (raw_json, ns, pod_name, expected) in cases {
        let bytes = serde_json::to_vec(&raw_json).unwrap();
        let meta = parse_partial_pod_object(&bytes).unwrap();
        table.upsert_pod_metadata(&meta, Some(&metrics));
        let got = table.lookup(ns, pod_name, Some(&metrics));
        match expected {
            Some((kind, name, ra, uid)) => {
                let owner = got.expect("expected resolved owner");
                assert_eq!(owner.kind, kind);
                assert_eq!(owner.name.as_str(), name);
                assert_eq!(owner.jobset_restart_attempt.as_deref(), ra);
                assert_eq!(owner.jobset_uid.as_deref(), uid);
            }
            None => assert!(got.is_none()),
        }
    }

    table.remove_pod("obs", "etcd-0", Some(&metrics));
    assert!(table.lookup("obs", "etcd-0", Some(&metrics)).is_none());
    let prom = metrics.render_prometheus();
    assert!(prom.contains("podlabel_cache_ops_count{operation=\"add\"} 6"));
    assert!(prom.contains("podlabel_cache_ops_count{operation=\"evict\"} 1"));
    assert!(prom.contains("podlabel_cache_ops_count{operation=\"queryhit\"} 6"));
    assert!(prom.contains("podlabel_cache_ops_count{operation=\"querymiss\"} 1"));
}

#[test]
fn test_enricher_rbac_spoofing_blacklist_and_startup_marker() {
    let table = PodOwnerTable::new(HashSet::new());
    let victim_pod = parse_partial_pod_object(
        serde_json::to_vec(&json!({
            "metadata": {
                "name": "secret-pod-123",
                "namespace": "tenant-victim",
                "labels": {
                    "jobset.sigs.k8s.io/jobset-name": "victim-jobset",
                    "jobset.sigs.k8s.io/restart-attempt": "2"
                },
                "ownerReferences": [{ "kind": "Job", "name": "victim-jobset-replicated" }]
            }
        }))
        .unwrap()
        .as_slice(),
    )
    .unwrap();
    table.upsert_pod_metadata(&victim_pod, None);

    let now = UNIX_EPOCH + Duration::from_secs(1_760_000_000);

    let legit_map: Map<String, Value> = serde_json::from_value(json!({
        "metadata": { "uid": "ev-1", "resourceVersion": "42", "namespace": "tenant-victim" },
        "involvedObject": { "kind": "Pod", "namespace": "tenant-victim", "name": "secret-pod-123" },
        "type": "Warning",
        "reason": "BackOff",
        "message": "Back-off restarting failed container",
        "count": 7,
        "firstTimestamp": "2026-05-01T00:00:00Z",
        "lastTimestamp": "2026-05-01T01:02:03.450000000Z"
    }))
    .unwrap();
    let enriched = enrich_event_object(legit_map, Some(&table), None, now);
    assert_eq!(enriched.insert_id.as_str(), "ev-1-42");
    assert_eq!(enriched.severity, LogSeverity::Warning);
    assert_eq!(
        enriched.resource,
        MonitoredResourceKind::K8sPod {
            namespace_name: "tenant-victim".to_string(),
            pod_name: "secret-pod-123".to_string()
        }
    );
    assert!(enriched.labels.contains(&(
        "logging.gke.io/top_level_controller_type".to_string(),
        "JobSet".to_string()
    )));
    assert!(enriched.labels.contains(&(
        "logging.gke.io/top_level_controller_name".to_string(),
        "victim-jobset".to_string()
    )));
    assert!(enriched.labels.contains(&(
        "jobset.sigs.k8s.io/restart-attempt".to_string(),
        "2".to_string()
    )));
    if let EntryPayload::Json(payload_map) = &enriched.payload {
        assert!(!payload_map.contains_key("count"));
        assert!(!payload_map.contains_key("firstTimestamp"));
        assert_eq!(
            payload_map.get("kind").and_then(Value::as_str),
            Some("Event")
        );
        assert_eq!(
            payload_map.get("apiVersion").and_then(Value::as_str),
            Some("v1")
        );
        assert_eq!(
            payload_map.get("reason").and_then(Value::as_str),
            Some("BackOff")
        );
    } else {
        panic!("expected JSON payload");
    }

    let spoofed_map: Map<String, Value> = serde_json::from_value(json!({
        "metadata": { "uid": "ev-spoof", "resourceVersion": "43", "namespace": "tenant-attacker" },
        "involvedObject": { "kind": "Pod", "namespace": "tenant-victim", "name": "secret-pod-123" },
        "type": "Normal",
        "series": { "lastObservedTime": "2026-05-01T02:00:00Z" }
    }))
    .unwrap();
    let spoofed_entry = enrich_event_object(spoofed_map, Some(&table), None, now);
    assert_eq!(spoofed_entry.resource, MonitoredResourceKind::K8sCluster);
    assert!(spoofed_entry.labels.is_empty());

    let shard0 = ShardSpec::resolve(ShardMode::Server, 4, Some(0), None).unwrap();
    let shard2 = ShardSpec::resolve(ShardMode::Server, 4, Some(2), None).unwrap();
    assert!(build_startup_marker_entry(&shard0, now).is_some());
    assert!(build_startup_marker_entry(&shard2, now).is_none());
}

#[test]
fn test_sharding_co_partitioning_and_statefulset_ordinal() {
    let resolved =
        ShardSpec::resolve(ShardMode::Server, 8, None, Some("event-exporter-gke-5")).unwrap();
    assert_eq!(resolved.shard_index, 5);
    assert_eq!(
        resolved.server_query_params().as_deref(),
        Some(
            "shardSelector=shardRange%28object.metadata.namespace%2C%20%270xa000000000000000%27%2C%20%270xc000000000000000%27%29"
        )
    );

    let shards: Vec<ShardSpec> = (0..4)
        .map(|i| ShardSpec::resolve(ShardMode::Client, 4, Some(i), None).unwrap())
        .collect();

    for idx in 0..500 {
        let ns = format!("ns-{}", idx % 17);
        let pod = format!("worker-pod-{idx}");
        let uid = format!("uid-{idx}");

        let pod_owners: Vec<u32> = shards
            .iter()
            .filter(|s| s.owns_pod(&ns, &pod))
            .map(|s| s.shard_index)
            .collect();
        assert_eq!(pod_owners.len(), 1);

        let event_owners: Vec<u32> = shards
            .iter()
            .filter(|s| s.owns_event(&ns, "Pod", &ns, &pod, &uid))
            .map(|s| s.shard_index)
            .collect();
        assert_eq!(event_owners, pod_owners);
    }
}

#[test]
fn test_metrics_server_large_bearer_header_start_time_and_exact_bounds() {
    let registry = Arc::new(MetricsRegistry::new());
    registry.observe_sink_latency(Duration::from_secs(60), 1);

    let stop_flag = Arc::new(AtomicU8::new(0));
    let (port, handle) =
        spawn_metrics_server(":0", Arc::clone(&registry), Arc::clone(&stop_flag)).unwrap();

    // Scrape over IPv4 with a >1 KiB Kubernetes ServiceAccount Bearer JWT header (regression for
    // TCP RST when closing before draining headers).
    let bearer = "a".repeat(1200);
    let mut stream = TcpStream::connect(("127.0.0.1", port)).unwrap();
    write!(
        stream,
        "GET /metrics HTTP/1.1\r\nHost: localhost:{port}\r\nAuthorization: Bearer {bearer}\r\n\r\n"
    )
    .unwrap();
    let mut resp = String::new();
    stream.read_to_string(&mut resp).unwrap();
    assert!(resp.contains("HTTP/1.1 200 OK"));
    assert!(
        resp.contains("\n# TYPE process_start_time_seconds gauge\nprocess_start_time_seconds ")
    );
    assert!(resp.contains("stackdriver_sink_records_latency_seconds_bucket{le=\"51.2578125\"} 0"));
    assert!(resp.contains("stackdriver_sink_records_latency_seconds_bucket{le=\"76.88671875\"} 1"));
    assert!(resp.contains("stackdriver_sink_records_latency_seconds_count 1"));

    // Scrape /healthz over IPv6 loopback when available on the host.
    if let Ok(mut v6) = TcpStream::connect(("::1", port)) {
        write!(v6, "GET /healthz HTTP/1.1\r\nHost: [::1]:{port}\r\n\r\n").unwrap();
        let mut h_resp = String::new();
        v6.read_to_string(&mut h_resp).unwrap();
        assert!(h_resp.ends_with("\r\n\r\nok\n"));
    }

    // Verify histogram bucket monotonicity while observations arrive concurrently with TCP scrapes.
    let writer_reg = Arc::clone(&registry);
    let writer = std::thread::spawn(move || {
        for i in 0..500u64 {
            writer_reg.observe_sink_latency(Duration::from_millis(100 * (i % 120)), 1);
        }
    });
    for _ in 0..10 {
        let mut s = TcpStream::connect(("127.0.0.1", port)).unwrap();
        write!(s, "GET /metrics HTTP/1.1\r\nHost: localhost:{port}\r\n\r\n").unwrap();
        let mut body = String::new();
        s.read_to_string(&mut body).unwrap();
        let mut prev = 0u64;
        for line in body.lines() {
            if let Some(rest) = line.strip_prefix("stackdriver_sink_records_latency_seconds_bucket")
            {
                let count: u64 = rest
                    .rsplit_once(' ')
                    .and_then(|(_, v)| v.parse().ok())
                    .expect("bucket count must parse");
                assert!(count >= prev, "non-monotonic bucket {count} < {prev}");
                prev = count;
            }
        }
    }
    writer.join().unwrap();

    stop_flag.store(1, Ordering::Release);
    handle.join().unwrap();
}

#[test]
fn test_one_million_pods_memory_benchmark() {
    assert_eq!(std::mem::size_of::<(PodKey, CompactOwner)>(), 24);

    let rss_before_bytes = read_process_rss_bytes();
    let table = PodOwnerTable::new(HashSet::new());

    const TOTAL_PODS: usize = 1_000_000;
    let ingest_start = Instant::now();
    for i in 0..TOTAL_PODS {
        let ns_idx = i % 250;
        let deploy_idx = i % 2_000;
        let namespace = format!("ns-{ns_idx}");
        let pod_name = format!("workload-{deploy_idx}-7f8d9c6b5-{i}");
        let raw = if i % 10 == 0 {
            format!(
                "{{\"metadata\":{{\"name\":\"{pod_name}\",\"namespace\":\"{namespace}\",\"resourceVersion\":\"{i}\",\
                 \"labels\":{{\"jobset.sigs.k8s.io/jobset-name\":\"tpu-jobset-{deploy_idx}\",\
                 \"jobset.sigs.k8s.io/restart-attempt\":\"1\",\"irrelevant-label\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}},\
                 \"ownerReferences\":[{{\"kind\":\"Job\",\"name\":\"tpu-jobset-{deploy_idx}-worker-0\"}}]}}}}"
            )
        } else {
            format!(
                "{{\"metadata\":{{\"name\":\"{pod_name}\",\"namespace\":\"{namespace}\",\"resourceVersion\":\"{i}\",\
                 \"labels\":{{\"pod-template-hash\":\"7f8d9c6b5\",\"app.kubernetes.io/instance\":\"inst-{deploy_idx}\"}},\
                 \"ownerReferences\":[{{\"kind\":\"ReplicaSet\",\"name\":\"workload-{deploy_idx}-7f8d9c6b5\"}}]}}}}"
            )
        };
        let meta = parse_partial_pod_object(raw.as_bytes()).unwrap();
        table.upsert_pod_metadata(&meta, None);
    }
    let ingest_elapsed = ingest_start.elapsed();

    assert_eq!(table.len(), TOTAL_PODS);

    let sample_deploy = table
        .lookup("ns-1", "workload-1-7f8d9c6b5-1", None)
        .expect("Deployment pod 1 must exist");
    assert_eq!(sample_deploy.kind, ControllerKind::Deployment);
    assert_eq!(sample_deploy.name.as_str(), "workload-1");

    let sample_jobset = table
        .lookup("ns-240", "workload-1990-7f8d9c6b5-999990", None)
        .expect("JobSet pod 999990 must exist");
    assert_eq!(sample_jobset.kind, ControllerKind::JobSet);
    assert_eq!(sample_jobset.name.as_str(), "tpu-jobset-1990");
    assert_eq!(sample_jobset.jobset_restart_attempt.as_deref(), Some("1"));

    const LOOKUP_ITERS: usize = 100_000;
    let lookup_start = Instant::now();
    for i in 0..LOOKUP_ITERS {
        let ns = format!("ns-{}", i % 250);
        let pod = format!("workload-{}-7f8d9c6b5-{i}", i % 2_000);
        assert!(table.lookup(&ns, &pod, None).is_some());
    }
    let lookup_elapsed = lookup_start.elapsed();

    let rss_after_bytes = read_process_rss_bytes();
    let rss_before_mib = (rss_before_bytes as f64) / (1024.0 * 1024.0);
    let rss_after_mib = (rss_after_bytes as f64) / (1024.0 * 1024.0);
    let rss_delta_bytes = rss_after_bytes.saturating_sub(rss_before_bytes);
    let rss_delta_mib = (rss_delta_bytes as f64) / (1024.0 * 1024.0);
    let bytes_per_pod = (rss_delta_bytes as f64) / (TOTAL_PODS as f64);
    let pods_per_sec = (TOTAL_PODS as f64) / ingest_elapsed.as_secs_f64();
    let ns_per_lookup = (lookup_elapsed.as_nanos() as f64) / (LOOKUP_ITERS as f64);
    println!(
        "[BENCHMARK_1M_PODS] pods={TOTAL_PODS} slot_bytes=24 rss_before_mib={rss_before_mib:.2} \
         rss_after_mib={rss_after_mib:.2} rss_delta_mib={rss_delta_mib:.2} \
         rss_bytes_per_pod={bytes_per_pod:.2} ingest_ms={:.1} pods_per_sec={pods_per_sec:.0} \
         lookup_100k_ms={:.2} ns_per_lookup={ns_per_lookup:.1}",
        ingest_elapsed.as_secs_f64() * 1000.0,
        lookup_elapsed.as_secs_f64() * 1000.0,
    );
    assert!(
        rss_after_bytes > 0,
        "expected non-zero VmRSS from /proc/self/status"
    );
    assert!(
        rss_delta_mib < 80.0,
        "expected 1M-pod PodOwnerTable RSS delta < 80 MiB, got {rss_delta_mib:.2} MiB"
    );
}

fn parse_vm_rss_bytes(status: &str) -> Option<u64> {
    status.lines().find_map(|line| {
        let rest = line.strip_prefix("VmRSS:")?;
        let kib = rest.split_whitespace().next()?.parse::<u64>().ok()?;
        Some(kib * 1024)
    })
}

fn read_process_rss_bytes() -> u64 {
    fs::read_to_string("/proc/self/status")
        .ok()
        .and_then(|s| parse_vm_rss_bytes(&s))
        .unwrap_or(0)
}
