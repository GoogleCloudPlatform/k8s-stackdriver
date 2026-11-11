//! Formal refinement, algebraic sharding bijection, WatchList premature-EOF regression,
//! RFC3339 calendar boundary, and Go v0.4.0 Prometheus/sink-opts parity tests.

#![forbid(unsafe_code)]

use event_exporter_rs::auth::{CloudTokenProvider, KubeTokenSource, resolve_logging_endpoint};
use event_exporter_rs::cli::ExporterConfig;
use event_exporter_rs::enricher::{
    ClusterCoordinates, enrich_event_object, format_unix_nanos_rfc3339, parse_rfc3339_unix_nanos,
};
use event_exporter_rs::k8s_stream::{
    InitialSyncLatch, KubeClientConfig, paginated_pod_relist, run_pod_reflector_loop,
};
use event_exporter_rs::metrics::MetricsRegistry;
use event_exporter_rs::pod_cache::{
    CompactOwner, ControllerKind, OwnerRefExtract, PodKey, PodMetadataExtract, PodOwnerTable,
    RelevantPodLabels, ResolvedPodOwner,
};
use event_exporter_rs::shard::{ShardMode, ShardSpec};
use event_exporter_rs::sink::{BatchExportOutcome, CloudLoggingSink, EventSink};
use serde_json::{Map, Value, json};
use std::collections::{BTreeMap, HashSet};
use std::io::{BufRead, BufReader, Read, Write};
use std::net::TcpListener;
use std::sync::Arc;
use std::sync::atomic::{AtomicU8, AtomicUsize, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

fn make_pod_spec(
    variant: usize,
    ns: &str,
    name: &str,
) -> (PodMetadataExtract, Option<ResolvedPodOwner>) {
    match variant {
        // 0: Naked pod (no owner -> must evict any prior cached entry)
        0 => (
            PodMetadataExtract {
                name: name.to_string(),
                namespace: ns.to_string(),
                resource_version: "1".to_string(),
                labels: RelevantPodLabels::default(),
                owner_references: Vec::new(),
            },
            None,
        ),
        // 1: Deployment via ReplicaSet + pod-template-hash
        1 => (
            PodMetadataExtract {
                name: name.to_string(),
                namespace: ns.to_string(),
                resource_version: "2".to_string(),
                labels: RelevantPodLabels {
                    pod_template_hash: Some("7f9c8d".to_string()),
                    ..Default::default()
                },
                owner_references: vec![OwnerRefExtract {
                    kind: "ReplicaSet".to_string(),
                    name: "frontend-7f9c8d".to_string(),
                }],
            },
            Some(ResolvedPodOwner {
                kind: ControllerKind::Deployment,
                name: "frontend".to_string(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
        ),
        // 2: StatefulSet
        2 => (
            PodMetadataExtract {
                name: name.to_string(),
                namespace: ns.to_string(),
                resource_version: "3".to_string(),
                labels: RelevantPodLabels::default(),
                owner_references: vec![OwnerRefExtract {
                    kind: "StatefulSet".to_string(),
                    name: "etcd-main".to_string(),
                }],
            },
            Some(ResolvedPodOwner {
                kind: ControllerKind::StatefulSet,
                name: "etcd-main".to_string(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
        ),
        // 3..=6: JobSet variants with 4 distinct (restart_attempt, jobset_uid) pairs
        // With max_inline_jobsets = 2, pairs #1 and #2 use inline u16 IDs, while pairs #3 and #4
        // spill across JOBSET_OVERFLOW_SENTINEL (u16::MAX) into jobset_overflow!
        v @ 3..=6 => {
            let idx = v - 2;
            let js_name = format!("tpu-train-{idx}");
            let attempt = format!("{idx}");
            let uid = format!("js-uid-{idx}");
            (
                PodMetadataExtract {
                    name: name.to_string(),
                    namespace: ns.to_string(),
                    resource_version: "4".to_string(),
                    labels: RelevantPodLabels {
                        jobset_name: Some(js_name.clone()),
                        jobset_restart_attempt: Some(attempt.clone()),
                        jobset_uid: Some(uid.clone()),
                        ..Default::default()
                    },
                    owner_references: vec![OwnerRefExtract {
                        kind: "Job".to_string(),
                        name: format!("{js_name}-worker-0"),
                    }],
                },
                Some(ResolvedPodOwner {
                    kind: ControllerKind::JobSet,
                    name: js_name,
                    jobset_restart_attempt: Some(attempt),
                    jobset_uid: Some(uid),
                }),
            )
        }
        // 7: CronJob via timestamped Job suffix
        7 => (
            PodMetadataExtract {
                name: name.to_string(),
                namespace: ns.to_string(),
                resource_version: "5".to_string(),
                labels: RelevantPodLabels::default(),
                owner_references: vec![OwnerRefExtract {
                    kind: "Job".to_string(),
                    name: "nightly-compact-28543210".to_string(),
                }],
            },
            Some(ResolvedPodOwner {
                kind: ControllerKind::CronJob,
                name: "nightly-compact".to_string(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
        ),
        // 8: DaemonSet
        _ => (
            PodMetadataExtract {
                name: name.to_string(),
                namespace: ns.to_string(),
                resource_version: "6".to_string(),
                labels: RelevantPodLabels::default(),
                owner_references: vec![OwnerRefExtract {
                    kind: "DaemonSet".to_string(),
                    name: "fluentbit-gke".to_string(),
                }],
            },
            Some(ResolvedPodOwner {
                kind: ControllerKind::DaemonSet,
                name: "fluentbit-gke".to_string(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
        ),
    }
}

#[test]
fn test_pod_owner_table_exhaustive_refinement_over_sentinel() {
    // Formal memory layout bound: 16B PodKey + 8B CompactOwner = 24B flat slot.
    assert_eq!(std::mem::size_of::<(PodKey, CompactOwner)>(), 24);

    let sys_ns: HashSet<String> = ["kube-system".to_string()].into_iter().collect();

    // Part 1: Exhaustive small-scope 3-step transition check across sentinel spill/overwrite/retain.
    // 7 actions per step -> 7^3 = 343 sequences, checking full observational equivalence at every step.
    let pods = [("default", "p0"), ("ml", "p1"), ("ml", "p2")];
    for s0 in 0..7usize {
        for s1 in 0..7usize {
            for s2 in 0..7usize {
                let table = PodOwnerTable::with_max_inline_jobsets(sys_ns.clone(), 2);
                let mut model: BTreeMap<(String, String), ResolvedPodOwner> = BTreeMap::new();

                for (step_idx, &act) in [s0, s1, s2].iter().enumerate() {
                    let (ns, name) = pods[step_idx % pods.len()];
                    match act {
                        0..=4 => {
                            // Map 0..=4 onto naked(0), Deployment(1), and 3 distinct JobSets(3,4,5)
                            // so inline cap=2 is exceeded within 3 steps.
                            let variant = match act {
                                0 => 0,
                                1 => 1,
                                2 => 3,
                                3 => 4,
                                _ => 5,
                            };
                            let (pod_meta, expected) = make_pod_spec(variant, ns, name);
                            table.upsert_pod_metadata(&pod_meta, None);
                            match expected {
                                Some(owner) => {
                                    model.insert((ns.to_string(), name.to_string()), owner);
                                }
                                None => {
                                    model.remove(&(ns.to_string(), name.to_string()));
                                }
                            }
                        }
                        5 => {
                            table.remove_pod(ns, name, None);
                            model.remove(&(ns.to_string(), name.to_string()));
                        }
                        _ => {
                            // Retain only p0
                            let keep_key = table.compute_key("default", "p0");
                            let live: HashSet<PodKey> = [keep_key].into_iter().collect();
                            table.retain_only(&live, None);
                            model.retain(|(k_ns, k_name), _| k_ns == "default" && k_name == "p0");
                        }
                    }

                    assert_eq!(table.len(), model.len());
                    for &(q_ns, q_name) in &pods {
                        assert_eq!(
                            table.lookup(q_ns, q_name, None),
                            model.get(&(q_ns.to_string(), q_name.to_string())).cloned(),
                            "refinement mismatch on sequence [{s0}, {s1}, {s2}] at step {step_idx}"
                        );
                    }
                }
            }
        }
    }

    // Part 2: 10,000-step deterministic PRNG state-machine trace with metrics conservation.
    let metrics = MetricsRegistry::new();
    let table = PodOwnerTable::with_max_inline_jobsets(sys_ns, 2);
    let mut model: BTreeMap<(String, String), ResolvedPodOwner> = BTreeMap::new();
    let universe: Vec<(String, String)> = (0..12)
        .map(|i| {
            let ns = if i == 11 {
                "kube-system".to_string()
            } else {
                format!("ns-{}", i % 3)
            };
            (ns, format!("pod-{i}"))
        })
        .collect();

    let mut rng: u64 = 0x9E37_79B9_7F4A_7C15;
    let mut next_u64 = || -> u64 {
        rng ^= rng << 13;
        rng ^= rng >> 7;
        rng ^= rng << 17;
        rng
    };

    for step in 0..10_000usize {
        let pick = (next_u64() as usize) % universe.len();
        let (ns, name) = &universe[pick];
        let op = next_u64() % 20;

        if op < 14 {
            let variant = (next_u64() as usize) % 9;
            let (pod_meta, expected) = make_pod_spec(variant, ns, name);
            table.upsert_pod_metadata(&pod_meta, Some(&metrics));
            if ns != "kube-system" {
                match expected {
                    Some(owner) => {
                        model.insert((ns.clone(), name.clone()), owner);
                    }
                    None => {
                        model.remove(&(ns.clone(), name.clone()));
                    }
                }
            }
        } else if op < 18 {
            table.remove_pod(ns, name, Some(&metrics));
            model.remove(&(ns.clone(), name.clone()));
        } else {
            let mask = next_u64();
            let mut live_keys = HashSet::new();
            let mut kept_pairs = HashSet::new();
            for (idx, (u_ns, u_name)) in universe.iter().enumerate() {
                if u_ns != "kube-system" && ((mask >> idx) & 1) == 1 {
                    live_keys.insert(table.compute_key(u_ns, u_name));
                    kept_pairs.insert((u_ns.clone(), u_name.clone()));
                }
            }
            table.retain_only(&live_keys, Some(&metrics));
            model.retain(|k, _| kept_pairs.contains(k));
        }

        assert_eq!(table.len(), model.len(), "len mismatch at step {step}");
        for (q_ns, q_name) in &universe {
            let got = table.lookup(q_ns, q_name, Some(&metrics));
            let want = model.get(&(q_ns.clone(), q_name.clone())).cloned();
            assert_eq!(
                got, want,
                "lookup mismatch for {q_ns}/{q_name} at step {step}"
            );
        }
    }

    // Verify podlabel_cache_ops_count conservation: total adds - total evicts == live len.
    let prom = metrics.render_prometheus();
    let parse_op = |op: &str| -> u64 {
        let prefix = format!("podlabel_cache_ops_count{{operation=\"{op}\"}} ");
        prom.lines()
            .find_map(|l| l.strip_prefix(&prefix))
            .and_then(|v| v.trim().parse::<u64>().ok())
            .expect("metric line must exist")
    };
    let adds = parse_op("add");
    let evicts = parse_op("evict");
    assert_eq!(adds.saturating_sub(evicts) as usize, model.len());
}

#[test]
fn test_shard_partition_bijection_and_pod_event_colocality() {
    for shard_count in 1..=16u32 {
        let client_shards: Vec<ShardSpec> = (0..shard_count)
            .map(|idx| ShardSpec::resolve(ShardMode::Client, shard_count, Some(idx), None).unwrap())
            .collect();
        let server_shards: Vec<ShardSpec> = (0..shard_count)
            .map(|idx| ShardSpec::resolve(ShardMode::Server, shard_count, Some(idx), None).unwrap())
            .collect();

        // Unique leader election across all shard counts.
        let leaders = client_shards.iter().filter(|s| s.is_leader()).count();
        assert_eq!(leaders, 1);
        assert!(client_shards[0].is_leader());

        for case_id in 0..2_000usize {
            let ns = match case_id % 6 {
                0 => String::new(),
                1 => "default".to_string(),
                2 => "kube-system".to_string(),
                3 => format!("tenant-{}", case_id % 23),
                4 => "ns-with/slash:and-dash".to_string(),
                _ => format!("λ-unicode-{}", case_id % 11),
            };
            let pod_name = match case_id % 5 {
                0 => "pod-0".to_string(),
                1 => format!("stateful-worker-{case_id}"),
                2 => "identical-pod-name".to_string(),
                3 => format!("jobset-tpu-{}-0-x9q", case_id % 31),
                _ => String::new(),
            };
            let uid = format!("event-uid-{case_id:06x}");

            // Pod partition bijection across both client-side and KEP-5866 server-side modes.
            let pod_owners: Vec<u32> = client_shards
                .iter()
                .filter(|s| s.owns_pod(&ns, &pod_name))
                .map(|s| s.shard_index)
                .collect();
            assert_eq!(
                pod_owners.len(),
                1,
                "Pod {ns}/{pod_name} must map to exactly 1 client shard for N={shard_count}"
            );

            let server_pod_owners: Vec<u32> = server_shards
                .iter()
                .filter(|s| s.owns_pod(&ns, &pod_name))
                .map(|s| s.shard_index)
                .collect();
            assert_eq!(
                server_pod_owners.len(),
                1,
                "Pod {ns}/{pod_name} must map to exactly 1 server shard even if apiserver returns unfiltered HTTP 200"
            );

            // Same-namespace Pod Event: bijection + exact co-locality with Pod shard in BOTH modes!
            if !ns.is_empty() && !pod_name.is_empty() {
                let ev_owners: Vec<u32> = client_shards
                    .iter()
                    .filter(|s| s.owns_event(&ns, "Pod", &ns, &pod_name, &uid))
                    .map(|s| s.shard_index)
                    .collect();
                assert_eq!(ev_owners, pod_owners);

                let srv_ev_owners: Vec<u32> = server_shards
                    .iter()
                    .filter(|s| s.owns_event(&ns, "Pod", &ns, &pod_name, &uid))
                    .map(|s| s.shard_index)
                    .collect();
                assert_eq!(srv_ev_owners, server_pod_owners);
            }

            // Cross-namespace spoofed Pod Event (RBAC boundary: event_ns != involved_ns):
            // Must partition bijectively without duplicates in both modes.
            let spoof_ev_owners: Vec<u32> = client_shards
                .iter()
                .filter(|s| s.owns_event("attacker-ns", "Pod", "victim-ns", &pod_name, &uid))
                .map(|s| s.shard_index)
                .collect();
            assert_eq!(spoof_ev_owners.len(), 1);
            let srv_spoof_owners = server_shards
                .iter()
                .filter(|s| s.owns_event("attacker-ns", "Pod", "victim-ns", &pod_name, &uid))
                .count();
            assert_eq!(srv_spoof_owners, 1);

            // Node and Cluster-scoped Events: partition bijection.
            let node_name = format!("gke-pool-node-{}", case_id % 37);
            let node_owners = client_shards
                .iter()
                .filter(|s| s.owns_event("default", "Node", "", &node_name, &uid))
                .count();
            assert_eq!(node_owners, 1);

            let deploy_owners = client_shards
                .iter()
                .filter(|s| s.owns_event(&ns, "Deployment", &ns, "web", &uid))
                .count();
            assert_eq!(deploy_owners, 1);
        }
    }
}

#[test]
fn test_watchlist_premature_eof_before_bookmark_regression() {
    // Pre-populate a stale zombie Pod that must be evicted only once a complete WatchList snapshot
    // (terminated by k8s.io/initial-events-end="true") succeeds.
    let pod_table = Arc::new(PodOwnerTable::new(HashSet::new()));
    pod_table.upsert_pod_metadata(
        &PodMetadataExtract {
            name: "stale-zombie-pod".to_string(),
            namespace: "default".to_string(),
            resource_version: "1".to_string(),
            labels: RelevantPodLabels::default(),
            owner_references: vec![OwnerRefExtract {
                kind: "DaemonSet".to_string(),
                name: "old-agent".to_string(),
            }],
        },
        None,
    );
    assert_eq!(pod_table.len(), 1);

    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let addr = listener.local_addr().unwrap();
    let wl_attempts = Arc::new(AtomicUsize::new(0));
    let wl_attempts_srv = Arc::clone(&wl_attempts);
    let stop_flag = Arc::new(AtomicU8::new(0));
    let stop_srv = Arc::clone(&stop_flag);

    let srv_handle = std::thread::spawn(move || {
        listener.set_nonblocking(true).unwrap();
        while stop_srv.load(Ordering::Acquire) == 0 {
            let Ok((mut stream, _)) = listener.accept() else {
                std::thread::sleep(Duration::from_millis(10));
                continue;
            };
            stream
                .set_read_timeout(Some(Duration::from_secs(2)))
                .unwrap();
            let mut reader = BufReader::new(&stream);
            let mut req_line = String::new();
            if reader.read_line(&mut req_line).unwrap_or(0) == 0 {
                continue;
            }
            loop {
                let mut hdr = String::new();
                if reader.read_line(&mut hdr).unwrap_or(0) == 0 || hdr == "\r\n" {
                    break;
                }
            }
            let path = req_line.split_whitespace().nth(1).unwrap_or("");
            if path.contains("sendInitialEvents=true") {
                let attempt = wl_attempts_srv.fetch_add(1, Ordering::SeqCst);
                let pod_high_rv = serde_json::to_string(&json!({
                    "type": "ADDED",
                    "object": {
                        "metadata": {
                            "name": "pod-a-high-rv",
                            "namespace": "default",
                            "resourceVersion": "5000",
                            "labels": { "pod-template-hash": "abc" },
                            "ownerReferences": [{ "kind": "ReplicaSet", "name": "web-abc" }]
                        }
                    }
                }))
                .unwrap();

                if attempt == 0 {
                    // Premature EOF after 1st ADDED frame (rv="5000") WITHOUT initial-events-end BOOKMARK!
                    let body = format!("{pod_high_rv}\n");
                    let resp = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                        body.len(),
                        body
                    );
                    let _ = stream.write_all(resp.as_bytes());
                } else {
                    // Second attempt: emits pod-a-high-rv (rv=5000), THEN pod-z-low-rv (rv=100, later
                    // in etcd key order despite lower RV!), THEN the initial-events-end BOOKMARK.
                    let pod_low_rv = serde_json::to_string(&json!({
                        "type": "ADDED",
                        "object": {
                            "metadata": {
                                "name": "pod-z-low-rv",
                                "namespace": "default",
                                "resourceVersion": "100",
                                "ownerReferences": [{ "kind": "StatefulSet", "name": "db" }]
                            }
                        }
                    }))
                    .unwrap();
                    let bookmark = serde_json::to_string(&json!({
                        "type": "BOOKMARK",
                        "object": {
                            "metadata": {
                                "resourceVersion": "5005",
                                "annotations": { "k8s.io/initial-events-end": "true" }
                            }
                        }
                    }))
                    .unwrap();
                    let body = format!("{pod_high_rv}\n{pod_low_rv}\n{bookmark}\n");
                    let resp = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                        body.len(),
                        body
                    );
                    let _ = stream.write_all(resp.as_bytes());
                }
            } else {
                // Subsequent incremental watch at resourceVersion=5005: return empty 200 after brief pause.
                std::thread::sleep(Duration::from_millis(50));
                let resp = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 0\r\nConnection: close\r\n\r\n";
                let _ = stream.write_all(resp.as_bytes());
            }
        }
    });

    let metrics = Arc::new(MetricsRegistry::new());
    let sync_latch = Arc::new(InitialSyncLatch::new());
    let kube = KubeClientConfig {
        api_base: format!("http://{addr}"),
        ca_cert_path: None,
        token_source: Arc::new(KubeTokenSource::new(None)),
        shard: ShardSpec::default(),
        enable_watch_list: true,
        list_page_size: 100,
        event_label_selector: None,
    };

    let tbl_bg = Arc::clone(&pod_table);
    let met_bg = Arc::clone(&metrics);
    let latch_bg = Arc::clone(&sync_latch);
    let stop_bg = Arc::clone(&stop_flag);
    let reflector = std::thread::spawn(move || {
        run_pod_reflector_loop(kube, tbl_bg, met_bg, latch_bg, stop_bg);
    });

    assert!(
        sync_latch.wait_timeout(Duration::from_secs(5), &stop_flag),
        "InitialSyncLatch must open on retry #2 once BOOKMARK arrives"
    );
    assert!(
        wl_attempts.load(Ordering::SeqCst) >= 2,
        "Truncated WatchList without bookmark must retry WatchList instead of switching to incremental watch"
    );
    assert_eq!(
        pod_table.lookup("default", "stale-zombie-pod", None),
        None,
        "Stale pod must be evicted once complete WatchList bookmark arrives"
    );
    assert_eq!(
        pod_table
            .lookup("default", "pod-a-high-rv", None)
            .map(|o| (o.kind, o.name)),
        Some((ControllerKind::Deployment, "web".to_string()))
    );
    assert_eq!(
        pod_table
            .lookup("default", "pod-z-low-rv", None)
            .map(|o| (o.kind, o.name)),
        Some((ControllerKind::StatefulSet, "db".to_string())),
        "Pod with lower RV ordered later in WatchList snapshot must not be skipped"
    );

    stop_flag.store(1, Ordering::Release);
    let _ = reflector.join();
    let _ = srv_handle.join();

    // Verify Go nil-slice `"items": null` List responses on empty/idle clusters parse cleanly.
    let null_listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let null_addr = null_listener.local_addr().unwrap();
    let null_srv = std::thread::spawn(move || {
        let (mut stream, _) = null_listener.accept().unwrap();
        let mut reader = BufReader::new(&stream);
        loop {
            let mut line = String::new();
            if reader.read_line(&mut line).unwrap_or(0) == 0 || line == "\r\n" {
                break;
            }
        }
        let body = r#"{"kind":"PartialObjectMetadataList","metadata":{"resourceVersion":"9001"},"items":null}"#;
        let resp = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        let _ = stream.write_all(resp.as_bytes());
    });
    let null_stop = AtomicU8::new(0);
    let mut null_shard = ShardSpec::default();
    let null_kube = KubeClientConfig {
        api_base: format!("http://{null_addr}"),
        ca_cert_path: None,
        token_source: Arc::new(KubeTokenSource::new(None)),
        shard: null_shard,
        enable_watch_list: false,
        list_page_size: 100,
        event_label_selector: None,
    };
    let rv = paginated_pod_relist(
        &null_kube,
        &mut null_shard,
        &pod_table,
        &metrics,
        &null_stop,
    )
    .expect("Kubernetes List with \"items\": null must deserialize cleanly");
    assert_eq!(rv, "9001");
    assert_eq!(pod_table.len(), 0);
    let _ = null_srv.join();
}

#[test]
fn test_rfc3339_calendar_bounds_and_go_sink_opts_flag_parity() {
    // Reject impossible Gregorian dates (matching Go time.Parse(time.RFC3339Nano, ...)).
    for invalid in [
        "2026-02-29T00:00:00Z", // 2026 is not a leap year
        "2100-02-29T00:00:00Z", // century non-leap year
        "2026-02-30T12:00:00Z",
        "2026-02-31T12:00:00Z",
        "2026-04-31T00:00:00Z",
        "2026-06-31T00:00:00Z",
        "2026-09-31T00:00:00Z",
        "2026-11-31T00:00:00Z",
        "2026-00-10T00:00:00Z",
        "2026-13-10T00:00:00Z",
        "2026-01-00T00:00:00Z",
    ] {
        assert_eq!(
            parse_rfc3339_unix_nanos(invalid),
            None,
            "must reject invalid date {invalid}"
        );
    }

    // Accept valid leap days and round-trip cleanly.
    for valid in [
        "2024-02-29T23:59:59.987654321Z",
        "2000-02-29T00:00:00Z",
        "2026-03-31T18:30:00+02:00",
    ] {
        assert!(
            parse_rfc3339_unix_nanos(valid).is_some(),
            "must accept valid date {valid}"
        );
    }
    let leap_nanos = parse_rfc3339_unix_nanos("2024-02-29T23:59:59.987654321Z").unwrap();
    assert_eq!(
        format_unix_nanos_rfc3339(leap_nanos),
        "2024-02-29T23:59:59.987654321Z"
    );

    // Go sdSinkFactory defines `-universeDomain` (camelCase) inside `-sink-opts`, and Go main.go
    // defaults `-system-namespaces` to `"kube-system,gke-connect"` when omitted.
    let cfg = ExporterConfig::from_args([
        "-sink-opts=-stackdriver-resource-model=new -universeDomain=apis-tpclp.goog -flush-delay=5s -max-buffer-size=100 -max-concurrency=10",
    ])
    .expect("Go -universeDomain camelCase flag inside -sink-opts must parse");
    assert_eq!(cfg.universe_domain.as_deref(), Some("apis-tpclp.goog"));
    assert!(cfg.system_namespaces.contains("kube-system"));
    assert!(cfg.system_namespaces.contains("gke-connect"));
    assert_eq!(cfg.system_namespaces.len(), 2);
    assert_eq!(
        resolve_logging_endpoint(&cfg),
        "https://logging.apis-tpclp.goog/"
    );
}

#[test]
fn test_gke_manifest_prometheus_whitelist_and_delivery_freshness_latency() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let addr = listener.local_addr().unwrap();
    let calls = Arc::new(AtomicUsize::new(0));
    let calls_srv = Arc::clone(&calls);

    let srv = std::thread::spawn(move || {
        for _ in 0..2 {
            let (mut stream, _) = listener.accept().unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(2)))
                .unwrap();
            let mut reader = BufReader::new(&stream);
            let mut content_len = 0usize;
            loop {
                let mut line = String::new();
                if reader.read_line(&mut line).unwrap_or(0) == 0 || line == "\r\n" {
                    break;
                }
                if let Some(rest) = line.to_ascii_lowercase().strip_prefix("content-length:") {
                    content_len = rest.trim().parse::<usize>().unwrap_or(0);
                }
            }
            if content_len > 0 {
                let mut body = vec![0u8; content_len];
                let _ = reader.read_exact(&mut body);
            }
            let n = calls_srv.fetch_add(1, Ordering::SeqCst);
            if n == 0 {
                let msg = "{\"error\":\"rate_limited\"}";
                let resp = format!(
                    "HTTP/1.1 429 Too Many Requests\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    msg.len(),
                    msg
                );
                let _ = stream.write_all(resp.as_bytes());
            } else {
                let msg = "{}";
                let resp = format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    msg.len(),
                    msg
                );
                let _ = stream.write_all(resp.as_bytes());
            }
        }
    });

    // Construct an Event whose lastTimestamp is 10 seconds prior to wall-clock export time.
    let now = SystemTime::now();
    let ten_secs_ago = now - Duration::from_secs(10);
    let ts_str = format_unix_nanos_rfc3339(
        ten_secs_ago.duration_since(UNIX_EPOCH).unwrap().as_nanos() as u64,
    );
    let ev_map: Map<String, Value> = serde_json::from_value(json!({
        "metadata": { "uid": "u-lat-1", "resourceVersion": "77", "namespace": "default" },
        "involvedObject": { "kind": "Pod", "namespace": "default", "name": "lat-pod" },
        "type": "Normal",
        "reason": "Scheduled",
        "lastTimestamp": ts_str
    }))
    .unwrap();

    let metrics = MetricsRegistry::new();
    metrics.add_received_entries(1);
    let entry = enrich_event_object(ev_map, None, Some(&metrics), now);
    assert!(entry.has_timestamp);

    let sink = CloudLoggingSink::new(
        &format!("http://{addr}"),
        ClusterCoordinates {
            project_id: "proj-1".to_string(),
            cluster_name: "clus-1".to_string(),
            location: "us-central1".to_string(),
        },
        Arc::new(CloudTokenProvider::unauthenticated()),
        None,
    );

    // 1st attempt: HTTP 429 -> must NOT record latency!
    let out1 = sink.export_batch(std::slice::from_ref(&entry), &metrics);
    assert!(matches!(out1, BatchExportOutcome::RetryableFailure { .. }));
    let prom_after_429 = metrics.render_prometheus();
    assert!(prom_after_429.contains("stackdriver_sink_request_count{code=\"429\"} 1"));
    assert!(
        prom_after_429.contains("stackdriver_sink_records_latency_seconds_count 0"),
        "Failed 429 export must not record delivery latency:\n{prom_after_429}"
    );

    // 2nd attempt: HTTP 200 -> records (now - event.lastTimestamp) ~= 10.0s (bucket > 6.75s, <= 15.1875s),
    // NOT ~1ms HTTP round-trip time (which would land in le="2")!
    let out2 = sink.export_batch(std::slice::from_ref(&entry), &metrics);
    assert_eq!(out2, BatchExportOutcome::Success { sent_entries: 1 });
    let _ = srv.join();

    let prom = metrics.render_prometheus();
    assert!(prom.contains("stackdriver_sink_records_latency_seconds_count 1"));
    assert!(
        prom.contains("stackdriver_sink_records_latency_seconds_bucket{le=\"2\"} 0")
            && prom.contains("stackdriver_sink_records_latency_seconds_bucket{le=\"6.75\"} 0")
            && prom.contains("stackdriver_sink_records_latency_seconds_bucket{le=\"15.1875\"} 1"),
        "Histogram must measure event-to-sink freshness (~10s), not RPC RTT (~1ms):\n{prom}"
    );

    // Every metric whitelisted by GKE's event-exporter-manifest.yaml must be present.
    for required_metric in [
        "podlabel_cache_ops_count",
        "podlabel_get_count",
        "podlabel_nolabel_pod_cache_ops_count",
        "stackdriver_sink_received_entry_count",
        "stackdriver_sink_successfully_sent_entry_count",
        "stackdriver_sink_request_count",
        "stackdriver_sink_records_latency_seconds_bucket",
        "stackdriver_sink_records_latency_seconds_sum",
        "stackdriver_sink_records_latency_seconds_count",
    ] {
        assert!(
            prom.contains(required_metric),
            "missing GKE whitelisted metric {required_metric}"
        );
    }
}
