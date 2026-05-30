# cf-quicktunnel-ipv6pool

Single-binary Go project focused on Cloudflare `TryCloudflare / Quick Tunnel` and lightweight remote-managed Tunnel token mode.

The tunnel path is intentionally kept small for personal use: startup can request a real Quick Tunnel or use a Cloudflare remote-managed tunnel token, connects to Cloudflare edge with `quic` or `http2`, and proxies traffic to the configured local origin.

## Status

### Working Today

- unified CLI and config validation
- Quick Tunnel request client
- local origin target parsing
- local reverse proxy for HTTP/HTTPS and WebSocket upgrade
- full Quick Tunnel main path using `quic` or `http2`
- remote-managed Cloudflare Tunnel token mode using local routing
- VLESS over WebSocket origin compatibility through the WebSocket proxy path

### Verified Externally

- explicit `http2`: public `trycloudflare.com` URL returned the local origin response
- explicit `quic`: public `trycloudflare.com` URL returned the local origin response
- remote-managed token tunnel: public hostname `test.910666.xyz` returned the local origin response
- `http2` and `quic`: `1GiB` downloads through a VLESS-over-WebSocket origin completed with matching SHA256
- `quic`: `256MiB` downloads through both Quick Tunnel and remote-managed token tunnel completed with matching SHA256
- large download RSS stayed in the tens of MiB range and did not grow with response size

Latest `256MiB` RSS smoke results on `linux/amd64` release build:

| Mode | Ready RSS | Warm RSS | Peak Download RSS | Final RSS |
|---|---:|---:|---:|---:|
| Remote-managed token tunnel, `quic` | `18,744 KB` | `19,132 KB` | `21,484 KB` | `21,356 KB` |
| Quick Tunnel, `quic` | `16,052 KB` | `16,056 KB` | `22,676 KB` | `22,032 KB` |

### Known Limits

- Quick Tunnel creation can be rate-limited by `api.trycloudflare.com`.
- Newly-created `trycloudflare.com` hostnames can have a short DNS or edge convergence window; warm up with small requests before large transfers.
- This project does not implement account login flows (supports single-tunnel CLI and optional multi-tunnel config mode).
- Remote-managed token mode does not download Cloudflare remote ingress rules; local `target` and `routes` remain the source of truth.
- Quick Tunnel currently runs with one HA connection in this implementation.

## Build

```bash
go build -buildvcs=false ./cmd/app
```

Current prototype version:

```text
0.1.0-prototype
```

## Release Build

```bash
./scripts/build-release.sh
```

Release builds use the tested compact settings:

```text
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w"
```

Default output:

```text
dist/cf-quicktunnel-ipv6pool-0.1.0-prototype-linux-amd64
dist/cf-quicktunnel-ipv6pool-0.1.0-prototype-linux-amd64.sha256
dist/cf-quicktunnel-ipv6pool-0.1.0-prototype-linux-amd64.manifest.txt
```

## Container Build

```bash
docker build -t cf-quicktunnel-ipv6pool:0.1.0-prototype .
```

Example run:

```bash
docker run --rm \
  cf-quicktunnel-ipv6pool:0.1.0-prototype \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080 \
  --health-listen=
```

## CI / Local Acceptance

Run the local pipeline:

```bash
./scripts/ci.sh
```

This currently performs:

1. `go test ./...`
2. compact release binary build into `dist/`
3. Docker image build

## E2E A/B Test

The repository includes real end-to-end Quick Tunnel throughput scripts for `http2` and `quic` against a local `sing-box` VLESS-over-WebSocket origin.

Single round:

```bash
./scripts/e2e/run_trycloudflare_ab.sh http2 1
./scripts/e2e/run_trycloudflare_ab.sh quic 1
```

Three-round A/B run:

```bash
./scripts/e2e/run_trycloudflare_ab_3rounds.sh
```

Environment notes:

- requires `sing-box` in `SING_BOX_BIN` or `/root/.local/bin/sing-box`
- uses `${TMPDIR:-/tmp}/cfqt-e2e/` for temporary files and logs
- each round uses dedicated ports and cleans child processes on exit
- the script includes Quick Tunnel DNS/edge warmup retries before the `1GiB` download starts

## Quick Tunnel

Run a local HTTP origin through Quick Tunnel:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Force a specific Cloudflare edge transport:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

For a WebSocket origin such as VLESS over WS:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=ws://127.0.0.1:10000
```

Path-based backend split (repeat `--cf-route`):

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080 \
  --cf-route=/api/*=http://127.0.0.1:9001 \
  --cf-route=/ws/*=ws://127.0.0.1:10000 \
  --cf-route=/secure/*=https://127.0.0.1:9443,server_name=secure.internal
```

Notes:

- `--cf-origin-server-name` and `--cf-origin-insecure-skip-verify` apply to the default `--cf-tunnel-target` only.
- Each `--cf-route` target has independent TLS options: append `server_name=...` or `insecure_skip_verify=true` to that route when needed. Without route options, URL host is the TLS server name and certificate verification stays enabled.
- `--cf-route` also supports `host=<public-hostname>` to match one Cloudflare Tunnel connector serving multiple public hostnames.

## Formal Cloudflare Tunnel Token Mode

Use a remote-managed Cloudflare Tunnel token when the tunnel is created in Cloudflare Zero Trust:

```bash
CF_TUNNEL_TOKEN='...' go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Or pass the token explicitly:

```bash
go run ./cmd/app \
  --cf-tunnel-token='...' \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Phase-one token mode does not import Cloudflare remote ingress management. Cloudflare public hostnames route traffic to this connector; local forwarding is still controlled by this project's `target` and `routes` configuration.

Multiple public hostnames on one Cloudflare Tunnel should be represented as one tunnel token plus host-aware local routes:

```yaml
cf_tunnel:
  tunnel_token: "..."
  edge_protocol: quic
  target: http://127.0.0.1:8080
  routes:
    - host: api.example.com
      path: /api/*
      target: http://127.0.0.1:9001
    - host: ws.example.com
      path: /ws/*
      target: ws://127.0.0.1:10000
```

If no `host` is set on a route, it remains a path-only fallback for any hostname.

## Optional Config File (Multi-Tunnel)

Use `--config=<path>` to load a YAML config file. This is optional and primarily for multi-tunnel setups.

Example:

```yaml
health_listen: ":9090"
shutdown_timeout: 10s

tunnels:
  - name: alpha
    cf_tunnel:
      edge_protocol: quic
      target: http://127.0.0.1:8081

  - name: beta
    cf_tunnel:
      tunnel_token: "..."
      edge_protocol: http2
      target: ws://127.0.0.1:10000
      routes:
        - host: test.910666.xyz
          path: /ws/*
          target: ws://127.0.0.1:10000
```

Run:

```bash
go run ./cmd/app --config=./config.yaml
```

Compatibility rules:

- Without `--config`, current single-tunnel CLI behavior is unchanged.
- With `--config`, file values are applied after CLI flags.
- If `tunnels` is present and non-empty, runtime starts in multi-tunnel mode.
- Config files must be YAML (`.yaml` or `.yml`) and use `snake_case` fields. JSON files and Go-style fields such as `CFTunnel` or `EdgeProtocol` are rejected.
- If `tunnel_token` is stored in YAML, keep the file private, for example `chmod 600 config.yaml`.

## Important Flags

### Global Controls

- `--log-level=debug|info|warn|error`
- `--log-format=text|json`
- `--health-listen=:9090`
- `--shutdown-timeout=10s`

### Tunnel Controls

- `--cf-edge-protocol=http2|quic` (default: `http2`)
- `--cf-tunnel-token=...` or `CF_TUNNEL_TOKEN=...` for remote-managed formal tunnel mode
- `--cf-tunnel-target=url`
- `--cf-origin-server-name=...`
- `--cf-origin-insecure-skip-verify`
- `--cf-route=/path=url[,host=...][,server_name=...][,insecure_skip_verify=true|false]` (repeatable, supports exact `/health` and prefix `/api/*`)

## Current Runtime Behavior

- Without a tunnel token, normal startup creates a real Quick Tunnel through `api.trycloudflare.com`.
- With `--cf-tunnel-token` or `CF_TUNNEL_TOKEN`, startup skips the Quick Tunnel API and uses the remote-managed tunnel credentials from the token.
- Runtime edge address discovery is internal and automatic.

If `api.trycloudflare.com` returns Cloudflare rate limiting such as `429` / `1015`, retry later. That failure is at Quick Tunnel API creation time, not necessarily at the local origin proxy path.

## Health Endpoints

When `--health-listen` is non-empty, the app exposes:

- `/live`: process liveness. Returns `200 OK` while the health server is running.
- `/ready`: tunnel readiness. Returns `200 OK` only when every configured tunnel has completed edge registration. Returns `503 Service Unavailable` while tunnels are pending, starting, failed, stopped, or exited.

Readiness response bodies are concise text summaries:

```text
mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]
mode=multi total=2 ready=2 failed=0 details=[alpha:ready,beta:ready]
```

## Release

- Version file: [VERSION](/root/cf-quicktunnel-ipv6pool/VERSION)
- Release notes: [RELEASE_NOTES.md](/root/cf-quicktunnel-ipv6pool/RELEASE_NOTES.md)
