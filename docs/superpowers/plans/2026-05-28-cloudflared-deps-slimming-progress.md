# Cloudflared Dependency Slimming Progress Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Continue reducing direct `cloudflared` runtime dependencies while preserving real Quick Tunnel behavior over `http2` and `quic`.

**Architecture:** Runtime protocol ownership is being moved into `internal/cftunnel/runtime`, with direct Cap'n Proto wire handling kept explicit and tested. Each dependency removal should be small, wire-compatible, and verified with unit tests, full tests, release build, and real trycloudflare link tests when RPC behavior changes.

**Tech Stack:** Go, Cap'n Proto via `github.com/cloudflare/cloudflared/tunnelrpc/proto`, Quick Tunnel e2e scripts, static release builds.

---

## Current Progress

**Git state**
- Branch: `slim-cloudflared-deps`
- Worktree: dirty (pending documentation updates in `NEXT_STEPS.md` and this progress file)
- Latest commits:
  - `7cf9b92 docs: enforce serial throughput sampling policy`
  - `586a41f refactor: remove runtime capnp pogs marshaling`

**Completed refactors**
- Removed direct runtime usage of `cloudflared/flow`, `cloudflared/quic`, and `cloudflared/tlsconfig`.
- Localized QUIC stream close behavior and flow error handling.
- Localized edge TLS configuration with embedded Cloudflare root CA.
- Aligned e2e binary build flags with release flags: `CGO_ENABLED=0 -trimpath -ldflags="-s -w"`.
- Localized QUIC data stream protocol types and round-trip tests.
- Localized registration-side runtime RPC data types and removed direct `RegistrationServer_PogsClient` use.
- Removed the runtime server-side dependency on `cloudflared/tunnelrpc/pogs`.
- Removed runtime `capnproto2/pogs` marshaling for `ConnectionOptions` and `TunnelAuth`.

**Known validation evidence**
- `go test -count=1 ./internal/cftunnel/runtime` passed after protocol localization.
- `go test -count=1 ./...` passed after protocol localization.
- `bash scripts/build-release.sh` passed after protocol localization.
- `go test -count=1 ./internal/cftunnel/runtime` passed after removing runtime `tunnelrpc/pogs`.
- `go test -count=1 ./...` passed after removing runtime `tunnelrpc/pogs`.
- `bash scripts/build-release.sh` passed after removing runtime `tunnelrpc/pogs`.
- Current release binary size after removing runtime `tunnelrpc/pogs`: `9,904,290 bytes`.
- Real Quick Tunnel e2e previously passed for `http2` and `quic`.
- Real Quick Tunnel e2e revalidated serially on 2026-05-28:
  - `http2`: `duration_seconds=9`, `throughput_mbps=954.44`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19512`, `rss_warm_kb=19512`, `peak_rss_kb=19524`, `rss_final_kb=19080`
  - `quic`: `duration_seconds=11`, `throughput_mbps=780.90`, `sha256=49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14`, `rss_ready_kb=19200`, `rss_warm_kb=19204`, `peak_rss_kb=19080`, `rss_final_kb=18992`

**Remaining direct `cloudflared` imports**
- `internal/cftunnel/runtime/quic_protocol.go`: `github.com/cloudflare/cloudflared/tunnelrpc/proto`
- `internal/cftunnel/runtime/quic_protocol_test.go`: `github.com/cloudflare/cloudflared/tunnelrpc/proto`

`proto` is still expected for wire schema access. Runtime path has removed reflection-based `pogs` marshaling; the next meaningful target is evaluating whether additional vendored `cloudflared` surface can be trimmed while preserving wire compatibility.

## Implementation Plan

### Task 1: Remove UDP Session POGS Adapter

**Files:**
- Modify: `internal/cftunnel/runtime/quic_protocol.go`
- Test: `internal/cftunnel/runtime/quic_protocol_test.go`

- [x] **Step 1: Add a focused marshal test**

  Add a test that constructs `runtimeRegisterUDPSessionResponse`, marshals it through the direct `proto.RegisterUdpSessionResponse` path, and verifies:
  - destination IP is preserved
  - destination port is preserved
  - close-after-idle hint is preserved
  - error field behavior matches existing POGS behavior

- [x] **Step 2: Replace `runtimeRegisterUDPSessionResponse.toPogs`**

  Remove `toPogs()` for UDP session response and replace the server adapter with direct Cap'n Proto response writing.

- [x] **Step 3: Run focused tests**

  Run: `go test -count=1 ./internal/cftunnel/runtime`

### Task 2: Remove Configuration POGS Adapter

**Files:**
- Modify: `internal/cftunnel/runtime/quic_protocol.go`
- Test: `internal/cftunnel/runtime/quic_protocol_test.go`

- [x] **Step 1: Add a focused marshal test**

  Add a test that constructs `runtimeUpdateConfigurationResponse`, marshals it through direct `proto.UpdateConfigurationResponse`, and verifies:
  - successful response is encoded as success
  - error response preserves the error message

- [x] **Step 2: Replace `runtimeUpdateConfigurationResponse.toPogs`**

  Remove `toPogs()` for configuration response and return direct schema-backed responses from the RPC server adapter.

- [x] **Step 3: Run focused tests**

  Run: `go test -count=1 ./internal/cftunnel/runtime`

### Task 3: Replace `CloudflaredServer_ServerToClient`

**Files:**
- Modify: `internal/cftunnel/runtime/quic_protocol.go`
- Modify: `internal/cftunnel/runtime/quic_rpc_server.go`
- Test: `internal/cftunnel/runtime/quic_protocol_test.go`

- [x] **Step 1: Map required server methods**

  Inspect generated `proto.CloudflaredServer` server interfaces and identify only the methods required by runtime RPC:
  - UDP session registration
  - configuration update
  - any server-to-client methods currently routed by the POGS adapter

- [x] **Step 2: Implement direct server-to-client binding**

  Replace `tunnelpogs.CloudflaredServer_ServerToClient(...)` with a local implementation that exposes the required methods using generated `proto` server interfaces.

- [x] **Step 3: Keep failures explicit**

  For unsupported methods, return explicit errors instead of silently succeeding.

- [x] **Step 4: Run focused tests**

  Run: `go test -count=1 ./internal/cftunnel/runtime`

### Task 4: Verify Dependency Reduction

**Files:**
- Modify: `go.mod` if `github.com/cloudflare/cloudflared/tunnelrpc/pogs` becomes unused indirectly by this module
- Modify: `go.sum` if dependency graph changes

- [x] **Step 1: Confirm remaining imports**

  Run: `grep -R "github.com/cloudflare/cloudflared/" -n -- cmd internal go.mod | sort`

  Expected runtime result:
  - `internal/cftunnel/runtime/quic_protocol.go` may still import `tunnelrpc/proto`
  - no `tunnelrpc/pogs` import from runtime code

- [x] **Step 2: Run full test suite**

  Run: `go test -count=1 ./...`

- [x] **Step 3: Build release binary**

  Run: `bash scripts/build-release.sh`

- [x] **Step 4: Record binary size**

  Run: `stat -c '%n %s' dist/cf-quicktunnel-ipv6pool-0.1.0-prototype-linux-amd64`

### Task 5: Run Real Link Tests

**Files:**
- Read logs from: `/tmp/cfqt-e2e`

**Execution rule (required for comparable throughput):**
- Run `http2` and `quic` serially, not in parallel.
- Parallel runs are acceptable only for functional smoke checks, not for `throughput_mbps` comparison.
- Always report the same sampled fields:
  - `duration_seconds`
  - `throughput_mbps`
  - `sha256`
  - `rss_ready_kb`
  - `rss_warm_kb`
  - `peak_rss_kb`
  - `rss_final_kb`

- [x] **Step 1: Test `http2` real Quick Tunnel**

  Run: `bash scripts/e2e/run_trycloudflare_ab.sh http2 1`

  Expected:
  - script creates a real `trycloudflare.com` URL
  - 1GiB transfer completes
  - SHA256 matches the generated source file

- [x] **Step 2: Test `quic` real Quick Tunnel**

  Run: `bash scripts/e2e/run_trycloudflare_ab.sh quic 1`

  Expected:
  - script creates a real `trycloudflare.com` URL
  - 1GiB transfer completes
  - SHA256 matches the generated source file

- [x] **Step 3: Inspect logs on failure**

  If a real-link test fails, inspect:
  - `/tmp/cfqt-e2e/<proto>-round1/cfqt.log`
  - `/tmp/cfqt-e2e/<proto>-round1/sing-client.log`
  - `/tmp/cfqt-e2e/<proto>-round1/sing-server.log`

### Task 6: Commit Only After User Approval

**Files:**
- Commit changed runtime code, tests, and this progress plan if desired.

- [x] **Step 1: Review diff**

  Run: `git diff --stat && git diff --check`

- [x] **Step 2: Commit only when requested**

  Example message after validation:

  ```bash
  git commit -m "refactor: remove runtime tunnelrpc pogs adapter"
  ```

## Notes for Next Worker

- Keep each protocol change small. The last real-link failure was caused by a Cap'n Proto field tag mismatch: `OriginLocalIP` needed `capnp:"originLocalIp"`.
- Do not assume unit tests are enough for RPC wire changes. Real-link `http2` and `quic` tests are required before claiming behavior is preserved.
- A temporary binary size increase is acceptable while local wrappers coexist with remaining generated adapters. Measure again after `tunnelrpc/pogs` is removed.

## Next Phase Plan

1. Build a safe-trim candidate list under `third_party/cloudflared`
   - enumerate runtime-reachable packages from `cmd/internal`
   - flag vendored directories not in runtime/test/build paths
2. Apply one minimal trim patch at a time
   - delete only clearly unreachable vendored directories
   - avoid touching `tunnelrpc/proto` and capnp runtime dependencies
3. Verify each trim patch
   - `go test -count=1 ./...`
   - `bash scripts/build-release.sh`
   - if protocol path changed: rerun serial real-link tests (`http2` then `quic`)
