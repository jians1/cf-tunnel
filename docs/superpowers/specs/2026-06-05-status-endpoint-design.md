# cf-tunnel Status Endpoint Design

**Date:** 2026-06-05

**Scope:** Add a structured `/status` endpoint on the existing `--health-listen` HTTP server so shell scripts and operators can read runtime tunnel metadata without scraping logs.

## Goal

Expose a stable JSON status surface for single-tunnel and multi-tunnel runtime state, including Quick Tunnel URLs when available, while keeping `/live` and `/ready` semantics unchanged.

## In Scope

- reuse the existing `--health-listen` listener
- add `GET /status`
- define a structured JSON schema for single and multi tunnel modes
- expose readiness summary and runtime tunnel metadata through a dedicated status provider
- include Quick Tunnel URL and hostname when available
- keep token-mode tunnels represented with empty quick-tunnel fields
- update tests and README examples for `/status`

## Out of Scope

- new listener flags
- write/control endpoints
- changing `/live` or `/ready` response semantics
- persisting status to disk
- auth or remote-access policy changes beyond current health listener behavior

## Design

### Listener Model

`/status` is served by the existing health server. If `--health-listen` is empty, no health or status endpoints are exposed.

### Endpoint Contract

- `GET /live`
  - unchanged
- `GET /ready`
  - unchanged plain-text readiness summary and status code behavior
- `GET /status`
  - returns `application/json`
  - returns `200 OK`
  - returns current in-memory status snapshot even when readiness is false

### Status Schema

Single tunnel response:

```json
{
  "mode": "single",
  "ready": true,
  "summary": "mode=single total=1 ready=1 failed=0 details=[cftunnel:ready]",
  "tunnel": {
    "name": "cftunnel",
    "status": "ready",
    "quick_tunnel": true,
    "quick_tunnel_url": "https://demo.trycloudflare.com",
    "hostname": "demo.trycloudflare.com",
    "protocol": "quic",
    "origin_url": "http://127.0.0.1:8080"
  }
}
```

Multi tunnel response:

```json
{
  "mode": "multi",
  "ready": true,
  "summary": "mode=multi total=2 ready=2 failed=0 details=[alpha:ready,beta:ready]",
  "tunnels": [
    {
      "name": "alpha",
      "status": "ready",
      "quick_tunnel": true,
      "quick_tunnel_url": "https://alpha.trycloudflare.com",
      "hostname": "alpha.trycloudflare.com",
      "protocol": "quic",
      "origin_url": "http://127.0.0.1:8081"
    },
    {
      "name": "beta",
      "status": "ready",
      "quick_tunnel": false,
      "quick_tunnel_url": "",
      "hostname": "",
      "protocol": "http2",
      "origin_url": "http://127.0.0.1:8082"
    }
  ]
}
```

### Internal Model

Add a health-facing status provider interface alongside the existing ready provider.

Health package owns the transport schema for `/status`.
Tunnel runners provide immutable snapshots with enough metadata for external consumers.

### Runner Behavior

Single runner:
- track current tunnel status string as today
- additionally track the latest session-derived metadata after prepare succeeds
- expose status even before connected: pending/starting with origin and protocol if known, quick-tunnel URL only once assigned

Multi runner:
- aggregate child runner snapshots
- expose one item per tunnel name
- readiness continues to derive from existing state map semantics

### Error Handling

- `/status` returns a JSON snapshot even when tunnels are not ready
- if no status provider is wired, return a minimal not-ready JSON payload instead of failing
- serialization errors return `500`, though none are expected from the fixed schema

## Engineering Rules

- `/ready` remains text-only for compatibility
- `/status` is the only structured runtime metadata endpoint in this cycle
- no log scraping assumptions should remain in new script examples
- JSON fields use snake_case for external consistency

## Verification

- health unit tests cover `/status` shape and default behavior
- runner tests cover single and multi snapshot contents
- `go test ./internal/health ./internal/cftunnel/...`
- README examples show `curl` usage against `/status`
