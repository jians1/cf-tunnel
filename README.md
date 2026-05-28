# cf-quicktunnel-ipv6pool

Single-binary Go project focused on Cloudflare `TryCloudflare / Quick Tunnel`.

The tunnel path is intentionally kept small for personal use: startup requests a real Quick Tunnel, connects to Cloudflare edge with `quic` or `http2`, and proxies traffic to the configured local origin.

## Status

### Working Today

- unified CLI and config validation
- Quick Tunnel request client
- local origin target parsing
- local reverse proxy for HTTP/HTTPS and WebSocket upgrade
- full Quick Tunnel main path using `quic` or `http2`
- VLESS over WebSocket origin compatibility through the WebSocket proxy path

### Verified Externally

- explicit `http2`: public `trycloudflare.com` URL returned the local origin response
- explicit `quic`: public `trycloudflare.com` URL returned the local origin response
- `http2` and `quic`: `1GiB` downloads through a VLESS-over-WebSocket origin completed with matching SHA256
- large download RSS stayed in the tens of MiB range and did not grow with response size

### Known Limits

- Quick Tunnel creation can be rate-limited by `api.trycloudflare.com`.
- Newly-created `trycloudflare.com` hostnames can have a short DNS or edge convergence window; warm up with small requests before large transfers.
- This project targets Quick Tunnel and does not implement named tunnels or account login flows.
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
  --cf-tunnel-target=127.0.0.1:8080 \
  --cf-origin-protocol=http \
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

Optional multi-tunnel real-link regression (disabled by default):

```bash
CI_E2E_MULTI=1 ./scripts/ci.sh
```

When enabled, CI additionally runs:

1. `scripts/e2e/run_trycloudflare_multi_tunnel_real.sh http2 1`
2. `scripts/e2e/run_trycloudflare_multi_tunnel_real.sh quic 1`

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
  --cf-tunnel-target=127.0.0.1:8080 \
  --cf-origin-protocol=http
```

Force a specific Cloudflare edge transport:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=127.0.0.1:8080 \
  --cf-origin-protocol=http
```

For a WebSocket origin such as VLESS over WS:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=127.0.0.1:10000 \
  --cf-origin-protocol=ws
```

Path-based backend split (repeat `--cf-route`):

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=127.0.0.1:8080 \
  --cf-origin-protocol=http \
  --cf-route=/api/*=127.0.0.1:9001 \
  --cf-route=/ws/*=ws://127.0.0.1:10000
```

Multi-tunnel mode (repeat `--cf-tunnel`):

```bash
go run ./cmd/app \
  --cf-tunnel=name=t1,target=127.0.0.1:8081,origin=http,edge=http2 \
  --cf-tunnel=name=t2,target=127.0.0.1:10000,origin=ws,edge=quic
```

Notes:

- In multi-tunnel mode, do not mix single-tunnel flags such as `--cf-tunnel-target`, `--cf-origin-protocol`, `--cf-route`.
- When `--health-listen` is enabled, `/ready` returns a multi-tunnel summary string in multi mode.

## Important Flags

### Global Controls

- `--log-level=debug|info|warn|error`
- `--log-format=text|json`
- `--health-listen=:9090`
- `--shutdown-timeout=10s`

### Tunnel Controls

- `--cf-quick-service=https://api.trycloudflare.com`
- `--cf-edge-protocol=quic|http2`
- `--cf-ha-connections=1`
- `--cf-tunnel-target=host:port|url`
- `--cf-origin-protocol=auto|http|https|ws|wss`
- `--cf-origin-server-name=...`
- `--cf-origin-insecure-skip-verify`
- `--cf-route=/path=host:port|url` (repeatable, supports exact `/health` and prefix `/api/*`)
- `--cf-tunnel=name=<name>,target=<host:port|url>,origin=<auto|http|https|ws|wss>[,edge=<quic|http2>][,quick=<url>][,ha=1][,server_name=<name>][,insecure_skip_verify=true|false]` (repeatable)

## Current Runtime Behavior

- Normal startup creates a real Quick Tunnel through `api.trycloudflare.com`.
- Runtime edge address discovery is internal and automatic.
- Quick Tunnel currently supports `--cf-ha-connections=1` only; larger values are rejected.

If `api.trycloudflare.com` returns Cloudflare rate limiting such as `429` / `1015`, retry later. That failure is at Quick Tunnel API creation time, not necessarily at the local origin proxy path.

## Release

- Version file: [VERSION](/root/cf-quicktunnel-ipv6pool/VERSION)
- Release notes: [RELEASE_NOTES.md](/root/cf-quicktunnel-ipv6pool/RELEASE_NOTES.md)
