# Release Notes

## 0.1.0

This is the first usable release candidate of `cf-tunnel`.

### Included

- single-binary CLI entrypoint
- Quick Tunnel request client
- remote-managed Cloudflare Tunnel token mode
- local origin target parsing and reverse proxying
- host-aware local route matching for multiple public hostnames on one connector
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
- remote-managed token tunnel to Cloudflare public hostname `test.910666.xyz`
- `256MiB` file downloads through both Quick Tunnel and remote-managed token tunnel with matching SHA256
- `256MiB` RSS smoke: token tunnel peak `21,484 KB`, Quick Tunnel peak `22,676 KB`
- dependency-trimmed runtime still passes real `http2` and `quic` end-to-end regression checks
- release binary size is `10,260,642 bytes` for `linux/amd64`

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

- `third_party/cloudflared/tunnelrpc/proto`
- `zombiezen.com/go/capnproto2`

### Known Limits

- normal full Quick Tunnel startup may be rate-limited by `api.trycloudflare.com`
- newly-created Quick Tunnel hostnames can have a short DNS or edge convergence window
- named tunnels and account login flows are not implemented
- this implementation currently supports one Quick Tunnel HA connection
- token mode does not download or apply Cloudflare remote ingress rules; configure local `target` and `routes`

### Recommended Usage

Use normal Quick Tunnel startup directly:

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Or use a remote-managed Cloudflare Tunnel token:

```bash
CF_TUNNEL_TOKEN='...' go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```
