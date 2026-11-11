//! Dual-Sink export engines: Cloud Logging v2 (`entries:write` gzipped JSON) and native
//! OpenTelemetry OTLP Logs (`ExportLogsServiceRequest` via gRPC `telemetry.googleapis.com:443` or
//! OTLP/HTTP `/v1/logs`).

use crate::auth::CloudTokenProvider;
use crate::enricher::{ClusterCoordinates, EnrichedEntry, EntryPayload, MonitoredResourceKind};
use crate::metrics::{MetricsRegistry, grpc_status_to_http_code};
use crate::otlp_proto::{
    AnyValue, ArrayValue, ExportLogsServiceRequest, ExportLogsServiceResponse,
    InstrumentationScope, KeyValue, KeyValueList, LogRecord, Resource, ResourceLogs, ScopeLogs,
    SeverityNumber, any_value,
};
use crate::transport::{OtlpGrpcClient, OtlpTarget, http_request, parse_otlp_target};
use prost::Message;
use serde_json::{Map, Value, json};
use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const EXPORT_RPC_TIMEOUT: Duration = Duration::from_secs(30);

/// Outcome of a single batch export attempt, driving retry/backoff and LOrIS metrics.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum BatchExportOutcome {
    Success { sent_entries: usize },
    RetryableFailure { status_code: String, reason: String },
    PermanentFailure { status_code: String, reason: String },
}

/// Sink implementation invoked by export worker threads.
pub trait EventSink: Send + Sync {
    fn export_batch(
        &self,
        batch: &[EnrichedEntry],
        metrics: &MetricsRegistry,
    ) -> BatchExportOutcome;
}

/// Builds the JSON request body for Cloud Logging `POST /v2/entries:write`.
pub fn build_cloud_logging_write_body(
    coords: &ClusterCoordinates,
    batch: &[EnrichedEntry],
) -> Vec<u8> {
    let mut entries_json = Vec::with_capacity(batch.len());
    for entry in batch {
        let mut resource_labels = Map::new();
        for (k, v) in entry.resource.resource_labels(coords) {
            resource_labels.insert(k.to_string(), Value::String(v));
        }

        let mut item = Map::new();
        item.insert(
            "insertId".to_string(),
            Value::String(entry.insert_id.clone()),
        );
        item.insert(
            "timestamp".to_string(),
            Value::String(entry.timestamp_rfc3339.clone()),
        );
        item.insert(
            "severity".to_string(),
            Value::String(entry.severity.cloud_logging_str().to_string()),
        );
        item.insert(
            "resource".to_string(),
            json!({
                "type": entry.resource.resource_type(),
                "labels": resource_labels,
            }),
        );

        if !entry.labels.is_empty() {
            let mut lbl_map = Map::new();
            for (k, v) in &entry.labels {
                lbl_map.insert(k.clone(), Value::String(v.clone()));
            }
            item.insert("labels".to_string(), Value::Object(lbl_map));
        }

        match &entry.payload {
            EntryPayload::Json(map) => {
                item.insert("jsonPayload".to_string(), Value::Object(map.clone()));
            }
            EntryPayload::Text(text) => {
                item.insert("textPayload".to_string(), Value::String(text.clone()));
            }
        }
        entries_json.push(Value::Object(item));
    }

    let request = json!({
        "logName": format!("projects/{}/logs/events", coords.project_id),
        "partialSuccess": true,
        "entries": entries_json,
    });
    serde_json::to_vec(&request).unwrap_or_default()
}

/// Cloud Logging v2 HTTP/JSON sink (`POST <endpoint>/v2/entries:write` with `Content-Encoding:
/// gzip`).
pub struct CloudLoggingSink {
    write_url: String,
    coords: ClusterCoordinates,
    token_provider: Arc<CloudTokenProvider>,
    ca_cert_path: Option<String>,
}

impl CloudLoggingSink {
    pub fn new(
        logging_base_endpoint: &str,
        coords: ClusterCoordinates,
        token_provider: Arc<CloudTokenProvider>,
        ca_cert_path: Option<String>,
    ) -> Self {
        let base = logging_base_endpoint.trim_end_matches('/');
        Self {
            write_url: format!("{base}/v2/entries:write"),
            coords,
            token_provider,
            ca_cert_path,
        }
    }
}

impl EventSink for CloudLoggingSink {
    fn export_batch(
        &self,
        batch: &[EnrichedEntry],
        metrics: &MetricsRegistry,
    ) -> BatchExportOutcome {
        if batch.is_empty() {
            return BatchExportOutcome::Success { sent_entries: 0 };
        }

        let auth_header = match self.token_provider.access_token() {
            Ok(Some(token)) => Some(format!("Bearer {token}")),
            Ok(None) => None,
            Err(err) => {
                metrics.inc_sink_request("401");
                return BatchExportOutcome::RetryableFailure {
                    status_code: "401".to_string(),
                    reason: err,
                };
            }
        };

        let payload = build_cloud_logging_write_body(&self.coords, batch);
        let mut headers: Vec<(&str, &str)> = vec![
            ("Content-Type", "application/json"),
            ("X-Goog-User-Project", self.coords.project_id.as_str()),
        ];
        if let Some(bearer) = auth_header.as_deref() {
            headers.push(("Authorization", bearer));
        }

        let response = http_request(
            "POST",
            &self.write_url,
            &headers,
            &payload,
            true,
            self.ca_cert_path.as_deref(),
            EXPORT_RPC_TIMEOUT,
        );

        match response {
            Ok(resp) => {
                let code_str = resp.status.to_string();
                metrics.inc_sink_request(&code_str);
                if (200..300).contains(&resp.status) {
                    let rejected = count_cloud_logging_partial_errors(&resp.body);
                    let accepted = batch.len().saturating_sub(rejected);
                    metrics.add_sent_entries(accepted as u64);
                    metrics.observe_entries_delivery_latency(&batch[..accepted], now_unix_nanos());
                    BatchExportOutcome::Success {
                        sent_entries: accepted,
                    }
                } else if is_retryable_http_status(resp.status) {
                    BatchExportOutcome::RetryableFailure {
                        status_code: code_str,
                        reason: String::from_utf8_lossy(&resp.body).into_owned(),
                    }
                } else {
                    BatchExportOutcome::PermanentFailure {
                        status_code: code_str,
                        reason: String::from_utf8_lossy(&resp.body).into_owned(),
                    }
                }
            }
            Err(err) => {
                metrics.inc_sink_request("503");
                BatchExportOutcome::RetryableFailure {
                    status_code: "503".to_string(),
                    reason: err,
                }
            }
        }
    }
}

fn count_cloud_logging_partial_errors(body: &[u8]) -> usize {
    let Ok(val) = serde_json::from_slice::<Value>(body) else {
        return 0;
    };
    val.get("logEntryErrors")
        .and_then(Value::as_object)
        .map(|m| m.len())
        .unwrap_or(0)
}

fn is_retryable_http_status(status: u16) -> bool {
    status == 408 || status == 429 || (500..600).contains(&status)
}

#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord)]
enum ResourceGroupKey {
    Pod {
        namespace_name: String,
        pod_name: String,
    },
    Node {
        node_name: String,
    },
    Cluster,
}

impl ResourceGroupKey {
    fn from_resource(res: &MonitoredResourceKind) -> Self {
        match res {
            MonitoredResourceKind::K8sPod {
                namespace_name,
                pod_name,
            } => Self::Pod {
                namespace_name: namespace_name.clone(),
                pod_name: pod_name.clone(),
            },
            MonitoredResourceKind::K8sNode { node_name } => Self::Node {
                node_name: node_name.clone(),
            },
            MonitoredResourceKind::K8sCluster => Self::Cluster,
        }
    }
}

/// Builds an `ExportLogsServiceRequest` protobuf matching Google Cloud `telemetry.googleapis.com`
/// OTLP-to-`LogEntry` translation (`gcp.resource_type`, `gcp.log_name`,
/// `logging.googleapis.com/insertId`).
pub fn build_otlp_export_request(
    coords: &ClusterCoordinates,
    batch: &[EnrichedEntry],
) -> ExportLogsServiceRequest {
    let mut grouped: BTreeMap<ResourceGroupKey, Vec<&EnrichedEntry>> = BTreeMap::new();
    for entry in batch {
        grouped
            .entry(ResourceGroupKey::from_resource(&entry.resource))
            .or_default()
            .push(entry);
    }

    let mut resource_logs = Vec::with_capacity(grouped.len());
    for (_, entries) in grouped {
        let first_res = &entries[0].resource;
        let mut res_attrs = Vec::with_capacity(6);
        res_attrs.push(KeyValue::string(
            "gcp.resource_type",
            first_res.resource_type(),
        ));
        for (k, v) in first_res.resource_labels(coords) {
            res_attrs.push(KeyValue::string(k, v));
        }

        let mut log_records = Vec::with_capacity(entries.len());
        for entry in entries {
            let mut attrs = Vec::with_capacity(entry.labels.len() + 2);
            attrs.push(KeyValue::string("gcp.log_name", "events"));
            if !entry.insert_id.is_empty() {
                attrs.push(KeyValue::string(
                    "logging.googleapis.com/insertId",
                    &entry.insert_id,
                ));
            }
            for (k, v) in &entry.labels {
                attrs.push(KeyValue::string(k, v));
            }

            let body = match &entry.payload {
                EntryPayload::Json(map) => Some(json_map_to_any_value(map)),
                EntryPayload::Text(text) => Some(AnyValue {
                    value: Some(any_value::Value::StringValue(text.clone())),
                }),
            };

            let sev = match entry.severity.otlp_severity_number() {
                13 => SeverityNumber::Warn as i32,
                _ => SeverityNumber::Info as i32,
            };

            log_records.push(LogRecord {
                time_unix_nano: entry.timestamp_unix_nanos,
                observed_time_unix_nano: entry.observed_unix_nanos,
                severity_number: sev,
                severity_text: entry.severity.cloud_logging_str().to_string(),
                body,
                attributes: attrs,
                ..Default::default()
            });
        }

        resource_logs.push(ResourceLogs {
            resource: Some(Resource {
                attributes: res_attrs,
                dropped_attributes_count: 0,
            }),
            scope_logs: vec![ScopeLogs {
                scope: Some(InstrumentationScope {
                    name: "gke-event-exporter-rs".to_string(),
                    version: "0.1.0".to_string(),
                    attributes: Vec::new(),
                    dropped_attributes_count: 0,
                }),
                log_records,
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        });
    }

    ExportLogsServiceRequest { resource_logs }
}

fn json_map_to_any_value(map: &Map<String, Value>) -> AnyValue {
    let mut values = Vec::with_capacity(map.len());
    for (k, v) in map {
        if v.is_null() {
            continue;
        }
        values.push(KeyValue {
            key: k.clone(),
            value: Some(json_value_to_any_value(v)),
        });
    }
    AnyValue {
        value: Some(any_value::Value::KvlistValue(KeyValueList { values })),
    }
}

fn json_value_to_any_value(val: &Value) -> AnyValue {
    let inner = match val {
        Value::Null => None,
        Value::Bool(b) => Some(any_value::Value::BoolValue(*b)),
        Value::Number(num) => {
            if let Some(i) = num.as_i64() {
                Some(any_value::Value::IntValue(i))
            } else if let Some(u) = num.as_u64() {
                if let Ok(i) = i64::try_from(u) {
                    Some(any_value::Value::IntValue(i))
                } else {
                    Some(any_value::Value::DoubleValue(u as f64))
                }
            } else {
                num.as_f64().map(any_value::Value::DoubleValue)
            }
        }
        Value::String(s) => Some(any_value::Value::StringValue(s.clone())),
        Value::Array(items) => Some(any_value::Value::ArrayValue(ArrayValue {
            values: items.iter().map(json_value_to_any_value).collect(),
        })),
        Value::Object(sub_map) => return json_map_to_any_value(sub_map),
    };
    AnyValue { value: inner }
}

enum OtlpTransportBackend {
    Grpc(OtlpGrpcClient),
    Http {
        url: String,
        ca_cert_path: Option<String>,
    },
}

/// Native OpenTelemetry OTLP Logs sink (`telemetry.googleapis.com:443` gRPC or OTLP/HTTP
/// `/v1/logs`).
pub struct OtlpSink {
    backend: OtlpTransportBackend,
    coords: ClusterCoordinates,
    token_provider: Arc<CloudTokenProvider>,
}

impl OtlpSink {
    pub fn new(
        otlp_endpoint: &str,
        coords: ClusterCoordinates,
        token_provider: Arc<CloudTokenProvider>,
        ca_cert_path: Option<String>,
    ) -> Result<Self, String> {
        let parsed = parse_otlp_target(otlp_endpoint)?;
        let backend = match parsed {
            OtlpTarget::Grpc { endpoint, use_tls } => OtlpTransportBackend::Grpc(
                OtlpGrpcClient::new(&endpoint, use_tls, ca_cert_path.as_deref())?,
            ),
            OtlpTarget::HttpProtobuf { url } => OtlpTransportBackend::Http { url, ca_cert_path },
        };
        Ok(Self {
            backend,
            coords,
            token_provider,
        })
    }
}

impl EventSink for OtlpSink {
    fn export_batch(
        &self,
        batch: &[EnrichedEntry],
        metrics: &MetricsRegistry,
    ) -> BatchExportOutcome {
        if batch.is_empty() {
            return BatchExportOutcome::Success { sent_entries: 0 };
        }

        let token = match self.token_provider.access_token() {
            Ok(tok) => tok,
            Err(err) => {
                metrics.inc_sink_request("401");
                return BatchExportOutcome::RetryableFailure {
                    status_code: "401".to_string(),
                    reason: err,
                };
            }
        };

        let request_msg = build_otlp_export_request(&self.coords, batch);

        match &self.backend {
            OtlpTransportBackend::Grpc(client) => {
                let res = client.export(
                    request_msg,
                    token.as_deref(),
                    Some(self.coords.project_id.as_str()),
                    EXPORT_RPC_TIMEOUT,
                );
                let http_code = grpc_status_to_http_code(res.grpc_status);
                metrics.inc_sink_request(http_code);

                if res.grpc_status == 0 {
                    let accepted = batch.len().saturating_sub(res.rejected_log_records);
                    metrics.add_sent_entries(accepted as u64);
                    metrics.observe_entries_delivery_latency(&batch[..accepted], now_unix_nanos());
                    BatchExportOutcome::Success {
                        sent_entries: accepted,
                    }
                } else if matches!(res.grpc_status, 1 | 4 | 8 | 10 | 14) {
                    BatchExportOutcome::RetryableFailure {
                        status_code: http_code.to_string(),
                        reason: res.error_message,
                    }
                } else {
                    BatchExportOutcome::PermanentFailure {
                        status_code: http_code.to_string(),
                        reason: res.error_message,
                    }
                }
            }
            OtlpTransportBackend::Http { url, ca_cert_path } => {
                let proto_bytes = request_msg.encode_to_vec();
                let auth_header = token.as_ref().map(|t| format!("Bearer {t}"));
                let mut headers: Vec<(&str, &str)> = vec![
                    ("Content-Type", "application/x-protobuf"),
                    ("X-Goog-User-Project", self.coords.project_id.as_str()),
                ];
                if let Some(bearer) = auth_header.as_deref() {
                    headers.push(("Authorization", bearer));
                }
                let resp = http_request(
                    "POST",
                    url,
                    &headers,
                    &proto_bytes,
                    true,
                    ca_cert_path.as_deref(),
                    EXPORT_RPC_TIMEOUT,
                );
                match resp {
                    Ok(r) => {
                        let code_str = r.status.to_string();
                        metrics.inc_sink_request(&code_str);
                        if (200..300).contains(&r.status) {
                            let rejected = count_otlp_rejected_records(&r.body);
                            let accepted = batch.len().saturating_sub(rejected);
                            metrics.add_sent_entries(accepted as u64);
                            metrics.observe_entries_delivery_latency(
                                &batch[..accepted],
                                now_unix_nanos(),
                            );
                            BatchExportOutcome::Success {
                                sent_entries: accepted,
                            }
                        } else if is_retryable_http_status(r.status) {
                            BatchExportOutcome::RetryableFailure {
                                status_code: code_str,
                                reason: String::from_utf8_lossy(&r.body).into_owned(),
                            }
                        } else {
                            BatchExportOutcome::PermanentFailure {
                                status_code: code_str,
                                reason: String::from_utf8_lossy(&r.body).into_owned(),
                            }
                        }
                    }
                    Err(err) => {
                        metrics.inc_sink_request("503");
                        BatchExportOutcome::RetryableFailure {
                            status_code: "503".to_string(),
                            reason: err,
                        }
                    }
                }
            }
        }
    }
}

fn count_otlp_rejected_records(response_proto: &[u8]) -> usize {
    if response_proto.is_empty() {
        return 0;
    }
    let Ok(resp) = ExportLogsServiceResponse::decode(response_proto) else {
        return 0;
    };
    resp.partial_success
        .map(|ps| ps.rejected_log_records.max(0) as usize)
        .unwrap_or(0)
}

fn now_unix_nanos() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos() as u64)
        .unwrap_or(0)
}
