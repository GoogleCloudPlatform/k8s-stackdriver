//! Streaming Kubernetes API watchers: `PodReflector` (`PartialObjectMetadata` WatchList + paginated
//! List fallback) and `EventStream` (direct streaming Watch without `DeltaFIFO` retention).

use crate::auth::KubeTokenSource;
use crate::enricher::{enrich_event_object, inspect_event_meta};
use crate::metrics::MetricsRegistry;
use crate::pipeline::BoundedBatchQueue;
use crate::pod_cache::{PodKey, PodOwnerTable, parse_partial_pod_object};
use crate::shard::{ShardMode, ShardSpec};
use crate::transport::{http_request, http_stream_lines};
use log::{info, warn};
use serde::Deserialize;
use serde_json::value::RawValue;
use serde_json::{Map, Value};
use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicU8, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::time::{Duration, SystemTime};

const POD_WATCH_ACCEPT: &str = "application/json;as=PartialObjectMetadata;v=v1;g=meta.k8s.io";
const POD_LIST_ACCEPT: &str =
    "application/json;as=PartialObjectMetadataList;v=v1;g=meta.k8s.io,application/json";
const WATCH_TIMEOUT: Duration = Duration::from_secs(305);
const LIST_TIMEOUT: Duration = Duration::from_secs(30);

/// Synchronization latch signaling when `PodReflector` finishes its initial cluster snapshot.
#[derive(Default)]
pub struct InitialSyncLatch {
    done: Mutex<bool>,
    cv: Condvar,
}

impl InitialSyncLatch {
    pub fn new() -> Self {
        Self {
            done: Mutex::new(false),
            cv: Condvar::new(),
        }
    }

    pub fn mark_ready(&self) {
        let mut guard = self.done.lock().unwrap_or_else(|e| e.into_inner());
        if !*guard {
            *guard = true;
            self.cv.notify_all();
        }
    }

    pub fn wait_timeout(&self, timeout: Duration, stop_flag: &AtomicU8) -> bool {
        let deadline = std::time::Instant::now() + timeout;
        let mut guard = self.done.lock().unwrap_or_else(|e| e.into_inner());
        while !*guard {
            if stop_flag.load(Ordering::Acquire) != 0 {
                return false;
            }
            let now = std::time::Instant::now();
            if now >= deadline {
                return false;
            }
            let step = (deadline - now).min(Duration::from_millis(50));
            let (next_guard, _) = self
                .cv
                .wait_timeout(guard, step)
                .unwrap_or_else(|e| e.into_inner());
            guard = next_guard;
        }
        true
    }
}

#[derive(Deserialize)]
struct BorrowedWatchFrame<'a> {
    #[serde(rename = "type")]
    event_type: &'a str,
    #[serde(borrow)]
    object: &'a RawValue,
}

#[derive(Default, Deserialize)]
struct BookmarkEnvelope {
    #[serde(default)]
    metadata: BookmarkMetadata,
    #[serde(default)]
    code: Option<u16>,
    #[serde(default)]
    message: Option<String>,
}

#[derive(Default, Deserialize)]
struct BookmarkMetadata {
    #[serde(default, rename = "resourceVersion")]
    resource_version: String,
    #[serde(default)]
    annotations: Option<HashMap<String, String>>,
}

#[derive(Default, Deserialize)]
struct ListMetadata {
    #[serde(default, rename = "resourceVersion")]
    resource_version: String,
    #[serde(default, rename = "continue")]
    continue_token: String,
}

#[derive(Deserialize)]
struct RawItemsList<'a> {
    #[serde(default)]
    metadata: ListMetadata,
    #[serde(default, borrow)]
    items: Option<Vec<&'a RawValue>>,
}

/// Shared Kubernetes API connection parameters.
#[derive(Clone)]
pub struct KubeClientConfig {
    pub api_base: String,
    pub ca_cert_path: Option<String>,
    pub token_source: Arc<KubeTokenSource>,
    pub shard: ShardSpec,
    pub enable_watch_list: bool,
    pub list_page_size: u32,
    pub event_label_selector: Option<String>,
}

impl KubeClientConfig {
    fn auth_header(&self) -> Result<Option<String>, String> {
        Ok(self
            .token_source
            .bearer_token()?
            .map(|t| format!("Bearer {t}")))
    }
}

/// Runs a single synchronization/watch cycle (or continuous loop until `stop_flag != 0`) of the
/// `PartialObjectMetadata` Pod reflector.
pub fn run_pod_reflector_loop(
    kube: KubeClientConfig,
    pod_table: Arc<PodOwnerTable>,
    metrics: Arc<MetricsRegistry>,
    sync_latch: Arc<InitialSyncLatch>,
    stop_flag: Arc<AtomicU8>,
) {
    let mut active_shard = kube.shard;
    let mut watch_list_supported = kube.enable_watch_list;
    let mut last_rv = String::new();
    let mut backoff = Duration::from_millis(200);

    while stop_flag.load(Ordering::Acquire) == 0 {
        if last_rv.is_empty() {
            if watch_list_supported {
                match stream_pod_watch_list(
                    &kube,
                    &mut active_shard,
                    &pod_table,
                    &metrics,
                    &sync_latch,
                    &stop_flag,
                ) {
                    WatchStepResult::Completed(new_rv) => {
                        last_rv = new_rv;
                        backoff = Duration::from_millis(200);
                        continue;
                    }
                    WatchStepResult::UnsupportedFeature => {
                        if active_shard.mode == ShardMode::Server {
                            warn!(
                                "API server rejected server-side Pod sharding; falling back to client-side sharding"
                            );
                            active_shard = active_shard.fallback_to_client();
                            continue;
                        }
                        warn!(
                            "API server rejected Pod WatchList; falling back to paginated List + Watch"
                        );
                        watch_list_supported = false;
                        continue;
                    }
                    WatchStepResult::Gone => {
                        last_rv.clear();
                    }
                    WatchStepResult::TransientError(err) => {
                        warn!("Pod WatchList stream interrupted: {err}");
                        sleep_with_stop(&stop_flag, backoff);
                        backoff = (backoff * 2).min(Duration::from_secs(10));
                        continue;
                    }
                }
            }

            match paginated_pod_relist(&kube, &mut active_shard, &pod_table, &metrics, &stop_flag) {
                Ok(rv) => {
                    last_rv = rv;
                    sync_latch.mark_ready();
                    backoff = Duration::from_millis(200);
                }
                Err(err) => {
                    warn!("Pod paginated List failed: {err}");
                    sleep_with_stop(&stop_flag, backoff);
                    backoff = (backoff * 2).min(Duration::from_secs(10));
                    continue;
                }
            }
        }

        if stop_flag.load(Ordering::Acquire) != 0 {
            break;
        }

        match stream_pod_watch(
            &kube,
            &active_shard,
            &last_rv,
            &pod_table,
            &metrics,
            &stop_flag,
        ) {
            WatchStepResult::Completed(new_rv) => {
                if !new_rv.is_empty() {
                    last_rv = new_rv;
                }
                backoff = Duration::from_millis(200);
            }
            WatchStepResult::Gone => {
                info!("Pod Watch received 410 Gone at rv={last_rv}; re-syncing PodOwnerTable");
                last_rv.clear();
            }
            WatchStepResult::UnsupportedFeature => {
                if active_shard.mode == ShardMode::Server {
                    active_shard = active_shard.fallback_to_client();
                }
                last_rv.clear();
            }
            WatchStepResult::TransientError(err) => {
                warn!("Pod Watch stream error: {err}");
                sleep_with_stop(&stop_flag, backoff);
                backoff = (backoff * 2).min(Duration::from_secs(10));
            }
        }
    }
}

enum WatchStepResult {
    Completed(String),
    Gone,
    UnsupportedFeature,
    TransientError(String),
}

fn stream_pod_watch_list(
    kube: &KubeClientConfig,
    active_shard: &mut ShardSpec,
    pod_table: &PodOwnerTable,
    metrics: &MetricsRegistry,
    sync_latch: &InitialSyncLatch,
    stop_flag: &AtomicU8,
) -> WatchStepResult {
    let auth = match kube.auth_header() {
        Ok(a) => a,
        Err(e) => return WatchStepResult::TransientError(e),
    };
    let mut query =
        "watch=true&sendInitialEvents=true&resourceVersionMatch=NotOlderThan&allowWatchBookmarks=true&timeoutSeconds=300"
            .to_string();
    if let Some(shard_q) = active_shard.server_query_params() {
        query.push('&');
        query.push_str(&shard_q);
    }
    let url = format!("{}/api/v1/pods?{query}", kube.api_base);

    let mut headers: Vec<(&str, &str)> = vec![("Accept", POD_WATCH_ACCEPT)];
    if let Some(bearer) = auth.as_deref() {
        headers.push(("Authorization", bearer));
    }

    let mut latest_rv = String::new();
    let mut initial_keys: Option<HashSet<PodKey>> = Some(HashSet::new());
    let mut saw_gone = false;

    let res = http_stream_lines(
        &url,
        &headers,
        kube.ca_cert_path.as_deref(),
        WATCH_TIMEOUT,
        stop_flag,
        |line| match apply_pod_watch_line(
            line,
            active_shard,
            pod_table,
            metrics,
            &mut initial_keys,
            sync_latch,
            &mut latest_rv,
        ) {
            PodFrameOutcome::Continue => true,
            PodFrameOutcome::Gone => {
                saw_gone = true;
                false
            }
        },
    );

    match res {
        Ok(resp) if resp.status == 200 => {
            if saw_gone {
                WatchStepResult::Gone
            } else if initial_keys.is_some() {
                WatchStepResult::TransientError(
                    "Pod WatchList closed before initial-events-end bookmark".to_string(),
                )
            } else {
                WatchStepResult::Completed(latest_rv)
            }
        }
        Ok(resp) if resp.status == 410 => WatchStepResult::Gone,
        Ok(resp) if matches!(resp.status, 400 | 406 | 422) => WatchStepResult::UnsupportedFeature,
        Ok(resp) => WatchStepResult::TransientError(format!(
            "HTTP {}: {}",
            resp.status,
            String::from_utf8_lossy(&resp.body)
        )),
        Err(err) => WatchStepResult::TransientError(err),
    }
}

fn stream_pod_watch(
    kube: &KubeClientConfig,
    active_shard: &ShardSpec,
    start_rv: &str,
    pod_table: &PodOwnerTable,
    metrics: &MetricsRegistry,
    stop_flag: &AtomicU8,
) -> WatchStepResult {
    let auth = match kube.auth_header() {
        Ok(a) => a,
        Err(e) => return WatchStepResult::TransientError(e),
    };
    let mut query = format!(
        "watch=true&allowWatchBookmarks=true&timeoutSeconds=300&resourceVersion={}",
        percent_encode_query_val(start_rv)
    );
    if let Some(shard_q) = active_shard.server_query_params() {
        query.push('&');
        query.push_str(&shard_q);
    }
    let url = format!("{}/api/v1/pods?{query}", kube.api_base);

    let mut headers: Vec<(&str, &str)> = vec![("Accept", POD_WATCH_ACCEPT)];
    if let Some(bearer) = auth.as_deref() {
        headers.push(("Authorization", bearer));
    }

    let mut latest_rv = start_rv.to_string();
    let mut no_initial_set: Option<HashSet<PodKey>> = None;
    let dummy_latch = InitialSyncLatch::new();
    let mut saw_gone = false;

    let res = http_stream_lines(
        &url,
        &headers,
        kube.ca_cert_path.as_deref(),
        WATCH_TIMEOUT,
        stop_flag,
        |line| match apply_pod_watch_line(
            line,
            active_shard,
            pod_table,
            metrics,
            &mut no_initial_set,
            &dummy_latch,
            &mut latest_rv,
        ) {
            PodFrameOutcome::Continue => true,
            PodFrameOutcome::Gone => {
                saw_gone = true;
                false
            }
        },
    );

    match res {
        Ok(resp) if resp.status == 200 => {
            if saw_gone {
                WatchStepResult::Gone
            } else {
                WatchStepResult::Completed(latest_rv)
            }
        }
        Ok(resp) if resp.status == 410 => WatchStepResult::Gone,
        Ok(resp) if matches!(resp.status, 400 | 422) => WatchStepResult::UnsupportedFeature,
        Ok(resp) => WatchStepResult::TransientError(format!(
            "HTTP {}: {}",
            resp.status,
            String::from_utf8_lossy(&resp.body)
        )),
        Err(err) => WatchStepResult::TransientError(err),
    }
}

enum PodFrameOutcome {
    Continue,
    Gone,
}

fn apply_pod_watch_line(
    line: &[u8],
    shard: &ShardSpec,
    pod_table: &PodOwnerTable,
    metrics: &MetricsRegistry,
    initial_keys: &mut Option<HashSet<PodKey>>,
    sync_latch: &InitialSyncLatch,
    latest_rv: &mut String,
) -> PodFrameOutcome {
    let Ok(frame) = serde_json::from_slice::<BorrowedWatchFrame<'_>>(line) else {
        return PodFrameOutcome::Continue;
    };
    let raw_obj = frame.object.get().as_bytes();
    match frame.event_type {
        "ADDED" | "MODIFIED" => {
            if let Ok(pod_meta) = parse_partial_pod_object(raw_obj) {
                if !pod_meta.resource_version.is_empty() {
                    *latest_rv = pod_meta.resource_version.clone();
                }
                if shard.owns_pod(&pod_meta.namespace, &pod_meta.name) {
                    if let Some(keys) = initial_keys.as_mut() {
                        keys.insert(pod_table.compute_key(&pod_meta.namespace, &pod_meta.name));
                    }
                    pod_table.upsert_pod_metadata(&pod_meta, Some(metrics));
                }
            }
            PodFrameOutcome::Continue
        }
        "DELETED" => {
            if let Ok(pod_meta) = parse_partial_pod_object(raw_obj) {
                if !pod_meta.resource_version.is_empty() {
                    *latest_rv = pod_meta.resource_version.clone();
                }
                if shard.owns_pod(&pod_meta.namespace, &pod_meta.name) {
                    if let Some(keys) = initial_keys.as_mut() {
                        keys.remove(&pod_table.compute_key(&pod_meta.namespace, &pod_meta.name));
                    }
                    pod_table.remove_pod(&pod_meta.namespace, &pod_meta.name, Some(metrics));
                }
            }
            PodFrameOutcome::Continue
        }
        "BOOKMARK" => {
            if let Ok(bm) = serde_json::from_slice::<BookmarkEnvelope>(raw_obj) {
                if !bm.metadata.resource_version.is_empty() {
                    *latest_rv = bm.metadata.resource_version;
                }
                let is_init_end = bm
                    .metadata
                    .annotations
                    .as_ref()
                    .and_then(|m| m.get("k8s.io/initial-events-end"))
                    .is_some_and(|v| v == "true");
                if is_init_end {
                    if let Some(live_keys) = initial_keys.take() {
                        pod_table.retain_only(&live_keys, Some(metrics));
                    }
                    sync_latch.mark_ready();
                }
            }
            PodFrameOutcome::Continue
        }
        "ERROR" => {
            if let Ok(err_env) = serde_json::from_slice::<BookmarkEnvelope>(raw_obj) {
                if err_env.code == Some(410) {
                    return PodFrameOutcome::Gone;
                }
                warn!(
                    "Pod Watch ERROR frame code={:?} msg={:?}",
                    err_env.code, err_env.message
                );
            }
            PodFrameOutcome::Gone
        }
        _ => PodFrameOutcome::Continue,
    }
}

/// Performs a bounded-memory paginated `PartialObjectMetadataList` Pod sync
/// (`limit=<page_size>&continue=...`).
pub fn paginated_pod_relist(
    kube: &KubeClientConfig,
    active_shard: &mut ShardSpec,
    pod_table: &PodOwnerTable,
    metrics: &MetricsRegistry,
    stop_flag: &AtomicU8,
) -> Result<String, String> {
    let mut continue_token = String::new();
    let mut list_rv = String::new();
    let mut live_keys = HashSet::new();

    loop {
        if stop_flag.load(Ordering::Acquire) != 0 {
            return Err("stopped during Pod paginated List".to_string());
        }
        let auth = kube.auth_header()?;
        let mut query = format!("limit={}", kube.list_page_size);
        if continue_token.is_empty() {
            query.push_str("&resourceVersion=0");
        } else {
            query.push_str("&continue=");
            query.push_str(&percent_encode_query_val(&continue_token));
        }
        if let Some(shard_q) = active_shard.server_query_params() {
            query.push('&');
            query.push_str(&shard_q);
        }

        let url = format!("{}/api/v1/pods?{query}", kube.api_base);
        let mut headers: Vec<(&str, &str)> = vec![("Accept", POD_LIST_ACCEPT)];
        if let Some(bearer) = auth.as_deref() {
            headers.push(("Authorization", bearer));
        }

        let resp = http_request(
            "GET",
            &url,
            &headers,
            &[],
            false,
            kube.ca_cert_path.as_deref(),
            LIST_TIMEOUT,
        )?;

        if matches!(resp.status, 400 | 422) && active_shard.mode == ShardMode::Server {
            warn!(
                "API server rejected server-side sharding on Pod List; falling back to client sharding"
            );
            *active_shard = active_shard.fallback_to_client();
            continue_token.clear();
            live_keys.clear();
            continue;
        }
        if resp.status != 200 {
            return Err(format!(
                "Pod List returned HTTP {}: {}",
                resp.status,
                String::from_utf8_lossy(&resp.body)
            ));
        }

        let page: RawItemsList<'_> = serde_json::from_slice(&resp.body)
            .map_err(|e| format!("invalid Pod PartialObjectMetadataList JSON: {e}"))?;
        if !page.metadata.resource_version.is_empty() {
            list_rv = page.metadata.resource_version;
        }

        for raw_item in page.items.unwrap_or_default() {
            if let Ok(pod_meta) = parse_partial_pod_object(raw_item.get().as_bytes())
                && active_shard.owns_pod(&pod_meta.namespace, &pod_meta.name)
            {
                live_keys.insert(pod_table.compute_key(&pod_meta.namespace, &pod_meta.name));
                pod_table.upsert_pod_metadata(&pod_meta, Some(metrics));
            }
        }

        if page.metadata.continue_token.is_empty() {
            break;
        }
        continue_token = page.metadata.continue_token;
    }

    pod_table.retain_only(&live_keys, Some(metrics));
    Ok(list_rv)
}

/// Streams Kubernetes Events directly into [`BoundedBatchQueue`] without `DeltaFIFO` retention.
pub fn run_event_stream_loop(
    kube: KubeClientConfig,
    pod_table: Option<Arc<PodOwnerTable>>,
    queue: Arc<BoundedBatchQueue>,
    metrics: Arc<MetricsRegistry>,
    stop_flag: Arc<AtomicU8>,
) {
    let mut active_shard = kube.shard;
    let mut current_rv = String::new();
    let mut highest_seen_rv_num: u64 = 0;
    let mut backoff = Duration::from_millis(200);

    while stop_flag.load(Ordering::Acquire) == 0 {
        if current_rv.is_empty() {
            match fetch_initial_event_rv(&kube, &mut active_shard) {
                Ok(rv) => {
                    if let Ok(n) = rv.parse::<u64>() {
                        highest_seen_rv_num = highest_seen_rv_num.max(n);
                    }
                    current_rv = rv;
                    backoff = Duration::from_millis(200);
                }
                Err(err) => {
                    warn!("Failed to initialize Event resourceVersion watermark: {err}");
                    sleep_with_stop(&stop_flag, backoff);
                    backoff = (backoff * 2).min(Duration::from_secs(10));
                    continue;
                }
            }
        }

        if stop_flag.load(Ordering::Acquire) != 0 {
            break;
        }

        match stream_events_watch(
            &kube,
            &active_shard,
            &current_rv,
            &mut highest_seen_rv_num,
            pod_table.as_deref(),
            &queue,
            &metrics,
            &stop_flag,
        ) {
            WatchStepResult::Completed(new_rv) => {
                if !new_rv.is_empty() {
                    current_rv = new_rv;
                }
                backoff = Duration::from_millis(200);
            }
            WatchStepResult::Gone => {
                info!(
                    "Event Watch received 410 Gone at rv={current_rv}; recovering missed events via paginated List"
                );
                match recover_events_after_gone(
                    &kube,
                    &mut active_shard,
                    &mut highest_seen_rv_num,
                    pod_table.as_deref(),
                    &queue,
                    &metrics,
                    &stop_flag,
                ) {
                    Ok(new_rv) => {
                        current_rv = new_rv;
                        backoff = Duration::from_millis(200);
                    }
                    Err(err) => {
                        warn!("Event 410 Gone recovery failed: {err}");
                        current_rv.clear();
                        sleep_with_stop(&stop_flag, backoff);
                        backoff = (backoff * 2).min(Duration::from_secs(10));
                    }
                }
            }
            WatchStepResult::UnsupportedFeature => {
                if active_shard.mode == ShardMode::Server {
                    warn!(
                        "API server rejected server-side Event sharding; falling back to client-side sharding"
                    );
                    active_shard = active_shard.fallback_to_client();
                }
            }
            WatchStepResult::TransientError(err) => {
                warn!("Event Watch stream interrupted: {err}");
                sleep_with_stop(&stop_flag, backoff);
                backoff = (backoff * 2).min(Duration::from_secs(10));
            }
        }
    }
}

fn fetch_initial_event_rv(
    kube: &KubeClientConfig,
    active_shard: &mut ShardSpec,
) -> Result<String, String> {
    let auth = kube.auth_header()?;
    let mut query = "limit=1&resourceVersion=0".to_string();
    if let Some(sel) = kube.event_label_selector.as_deref() {
        query.push_str("&labelSelector=");
        query.push_str(&percent_encode_query_val(sel));
    }
    if let Some(shard_q) = active_shard.server_query_params() {
        query.push('&');
        query.push_str(&shard_q);
    }
    let url = format!("{}/api/v1/events?{query}", kube.api_base);
    let mut headers: Vec<(&str, &str)> = vec![("Accept", POD_LIST_ACCEPT)];
    if let Some(bearer) = auth.as_deref() {
        headers.push(("Authorization", bearer));
    }

    let resp = http_request(
        "GET",
        &url,
        &headers,
        &[],
        false,
        kube.ca_cert_path.as_deref(),
        LIST_TIMEOUT,
    )?;
    if matches!(resp.status, 400 | 422) && active_shard.mode == ShardMode::Server {
        *active_shard = active_shard.fallback_to_client();
        return fetch_initial_event_rv(kube, active_shard);
    }
    if resp.status != 200 {
        return Err(format!(
            "Event initial List returned HTTP {}: {}",
            resp.status,
            String::from_utf8_lossy(&resp.body)
        ));
    }
    let parsed: RawItemsList<'_> =
        serde_json::from_slice(&resp.body).map_err(|e| format!("invalid Event List JSON: {e}"))?;
    Ok(if parsed.metadata.resource_version.is_empty() {
        "0".to_string()
    } else {
        parsed.metadata.resource_version
    })
}

#[allow(clippy::too_many_arguments)]
fn stream_events_watch(
    kube: &KubeClientConfig,
    shard: &ShardSpec,
    start_rv: &str,
    highest_seen_rv_num: &mut u64,
    pod_table: Option<&PodOwnerTable>,
    queue: &BoundedBatchQueue,
    metrics: &MetricsRegistry,
    stop_flag: &AtomicU8,
) -> WatchStepResult {
    let auth = match kube.auth_header() {
        Ok(a) => a,
        Err(e) => return WatchStepResult::TransientError(e),
    };
    let mut query = format!(
        "watch=true&allowWatchBookmarks=true&timeoutSeconds=300&resourceVersion={}",
        percent_encode_query_val(start_rv)
    );
    if let Some(sel) = kube.event_label_selector.as_deref() {
        query.push_str("&labelSelector=");
        query.push_str(&percent_encode_query_val(sel));
    }
    if let Some(shard_q) = shard.server_query_params() {
        query.push('&');
        query.push_str(&shard_q);
    }
    let url = format!("{}/api/v1/events?{query}", kube.api_base);

    let mut headers: Vec<(&str, &str)> = vec![("Accept", "application/json")];
    if let Some(bearer) = auth.as_deref() {
        headers.push(("Authorization", bearer));
    }

    let mut latest_rv = start_rv.to_string();
    let mut saw_gone = false;

    let res = http_stream_lines(
        &url,
        &headers,
        kube.ca_cert_path.as_deref(),
        WATCH_TIMEOUT,
        stop_flag,
        |line| {
            let Ok(frame) = serde_json::from_slice::<BorrowedWatchFrame<'_>>(line) else {
                return true;
            };
            let raw_bytes = frame.object.get().as_bytes();
            match frame.event_type {
                "ADDED" | "MODIFIED" => {
                    if let Ok(event_map) = serde_json::from_slice::<Map<String, Value>>(raw_bytes) {
                        let meta = inspect_event_meta(&event_map);
                        if !meta.resource_version.is_empty() {
                            latest_rv = meta.resource_version.clone();
                            if let Ok(n) = meta.resource_version.parse::<u64>() {
                                *highest_seen_rv_num = (*highest_seen_rv_num).max(n);
                            }
                        }
                        if shard.owns_event(
                            &meta.event_namespace,
                            &meta.involved_kind,
                            &meta.involved_namespace,
                            &meta.involved_name,
                            &meta.uid,
                        ) {
                            let enriched = enrich_event_object(
                                event_map,
                                pod_table,
                                Some(metrics),
                                SystemTime::now(),
                            );
                            if !queue.push(enriched, stop_flag) {
                                return false;
                            }
                        }
                    }
                    true
                }
                "BOOKMARK" => {
                    if let Ok(bm) = serde_json::from_slice::<BookmarkEnvelope>(raw_bytes)
                        && !bm.metadata.resource_version.is_empty()
                    {
                        if let Ok(n) = bm.metadata.resource_version.parse::<u64>() {
                            *highest_seen_rv_num = (*highest_seen_rv_num).max(n);
                        }
                        latest_rv = bm.metadata.resource_version;
                    }
                    true
                }
                "ERROR" => {
                    if let Ok(err_env) = serde_json::from_slice::<BookmarkEnvelope>(raw_bytes) {
                        if err_env.code == Some(410) {
                            saw_gone = true;
                        }
                    } else {
                        saw_gone = true;
                    }
                    false
                }
                _ => true,
            }
        },
    );

    match res {
        Ok(resp) if resp.status == 200 => {
            if saw_gone {
                WatchStepResult::Gone
            } else {
                WatchStepResult::Completed(latest_rv)
            }
        }
        Ok(resp) if resp.status == 410 => WatchStepResult::Gone,
        Ok(resp) if matches!(resp.status, 400 | 422) => WatchStepResult::UnsupportedFeature,
        Ok(resp) => WatchStepResult::TransientError(format!(
            "HTTP {}: {}",
            resp.status,
            String::from_utf8_lossy(&resp.body)
        )),
        Err(err) => WatchStepResult::TransientError(err),
    }
}

#[allow(clippy::too_many_arguments)]
fn recover_events_after_gone(
    kube: &KubeClientConfig,
    active_shard: &mut ShardSpec,
    highest_seen_rv_num: &mut u64,
    pod_table: Option<&PodOwnerTable>,
    queue: &BoundedBatchQueue,
    metrics: &MetricsRegistry,
    stop_flag: &AtomicU8,
) -> Result<String, String> {
    let baseline_rv = *highest_seen_rv_num;
    let mut max_seen_in_list = baseline_rv;
    let mut continue_token = String::new();
    let mut list_rv = String::new();

    loop {
        if stop_flag.load(Ordering::Acquire) != 0 {
            return Err("stopped during Event 410 recovery".to_string());
        }
        let auth = kube.auth_header()?;
        let mut query = format!("limit={}", kube.list_page_size);
        if !continue_token.is_empty() {
            query.push_str("&continue=");
            query.push_str(&percent_encode_query_val(&continue_token));
        }
        if let Some(sel) = kube.event_label_selector.as_deref() {
            query.push_str("&labelSelector=");
            query.push_str(&percent_encode_query_val(sel));
        }
        if let Some(shard_q) = active_shard.server_query_params() {
            query.push('&');
            query.push_str(&shard_q);
        }

        let url = format!("{}/api/v1/events?{query}", kube.api_base);
        let mut headers: Vec<(&str, &str)> = vec![("Accept", "application/json")];
        if let Some(bearer) = auth.as_deref() {
            headers.push(("Authorization", bearer));
        }

        let resp = http_request(
            "GET",
            &url,
            &headers,
            &[],
            false,
            kube.ca_cert_path.as_deref(),
            LIST_TIMEOUT,
        )?;
        if matches!(resp.status, 400 | 422) && active_shard.mode == ShardMode::Server {
            *active_shard = active_shard.fallback_to_client();
            continue_token.clear();
            continue;
        }
        if resp.status != 200 {
            return Err(format!("Event recovery List returned HTTP {}", resp.status));
        }

        let page: RawItemsList<'_> = serde_json::from_slice(&resp.body)
            .map_err(|e| format!("invalid Event recovery List JSON: {e}"))?;
        if !page.metadata.resource_version.is_empty() {
            if let Ok(n) = page.metadata.resource_version.parse::<u64>() {
                max_seen_in_list = max_seen_in_list.max(n);
            }
            list_rv = page.metadata.resource_version;
        }

        for raw_item in page.items.unwrap_or_default() {
            let Ok(event_map) =
                serde_json::from_slice::<Map<String, Value>>(raw_item.get().as_bytes())
            else {
                continue;
            };
            let meta = inspect_event_meta(&event_map);
            let item_rv_num = meta.resource_version.parse::<u64>().unwrap_or(0);
            if baseline_rv > 0 && item_rv_num > 0 && item_rv_num <= baseline_rv {
                continue;
            }
            max_seen_in_list = max_seen_in_list.max(item_rv_num);
            if active_shard.owns_event(
                &meta.event_namespace,
                &meta.involved_kind,
                &meta.involved_namespace,
                &meta.involved_name,
                &meta.uid,
            ) {
                let enriched =
                    enrich_event_object(event_map, pod_table, Some(metrics), SystemTime::now());
                if !queue.push(enriched, stop_flag) {
                    return Ok(list_rv);
                }
            }
        }

        if page.metadata.continue_token.is_empty() {
            break;
        }
        continue_token = page.metadata.continue_token;
    }

    *highest_seen_rv_num = max_seen_in_list;
    Ok(list_rv)
}

fn percent_encode_query_val(raw: &str) -> String {
    let mut out = String::with_capacity(raw.len());
    for &b in raw.as_bytes() {
        if b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.' | b'~') {
            out.push(char::from(b));
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

fn sleep_with_stop(stop_flag: &AtomicU8, duration: Duration) {
    let deadline = std::time::Instant::now() + duration;
    while stop_flag.load(Ordering::Acquire) == 0 {
        let now = std::time::Instant::now();
        if now >= deadline {
            break;
        }
        std::thread::sleep((deadline - now).min(Duration::from_millis(50)));
    }
}
