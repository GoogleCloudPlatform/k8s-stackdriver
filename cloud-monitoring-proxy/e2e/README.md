# Live end-to-end suite

Verifies a deployed proxy against a real GKE cluster and against direct
Cloud Monitoring reads. Run with `E2E_PROJECT=<your-project> make e2e`
(~12 min; add `-short` to skip the slow eviction test).

Prerequisites:

- Cluster + proxy deployed per `hack/cluster-up.sh` and `hack/deploy.sh`
  (sample workload applied: `deploy/manifests/sample-workload.yaml`).
- `kubectl` context pointing at the cluster; Application Default Credentials
  with `roles/monitoring.viewer` on the project.
- Environment variables: `E2E_PROJECT` (required), plus optional
  `E2E_CLUSTER`, `E2E_LOCATION`, `E2E_NAMESPACE` (defaults match
  `hack/cluster-up.sh`).

What it checks:

| Test | Assertion |
|---|---|
| FamiliesPresent | Preset families served with plausible values and correct scope labels |
| CrossCheckAgainstGCM | Proxy gauge/counter values match one of GCM's two most recent points for the same series |
| DataAgeWithinBounds | `gcmproxy_data_age_seconds` < ingestDelay + 2×samplePeriod + margin, per metric type |
| BilledEstimateAdvances | Billed-series estimate advances ≈ cached series per poll cycle |
| DeltaCountersMonotonic | Delta-accumulated counters never decrease between scrapes |
| StalenessEviction | Deleting the sample workload evicts its series within lookback + staleAfter + sweep cadence (~7–10 min) |

The suite self-heals `kubectl port-forward` drops and treats scrape errors
in wait loops as "unknown", never as success.
