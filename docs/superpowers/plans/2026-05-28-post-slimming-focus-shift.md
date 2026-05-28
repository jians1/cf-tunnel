# Post-Slimming Focus Shift Plan

## Context

As of 2026-05-28, the dependency slimming phase is functionally complete for the current architecture target.

Recent validated commits:

- `91cb794 chore: keep only cloudflared tunnelrpc subtree`
- `f9b477d chore: remove remaining cloudflared tunnelrpc tests`
- `eabcc5a chore: keep only cloudflared tunnelrpc proto package`
- `d4aef3e chore: remove unused capnp source files`
- `8817692 chore: tidy go module dependencies`

Current release binary baseline (`linux/amd64`):

- `9,744,546 bytes`

Latest serial real-link checks (1 GiB):

- `http2`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`
- `quic`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`

## Conclusion

Further dependency-file trimming in current scope is low-yield for binary size. Next engineering value is higher in:

1. code quality hardening
2. targeted performance optimization
3. runtime resource occupancy improvements

## Next Iteration Plan

### A. Code Quality

- Audit `internal/cftunnel/runtime` for duplicate protocol/registration flow logic.
- Consolidate config and error invariants in one place per concern.
- Strengthen unit tests for reconnect, shutdown, and failure propagation.

### B. Performance

- Establish reproducible perf baseline from serial e2e (`http2`, `quic`).
- Add targeted profiles to identify hot allocations and CPU-heavy paths.
- Rank top bottlenecks by measured cost before implementing optimizations.

### C. Resource Occupancy

- Reduce transient allocations on data stream path (buffer reuse where safe).
- Tighten goroutine lifecycle and cancellation propagation on shutdown.
- Re-check RSS fields (`rss_ready_kb`, `rss_warm_kb`, `peak_rss_kb`, `rss_final_kb`) per change.

## Validation Gate Per Change

For each optimization patch:

1. `go test -count=1 ./...`
2. `bash scripts/build-release.sh`
3. serial real-link checks:
   - `bash scripts/e2e/run_trycloudflare_ab.sh http2 1`
   - `bash scripts/e2e/run_trycloudflare_ab.sh quic 1`

## Guardrails

- Keep changes small and attributable.
- Avoid mixing optimization patches with feature additions.
- Prefer evidence-based optimization over assumption-based refactors.
