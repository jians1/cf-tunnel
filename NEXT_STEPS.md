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

- `http2`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19304`, `rss_warm_kb=19528`, `peak_rss_kb=19532`, `rss_final_kb=19532`
- `quic`: `duration_seconds=10`, `throughput_mbps=858.99`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=21384`, `rss_warm_kb=21388`, `peak_rss_kb=21868`, `rss_final_kb=21792`

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

## Guardrails

- Keep diffs small and attributable.
- Do not mix quality/performance optimization with unrelated feature work.
- Always compare release builds, not plain `go build`, when discussing shipping size.
- Treat old memory numbers collected with unknown sampling as historical hints, not hard baselines.
