# cf-tunnel Client IP Header Forwarding Design

**Date:** 2026-06-05

**Scope:** Add HTTP origin header forwarding for client IP semantics using `CF-Connecting-IP`, `X-Real-IP`, and `X-Forwarded-For` on the route/origin proxy layer. This scope does not attempt to rewrite socket-level `remote_addr` or add transparent proxy behavior.

## Goal

Expose stable and explicit client IP forwarding behavior for HTTP/HTTPs/WebSocket origin requests so backend applications can consume real client IP information through trusted headers instead of parsing runtime logs or relying on proxy-local `remote_addr`.

## Problem Statement

The current HTTP origin proxy rewrites host and target URL information, but it does not explicitly forward client IP headers to backend origins. As a result:

- backend applications do not receive a consistent real-client-IP signal
- `remote_addr` at the backend remains the local proxy hop, which is expected but often misunderstood
- Cloudflare-originated headers are not normalized or documented as an application-facing contract
- shell scripts and application frameworks cannot rely on a documented forwarding policy

## In Scope

- HTTP/HTTPS/WebSocket origin requests routed through the existing reverse proxy path
- forwarding and normalization policy for:
  - `CF-Connecting-IP`
  - `X-Real-IP`
  - `X-Forwarded-For`
- documented trust boundary for when upstream-supplied headers are accepted
- regression tests for request rewriting behavior
- README documentation for backend operators
- governance CSV for phased execution tracking

## Out of Scope

- rewriting backend socket `remote_addr`
- transparent proxy or TPROXY behavior
- PROXY protocol support
- non-HTTP TCP origin flows
- Cloudflare account policy changes
- custom user-configurable header name mapping in this cycle

## Design Principles

### P1: Headers, Not Socket Identity

This project operates as an application-layer reverse proxy. It may forward real client IP semantics in HTTP headers, but it does not and cannot make backend `remote_addr` equal to the original client IP under the current architecture.

### P2: Preserve Trusted Cloudflare Semantics

If the upstream request already contains a trusted `CF-Connecting-IP`, that value is the highest-priority client IP signal and should be preserved into the backend request.

### P3: Normalize for Common Backend Frameworks

Backends often read one of three fields:

- `CF-Connecting-IP`
- `X-Real-IP`
- `X-Forwarded-For`

The proxy should emit all three consistently so common frameworks can be configured without custom adaptation.

### P4: Do Not Overload `X-Forwarded-For` With Edge-Hop Semantics

The main purpose of `X-Forwarded-For` here is to expose the real client IP chain to the backend application. The final Cloudflare edge hop should not be injected into `X-Forwarded-For` as a primary behavior in this cycle.

## Forwarding Policy

### Incoming Signal Priority

For HTTP/WS origin forwarding, determine the effective client IP in this order:

1. trusted incoming `CF-Connecting-IP`
2. otherwise trusted incoming `X-Forwarded-For` first client element, if present and valid
3. otherwise trusted incoming `X-Real-IP`, if present and valid
4. otherwise no effective client IP is available

This cycle assumes the tunnel-facing request path is trusted Cloudflare-delivered traffic. No additional dynamic trust configuration is introduced.

### Outgoing Header Contract

When an effective client IP is available, set:

- `CF-Connecting-IP: <client-ip>`
- `X-Real-IP: <client-ip>`
- `X-Forwarded-For: <client-ip>` or normalized forwarded chain beginning with `<client-ip>`

When no effective client IP is available:

- do not fabricate a client IP from local proxy `remote_addr`
- preserve current behavior except for explicitly documented normalization rules

### `X-Forwarded-For` Normalization Rule

For this cycle, the simplest and safest rule is:

- if `CF-Connecting-IP` is present and valid, write `X-Forwarded-For` beginning with that client IP
- if a trusted incoming `X-Forwarded-For` chain exists, normalize it so the first element is the effective client IP and preserve remaining valid chain elements
- do not append synthetic Cloudflare edge-hop metadata into `X-Forwarded-For`

## Proposed Implementation Shape

### Proxy Rewrite Layer

Extend the existing request rewrite path in `internal/cftunnel/origin/proxy.go` so request header normalization occurs together with host rewrite logic.

### Helper Boundaries

Create focused helper logic for:

- extracting the effective client IP from trusted incoming headers
- normalizing `X-Forwarded-For`
- applying forwarded IP headers to the backend request

This keeps header policy testable without entangling it with target URL rewrite behavior.

## Engineering Boundaries

- no config flag is added in this cycle
- no user-selectable trust list is added in this cycle
- the policy applies only to HTTP-origin proxying code paths
- the route-splitting layer and default proxy path must share the same behavior
- documentation must explicitly state that backend `remote_addr` still represents the local proxy hop

## Delivery Strategy

### Phase P0: Policy Freeze

**Objective:** Freeze the forwarding policy, trust assumptions, and non-goals.

**Tasks:**

- document header priority order
- document what is and is not changed
- initialize the governance ledger

**Exit Criteria:**

- the forwarding policy is written and unambiguous
- the CSV ledger exists with phases and verification criteria

### Phase P1: HTTP Proxy Header Normalization

**Objective:** Implement a deterministic client IP forwarding policy in the HTTP origin proxy.

**Tasks:**

- extract effective client IP from trusted incoming headers
- normalize outgoing `CF-Connecting-IP`, `X-Real-IP`, and `X-Forwarded-For`
- keep host rewrite behavior intact

**Boundary Rules:**

- do not change non-HTTP proxy paths
- do not introduce edge-hop synthetic headers in this cycle

**Exit Criteria:**

- backend requests receive normalized client IP headers when source information exists
- existing host rewrite logic continues to pass

### Phase P2: Tests and Regression Guards

**Objective:** Lock the forwarding contract with focused tests.

**Tasks:**

- add proxy tests for trusted `CF-Connecting-IP`
- add tests for normalization of `X-Forwarded-For`
- add tests showing `remote_addr` is not used as a fabricated client IP signal

**Boundary Rules:**

- tests must target behavior, not implementation details
- do not weaken current host/header rewrite tests

**Exit Criteria:**

- targeted tests pass for single-hop and chained-header scenarios
- no existing proxy tests regress

### Phase P3: Documentation and Closure

**Objective:** Publish the contract and record execution evidence.

**Tasks:**

- update README with backend guidance
- record validation commands and actual results in the CSV ledger
- close the ledger into terminal states

**Boundary Rules:**

- no additional feature scope in closure
- newly discovered unrelated forwarding ideas are logged separately

**Exit Criteria:**

- docs explain how backends should consume client IP
- all tasks are in terminal states with evidence

## Verification Model

Minimum verification for this cycle:

- targeted proxy tests in `internal/cftunnel/origin`
- package tests for `internal/cftunnel/...`
- full `go test ./...`
- README inspection for forwarding policy clarity

## File Outputs

This design produces:

- spec: `docs/superpowers/specs/2026-06-05-client-ip-forwarding-design.md`
- ledger: `docs/governance/2026-06-05-client-ip-forwarding-task-ledger.csv`
