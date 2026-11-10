//! Entrypoint for GKE `event-exporter` (100% pure-Rust rewrite).

#![forbid(unsafe_code)]

use event_exporter_rs::auth::{
    CloudTokenProvider, KubeTokenSource, resolve_cluster_coordinates, resolve_kube_api_base,
    resolve_kube_ca_path, resolve_logging_endpoint, resolve_otlp_endpoint,
};
use event_exporter_rs::cli::{ExporterConfig, SinkKind};
use event_exporter_rs::enricher::build_startup_marker_entry;
use event_exporter_rs::init_logging;
use event_exporter_rs::k8s_stream::{
    InitialSyncLatch, KubeClientConfig, run_event_stream_loop, run_pod_reflector_loop,
};
use event_exporter_rs::metrics::{MetricsRegistry, spawn_metrics_server};
use event_exporter_rs::pipeline::{BoundedBatchQueue, spawn_export_workers};
use event_exporter_rs::pod_cache::PodOwnerTable;
use event_exporter_rs::shard::ShardSpec;
use event_exporter_rs::sink::{CloudLoggingSink, EventSink, OtlpSink};
use event_exporter_rs::transport::install_shutdown_signal_listener;
use log::{error, info, warn};
use std::sync::Arc;
use std::sync::atomic::{AtomicU8, Ordering};
use std::time::{Duration, SystemTime};

fn main() {
    init_logging();

    let args: Vec<String> = std::env::args().skip(1).collect();
    let cfg = match ExporterConfig::from_args(&args) {
        Ok(c) => c,
        Err(err) => {
            error!("Failed to parse CLI flags: {err}");
            std::process::exit(2);
        }
    };

    let pod_name_env = std::env::var("POD_NAME").ok();
    let shard = match ShardSpec::resolve(
        cfg.shard_mode,
        cfg.shard_count,
        cfg.shard_index,
        pod_name_env.as_deref(),
    ) {
        Ok(s) => s,
        Err(err) => {
            error!("Invalid shard configuration: {err}");
            std::process::exit(2);
        }
    };

    let coords = match resolve_cluster_coordinates(&cfg, None) {
        Ok(c) => c,
        Err(err) => {
            error!("Failed to resolve GKE cluster coordinates: {err}");
            std::process::exit(1);
        }
    };

    let kube_api_base = match resolve_kube_api_base(&cfg) {
        Ok(u) => u,
        Err(err) => {
            error!("Failed to resolve Kubernetes API endpoint: {err}");
            std::process::exit(1);
        }
    };
    let kube_ca_path = resolve_kube_ca_path(&cfg, &kube_api_base);

    let stop_flag = Arc::new(AtomicU8::new(0));
    if let Err(err) = install_shutdown_signal_listener(Arc::clone(&stop_flag)) {
        warn!("Signal listener setup warning: {err}");
    }

    let metrics = Arc::new(MetricsRegistry::new());
    let (metrics_port, metrics_handle) = match spawn_metrics_server(
        &cfg.prometheus_endpoint,
        Arc::clone(&metrics),
        Arc::clone(&stop_flag),
    ) {
        Ok(pair) => pair,
        Err(err) => {
            error!("{err}");
            std::process::exit(1);
        }
    };
    info!("Prometheus server listening on port {metrics_port}");

    let token_provider = Arc::new(CloudTokenProvider::from_config(&cfg, None));
    let sink: Arc<dyn EventSink> = match cfg.sink_kind {
        SinkKind::CloudLogging => {
            let endpoint = resolve_logging_endpoint(&cfg);
            info!(
                "Starting GKE event-exporter (sink=CloudLogging, endpoint={endpoint}, shard={}/{}, mode={:?})",
                shard.shard_index, shard.shard_count, shard.mode
            );
            Arc::new(CloudLoggingSink::new(
                &endpoint,
                coords,
                token_provider,
                None,
            ))
        }
        SinkKind::Otlp => {
            let endpoint = resolve_otlp_endpoint(&cfg);
            info!(
                "Starting GKE event-exporter (sink=OTLP, endpoint={endpoint}, shard={}/{}, mode={:?})",
                shard.shard_index, shard.shard_count, shard.mode
            );
            match OtlpSink::new(&endpoint, coords, token_provider, None) {
                Ok(otlp) => Arc::new(otlp),
                Err(err) => {
                    error!("Failed to initialize OTLP sink: {err}");
                    std::process::exit(1);
                }
            }
        }
    };

    let queue = Arc::new(BoundedBatchQueue::new(
        cfg.queue_capacity_bytes,
        Arc::clone(&metrics),
    ));
    let worker_handles = spawn_export_workers(
        cfg.export_workers,
        cfg.max_batch_entries,
        cfg.max_batch_bytes,
        cfg.flush_delay,
        Arc::clone(&queue),
        Arc::clone(&sink),
        Arc::clone(&metrics),
        Arc::clone(&stop_flag),
    );

    if let Some(startup_entry) = build_startup_marker_entry(&shard, SystemTime::now()) {
        let _ = queue.push(startup_entry, &stop_flag);
    }

    let kube_cfg = KubeClientConfig {
        api_base: kube_api_base,
        ca_cert_path: kube_ca_path,
        token_source: Arc::new(KubeTokenSource::new(cfg.kube_bearer_token_path.clone())),
        shard,
        enable_watch_list: cfg.enable_watch_list,
        list_page_size: cfg.list_page_size,
        event_label_selector: cfg.event_label_selector.clone(),
    };

    let (pod_table, pod_handle) = if cfg.watch_pods {
        let table = Arc::new(PodOwnerTable::new(cfg.system_namespaces.clone()));
        let sync_latch = Arc::new(InitialSyncLatch::new());
        let pod_kube = kube_cfg.clone();
        let pod_table_clone = Arc::clone(&table);
        let pod_metrics = Arc::clone(&metrics);
        let pod_latch = Arc::clone(&sync_latch);
        let pod_stop = Arc::clone(&stop_flag);
        let handle = std::thread::Builder::new()
            .name("ee-pod-watch".to_string())
            .spawn(move || {
                run_pod_reflector_loop(pod_kube, pod_table_clone, pod_metrics, pod_latch, pod_stop);
            })
            .expect("failed to spawn Pod reflector thread");

        if !sync_latch.wait_timeout(Duration::from_secs(30), &stop_flag) {
            warn!(
                "Proceeding to Event watch before initial Pod sync finished (30s timeout or shutdown)"
            );
        }
        (Some(table), Some(handle))
    } else {
        (None, None)
    };

    run_event_stream_loop(
        kube_cfg,
        pod_table,
        Arc::clone(&queue),
        Arc::clone(&metrics),
        Arc::clone(&stop_flag),
    );

    stop_flag.store(1, Ordering::Release);
    queue.close();
    for handle in worker_handles {
        let _ = handle.join();
    }
    if let Some(h) = pod_handle {
        let _ = h.join();
    }
    let _ = metrics_handle.join();
    info!("GKE event-exporter shut down cleanly");
}
