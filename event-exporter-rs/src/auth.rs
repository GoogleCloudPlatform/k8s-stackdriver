//! GKE cluster coordinate discovery, Kubernetes ServiceAccount token rotation, and GCP access token
//! acquisition (GKE `NodeTokenBroker` external-account ADC, direct token broker URL, and GCE
//! Metadata).

use crate::cli::ExporterConfig;
use crate::enricher::ClusterCoordinates;
use crate::transport::http_request;
use serde::Deserialize;
use std::fs;
use std::sync::Mutex;
use std::time::{Duration, Instant};

const DEFAULT_K8S_TOKEN_PATH: &str = "/var/run/secrets/kubernetes.io/serviceaccount/token";
const DEFAULT_K8S_CA_PATH: &str = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt";
const DEFAULT_METADATA_HOST: &str = "169.254.169.254";
const K8S_TOKEN_REFRESH_INTERVAL: Duration = Duration::from_secs(60);

/// Formats a host (bracketing raw IPv6 literals) and port into an `scheme://host:port` base URL.
pub fn format_base_url(scheme: &str, host: &str, port: Option<u16>) -> String {
    let trimmed = host.trim();
    let bracketed = if trimmed.contains(':') && !trimmed.starts_with('[') {
        format!("[{trimmed}]")
    } else {
        trimmed.to_string()
    };
    match port {
        Some(p) => format!("{scheme}://{bracketed}:{p}"),
        None => format!("{scheme}://{bracketed}"),
    }
}

/// Resolves the Kubernetes API server base URL (`--kube-api-endpoint` or
/// `KUBERNETES_SERVICE_HOST`/`PORT`).
pub fn resolve_kube_api_base(cfg: &ExporterConfig) -> Result<String, String> {
    if let Some(explicit) = cfg.kube_api_endpoint.as_deref() {
        return Ok(explicit.trim_end_matches('/').to_string());
    }
    let host = std::env::var("KUBERNETES_SERVICE_HOST").map_err(|_| {
        "KUBERNETES_SERVICE_HOST not set and --kube-api-endpoint not provided".to_string()
    })?;
    let port = std::env::var("KUBERNETES_SERVICE_PORT")
        .ok()
        .and_then(|s| s.parse::<u16>().ok())
        .unwrap_or(443);
    Ok(format_base_url("https", &host, Some(port)))
}

/// Resolves the Kubernetes API CA bundle path (`None` for plain `http://` test endpoints).
pub fn resolve_kube_ca_path(cfg: &ExporterConfig, kube_api_base: &str) -> Option<String> {
    if kube_api_base.starts_with("http://") {
        return None;
    }
    if let Some(explicit) = cfg.kube_ca_cert_path.as_ref() {
        return Some(explicit.clone());
    }
    if std::path::Path::new(DEFAULT_K8S_CA_PATH).exists() {
        Some(DEFAULT_K8S_CA_PATH.to_string())
    } else {
        None
    }
}

/// Resolves the Cloud Logging base URL, substituting `--universe-domain` on Sovereign/TPC clusters.
pub fn resolve_logging_endpoint(cfg: &ExporterConfig) -> String {
    let raw = cfg.logging_endpoint.trim();
    if let Some(ud) = cfg.universe_domain.as_deref()
        && (raw == "https://logging.googleapis.com/" || raw == "https://logging.googleapis.com")
    {
        return format!("https://logging.{ud}/");
    }
    if raw.ends_with('/') {
        raw.to_string()
    } else {
        format!("{raw}/")
    }
}

/// Resolves the OTLP target endpoint, substituting `--universe-domain` when using default
/// `telemetry.googleapis.com:443`.
pub fn resolve_otlp_endpoint(cfg: &ExporterConfig) -> String {
    let raw = cfg.otlp_endpoint.trim();
    if let Some(ud) = cfg.universe_domain.as_deref()
        && (raw == "telemetry.googleapis.com:443" || raw == "telemetry.googleapis.com")
    {
        return format!("telemetry.{ud}:443");
    }
    raw.to_string()
}

/// Resolves `project_id`, `cluster_name`, and `location` from CLI flags or GCE Metadata Server.
pub fn resolve_cluster_coordinates(
    cfg: &ExporterConfig,
    metadata_host_override: Option<&str>,
) -> Result<ClusterCoordinates, String> {
    let metadata_base = metadata_base_url(metadata_host_override);
    let project_id = match cfg.project_id.clone() {
        Some(v) => v,
        None => fetch_metadata(&metadata_base, "/computeMetadata/v1/project/project-id")?,
    };
    let cluster_name = match cfg.cluster_name.clone() {
        Some(v) => v,
        None => fetch_metadata(
            &metadata_base,
            "/computeMetadata/v1/instance/attributes/cluster-name",
        )?,
    };
    let location = match cfg.location.clone() {
        Some(v) => v,
        None => fetch_metadata(
            &metadata_base,
            "/computeMetadata/v1/instance/attributes/cluster-location",
        )?,
    };
    Ok(ClusterCoordinates {
        project_id,
        cluster_name,
        location,
    })
}

fn metadata_base_url(override_host: Option<&str>) -> String {
    let raw_host = override_host
        .map(ToString::to_string)
        .or_else(|| std::env::var("GCE_METADATA_HOST").ok())
        .filter(|s| !s.trim().is_empty())
        .unwrap_or_else(|| DEFAULT_METADATA_HOST.to_string());
    if raw_host.starts_with("http://") || raw_host.starts_with("https://") {
        return raw_host.trim_end_matches('/').to_string();
    }
    if let Some((h, p)) = raw_host.rsplit_once(':')
        && !h.contains(':')
        && let Ok(port) = p.parse::<u16>()
    {
        return format_base_url("http", h, Some(port));
    }
    format_base_url("http", &raw_host, None)
}

fn fetch_metadata(metadata_base: &str, path: &str) -> Result<String, String> {
    let url = format!("{metadata_base}{path}");
    let resp = http_request(
        "GET",
        &url,
        &[("Metadata-Flavor", "Google")],
        &[],
        false,
        None,
        Duration::from_secs(5),
    )?;
    if resp.status != 200 {
        return Err(format!("GCE metadata {path} returned HTTP {}", resp.status));
    }
    Ok(String::from_utf8_lossy(&resp.body).trim().to_string())
}

/// Reloading reader for projected Kubernetes ServiceAccount bearer tokens.
pub struct KubeTokenSource {
    token_path: Option<String>,
    cached: Mutex<Option<(String, Instant)>>,
}

impl KubeTokenSource {
    pub fn new(explicit_path: Option<String>) -> Self {
        let token_path = explicit_path.or_else(|| {
            if std::path::Path::new(DEFAULT_K8S_TOKEN_PATH).exists() {
                Some(DEFAULT_K8S_TOKEN_PATH.to_string())
            } else {
                None
            }
        });
        Self {
            token_path,
            cached: Mutex::new(None),
        }
    }

    pub fn bearer_token(&self) -> Result<Option<String>, String> {
        let Some(path) = self.token_path.as_deref() else {
            return Ok(None);
        };
        let mut guard = self.cached.lock().unwrap_or_else(|e| e.into_inner());
        if let Some((token, loaded_at)) = guard.as_ref()
            && loaded_at.elapsed() < K8S_TOKEN_REFRESH_INTERVAL
        {
            return Ok(Some(token.clone()));
        }
        let raw = fs::read_to_string(path)
            .map_err(|e| format!("failed to read Kubernetes SA token '{path}': {e}"))?;
        let trimmed = raw.trim().to_string();
        *guard = Some((trimmed.clone(), Instant::now()));
        Ok(Some(trimmed))
    }
}

#[derive(Clone, Debug, Deserialize)]
struct ExternalAccountAdc {
    #[serde(default, rename = "type")]
    cred_type: String,
    #[serde(default)]
    audience: String,
    #[serde(default)]
    subject_token_type: String,
    #[serde(default)]
    token_url: String,
    #[serde(default)]
    credential_source: Option<ExternalCredentialSource>,
}

#[derive(Clone, Debug, Deserialize)]
struct ExternalCredentialSource {
    #[serde(default)]
    file: String,
}

#[derive(Deserialize)]
struct OAuthTokenResponse {
    access_token: String,
    #[serde(default)]
    expires_in: u64,
}

struct CachedCloudToken {
    access_token: String,
    expires_at: Instant,
}

/// Acquires and caches GCP OAuth2 access tokens via `NodeTokenBroker` external-account ADC,
/// direct `--node-token-broker` HTTP endpoint, or GCE Metadata Server fallback.
pub struct CloudTokenProvider {
    adc_path: Option<String>,
    node_token_broker_url: Option<String>,
    metadata_base: String,
    disabled: bool,
    cached: Mutex<Option<CachedCloudToken>>,
}

impl CloudTokenProvider {
    pub fn from_config(cfg: &ExporterConfig, metadata_host_override: Option<&str>) -> Self {
        let adc_path = std::env::var("GOOGLE_APPLICATION_CREDENTIALS")
            .ok()
            .filter(|s| !s.trim().is_empty());
        Self {
            adc_path,
            node_token_broker_url: cfg.node_token_broker.clone(),
            metadata_base: metadata_base_url(metadata_host_override),
            disabled: false,
            cached: Mutex::new(None),
        }
    }

    pub fn unauthenticated() -> Self {
        Self {
            adc_path: None,
            node_token_broker_url: None,
            metadata_base: String::new(),
            disabled: true,
            cached: Mutex::new(None),
        }
    }

    pub fn access_token(&self) -> Result<Option<String>, String> {
        if self.disabled {
            return Ok(None);
        }
        {
            let guard = self.cached.lock().unwrap_or_else(|e| e.into_inner());
            if let Some(entry) = guard.as_ref()
                && Instant::now() < entry.expires_at
            {
                return Ok(Some(entry.access_token.clone()));
            }
        }

        let (token, ttl) = self.fetch_fresh_token()?;
        let refresh_lead = Duration::from_secs(60).min(ttl / 2);
        let expires_at = Instant::now() + ttl.saturating_sub(refresh_lead);
        let mut guard = self.cached.lock().unwrap_or_else(|e| e.into_inner());
        *guard = Some(CachedCloudToken {
            access_token: token.clone(),
            expires_at,
        });
        Ok(Some(token))
    }

    fn fetch_fresh_token(&self) -> Result<(String, Duration), String> {
        if let Some(adc_file) = self.adc_path.as_deref() {
            match exchange_external_account_adc(adc_file) {
                Ok(pair) => return Ok(pair),
                Err(adc_err) => {
                    // Preserve GKE MODE_TOKEN_BROKER_WITH_FALLBACK resilience onto GCE metadata.
                    if let Ok(fallback) = self.fetch_gce_metadata_token() {
                        return Ok(fallback);
                    }
                    return Err(adc_err);
                }
            }
        }
        if let Some(broker_url) = self.node_token_broker_url.as_deref() {
            return fetch_token_from_url(broker_url, &[]);
        }
        self.fetch_gce_metadata_token()
    }

    fn fetch_gce_metadata_token(&self) -> Result<(String, Duration), String> {
        let url = format!(
            "{}/computeMetadata/v1/instance/service-accounts/default/token",
            self.metadata_base
        );
        fetch_token_from_url(&url, &[("Metadata-Flavor", "Google")])
    }
}

fn exchange_external_account_adc(adc_path: &str) -> Result<(String, Duration), String> {
    let raw_adc = fs::read_to_string(adc_path)
        .map_err(|e| format!("failed to read GOOGLE_APPLICATION_CREDENTIALS '{adc_path}': {e}"))?;
    let parsed: ExternalAccountAdc = serde_json::from_str(&raw_adc)
        .map_err(|e| format!("invalid external_account JSON in '{adc_path}': {e}"))?;
    if parsed.cred_type != "external_account" {
        return Err(format!(
            "unsupported ADC credential type '{}'",
            parsed.cred_type
        ));
    }
    let ksa_path = parsed
        .credential_source
        .as_ref()
        .map(|cs| cs.file.trim())
        .filter(|p| !p.is_empty())
        .ok_or_else(|| "external_account ADC missing credential_source.file".to_string())?;
    let subject_token = fs::read_to_string(ksa_path)
        .map_err(|e| format!("failed to read KSA subject token '{ksa_path}': {e}"))?
        .trim()
        .to_string();

    let body = serde_json::json!({
        "grantType": "urn:ietf:params:oauth:grant-type:token-exchange",
        "audience": parsed.audience,
        "scope": "https://www.googleapis.com/auth/logging.write",
        "requestedTokenType": "urn:ietf:params:oauth:token-type:access_token",
        "subjectToken": subject_token,
        "subjectTokenType": parsed.subject_token_type,
    });
    let payload = serde_json::to_vec(&body).map_err(|e| e.to_string())?;
    let resp = http_request(
        "POST",
        &parsed.token_url,
        &[("Content-Type", "application/json")],
        &payload,
        false,
        None,
        Duration::from_secs(10),
    )?;
    if resp.status != 200 {
        return Err(format!(
            "STS token exchange at '{}' failed with HTTP {}: {}",
            parsed.token_url,
            resp.status,
            String::from_utf8_lossy(&resp.body)
        ));
    }
    parse_oauth_token_body(&resp.body)
}

fn fetch_token_from_url(url: &str, headers: &[(&str, &str)]) -> Result<(String, Duration), String> {
    let resp = http_request(
        "GET",
        url,
        headers,
        &[],
        false,
        None,
        Duration::from_secs(5),
    )?;
    if resp.status != 200 {
        return Err(format!(
            "token endpoint '{url}' returned HTTP {}",
            resp.status
        ));
    }
    parse_oauth_token_body(&resp.body)
}

fn parse_oauth_token_body(body: &[u8]) -> Result<(String, Duration), String> {
    let parsed: OAuthTokenResponse =
        serde_json::from_slice(body).map_err(|e| format!("invalid OAuth token JSON: {e}"))?;
    if parsed.access_token.is_empty() {
        return Err("OAuth token response had empty access_token".to_string());
    }
    let ttl_secs = if parsed.expires_in == 0 {
        300
    } else {
        parsed.expires_in
    };
    Ok((parsed.access_token, Duration::from_secs(ttl_secs)))
}
