# cf-tunnel

Single-binary Go project focused on Cloudflare `TryCloudflare / Quick Tunnel` and lightweight remote-managed Tunnel token mode.

`cf-tunnel` 是一个单二进制 Go 项目，聚焦 Cloudflare `TryCloudflare / Quick Tunnel` 以及轻量级 remote-managed Tunnel token 模式。

The tunnel path is intentionally kept small for personal use: startup can request a real Quick Tunnel or use a Cloudflare remote-managed tunnel token, connects to Cloudflare edge with `quic` or `http2`, and proxies traffic to the configured local origin.

隧道路径刻意保持精简，适合个人使用：启动时可以申请真实 Quick Tunnel，也可以使用 Cloudflare remote-managed tunnel token，通过 `quic` 或 `http2` 连接 Cloudflare edge，并把流量转发到配置的本地源站。

## Status / 状态

### Working Today / 当前可用

- unified CLI and config validation / 统一 CLI 和配置校验
- Quick Tunnel request client / Quick Tunnel 申请客户端
- local origin target parsing / 本地源站目标解析
- local reverse proxy for HTTP/HTTPS and WebSocket upgrade / 支持 HTTP/HTTPS 与 WebSocket upgrade 的本地反向代理
- full Quick Tunnel main path using `quic` or `http2` / 基于 `quic` 或 `http2` 的完整 Quick Tunnel 主路径
- remote-managed Cloudflare Tunnel token mode using local routing / 使用本地路由的 remote-managed Cloudflare Tunnel token 模式
- VLESS over WebSocket origin compatibility through the WebSocket proxy path / 通过 WebSocket 代理路径兼容 VLESS over WebSocket 源站

### Verified Externally / 外部验证

- explicit `http2`: public `trycloudflare.com` URL returned the local origin response / 显式 `http2`：公网 `trycloudflare.com` URL 可返回本地源站响应
- explicit `quic`: public `trycloudflare.com` URL returned the local origin response / 显式 `quic`：公网 `trycloudflare.com` URL 可返回本地源站响应
- remote-managed token tunnel: public hostname `test.910666.xyz` returned the local origin response / remote-managed token tunnel：公网 hostname `test.910666.xyz` 可返回本地源站响应
- `http2` and `quic`: `1GiB` downloads through a VLESS-over-WebSocket origin completed with matching SHA256 / `http2` 和 `quic`：通过 VLESS-over-WebSocket 源站完成 `1GiB` 下载且 SHA256 匹配
- `quic`: `256MiB` downloads through both Quick Tunnel and remote-managed token tunnel completed with matching SHA256 / `quic`：Quick Tunnel 与 remote-managed token tunnel 均完成 `256MiB` 下载且 SHA256 匹配
- large download RSS stayed in the tens of MiB range and did not grow with response size / 大文件下载时 RSS 保持在几十 MiB 范围内，不随响应体大小线性增长

Latest `256MiB` RSS smoke results on `linux/amd64` release build:

`linux/amd64` release build 上最新 `256MiB` RSS 冒烟结果：

| Mode / 模式 | Ready RSS | Warm RSS | Peak Download RSS | Final RSS |
|---|---:|---:|---:|---:|
| Remote-managed token tunnel, `quic` | `18,744 KB` | `19,132 KB` | `21,484 KB` | `21,356 KB` |
| Quick Tunnel, `quic` | `16,052 KB` | `16,056 KB` | `22,676 KB` | `22,032 KB` |

### Known Limits / 已知限制

- Quick Tunnel creation can be rate-limited by `api.trycloudflare.com`. / Quick Tunnel 创建可能受到 `api.trycloudflare.com` 限流。
- Newly-created `trycloudflare.com` hostnames can have a short DNS or edge convergence window; warm up with small requests before large transfers. / 新创建的 `trycloudflare.com` hostname 可能存在短暂 DNS 或 edge 收敛窗口；大流量传输前建议先用小请求预热。
- This project does not implement account login flows (supports single-tunnel CLI and optional multi-tunnel config mode). / 本项目不实现账号登录流程（支持单隧道 CLI 和可选的多隧道配置文件模式）。
- Remote-managed token mode does not download Cloudflare remote ingress rules; local `target` and `routes` remain the source of truth. / Remote-managed token 模式不会下载 Cloudflare 远端 ingress 规则；本地 `target` 和 `routes` 仍是转发事实来源。
- Quick Tunnel currently runs with one HA connection in this implementation. / 当前实现中 Quick Tunnel 使用一个 HA connection。

## Build / 构建

```bash
go build -buildvcs=false ./cmd/app
```

Current version / 当前版本：

```text
0.1.0
```

## Release Build / 发布构建

```bash
./scripts/build-release.sh
```

Release builds use the tested compact settings:

发布构建使用已验证的精简参数：

```text
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w"
```

Default output / 默认输出：

```text
dist/cf-tunnel-0.1.0-linux-amd64
dist/cf-tunnel-0.1.0-linux-amd64.sha256
dist/cf-tunnel-0.1.0-linux-amd64.manifest.txt
```

## Container Build / 容器构建

```bash
docker build -t cf-tunnel:0.1.0 .
```

Example run / 运行示例：

```bash
docker run --rm \
  cf-tunnel:0.1.0 \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080 \
  --health-listen=
```

## CI / Local Acceptance / CI 与本地验收

Run the local pipeline:

运行本地流水线：

```bash
./scripts/ci.sh
```

This currently performs:

当前会执行：

1. `go test ./...`
2. compact release binary build into `dist/` / 将精简 release 二进制构建到 `dist/`
3. Docker image build / Docker 镜像构建

## E2E A/B Test / 端到端 A/B 测试

The repository includes real end-to-end Quick Tunnel throughput scripts for `http2` and `quic` against a local `sing-box` VLESS-over-WebSocket origin.

仓库包含真实端到端 Quick Tunnel 吞吐测试脚本，可使用 `http2` 和 `quic` 对本地 `sing-box` VLESS-over-WebSocket 源站进行 A/B 测试。

Single round / 单轮测试：

```bash
./scripts/e2e/run_trycloudflare_ab.sh http2 1
./scripts/e2e/run_trycloudflare_ab.sh quic 1
```

Three-round A/B run / 三轮 A/B 测试：

```bash
./scripts/e2e/run_trycloudflare_ab_3rounds.sh
```

Environment notes / 环境说明：

- requires `sing-box` in `SING_BOX_BIN` or `/root/.local/bin/sing-box` / 需要通过 `SING_BOX_BIN` 指定 `sing-box`，或使用 `/root/.local/bin/sing-box`
- uses `${TMPDIR:-/tmp}/cfqt-e2e/` for temporary files and logs / 使用 `${TMPDIR:-/tmp}/cfqt-e2e/` 存放临时文件和日志
- each round uses dedicated ports and cleans child processes on exit / 每轮使用独立端口，并在退出时清理子进程
- the script includes Quick Tunnel DNS/edge warmup retries before the `1GiB` download starts / 脚本在开始 `1GiB` 下载前包含 Quick Tunnel DNS/edge 预热重试

## Quick Tunnel / Quick Tunnel

Run a local HTTP origin through Quick Tunnel:

通过 Quick Tunnel 暴露本地 HTTP 源站：

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Force a specific Cloudflare edge transport:

强制指定 Cloudflare edge 传输协议：

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

For a WebSocket origin such as VLESS over WS:

用于 WebSocket 源站，例如 VLESS over WS：

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=ws://127.0.0.1:10000
```

Path-based backend split (repeat `--cf-route`):

基于路径拆分后端（可重复使用 `--cf-route`）：

```bash
go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080 \
  --cf-route=/api/*=http://127.0.0.1:9001 \
  --cf-route=/ws/*=ws://127.0.0.1:10000 \
  --cf-route=/secure/*=https://127.0.0.1:9443,server_name=secure.internal
```

Notes / 说明：

- `--cf-origin-server-name` and `--cf-origin-insecure-skip-verify` apply to the default `--cf-tunnel-target` only. / `--cf-origin-server-name` 和 `--cf-origin-insecure-skip-verify` 只作用于默认 `--cf-tunnel-target`。
- Each `--cf-route` target has independent TLS options: append `server_name=...` or `insecure_skip_verify=true` to that route when needed. Without route options, URL host is the TLS server name and certificate verification stays enabled. / 每个 `--cf-route` 目标有独立 TLS 选项：需要时可在该 route 后追加 `server_name=...` 或 `insecure_skip_verify=true`。未设置 route 选项时，URL host 会作为 TLS server name，证书校验保持启用。
- `--cf-route` also supports `host=<public-hostname>` to match one Cloudflare Tunnel connector serving multiple public hostnames. / `--cf-route` 也支持 `host=<public-hostname>`，用于一个 Cloudflare Tunnel connector 服务多个公网 hostname 的场景。

## Formal Cloudflare Tunnel Token Mode / 正式 Cloudflare Tunnel Token 模式

Use a remote-managed Cloudflare Tunnel token when the tunnel is created in Cloudflare Zero Trust:

当隧道已在 Cloudflare Zero Trust 中创建时，可以使用 remote-managed Cloudflare Tunnel token：

```bash
CF_TUNNEL_TOKEN='...' go run ./cmd/app \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Or pass the token explicitly:

也可以显式传入 token：

```bash
go run ./cmd/app \
  --cf-tunnel-token='...' \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Phase-one token mode does not import Cloudflare remote ingress management. Cloudflare public hostnames route traffic to this connector; local forwarding is still controlled by this project's `target` and `routes` configuration.

第一阶段 token 模式不会导入 Cloudflare 远端 ingress 管理规则。Cloudflare 公网 hostname 会把流量路由到这个 connector；本地转发仍由本项目的 `target` 和 `routes` 配置控制。

Multiple public hostnames on one Cloudflare Tunnel should be represented as one tunnel token plus host-aware local routes:

一个 Cloudflare Tunnel 上的多个公网 hostname 应表示为一个 tunnel token 加多条带 host 匹配的本地 route：

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

如果 route 未设置 `host`，它会作为任意 hostname 的纯路径 fallback。

## Optional Config File (Multi-Tunnel) / 可选配置文件（多隧道）

Use `--config=<path>` to load a YAML config file. This is optional and primarily for multi-tunnel setups.

使用 `--config=<path>` 加载 YAML 配置文件。该功能是可选的，主要用于多隧道场景。

Example / 示例：

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

Run / 运行：

```bash
go run ./cmd/app --config=./config.yaml
```

Compatibility rules / 兼容规则：

- Without `--config`, current single-tunnel CLI behavior is unchanged. / 不使用 `--config` 时，当前单隧道 CLI 行为不变。
- With `--config`, file values are applied after CLI flags. / 使用 `--config` 时，配置文件值会在 CLI flags 之后应用。
- If `tunnels` is present and non-empty, runtime starts in multi-tunnel mode. / 如果 `tunnels` 存在且非空，运行时进入多隧道模式。
- Config files must be YAML (`.yaml` or `.yml`) and use `snake_case` fields. JSON files and Go-style fields such as `CFTunnel` or `EdgeProtocol` are rejected. / 配置文件必须是 YAML（`.yaml` 或 `.yml`），并使用 `snake_case` 字段。JSON 文件以及 `CFTunnel`、`EdgeProtocol` 这类 Go 风格字段会被拒绝。
- If `tunnel_token` is stored in YAML, keep the file private, for example `chmod 600 config.yaml`. / 如果把 `tunnel_token` 存在 YAML 中，请保护该文件，例如执行 `chmod 600 config.yaml`。

## Parameters / 参数

Usage / 用法：

```bash
cf-tunnel --cf-tunnel-target=<url> [options]
cf-tunnel --config=<config.yaml>
```

Required / 必填：

- `--cf-tunnel-target=<url>` (required when `--config` is not used) / 未使用 `--config` 时必填

Optional / 可选：

- `--config=<path>`: YAML config file (`.yaml` / `.yml`) / YAML 配置文件（`.yaml` / `.yml`）
- `--cf-edge-protocol=http2|quic` (default: `http2`) / Cloudflare edge 传输协议，默认 `http2`
- `--cf-tunnel-token=...` or `CF_TUNNEL_TOKEN=...` for remote-managed formal tunnel mode / remote-managed 正式 tunnel 模式使用的 token，可通过参数或环境变量提供
- `--cf-origin-server-name=...` / 默认源站 TLS server name
- `--cf-origin-insecure-skip-verify` / 默认源站 TLS 跳过证书校验
- `--cf-route=/path=url[,host=...][,server_name=...][,insecure_skip_verify=true|false]` (repeatable, supports exact `/health` and prefix `/api/*`) / 可重复设置的路由，支持精确路径 `/health` 和前缀路径 `/api/*`
- `--log-level=debug|info|warn|error` / 日志级别
- `--log-format=text|json` / 日志格式
- `--health-listen=:9090` / 健康检查监听地址
- `--shutdown-timeout=10s` / 优雅关闭超时时间

Precedence and override rules / 优先级和覆盖规则：

- Parse order is CLI first, then `--config`. / 解析顺序是先 CLI，再 `--config`。
- If config file contains `cf_tunnel`, it replaces single-tunnel CLI fields as one block. / 如果配置文件包含 `cf_tunnel`，它会作为一个整体替换单隧道 CLI 字段。
- If config file contains non-empty `tunnels`, runtime enters multi-tunnel mode and single-tunnel CLI fields (`--cf-*` for single tunnel) are not used. / 如果配置文件包含非空 `tunnels`，运行时进入多隧道模式，单隧道 CLI 字段（单隧道 `--cf-*`）不会被使用。
- Global controls (`log-level`, `log-format`, `health-listen`, `shutdown-timeout`) are also overridden by config file when the corresponding fields are set. / 全局控制项（`log-level`、`log-format`、`health-listen`、`shutdown-timeout`）在配置文件设置对应字段时也会被覆盖。

## Current Runtime Behavior / 当前运行行为

- Without a tunnel token, normal startup creates a real Quick Tunnel through `api.trycloudflare.com`. / 没有 tunnel token 时，正常启动会通过 `api.trycloudflare.com` 创建真实 Quick Tunnel。
- With `--cf-tunnel-token` or `CF_TUNNEL_TOKEN`, startup skips the Quick Tunnel API and uses the remote-managed tunnel credentials from the token. / 使用 `--cf-tunnel-token` 或 `CF_TUNNEL_TOKEN` 时，启动会跳过 Quick Tunnel API，改用 token 中的 remote-managed tunnel 凭据。
- Runtime edge address discovery is internal and automatic. / 运行时 edge 地址发现是内部自动完成的。

If `api.trycloudflare.com` returns Cloudflare rate limiting such as `429` / `1015`, retry later. That failure is at Quick Tunnel API creation time, not necessarily at the local origin proxy path.

如果 `api.trycloudflare.com` 返回 Cloudflare 限流，例如 `429` / `1015`，请稍后重试。该失败发生在 Quick Tunnel API 创建阶段，不一定代表本地源站代理路径有问题。

## Health Endpoints / 健康检查接口

When `--health-listen` is non-empty, the app exposes:

当 `--health-listen` 非空时，应用会暴露：

- `/live`: process liveness. Returns `200 OK` while the health server is running. / 进程存活检查。健康检查服务运行时返回 `200 OK`。
- `/ready`: tunnel readiness. Returns `200 OK` only when every configured tunnel has completed edge registration. Returns `503 Service Unavailable` while tunnels are pending, starting, failed, stopped, or exited. / 隧道就绪检查。只有所有已配置隧道完成 edge registration 后才返回 `200 OK`；隧道处于 pending、starting、failed、stopped 或 exited 时返回 `503 Service Unavailable`。

Readiness response bodies are concise text summaries:

`/ready` 响应体是简短文本摘要：

```text
mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]
mode=multi total=2 ready=2 failed=0 details=[alpha:ready,beta:ready]
```

## Release / 发布

- Version file / 版本文件: [VERSION](VERSION)
- Release notes / 发布说明: [RELEASE_NOTES.md](RELEASE_NOTES.md)
