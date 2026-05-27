# Cloudflare Quick Tunnel Analysis

## Goal

Identify the minimum viable integration boundary for adding real Cloudflare `TryCloudflare / Quick Tunnel` support into this project without copying the full `cloudflared` command application.

Analysis source:

- upstream repo cloned at `/root/.cache/cloudflared-upstream`
- branch state at clone time: upstream default branch on `2026-05-26`

## High-Level Conclusion

Quick Tunnel is not implemented as an isolated reusable package inside `cloudflared`.

The upstream flow is:

1. `cmd/cloudflared/tunnel/quick_tunnel.go`
2. `cmd/cloudflared/tunnel/StartServer(...)`
3. `prepareTunnelConfig(...)`
4. `supervisor.StartTunnelDaemon(...)`
5. `connection`, `orchestration`, `ingress`, `edgediscovery`, `quic/http2`, `tunnelrpc`

This means a direct "copy just one file" approach will not work.

The correct strategy is:

- reimplement the Quick Tunnel request step locally
- reuse or port the minimum tunnel runtime packages from upstream
- avoid importing the full `cmd/cloudflared/tunnel` command layer

## Verified Upstream Entry Points

### Quick Tunnel Request

Primary upstream file:

- `/root/.cache/cloudflared-upstream/cmd/cloudflared/tunnel/quick_tunnel.go`

What it does:

- POSTs to `https://api.trycloudflare.com/tunnel`
- parses the returned tunnel ID, account tag, secret, hostname
- constructs `connection.Credentials`
- forces default protocol to `quic` when not explicitly set
- forces `ha-connections=1`
- calls `StartServer(...)`

### Main Server Startup

Primary upstream file:

- `/root/.cache/cloudflared-upstream/cmd/cloudflared/tunnel/cmd.go`

Key function:

- `StartServer(...)`

What it does:

- prepares tunnel config
- sets up observer, metrics, management, diagnostics, orchestrator
- disables ICMP router for quick tunnels
- starts supervisor daemon

### Tunnel Configuration Assembly

Primary upstream file:

- `/root/.cache/cloudflared-upstream/cmd/cloudflared/tunnel/configuration.go`

Key function:

- `prepareTunnelConfig(...)`

What it does:

- builds client config
- parses ingress/origin config
- sets protocol selector
- builds edge TLS configs for `quic` and `http2`
- prepares origin dialer and DNS services
- produces `supervisor.TunnelConfig`

## Important Architectural Finding

The upstream Quick Tunnel path is embedded in CLI-heavy code, but the true runtime core is lower in the stack.

This gives us a practical split:

### Keep the Behavior

- Quick Tunnel API request
- tunnel credential assembly
- single-connection policy for quick tunnels
- edge transport selection: `auto | quic | http2`
- local origin forwarding

### Avoid Porting as-is

- `urfave/cli` command layer
- account login flows
- named tunnel CRUD
- autoupdater
- systemd integration
- sentry init
- diagnostic endpoints
- management plane features not required for quick tunnels
- broad config file compatibility layer

## Dependency Surface Observed

The upstream package `./cmd/cloudflared/tunnel` pulls a very large graph, including:

- `cmd/cloudflared/updater`
- `metrics`
- `diagnostic`
- `management`
- `prechecks`
- `token`
- `validation`
- `proxydns`
- `signal`

This is too large to vendor wholesale into this project.

## Minimum Runtime Packages Likely Needed

These are the upstream areas most likely required for a real Quick Tunnel implementation.

### Tier 1: Core Keep Candidates

- `connection`
- `supervisor`
- `ingress`
- `edgediscovery`
- `client`
- `tlsconfig`
- `tunnelrpc`
- `quic`
- `quic/v3`
- `retry`
- `features`
- `connection/dialopts`

Reason:

- these appear on the critical path for edge connection setup, protocol negotiation, origin proxying, and tunnel supervision

### Tier 2: Likely Partial Keep or Selective Port

- `orchestration`
- `ingress/origins`
- `websocket`
- `stream`
- `socks`
- `flow`
- `datagramsession`

Reason:

- some of these may be required indirectly by the origin/proxy path or by the supervisor runtime
- they should be evaluated individually during extraction

### Tier 3: Strong Delete Candidates for Phase 1

- `cmd/cloudflared/updater`
- `diagnostic`
- `management`
- `metrics`
- `prechecks`
- `cfapi`
- `credentials` user-login flows
- `token`
- `validation`
- `cmd/cloudflared/proxydns`
- account-facing tunnel creation/list/delete helpers

Reason:

- not needed for the target product scope
- high dependency cost
- mostly command/application concerns rather than tunnel runtime concerns

## Quick Tunnel Request Can Be Reimplemented Cleanly

The upstream `quick_tunnel.go` logic is small and should be reimplemented locally instead of imported.

Local implementation should do:

1. POST to configured quick-service endpoint
2. decode:
   - tunnel ID
   - account tag
   - secret
   - hostname
3. build local credential struct
4. force single connection for quick tunnel mode
5. hand off to extracted tunnel runtime

This part does not justify importing upstream CLI code.

## Recommended Local Module Split

Suggested internal layout for the tunnel feature:

- `internal/cftunnel/api`
  - quick tunnel request/response client
- `internal/cftunnel/credentials`
  - local tunnel credential struct conversion
- `internal/cftunnel/runtime`
  - wrapper around extracted upstream runtime pieces
- `internal/cftunnel/origin`
  - target parsing
  - HTTP/HTTPS origin adapter
  - WebSocket upgrade forwarding
- `internal/cftunnel/config`
  - tunnel-specific normalization from app config

## Recommended Extraction Strategy

### Phase A

Do not import `cmd/cloudflared/tunnel`.

Instead:

- copy the Quick Tunnel HTTP request logic into local code
- create a local adapter config model

### Phase B

Prototype a minimal runtime bridge using upstream lower-level packages only.

Start from:

- `connection`
- `supervisor`
- `ingress`
- `edgediscovery`

If those still drag too much command-layer setup, selectively port:

- the minimal parts of `prepareTunnelConfig(...)`
- only the config assembly required for:
  - edge protocol selection
  - edge TLS
  - one local origin target
  - one connection

### Phase C

Delete or stub nonessential subsystems early:

- metrics
- management
- diagnostics
- updater
- prechecks
- system integrations

This keeps the extraction honest and prevents silently rebuilding most of `cloudflared`.

## Binary Size Implication

This analysis reinforces the earlier estimate:

- if we accidentally keep most of `StartServer(...)`, binary size will drift upward quickly
- meaningful size reduction only happens if command/application subsystems are cut, not just flags

Expected direction:

- local Quick Tunnel request + extracted runtime core is still realistic
- importing full upstream command path is not

## Risks Confirmed

### Risk 1: Runtime Coupling

Even below the CLI layer, `supervisor`, `connection`, `ingress`, and `orchestration` are tightly coupled.

Effect:

- extraction will be iterative
- some local adapter shims will be needed

### Risk 2: Hidden Feature Drag

Metrics, management, and diagnostics are woven into upstream startup.

Effect:

- if not explicitly cut, they will come along by accident

### Risk 3: Origin Path Complexity

The origin side is not only plain HTTP forwarding; upstream ingress also includes DNS, TCP, virtual services, and reserved services.

Effect:

- we must keep our scope narrow: one target, HTTP/HTTPS origin, WebSocket upgrade support

## Immediate Implementation Plan

1. Add a local `internal/cftunnel/api` package that implements the Quick Tunnel request.
2. Add local response and credential structs matching the upstream API contract.
3. Start a runtime spike that tries to assemble the minimum tunnel config without importing upstream CLI packages.
4. Document the first concrete package subset that compiles inside this repo.

## Current Decision

Proceed with:

- local reimplementation of Quick Tunnel API request
- selective extraction of lower-level tunnel runtime

Do not proceed with:

- importing or vendoring the full `cmd/cloudflared/tunnel` package tree as the production integration boundary
