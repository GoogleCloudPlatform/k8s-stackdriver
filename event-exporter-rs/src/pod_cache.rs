//! Bounded-memory `PodOwnerTable` (~28–32 B/pod, zero per-Pod heap allocations) and streaming
//! `PartialObjectMetadata` Pod JSON deserializer.

use crate::metrics::MetricsRegistry;
use serde::Deserialize;
use serde::de::{Deserializer, IgnoredAny, MapAccess, Visitor};
use std::collections::hash_map::RandomState;
use std::collections::{HashMap, HashSet};
use std::fmt;
use std::hash::{BuildHasher, Hash, Hasher};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, RwLock};

/// Top-level Kubernetes workload controller types recognized by GKE Cloud Logging & Monarch.
#[repr(u8)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ControllerKind {
    Deployment = 1,
    DaemonSet = 2,
    StatefulSet = 3,
    CronJob = 4,
    JobSet = 5,
    Job = 6,
}

impl ControllerKind {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Deployment => "Deployment",
            Self::DaemonSet => "DaemonSet",
            Self::StatefulSet => "StatefulSet",
            Self::CronJob => "CronJob",
            Self::JobSet => "JobSet",
            Self::Job => "Job",
        }
    }
}

/// Resolved workload owner and optional JobSet metadata attached to `k8s_pod` Event LogEntries.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ResolvedPodOwner {
    pub kind: ControllerKind,
    pub name: String,
    pub jobset_restart_attempt: Option<String>,
    pub jobset_uid: Option<String>,
}

/// 128-bit double-SipHash key with 8-byte alignment so `(PodKey, CompactOwner)` packs into 24 bytes
/// without `u128`'s 16-byte alignment padding.
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct PodKey([u64; 2]);

impl Hash for PodKey {
    fn hash<H: Hasher>(&self, state: &mut H) {
        state.write_u64(self.0[0]);
    }
}

#[derive(Default)]
struct IdentityBuildHasher;

#[derive(Default)]
struct IdentityHasher(u64);

impl BuildHasher for IdentityBuildHasher {
    type Hasher = IdentityHasher;

    fn build_hasher(&self) -> Self::Hasher {
        IdentityHasher(0)
    }
}

impl Hasher for IdentityHasher {
    fn finish(&self) -> u64 {
        self.0
    }

    fn write(&mut self, bytes: &[u8]) {
        for &b in bytes {
            self.0 = self.0.rotate_left(8) ^ u64::from(b);
        }
    }

    fn write_u64(&mut self, val: u64) {
        self.0 = val;
    }
}

const NUM_SHARDS: usize = 64;
const JOBSET_OVERFLOW_SENTINEL: u16 = u16::MAX;

/// 8-byte value stored inline per Pod in `PodOwnerTable`.
#[repr(C)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CompactOwner {
    kind: ControllerKind,
    _pad: u8,
    jobset_extra_id: u16,
    controller_name_id: u32,
}

const _: () = assert!(std::mem::size_of::<(PodKey, CompactOwner)>() == 24);

#[derive(Clone, Debug, PartialEq, Eq, Hash)]
struct JobSetExtraIds {
    restart_attempt_id: u32,
    jobset_uid_id: u32,
}

#[derive(Default)]
struct StringInterner {
    by_str: HashMap<Arc<str>, u32>,
    values: Vec<Arc<str>>,
}

impl StringInterner {
    fn new() -> Self {
        let mut values = Vec::with_capacity(64);
        values.push(Arc::<str>::from(""));
        Self {
            by_str: HashMap::with_capacity(64),
            values,
        }
    }

    fn intern(&mut self, value: &str) -> u32 {
        if value.is_empty() {
            return 0;
        }
        if let Some(&existing) = self.by_str.get(value) {
            return existing;
        }
        let id = u32::try_from(self.values.len()).unwrap_or(0);
        if id == 0 {
            return 0;
        }
        let shared: Arc<str> = Arc::from(value);
        self.values.push(Arc::clone(&shared));
        self.by_str.insert(shared, id);
        id
    }

    fn resolve(&self, id: u32) -> &str {
        self.values
            .get(id as usize)
            .map(AsRef::as_ref)
            .unwrap_or("")
    }
}

#[derive(Default)]
struct PodShard {
    entries: HashMap<PodKey, CompactOwner, IdentityBuildHasher>,
    jobset_overflow: HashMap<PodKey, JobSetExtraIds, IdentityBuildHasher>,
}

impl PodShard {
    fn new() -> Self {
        Self {
            entries: HashMap::with_hasher(IdentityBuildHasher),
            jobset_overflow: HashMap::with_hasher(IdentityBuildHasher),
        }
    }
}

struct OwnerPool {
    interner: StringInterner,
    jobset_extras: Vec<JobSetExtraIds>,
    jobset_extra_dedup: HashMap<JobSetExtraIds, u16>,
    max_inline_jobsets: usize,
}

impl OwnerPool {
    fn new(max_inline_jobsets: usize) -> Self {
        let mut jobset_extras = Vec::with_capacity(8);
        jobset_extras.push(JobSetExtraIds {
            restart_attempt_id: 0,
            jobset_uid_id: 0,
        });
        Self {
            interner: StringInterner::new(),
            jobset_extras,
            jobset_extra_dedup: HashMap::new(),
            max_inline_jobsets: max_inline_jobsets.min((JOBSET_OVERFLOW_SENTINEL as usize) - 1),
        }
    }

    fn try_encode_existing(&self, owner: &ResolvedPodOwner) -> Option<CompactOwner> {
        let controller_name_id = if owner.name.is_empty() {
            0
        } else {
            *self.interner.by_str.get(owner.name.as_str())?
        };
        let restart_attempt_id = match owner.jobset_restart_attempt.as_deref() {
            None | Some("") => 0,
            Some(s) => *self.interner.by_str.get(s)?,
        };
        let jobset_uid_id = match owner.jobset_uid.as_deref() {
            None | Some("") => 0,
            Some(s) => *self.interner.by_str.get(s)?,
        };
        let jobset_extra_id = if restart_attempt_id == 0 && jobset_uid_id == 0 {
            0
        } else {
            let pair = JobSetExtraIds {
                restart_attempt_id,
                jobset_uid_id,
            };
            *self.jobset_extra_dedup.get(&pair)?
        };
        Some(CompactOwner {
            kind: owner.kind,
            _pad: 0,
            jobset_extra_id,
            controller_name_id,
        })
    }

    fn encode_mut(&mut self, owner: &ResolvedPodOwner) -> (CompactOwner, Option<JobSetExtraIds>) {
        let controller_name_id = self.interner.intern(&owner.name);
        let restart_attempt_id = owner
            .jobset_restart_attempt
            .as_deref()
            .map(|s| self.interner.intern(s))
            .unwrap_or(0);
        let jobset_uid_id = owner
            .jobset_uid
            .as_deref()
            .map(|s| self.interner.intern(s))
            .unwrap_or(0);

        let (jobset_extra_id, overflow) = if restart_attempt_id == 0 && jobset_uid_id == 0 {
            (0, None)
        } else {
            let pair = JobSetExtraIds {
                restart_attempt_id,
                jobset_uid_id,
            };
            if let Some(&existing_id) = self.jobset_extra_dedup.get(&pair) {
                (existing_id, None)
            } else if self.jobset_extras.len() <= self.max_inline_jobsets {
                let new_id = self.jobset_extras.len() as u16;
                self.jobset_extras.push(pair.clone());
                self.jobset_extra_dedup.insert(pair, new_id);
                (new_id, None)
            } else {
                (JOBSET_OVERFLOW_SENTINEL, Some(pair))
            }
        };

        (
            CompactOwner {
                kind: owner.kind,
                _pad: 0,
                jobset_extra_id,
                controller_name_id,
            },
            overflow,
        )
    }
}

/// Thread-safe, lock-sharded, zero-per-Pod-allocation workload owner lookup table.
pub struct PodOwnerTable {
    hasher_hi: RandomState,
    hasher_lo: RandomState,
    system_namespaces: HashSet<String>,
    // Lock order: always acquire `pool` (read or write) before any `shards[i]`.
    pool: RwLock<OwnerPool>,
    shards: [RwLock<PodShard>; NUM_SHARDS],
    live_pods: AtomicUsize,
    interned_count: AtomicUsize,
    dead_jobsets: AtomicUsize,
}

impl PodOwnerTable {
    pub fn new(system_namespaces: HashSet<String>) -> Self {
        Self::with_max_inline_jobsets(system_namespaces, (JOBSET_OVERFLOW_SENTINEL as usize) - 1)
    }

    pub fn with_max_inline_jobsets(
        system_namespaces: HashSet<String>,
        max_inline_jobsets: usize,
    ) -> Self {
        Self {
            hasher_hi: RandomState::new(),
            hasher_lo: RandomState::new(),
            system_namespaces,
            pool: RwLock::new(OwnerPool::new(max_inline_jobsets)),
            shards: std::array::from_fn(|_| RwLock::new(PodShard::new())),
            live_pods: AtomicUsize::new(0),
            interned_count: AtomicUsize::new(1),
            dead_jobsets: AtomicUsize::new(0),
        }
    }

    pub fn compute_key(&self, namespace: &str, pod_name: &str) -> PodKey {
        let hi = hash_pair(&self.hasher_hi, namespace, pod_name);
        let lo = hash_pair(&self.hasher_lo, namespace, pod_name);
        PodKey([hi, lo])
    }

    fn shard_index(key: PodKey) -> usize {
        // `PodKey::hash` feeds `key.0[0]` to `hashbrown`; select shards from `key.0[1]` so each
        // shard's internal SwissTable retains full 64-bit hash entropy.
        (key.0[1] >> 58) as usize
    }

    fn str_limit(&self) -> usize {
        self.live_pods
            .load(Ordering::Relaxed)
            .saturating_mul(4)
            .max(4096)
    }

    /// Upserts or removes a Pod's top-level workload owner from parsed `PartialObjectMetadata`.
    pub fn upsert_pod_metadata(&self, pod: &PodMetadataExtract, metrics: Option<&MetricsRegistry>) {
        if pod.namespace.is_empty()
            || pod.name.is_empty()
            || self.system_namespaces.contains(&pod.namespace)
        {
            return;
        }
        let key = self.compute_key(&pod.namespace, &pod.name);
        let Some(owner) = infer_top_level_owner(pod) else {
            self.remove_by_key(key, metrics);
            return;
        };

        let shard_idx = Self::shard_index(key);

        let pool_read = self.pool.read().unwrap_or_else(|e| e.into_inner());
        if let Some(compact) = pool_read.try_encode_existing(&owner) {
            let mut shard = self.shards[shard_idx]
                .write()
                .unwrap_or_else(|e| e.into_inner());
            let is_new = self.write_shard_entry(&mut shard, key, compact, None);
            drop(shard);
            drop(pool_read);
            if is_new && let Some(m) = metrics {
                m.inc_pod_cache_op("add");
            }
            return;
        }
        drop(pool_read);

        let mut pool_write = self.pool.write().unwrap_or_else(|e| e.into_inner());
        if (owner.jobset_restart_attempt.is_some() || owner.jobset_uid.is_some())
            && pool_write.jobset_extras.len() > pool_write.max_inline_jobsets
            && self.dead_jobsets.load(Ordering::Relaxed) > 0
        {
            self.compact_locked(&mut pool_write);
        }
        let (compact, overflow) = pool_write.encode_mut(&owner);
        self.interned_count
            .store(pool_write.interner.values.len(), Ordering::Relaxed);
        let mut shard = self.shards[shard_idx]
            .write()
            .unwrap_or_else(|e| e.into_inner());
        let is_new = self.write_shard_entry(&mut shard, key, compact, overflow);
        drop(shard);
        if pool_write.interner.values.len() > self.str_limit() {
            self.compact_locked(&mut pool_write);
        }
        drop(pool_write);

        if is_new && let Some(m) = metrics {
            m.inc_pod_cache_op("add");
        }
    }

    fn write_shard_entry(
        &self,
        shard: &mut PodShard,
        key: PodKey,
        compact: CompactOwner,
        overflow: Option<JobSetExtraIds>,
    ) -> bool {
        let prev = shard.entries.insert(key, compact);
        match prev {
            None => {
                self.live_pods.fetch_add(1, Ordering::Relaxed);
            }
            Some(old) => {
                if old.jobset_extra_id == JOBSET_OVERFLOW_SENTINEL && overflow.is_none() {
                    shard.jobset_overflow.remove(&key);
                }
                if old.jobset_extra_id != 0 && old.jobset_extra_id != compact.jobset_extra_id {
                    self.dead_jobsets.fetch_add(1, Ordering::Relaxed);
                }
            }
        }
        if let Some(pair) = overflow {
            shard.jobset_overflow.insert(key, pair);
        }
        prev.is_none()
    }

    /// Removes a deleted Pod from the lookup table.
    pub fn remove_pod(&self, namespace: &str, pod_name: &str, metrics: Option<&MetricsRegistry>) {
        if namespace.is_empty() || pod_name.is_empty() {
            return;
        }
        let key = self.compute_key(namespace, pod_name);
        self.remove_by_key(key, metrics);
    }

    fn remove_by_key(&self, key: PodKey, metrics: Option<&MetricsRegistry>) {
        let shard_idx = Self::shard_index(key);
        let pool_read = self.pool.read().unwrap_or_else(|e| e.into_inner());
        let mut shard = self.shards[shard_idx]
            .write()
            .unwrap_or_else(|e| e.into_inner());
        let removed = shard.entries.remove(&key);
        if let Some(prev) = removed {
            self.live_pods.fetch_sub(1, Ordering::Relaxed);
            if prev.jobset_extra_id == JOBSET_OVERFLOW_SENTINEL {
                shard.jobset_overflow.remove(&key);
            }
            if prev.jobset_extra_id != 0 {
                self.dead_jobsets.fetch_add(1, Ordering::Relaxed);
            }
        }
        drop(shard);
        drop(pool_read);

        if removed.is_some() {
            if self.interned_count.load(Ordering::Relaxed) > self.str_limit() {
                let mut pool_write = self.pool.write().unwrap_or_else(|e| e.into_inner());
                if pool_write.interner.values.len() > self.str_limit() {
                    self.compact_locked(&mut pool_write);
                }
            }
            if let Some(m) = metrics {
                m.inc_pod_cache_op("evict");
            }
        }
    }

    /// Evicts any cached Pods whose keys are not in `live_keys` after a full re-list recovery.
    pub fn retain_only(&self, live_keys: &HashSet<PodKey>, metrics: Option<&MetricsRegistry>) {
        let mut evicted = 0usize;
        let mut pool_write = self.pool.write().unwrap_or_else(|e| e.into_inner());
        for shard_lock in &self.shards {
            let mut shard = shard_lock.write().unwrap_or_else(|e| e.into_inner());
            let PodShard {
                entries,
                jobset_overflow,
            } = &mut *shard;
            entries.retain(|k, v| {
                let keep = live_keys.contains(k);
                if !keep {
                    evicted += 1;
                    if v.jobset_extra_id == JOBSET_OVERFLOW_SENTINEL {
                        jobset_overflow.remove(k);
                    }
                }
                keep
            });
        }
        if evicted > 0 {
            self.live_pods.fetch_sub(evicted, Ordering::Relaxed);
            self.compact_locked(&mut pool_write);
        }
        drop(pool_write);
        if evicted > 0
            && let Some(m) = metrics
        {
            m.add_pod_cache_op("evict", evicted as u64);
        }
    }

    fn compact_locked(&self, pool: &mut OwnerPool) {
        let old_interner = std::mem::replace(&mut pool.interner, StringInterner::new());
        let mut fresh_extras = Vec::with_capacity(8);
        fresh_extras.push(JobSetExtraIds {
            restart_attempt_id: 0,
            jobset_uid_id: 0,
        });
        let old_extras = std::mem::replace(&mut pool.jobset_extras, fresh_extras);
        pool.jobset_extra_dedup.clear();
        self.dead_jobsets.store(0, Ordering::Relaxed);

        let mut total_live = 0usize;
        for shard_lock in &self.shards {
            let mut shard = shard_lock.write().unwrap_or_else(|e| e.into_inner());
            total_live += shard.entries.len();
            let mut old_overflow = std::mem::replace(
                &mut shard.jobset_overflow,
                HashMap::with_hasher(IdentityBuildHasher),
            );
            let PodShard {
                entries,
                jobset_overflow,
            } = &mut *shard;

            for (&key, compact) in entries.iter_mut() {
                compact.controller_name_id = pool
                    .interner
                    .intern(old_interner.resolve(compact.controller_name_id));
                if compact.jobset_extra_id == 0 {
                    continue;
                }
                let old_pair = if compact.jobset_extra_id == JOBSET_OVERFLOW_SENTINEL {
                    old_overflow.remove(&key)
                } else {
                    old_extras.get(compact.jobset_extra_id as usize).cloned()
                };
                let Some(ids) = old_pair else {
                    compact.jobset_extra_id = 0;
                    continue;
                };
                let restart_attempt_id = pool
                    .interner
                    .intern(old_interner.resolve(ids.restart_attempt_id));
                let jobset_uid_id = pool
                    .interner
                    .intern(old_interner.resolve(ids.jobset_uid_id));
                if restart_attempt_id == 0 && jobset_uid_id == 0 {
                    compact.jobset_extra_id = 0;
                    continue;
                }
                let pair = JobSetExtraIds {
                    restart_attempt_id,
                    jobset_uid_id,
                };
                compact.jobset_extra_id =
                    if let Some(&existing_id) = pool.jobset_extra_dedup.get(&pair) {
                        existing_id
                    } else if pool.jobset_extras.len() <= pool.max_inline_jobsets {
                        let new_id = pool.jobset_extras.len() as u16;
                        pool.jobset_extras.push(pair.clone());
                        pool.jobset_extra_dedup.insert(pair, new_id);
                        new_id
                    } else {
                        jobset_overflow.insert(key, pair);
                        JOBSET_OVERFLOW_SENTINEL
                    };
            }
        }
        self.live_pods.store(total_live, Ordering::Relaxed);
        self.interned_count
            .store(pool.interner.values.len(), Ordering::Relaxed);
    }

    /// Looks up a Pod's top-level workload controller in O(1) time under shared read locks.
    pub fn lookup(
        &self,
        namespace: &str,
        pod_name: &str,
        metrics: Option<&MetricsRegistry>,
    ) -> Option<ResolvedPodOwner> {
        if namespace.is_empty() || pod_name.is_empty() || self.system_namespaces.contains(namespace)
        {
            return None;
        }
        let key = self.compute_key(namespace, pod_name);
        let shard_idx = Self::shard_index(key);
        let pool = self.pool.read().unwrap_or_else(|e| e.into_inner());
        let shard = self.shards[shard_idx]
            .read()
            .unwrap_or_else(|e| e.into_inner());
        let Some(&compact) = shard.entries.get(&key) else {
            drop(shard);
            drop(pool);
            if let Some(m) = metrics {
                m.inc_pod_cache_op("querymiss");
            }
            return None;
        };

        let controller_name = pool
            .interner
            .resolve(compact.controller_name_id)
            .to_string();
        let (restart_attempt, jobset_uid) = if compact.jobset_extra_id == 0 {
            (None, None)
        } else {
            let extra = if compact.jobset_extra_id == JOBSET_OVERFLOW_SENTINEL {
                shard.jobset_overflow.get(&key).cloned()
            } else {
                pool.jobset_extras
                    .get(compact.jobset_extra_id as usize)
                    .cloned()
            };
            match extra {
                Some(ids) => {
                    let ra = non_empty_string(pool.interner.resolve(ids.restart_attempt_id));
                    let uid = non_empty_string(pool.interner.resolve(ids.jobset_uid_id));
                    (ra, uid)
                }
                None => (None, None),
            }
        };
        drop(shard);
        drop(pool);

        if let Some(m) = metrics {
            m.inc_pod_cache_op("queryhit");
        }
        Some(ResolvedPodOwner {
            kind: compact.kind,
            name: controller_name,
            jobset_restart_attempt: restart_attempt,
            jobset_uid,
        })
    }

    /// Returns the number of cached Pods currently in the table.
    pub fn len(&self) -> usize {
        self.live_pods.load(Ordering::Relaxed)
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }
}

fn non_empty_string(s: &str) -> Option<String> {
    if s.is_empty() {
        None
    } else {
        Some(s.to_string())
    }
}

fn hash_pair(state: &RandomState, namespace: &str, pod_name: &str) -> u64 {
    let mut hasher = state.build_hasher();
    hasher.write_usize(namespace.len());
    hasher.write(namespace.as_bytes());
    hasher.write_u8(0xff);
    hasher.write_usize(pod_name.len());
    hasher.write(pod_name.as_bytes());
    hasher.finish()
}

/// Infers top-level GKE workload controller matching `parser.lua` and `pod_label_collector.go`.
pub fn infer_top_level_owner(pod: &PodMetadataExtract) -> Option<ResolvedPodOwner> {
    let mut result: Option<ResolvedPodOwner> = None;
    for owner in &pod.owner_references {
        if owner.name.is_empty() {
            continue;
        }
        let candidate = match owner.kind.as_str() {
            "DaemonSet" => Some(ResolvedPodOwner {
                kind: ControllerKind::DaemonSet,
                name: owner.name.clone(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
            "StatefulSet" => Some(ResolvedPodOwner {
                kind: ControllerKind::StatefulSet,
                name: owner.name.clone(),
                jobset_restart_attempt: None,
                jobset_uid: None,
            }),
            "ReplicaSet" => {
                if let Some(hash) = pod.labels.pod_template_hash.as_deref() {
                    if !hash.is_empty() {
                        let suffix = format!("-{hash}");
                        if let Some(prefix) = owner.name.strip_suffix(&suffix) {
                            if !prefix.is_empty() {
                                Some(ResolvedPodOwner {
                                    kind: ControllerKind::Deployment,
                                    name: prefix.to_string(),
                                    jobset_restart_attempt: None,
                                    jobset_uid: None,
                                })
                            } else {
                                None
                            }
                        } else {
                            None
                        }
                    } else {
                        None
                    }
                } else {
                    None
                }
            }
            "Job" => {
                if let Some(cronjob_name) = strip_cronjob_timestamp_suffix(&owner.name) {
                    Some(ResolvedPodOwner {
                        kind: ControllerKind::CronJob,
                        name: cronjob_name.to_string(),
                        jobset_restart_attempt: None,
                        jobset_uid: None,
                    })
                } else if let Some(jobset_name) =
                    pod.labels.jobset_name.as_deref().filter(|s| !s.is_empty())
                {
                    Some(ResolvedPodOwner {
                        kind: ControllerKind::JobSet,
                        name: jobset_name.to_string(),
                        jobset_restart_attempt: pod
                            .labels
                            .jobset_restart_attempt
                            .clone()
                            .filter(|s| !s.is_empty()),
                        jobset_uid: pod.labels.jobset_uid.clone().filter(|s| !s.is_empty()),
                    })
                } else {
                    Some(ResolvedPodOwner {
                        kind: ControllerKind::Job,
                        name: owner.name.clone(),
                        jobset_restart_attempt: None,
                        jobset_uid: None,
                    })
                }
            }
            _ => None,
        };
        if candidate.is_some() {
            result = candidate;
        }
    }
    result
}

/// Matches Kubernetes CronJob's 8-digit Unix minute suffix (`-[2-5][0-9]{7}`, valid 2008..2084).
fn strip_cronjob_timestamp_suffix(job_name: &str) -> Option<&str> {
    let (prefix, suffix) = job_name.rsplit_once('-')?;
    if prefix.is_empty() || suffix.len() != 8 {
        return None;
    }
    let bytes = suffix.as_bytes();
    if !(b'2'..=b'5').contains(&bytes[0]) {
        return None;
    }
    if !bytes[1..].iter().all(u8::is_ascii_digit) {
        return None;
    }
    Some(prefix)
}

/// Minimal `OwnerReference` fields extracted during streaming deserialization.
#[derive(Clone, Debug, Default, Deserialize)]
pub struct OwnerRefExtract {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub name: String,
}

/// Selective Pod labels deserialized without allocating a `HashMap` or retaining arbitrary user
/// labels.
#[derive(Clone, Debug, Default)]
pub struct RelevantPodLabels {
    pub pod_template_hash: Option<String>,
    pub jobset_name: Option<String>,
    pub jobset_restart_attempt: Option<String>,
    pub jobset_uid: Option<String>,
}

impl<'de> Deserialize<'de> for RelevantPodLabels {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        struct LabelsVisitor;

        impl<'de> Visitor<'de> for LabelsVisitor {
            type Value = RelevantPodLabels;

            fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
                formatter.write_str("a Kubernetes metadata.labels map or null")
            }

            fn visit_unit<E>(self) -> Result<Self::Value, E> {
                Ok(RelevantPodLabels::default())
            }

            fn visit_none<E>(self) -> Result<Self::Value, E> {
                Ok(RelevantPodLabels::default())
            }

            fn visit_some<D>(self, deserializer: D) -> Result<Self::Value, D::Error>
            where
                D: Deserializer<'de>,
            {
                deserializer.deserialize_any(self)
            }

            fn visit_map<M>(self, mut map: M) -> Result<Self::Value, M::Error>
            where
                M: MapAccess<'de>,
            {
                let mut out = RelevantPodLabels::default();
                while let Some(key) = map.next_key::<std::borrow::Cow<'de, str>>()? {
                    match key.as_ref() {
                        "pod-template-hash" => {
                            out.pod_template_hash = map.next_value::<Option<String>>()?;
                        }
                        "jobset.sigs.k8s.io/jobset-name" | "jobset.x-k8s.io/jobset-name" => {
                            let val = map.next_value::<Option<String>>()?;
                            if out.jobset_name.is_none() || key == "jobset.sigs.k8s.io/jobset-name"
                            {
                                out.jobset_name = val;
                            }
                        }
                        "jobset.sigs.k8s.io/restart-attempt"
                        | "jobset.x-k8s.io/restart-attempt" => {
                            let val = map.next_value::<Option<String>>()?;
                            if out.jobset_restart_attempt.is_none()
                                || key == "jobset.sigs.k8s.io/restart-attempt"
                            {
                                out.jobset_restart_attempt = val;
                            }
                        }
                        "jobset.sigs.k8s.io/jobset-uid" | "jobset.x-k8s.io/jobset-uid" => {
                            let val = map.next_value::<Option<String>>()?;
                            if out.jobset_uid.is_none() || key == "jobset.sigs.k8s.io/jobset-uid" {
                                out.jobset_uid = val;
                            }
                        }
                        _ => {
                            let _ = map.next_value::<IgnoredAny>()?;
                        }
                    }
                }
                Ok(out)
            }
        }

        deserializer.deserialize_option(LabelsVisitor)
    }
}

/// Extracted `PartialObjectMetadata` fields for a single Pod; skips `annotations`, `managedFields`,
/// `spec`, and `status` in-flight via serde field ignoring.
#[derive(Clone, Debug, Default, Deserialize)]
pub struct PodMetadataExtract {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub namespace: String,
    #[serde(default, rename = "resourceVersion")]
    pub resource_version: String,
    #[serde(default)]
    pub labels: RelevantPodLabels,
    #[serde(default, rename = "ownerReferences")]
    pub owner_references: Vec<OwnerRefExtract>,
}

/// Parses a `PartialObjectMetadata` JSON object directly into [`PodMetadataExtract`].
pub fn parse_partial_pod_object(bytes: &[u8]) -> Result<PodMetadataExtract, serde_json::Error> {
    #[derive(Deserialize)]
    struct Wrapper {
        #[serde(default)]
        metadata: PodMetadataExtract,
    }
    let parsed: Wrapper = serde_json::from_slice(bytes)?;
    Ok(parsed.metadata)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_churned_jobs_and_jobsets_reclaim_interner_and_inline_slots() {
        let table = PodOwnerTable::with_max_inline_jobsets(HashSet::new(), 4);
        for i in 0..10_000usize {
            let pod = PodMetadataExtract {
                name: format!("ephemeral-pod-{i}"),
                namespace: "batch".to_string(),
                resource_version: i.to_string(),
                labels: RelevantPodLabels {
                    jobset_name: Some(format!("jobset-{i}")),
                    jobset_restart_attempt: Some("0".to_string()),
                    jobset_uid: Some(format!("00000000-0000-0000-0000-{i:012}")),
                    ..Default::default()
                },
                owner_references: vec![OwnerRefExtract {
                    kind: "Job".to_string(),
                    name: format!("jobset-{i}-worker-0"),
                }],
            };
            table.upsert_pod_metadata(&pod, None);
            if i >= 2 {
                table.remove_pod("batch", &format!("ephemeral-pod-{}", i - 2), None);
            }
        }
        assert_eq!(table.len(), 2);
        let pool = table.pool.read().unwrap();
        assert!(
            pool.interner.values.len() <= 4096,
            "interner leaked {} strings",
            pool.interner.values.len()
        );
        let overflow_total: usize = table
            .shards
            .iter()
            .map(|s| s.read().unwrap().jobset_overflow.len())
            .sum();
        assert_eq!(
            overflow_total, 0,
            "2 live JobSets must fit in 4 inline slots without overflow spill"
        );
    }

    #[test]
    fn test_concurrent_sharded_upsert_lookup_and_compaction() {
        let table = Arc::new(PodOwnerTable::with_max_inline_jobsets(HashSet::new(), 4));
        std::thread::scope(|s| {
            for worker in 0..4usize {
                let t = Arc::clone(&table);
                s.spawn(move || {
                    for i in 0..1_500usize {
                        let js = (worker * 1_500 + i) % 12;
                        let pod_name = format!("pod-{worker}-{i}");
                        let pod = PodMetadataExtract {
                            name: pod_name.clone(),
                            namespace: "ml".to_string(),
                            resource_version: i.to_string(),
                            labels: RelevantPodLabels {
                                jobset_name: Some(format!("jobset-{js}")),
                                jobset_restart_attempt: Some((js % 3).to_string()),
                                jobset_uid: Some(format!("uid-{js}")),
                                ..Default::default()
                            },
                            owner_references: vec![OwnerRefExtract {
                                kind: "Job".to_string(),
                                name: format!("jobset-{js}-w0"),
                            }],
                        };
                        t.upsert_pod_metadata(&pod, None);
                        let got = t
                            .lookup("ml", &pod_name, None)
                            .expect("inserted pod must resolve");
                        assert_eq!(got.name, format!("jobset-{js}"));
                        if i >= 8 {
                            t.remove_pod("ml", &format!("pod-{worker}-{}", i - 8), None);
                        }
                    }
                });
            }
        });
        assert_eq!(table.len(), 32);
        let live_sum: usize = table
            .shards
            .iter()
            .map(|s| s.read().unwrap().entries.len())
            .sum();
        assert_eq!(live_sum, 32);
    }
}
