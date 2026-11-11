//! Byte-bounded micro-batching queue (`16 MiB` hard cap) and concurrent sink export worker pool.

use crate::enricher::EnrichedEntry;
use crate::metrics::MetricsRegistry;
use crate::sink::{BatchExportOutcome, EventSink};
use log::warn;
use std::collections::VecDeque;
use std::sync::atomic::{AtomicU8, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

struct QueueState {
    entries: VecDeque<EnrichedEntry>,
    queued_bytes: usize,
    closed: bool,
}

/// Thread-safe bounded queue that caps in-flight Event bytes and propagates backpressure to the
/// Kubernetes Watch reader instead of growing unboundedly during sink outages.
pub struct BoundedBatchQueue {
    capacity_bytes: usize,
    metrics: Arc<MetricsRegistry>,
    state: Mutex<QueueState>,
    not_empty: Condvar,
    not_full: Condvar,
}

impl BoundedBatchQueue {
    pub fn new(capacity_bytes: usize, metrics: Arc<MetricsRegistry>) -> Self {
        Self {
            capacity_bytes: capacity_bytes.max(4096),
            metrics,
            state: Mutex::new(QueueState {
                entries: VecDeque::with_capacity(512),
                queued_bytes: 0,
                closed: false,
            }),
            not_empty: Condvar::new(),
            not_full: Condvar::new(),
        }
    }

    /// Enqueues an enriched entry, blocking when `queued_bytes` reaches `capacity_bytes`.
    /// Returns `false` if the queue is closed or `stop_flag` is signaled.
    pub fn push(&self, entry: EnrichedEntry, stop_flag: &AtomicU8) -> bool {
        let cost = entry.estimated_bytes.max(64);
        let mut guard = self.state.lock().unwrap_or_else(|e| e.into_inner());
        while !guard.closed
            && !guard.entries.is_empty()
            && guard.queued_bytes.saturating_add(cost) > self.capacity_bytes
        {
            if stop_flag.load(Ordering::Acquire) != 0 {
                return false;
            }
            let (next_guard, _) = self
                .not_full
                .wait_timeout(guard, Duration::from_millis(50))
                .unwrap_or_else(|e| e.into_inner());
            guard = next_guard;
        }
        if guard.closed {
            return false;
        }
        guard.queued_bytes = guard.queued_bytes.saturating_add(cost);
        guard.entries.push_back(entry);
        self.metrics.add_received_entries(1);
        self.not_empty.notify_one();
        true
    }

    /// Pops up to `max_entries` / `max_bytes`, waiting up to `flush_delay` to coalesce
    /// micro-batches. Returns `None` once the queue is closed and completely drained.
    pub fn pop_batch(
        &self,
        max_entries: usize,
        max_bytes: usize,
        flush_delay: Duration,
    ) -> Option<Vec<EnrichedEntry>> {
        let entry_cap = max_entries.max(1);
        let byte_cap = max_bytes.max(4096);

        let mut guard = self.state.lock().unwrap_or_else(|e| e.into_inner());
        while guard.entries.is_empty() && !guard.closed {
            guard = self
                .not_empty
                .wait(guard)
                .unwrap_or_else(|e| e.into_inner());
        }
        if guard.entries.is_empty() && guard.closed {
            return None;
        }

        if !flush_delay.is_zero() && !guard.closed {
            let deadline = Instant::now() + flush_delay;
            while !guard.closed && guard.entries.len() < entry_cap && guard.queued_bytes < byte_cap
            {
                let now = Instant::now();
                if now >= deadline {
                    break;
                }
                let (next_guard, _) = self
                    .not_empty
                    .wait_timeout(guard, deadline - now)
                    .unwrap_or_else(|e| e.into_inner());
                guard = next_guard;
            }
        }

        let mut batch = Vec::with_capacity(guard.entries.len().min(entry_cap));
        let mut batch_bytes = 0usize;
        while let Some(front) = guard.entries.front() {
            let cost = front.estimated_bytes.max(64);
            if !batch.is_empty()
                && (batch.len() >= entry_cap || batch_bytes.saturating_add(cost) > byte_cap)
            {
                break;
            }
            let item = guard.entries.pop_front().unwrap();
            guard.queued_bytes = guard.queued_bytes.saturating_sub(cost);
            batch_bytes = batch_bytes.saturating_add(cost);
            batch.push(item);
        }

        self.not_full.notify_all();
        if !guard.entries.is_empty() {
            self.not_empty.notify_one();
        }
        Some(batch)
    }

    /// Closes the queue for new producers while allowing consumers to drain remaining entries.
    pub fn close(&self) {
        let mut guard = self.state.lock().unwrap_or_else(|e| e.into_inner());
        guard.closed = true;
        self.not_empty.notify_all();
        self.not_full.notify_all();
    }
}

/// Spawns `worker_count` sink export threads that drain [`BoundedBatchQueue`] with bounded retry
/// backoff.
#[allow(clippy::too_many_arguments)]
pub fn spawn_export_workers(
    worker_count: usize,
    max_batch_entries: usize,
    max_batch_bytes: usize,
    flush_delay: Duration,
    queue: Arc<BoundedBatchQueue>,
    sink: Arc<dyn EventSink>,
    metrics: Arc<MetricsRegistry>,
    stop_flag: Arc<AtomicU8>,
) -> Vec<JoinHandle<()>> {
    let count = worker_count.max(1);
    let mut handles = Vec::with_capacity(count);
    for idx in 0..count {
        let q = Arc::clone(&queue);
        let s = Arc::clone(&sink);
        let m = Arc::clone(&metrics);
        let stop = Arc::clone(&stop_flag);
        let handle = std::thread::Builder::new()
            .name(format!("ee-sink-{idx}"))
            .spawn(move || {
                while let Some(batch) = q.pop_batch(max_batch_entries, max_batch_bytes, flush_delay)
                {
                    export_with_retries(&batch, s.as_ref(), &m, &stop);
                }
            })
            .expect("failed to spawn sink worker thread");
        handles.push(handle);
    }
    handles
}

fn export_with_retries(
    batch: &[EnrichedEntry],
    sink: &dyn EventSink,
    metrics: &MetricsRegistry,
    stop_flag: &AtomicU8,
) {
    const MAX_ATTEMPTS: usize = 6;
    let mut backoff = Duration::from_millis(200);

    for attempt in 1..=MAX_ATTEMPTS {
        match sink.export_batch(batch, metrics) {
            BatchExportOutcome::Success { .. } => return,
            BatchExportOutcome::PermanentFailure {
                status_code,
                reason,
            } => {
                warn!(
                    "Dropping batch of {} entries on non-retryable status {status_code}: {reason}",
                    batch.len()
                );
                return;
            }
            BatchExportOutcome::RetryableFailure {
                status_code,
                reason,
            } => {
                if attempt == MAX_ATTEMPTS || stop_flag.load(Ordering::Acquire) != 0 {
                    warn!(
                        "Giving up on batch of {} entries after {attempt} attempts (last status {status_code}: {reason})",
                        batch.len()
                    );
                    return;
                }
                warn!(
                    "Retrying batch of {} entries after status {status_code} (attempt {attempt}/{MAX_ATTEMPTS})",
                    batch.len()
                );
                std::thread::sleep(backoff);
                backoff = (backoff * 2).min(Duration::from_secs(10));
            }
        }
    }
}
