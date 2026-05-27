# Next Steps

## Current State

- The current release candidate is stable enough to publish as `0.1.0-prototype`.
- Real end-to-end regression checks still pass for both `http2` and `quic` after dependency trimming:
  - `1GiB` downloads completed with matching SHA256
  - RSS stayed in the tens of MiB range
- The `cloudflared` dependency surface has been trimmed substantially:
  - removed `management` / `go-jose` from the production build graph
  - removed `urfave/cli`, `fsnotify`, and related command-only/runtime-unneeded packages from the production build graph
  - removed `hello`, `ipaccess`, `socks`, `otel`, `gorilla/websocket`, `connection`, `tunnelrpc`, `tunnelrpc/quic`, and `tunnelrpc/metrics` from the production build graph
- The remaining heavy runtime dependency cluster is now concentrated in:
  - `third_party/cloudflared/tunnelrpc/pogs`
  - `third_party/cloudflared/tunnelrpc/proto`
  - `zombiezen.com/go/capnproto2`

## Fixed Benchmark Output Standard

All future RSS comparisons should use the same fields from `scripts/e2e/run_trycloudflare_ab.sh`:

- `rss_ready_kb`
- `rss_warm_kb`
- `peak_rss_kb`
- `rss_final_kb`

Do not compare new results with older ad-hoc measurements unless the sampling method is known to match.

## Release Size Baseline

Current release build:

- command: `./scripts/build-release.sh`
- target: `linux/amd64`
- size: `11,309,218 bytes`

Comparison point:

- `502c10b` release build size: `13,303,970 bytes`
- current is smaller by `1,994,752 bytes` (`14.99%`)

## Recommended Next Work

Primary goal: decide whether to keep shipping the current `0.1.0-prototype` as-is or continue into a higher-risk protocol-layer rewrite.

Recommended order:

1. Preserve the current publishable state
   - keep the `0.1.0-prototype` release artifacts and notes
   - treat current end-to-end results as the release baseline

2. If further slimming is required, scope it as a new high-risk effort
   - replace `tunnelrpc/pogs` with a minimal local protocol layer
   - reduce or eliminate `zombiezen.com/go/capnproto2/pogs` reflection-based bindings
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
