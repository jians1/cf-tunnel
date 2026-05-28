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

- `--cf-tunnel-target` is required.
- `--cf-tunnel-target` must be either `host:port` or a full URL.
- If target has no scheme, `--cf-origin-protocol` cannot be `auto`.
- If target has a scheme and `--cf-origin-protocol` is explicit, it must be compatible.
- `--cf-ha-connections` currently supports only `1`.

## Next Construction Phases

### Priority Order

1. Add path-based backend routing compatibility layer (Nginx-like path split).
2. Add multi-tunnel creation and runtime management.

### Phase A: Path-Based Backend Routing (First)

#### Goal

Allow one public Quick Tunnel endpoint to dispatch requests to different local backends by URL path.

#### Scope

- Add route table config for exact and prefix matching.
- Route HTTP and WebSocket traffic by path to different origin targets.
- Keep current single default origin as fallback route.
- Preserve existing headers and current proxy behavior.

#### Out of Scope (A)

- Regex routing.
- Dynamic config hot reload.
- Weighted load balancing.

#### Delivery Milestones (A)

1. Config model and validation for route rules.
2. Router component in `internal/cftunnel/origin` with deterministic match priority.
3. Integration into current proxy entry path with zero behavior change when no route rules are configured.
4. Unit tests for matching/precedence/error cases plus e2e smoke test.

#### Task Breakdown (A, Ready to Execute)

##### Task A1: Route Config Modeling

**Goal**

Define route rule data structures and integrate them into existing app config without changing current startup behavior when no rules are configured.

**Files**

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Steps**

1. Add route config structs (rule list, path, target) in `internal/config/config.go`.
2. Attach route config to the existing cftunnel config tree.
3. Keep legacy single-target behavior as default when route list is empty.
4. Add config tests for empty route set and required route fields.

**Acceptance**

- Config can parse route list structure.
- Empty routes do not affect existing single-target mode.

**Verification**

- `go test ./internal/config -v`

##### Task A2: Route Rule Validation and Conflict Detection

**Goal**

Guarantee deterministic and safe routing semantics at startup by rejecting invalid or ambiguous route rules.

**Files**

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Steps**

1. Validate route path grammar:
   - exact path: `/health`
   - prefix path: `/api/*`
   - default path: `/`
2. Reject invalid wildcard forms (for example `*`, `/api*`, `/a*b`).
3. Reject duplicates for exact rules and normalized prefix rules.
4. Add tests for valid combinations and all rejection cases.

**Acceptance**

- Invalid rules fail validation with explicit error text.
- No ambiguous duplicate exact/prefix rules can pass validation.

**Verification**

- `go test ./internal/config -v`

##### Task A3: Router Component (Exact > Longest Prefix > Default)

**Goal**

Implement a dedicated path router in origin module that selects backend target deterministically for each request path.

**Files**

- Create: `internal/cftunnel/origin/router.go`
- Create: `internal/cftunnel/origin/router_test.go`

**Steps**

1. Implement route compilation from config rules into immutable runtime entries.
2. Implement `Match(path)` with precedence:
   - exact match first
   - then longest prefix match
   - then default `/`
3. Return explicit build error on invalid compiled state.
4. Add unit tests for precedence and no-match fallback behavior.

**Acceptance**

- Matching order is deterministic and test-covered.
- Exact route overrides prefix route for same path.

**Verification**

- `go test ./internal/cftunnel/origin -run Router -v`

##### Task A4: Proxy Integration (HTTP + WS)

**Goal**

Integrate router into existing origin forwarding path so that backend selection happens per request path while preserving existing proxy behavior.

**Files**

- Modify: `internal/cftunnel/origin/proxy.go`
- Modify: `internal/cftunnel/runtime/upstream_origin_proxy.go`
- Modify: `internal/cftunnel/session_setup.go`

**Steps**

1. Build router during cftunnel session setup.
2. Inject router into origin proxy runtime object.
3. Resolve target by request path before forwarding both HTTP and WS requests.
4. Keep current headers/upgrade behavior unchanged.
5. Fail fast at startup if router build fails (no silent fallback).

**Acceptance**

- Requests route to target backend by path.
- No-routes configuration behaves exactly like current version.

**Verification**

- `go test ./internal/cftunnel/... -v`

##### Task A5: Path Split Smoke Tests

**Goal**

Verify end-to-end path-based split for HTTP and WebSocket traffic through current cftunnel request path.

**Files**

- Modify: `internal/cftunnel/main_path_mock_test.go`
- Modify (if needed by current test style): `internal/cftunnel/runtime/*_test.go`

**Steps**

1. Add two local test backends:
   - backend A for `/api/*`
   - backend B for `/ws/*`
2. Assert `/api/...` reaches backend A with response marker.

## Execution Status (2026-05-29)

### Phase A Delivery Status

- A1 done: route config model landed in `internal/config`.
- A2 done: route grammar/duplicate validation landed with tests.
- A3 done: deterministic router (`exact > longest-prefix > default`) landed with tests.
- A4 done: HTTP + WebSocket path-based backend dispatch integrated in runtime path.
- A5 done: smoke tests and real-link e2e path-split validation are now part of current scripts.

### Current Verified Baseline

- Local and package-level tests pass (`go test ./...`).
- Real-link e2e validation passes for both `http2` and `quic`.
- Path-based routing checks pass in e2e (`path_routing_check=pass`).

## Next Construction Phase (Priority 2)

### Phase B: Multi-Tunnel Creation and Runtime Management

#### Goal

Run multiple Quick Tunnel instances in one process with shared lifecycle and deterministic startup/shutdown behavior.

#### Scope

- Add config model for multiple tunnel entries.
- Build per-tunnel runtime sessions with isolated origin/routing config.
- Start/stop tunnels under one lifecycle manager.
- Keep unified logging while adding per-tunnel identifiers.

#### Out of Scope (B)

- Dynamic hot-reload of tunnel definitions.
- Cross-tunnel load balancing policy.
- Named tunnel/account login management.

#### Task Breakdown (B, Proposed)

##### Task B1: Config Model for Multi-Tunnel

- Add `[]CFTunnelConfig` style top-level structure or equivalent CLI/file mapping.
- Enforce unique tunnel names/ids and deterministic validation errors.

##### Task B2: Runner Refactor for Multi-Instance

- Extract single-tunnel startup into reusable per-instance unit.
- Add orchestrator to start all configured tunnels and aggregate run errors.

##### Task B3: Lifecycle and Shutdown Semantics

- Define fail-fast vs fail-continue startup policy (explicit and test-covered).
- Ensure graceful shutdown ordering and timeout handling across instances.

##### Task B4: Observability and Ops Safety

- Add per-tunnel log fields.
- Add minimal health/readiness surface for multi-instance visibility.

##### Task B5: Verification

- Unit tests for config and lifecycle orchestration.
- Integration smoke test with at least two active tunnels.
- Real-link serial validation confirming no regression for single-tunnel mode.
3. Assert websocket upgrade and message exchange on `/ws/...` reaches backend B.
4. Assert unmatched path falls back to default backend.

**Acceptance**

- HTTP route split passes.
- WS route split passes.
- Default fallback is verified.

**Verification**

- `go test ./internal/cftunnel -run Path -v`

##### Task A6: Documentation Update

**Goal**

Document route configuration contract and operational behavior so the feature is directly usable after release.

**Files**

- Modify: `README.md`
- Modify: `PROJECT_PLAN.md`

**Steps**

1. Add route config examples (exact, prefix, default).
2. Document precedence: exact > longest prefix > default.
3. Document current limits (no regex, no hot reload, no LB).
4. Add troubleshooting note: startup fails on route conflicts by design.

**Acceptance**

- Examples match actual parser/validator behavior.
- README and plan are consistent with code and CLI.

**Verification**

- `go test ./...`

#### Acceptance (A)

- Same binary can proxy `/api/*` and `/ws/*` to different local targets.
- Existing single-target startup remains backward compatible.
- All existing tests pass and new routing tests pass.

#### Definition of Done (A)

- Route config supports exact/prefix/default rules.
- Matching precedence is deterministic: exact > longest prefix > default.
- Invalid/ambiguous route rules fail fast at startup.
- HTTP and WS split routing are covered by automated tests.
- `go test ./...` passes.
- README includes usage examples and routing precedence.

### Phase B: Multi-Tunnel Creation (Second)

#### Goal

Run multiple Quick Tunnel sessions in one process to improve isolation and resilience for multi-service scenarios.

#### Scope

- Add tunnel count and per-tunnel session tracking.
- Create multiple Quick Tunnel reservations concurrently with bounded retry behavior.
- Start and supervise one runtime instance per session.
- Report each tunnel hostname/url in startup summary logs.

#### Out of Scope (B)

- Cross-tunnel traffic balancing policies.
- Dynamic auto-scaling tunnel count.
- Named tunnel/account login flows.

#### Delivery Milestones (B)

1. Session list model and lifecycle orchestration refactor.
2. Parallel reservation creation with failure policy (fail-fast in first version).
3. Runtime fan-out runner and shutdown coordination.
4. Unit tests for partial failures and context cancel/shutdown behavior.

#### Acceptance (B)

- Configured tunnel count creates that number of independent `trycloudflare.com` hostnames.
- Process shutdown remains graceful and bounded by `--shutdown-timeout`.
- Failures are explicit in logs, no silent fallback to single-tunnel mode.

### Risks and Controls

- Quick Tunnel API rate limits may block multi-tunnel bootstrap.
  - Control: bounded retries + explicit failure surfacing + staged rollout (small tunnel count first).
- Route rule conflicts may create ambiguous behavior.
  - Control: strict validation and deterministic precedence (exact > longest prefix > default).
- Scope expansion risk.
  - Control: ship Phase A minimal set first, then Phase B with fail-fast policy.

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
