# Project Construction Plan

## Goal

Build a single-binary Go application for lightweight Cloudflare tunnel forwarding.

The application forwards traffic from Cloudflare tunnel endpoints to configured local origin targets. The current baseline supports Quick Tunnel and first-stage formal Cloudflare Tunnel token mode, and the active construction plan makes `/ready` reflect real Cloudflare edge registration readiness.

## Product Scope

### In Scope

- Single Go binary
- CLI flag based startup model
- Cloudflare Quick Tunnel support
- First-stage formal Cloudflare Tunnel token support
- Edge transport selection: `quic | http2`
- Origin scheme selection through full target URLs: `http | https | ws | wss`
- Local reverse proxy to configured targets
- Path-based backend routing for single-tunnel mode
- Host-aware backend routing for formal tunnel public hostname fan-out
- Unified logging
- Unified graceful shutdown
- Optional health endpoint
- Real tunnel readiness endpoint

### Out of Scope

- IPv6 pool outbound proxy
- Cloudflare account login flows
- Tunnel creation, deletion, and account management
- Access, DNS, metrics, updater, diagnostic subcommands from `cloudflared`
- Embedded `sing-box`
- Web UI
- Dynamic route or tunnel hot reload
- Cloudflare remote ingress download, parsing, or hot reload
- Cross-tunnel load balancing

## Current Architecture

### Process Model

- One process
- One CLI parser
- One shared logger
- One shared signal handler
- One shared lifecycle manager
- Single-tunnel runner with Quick Tunnel and token-mode session paths
- Optional health runner
- Shared readiness contract for single-tunnel and multi-tunnel modes

### Module Boundaries

- `cmd/app`
  - entrypoint
  - flag parsing
  - lifecycle bootstrap
- `internal/config`
  - config structs
  - defaults
  - validation
- `internal/logging`
  - logger initialization
- `internal/runtime`
  - signal handling
  - shutdown orchestration
- `internal/cftunnel`
  - tunnel runner
  - quick tunnel reservation path
  - formal tunnel token session path
  - edge transport selection
  - origin connector
- `internal/cftunnel/origin`
  - target parsing
  - route matching
  - reverse proxy behavior
  - HTTP/HTTPS/WS/WSS handling
- `internal/health`
  - optional liveness/readiness endpoint
  - `/ready` converts structured tunnel readiness into `200 OK` or `503 Service Unavailable`

## CLI Contract

### Single-Tunnel Mode

```text
--cf-edge-protocol=quic|http2
--cf-tunnel-target=http://127.0.0.1:8080
--cf-tunnel-token=
--cf-origin-server-name=
--cf-origin-insecure-skip-verify=false
--cf-route=/api/*=http://127.0.0.1:9001
--cf-route=/api/*=http://127.0.0.1:9001,host=api.example.com
--cf-route=/secure/*=https://127.0.0.1:9443,server_name=secure.internal
--health-listen=:9090
--log-level=info
--log-format=text
--shutdown-timeout=10s
```

### Validation Rules

- `--cf-tunnel-target` is required in single-tunnel mode.
- `--cf-tunnel-target` must be a full URL with scheme and host.
- `--cf-tunnel-token` or `CF_TUNNEL_TOKEN` enables formal tunnel token mode.
- `--cf-route` supports exact paths such as `/health`, prefix paths such as `/api/*`, and default `/`.
- `--cf-route` may include `host=example.com`; host-specific routes take precedence over path-only fallback routes.
- `--cf-origin-server-name` and `--cf-origin-insecure-skip-verify` apply only to the default `--cf-tunnel-target`.
- `--cf-route` targets use independent TLS options: URL host is the default TLS server name and certificate verification defaults to enabled unless the route specifies otherwise.
- Route precedence is deterministic: host-specific matches before path-only fallback, with exact > longest prefix > default inside each group.
- Invalid or duplicate route rules fail fast at startup.
- `/live` returns `200 OK` while the health server is alive.
- `/ready` returns `200 OK` only when every configured tunnel has completed edge registration; pending, starting, failed, stopped, or exited tunnels return `503 Service Unavailable`.

## Execution Status (2026-05-29)

### Completed

- Quick Tunnel request client and real tunnel startup.
- Edge protocol support for `quic` and `http2`.
- HTTP/HTTPS and WebSocket origin proxying.
- Path-based backend routing for HTTP and WebSocket traffic.
- Route validation and deterministic router precedence.
- Real-link path split validation integrated into the benchmark script.

### Current Verified Baseline

- Local and package-level tests pass with `go test ./...` in the last recorded verification cycle.
- Real-link e2e validation has passed for both `http2` and `quic`.
- Path-based routing checks have passed in e2e with `path_routing_check=pass`.
- Formal tunnel token smoke validation has passed with a real Cloudflare Tunnel token and configured public hostname.
- RSS stayed in the tens-of-MiB range in recorded serial rounds.

## Active Construction Plan

The active plan is `docs/superpowers/plans/2026-05-29-readiness-contract.md`.

### Readiness Contract

#### Goal

Make `/ready` report real tunnel readiness for single-tunnel and multi-tunnel modes instead of treating startup or a static summary string as ready.

#### Remaining Scope

- Add a structured readiness provider in `internal/health`.
- Track single tunnel lifecycle states and mark ready from runtime registration.
- Aggregate multi-tunnel readiness with `ready` and `failed` counts.
- Wire health readiness for single-tunnel and multi-tunnel startup paths.
- Document `/live` versus `/ready` semantics.

## Risks and Controls

- Route rule conflicts may create ambiguous behavior.
  - Control: strict validation and deterministic precedence.
- Network benchmarks are noisy.
  - Control: use serial runs and report the fixed RSS/throughput fields from the e2e scripts.
- Scope expansion risk.
  - Control: keep token-mode phase one separate from remote ingress management, dynamic reload, and optimization work.

## Testing Plan

### Unit Tests

- config validation
- target parsing
- target URL parsing
- route matching and proxy behavior
- cftunnel runtime/session behavior

### Integration / E2E

- Quick Tunnel with local HTTP target
- Quick Tunnel with local WS target
- path-based split routing through a real link
- host-aware local routing
- token-mode session construction
- protocol verification for `http2` and `quic`

### Manual Verification

- run app with single Quick Tunnel mode
- verify public `trycloudflare.com` URLs forward to their configured local targets
- run app with a formal tunnel token
- verify configured Cloudflare public hostnames fan out through local Host/Path routes
