//! Pure-Rust HTTP/1.1, HTTP/2 (`reqwest` + `rustls`/`ring`), streaming Kubernetes Watch line
//! framing, gzip compression (`flate2`), and OTLP gRPC (`tonic`) transport layer.

use crate::otlp_proto::{ExportLogsServiceRequest, ExportLogsServiceResponse};
use flate2::Compression;
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use http::uri::PathAndQuery;
use std::collections::HashMap;
use std::future::Future;
use std::io::{Read, Write};
use std::sync::atomic::{AtomicU8, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;
use tokio::runtime::Runtime;
use tonic::client::Grpc;
use tonic::codec::ProstCodec;
use tonic::metadata::MetadataValue;
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Endpoint};

const MAX_STREAM_LINE_BYTES: usize = 32 * 1024 * 1024;
const MAX_ERROR_BODY_BYTES: usize = 64 * 1024;

fn shared_runtime() -> &'static Runtime {
    static RUNTIME: OnceLock<Runtime> = OnceLock::new();
    RUNTIME.get_or_init(|| {
        let _ = rustls::crypto::ring::default_provider().install_default();
        tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .thread_name("ee-tokio")
            .build()
            .expect("failed to initialize tokio transport runtime")
    })
}

fn block_on_transport<F: Future>(fut: F) -> F::Output {
    let rt = shared_runtime();
    if tokio::runtime::Handle::try_current().is_ok() {
        tokio::task::block_in_place(|| rt.block_on(fut))
    } else {
        rt.block_on(fut)
    }
}

/// Registers safe POSIX `SIGINT` and `SIGTERM` listeners on the shared Tokio runtime that set
/// `stop_flag` to `1` upon shutdown signal delivery.
pub fn install_shutdown_signal_listener(stop_flag: Arc<AtomicU8>) -> Result<(), String> {
    block_on_transport(async move {
        let mut sigint = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt())
            .map_err(|e| format!("failed to register SIGINT handler: {e}"))?;
        let mut sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .map_err(|e| format!("failed to register SIGTERM handler: {e}"))?;
        tokio::spawn(async move {
            tokio::select! {
                _ = sigint.recv() => {}
                _ = sigterm.recv() => {}
            }
            stop_flag.store(1, Ordering::Release);
        });
        Ok(())
    })
}

fn configure_env_proxies(mut builder: reqwest::ClientBuilder) -> reqwest::ClientBuilder {
    // Always bypass loopback and GCE link-local metadata even when HTTP(S)_PROXY is set without
    // NO_PROXY (matching Go net/http.ProxyFromEnvironment behavior on private GKE nodes).
    const BUILTIN_NO_PROXY: &str =
        "localhost,127.0.0.0/8,::1,169.254.0.0/16,metadata.google.internal";
    builder = builder.no_proxy();

    let user_no_proxy = std::env::var("no_proxy")
        .or_else(|_| std::env::var("NO_PROXY"))
        .unwrap_or_default();
    let merged = if user_no_proxy.trim().is_empty() {
        BUILTIN_NO_PROXY.to_string()
    } else {
        format!("{BUILTIN_NO_PROXY},{}", user_no_proxy.trim())
    };
    let no_proxy = reqwest::NoProxy::from_string(&merged);

    if let Ok(val) = std::env::var("http_proxy").or_else(|_| std::env::var("HTTP_PROXY"))
        && !val.trim().is_empty()
        && let Ok(proxy) = reqwest::Proxy::http(val.trim())
    {
        builder = builder.proxy(proxy.no_proxy(no_proxy.clone()));
    }
    if let Ok(val) = std::env::var("https_proxy").or_else(|_| std::env::var("HTTPS_PROXY"))
        && !val.trim().is_empty()
        && let Ok(proxy) = reqwest::Proxy::https(val.trim())
    {
        builder = builder.proxy(proxy.no_proxy(no_proxy.clone()));
    }
    if let Ok(val) = std::env::var("all_proxy").or_else(|_| std::env::var("ALL_PROXY"))
        && !val.trim().is_empty()
        && let Ok(proxy) = reqwest::Proxy::all(val.trim())
    {
        builder = builder.proxy(proxy.no_proxy(no_proxy));
    }
    builder
}

fn http_client_for(ca_cert_path: Option<&str>) -> Result<reqwest::Client, String> {
    static CLIENTS: OnceLock<Mutex<HashMap<Option<String>, reqwest::Client>>> = OnceLock::new();
    let pool = CLIENTS.get_or_init(|| Mutex::new(HashMap::new()));
    let key = ca_cert_path.map(str::to_string);
    {
        let guard = pool.lock().unwrap_or_else(|e| e.into_inner());
        if let Some(existing) = guard.get(&key) {
            return Ok(existing.clone());
        }
    }

    let _ = shared_runtime();
    let mut builder = configure_env_proxies(
        reqwest::Client::builder()
            .use_rustls_tls()
            .pool_idle_timeout(Duration::from_secs(90))
            .tcp_keepalive(Duration::from_secs(30)),
    );

    if let Some(path) = ca_cert_path {
        let pem = std::fs::read(path)
            .map_err(|e| format!("failed to read CA certificate bundle '{path}': {e}"))?;
        let certs = reqwest::Certificate::from_pem_bundle(&pem)
            .map_err(|e| format!("invalid PEM CA certificate in '{path}': {e}"))?;
        for cert in certs {
            builder = builder.add_root_certificate(cert);
        }
    }

    let client = builder
        .build()
        .map_err(|e| format!("failed to build HTTP client: {e}"))?;

    let mut guard = pool.lock().unwrap_or_else(|e| e.into_inner());
    guard.insert(key, client.clone());
    Ok(client)
}

/// Parsed components of an HTTP or HTTPS URL.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ParsedUrl {
    pub use_tls: bool,
    pub host: String,
    pub port: u16,
    pub path_and_query: String,
}

/// Parses an `http://` or `https://` URL, supporting IPv4, hostnames, and bracketed IPv6 literals.
pub fn parse_http_url(raw: &str) -> Result<ParsedUrl, String> {
    let trimmed = raw.trim();
    let (use_tls, rest, default_port) = if let Some(after) = trimmed.strip_prefix("https://") {
        (true, after, 443u16)
    } else if let Some(after) = trimmed.strip_prefix("http://") {
        (false, after, 80u16)
    } else {
        return Err(format!(
            "URL must start with http:// or https://: {trimmed}"
        ));
    };

    let (authority, path_and_query) = match rest.find('/') {
        Some(idx) => (&rest[..idx], rest[idx..].to_string()),
        None => (rest, "/".to_string()),
    };
    if authority.is_empty() {
        return Err(format!("URL has empty host authority: {trimmed}"));
    }

    let (host, port) = parse_host_port(authority, default_port)?;
    Ok(ParsedUrl {
        use_tls,
        host,
        port,
        path_and_query,
    })
}

/// Endpoint configuration for OTLP export (gRPC by default, HTTP `/v1/logs` when explicitly
/// targeted).
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum OtlpTarget {
    Grpc { endpoint: String, use_tls: bool },
    HttpProtobuf { url: String },
}

/// Parses an OTLP endpoint flag into either a gRPC channel target or an OTLP/HTTP `/v1/logs` URL.
pub fn parse_otlp_target(raw: &str) -> Result<OtlpTarget, String> {
    let trimmed = raw.trim().trim_end_matches('/');
    if trimmed.is_empty() {
        return Err("OTLP endpoint cannot be empty".to_string());
    }
    if (trimmed.starts_with("http://") || trimmed.starts_with("https://"))
        && trimmed.ends_with("/v1/logs")
    {
        let _ = parse_http_url(trimmed)?;
        return Ok(OtlpTarget::HttpProtobuf {
            url: trimmed.to_string(),
        });
    }

    if let Some(after) = trimmed.strip_prefix("https://") {
        let authority = after.split('/').next().unwrap_or(after);
        let (host, port) = parse_host_port(authority, 443)?;
        return Ok(OtlpTarget::Grpc {
            endpoint: format_host_port(&host, port),
            use_tls: true,
        });
    }
    if let Some(after) = trimmed
        .strip_prefix("http://")
        .or_else(|| trimmed.strip_prefix("grpc://"))
    {
        let authority = after.split('/').next().unwrap_or(after);
        let (host, port) = parse_host_port(authority, 4317)?;
        return Ok(OtlpTarget::Grpc {
            endpoint: format_host_port(&host, port),
            use_tls: false,
        });
    }

    let (host, port) = parse_host_port(trimmed, 443)?;
    Ok(OtlpTarget::Grpc {
        endpoint: format_host_port(&host, port),
        use_tls: true,
    })
}

fn parse_host_port(authority: &str, default_port: u16) -> Result<(String, u16), String> {
    if let Some(after_bracket) = authority.strip_prefix('[') {
        let close_pos = after_bracket
            .find(']')
            .ok_or_else(|| format!("unclosed IPv6 bracket in authority: {authority}"))?;
        let host = &after_bracket[..close_pos];
        let remainder = &after_bracket[close_pos + 1..];
        if remainder.is_empty() {
            return Ok((host.to_string(), default_port));
        }
        let port_str = remainder
            .strip_prefix(':')
            .ok_or_else(|| format!("invalid port separator in authority: {authority}"))?;
        let port = port_str
            .parse::<u16>()
            .map_err(|e| format!("invalid port '{port_str}' in {authority}: {e}"))?;
        return Ok((host.to_string(), port));
    }

    if let Some((host_part, port_part)) = authority.rsplit_once(':')
        && !host_part.contains(':')
    {
        let port = port_part
            .parse::<u16>()
            .map_err(|e| format!("invalid port '{port_part}' in {authority}: {e}"))?;
        return Ok((host_part.to_string(), port));
    }
    Ok((authority.to_string(), default_port))
}

fn format_host_port(host: &str, port: u16) -> String {
    if host.contains(':') {
        format!("[{host}]:{port}")
    } else {
        format!("{host}:{port}")
    }
}

/// Completed HTTP response returned by [`http_request`] or [`http_stream_lines`].
#[derive(Clone, Debug)]
pub struct HttpResponse {
    pub status: u16,
    pub body: Vec<u8>,
}

/// Compresses bytes using standard gzip framing.
pub fn gzip_bytes(input: &[u8]) -> Result<Vec<u8>, String> {
    let mut encoder = GzEncoder::new(
        Vec::with_capacity(input.len() / 2 + 32),
        Compression::fast(),
    );
    encoder
        .write_all(input)
        .map_err(|e| format!("gzip compression write failed: {e}"))?;
    encoder
        .finish()
        .map_err(|e| format!("gzip compression finish failed: {e}"))
}

/// Decompresses a gzipped buffer up to `max_bytes`.
pub fn gunzip_bytes(compressed: &[u8], max_bytes: usize) -> Result<Vec<u8>, String> {
    let decoder = GzDecoder::new(compressed);
    let mut limited = decoder.take((max_bytes as u64).saturating_add(1));
    let mut out = Vec::new();
    limited
        .read_to_end(&mut out)
        .map_err(|e| format!("gunzip decompression failed: {e}"))?;
    if out.len() > max_bytes {
        return Err(format!(
            "gunzip payload exceeded max_bytes limit ({max_bytes})"
        ));
    }
    Ok(out)
}

/// Executes a unary HTTP/HTTPS request, optionally gzip-compressing the request payload.
pub fn http_request(
    method: &str,
    url: &str,
    headers: &[(&str, &str)],
    body: &[u8],
    gzip_body: bool,
    ca_cert_path: Option<&str>,
    timeout: Duration,
) -> Result<HttpResponse, String> {
    let _ = parse_http_url(url)?;
    let client = http_client_for(ca_cert_path)?;
    let req_method = reqwest::Method::from_bytes(method.as_bytes())
        .map_err(|e| format!("invalid HTTP method '{method}': {e}"))?;

    let payload = if gzip_body {
        gzip_bytes(body)?
    } else {
        body.to_vec()
    };

    block_on_transport(async move {
        let mut req = client.request(req_method, url.trim()).timeout(timeout);
        for &(k, v) in headers {
            req = req.header(k, v);
        }
        if gzip_body {
            req = req.header("Content-Encoding", "gzip");
        }
        if !payload.is_empty() || matches!(method, "POST" | "PUT" | "PATCH") {
            req = req.body(payload);
        }

        let resp = req
            .send()
            .await
            .map_err(|e| format!("HTTP request failed for {url}: {e}"))?;
        let status = resp.status().as_u16();
        let bytes = resp
            .bytes()
            .await
            .map_err(|e| format!("HTTP response body read failed for {url}: {e}"))?;
        Ok(HttpResponse {
            status,
            body: bytes.to_vec(),
        })
    })
}

/// Streams newline-delimited HTTP/HTTPS chunks (e.g. Kubernetes `?watch=true`) without buffering
/// the entire stream. If the server returns a non-200 status (such as `410 Gone` or `422`),
/// returns `Ok(HttpResponse { status, body })` with the error payload.
pub fn http_stream_lines<F>(
    url: &str,
    headers: &[(&str, &str)],
    ca_cert_path: Option<&str>,
    timeout: Duration,
    stop_flag: &AtomicU8,
    mut on_line: F,
) -> Result<HttpResponse, String>
where
    F: FnMut(&[u8]) -> bool,
{
    let _ = parse_http_url(url)?;
    let client = http_client_for(ca_cert_path)?;

    block_on_transport(async {
        if stop_flag.load(Ordering::Acquire) != 0 {
            return Ok(HttpResponse {
                status: 200,
                body: Vec::new(),
            });
        }

        let deadline = tokio::time::Instant::now() + timeout;
        let mut req = client.get(url.trim());
        for &(k, v) in headers {
            req = req.header(k, v);
        }

        let send_fut = req.send();
        tokio::pin!(send_fut);

        let mut resp = loop {
            if stop_flag.load(Ordering::Acquire) != 0 {
                return Ok(HttpResponse {
                    status: 200,
                    body: Vec::new(),
                });
            }
            let now = tokio::time::Instant::now();
            if now >= deadline {
                return Err(format!("HTTP stream connect timeout for {url}"));
            }
            let tick = (deadline - now).min(Duration::from_millis(25));
            tokio::select! {
                res = &mut send_fut => {
                    break res.map_err(|e| format!("HTTP stream connect failed for {url}: {e}"))?;
                }
                _ = tokio::time::sleep(tick) => {}
            }
        };

        let status = resp.status().as_u16();
        if status != 200 {
            let mut err_body = Vec::new();
            while let Ok(Some(chunk)) = resp.chunk().await {
                let remaining = MAX_ERROR_BODY_BYTES.saturating_sub(err_body.len());
                if remaining == 0 {
                    break;
                }
                err_body.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
            }
            return Ok(HttpResponse {
                status,
                body: err_body,
            });
        }

        let mut buf: Vec<u8> = Vec::with_capacity(8192);
        loop {
            if stop_flag.load(Ordering::Acquire) != 0 {
                return Ok(HttpResponse {
                    status: 200,
                    body: Vec::new(),
                });
            }
            let now = tokio::time::Instant::now();
            if now >= deadline {
                return Ok(HttpResponse {
                    status: 200,
                    body: Vec::new(),
                });
            }
            let tick = (deadline - now).min(Duration::from_millis(25));

            let next_chunk = tokio::select! {
                c = resp.chunk() => c,
                _ = tokio::time::sleep(tick) => continue,
            };

            match next_chunk {
                Ok(Some(chunk)) => {
                    if buf.len().saturating_add(chunk.len()) > MAX_STREAM_LINE_BYTES {
                        return Err(format!(
                            "HTTP stream line exceeded {MAX_STREAM_LINE_BYTES} bytes without newline"
                        ));
                    }
                    buf.extend_from_slice(&chunk);
                    let mut start = 0usize;
                    while let Some(rel_nl) = buf[start..].iter().position(|&b| b == b'\n') {
                        let line_end = start + rel_nl;
                        let mut line = &buf[start..line_end];
                        if let Some(stripped) = line.strip_suffix(b"\r") {
                            line = stripped;
                        }
                        start = line_end + 1;
                        if !line.is_empty() && !on_line(line) {
                            return Ok(HttpResponse {
                                status: 200,
                                body: Vec::new(),
                            });
                        }
                    }
                    if start > 0 {
                        buf.drain(..start);
                    }
                }
                Ok(None) => {
                    let mut tail = buf.as_slice();
                    if let Some(stripped) = tail.strip_suffix(b"\r") {
                        tail = stripped;
                    }
                    if !tail.is_empty() {
                        let _ = on_line(tail);
                    }
                    return Ok(HttpResponse {
                        status: 200,
                        body: Vec::new(),
                    });
                }
                Err(err) => {
                    if stop_flag.load(Ordering::Acquire) != 0 {
                        return Ok(HttpResponse {
                            status: 200,
                            body: Vec::new(),
                        });
                    }
                    return Err(format!(
                        "HTTP stream chunk read failed for {url}: {e}",
                        e = err
                    ));
                }
            }
        }
    })
}

/// Result of an OTLP gRPC `LogsService/Export` call.
#[derive(Clone, Debug)]
pub struct GrpcExportResult {
    pub grpc_status: i32,
    pub rejected_log_records: usize,
    pub error_message: String,
}

/// Thread-safe pure-Rust gRPC client (`tonic` + `rustls`) for
/// `/opentelemetry.proto.collector.logs.v1.LogsService/Export`.
#[derive(Clone)]
pub struct OtlpGrpcClient {
    channel: Channel,
}

impl OtlpGrpcClient {
    pub fn new(endpoint: &str, use_tls: bool, ca_cert_path: Option<&str>) -> Result<Self, String> {
        let rt = shared_runtime();
        let _enter = rt.enter();

        let scheme = if use_tls { "https" } else { "http" };
        let uri = format!("{scheme}://{endpoint}");
        let mut ep = Endpoint::from_shared(uri.clone())
            .map_err(|e| format!("invalid OTLP gRPC endpoint '{uri}': {e}"))?
            .connect_timeout(Duration::from_secs(10))
            .tcp_keepalive(Some(Duration::from_secs(30)))
            .http2_keep_alive_interval(Duration::from_secs(30))
            .keep_alive_while_idle(true);

        if use_tls {
            let mut tls = ClientTlsConfig::new().with_enabled_roots();
            if let Some(path) = ca_cert_path {
                let pem = std::fs::read(path)
                    .map_err(|e| format!("failed to read OTLP CA certificate '{path}': {e}"))?;
                tls = tls.ca_certificate(Certificate::from_pem(pem));
            }
            ep = ep
                .tls_config(tls)
                .map_err(|e| format!("failed to configure OTLP gRPC TLS for '{uri}': {e}"))?;
        }

        Ok(Self {
            channel: ep.connect_lazy(),
        })
    }

    pub fn export(
        &self,
        request: ExportLogsServiceRequest,
        bearer_token: Option<&str>,
        quota_project: Option<&str>,
        timeout: Duration,
    ) -> GrpcExportResult {
        let channel = self.channel.clone();
        block_on_transport(async move {
            let mut grpc = Grpc::new(channel);
            if let Err(err) = grpc.ready().await {
                return GrpcExportResult {
                    grpc_status: 14, // UNAVAILABLE
                    rejected_log_records: 0,
                    error_message: format!("OTLP gRPC channel not ready: {err}"),
                };
            }

            let mut tonic_req = tonic::Request::new(request);
            tonic_req.set_timeout(timeout);

            if let Some(tok) = bearer_token {
                let header_val = format!("Bearer {tok}");
                match MetadataValue::try_from(header_val.as_str()) {
                    Ok(val) => {
                        tonic_req.metadata_mut().insert("authorization", val);
                    }
                    Err(err) => {
                        return GrpcExportResult {
                            grpc_status: 16, // UNAUTHENTICATED
                            rejected_log_records: 0,
                            error_message: format!("invalid OTLP bearer token metadata: {err}"),
                        };
                    }
                }
            }

            if let Some(proj) = quota_project
                && !proj.is_empty()
                && let Ok(val) = MetadataValue::try_from(proj)
            {
                tonic_req.metadata_mut().insert("x-goog-user-project", val);
            }

            let path = PathAndQuery::from_static(
                "/opentelemetry.proto.collector.logs.v1.LogsService/Export",
            );
            let codec: ProstCodec<ExportLogsServiceRequest, ExportLogsServiceResponse> =
                ProstCodec::default();

            match grpc.unary(tonic_req, path, codec).await {
                Ok(resp) => {
                    let rejected = resp
                        .into_inner()
                        .partial_success
                        .map(|ps| ps.rejected_log_records.max(0) as usize)
                        .unwrap_or(0);
                    GrpcExportResult {
                        grpc_status: 0,
                        rejected_log_records: rejected,
                        error_message: String::new(),
                    }
                }
                Err(status) => GrpcExportResult {
                    grpc_status: status.code() as i32,
                    rejected_log_records: 0,
                    error_message: status.message().to_string(),
                },
            }
        })
    }
}
