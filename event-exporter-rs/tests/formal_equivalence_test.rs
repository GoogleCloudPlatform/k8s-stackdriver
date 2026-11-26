//! Formal refinement, algebraic sharding bijection, and RFC3339 / Go `-sink-opts` parity tests.

#![forbid(unsafe_code)]

use event_exporter_rs::cli::ExporterConfig;
use event_exporter_rs::enricher::{format_unix_nanos_rfc3339, parse_rfc3339_unix_nanos};
use event_exporter_rs::metrics::MetricsRegistry;
use event_exporter_rs::pod_cache::{
    CompactOwner, ControllerKind, OwnerRefExtract, PodKey, PodMetadataExtract, PodOwnerTable,
    RelevantPodLabels, ResolvedPodOwner,
};
use event_exporter_rs::shard::{ShardMode, ShardSpec};
use std::collections::{BTreeMap, HashSet};

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
}
