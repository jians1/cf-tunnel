# Project Construction Plan

## Goal

Build a single-binary Go application with two independent runtime features:

1. Cloudflare `TryCloudflare / Quick Tunnel`
2. `IPv6 pool` outbound proxy service

Both features must be switchable independently through CLI boolean flags, and may run together in one process.

This project does not embed `sing-box` logic. For the Cloudflare tunnel feature, the application only needs to forward to a configured local target through `--cf-tunnel-target`.

## Product Scope

### In Scope

- Single Go binary
- CLI flag based startup model
- Cloudflare Quick Tunnel support
- Edge transport selection: `auto | quic | http2`
- Origin protocol selection: `auto | http | https | ws | wss`
- Local reverse proxy to configured target
- Independent IPv6 pool HTTP proxy
- Independent IPv6 pool SOCKS5 proxy
- Unified logging
- Unified graceful shutdown
- Optional health endpoint

### Out of Scope

- Named Tunnel management
- Cloudflare account login flows
- Access, DNS, metrics, updater, diagnostic subcommands from `cloudflared`
- Embedded `sing-box`
- Complex config file formats in phase 1
- Web UI

## Target Architecture

The application should be a thin host around two isolated runners.

### Process Model

- One process
- One CLI parser
- One shared logger
- One shared signal handler
- One shared lifecycle manager
- Two optional runners started based on flags

### Module Boundaries

- `cmd/app`
  - entrypoint
  - flag parsing
  - config validation
  - lifecycle bootstrap
- `internal/config`
  - flag-backed config structs
  - defaults
  - validation
- `internal/logging`
  - logger initialization
- `internal/runtime`
  - signal handling
  - errgroup / shutdown orchestration
- `internal/cftunnel`
  - quick tunnel runner
  - edge transport selection
  - origin connector
  - cloudflare integration adapter
- `internal/cftunnel/origin`
  - local target parsing
  - reverse proxy behavior
  - HTTP/HTTPS/WS/WSS handling
- `internal/ipv6pool`
  - HTTP proxy server
  - SOCKS5 proxy server
  - IPv6 address selection strategy
  - outbound dialer
- `internal/health`
  - optional liveness/readiness endpoint

## Recommended Source Strategy

### Cloudflare Tunnel

Do not fully reimplement the Cloudflare tunnel protocol.

Recommended approach:

- fork or vendor the minimum required logic from `cloudflared`
- keep only the Quick Tunnel path and required shared tunnel core
- preserve only `quic` and `http2` edge transport support
- remove unrelated command, management, update, and platform layers

Rationale:

- lower protocol compatibility risk
- smaller maintenance burden than a clean-room implementation
- still allows substantial size reduction compared with full `cloudflared`

### IPv6 Pool

Use `go-proxy-ipv6-pool` as the reference implementation boundary.

Recommended approach:

- reimplement the small IPv6 pool feature set inside `internal/ipv6pool`
- keep it isolated from tunnel logic
- avoid directly coupling its CLI or global state into the application core

Rationale:

- the feature is comparatively small
- internalizing it yields cleaner lifecycle and logging integration
- easier to maintain a single binary and a single config model

## CLI Contract

Use boolean feature toggles and flat flags.

### Core Flags

```text
--enable-cf-tunnel
--cf-edge-protocol=auto
--cf-tunnel-target=http://127.0.0.1:8080
--cf-origin-protocol=auto
--cf-origin-server-name=
--cf-origin-insecure-skip-verify=false

--enable-ipv6-pool
--ipv6-pool-http=:3128
--ipv6-pool-socks5=:3129
--ipv6-pool-bind-interface=
--ipv6-pool-cidr=
--ipv6-pool-strategy=random

--health-listen=:9090
--log-level=info
--log-format=text
```

### Validation Rules

- if `--enable-cf-tunnel=true`, `--cf-tunnel-target` is required
- `--cf-tunnel-target` must be either `host:port` or a full URL
- if `--enable-ipv6-pool=true`, at least one of `--ipv6-pool-http` or `--ipv6-pool-socks5` must be set
- if neither feature is enabled, exit with validation error
- if `--cf-origin-protocol=auto`, infer from target scheme
- if target has no scheme, require explicit `--cf-origin-protocol`
- if `https` or `wss` is used and custom SNI is needed, accept `--cf-origin-server-name`
- if target has a scheme, explicit `--cf-origin-protocol` may only be used as an override and must remain compatible with the target format
- if `--enable-ipv6-pool=true`, require at least one of `--ipv6-pool-cidr` or `--ipv6-pool-bind-interface`

## Config Model

Suggested Go structs:

```go
type AppConfig struct {
	LogLevel     string
	LogFormat    string
	HealthListen string
	CFTunnel     CFTunnelConfig
	IPv6Pool     IPv6PoolConfig
}

type CFTunnelConfig struct {
	Enabled                bool
	EdgeProtocol           string
	Target                 string
	OriginProtocol         string
	OriginServerName       string
	InsecureSkipVerify     bool
}

type IPv6PoolConfig struct {
	Enabled       bool
	HTTPListen    string
	SOCKS5Listen  string
	BindInterface string
	CIDR          string
	Strategy      string
}
```

## Runner Behavior

### Cloudflare Tunnel Runner

Responsibilities:

- create or request Quick Tunnel
- select edge transport: `auto`, `quic`, or `http2`
- forward ingress traffic to the configured local target
- support HTTP and WebSocket style origin forwarding
- expose assigned public tunnel URL in logs
- shut down cleanly on process signal

Non-responsibilities:

- managing `sing-box`
- origin service discovery
- advanced multi-origin routing in phase 1

### IPv6 Pool Runner

Responsibilities:

- start HTTP proxy if configured
- start SOCKS5 proxy if configured
- select outbound IPv6 source addresses from configured pool
- keep implementation independent from Cloudflare tunnel flow
- shut down cleanly on process signal

## Reverse Proxy Requirements

The origin side should support:

- plain HTTP upstream
- HTTPS upstream
- WebSocket upgrade forwarding over HTTP upstream
- secure WebSocket upgrade forwarding over HTTPS upstream

Implementation notes:

- prefer a standard reverse-proxy pattern for HTTP/HTTPS
- explicitly preserve `Upgrade` and WebSocket headers
- keep TLS settings isolated to the origin dialer
- do not implement a separate standalone WebSocket proxy stack unless HTTP upgrade forwarding proves insufficient
- avoid premature support for complex load balancing

## Health and Observability

Phase 1 should include simple operational visibility.

Recommended endpoints:

- `/live`
- `/ready`

Recommended logs:

- enabled feature summary at startup
- quick tunnel assigned URL
- selected edge transport
- configured origin target
- IPv6 pool listeners
- fatal runner errors

## Binary Size Strategy

Main size drivers:

- Go runtime
- TLS stack
- HTTP/2
- QUIC
- Cloudflare tunnel core

Expected ranges:

- minimal tunnel-only build: roughly `10-18 MB`
- tunnel + IPv6 pool: roughly `12-20 MB`

Optimization steps:

- remove unused `cloudflared` packages
- strip symbols with `-ldflags="-s -w"`
- disable debug assets and unused commands
- evaluate UPX only after functional stabilization

## Implementation Phases

### Phase 0: Source Analysis

- inspect `cloudflared` package graph for Quick Tunnel path
- identify minimum packages required for:
  - quick tunnel registration
  - edge session establishment
  - `quic/http2` support
  - local origin proxying
- inspect `go-proxy-ipv6-pool` behavior and isolate reusable concepts

Deliverable:

- dependency map
- keep/remove package list

### Phase 1: Project Skeleton

- create Go module
- add `cmd/app`
- add config, logging, runtime, health scaffolding
- define flag contract and validation

Deliverable:

- runnable empty host process with validated flags

### Phase 2: IPv6 Pool Integration

- implement internal HTTP proxy
- implement internal SOCKS5 proxy
- implement IPv6 source selection logic
- wire graceful shutdown and logs

Deliverable:

- standalone IPv6 pool mode works

### Phase 3: Quick Tunnel Integration

- vendor or port minimal Cloudflare tunnel core
- implement Quick Tunnel startup path
- support `auto/quic/http2`
- proxy requests to configured local target
- print allocated tunnel hostname

Deliverable:

- standalone Quick Tunnel mode works

### Phase 4: Combined Mode

- run both runners in one process
- ensure independent startup and failure behavior
- define policy:
  - fail-fast if one enabled runner cannot start
  - in phase 1, terminate whole process on fatal runner error
  - keep room for a future isolated failure policy if deployment needs partial service survival

Deliverable:

- both features can be enabled together

### Phase 5: Hardening

- improve error messages
- add readiness semantics
- test signal handling
- test TLS origin options
- test WebSocket proxy path
- trim dependencies and binary size

Deliverable:

- release candidate

## Testing Plan

### Unit Tests

- config validation
- protocol inference
- origin target parsing
- IPv6 selection logic

### Integration Tests

- quick tunnel enabled with local HTTP target
- quick tunnel enabled with local WS target
- IPv6 pool HTTP proxy outbound through selected IPv6
- IPv6 pool SOCKS5 outbound through selected IPv6
- combined mode startup and shutdown

### Manual Verification

- run only tunnel mode
- run only IPv6 pool mode
- run both together
- verify tunnel URL forwards to target
- verify proxy egress uses IPv6 addresses from pool

## Risk Register

### Risk 1: Cloudflare Internal Coupling

Quick Tunnel behavior is tightly coupled to upstream `cloudflared` internals and may change.

Mitigation:

- keep adaptation layer thin
- track upstream changes
- avoid clean-room protocol reimplementation

### Risk 2: Binary Size Drift

Keeping too much of `cloudflared` will prevent meaningful size reduction.

Mitigation:

- actively prune command and admin layers
- review dependency graph before implementation settles

### Risk 3: WebSocket Proxy Edge Cases

WS/WSS proxying often fails if header handling is too generic.

Mitigation:

- test upgrade handling explicitly
- keep a dedicated origin adapter for WS/WSS behavior

### Risk 4: IPv6 Environment Variance

IPv6 pool behavior depends on host networking and address availability.

Mitigation:

- require explicit CIDR or interface settings
- fail clearly when the pool cannot be built

### Risk 5: Target Format Ambiguity

Origin target parsing can become error-prone if `host:port`, URL form, and explicit origin protocol overrides are not validated consistently.

Mitigation:

- validate target format before runner startup
- treat scheme-less targets and URL targets differently
- keep `cf-origin-protocol` override rules explicit

## Decisions Locked In

- one binary
- no subcommands
- feature activation through boolean flags
- `cf-tunnel-target` is the only origin target input for the tunnel feature
- `sing-box` is out of scope for this project and treated as an external local upstream
- Cloudflare tunnel and IPv6 pool remain internally independent

## Immediate Next Tasks

The original bootstrap tasks in this section are complete (module skeleton, CLI/config validation, runner orchestration, and initial feature integration).

Current phase priorities:

1. Preserve the current publishable `0.1.0-prototype` baseline and keep release evidence up to date.
2. Continue `third_party/cloudflared` dependency trimming in small, isolated patches only.
3. Verify every trim patch with `go test -count=1 ./...` and `bash scripts/build-release.sh`.
4. Re-run serial real-link e2e (`http2` then `quic`) when protocol-path behavior changes.
5. Stop trimming when size gains flatten or wire-compatibility risk outweighs benefit.
