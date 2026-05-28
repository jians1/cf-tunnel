# Project Construction Plan

## Goal

Build a single-binary Go application focused on Cloudflare `TryCloudflare / Quick Tunnel`.

The application forwards traffic from a real Quick Tunnel to a configured local origin target via `--cf-tunnel-target`.

## Product Scope

### In Scope

- Single Go binary
- CLI flag based startup model
- Cloudflare Quick Tunnel support
- Edge transport selection: `quic | http2`
- Origin protocol selection: `auto | http | https | ws | wss`
- Local reverse proxy to configured target
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

## Target Architecture

### Process Model

- One process
- One CLI parser
- One shared logger
- One shared signal handler
- One shared lifecycle manager
- One optional runner (`cftunnel`) plus optional health runner

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
  - reverse proxy behavior
  - HTTP/HTTPS/WS/WSS handling
- `internal/health`
  - optional liveness/readiness endpoint

## CLI Contract

```text
--enable-cf-tunnel
--cf-quick-service=https://api.trycloudflare.com
--cf-edge-protocol=quic|http2
--cf-ha-connections=1
--cf-tunnel-target=http://127.0.0.1:8080
--cf-origin-protocol=auto|http|https|ws|wss
--cf-origin-server-name=
--cf-origin-insecure-skip-verify=false
--health-listen=:9090
--log-level=info
--log-format=text
--shutdown-timeout=10s
```

### Validation Rules

- `--enable-cf-tunnel=true` is required.
- `--cf-tunnel-target` is required.
- `--cf-tunnel-target` must be either `host:port` or a full URL.
- If target has no scheme, `--cf-origin-protocol` cannot be `auto`.
- If target has a scheme and `--cf-origin-protocol` is explicit, it must be compatible.
- `--cf-ha-connections` currently supports only `1`.

## Testing Plan

### Unit Tests

- config validation
- target parsing
- origin protocol compatibility
- cftunnel runtime/session behavior

### Integration / E2E

- quick tunnel with local HTTP target
- quick tunnel with local WS target
- protocol verification for `http2` and `quic`

### Manual Verification

- run app with Quick Tunnel only
- verify public `trycloudflare.com` URL forwards to local target

