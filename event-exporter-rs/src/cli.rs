//! CLI flag parser compatible with GKE `event-exporter` single-dash Go flags and new Rust/OTLP
//! flags.

use crate::shard::ShardMode;
use std::collections::HashSet;
use std::time::Duration;

/// Destination sink backend selected at startup.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SinkKind {
    CloudLogging,
    Otlp,
}

/// Complete runtime configuration parsed from CLI flags.
#[derive(Clone, Debug)]
pub struct ExporterConfig {
    pub watch_pods: bool,
    pub system_namespaces: HashSet<String>,
    pub prometheus_endpoint: String,
    pub sink_kind: SinkKind,
    pub logging_endpoint: String,
    pub otlp_endpoint: String,
    pub project_id: Option<String>,
    pub cluster_name: Option<String>,
    pub location: Option<String>,
    pub universe_domain: Option<String>,
    pub node_token_broker: Option<String>,
    pub kube_api_endpoint: Option<String>,
    pub kube_ca_cert_path: Option<String>,
    pub kube_bearer_token_path: Option<String>,
    pub shard_mode: ShardMode,
    pub shard_count: u32,
    pub shard_index: Option<u32>,
    pub enable_watch_list: bool,
    pub list_page_size: u32,
    pub event_label_selector: Option<String>,
    pub flush_delay: Duration,
    pub max_batch_entries: usize,
    pub max_batch_bytes: usize,
    pub queue_capacity_bytes: usize,
    pub export_workers: usize,
}

impl Default for ExporterConfig {
    fn default() -> Self {
        Self {
            watch_pods: true,
            system_namespaces: ["kube-system".to_string(), "gke-connect".to_string()]
                .into_iter()
                .collect(),
            prometheus_endpoint: ":8080".to_string(),
            sink_kind: SinkKind::CloudLogging,
            logging_endpoint: "https://logging.googleapis.com/".to_string(),
            otlp_endpoint: "telemetry.googleapis.com:443".to_string(),
            project_id: None,
            cluster_name: None,
            location: None,
            universe_domain: None,
            node_token_broker: None,
            kube_api_endpoint: None,
            kube_ca_cert_path: None,
            kube_bearer_token_path: None,
            shard_mode: ShardMode::None,
            shard_count: 1,
            shard_index: None,
            enable_watch_list: true,
            list_page_size: 500,
            event_label_selector: None,
            flush_delay: Duration::from_secs(5),
            max_batch_entries: 500,
            max_batch_bytes: 4 * 1024 * 1024,
            queue_capacity_bytes: 16 * 1024 * 1024,
            export_workers: 4,
        }
    }
}

impl ExporterConfig {
    /// Parses CLI arguments (excluding `argv[0]`), supporting both `--flag=val`, `-flag=val`,
    /// space-separated flag values, and GKE's nested `-sink-opts="..."` string.
    pub fn from_args<I, S>(args: I) -> Result<Self, String>
    where
        I: IntoIterator<Item = S>,
        S: AsRef<str>,
    {
        let mut cfg = Self::default();
        let mut shard_mode_explicit = false;
        let tokens: Vec<String> = args.into_iter().map(|s| s.as_ref().to_string()).collect();
        apply_tokens(&tokens, &mut cfg, &mut shard_mode_explicit)?;
        if cfg.shard_count > 1 && !shard_mode_explicit {
            cfg.shard_mode = ShardMode::Client;
        }
        Ok(cfg)
    }
}

fn apply_tokens(
    tokens: &[String],
    cfg: &mut ExporterConfig,
    shard_mode_explicit: &mut bool,
) -> Result<(), String> {
    let mut idx = 0usize;
    while idx < tokens.len() {
        let raw = tokens[idx].trim();
        idx += 1;
        if raw.is_empty() || raw == "--" {
            continue;
        }
        let stripped = raw.trim_start_matches('-');
        if stripped.is_empty() {
            continue;
        }

        let (name, inline_val) = match stripped.split_once('=') {
            Some((k, v)) => (k, Some(v)),
            None => (stripped, None),
        };

        if is_boolean_flag(name) {
            let bool_val = match inline_val {
                Some(v) => parse_bool(name, v)?,
                None => true,
            };
            apply_bool_flag(name, bool_val, cfg);
            continue;
        }

        let value = match inline_val {
            Some(v) => v.to_string(),
            None => {
                if is_ignored_legacy_flag(name) {
                    if idx < tokens.len() && !tokens[idx].starts_with('-') {
                        idx += 1;
                    }
                    continue;
                }
                if idx >= tokens.len() {
                    return Err(format!("flag --{name} requires a value"));
                }
                let next = tokens[idx].clone();
                idx += 1;
                next
            }
        };

        apply_kv_flag(name, &value, cfg, shard_mode_explicit)?;
    }
    Ok(())
}

fn is_boolean_flag(name: &str) -> bool {
    matches!(
        name,
        "watch-pods"
            | "enable-pod-owner-label"
            | "enable-watch-list"
            | "lister-watcher-enable-streaming"
            | "enable-pod-label-Buffer"
            | "enable-pod-label-buffer"
            | "alsologtostderr"
            | "logtostderr"
    )
}

fn is_ignored_legacy_flag(name: &str) -> bool {
    matches!(
        name,
        "v" | "vmodule"
            | "log_backtrace_at"
            | "stderrthreshold"
            | "log_dir"
            | "kube-qps"
            | "kube-burst"
            | "max-dangling-pods"
            | "stackdriver-resource-model"
            | "custom-resource-type"
            | "storage-type"
    )
}

fn apply_bool_flag(name: &str, val: bool, cfg: &mut ExporterConfig) {
    match name {
        "watch-pods" | "enable-pod-owner-label" => cfg.watch_pods = val,
        "enable-watch-list" | "lister-watcher-enable-streaming" => cfg.enable_watch_list = val,
        _ => {}
    }
}

fn apply_kv_flag(
    name: &str,
    value: &str,
    cfg: &mut ExporterConfig,
    shard_mode_explicit: &mut bool,
) -> Result<(), String> {
    match name {
        "sink" => {
            cfg.sink_kind = match value.to_ascii_lowercase().as_str() {
                "stackdriver" | "cloudlogging" | "cloud-logging" => SinkKind::CloudLogging,
                "otlp" => SinkKind::Otlp,
                other => {
                    return Err(format!(
                        "unsupported --sink={other} (expected stackdriver or otlp)"
                    ));
                }
            };
        }
        "sink-opts" => {
            let sub_tokens = split_sink_opts(value);
            apply_tokens(&sub_tokens, cfg, shard_mode_explicit)?;
        }
        "system-namespaces" => {
            cfg.system_namespaces = value
                .split(',')
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(ToString::to_string)
                .collect();
        }
        "ResyncPeriod" | "resync-period" => {
            let _ = parse_go_duration(value)?;
        }
        "prometheus-endpoint" => {
            cfg.prometheus_endpoint = value.to_string();
        }
        "endpoint" => {
            if !value.is_empty() {
                cfg.logging_endpoint = value.to_string();
            }
        }
        "otlp-endpoint" => {
            if !value.is_empty() {
                cfg.otlp_endpoint = value.to_string();
            }
        }
        "project-id" | "ProjectID" => {
            cfg.project_id = non_empty_opt(value);
        }
        "cluster-name" => {
            cfg.cluster_name = non_empty_opt(value);
        }
        "location" | "Location" => {
            cfg.location = non_empty_opt(value);
        }
        "universe-domain" | "universeDomain" => {
            cfg.universe_domain = non_empty_opt(value);
        }
        "node-token-broker" => {
            cfg.node_token_broker = non_empty_opt(value);
        }
        "kube-api-endpoint" | "apiserver" => {
            cfg.kube_api_endpoint = non_empty_opt(value);
        }
        "kube-ca-cert" => {
            cfg.kube_ca_cert_path = non_empty_opt(value);
        }
        "kube-bearer-token-file" => {
            cfg.kube_bearer_token_path = non_empty_opt(value);
        }
        "shard-mode" => {
            cfg.shard_mode = ShardMode::parse(value)?;
            *shard_mode_explicit = true;
        }
        "shard-count" => {
            cfg.shard_count = value
                .parse::<u32>()
                .map_err(|e| format!("invalid --shard-count='{value}': {e}"))?;
            if cfg.shard_count == 0 {
                return Err("--shard-count must be >= 1".to_string());
            }
        }
        "shard-index" => {
            cfg.shard_index = Some(
                value
                    .parse::<u32>()
                    .map_err(|e| format!("invalid --shard-index='{value}': {e}"))?,
            );
        }
        "lister-watcher-options-limit" => {
            let parsed = value
                .parse::<u32>()
                .map_err(|e| format!("invalid --{name}='{value}': {e}"))?;
            if parsed > 0 {
                cfg.list_page_size = parsed;
            }
        }
        "list-page-size" => {
            cfg.list_page_size = value
                .parse::<u32>()
                .map_err(|e| format!("invalid --{name}='{value}': {e}"))?;
            if cfg.list_page_size == 0 {
                return Err(format!("--{name} must be >= 1"));
            }
        }
        "event-label-selector" => {
            cfg.event_label_selector = non_empty_opt(value);
        }
        "flush-delay" => {
            cfg.flush_delay = parse_go_duration(value)?;
        }
        "max-buffer-size" | "max-batch-entries" => {
            let parsed = value
                .parse::<usize>()
                .map_err(|e| format!("invalid --{name}='{value}': {e}"))?;
            if parsed > 0 {
                cfg.max_batch_entries = parsed.min(1000);
            }
        }
        "max-batch-bytes" => {
            cfg.max_batch_bytes = value
                .parse::<usize>()
                .map_err(|e| format!("invalid --max-batch-bytes='{value}': {e}"))?;
        }
        "queue-capacity-bytes" => {
            cfg.queue_capacity_bytes = value
                .parse::<usize>()
                .map_err(|e| format!("invalid --queue-capacity-bytes='{value}': {e}"))?;
        }
        "max-concurrency" | "export-workers" => {
            let parsed = value
                .parse::<usize>()
                .map_err(|e| format!("invalid --{name}='{value}': {e}"))?;
            if parsed > 0 {
                cfg.export_workers = parsed;
            }
        }
        other if is_ignored_legacy_flag(other) => {}
        other => return Err(format!("unknown flag: --{other}")),
    }
    Ok(())
}

fn non_empty_opt(raw: &str) -> Option<String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

fn parse_bool(flag: &str, raw: &str) -> Result<bool, String> {
    match raw.trim().to_ascii_lowercase().as_str() {
        "true" | "1" | "yes" => Ok(true),
        "false" | "0" | "no" => Ok(false),
        _ => Err(format!("invalid boolean value '{raw}' for --{flag}")),
    }
}

fn split_sink_opts(raw: &str) -> Vec<String> {
    let mut out = Vec::new();
    let mut current = String::new();
    let mut quote: Option<char> = None;
    for ch in raw.chars() {
        match quote {
            Some(q) if ch == q => quote = None,
            Some(_) => current.push(ch),
            None if ch == '"' || ch == '\'' => quote = Some(ch),
            None if ch.is_whitespace() => {
                if !current.is_empty() {
                    out.push(std::mem::take(&mut current));
                }
            }
            None => current.push(ch),
        }
    }
    if !current.is_empty() {
        out.push(current);
    }
    out
}

/// Parses standard Go-style duration strings such as `500ms`, `5s`, `30m`, `1h30m`, or `0`.
pub fn parse_go_duration(raw: &str) -> Result<Duration, String> {
    let trimmed = raw.trim();
    if trimmed == "0" {
        return Ok(Duration::ZERO);
    }
    if trimmed.is_empty() {
        return Err("empty duration string".to_string());
    }

    let bytes = trimmed.as_bytes();
    let mut pos = 0usize;
    let mut total = Duration::ZERO;
    while pos < bytes.len() {
        let num_start = pos;
        while pos < bytes.len() && bytes[pos].is_ascii_digit() {
            pos += 1;
        }
        if pos == num_start {
            return Err(format!("invalid duration '{trimmed}'"));
        }
        let amount = trimmed[num_start..pos]
            .parse::<u64>()
            .map_err(|e| format!("invalid duration number in '{trimmed}': {e}"))?;

        let unit_start = pos;
        while pos < bytes.len() && bytes[pos].is_ascii_alphabetic() {
            pos += 1;
        }
        let unit = &trimmed[unit_start..pos];
        let step = match unit {
            "ns" => Duration::from_nanos(amount),
            "us" => Duration::from_micros(amount),
            "ms" => Duration::from_millis(amount),
            "s" => Duration::from_secs(amount),
            "m" => Duration::from_secs(amount.saturating_mul(60)),
            "h" => Duration::from_secs(amount.saturating_mul(3600)),
            _ => return Err(format!("unsupported duration unit '{unit}' in '{trimmed}'")),
        };
        total = total.saturating_add(step);
    }
    Ok(total)
}
