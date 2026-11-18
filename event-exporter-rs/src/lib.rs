//! High-scale, bounded-memory GKE Kubernetes Event Exporter (100% safe pure Rust).

#![forbid(unsafe_code)]

use crate::enricher::format_unix_nanos_rfc3339;
use log::{LevelFilter, Metadata, Record};
use std::io::Write;
use std::sync::OnceLock;
use std::time::{SystemTime, UNIX_EPOCH};

pub mod auth;
pub mod cli;
pub mod enricher;
pub mod metrics;
pub mod otlp_proto;
pub mod pipeline;
pub mod pod_cache;
pub mod shard;
pub mod sink;
pub mod transport;

struct StderrLogger;

impl log::Log for StderrLogger {
    fn enabled(&self, metadata: &Metadata<'_>) -> bool {
        metadata.level() <= log::Level::Info
    }

    fn log(&self, record: &Record<'_>) {
        if !self.enabled(record.metadata()) {
            return;
        }
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_nanos() as u64)
            .unwrap_or(0);
        let ts = format_unix_nanos_rfc3339(nanos);
        let mut stderr = std::io::stderr().lock();
        let _ = writeln!(stderr, "{ts} [{}] {}", record.level(), record.args());
    }

    fn flush(&self) {
        let _ = std::io::stderr().flush();
    }
}

static LOGGER: StderrLogger = StderrLogger;

/// Initializes the lightweight stderr logger once per process.
pub fn init_logging() {
    static INIT: OnceLock<()> = OnceLock::new();
    INIT.get_or_init(|| {
        if log::set_logger(&LOGGER).is_ok() {
            log::set_max_level(LevelFilter::Info);
        }
    });
}
