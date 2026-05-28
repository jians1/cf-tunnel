# Next Steps

## Current State

- `0.1.0-prototype` baseline is usable.
- Path-based routing (`/api/*`, `/ws`, default route) is implemented and validated.
- Real-link download checks pass for both `http2` and `quic` with `path_routing_check=pass`.
- RSS remains in the tens-of-MiB range with no monotonic growth trend observed in serial rounds.

## Fixed Benchmark Output Standard

All RSS comparisons must use fields produced by `scripts/e2e/run_trycloudflare_ab.sh`:

- `rss_ready_kb`
- `rss_warm_kb`
- `peak_rss_kb`
- `rss_final_kb`

For throughput comparisons, run protocols serially only:

```bash
bash scripts/e2e/run_trycloudflare_ab.sh http2 1
bash scripts/e2e/run_trycloudflare_ab.sh quic 1
```

When reporting, always include:

- `duration_seconds`
- `throughput_mbps`
- `sha256`
- `rss_ready_kb`
- `rss_warm_kb`
- `peak_rss_kb`
- `rss_final_kb`
- `path_routing_check`

## Recently Completed (Archived)

- Phase A path-based backend routing delivery (A1-A5).
- Real-link e2e path split validation integrated into benchmark script.
- `http2/quic` 3-round serial verification with stable checksum and routing checks.

## Next Phase Execution Checklist (Phase B: Multi-Tunnel)

1. B1 Config Modeling
- Define multi-tunnel config schema (name + per-tunnel cftunnel config).
- Enforce unique tunnel name/identifier validation.
- Keep single-tunnel CLI behavior backward compatible.
- Status: done.

2. B2 Runtime Orchestration
- Refactor single-tunnel runner into reusable per-instance startup unit.
- Add multi-instance orchestrator to start all tunnels under one lifecycle.
- Define explicit startup policy when one tunnel fails (fail-fast vs continue).
- Status: done (policy = fail-fast).

3. B3 Lifecycle and Shutdown
- Add deterministic shutdown ordering for all tunnel instances.
- Ensure shared shutdown timeout is enforced and test-covered.
- Prevent partial-success silent states; surface explicit aggregated errors.
- Status: partial.
- Implemented: fail-fast cancellation, wait-for-all goroutines, aggregated error output.
- Remaining: stronger deterministic shutdown ordering contract and dedicated lifecycle regression cases.

4. B4 Observability
- Add per-tunnel log fields (`tunnel_name`/`tunnel_id`).
- Extend health/readiness to expose multi-instance status summary.
- Status: partial.
- Implemented: `tunnel_name` log field and `/ready` multi-tunnel summary provider.
- Remaining: include stable tunnel identifier field and richer readiness dimensions.

5. B5 Verification and Release Gate
- Unit tests: config parsing/validation and orchestrator lifecycle semantics.
- Integration smoke: at least 2 tunnels active simultaneously.
- Real-link regression: single-tunnel mode must keep current throughput/RSS envelope.
- Status: in progress.
- Implemented: config + orchestrator unit tests and full `go test ./...` pass.
- Remaining:
  - add 2-tunnel simultaneous integration smoke
  - run serial real-link regression:
    - `bash scripts/e2e/run_trycloudflare_ab.sh http2 1`
    - `bash scripts/e2e/run_trycloudflare_ab.sh quic 1`

## Guardrails (Still Active)

- Keep diffs small and attributable.
- Do not mix optimization work with unrelated feature changes.
- Use release builds when discussing shipping binary size.
- Treat one-shot network benchmarks as noisy; use repeated serial rounds before drawing conclusions.
