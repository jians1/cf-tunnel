# Next Steps

## Current State

- The current release candidate is stable enough to publish as `0.1.0-prototype`.
- Latest validated commits:
  - `586a41f refactor: remove runtime capnp pogs marshaling`
  - `7cf9b92 docs: enforce serial throughput sampling policy`
- Real end-to-end regression checks still pass for both `http2` and `quic` after dependency trimming:
  - `1GiB` downloads completed with matching SHA256
  - RSS stayed in the tens of MiB range
- The `cloudflared` dependency surface has been trimmed substantially:
  - removed `management` / `go-jose` from the production build graph
  - removed `urfave/cli`, `fsnotify`, and related command-only/runtime-unneeded packages from the production build graph
  - removed `hello`, `ipaccess`, `socks`, `otel`, `gorilla/websocket`, `connection`, `tunnelrpc`, `tunnelrpc/quic`, and `tunnelrpc/metrics` from the production build graph
- Removed the runtime server-side dependency on `third_party/cloudflared/tunnelrpc/pogs`
- The remaining Cloudflared runtime dependency is now concentrated in:
  - `third_party/cloudflared/tunnelrpc/proto`
  - `zombiezen.com/go/capnproto2`

Latest serial real-link measurements on 2026-05-28:

- `http2`: `duration_seconds=9`, `throughput_mbps=954.44`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19512`, `rss_warm_kb=19512`, `peak_rss_kb=19524`, `rss_final_kb=19080`
- `quic`: `duration_seconds=11`, `throughput_mbps=780.90`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19200`, `rss_warm_kb=19204`, `peak_rss_kb=19080`, `rss_final_kb=18992`

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
- size: `9,904,290 bytes`

Comparison point:

- `502c10b` release build size: `13,303,970 bytes`
- current is smaller by `3,399,680 bytes` (`25.55%`)

## Recommended Next Work

Primary goal: decide whether to keep shipping the current `0.1.0-prototype` as-is or continue into a higher-risk protocol-layer rewrite.

Recommended order:

1. Preserve the current publishable state
   - keep the `0.1.0-prototype` release artifacts and notes
   - treat current end-to-end results as the release baseline

2. If further slimming is required, scope it as a new high-risk effort
   - keep `internal/` runtime path pinned to direct `tunnelrpc/proto` schema access only
   - evaluate whether `third_party/cloudflared` vendored tree can be further trimmed without breaking wire compatibility:
     - build a candidate list of vendored directories unreachable from runtime/test/build paths
     - remove only clearly unreachable directories in small, isolated patches
     - verify each patch with `go test -count=1 ./...` and `bash scripts/build-release.sh`
   - avoid mixing this with unrelated cleanup

3. After any protocol-layer change
   - rebuild the release binary
   - run `go test ./internal/cftunnel/...`
   - rerun real `http2` and `quic` end-to-end downloads

## Guardrails

- Keep diffs small and attributable.
- Do not mix dependency trimming with unrelated cleanup.
- Always compare release builds, not plain `go build`, when discussing shipping size.
- Treat old memory numbers collected with unknown sampling as historical hints, not hard baselines.
