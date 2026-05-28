# Next Steps

## Current State

- `0.1.0-prototype` baseline is usable.
- Real-link download checks continue to pass for both `http2` and `quic`.
- RSS remains stable in the tens-of-MiB range; no monotonic growth trend was observed.
- Dependency trimming and startup/runtime allocation optimizations from previous rounds are already landed.

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

## Active Priorities

1. Maintain release stability and keep verification evidence fresh.
2. Prioritize correctness and lifecycle robustness over micro-optimization.
3. Only take performance/memory changes that show clear repeatable gains in multi-round serial tests.

## Guardrails

- Keep diffs small and attributable.
- Do not mix optimization work with unrelated feature changes.
- Use release builds when discussing shipping binary size.
- Treat one-shot network benchmarks as noisy; use repeated serial rounds before drawing conclusions.
