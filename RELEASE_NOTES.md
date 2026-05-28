# Release Notes

## 0.1.0-prototype

This is the first usable release candidate of `cf-quicktunnel-ipv6pool`.

### Included

- single-binary CLI entrypoint
- IPv6 pool HTTP proxy
- IPv6 pool SOCKS5 proxy
- Quick Tunnel request client
- local origin target parsing and reverse proxying
- full Quick Tunnel main path using `quic` or `http2`
- WebSocket origin proxying for WS-based protocols
- compact release build using `CGO_ENABLED=0`, `-buildvcs=false`, `-trimpath`, and `-ldflags="-s -w"`

### Verified

- explicit `http2` Quick Tunnel to local origin
- explicit `quic` Quick Tunnel to local origin
- `auto` Quick Tunnel path selecting `quic`
- local WebSocket origin response through the tunnel path
- sing-box VLESS-over-WebSocket origin through both `http2` and `quic`
- `1GiB` file downloads through both `http2` and `quic` with matching SHA256
- dependency-trimmed runtime still passes real `http2` and `quic` end-to-end regression checks
- release binary size reduced to `9,744,546 bytes` for `linux/amd64`

### Dependency Trimming Included

- removed management-path production dependencies including `go-jose`
- removed command-only production dependencies including `urfave/cli` and `fsnotify`
- removed production-path dependencies on:
  - `hello`
  - `ipaccess`
  - `socks`
  - `otel`
  - `gorilla/websocket`
  - `cloudflared/connection`
  - `cloudflared/tunnelrpc`
  - `cloudflared/tunnelrpc/quic`
  - `cloudflared/tunnelrpc/metrics`

### Remaining Large Runtime Surface

The main remaining protocol/runtime weight is concentrated in:

- `third_party/cloudflared/tunnelrpc/pogs`
- `third_party/cloudflared/tunnelrpc/proto`
- `zombiezen.com/go/capnproto2`

### Known Limits

- normal full Quick Tunnel startup may be rate-limited by `api.trycloudflare.com`
- newly-created Quick Tunnel hostnames can have a short DNS or edge convergence window
- named tunnels and account login flows are not implemented
- this implementation currently supports one Quick Tunnel HA connection

### Recommended Usage

Use normal Quick Tunnel startup directly:

```bash
go run ./cmd/app \
  --enable-cf-tunnel \
  --cf-edge-protocol=auto \
  --cf-tunnel-target=127.0.0.1:8080 \
  --cf-origin-protocol=http
```
