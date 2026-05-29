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

## Next Phase Execution Checklist (Phase B: Optional Config File)

1. Decide config-file scope
- Keep CLI single-tunnel only.
- Use config file only if multi-tunnel or larger route sets are needed.

2. Preserve current runtime behavior
- Do not reintroduce complex multi-tunnel CLI syntax.
- Keep single-tunnel CLI backward compatible.

## Guardrails (Still Active)

- Keep diffs small and attributable.
- Do not mix optimization work with unrelated feature changes.
- Use release builds when discussing shipping binary size.
- Treat one-shot network benchmarks as noisy; use repeated serial rounds before drawing conclusions.
