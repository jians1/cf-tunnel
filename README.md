# cf-tunnel

单二进制 Go 工具，用于把本地 HTTP/HTTPS/WebSocket 源站暴露到 Cloudflare：

- **Quick Tunnel**（`trycloudflare.com`，无需账号）
- **正式 Tunnel Token**（Cloudflare Zero Trust 里已创建的 remote-managed tunnel）

支持 `quic` / `http2` 连接 edge，支持单隧道 CLI、YAML 配置文件，以及多隧道并行。

当前版本：`0.1.0`（见 [VERSION](VERSION)）

---

## 目录

- [快速开始](#快速开始)
- [示例配置](#示例配置)
- [怎么选启动方式](#怎么选启动方式)
- [CLI 参数](#cli-参数)
- [YAML 配置参考](#yaml-配置参考)
- [常见场景示例](#常见场景示例)
- [CLI 与 YAML 对照](#cli-与-yaml-对照)
- [覆盖规则](#覆盖规则)
- [健康检查与状态接口](#健康检查与状态接口)
- [客户端 IP 透传](#客户端-ip-透传)
- [构建与发布](#构建与发布)
- [已知限制](#已知限制)

---

## 快速开始

### 1. 构建

```bash
go build -buildvcs=false -o cf-tunnel ./cmd/app
```

### 2. 最简 CLI（Quick Tunnel）

把本机 `8080` 暴露出去：

```bash
./cf-tunnel --cf-tunnel-target=http://127.0.0.1:8080
```

默认使用 `http2`。想用 `quic`：

```bash
./cf-tunnel \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

启动后日志里会出现 `*.trycloudflare.com` 公网 URL。

### 3. 最简配置文件

可直接使用仓库示例：

```bash
./cf-tunnel --config=./examples/quick-tunnel.yaml
```

或自建 `config.yaml`：

```yaml
cf_tunnel:
  edge_protocol: quic
  target: http://127.0.0.1:8080
```

```bash
./cf-tunnel --config=./config.yaml
```

---

## 示例配置

仓库 [`examples/`](examples/) 目录提供可直接改的模板：

| 文件 | 场景 |
|------|------|
| [examples/quick-tunnel.yaml](examples/quick-tunnel.yaml) | 单隧道 Quick Tunnel（最常用） |
| [examples/token-with-routes.yaml](examples/token-with-routes.yaml) | 正式 token + 多 hostname / 路径路由 |
| [examples/multi-tunnel.yaml](examples/multi-tunnel.yaml) | 进程内并行多条隧道 |

```bash
# Quick Tunnel
./cf-tunnel --config=./examples/quick-tunnel.yaml

# 正式 token（先改文件里的 tunnel_token，并建议 chmod 600）
chmod 600 ./examples/token-with-routes.yaml
./cf-tunnel --config=./examples/token-with-routes.yaml

# 多隧道
./cf-tunnel --config=./examples/multi-tunnel.yaml
```

完整字段说明见下方 [YAML 配置参考](#yaml-配置参考)。

---

## 怎么选启动方式

| 场景 | 推荐方式 |
|------|----------|
| 临时暴露一个本地端口 | CLI：`--cf-tunnel-target` |
| 长期跑、参数较多 | 配置文件：根级 `cf_tunnel` |
| 已有 Cloudflare 正式隧道 | `tunnel_token` + 本地 `target`/`routes` |
| 一台机器跑多条隧道 | 配置文件：`tunnels` 列表 |
| 一个 token 上多个公网域名 | 一条隧道 + 带 `host` 的 `routes` |

要点：

- **没有 token** → 走 Quick Tunnel API 申请临时域名
- **有 token** → 跳过 Quick Tunnel，使用 Zero Trust 里的正式隧道
- **Token 模式不会下载 Cloudflare 远端 ingress**；本地转发只认本项目的 `target` 和 `routes`

---

## CLI 参数

### 用法

```bash
cf-tunnel --cf-tunnel-target=<url> [options]
cf-tunnel --config=<config.yaml>
```

### 必填

| 参数 | 说明 |
|------|------|
| `--cf-tunnel-target=<url>` | 默认源站。未使用 `--config` 时必填 |

源站 URL 需带 scheme 和 host，例如：

- `http://127.0.0.1:8080`
- `https://127.0.0.1:8443`
- `ws://127.0.0.1:10000`
- `wss://127.0.0.1:10000`

### 可选

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--config=<path>` | 空 | YAML 配置文件（`.yaml` / `.yml`） |
| `--cf-edge-protocol=http2\|quic` | `http2` | Cloudflare edge 传输协议 |
| `--cf-ha-connections=<n>` | `4` | HA edge 连接数，范围 `1`–`256` |
| `--cf-tunnel-token=<token>` | 环境变量 `CF_TUNNEL_TOKEN` | 正式 remote-managed tunnel token |
| `--cf-origin-server-name=<name>` | 空 | 默认源站 TLS SNI |
| `--cf-origin-insecure-skip-verify` | `false` | 默认源站跳过 TLS 证书校验 |
| `--cf-route=...` | 无 | 可重复；路径路由，见下方格式 |
| `--log-level=debug\|info\|warn\|error` | `info` | 日志级别 |
| `--log-format=text\|json` | `text` | 日志格式 |
| `--health-listen=<addr>` | `:9090` | 健康检查监听；设为空可关闭 |
| `--shutdown-timeout=<duration>` | `10s` | 优雅关闭超时 |

### `--cf-route` 格式

```text
--cf-route=/path=url[,host=<hostname>][,strip_path_prefix=true|false][,origin_server_name=<name>][,origin_insecure_skip_verify=true|false]
```

示例：

```bash
./cf-tunnel \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080 \
  --cf-route=/api/*=http://127.0.0.1:9001,strip_path_prefix=true \
  --cf-route=/ws/*=ws://127.0.0.1:10000 \
  --cf-route=/secure/*=https://127.0.0.1:9443,origin_server_name=secure.internal
```

说明：

- 路径支持精确匹配（`/health`）和前缀匹配（`/api/*`）
- `--cf-origin-server-name` / `--cf-origin-insecure-skip-verify` **只作用于默认** `--cf-tunnel-target`
- 每个 route 可单独写 TLS 选项；目标 URL 本身含逗号时，请把选项放在最后
- 未写 route TLS 选项时：用 URL 的 host 作 SNI，并开启证书校验
- `strip_path_prefix=true`：转发前去掉匹配前缀，例如 `/api/users?x=1` → `/users?x=1`（默认 `false`）
- `host=<公网 hostname>`：同一 connector 服务多个公网域名时使用

命名约定：

- CLI：`kebab-case`（如 `--cf-origin-server-name`）
- YAML：`snake_case`（如 `origin_server_name`）
- route 子选项使用完整键名：`strip_path_prefix`、`origin_server_name`、`origin_insecure_skip_verify`（不接受旧缩写）

---

## YAML 配置参考

配置文件必须是 YAML（`.yaml` / `.yml`），字段使用 `snake_case`。  
JSON、以及 `CFTunnel` / `EdgeProtocol` 这类 Go 风格字段名会被拒绝。

### 顶层字段

| 字段 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| `log_level` | string | `info` | 否 | `debug` / `info` / `warn` / `error` |
| `log_format` | string | `text` | 否 | `text` / `json` |
| `health_listen` | string | `:9090` | 否 | 健康检查监听地址 |
| `shutdown_timeout` | duration | `10s` | 否 | 如 `10s`、`1m` |
| `cf_tunnel` | object | — | 二选一* | 单隧道配置 |
| `tunnels` | list | — | 二选一* | 多隧道列表；非空则进入多隧道模式 |

\* 使用 `--config` 时，最终仍须能解析出有效隧道：根级 `cf_tunnel`，或非空 `tunnels`。每条隧道都必须有 `target`。

### `cf_tunnel` / `tunnels[].cf_tunnel` 字段

| 字段 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| `target` | string | — | **是** | 默认源站 URL |
| `edge_protocol` | string | `http2` | 否 | `http2` 或 `quic` |
| `ha_connections` | int | `4` | 否 | `1`–`256` |
| `tunnel_token` | string | 空 | 否 | 正式隧道 token；省略则走 Quick Tunnel |
| `origin_server_name` | string | 空 | 否 | 默认源站 TLS SNI |
| `origin_insecure_skip_verify` | bool | `false` | 否 | 默认源站跳过证书校验 |
| `routes` | list | 空 | 否 | 路径/主机路由 |
| `quick_service` | string | `https://api.trycloudflare.com` | 否 | Quick Tunnel API；一般不用改 |

### `tunnels[]` 字段

| 字段 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| `name` | string | — | **是** | 隧道名称，全局唯一 |
| `cf_tunnel` | object | — | **是** | 同上表 |

### `routes[]` 字段

| 字段 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| `path` | string | — | **是** | 如 `/health` 或 `/api/*` |
| `target` | string | — | **是** | 该路由源站 URL |
| `host` | string | 空 | 否 | 公网 hostname；省略则任意 host 都可匹配该 path |
| `strip_path_prefix` | bool | `false` | 否 | 转发前去掉路径前缀 |
| `origin_server_name` | string | 空 | 否 | 该路由 TLS SNI |
| `origin_insecure_skip_verify` | bool | 未设置 | 否 | 该路由是否跳过证书校验 |

路由匹配注意：

- 未设置 `host` 的 route 是任意 hostname 上的路径 fallback
- 同一 `host + path` 不可重复
- 前缀通配只允许尾部 `/*` 形式

### Token 安全

若把 `tunnel_token` 写进 YAML，请限制文件权限，例如：

```bash
chmod 600 config.yaml
```

---

## 常见场景示例

### Quick Tunnel（CLI）

```bash
./cf-tunnel \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

WebSocket 源站（如 VLESS over WS）：

```bash
./cf-tunnel \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=ws://127.0.0.1:10000
```

### 正式 Tunnel Token（CLI）

```bash
export CF_TUNNEL_TOKEN='...'
./cf-tunnel \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

或：

```bash
./cf-tunnel \
  --cf-tunnel-token='...' \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

### 单隧道配置文件

完整模板见 [examples/quick-tunnel.yaml](examples/quick-tunnel.yaml)。

```yaml
log_level: info
health_listen: ":9090"
shutdown_timeout: 10s

cf_tunnel:
  edge_protocol: quic
  ha_connections: 4
  target: http://127.0.0.1:8080
```

```bash
./cf-tunnel --config=./examples/quick-tunnel.yaml
```

### Token + 多 hostname 路由

一个 Cloudflare Tunnel token，本地按公网域名拆到不同后端。完整模板见 [examples/token-with-routes.yaml](examples/token-with-routes.yaml)。

```yaml
cf_tunnel:
  tunnel_token: "..."
  edge_protocol: quic
  ha_connections: 4
  target: http://127.0.0.1:8080
  routes:
    - host: api.example.com
      path: /api/*
      target: http://127.0.0.1:9001
      strip_path_prefix: true
    - host: ws.example.com
      path: /ws/*
      target: ws://127.0.0.1:10000
```

Cloudflare 侧把各 hostname 指到该 tunnel connector；本进程只按本地 `routes` 转发。

### 多隧道配置文件

完整模板见 [examples/multi-tunnel.yaml](examples/multi-tunnel.yaml)。

```yaml
health_listen: ":9090"
shutdown_timeout: 10s

tunnels:
  - name: alpha
    cf_tunnel:
      edge_protocol: quic
      ha_connections: 4
      target: http://127.0.0.1:8081

  - name: beta
    cf_tunnel:
      tunnel_token: "..."
      edge_protocol: http2
      ha_connections: 4
      target: ws://127.0.0.1:10000
      routes:
        - host: test.example.com
          path: /ws/*
          target: ws://127.0.0.1:10000
```

```bash
./cf-tunnel --config=./examples/multi-tunnel.yaml
```

---

## CLI 与 YAML 对照

| CLI | YAML | 作用域 |
|-----|------|--------|
| `--log-level` | `log_level` | 全局 |
| `--log-format` | `log_format` | 全局 |
| `--health-listen` | `health_listen` | 全局 |
| `--shutdown-timeout` | `shutdown_timeout` | 全局 |
| `--cf-tunnel-target` | `cf_tunnel.target` | 单隧道 |
| `--cf-edge-protocol` | `cf_tunnel.edge_protocol` | 单隧道 |
| `--cf-ha-connections` | `cf_tunnel.ha_connections` | 单隧道 |
| `--cf-tunnel-token` / `CF_TUNNEL_TOKEN` | `cf_tunnel.tunnel_token` | 单隧道 |
| `--cf-origin-server-name` | `cf_tunnel.origin_server_name` | 单隧道 |
| `--cf-origin-insecure-skip-verify` | `cf_tunnel.origin_insecure_skip_verify` | 单隧道 |
| `--cf-route=...` | `cf_tunnel.routes[]` | 单隧道 |
| （无直接 CLI） | `cf_tunnel.quick_service` | 单隧道 |
| （无直接 CLI） | `tunnels[].name` / `tunnels[].cf_tunnel` | 多隧道 |

多隧道只能通过配置文件的 `tunnels` 列表启用。

---

## 覆盖规则

1. 解析顺序：**先 CLI，再 `--config`**。
2. 配置文件含 `cf_tunnel` 时：**整块替换**单隧道相关 CLI 字段（不是逐字段 merge）。
3. 配置文件含**非空** `tunnels` 时：进入多隧道模式，单隧道 CLI 的 `--cf-*` **不再使用**。
4. 全局项（`log_level`、`log_format`、`health_listen`、`shutdown_timeout`）在配置文件写了对应字段时会被覆盖。
5. 不使用 `--config` 时，行为与纯 CLI 一致。

---

## 健康检查与状态接口

当 `health_listen` / `--health-listen` 非空时提供：

| 路径 | 含义 |
|------|------|
| `GET /live` | 进程存活；服务在跑即 `200` |
| `GET /ready` | 隧道就绪；**所有**已配置隧道完成 edge registration 才 `200`，否则 `503` |
| `GET /status` | 结构化 JSON 状态（含 Quick Tunnel URL 等） |

`/ready` 文本摘要示例：

```text
mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]
mode=multi total=2 ready=2 failed=0 details=[alpha:ready,beta:ready]
```

`/status` 单隧道示例：

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

```bash
curl -fsS http://127.0.0.1:9090/status
curl -fsS http://127.0.0.1:9090/status | jq -r '.tunnel.quick_tunnel_url'
```

隧道状态包括：`pending`、`starting`、`ready`、`failed`、`stopped`、`exited` 等。若运行时未接入 readiness provider，`/ready` 会返回 `503`，避免误报健康。

---

## 客户端 IP 透传

后端若要读真实客户端 IP，应信任转发头，而不是 socket `remote_addr`。

代理会向 HTTP/HTTPS/WebSocket 源站透传并规范化：

- `CF-Connecting-IP`
- `X-Real-IP`
- `X-Forwarded-For`

取值优先级：

1. `CF-Connecting-IP`
2. `X-Forwarded-For` 中第一个有效客户端 IP
3. `X-Real-IP`

不会把本地代理地址伪造成客户端 IP，也不会改写 backend 的 `remote_addr`。

---

## 构建与发布

### 本地构建

```bash
go build -buildvcs=false -o cf-tunnel ./cmd/app
```

### 发布构建

```bash
./scripts/build-release.sh
```

等价于精简参数：

```text
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w"
```

默认产物：

```text
dist/cf-tunnel-0.1.0-linux-amd64
dist/cf-tunnel-0.1.0-linux-amd64.sha256
dist/cf-tunnel-0.1.0-linux-amd64.manifest.txt
```

### 本地验收

```bash
./scripts/ci.sh
```

会执行：

1. `go test ./...`
2. 精简 release 二进制输出到 `dist/`

项目当前不提供 Docker 镜像。

### 端到端脚本（可选）

仓库含对真实 Quick Tunnel 的吞吐 A/B 脚本（需本地 `sing-box`）：

```bash
./scripts/e2e/run_trycloudflare_ab.sh http2 1
./scripts/e2e/run_trycloudflare_ab.sh quic 1
./scripts/e2e/run_trycloudflare_ab_3rounds.sh
```

- `sing-box`：`SING_BOX_BIN` 或 `/root/.local/bin/sing-box`
- 临时目录：`${TMPDIR:-/tmp}/cfqt-e2e/`

### 版本与说明

- 版本：[VERSION](VERSION)
- 发布说明：[RELEASE_NOTES.md](RELEASE_NOTES.md)

工具链基线：Go `1.26.3`

---

## 已知限制

- Quick Tunnel 创建可能被 `api.trycloudflare.com` 限流（如 `429` / `1015`），请稍后重试。
- 新 `trycloudflare.com` hostname 可能有短暂 DNS / edge 收敛窗口；大流量前建议先用小请求预热。
- 不实现 Cloudflare 账号登录；只用 CLI 或配置文件。
- Token 模式**不**拉取远端 ingress；本地 `target` / `routes` 才是转发真相。
- 默认 `4` 条 HA connection；每条独立连 edge。瞬时故障（如 `EDUPCONN`、edge 断开）会换 edge 并退避重连，进程一般不退出，Quick Tunnel 域名也不会因此更换。
- 仅当单条连接**连续**重连超过上限（默认 5 次）仍失败时，进程才退出，交给 systemd 等外部管理重启。
- Edge 地址发现在运行时内部自动完成，无需手动配置。
