//! Kubernetes Event to GKE Cloud Logging / OTLP enrichment, RBAC validation, and RFC3339
//! timestamps.

use crate::pod_cache::{ControllerKind, PodOwnerTable};
use crate::shard::ShardSpec;
use serde_json::{Map, Value};
use std::time::{SystemTime, UNIX_EPOCH};

/// Cluster identity coordinates attached to every MonitoredResource.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ClusterCoordinates {
    pub project_id: String,
    pub cluster_name: String,
    pub location: String,
}

/// Target GKE MonitoredResource type and distinguishing labels.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum MonitoredResourceKind {
    K8sPod {
        namespace_name: String,
        pod_name: String,
    },
    K8sNode {
        node_name: String,
    },
    K8sCluster,
}

impl MonitoredResourceKind {
    pub fn resource_type(&self) -> &'static str {
        match self {
            Self::K8sPod { .. } => "k8s_pod",
            Self::K8sNode { .. } => "k8s_node",
            Self::K8sCluster => "k8s_cluster",
        }
    }

    pub fn resource_labels(&self, coords: &ClusterCoordinates) -> Vec<(&'static str, String)> {
        let mut labels = vec![
            ("project_id", coords.project_id.clone()),
            ("location", coords.location.clone()),
            ("cluster_name", coords.cluster_name.clone()),
        ];
        match self {
            Self::K8sPod {
                namespace_name,
                pod_name,
            } => {
                labels.push(("namespace_name", namespace_name.clone()));
                labels.push(("pod_name", pod_name.clone()));
            }
            Self::K8sNode { node_name } => {
                labels.push(("node_name", node_name.clone()));
            }
            Self::K8sCluster => {}
        }
        labels
    }
}

/// Normalized log severity matching Cloud Logging (`INFO`, `WARNING`) and OTLP (`9`, `13`).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum LogSeverity {
    Info,
    Warning,
}

impl LogSeverity {
    pub fn cloud_logging_str(self) -> &'static str {
        match self {
            Self::Info => "INFO",
            Self::Warning => "WARNING",
        }
    }

    pub fn otlp_severity_number(self) -> i32 {
        match self {
            Self::Info => 9,
            Self::Warning => 13,
        }
    }
}

/// Payload representation shared between Cloud Logging (`jsonPayload` / `textPayload`) and OTLP
/// (`kvlist_value` / `string_value`).
#[derive(Clone, Debug, PartialEq)]
pub enum EntryPayload {
    Json(Map<String, Value>),
    Text(String),
}

/// Enriched log record ready for batching and export to Cloud Logging v2 or OTLP Logs.
#[derive(Clone, Debug, PartialEq)]
pub struct EnrichedEntry {
    pub insert_id: String,
    pub timestamp_rfc3339: String,
    pub timestamp_unix_nanos: u64,
    pub has_timestamp: bool,
    pub observed_unix_nanos: u64,
    pub severity: LogSeverity,
    pub resource: MonitoredResourceKind,
    pub labels: Vec<(String, String)>,
    pub payload: EntryPayload,
    pub estimated_bytes: usize,
}

/// Parsed routing fields extracted before mutating an Event JSON map.
#[derive(Clone, Debug, Default)]
pub struct EventRoutingMeta {
    pub uid: String,
    pub resource_version: String,
    pub event_namespace: String,
    pub involved_kind: String,
    pub involved_namespace: String,
    pub involved_name: String,
}

/// Extracts routing and deduplication fields from a raw Event JSON map.
pub fn inspect_event_meta(obj: &Map<String, Value>) -> EventRoutingMeta {
    let metadata = obj.get("metadata").and_then(Value::as_object);
    let involved = obj.get("involvedObject").and_then(Value::as_object);

    EventRoutingMeta {
        uid: get_str(metadata, "uid").to_string(),
        resource_version: get_str(metadata, "resourceVersion").to_string(),
        event_namespace: get_str(metadata, "namespace").to_string(),
        involved_kind: get_str(involved, "kind").to_string(),
        involved_namespace: get_str(involved, "namespace").to_string(),
        involved_name: get_str(involved, "name").to_string(),
    }
}

/// Enriches a Kubernetes Event JSON map into an [`EnrichedEntry`], enforcing RBAC cross-namespace
/// Pod spoofing protection, `fieldBlacklist` stripping (`count`, `firstTimestamp`), and Pod owner
/// labels.
pub fn enrich_event_object(
    mut event_obj: Map<String, Value>,
    pod_table: Option<&PodOwnerTable>,
    metrics: Option<&crate::metrics::MetricsRegistry>,
    now: SystemTime,
) -> EnrichedEntry {
    let meta = inspect_event_meta(&event_obj);

    let insert_id = if meta.uid.is_empty() {
        meta.resource_version.clone()
    } else if meta.resource_version.is_empty() {
        meta.uid.clone()
    } else {
        format!("{}-{}", meta.uid, meta.resource_version)
    };

    let severity = match event_obj.get("type").and_then(Value::as_str) {
        Some("Warning") => LogSeverity::Warning,
        _ => LogSeverity::Info,
    };

    // Enforce RBAC namespace boundary: an Event in namespace A claiming involvedObject.kind ==
    // "Pod" in namespace B (or cluster-scoped) must NOT resolve as k8s_pod or receive Pod B's
    // owner labels.
    let resource = if meta.involved_kind == "Pod"
        && !meta.event_namespace.is_empty()
        && meta.event_namespace == meta.involved_namespace
        && !meta.involved_name.is_empty()
    {
        MonitoredResourceKind::K8sPod {
            namespace_name: meta.involved_namespace.clone(),
            pod_name: meta.involved_name.clone(),
        }
    } else if meta.involved_kind == "Node" && !meta.involved_name.is_empty() {
        MonitoredResourceKind::K8sNode {
            node_name: meta.involved_name.clone(),
        }
    } else {
        MonitoredResourceKind::K8sCluster
    };

    let mut labels = Vec::new();
    if let (
        MonitoredResourceKind::K8sPod {
            namespace_name,
            pod_name,
        },
        Some(table),
    ) = (&resource, pod_table)
    {
        if let Some(m) = metrics {
            m.inc_podlabel_get();
        }
        if let Some(owner) = table.lookup(namespace_name, pod_name, metrics) {
            labels.push((
                "logging.gke.io/top_level_controller_type".to_string(),
                owner.kind.as_str().to_string(),
            ));
            labels.push((
                "logging.gke.io/top_level_controller_name".to_string(),
                owner.name,
            ));
            if owner.kind == ControllerKind::JobSet {
                if let Some(attempt) = owner.jobset_restart_attempt {
                    labels.push(("jobset.sigs.k8s.io/restart-attempt".to_string(), attempt));
                }
                if let Some(uid) = owner.jobset_uid {
                    labels.push(("jobset.sigs.k8s.io/jobset-uid".to_string(), uid));
                }
            }
        }
    }

    let observed_nanos = system_time_to_nanos(now);
    let (timestamp_rfc3339, timestamp_unix_nanos, has_timestamp) =
        select_event_timestamp(&event_obj, now);

    // Strip Go event-exporter fieldBlacklist keys and ensure TypeMeta is present even when an Event
    // came from a Kubernetes EventList (whose items omit kind and apiVersion).
    event_obj.remove("count");
    event_obj.remove("firstTimestamp");
    event_obj
        .entry("kind".to_string())
        .or_insert_with(|| Value::String("Event".to_string()));
    event_obj
        .entry("apiVersion".to_string())
        .or_insert_with(|| Value::String("v1".to_string()));

    let estimated_bytes = estimate_map_bytes(&event_obj) + 256;
    EnrichedEntry {
        insert_id,
        timestamp_rfc3339,
        timestamp_unix_nanos,
        has_timestamp,
        observed_unix_nanos: observed_nanos,
        severity,
        resource,
        labels,
        payload: EntryPayload::Json(event_obj),
        estimated_bytes,
    }
}

/// Builds the startup event-loss warning entry. Returns `None` on non-leader shards (`shard_index >
/// 0`).
pub fn build_startup_marker_entry(shard: &ShardSpec, now: SystemTime) -> Option<EnrichedEntry> {
    if !shard.is_leader() {
        return None;
    }
    let nanos = system_time_to_nanos(now);
    let ts = format_unix_nanos_rfc3339(nanos);
    let message =
        "Event exporter started watching. Some events may have been lost up to this point."
            .to_string();
    Some(EnrichedEntry {
        insert_id: format!("startup-{nanos}"),
        timestamp_rfc3339: ts,
        timestamp_unix_nanos: nanos,
        has_timestamp: true,
        observed_unix_nanos: nanos,
        severity: LogSeverity::Warning,
        resource: MonitoredResourceKind::K8sCluster,
        labels: Vec::new(),
        payload: EntryPayload::Text(message),
        estimated_bytes: 256,
    })
}

fn get_str<'a>(map: Option<&'a Map<String, Value>>, key: &str) -> &'a str {
    map.and_then(|m| m.get(key))
        .and_then(Value::as_str)
        .unwrap_or("")
}

fn is_usable_timestamp(raw: &str) -> bool {
    let trimmed = raw.trim();
    !trimmed.is_empty() && trimmed != "null" && !trimmed.starts_with("0001-01-01")
}

fn select_event_timestamp(event_obj: &Map<String, Value>, now: SystemTime) -> (String, u64, bool) {
    let candidates = [
        event_obj.get("lastTimestamp").and_then(Value::as_str),
        event_obj
            .get("series")
            .and_then(Value::as_object)
            .and_then(|s| s.get("lastObservedTime"))
            .and_then(Value::as_str),
        event_obj.get("eventTime").and_then(Value::as_str),
        event_obj
            .get("metadata")
            .and_then(Value::as_object)
            .and_then(|m| m.get("creationTimestamp"))
            .and_then(Value::as_str),
    ];

    for candidate in candidates.into_iter().flatten() {
        if is_usable_timestamp(candidate)
            && let Some(nanos) = parse_rfc3339_unix_nanos(candidate)
        {
            return (candidate.to_string(), nanos, true);
        }
    }

    let fallback_nanos = system_time_to_nanos(now);
    (
        format_unix_nanos_rfc3339(fallback_nanos),
        fallback_nanos,
        false,
    )
}

fn system_time_to_nanos(t: SystemTime) -> u64 {
    t.duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos() as u64)
        .unwrap_or(0)
}

fn estimate_map_bytes(map: &Map<String, Value>) -> usize {
    map.iter()
        .map(|(k, v)| k.len() + estimate_value_bytes(v) + 6)
        .sum::<usize>()
        + 2
}

fn estimate_value_bytes(val: &Value) -> usize {
    match val {
        Value::Null => 4,
        Value::Bool(_) => 5,
        Value::Number(_) => 12,
        Value::String(s) => s.len() + 2,
        Value::Array(arr) => arr.iter().map(estimate_value_bytes).sum::<usize>() + arr.len() + 2,
        Value::Object(obj) => estimate_map_bytes(obj),
    }
}

/// Parses an RFC3339 timestamp (`YYYY-MM-DDTHH:MM:SS[.fraction](Z|+HH:MM|-HH:MM)`) into Unix epoch
/// nanoseconds.
pub fn parse_rfc3339_unix_nanos(raw: &str) -> Option<u64> {
    let s = raw.trim().as_bytes();
    if s.len() < 20 {
        return None;
    }
    if s[4] != b'-' || s[7] != b'-' || (s[10] != b'T' && s[10] != b't' && s[10] != b' ') {
        return None;
    }
    if s[13] != b':' || s[16] != b':' {
        return None;
    }

    let year = parse_digits_i64(&s[0..4])?;
    let month = parse_digits_u32(&s[5..7])?;
    let day = parse_digits_u32(&s[8..10])?;
    let hour = parse_digits_u32(&s[11..13])?;
    let minute = parse_digits_u32(&s[14..16])?;
    let second = parse_digits_u32(&s[17..19])?;

    if !(1..=12).contains(&month)
        || day < 1
        || day > days_in_month(year, month)
        || hour > 23
        || minute > 59
        || second > 60
    {
        return None;
    }

    let mut pos = 19usize;
    let mut nanos: u64 = 0;
    if pos < s.len() && s[pos] == b'.' {
        pos += 1;
        let frac_start = pos;
        let mut scale: u64 = 100_000_000;
        while pos < s.len() && s[pos].is_ascii_digit() {
            if scale > 0 {
                nanos += u64::from(s[pos] - b'0') * scale;
                scale /= 10;
            }
            pos += 1;
        }
        if pos == frac_start {
            return None;
        }
    }

    let tz_offset_secs: i64 = if pos + 1 == s.len() && (s[pos] == b'Z' || s[pos] == b'z') {
        0
    } else if pos + 6 == s.len() && (s[pos] == b'+' || s[pos] == b'-') && s[pos + 3] == b':' {
        let sign = if s[pos] == b'+' { 1i64 } else { -1i64 };
        let tz_h = i64::from(parse_digits_u32(&s[pos + 1..pos + 3])?);
        let tz_m = i64::from(parse_digits_u32(&s[pos + 4..pos + 6])?);
        if tz_h > 23 || tz_m > 59 {
            return None;
        }
        sign * (tz_h * 3600 + tz_m * 60)
    } else {
        return None;
    };

    let days = days_from_civil(year, month, day);
    let utc_secs = days
        .checked_mul(86_400)?
        .checked_add(i64::from(hour) * 3600 + i64::from(minute) * 60 + i64::from(second.min(59)))?
        .checked_sub(tz_offset_secs)?;
    if utc_secs < 0 {
        return None;
    }
    (utc_secs as u64)
        .checked_mul(1_000_000_000)?
        .checked_add(nanos)
}

/// Formats Unix epoch nanoseconds as an RFC3339 UTC string (`YYYY-MM-DDTHH:MM:SS.nnnnnnnnnZ`).
pub fn format_unix_nanos_rfc3339(epoch_nanos: u64) -> String {
    let total_secs = (epoch_nanos / 1_000_000_000) as i64;
    let subsec_nanos = epoch_nanos % 1_000_000_000;
    let days = total_secs.div_euclid(86_400);
    let sod = total_secs.rem_euclid(86_400);
    let (year, month, day) = civil_from_days(days);
    let hour = sod / 3600;
    let minute = (sod % 3600) / 60;
    let second = sod % 60;
    if subsec_nanos == 0 {
        format!("{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}:{second:02}Z")
    } else {
        format!(
            "{year:04}-{month:02}-{day:02}T{hour:02}:{minute:02}:{second:02}.{subsec_nanos:09}Z"
        )
    }
}

fn parse_digits_u32(bytes: &[u8]) -> Option<u32> {
    let mut val = 0u32;
    for &b in bytes {
        if !b.is_ascii_digit() {
            return None;
        }
        val = val.checked_mul(10)?.checked_add(u32::from(b - b'0'))?;
    }
    Some(val)
}

fn parse_digits_i64(bytes: &[u8]) -> Option<i64> {
    parse_digits_u32(bytes).map(i64::from)
}

fn days_in_month(year: i64, month: u32) -> u32 {
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if year % 4 == 0 && (year % 100 != 0 || year % 400 == 0) => 29,
        2 => 28,
        _ => 0,
    }
}

fn days_from_civil(year: i64, month: u32, day: u32) -> i64 {
    let y = year - if month <= 2 { 1 } else { 0 };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = (y - era * 400) as u32;
    let m = month;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + day - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146_097 + i64::from(doe) - 719_468
}

fn civil_from_days(days_since_epoch: i64) -> (i64, u32, u32) {
    let z = days_since_epoch + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = (z - era * 146_097) as u32;
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let y = i64::from(yoe) + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let year = y + if m <= 2 { 1 } else { 0 };
    (year, m, d)
}
