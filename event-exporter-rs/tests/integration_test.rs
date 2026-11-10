//! Unit, golden wire-parity, live OTLP gRPC `tonic` server, GKE auth, and 1,000,000-pod
//! memory benchmark tests.

#![forbid(unsafe_code)]

use event_exporter_rs::auth::{
    CloudTokenProvider, KubeTokenSource, resolve_cluster_coordinates, resolve_logging_endpoint,
    resolve_otlp_endpoint,
};
use event_exporter_rs::cli::{ExporterConfig, SinkKind};
use event_exporter_rs::enricher::{
    ClusterCoordinates, EntryPayload, LogSeverity, MonitoredResourceKind,
    build_startup_marker_entry, enrich_event_object, parse_rfc3339_unix_nanos,
};
use event_exporter_rs::metrics::{MetricsRegistry, grpc_status_to_http_code, spawn_metrics_server};
use event_exporter_rs::otlp_proto::{
    ExportLogsPartialSuccess, ExportLogsServiceRequest, ExportLogsServiceResponse, SeverityNumber,
    any_value,
};
use event_exporter_rs::pod_cache::{
    CompactOwner, ControllerKind, PodKey, PodOwnerTable, parse_partial_pod_object,
};
use event_exporter_rs::shard::{ShardMode, ShardSpec};
use event_exporter_rs::sink::{
    BatchExportOutcome, EventSink, OtlpSink, build_cloud_logging_write_body,
    build_otlp_export_request,
};
use event_exporter_rs::transport::gunzip_bytes;
use prost::Message;
use serde_json::{Map, Value, json};
use std::collections::{HashMap, HashSet};
use std::fs;
use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::atomic::{AtomicU8, AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
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
    assert_eq!(
        resolve_logging_endpoint(&cfg),
        "https://logging.apis-tpclp.goog/"
    );
    assert_eq!(resolve_otlp_endpoint(&cfg), "telemetry.apis-tpclp.goog:443");

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

fn kv_str(attrs: &[event_exporter_rs::otlp_proto::KeyValue]) -> HashMap<String, String> {
    attrs
        .iter()
        .filter_map(|kv| {
            let s = match kv.value.as_ref()?.value.as_ref()? {
                any_value::Value::StringValue(s) => s.clone(),
                _ => return None,
            };
            Some((kv.key.clone(), s))
        })
        .collect()
}

#[test]
fn test_otlp_proto_translation_and_loris_status_mapping() {
    assert_eq!(grpc_status_to_http_code(0), "200");
    assert_eq!(grpc_status_to_http_code(7), "403");
    assert_eq!(grpc_status_to_http_code(8), "429");
    assert_eq!(grpc_status_to_http_code(14), "503");

    let coords = ClusterCoordinates {
        project_id: "test-proj".to_string(),
        cluster_name: "mega-cluster-1".to_string(),
        location: "us-central1".to_string(),
    };
    let now = UNIX_EPOCH + Duration::from_secs(1_760_000_000);
    let event_map: Map<String, Value> = serde_json::from_value(json!({
        "metadata": { "uid": "u1", "resourceVersion": "99", "namespace": "default" },
        "involvedObject": { "kind": "Node", "name": "gke-node-pool-1-abc" },
        "type": "Warning",
        "reason": "NodeNotReady",
        "eventTime": "2026-05-02T12:34:56.789000000Z"
    }))
    .unwrap();
    let node_entry = enrich_event_object(event_map, None, None, now);
    let startup_entry = build_startup_marker_entry(&ShardSpec::default(), now).unwrap();

    let batch = vec![node_entry, startup_entry];
    let otlp_req = build_otlp_export_request(&coords, &batch);
    let encoded = otlp_req.encode_to_vec();
    let decoded = ExportLogsServiceRequest::decode(encoded.as_slice()).expect("parse OTLP request");
    assert_eq!(decoded.resource_logs.len(), 2);

    let mut saw_node = false;
    let mut saw_startup = false;
    for rl in &decoded.resource_logs {
        let res_attrs = kv_str(&rl.resource.as_ref().unwrap().attributes);
        let res_type = res_attrs
            .get("gcp.resource_type")
            .map(String::as_str)
            .unwrap_or("");
        for sl in &rl.scope_logs {
            for lr in &sl.log_records {
                let rec_attrs = kv_str(&lr.attributes);
                assert_eq!(
                    rec_attrs.get("gcp.log_name").map(String::as_str),
                    Some("events")
                );
                assert_eq!(lr.severity_number, SeverityNumber::Warn as i32);
                if res_type == "k8s_node" {
                    saw_node = true;
                    assert_eq!(
                        res_attrs.get("node_name").map(String::as_str),
                        Some("gke-node-pool-1-abc")
                    );
                    assert_eq!(
                        rec_attrs
                            .get("logging.googleapis.com/insertId")
                            .map(String::as_str),
                        Some("u1-99")
                    );
                    assert!(matches!(
                        lr.body.as_ref().and_then(|b| b.value.as_ref()),
                        Some(any_value::Value::KvlistValue(_))
                    ));
                    assert_eq!(
                        lr.time_unix_nano,
                        parse_rfc3339_unix_nanos("2026-05-02T12:34:56.789000000Z").unwrap()
                    );
                } else if res_type == "k8s_cluster" {
                    saw_startup = true;
                    let Some(any_value::Value::StringValue(body_str)) =
                        lr.body.as_ref().and_then(|b| b.value.as_ref())
                    else {
                        panic!("expected string body for startup marker");
                    };
                    assert!(body_str.contains("Event exporter started watching"));
                }
            }
        }
    }
    assert!(saw_node && saw_startup);

    let captured_otlp_bytes: Arc<Mutex<Vec<u8>>> = Arc::new(Mutex::new(Vec::new()));
    let captured_clone = Arc::clone(&captured_otlp_bytes);
    let server = MockHttpServer::start(move |req| {
        if req.path == "/v1/logs" {
            let decompressed = gunzip_bytes(&req.body, 4 * 1024 * 1024).expect("gunzip OTLP body");
            *captured_clone.lock().unwrap() = decompressed;
            MockHttpReply::ok_bytes("application/x-protobuf", Vec::new())
        } else {
            MockHttpReply::status(404, "not found")
        }
    });

    let metrics = MetricsRegistry::new();
    let sink = OtlpSink::new(
        &format!("{}/v1/logs", server.base_url()),
        coords.clone(),
        Arc::new(CloudTokenProvider::unauthenticated()),
        None,
    )
    .unwrap();
    let outcome = sink.export_batch(&batch, &metrics);
    assert_eq!(outcome, BatchExportOutcome::Success { sent_entries: 2 });
    let wire_req = ExportLogsServiceRequest::decode(captured_otlp_bytes.lock().unwrap().as_slice())
        .expect("valid OTLP");
    assert_eq!(wire_req.resource_logs.len(), 2);

    let cl_json: Value =
        serde_json::from_slice(&build_cloud_logging_write_body(&coords, &batch)).unwrap();
    assert_eq!(
        cl_json["logName"].as_str(),
        Some("projects/test-proj/logs/events")
    );
    assert_eq!(cl_json["entries"].as_array().unwrap().len(), 2);
}

#[test]
fn test_otlp_grpc_live_tonic_server_export_and_partial_success() {
    use std::convert::Infallible;
    use std::future::Future;
    use std::pin::Pin;
    use std::task::{Context, Poll};
    use tonic::body::BoxBody;
    use tonic::codec::ProstCodec;
    use tonic::server::{NamedService, UnaryService};

    #[derive(Clone)]
    struct MockOtlpGrpcSvc {
        calls: Arc<AtomicUsize>,
        captured_auth: Arc<Mutex<String>>,
        captured_proj: Arc<Mutex<String>>,
        captured_records: Arc<AtomicUsize>,
    }

    impl NamedService for MockOtlpGrpcSvc {
        const NAME: &'static str = "opentelemetry.proto.collector.logs.v1.LogsService";
    }

    struct ExportUnaryMethod(MockOtlpGrpcSvc);

    impl UnaryService<ExportLogsServiceRequest> for ExportUnaryMethod {
        type Response = ExportLogsServiceResponse;
        type Future = Pin<
            Box<dyn Future<Output = Result<tonic::Response<Self::Response>, tonic::Status>> + Send>,
        >;

        fn call(&mut self, request: tonic::Request<ExportLogsServiceRequest>) -> Self::Future {
            let svc = self.0.clone();
            Box::pin(async move {
                let n = svc.calls.fetch_add(1, Ordering::SeqCst);
                if n == 0 {
                    return Err(tonic::Status::resource_exhausted("quota burst"));
                }
                if let Some(a) = request.metadata().get("authorization") {
                    *svc.captured_auth.lock().unwrap() = a.to_str().unwrap_or("").to_string();
                }
                if let Some(p) = request.metadata().get("x-goog-user-project") {
                    *svc.captured_proj.lock().unwrap() = p.to_str().unwrap_or("").to_string();
                }
                let count: usize = request
                    .into_inner()
                    .resource_logs
                    .iter()
                    .flat_map(|rl| &rl.scope_logs)
                    .map(|sl| sl.log_records.len())
                    .sum();
                svc.captured_records.fetch_add(count, Ordering::SeqCst);
                Ok(tonic::Response::new(ExportLogsServiceResponse {
                    partial_success: Some(ExportLogsPartialSuccess {
                        rejected_log_records: 1,
                        error_message: "1 test record dropped".to_string(),
                    }),
                }))
            })
        }
    }

    impl tonic::codegen::Service<http::Request<BoxBody>> for MockOtlpGrpcSvc {
        type Response = http::Response<BoxBody>;
        type Error = Infallible;
        type Future = Pin<Box<dyn Future<Output = Result<Self::Response, Self::Error>> + Send>>;

        fn poll_ready(&mut self, _cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
            Poll::Ready(Ok(()))
        }

        fn call(&mut self, req: http::Request<BoxBody>) -> Self::Future {
            let svc = self.clone();
            Box::pin(async move {
                if req.uri().path() == "/opentelemetry.proto.collector.logs.v1.LogsService/Export" {
                    let mut grpc = tonic::server::Grpc::new(ProstCodec::<
                        ExportLogsServiceResponse,
                        ExportLogsServiceRequest,
                    >::default());
                    Ok(grpc.unary(ExportUnaryMethod(svc), req).await)
                } else {
                    Ok(http::Response::builder()
                        .status(404)
                        .body(tonic::body::empty_body())
                        .unwrap())
                }
            })
        }
    }

    let calls = Arc::new(AtomicUsize::new(0));
    let captured_auth = Arc::new(Mutex::new(String::new()));
    let captured_proj = Arc::new(Mutex::new(String::new()));
    let captured_records = Arc::new(AtomicUsize::new(0));

    let svc = MockOtlpGrpcSvc {
        calls: Arc::clone(&calls),
        captured_auth: Arc::clone(&captured_auth),
        captured_proj: Arc::clone(&captured_proj),
        captured_records: Arc::clone(&captured_records),
    };

    let std_listener = TcpListener::bind("127.0.0.1:0").unwrap();
    std_listener.set_nonblocking(true).unwrap();
    let grpc_port = std_listener.local_addr().unwrap().port();
    let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();

    let server_thread = std::thread::spawn(move || {
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        rt.block_on(async move {
            let tokio_listener = tokio::net::TcpListener::from_std(std_listener).unwrap();
            let incoming = tokio_stream::wrappers::TcpListenerStream::new(tokio_listener);
            let _ = tonic::transport::Server::builder()
                .add_service(svc)
                .serve_with_incoming_shutdown(incoming, async {
                    let _ = shutdown_rx.await;
                })
                .await;
        });
    });

    let tok_server = MockHttpServer::start(|_| {
        MockHttpReply::ok_json(&json!({ "access_token": "mock-grpc-token", "expires_in": 600 }))
    });
    let tok_cfg = ExporterConfig {
        node_token_broker: Some(format!("{}/token", tok_server.base_url())),
        ..Default::default()
    };
    let coords = ClusterCoordinates {
        project_id: "grpc-proj-1".to_string(),
        cluster_name: "grpc-cluster".to_string(),
        location: "us-central1".to_string(),
    };
    let sink = OtlpSink::new(
        &format!("http://127.0.0.1:{grpc_port}"),
        coords,
        Arc::new(CloudTokenProvider::from_config(&tok_cfg, None)),
        None,
    )
    .expect("create OtlpSink over gRPC");

    let now = UNIX_EPOCH + Duration::from_secs(1_760_000_000);
    let e1 = build_startup_marker_entry(&ShardSpec::default(), now).unwrap();
    let e2 =
        build_startup_marker_entry(&ShardSpec::default(), now + Duration::from_secs(1)).unwrap();
    let batch = vec![e1, e2];
    let metrics = MetricsRegistry::new();

    // First call returns gRPC RESOURCE_EXHAUSTED (8 -> HTTP "429" retryable).
    let first = sink.export_batch(&batch, &metrics);
    assert!(matches!(
        first,
        BatchExportOutcome::RetryableFailure { ref status_code, .. } if status_code == "429"
    ));

    // Second call succeeds with 1 partial_success rejected record (2 - 1 = 1 accepted).
    let second = sink.export_batch(&batch, &metrics);
    assert_eq!(second, BatchExportOutcome::Success { sent_entries: 1 });
    assert_eq!(
        captured_auth.lock().unwrap().as_str(),
        "Bearer mock-grpc-token"
    );
    assert_eq!(captured_proj.lock().unwrap().as_str(), "grpc-proj-1");
    assert_eq!(captured_records.load(Ordering::SeqCst), 2);
    let prom = metrics.render_prometheus();
    assert!(prom.contains("stackdriver_sink_request_count{code=\"429\"} 1"));
    assert!(prom.contains("stackdriver_sink_request_count{code=\"200\"} 1"));
    assert!(prom.contains("stackdriver_sink_successfully_sent_entry_count 1"));

    let _ = shutdown_tx.send(());
    let _ = server_thread.join();
}

#[test]
fn test_node_token_broker_adc_and_metadata_discovery() {
    let dir = tempfile::tempdir().unwrap();
    let ksa_path = dir.path().join("ksa.jwt");
    fs::write(&ksa_path, "projected-ksa-jwt-token-xyz\n").unwrap();

    let server = MockHttpServer::start(move |req| match req.path.as_str() {
        "/computeMetadata/v1/project/project-id" => MockHttpReply::ok_text("gke-prod-proj"),
        "/computeMetadata/v1/instance/attributes/cluster-name" => {
            MockHttpReply::ok_text("linked-runner-01")
        }
        "/computeMetadata/v1/instance/attributes/cluster-location" => {
            MockHttpReply::ok_text("us-east4")
        }
        "/v1/token" => {
            let body: Value = serde_json::from_slice(&req.body).unwrap();
            assert_eq!(
                body["subjectToken"].as_str(),
                Some("projected-ksa-jwt-token-xyz")
            );
            MockHttpReply::ok_json(&json!({
                "access_token": "mock-exchanged-sts-token",
                "expires_in": 3600
            }))
        }
        _ => MockHttpReply::status(404, "unknown"),
    });

    let adc_path = dir.path().join("adc.json");
    fs::write(
        &adc_path,
        serde_json::to_vec(&json!({
            "type": "external_account",
            "audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/p",
            "subject_token_type": "kubernetes_service_account_token",
            "token_url": format!("{}/v1/token", server.base_url()),
            "credential_source": { "file": ksa_path.to_str().unwrap() }
        }))
        .unwrap(),
    )
    .unwrap();

    let cfg = ExporterConfig::default();
    let coords = resolve_cluster_coordinates(&cfg, Some(&server.base_url())).unwrap();
    assert_eq!(coords.project_id.as_str(), "gke-prod-proj");
    assert_eq!(coords.cluster_name.as_str(), "linked-runner-01");
    assert_eq!(coords.location.as_str(), "us-east4");

    let broker_server = MockHttpServer::start(|_| {
        MockHttpReply::ok_json(&json!({ "access_token": "mock-broker-direct", "expires_in": 600 }))
    });
    let broker_cfg = ExporterConfig {
        node_token_broker: Some(format!("{}/token", broker_server.base_url())),
        ..Default::default()
    };
    let provider = CloudTokenProvider::from_config(&broker_cfg, Some(&server.base_url()));
    assert_eq!(
        provider.access_token().unwrap().as_deref(),
        Some("mock-broker-direct")
    );

    let kube_tok = KubeTokenSource::new(Some(ksa_path.to_string_lossy().into_owned()));
    assert_eq!(
        kube_tok.bearer_token().unwrap().as_deref(),
        Some("projected-ksa-jwt-token-xyz")
    );
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

struct MockHttpRequest {
    path: String,
    body: Vec<u8>,
}

struct MockHttpReply {
    status: u16,
    content_type: &'static str,
    body: Vec<u8>,
}

impl MockHttpReply {
    fn ok_text(text: &str) -> Self {
        Self {
            status: 200,
            content_type: "text/plain",
            body: text.as_bytes().to_vec(),
        }
    }

    fn ok_json(val: &Value) -> Self {
        Self {
            status: 200,
            content_type: "application/json",
            body: serde_json::to_vec(val).unwrap(),
        }
    }

    fn ok_bytes(content_type: &'static str, body: Vec<u8>) -> Self {
        Self {
            status: 200,
            content_type,
            body,
        }
    }

    fn status(code: u16, msg: &str) -> Self {
        Self {
            status: code,
            content_type: "application/json",
            body: msg.as_bytes().to_vec(),
        }
    }
}

struct MockHttpServer {
    port: u16,
    stop: Arc<AtomicU8>,
    handle: Option<std::thread::JoinHandle<()>>,
}

impl MockHttpServer {
    fn start<F>(handler: F) -> Self
    where
        F: Fn(MockHttpRequest) -> MockHttpReply + Send + Sync + 'static,
    {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        listener.set_nonblocking(true).unwrap();
        let stop = Arc::new(AtomicU8::new(0));
        let stop_clone = Arc::clone(&stop);
        let handler = Arc::new(handler);

        let handle = std::thread::spawn(move || {
            while stop_clone.load(Ordering::Acquire) == 0 {
                match listener.accept() {
                    Ok((stream, _)) => {
                        let h = Arc::clone(&handler);
                        std::thread::spawn(move || serve_one_http_conn(stream, h.as_ref()));
                    }
                    Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                        std::thread::sleep(Duration::from_millis(5));
                    }
                    Err(_) => break,
                }
            }
        });

        Self {
            port,
            stop,
            handle: Some(handle),
        }
    }

    fn base_url(&self) -> String {
        format!("http://127.0.0.1:{}", self.port)
    }
}

impl Drop for MockHttpServer {
    fn drop(&mut self) {
        self.stop.store(1, Ordering::Release);
        if let Some(h) = self.handle.take() {
            let _ = h.join();
        }
    }
}

fn serve_one_http_conn<F>(stream: TcpStream, handler: &F)
where
    F: Fn(MockHttpRequest) -> MockHttpReply,
{
    let _ = stream.set_read_timeout(Some(Duration::from_secs(5)));
    let _ = stream.set_write_timeout(Some(Duration::from_secs(5)));
    let Ok(mut writer) = stream.try_clone() else {
        return;
    };
    let mut reader = BufReader::new(stream);
    let mut request_line = String::new();
    if reader.read_line(&mut request_line).unwrap_or(0) == 0 {
        return;
    }
    let path = request_line
        .split_whitespace()
        .nth(1)
        .unwrap_or("/")
        .to_string();

    let mut content_length = 0usize;
    loop {
        let mut hdr = String::new();
        if reader.read_line(&mut hdr).unwrap_or(0) == 0 || hdr == "\r\n" || hdr == "\n" {
            break;
        }
        if let Some((k, v)) = hdr.split_once(':')
            && k.trim().eq_ignore_ascii_case("content-length")
        {
            content_length = v.trim().parse::<usize>().unwrap_or(0);
        }
    }

    let mut body = vec![0u8; content_length];
    if content_length > 0 {
        let _ = reader.read_exact(&mut body);
    }

    let reply = handler(MockHttpRequest { path, body });
    let reason = match reply.status {
        200 => "OK",
        410 => "Gone",
        422 => "Unprocessable Entity",
        429 => "Too Many Requests",
        _ => "Error",
    };
    let header = format!(
        "HTTP/1.1 {} {reason}\r\nContent-Type: {}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        reply.status,
        reply.content_type,
        reply.body.len()
    );
    let _ = writer.write_all(header.as_bytes());
    if !reply.body.is_empty() {
        let _ = writer.write_all(&reply.body);
    }
    let _ = writer.flush();
}
