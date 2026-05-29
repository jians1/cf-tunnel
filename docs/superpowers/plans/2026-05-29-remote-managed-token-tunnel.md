# Remote Managed Token Tunnel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-stage formal Cloudflare Tunnel support using a remote-managed tunnel token while keeping this project's local Host/Path routing layer and avoiding new heavy cloudflared dependency chains.

**Architecture:** Token mode is a second session preparation path next to Quick Tunnel. Quick Tunnel continues to call `api.trycloudflare.com`; token mode decodes `CF_TUNNEL_TOKEN` / `--cf-tunnel-token` into tunnel credentials and reuses the existing QUIC/HTTP2 edge runtime. Public hostname fan-out is handled locally by extending existing route rules with optional `Host`, not by importing Cloudflare remote ingress management.

**Tech Stack:** Go, standard `encoding/base64` and `encoding/json`, existing `github.com/google/uuid`, existing internal `cftunnel/runtime`, existing local origin router.

---

## Overall Task Objective

Build a first-stage formal Cloudflare Tunnel mode for this project. The new mode must let users run this binary with a Cloudflare remote-managed tunnel token, reuse the existing lightweight QUIC/HTTP2 edge runtime, and keep local routing decisions inside this project.

The end state should satisfy these project-level outcomes:

- A valid `CF_TUNNEL_TOKEN` or `--cf-tunnel-token` starts a formal tunnel without calling `api.trycloudflare.com`.
- Existing Quick Tunnel behavior remains compatible and keeps using the current reservation path.
- Multiple Cloudflare public hostnames on one formal tunnel are represented as one connector plus local `Host + Path` routes, not as multiple tunnel sessions.
- The implementation does not import `sing-cloudflared` or restore the full `cloudflared` dependency chain.
- Failure cases are explicit: malformed token, missing credential fields, invalid host route, or broken runtime session must fail visibly.

## Project Scope

In scope:

- Remote-managed token decoding.
- Token-mode session construction.
- Runner branch that skips Quick Tunnel creation when token mode is active.
- Optional `Host` matching in the existing route layer.
- README documentation for token mode and multi-public-hostname routing.

Out of scope for phase one:

- Cloudflare remote ingress download, parsing, hot reload, or dynamic application.
- Tunnel creation, deletion, login, account management, diagnostics, or metrics.
- Replacing the current local route layer with `sing-cloudflared` ingress logic.
- Adding new heavy dependencies beyond standard library and already-used packages.

## Phase Milestones

### Phase 1: Local Routing Model Upgrade

Objective: Make the project capable of distinguishing multiple public hostnames that enter the same formal tunnel.

Deliverables:

- `RouteRule` supports optional `Host`.
- Router matches `Host + Path` before path-only fallback, preserving existing path behavior.
- Existing Quick Tunnel route behavior continues to pass tests.

Exit criteria:

- `go test ./internal/config ./internal/cftunnel/origin -count=1` passes.
- Tests prove host-specific routes and fallback routes both work.

### Phase 2: Formal Tunnel Identity Path

Objective: Add a small, dependency-light way to obtain formal tunnel credentials from a Cloudflare token.

Deliverables:

- Config accepts `--cf-tunnel-token` and `CF_TUNNEL_TOKEN`, with flag overriding env.
- Token decoder parses Cloudflare token fields `a`, `t`, and `s`.
- Malformed or incomplete tokens fail with clear errors.

Exit criteria:

- `go test ./internal/config ./internal/cftunnel/credentials -count=1` passes.
- Dependency diff shows no import of `sing-cloudflared`.

### Phase 3: Runtime Session Integration

Objective: Reuse the current edge runtime with token-derived credentials while keeping Quick Tunnel session semantics intact.

Deliverables:

- Token mode builds a runtime session without requiring Quick Tunnel hostname/public URL.
- Quick Tunnel mode still requires hostname/public URL.
- Upstream adapter validates generic credentials for formal tunnels.

Exit criteria:

- `go test ./internal/cftunnel/runtime -count=1` passes.
- Tests prove token sessions do not fake `trycloudflare.com` URLs.

### Phase 4: Runner Switch and User-Facing Behavior

Objective: Connect the token identity path into application startup.

Deliverables:

- Token mode skips Quick Tunnel reservation API.
- Quick Tunnel mode remains unchanged when no token is provided.
- Startup logs distinguish formal tunnel mode from Quick Tunnel mode.

Exit criteria:

- `go test ./internal/cftunnel ./cmd/app -count=1` passes.
- Tests prove token mode does not call the Quick Tunnel API.

### Phase 5: Documentation and Release Verification

Objective: Make the feature operable and verify it does not regress size or baseline behavior.

Deliverables:

- README documents formal token mode, multi-public-hostname model, and phase-one remote ingress limitation.
- Full test suite and binary build pass.
- Binary size is checked to catch accidental heavy dependency growth.

Exit criteria:

- `go test ./... -count=1` passes.
- `go build -o /tmp/cf-quicktunnel-ipv6pool ./cmd/app` succeeds.
- `ls -lh /tmp/cf-quicktunnel-ipv6pool` is recorded in the final implementation summary.

## File Map

- Modify `internal/config/config.go`: add `TunnelToken` to `CFTunnelConfig`, CLI/env/config parsing, validation, and optional `Host` on `RouteRule`.
- Modify `internal/config/config_test.go`: cover token parsing precedence, token mode validation, and host route validation.
- Modify `internal/cftunnel/origin/router.go`: match route by optional host plus existing path semantics.
- Modify `internal/cftunnel/origin/router_test.go`: cover host-specific routing and fallback path-only routing.
- Create `internal/cftunnel/credentials/token.go`: decode Cloudflare tunnel token into existing credentials type.
- Create `internal/cftunnel/credentials/token_test.go`: valid token and malformed token tests.
- Modify `internal/cftunnel/config/runtime.go`: carry token mode into normalized runtime config without changing Quick Tunnel defaults.
- Modify `internal/cftunnel/runtime/session.go`: split generic credential validation from Quick Tunnel hostname validation and add token session builder.
- Modify `internal/cftunnel/runtime/session_test.go`: cover token session and Quick Tunnel session validation.
- Modify `internal/cftunnel/runtime/upstream_adapter.go`: bind token sessions without requiring Quick Tunnel hostname.
- Modify `internal/cftunnel/runtime/upstream_adapter_test.go`: cover token-mode binding.
- Modify `internal/cftunnel/session_setup.go`: branch between Quick Tunnel reservation and token session preparation.
- Modify `internal/cftunnel/session_setup_test.go`: assert token mode skips Quick Tunnel API and rejects invalid token.
- Modify `internal/cftunnel/summary.go`: log remote-managed token sessions without fake public URL.
- Modify `README.md`: document token mode, multiple public hostname model, and no remote ingress support in phase one.

---

### Task 1: Add Host-Aware Local Route Matching

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cftunnel/origin/router.go`
- Modify: `internal/cftunnel/origin/router_test.go`

- [x] **Step 1: Write failing config tests for route host validation**

Add tests to `internal/config/config_test.go` near existing route tests:

```go
func TestParseRouteAcceptsHostOption(t *testing.T) {
	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-route=/api/*=http://127.0.0.1:9001,host=api.example.com",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 1 {
		t.Fatalf("expected one route, got %d", len(cfg.CFTunnel.Routes))
	}
	if cfg.CFTunnel.Routes[0].Host != "api.example.com" {
		t.Fatalf("unexpected route host %q", cfg.CFTunnel.Routes[0].Host)
	}
}

func TestParseRouteRejectsInvalidHostOption(t *testing.T) {
	_, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-route=/api/*=http://127.0.0.1:9001,host=bad host",
	})
	if err == nil {
		t.Fatal("expected invalid host error")
	}
	if !strings.Contains(err.Error(), "route[0].host") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

Ensure `strings` is imported if not already present.

- [x] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/config -run 'TestParseRouteAcceptsHostOption|TestParseRouteRejectsInvalidHostOption' -count=1
```

Expected: FAIL because `RouteRule.Host` does not exist or `host` option is unsupported.

- [x] **Step 3: Implement host field and validation**

In `internal/config/config.go`, update `RouteRule`:

```go
type RouteRule struct {
	Host                  string
	Path                  string
	Target                string
	OriginServerName      string
	InsecureSkipVerify    bool
	InsecureSkipVerifySet bool
}
```

In `parseRouteTargetOptions`, add option handling:

```go
		case "host":
			route.Host = strings.ToLower(strings.TrimSpace(value))
```

In `validateRouteRules`, validate host before path normalization:

```go
		host := strings.TrimSpace(route.Host)
		if host != "" {
			if err := validateRouteHost(host); err != nil {
				return fmt.Errorf("route[%d].host: %w", i, err)
			}
		}
```

Add helper near route validation:

```go
func validateRouteHost(host string) error {
	if strings.Contains(host, "://") {
		return errors.New("must be a hostname, not a URL")
	}
	if strings.ContainsAny(host, " /\\") {
		return errors.New("must not contain spaces or path separators")
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return errors.New("must not start or end with '.'")
	}
	if strings.Contains(host, "..") {
		return errors.New("must not contain empty labels")
	}
	return nil
}
```

- [x] **Step 4: Write failing router tests for host precedence**

Add to `internal/cftunnel/origin/router_test.go`:

```go
func TestRouterMatchesHostSpecificRouteBeforePathFallback(t *testing.T) {
	router, err := NewRouter(origin.Target{Raw: "http://127.0.0.1:8080"}, []appconfig.RouteRule{
		{Host: "api.example.com", Path: "/api/*", Target: "http://127.0.0.1:9001"},
		{Path: "/api/*", Target: "http://127.0.0.1:9002"},
	})
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	matched := router.Match("api.example.com", "/api/users")
	if matched.Target.Raw != "http://127.0.0.1:9001" {
		t.Fatalf("expected host route, got %q", matched.Target.Raw)
	}

	fallback := router.Match("www.example.com", "/api/users")
	if fallback.Target.Raw != "http://127.0.0.1:9002" {
		t.Fatalf("expected fallback route, got %q", fallback.Target.Raw)
	}
}
```

If current router method signature does not accept host, update the test to call the nearest request-based API and assert the same behavior.

- [x] **Step 5: Run router test to verify failure**

Run:

```bash
go test ./internal/cftunnel/origin -run TestRouterMatchesHostSpecificRouteBeforePathFallback -count=1
```

Expected: FAIL because router does not match by host yet.

- [x] **Step 6: Implement host-aware route matching**

Update `internal/cftunnel/origin/router.go` so each compiled route keeps `Host string`. Matching rule:

```go
func routeHostMatches(routeHost, requestHost string) bool {
	if routeHost == "" {
		return true
	}
	requestHost = strings.ToLower(requestHost)
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = h
	}
	return routeHost == requestHost
}
```

In the route loop, require both host and path to match. Preserve current order, so specific rules should be listed before fallback rules. If existing router exposes `Match(path string)`, change it to `Match(host, path string)` and update call sites in `routed_proxy.go` to pass `r.Host`.

- [x] **Step 7: Run focused tests**

Run:

```bash
go test ./internal/config ./internal/cftunnel/origin -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/cftunnel/origin/router.go internal/cftunnel/origin/router_test.go internal/cftunnel/origin/routed_proxy.go
git commit -m "feat: add host-aware local route matching"
```

---

### Task 2: Add Tunnel Token Configuration Surface

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: Write failing tests for token config**

Add to `internal/config/config_test.go`:

```go
func TestParseTunnelTokenFromFlag(t *testing.T) {
	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-tunnel-token=token-from-flag",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-flag" {
		t.Fatalf("unexpected token %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestParseTunnelTokenFromEnv(t *testing.T) {
	t.Setenv("CF_TUNNEL_TOKEN", "token-from-env")
	cfg, err := Parse([]string{"--cf-tunnel-target=http://127.0.0.1:8080"})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-env" {
		t.Fatalf("unexpected token %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestTunnelTokenFlagOverridesEnv(t *testing.T) {
	t.Setenv("CF_TUNNEL_TOKEN", "token-from-env")
	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-tunnel-token=token-from-flag",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-flag" {
		t.Fatalf("expected flag token to win, got %q", cfg.CFTunnel.TunnelToken)
	}
}
```

- [x] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/config -run 'TestParseTunnelToken' -count=1
```

Expected: FAIL because the flag and field do not exist.

- [x] **Step 3: Implement token config parsing**

In `internal/config/config.go`, add field:

```go
type CFTunnelConfig struct {
	QuickService       string
	TunnelToken        string
	EdgeProtocol       string
	Target             string
	OriginServerName   string
	InsecureSkipVerify bool
	Routes             []RouteRule
}
```

In `Parse`, add:

```go
	cfg.CFTunnel.TunnelToken = strings.TrimSpace(os.Getenv("CF_TUNNEL_TOKEN"))
	fs.StringVar(&cfg.CFTunnel.TunnelToken, "cf-tunnel-token", cfg.CFTunnel.TunnelToken, "Cloudflare remote-managed tunnel token")
```

Keep `Target` required in phase one even in token mode because local origin routing still needs a default origin.

- [x] **Step 4: Run config tests**

Run:

```bash
go test ./internal/config -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add formal tunnel token config"
```

---

### Task 3: Decode Remote-Managed Tunnel Token

**Files:**
- Create: `internal/cftunnel/credentials/token.go`
- Create: `internal/cftunnel/credentials/token_test.go`

- [x] **Step 1: Write failing token tests**

Create `internal/cftunnel/credentials/token_test.go`:

```go
package credentials

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
)

func TestParseTunnelToken(t *testing.T) {
	tunnelID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secret := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	raw := `{"a":"account-tag","t":"` + tunnelID.String() + `","s":"` + secret + `","e":"edge.example.com"}`
	token := base64.StdEncoding.EncodeToString([]byte(raw))

	parsed, err := ParseTunnelToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if parsed.AccountTag != "account-tag" {
		t.Fatalf("unexpected account tag %q", parsed.AccountTag)
	}
	if parsed.TunnelID != tunnelID {
		t.Fatalf("unexpected tunnel id %s", parsed.TunnelID)
	}
	if string(parsed.TunnelSecret) != "secret-value" {
		t.Fatalf("unexpected secret %q", string(parsed.TunnelSecret))
	}
	if parsed.Endpoint != "edge.example.com" {
		t.Fatalf("unexpected endpoint %q", parsed.Endpoint)
	}
}

func TestParseTunnelTokenRejectsMalformedBase64(t *testing.T) {
	_, err := ParseTunnelToken("not base64")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseTunnelTokenRejectsMissingSecret(t *testing.T) {
	raw := `{"a":"account-tag","t":"11111111-1111-1111-1111-111111111111"}`
	token := base64.StdEncoding.EncodeToString([]byte(raw))
	_, err := ParseTunnelToken(token)
	if err == nil {
		t.Fatal("expected missing secret error")
	}
}
```

- [x] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/cftunnel/credentials -run 'TestParseTunnelToken' -count=1
```

Expected: FAIL because `ParseTunnelToken` does not exist.

- [x] **Step 3: Implement token decoder**

Create `internal/cftunnel/credentials/token.go`:

```go
package credentials

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type TunnelToken struct {
	AccountTag   string    `json:"a"`
	TunnelSecret []byte    `json:"s"`
	TunnelID     uuid.UUID `json:"t"`
	Endpoint     string    `json:"e,omitempty"`
}

func ParseTunnelToken(token string) (Credentials, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Credentials{}, errors.New("missing tunnel token")
	}
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return Credentials{}, fmt.Errorf("decode tunnel token: %w", err)
	}
	var parsed TunnelToken
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Credentials{}, fmt.Errorf("unmarshal tunnel token: %w", err)
	}
	if parsed.AccountTag == "" {
		return Credentials{}, errors.New("missing tunnel token account tag")
	}
	if parsed.TunnelID == uuid.Nil {
		return Credentials{}, errors.New("missing tunnel token tunnel id")
	}
	if len(parsed.TunnelSecret) == 0 {
		return Credentials{}, errors.New("missing tunnel token secret")
	}
	return Credentials{
		AccountTag:   parsed.AccountTag,
		TunnelID:     parsed.TunnelID,
		TunnelSecret: append([]byte(nil), parsed.TunnelSecret...),
	}, nil
}
```

If existing `Credentials` already has an `Endpoint` field, copy it too. If it does not, do not add endpoint support in phase one; endpoint is not required for current edge discovery path.

- [x] **Step 4: Run token tests**

Run:

```bash
go test ./internal/cftunnel/credentials -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/cftunnel/credentials/token.go internal/cftunnel/credentials/token_test.go
git commit -m "feat: decode remote managed tunnel token"
```

---

### Task 4: Build Runtime Sessions From Token Credentials

**Files:**
- Modify: `internal/cftunnel/config/runtime.go`
- Modify: `internal/cftunnel/runtime/session.go`
- Modify: `internal/cftunnel/runtime/session_test.go`
- Modify: `internal/cftunnel/runtime/upstream_adapter.go`
- Modify: `internal/cftunnel/runtime/upstream_adapter_test.go`

- [x] **Step 1: Write failing session test for token mode**

Add to `internal/cftunnel/runtime/session_test.go`:

```go
func TestBuildTokenSession(t *testing.T) {
	cfg := tunnelconfig.RuntimeConfig{
		EdgeProtocol:       "quic",
		HAConnections:      1,
		QuickTunnelDefault: false,
		Origin: origin.Target{
			Raw: "http://127.0.0.1:8080",
		},
	}
	creds := credentials.Credentials{
		TunnelID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		AccountTag:   "account-tag",
		TunnelSecret: []byte("secret-value"),
	}

	session, err := BuildTokenSession(cfg, creds)
	if err != nil {
		t.Fatalf("build token session: %v", err)
	}
	if session.QuickTunnel {
		t.Fatal("expected formal tunnel session")
	}
	if session.Hostname != "" || session.PublicURL != "" {
		t.Fatalf("formal token session should not fake public URL: hostname=%q url=%q", session.Hostname, session.PublicURL)
	}
	if session.TunnelID != creds.TunnelID.String() {
		t.Fatalf("unexpected tunnel id %q", session.TunnelID)
	}
}
```

Add imports for `github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/credentials` and `github.com/google/uuid` if missing.

- [x] **Step 2: Run session test to verify failure**

Run:

```bash
go test ./internal/cftunnel/runtime -run TestBuildTokenSession -count=1
```

Expected: FAIL because `BuildTokenSession` does not exist.

- [x] **Step 3: Implement token session builder and validation split**

In `internal/cftunnel/runtime/session.go`, add:

```go
func (s Session) ValidateRequiredCredentialFields() error {
	if s.TunnelID == "" {
		return fmt.Errorf("missing tunnel id")
	}
	if s.AccountTag == "" {
		return fmt.Errorf("missing account tag")
	}
	if len(s.Secret) == 0 {
		return fmt.Errorf("missing tunnel secret")
	}
	return nil
}

func (s Session) ValidateRequiredQuickTunnelFields() error {
	if err := s.ValidateRequiredCredentialFields(); err != nil {
		return err
	}
	if s.Hostname == "" {
		return fmt.Errorf("missing quick tunnel hostname")
	}
	return nil
}
```

Add builder:

```go
func BuildTokenSession(cfg tunnelconfig.RuntimeConfig, creds credentials.Credentials) (Session, error) {
	if creds.TunnelID.String() == "" {
		return Session{}, fmt.Errorf("missing tunnel id")
	}
	session := Session{
		TunnelID:   creds.TunnelID.String(),
		AccountTag: creds.AccountTag,
		Secret:     append([]byte(nil), creds.TunnelSecret...),
		Edge: EdgeSettings{
			Protocol: cfg.EdgeProtocol,
		},
		Origin: OriginSettings{
			RawTarget:            cfg.Origin.Raw,
			URL:                  cfg.Origin.URL.String(),
			Protocol:             cfg.Origin.Protocol,
			ServerName:           cfg.Origin.ServerName,
			InsecureSkipVerify:   cfg.Origin.InsecureSkipVerify,
			WebsocketUpgradeMode: cfg.Origin.WebsocketUpgradeMode,
			Routes:               append([]appconfig.RouteRule(nil), cfg.Routes...),
		},
		QuickTunnel:   false,
		HAConnections: cfg.HAConnections,
	}
	if err := session.ValidateRequiredCredentialFields(); err != nil {
		return Session{}, err
	}
	return session, nil
}
```

Add import for `internal/cftunnel/credentials`.

- [x] **Step 4: Write failing upstream adapter test for token mode**

Add to `internal/cftunnel/runtime/upstream_adapter_test.go`:

```go
func TestUpstreamAdapterBindsFormalTunnelWithoutHostname(t *testing.T) {
	session := Session{
		TunnelID:   "11111111-1111-1111-1111-111111111111",
		AccountTag: "account-tag",
		Secret:     []byte("secret-value"),
		Edge:       EdgeSettings{Protocol: "quic"},
		Origin:     OriginSettings{RawTarget: "http://127.0.0.1:8080"},
	}
	binding, err := NewUpstreamAdapter().Bind(session)
	if err != nil {
		t.Fatalf("bind upstream: %v", err)
	}
	if binding.TunnelProperties.QuickTunnelURL != "" {
		t.Fatalf("expected no quick tunnel url, got %q", binding.TunnelProperties.QuickTunnelURL)
	}
}
```

- [x] **Step 5: Run upstream adapter test to verify failure**

Run:

```bash
go test ./internal/cftunnel/runtime -run TestUpstreamAdapterBindsFormalTunnelWithoutHostname -count=1
```

Expected: FAIL because `Bind` still requires Quick Tunnel hostname.

- [x] **Step 6: Relax upstream adapter validation for non-Quick Tunnel sessions**

In `internal/cftunnel/runtime/upstream_adapter.go`, replace Quick Tunnel validation with generic credentials validation:

```go
	if err := session.ValidateRequiredCredentialFields(); err != nil {
		return nil, err
	}
```

Keep `QuickTunnelURL: session.Hostname`; it will be empty in token mode.

- [x] **Step 7: Run runtime tests**

Run:

```bash
go test ./internal/cftunnel/runtime -count=1
```

Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add internal/cftunnel/config/runtime.go internal/cftunnel/runtime/session.go internal/cftunnel/runtime/session_test.go internal/cftunnel/runtime/upstream_adapter.go internal/cftunnel/runtime/upstream_adapter_test.go
git commit -m "feat: build formal tunnel sessions from token credentials"
```

---

### Task 5: Integrate Token Session Preparation Into Runner

**Files:**
- Modify: `internal/cftunnel/session_setup.go`
- Modify: `internal/cftunnel/session_setup_test.go`
- Modify: `internal/cftunnel/runner.go`
- Modify: `internal/cftunnel/summary.go`

- [x] **Step 1: Write failing session setup test that token skips Quick Tunnel API**

Add to `internal/cftunnel/session_setup_test.go`:

```go
func TestPrepareSessionWithTokenSkipsQuickTunnelReservation(t *testing.T) {
	token := testTunnelToken(t)
	called := false
	prepared, err := prepareTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		TunnelToken:  token,
		EdgeProtocol: config.EdgeProtocolQUIC,
		Target:       "http://127.0.0.1:8080",
	}, slog.Default(), func(context.Context, tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
		called = true
		return nil, errors.New("reservation should not be called")
	})
	if err != nil {
		t.Fatalf("prepare token session: %v", err)
	}
	if called {
		t.Fatal("quick tunnel reservation was called in token mode")
	}
	if prepared.session.QuickTunnel {
		t.Fatal("expected formal tunnel session")
	}
}

func testTunnelToken(t *testing.T) string {
	t.Helper()
	secret := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	raw := `{"a":"account-tag","t":"11111111-1111-1111-1111-111111111111","s":"` + secret + `"}`
	return base64.StdEncoding.EncodeToString([]byte(raw))
}
```

Add imports `encoding/base64` and `errors` if missing.

- [x] **Step 2: Run test to verify failure**

Run:

```bash
go test ./internal/cftunnel -run TestPrepareSessionWithTokenSkipsQuickTunnelReservation -count=1
```

Expected: FAIL because `prepareTunnelSessionWith` does not exist.

- [x] **Step 3: Rename setup function and add token branch**

In `internal/cftunnel/session_setup.go`:

- Keep `prepareQuickTunnelSession` as a compatibility wrapper if tests call it.
- Add `prepareTunnelSession` used by runner.
- Add `prepareTunnelSessionWith` used by tests.

Implementation shape:

```go
func prepareTunnelSession(ctx context.Context, cfg config.CFTunnelConfig, logger *slog.Logger) (*preparedSession, error) {
	return prepareTunnelSessionWith(ctx, cfg, logger, defaultQuickTunnelReservation)
}

func defaultQuickTunnelReservation(ctx context.Context, runtimeConfig tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
	client := api.NewClientWithOptions(runtimeConfig.QuickService, buildUserAgent(), api.ClientOptions{
		Timeout:       runtimeConfig.QuickServiceTimeout,
		RetryBackoffs: runtimeConfig.RetryBackoffs,
	})
	return client.CreateQuickTunnel(ctx)
}

func prepareTunnelSessionWith(ctx context.Context, cfg config.CFTunnelConfig, logger *slog.Logger, reserve quickTunnelReservationFunc) (*preparedSession, error) {
	runtimeConfig, err := tunnelconfig.Normalize(cfg)
	if err != nil {
		return nil, fmt.Errorf("normalize cftunnel config: %w", err)
	}
	if strings.TrimSpace(cfg.TunnelToken) != "" {
		creds, err := credentials.ParseTunnelToken(cfg.TunnelToken)
		if err != nil {
			return nil, fmt.Errorf("parse tunnel token: %w", err)
		}
		session, err := tunnelruntime.BuildTokenSession(runtimeConfig, creds)
		if err != nil {
			return nil, fmt.Errorf("build formal tunnel runtime session: %w", err)
		}
		return &preparedSession{runtimeConfig: runtimeConfig, session: session}, nil
	}
	return prepareQuickTunnelSessionFromRuntime(ctx, runtimeConfig, reserve)
}
```

Use local helper names that fit the file. Import `strings` and `internal/cftunnel/credentials`.

- [x] **Step 4: Update runner to use generic preparation**

In `internal/cftunnel/runner.go`, replace:

```go
	prepared, err := prepareQuickTunnelSession(ctx, r.cfg, r.logger)
```

with:

```go
	prepared, err := prepareTunnelSession(ctx, r.cfg, r.logger)
```

Replace summary call with a generic summary helper from the next step.

- [x] **Step 5: Update summary for formal tunnel mode**

In `internal/cftunnel/summary.go`, preserve existing Quick Tunnel output. Add:

```go
func logTunnelSummary(logger *slog.Logger, protocol string, session tunnelruntime.Session) {
	if session.QuickTunnel {
		logQuickTunnelSummary(logger, protocol, session)
		return
	}
	logger.Info("formal cloudflare tunnel ready",
		"protocol", protocol,
		"tunnel_id", session.TunnelID,
		"ha_connections", session.HAConnections,
	)
}
```

Use this in `runner.go`:

```go
	logTunnelSummary(r.logger, formatProtocol(r.cfg.EdgeProtocol), prepared.session)
```

- [x] **Step 6: Run cftunnel tests**

Run:

```bash
go test ./internal/cftunnel -count=1
```

Expected: PASS.

- [x] **Step 7: Run broader affected tests**

Run:

```bash
go test ./internal/config ./internal/cftunnel/... ./cmd/app -count=1
```

Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add internal/cftunnel/session_setup.go internal/cftunnel/session_setup_test.go internal/cftunnel/runner.go internal/cftunnel/summary.go
git commit -m "feat: run remote managed tunnel from token"
```

---

### Task 6: Document Phase-One Semantics and Verify Build

**Files:**
- Modify: `README.md`
- Optionally modify: `RELEASE_NOTES.md`

- [x] **Step 1: Update README with token mode**

Add a section after Quick Tunnel examples:

```markdown
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

Phase-one token mode does not import Cloudflare remote ingress management. Cloudflare public hostnames route traffic to this connector; local forwarding is still controlled by this project's `Target` and `Routes` configuration.

Multiple public hostnames on one Cloudflare Tunnel should be represented as one tunnel token plus host-aware local routes:

```json
{
  "cf_tunnel": {
    "TunnelToken": "...",
    "EdgeProtocol": "quic",
    "Target": "http://127.0.0.1:8080",
    "Routes": [
      {
        "Host": "api.example.com",
        "Path": "/api/*",
        "Target": "http://127.0.0.1:9001"
      },
      {
        "Host": "ws.example.com",
        "Path": "/*",
        "Target": "ws://127.0.0.1:10000"
      }
    ]
  }
}
```
```

- [x] **Step 2: Update Important Flags**

Add:

```markdown
- `--cf-tunnel-token=...` or `CF_TUNNEL_TOKEN=...` for remote-managed formal tunnel mode
```

Add route note:

```markdown
- `--cf-route` supports `host=<public-hostname>` as an optional route option.
```

- [x] **Step 3: Run full tests**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [x] **Step 4: Build binary and inspect size**

Run:

```bash
go build -o /tmp/cf-quicktunnel-ipv6pool ./cmd/app
ls -lh /tmp/cf-quicktunnel-ipv6pool
```

Expected: build succeeds. Size should not jump from importing `sing-cloudflared`; only standard library and already-used uuid are involved.

- [ ] **Step 5: Optional real token smoke test**

Not run in this pass because no real Cloudflare Tunnel token and configured public hostname were provided.

Only run when a valid Cloudflare Tunnel token and configured public hostname are available:

```bash
CF_TUNNEL_TOKEN='...' go run ./cmd/app \
  --log-level=debug \
  --cf-edge-protocol=quic \
  --cf-tunnel-target=http://127.0.0.1:8080
```

Expected: logs show `formal cloudflare tunnel ready`, no Quick Tunnel API request, and requests to the configured public hostname reach the local origin.

- [x] **Step 6: Commit docs**

```bash
git add README.md RELEASE_NOTES.md
git commit -m "docs: document formal tunnel token mode"
```

---

## Self-Review

- Spec coverage: token mode, no remote ingress, local Host/Path routing, no heavy dependency import, Quick Tunnel compatibility, and verification are covered by Tasks 1-6.
- Placeholder scan: no `TBD`, `TODO`, or unspecified validation steps remain.
- Type consistency: plan consistently uses `TunnelToken`, `RouteRule.Host`, `BuildTokenSession`, and `prepareTunnelSessionWith`.
- Risk called out: Cloudflare remote ingress is intentionally not implemented in phase one; local routing remains the source of truth.
