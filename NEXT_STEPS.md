# Next Steps

## Current State

- The current release candidate is stable enough to publish as `0.1.0-prototype`.
- Latest validated commits:
  - `d4aef3e chore: remove unused capnp source files`
  - `8817692 chore: tidy go module dependencies`
- Real end-to-end regression checks still pass for both `http2` and `quic` after dependency trimming:
  - `1GiB` downloads completed with matching SHA256
  - RSS stayed in the tens of MiB range
- The `cloudflared` dependency surface has been trimmed substantially:
  - removed `management` / `go-jose` from the production build graph
  - removed `urfave/cli`, `fsnotify`, and related command-only/runtime-unneeded packages from the production build graph
  - removed `hello`, `ipaccess`, `socks`, `otel`, `gorilla/websocket`, `connection`, `tunnelrpc`, `tunnelrpc/quic`, and `tunnelrpc/metrics` from the production build graph
- Removed the runtime server-side dependency on `third_party/cloudflared/tunnelrpc/pogs`
- The remaining Cloudflared runtime dependency is now concentrated in:
  - `third_party/cloudflared/tunnelrpc/proto/*.capnp.go`
  - `zombiezen.com/go/capnproto2`

Latest serial real-link measurements on 2026-05-28:

- `http2`: `duration_seconds=9`, `throughput_mbps=954.44`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=17444`, `rss_warm_kb=17488`, `peak_rss_kb=17496`, `rss_final_kb=17496`
- `quic`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=20384`, `rss_warm_kb=20388`, `peak_rss_kb=20852`, `rss_final_kb=20308`

## Fixed Benchmark Output Standard

All future RSS comparisons should use the same fields from `scripts/e2e/run_trycloudflare_ab.sh`:

- `rss_ready_kb`
- `rss_warm_kb`
- `peak_rss_kb`
- `rss_final_kb`

Do not compare new results with older ad-hoc measurements unless the sampling method is known to match.

For real-link throughput measurements, `http2` and `quic` MUST run serially.

- Required order: finish one protocol run, then start the next.
- Forbidden: running `http2` and `quic` at the same time for throughput comparison.
- Reason: parallel runs contend for CPU/network and distort `throughput_mbps`.

Recommended serial commands:

```bash
bash scripts/e2e/run_trycloudflare_ab.sh http2 1
bash scripts/e2e/run_trycloudflare_ab.sh quic 1
```

When reporting results, include these fields from each run's `result.txt`:

- `duration_seconds`
- `throughput_mbps`
- `sha256`
- `rss_ready_kb`
- `rss_warm_kb`
- `peak_rss_kb`
- `rss_final_kb`

## Release Size Baseline

Current release build:

- command: `./scripts/build-release.sh`
- target: `linux/amd64`
- size: `9,744,546 bytes`

Comparison point:

- `502c10b` release build size: `13,303,970 bytes`
- current is smaller by `3,399,680 bytes` (`25.55%`)

## Recommended Next Work

Primary goal: shift focus from dependency slimming to code quality, performance optimization, and resource-usage stability.

Recommended order:

1. Code quality baseline
   - map top error-prone paths in `internal/cftunnel/runtime`, `internal/cftunnel/api`, `internal/ipv6pool`
   - remove duplicate validation/config branches and unify invariants
   - add/strengthen focused unit tests for connection lifecycle and failure paths

2. Performance baseline and profiling
   - collect reproducible baseline from serial `http2`/`quic` e2e runs (throughput + RSS fields)
   - run targeted CPU/memory profiling on hot paths (registration, data stream forwarding, reconnect path)
   - identify top 3 bottlenecks and estimate impact before changing code

3. Resource occupancy optimization
   - optimize buffer reuse and reduce avoidable allocations in data plane
   - tune goroutine lifecycle and shutdown path to prevent lingering workers
   - verify regression gates after each optimization:
     - `go test -count=1 ./...`
     - `bash scripts/build-release.sh`
     - serial real-link `http2` and `quic` runs

## Profiling Baseline (2026-05-28)

First reproducible profiling artifacts have been collected from focused runtime tests.

Artifacts directory:

- `artifacts/profiles/2026-05-28/registration.{cpu,mem}.pprof`
- `artifacts/profiles/2026-05-28/forwarding.{cpu,mem}.pprof`
- `artifacts/profiles/2026-05-28/reconnect.{cpu,mem}.pprof`
- `artifacts/profiles/2026-05-28/*.top.txt`

Sampling commands:

```bash
go test -count=200 ./internal/cftunnel/runtime -run '^TestHTTP2ServerControlStreamRegistrationLifecycle$' -cpuprofile artifacts/profiles/2026-05-28/registration.cpu.pprof -memprofile artifacts/profiles/2026-05-28/registration.mem.pprof
go test -count=200 ./internal/cftunnel/runtime -run '^TestUpstreamOriginProxyProxyTCPForwardsBytes$' -cpuprofile artifacts/profiles/2026-05-28/forwarding.cpu.pprof -memprofile artifacts/profiles/2026-05-28/forwarding.mem.pprof
go test -count=200 ./internal/cftunnel/runtime -run '^TestBridgeRunnerQUICUsesConfiguredEdgeDial$' -cpuprofile artifacts/profiles/2026-05-28/reconnect.cpu.pprof -memprofile artifacts/profiles/2026-05-28/reconnect.mem.pprof
```

Initial bottleneck view (top signals):

1. TLS/root initialization allocations are repeatedly dominant in setup-heavy paths:
   - `crypto/x509.(*CertPool).Clone`
   - `crypto/x509.(*CertPool).AppendCertsFromPEM`
   - reached ~`59.21%` cumulative alloc path under `PrepareRuntime` in registration sample
2. Stream forwarding alloc hotspot is `io.copyBuffer`:
   - ~`63.97%` alloc_space in forwarding sample
3. QUIC bridge startup alloc profile is still setup-dominant:
   - `buildEdgeTLSConfigs -> newEdgeTLSConfig` cumulative alloc path ~`84.78%`

Interpretation guardrail:

- These are focused unit-test profiles and emphasize startup/setup costs.
- Before large code changes, validate with one follow-up profile pass that uses longer-lived data-plane traffic.

## Optimization Round 1 (2026-05-28)

Patch:

- Added one-time cached edge root CA pool initialization in runtime TLS setup (`sync.Once`).
- Replaced per-call `SystemCertPool + AppendCertsFromPEM` in `newEdgeTLSConfig` with shared cached pool.
- Added regression test coverage for reuse behavior.

Profile delta (registration sample, `TestHTTP2ServerControlStreamRegistrationLifecycle`, `-count=200`):

- before total alloc_space: `33014.86kB`
- after total alloc_space: `14080.47kB`
- reduction: `57.35%`
- `PrepareRuntime` cumulative alloc path:
  - before: `19549.19kB` (`59.21%`)
  - after: `3587.22kB` (`25.48%`)

Post-patch release and regression gates:

- `go test -count=1 ./...` passed
- `bash scripts/build-release.sh` passed
- release size (`linux/amd64`): `9,744,546 bytes` (unchanged)

Post-patch serial real-link measurements on 2026-05-28:

- `http2`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=16960`, `rss_warm_kb=16964`, `peak_rss_kb=17416`, `rss_final_kb=17416`
- `quic`: `duration_seconds=13`, `throughput_mbps=660.76`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19448`, `rss_warm_kb=19452`, `peak_rss_kb=19836`, `rss_final_kb=19332`

Note:

- This round improves setup allocations clearly.
- Throughput fluctuates on real-link runs; use repeated serial rounds before concluding protocol throughput trend.

## Optimization Round 2 (2026-05-28)

Patch:

- Optimized TCP forwarding copy path in `ProxyTCP`:
  - introduced pooled fixed-size buffers (`32KiB`)
  - replaced `io.Copy` fast-path usage with explicit buffered stream copy loop to ensure buffer reuse is effective

Validation:

- `go test -count=1 ./internal/cftunnel/runtime` passed
- `go test -count=1 ./...` passed

Profile delta (forwarding sample, `TestUpstreamOriginProxyProxyTCPForwardsBytes`, `-count=200`):

- before total alloc_space: `16512.60kB`
- after total alloc_space: `3848.11kB`
- reduction: `76.70%`

Hotspot change:

- before: `io.copyBuffer` `10563.33kB` (`63.97%`)
- after: `io.copyBuffer` no longer appears as dominant allocator in top output

## Reconnect Follow-up (2026-05-28)

Actions:

- Re-sampled reconnect path with current code:
  - `go test -count=200 ./internal/cftunnel/runtime -run '^TestBridgeRunnerQUICUsesConfiguredEdgeDial$' -cpuprofile ... -memprofile ...`
- Optimized orchestrator config JSON build path:
  - replaced dynamic `map[string]any` marshaling with typed structs in `NewUpstreamOrchestrator`

Observed reconnect profile status:

- early baseline (before optimization rounds): `35814.72kB` alloc_space
- after optimization rounds, reconnect sample stabilized around ~`11MB` alloc_space
  - sample A: `10793.32kB`
  - sample B: `11057.80kB`

Current dominant residual cost remains one-time runtime initialization:

- `crypto/x509.SystemCertPool` / `AppendCertsFromPEM`
- test/runtime one-time initialization paths (including profiling overhead)

Conclusion:

- Reconnect path has already improved substantially versus original baseline.
- Additional small edits now show sampling noise; next useful step is multi-round averaging before further micro-optimizations.

Validation:

- `go test -count=1 ./internal/cftunnel/runtime` passed
- `go test -count=1 ./...` passed
- `bash scripts/build-release.sh` passed

## Multi-round Sampling Snapshot (2026-05-28)

To evaluate baseline stability, we ran 5 serial rounds per path with identical command shape (`go test -count=200 ... -memprofile ...`).

Raw totals (`alloc_space`, kB):

- `registration`: `14352.21`, `15890.21`, `12418.67`, `8713.90`, `14352.32`
- `forwarding`: `3072.52`, `3600.45`, `3088.67`, `2577.13`, `2560.36`
- `reconnect`: `4609.96`, `4609.17`, `6898.76`, `9854.06`, `10484.18`

Stats (all 5 rounds):

- `registration`: mean `13145.46`, min `8713.90`, max `15890.21`, range/mean `54.59%`
- `forwarding`: mean `2979.83`, min `2560.36`, max `3600.45`, range/mean `34.90%`
- `reconnect`: mean `7291.23`, min `4609.17`, max `10484.18`, range/mean `80.58%`

Stats (drop round 1 warmup):

- `registration`: mean `12843.77`, min `8713.90`, max `15890.21`, range/mean `55.87%`
- `forwarding`: mean `2956.65`, min `2560.36`, max `3600.45`, range/mean `35.18%`
- `reconnect`: mean `7961.54`, min `4609.17`, max `10484.18`, range/mean `73.79%`

Interpretation:

- `forwarding` is now materially improved and relatively stable after optimization.
- `registration` and `reconnect` remain noisy in unit-test-based profiles, indicating initialization and test harness effects still dominate.

Decision:

- Pause further micro-optimization on `registration/reconnect` from this test shape.
- Next profiling step should move to longer-lived data-plane traffic scenarios to reduce one-time initialization noise before additional tuning.

## Guardrails

- Keep diffs small and attributable.
- Do not mix quality/performance optimization with unrelated feature work.
- Always compare release builds, not plain `go build`, when discussing shipping size.
- Treat old memory numbers collected with unknown sampling as historical hints, not hard baselines.
