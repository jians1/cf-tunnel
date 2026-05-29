# Project Construction Plan

## Goal

Build a single-binary Go application focused on Cloudflare `TryCloudflare / Quick Tunnel`.

The application forwards traffic from a real Quick Tunnel endpoint to configured local origin targets. It supports single-tunnel mode with optional path-based backend routing.

## Product Scope

### In Scope

- Single Go binary
- CLI flag based startup model
- Cloudflare Quick Tunnel support
- Edge transport selection: `quic | http2`
- Origin scheme selection through full target URLs: `http | https | ws | wss`
- Local reverse proxy to configured targets
- Path-based backend routing for single-tunnel mode
- Unified logging
- Unified graceful shutdown
- Optional health endpoint

### Out of Scope

- IPv6 pool outbound proxy
- Named Tunnel management
- Cloudflare account login flows
- Access, DNS, metrics, updater, diagnostic subcommands from `cloudflared`
- Embedded `sing-box`
- Web UI
- Dynamic route or tunnel hot reload
- Cross-tunnel load balancing

## Current Architecture

### Process Model

- One process
- One CLI parser
- One shared logger
- One shared signal handler
- One shared lifecycle manager
- Single-tunnel runner
- Optional health runner

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
  - quick tunnel runner
  - edge transport selection
  - origin connector
- `internal/cftunnel/origin`
  - target parsing
  - route matching
  - reverse proxy behavior
  - HTTP/HTTPS/WS/WSS handling
- `internal/health`
  - optional liveness/readiness endpoint

## CLI Contract

### Single-Tunnel Mode

```text
--cf-edge-protocol=quic|http2
--cf-tunnel-target=http://127.0.0.1:8080
--cf-origin-server-name=
--cf-origin-insecure-skip-verify=false
--cf-route=/api/*=http://127.0.0.1:9001
--cf-route=/secure/*=https://127.0.0.1:9443,server_name=secure.internal
--health-listen=:9090
--log-level=info
--log-format=text
--shutdown-timeout=10s
```

### Validation Rules

- `--cf-tunnel-target` is required in single-tunnel mode.
- `--cf-tunnel-target` must be a full URL with scheme and host.
- `--cf-route` supports exact paths such as `/health`, prefix paths such as `/api/*`, and default `/`.
- `--cf-origin-server-name` and `--cf-origin-insecure-skip-verify` apply only to the default `--cf-tunnel-target`.
- `--cf-route` targets use independent TLS options: URL host is the default TLS server name and certificate verification defaults to enabled unless the route specifies otherwise.
- Route precedence is deterministic: exact > longest prefix > default.
- Invalid or duplicate route rules fail fast at startup.

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
- RSS stayed in the tens-of-MiB range in recorded serial rounds.

## Active Construction Plan

### Phase B: Configuration File Evaluation

#### Goal

Evaluate an optional configuration file for complex topologies without reintroducing complex multi-tunnel CLI syntax.

#### Remaining Scope

- Keep the CLI focused on single Quick Tunnel startup.
- Decide whether a config file is needed for future multi-tunnel use.
- If added, keep config-file semantics separate from the current CLI contract.

## Risks and Controls

- Route rule conflicts may create ambiguous behavior.
  - Control: strict validation and deterministic precedence.
- Network benchmarks are noisy.
  - Control: use serial runs and report the fixed RSS/throughput fields from the e2e scripts.
- Scope expansion risk.
  - Control: keep Phase B hardening separate from optimization or feature work.

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
- protocol verification for `http2` and `quic`

### Manual Verification

- run app with single Quick Tunnel mode
- verify public `trycloudflare.com` URLs forward to their configured local targets
