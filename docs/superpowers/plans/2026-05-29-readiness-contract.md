# Readiness Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/ready` report real tunnel readiness for single-tunnel and multi-tunnel modes instead of treating startup or a static summary string as ready.

**Architecture:** Introduce one explicit readiness contract at the health boundary: a provider returns `Ready bool` plus a human-readable summary. Tunnel runners own their lifecycle state and expose snapshots; the health runner only converts snapshots into HTTP status and response body. Runtime connection registration should mark a tunnel ready through the existing `ConnectedFuse` hook instead of inferring readiness from goroutine startup.

**Tech Stack:** Go, standard `net/http`, existing `internal/health`, existing `internal/cftunnel` runners, existing `internal/cftunnel/runtime.ConnectedFuse`.

---

## Overall Task Objective

Readiness must become an operational signal that answers one question: can this process currently receive Cloudflare tunnel traffic and forward it to the configured local origin path?

The end state must satisfy these project-level outcomes:

- `/live` remains a process liveness endpoint and returns `200 OK` while the health server is running.
- `/ready` returns `503 Service Unavailable` until every configured required tunnel has completed edge registration.
- `/ready` returns `200 OK` only when all configured tunnel runners are ready.
- Single-tunnel and multi-tunnel modes use the same readiness model.
- Failed, stopped, exited, pending, or still-starting tunnels make `/ready` return `503`.
- The response body stays concise and useful for scripts: `mode=<single|multi> total=<n> ready=<n> failed=<n> details=[name:status,...]`.
- Existing shutdown behavior remains explicit: when the application context is canceled, runners may stop cleanly, but stopped tunnels are not reported as ready.

## Project Scope

In scope:

- A structured readiness result in `internal/health`.
- A concurrency-safe readiness tracker for `internal/cftunnel.Runner`.
- A concurrency-safe aggregate readiness summary for `internal/cftunnel.MultiRunner`.
- Wiring health readiness for both single-tunnel and multi-tunnel modes in `cmd/app`.
- Runtime connection registration hooks that transition tunnel state from `starting` to `ready`.
- Unit tests for health status codes, single readiness, multi readiness, and registration-triggered state changes.
- README documentation for `/live` and `/ready` semantics.

Out of scope:

- Remote management plane health checks.
- Dynamic route reload.
- Per-route origin probing.
- Background active HTTP probes to local origins.
- Changing Cloudflare tunnel retry or reconnect policy.
- Adding Prometheus metrics or a metrics endpoint.

## Readiness Contract

### HTTP Semantics

- `GET /live`
  - Returns `200 OK`.
  - Body: `ok`.
  - Meaning: process and health server are alive.

- `GET /ready`
  - Returns `200 OK` when readiness provider reports `Ready=true`.
  - Returns `503 Service Unavailable` when readiness provider reports `Ready=false`.
  - Body is always the provider summary string.
  - If no readiness provider is configured, preserve current lightweight behavior by returning `200 OK` with body `ready`.

### Tunnel State Semantics

Allowed tunnel states:

- `pending`: runner exists but has not started.
- `starting`: runner goroutine started but no edge registration has completed.
- `ready`: at least one edge connection registration completed for this runner.
- `failed`: runner returned a non-canceled error.
- `stopped`: runner stopped because the parent context was canceled.
- `exited`: runner returned without context cancellation.

Readiness rule:

- A tunnel is ready only in state `ready`.
- A multi-tunnel process is ready only when all configured tunnel states are `ready`.
- `starting` must not count as ready.
- `failed`, `stopped`, `exited`, and `pending` must make `/ready` return `503`.

## Construction Rules

- Use TDD for behavior changes. Each task starts with a failing test and records the focused command that proves the failure.
- Keep the readiness contract in one place. The health runner decides HTTP status from a structured `Ready` boolean, not by parsing summary text.
- Do not add silent fallback paths. Missing state must be visible as `pending` or `not ready`.
- Do not make `/live` depend on tunnel state.
- Do not add origin probing; registration completion is the readiness signal for this phase.
- Keep output stable and script-friendly. Do not introduce JSON unless the CLI contract is explicitly changed later.
- Preserve existing graceful shutdown behavior and errors.
- Keep commits small: health contract, tunnel state tracking, runtime wiring, app wiring/docs.

## File Map

- Modify `internal/health/runner.go`: add structured readiness provider and make `/ready` choose `200` or `503`.
- Modify `internal/health/runner_test.go`: cover ready and not-ready status codes and default behavior.
- Modify `internal/cftunnel/runner.go`: add per-runner readiness tracker and expose readiness summary/provider.
- Modify `internal/cftunnel/multi_runner.go`: aggregate readiness from tunnel runners and stop counting `starting` as running/ready.
- Modify `internal/cftunnel/multi_runner_test.go`: cover pending, starting, ready, failed, stopped, and aggregate status.
- Modify `internal/cftunnel/runtime/http2_options.go`: reuse or extend `ConnectedFuse` with a concrete state implementation if needed.
- Modify `internal/cftunnel/runtime/quic_runtime.go`: pass a real connected fuse into control stream setup so QUIC registration marks readiness.
- Modify `internal/cftunnel/runtime/http2_server.go`: keep HTTP2 connected fuse path wired through `HTTP2ServerOptions`.
- Modify `cmd/app/main.go`: wire single and multi tunnel readiness providers into the health runner.
- Modify `cmd/app/main_test.go`: assert health runner receives a readiness provider for both single and multi modes.
- Modify `README.md`: document `/live` versus `/ready`.

---

### Task 1: Add Structured Health Readiness Contract

**Files:**
- Modify: `internal/health/runner.go`
- Modify: `internal/health/runner_test.go`

- [ ] **Step 1: Write failing tests for `/ready` status codes**

Add tests that configure a provider returning ready and not-ready states:

```go
func TestRunnerReadyReturnsServiceUnavailableWhenProviderNotReady(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listen := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	runner := NewRunner(listen, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.SetReadyProvider(func() ReadyStatus {
		return ReadyStatus{Ready: false, Summary: "mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runner run: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("health runner did not stop")
		}
	}()

	waitForHealthStatus(t, "http://"+listen+"/live", http.StatusOK)
	resp, err := http.Get("http://" + listen + "/ready")
	if err != nil {
		t.Fatalf("get ready: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read ready body: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected ready status: %d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "cftunnel:starting") {
		t.Fatalf("unexpected ready body: %s", string(body))
	}
}

func TestRunnerReadyReturnsOKWhenProviderReady(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listen := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	runner := NewRunner(listen, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.SetReadyProvider(func() ReadyStatus {
		return ReadyStatus{Ready: true, Summary: "mode=single total=1 ready=1 failed=0 details=[cftunnel:ready]"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runner run: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("health runner did not stop")
		}
	}()

	waitForHealthStatus(t, "http://"+listen+"/ready", http.StatusOK)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/health -run 'TestRunnerReadyReturns(ServiceUnavailable|OK)' -count=1
```

Expected: FAIL because `ReadyStatus`, `SetReadyProvider`, and status-code behavior do not exist yet.

- [ ] **Step 3: Implement structured provider**

In `internal/health/runner.go`, add:

```go
type ReadyStatus struct {
	Ready   bool
	Summary string
}

type ReadyProvider func() ReadyStatus
```

Change `Runner` to store `ready ReadyProvider`. Replace `SetReadySummaryProvider` with:

```go
func (r *Runner) SetReadyProvider(fn ReadyProvider) {
	r.ready = fn
}

func (r *Runner) ReadyStatus() ReadyStatus {
	if r.ready != nil {
		status := r.ready()
		if status.Summary == "" {
			status.Summary = "not ready"
		}
		return status
	}
	return ReadyStatus{Ready: true, Summary: "ready"}
}
```

Update `/ready` handler:

```go
mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
	status := r.ReadyStatus()
	if status.Ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_, _ = w.Write([]byte(status.Summary))
})
```

- [ ] **Step 4: Run health tests**

Run:

```bash
go test ./internal/health -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/health/runner.go internal/health/runner_test.go
git commit -m "feat: add structured health readiness contract"
```

---

### Task 2: Track Single Tunnel Readiness

**Files:**
- Modify: `internal/cftunnel/runner.go`
- Modify: `internal/cftunnel/runtime/http2_options.go`
- Modify: `internal/cftunnel/runtime/quic_runtime.go`
- Modify: `internal/cftunnel/runner_test.go` or create focused tests in `internal/cftunnel/readiness_test.go`

- [ ] **Step 1: Write failing tests for single runner readiness**

Create `internal/cftunnel/readiness_test.go`:

```go
package cftunnel

import (
	"strings"
	"testing"

	apphealth "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/health"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestRunnerReadinessDefaultsToPending(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	status := runner.ReadyStatus()
	if status.Ready {
		t.Fatalf("expected runner to start not ready: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=single total=1 ready=0 failed=0 details=[cftunnel:pending]") {
		t.Fatalf("unexpected summary: %s", status.Summary)
	}
}

func TestRunnerReadinessMarksReadyWhenConnected(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	runner.markReadyForTest()

	status := runner.ReadyStatus()
	if !status.Ready {
		t.Fatalf("expected ready runner: %#v", status)
	}
	if !strings.Contains(status.Summary, "details=[cftunnel:ready]") {
		t.Fatalf("unexpected summary: %s", status.Summary)
	}
}

var _ apphealth.ReadyProvider = (*Runner)(nil).ReadyStatus
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/cftunnel -run TestRunnerReadiness -count=1
```

Expected: FAIL because `ReadyStatus` and readiness state methods do not exist.

- [ ] **Step 3: Implement runner readiness state**

Add an internal readiness tracker in `internal/cftunnel/runner.go`:

```go
type tunnelReadiness struct {
	mu     sync.RWMutex
	name   string
	status string
}

func newTunnelReadiness(name string) *tunnelReadiness {
	return &tunnelReadiness{name: name, status: "pending"}
}

func (r *tunnelReadiness) set(status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *tunnelReadiness) snapshot() (name, status string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.name, r.status
}

func (r *tunnelReadiness) Connected() {
	r.set("ready")
}

func (r *tunnelReadiness) IsConnected() bool {
	_, status := r.snapshot()
	return status == "ready"
}
```

Add `readiness *tunnelReadiness` to `Runner`, initialize it in `NewRunner` and `NewNamedRunner`, and add:

```go
func (r *Runner) ReadyStatus() health.ReadyStatus {
	name, status := r.readiness.snapshot()
	ready := status == "ready"
	failed := 0
	if status == "failed" {
		failed = 1
	}
	readyCount := 0
	if ready {
		readyCount = 1
	}
	return health.ReadyStatus{
		Ready: ready,
		Summary: fmt.Sprintf(
			"mode=single total=1 ready=%d failed=%d details=[%s:%s]",
			readyCount,
			failed,
			name,
			status,
		),
	}
}
```

In `Run`, set state to `starting` before session preparation. Set `failed` on non-canceled error, `stopped` on context cancellation, and `exited` on clean return without cancellation. Keep `ready` set by the runtime connected fuse.

Pass `r.readiness` into HTTP2 options and QUIC options so control stream registration calls `Connected()`.

- [ ] **Step 4: Run cftunnel tests**

Run:

```bash
go test ./internal/cftunnel -run TestRunnerReadiness -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cftunnel/runner.go internal/cftunnel/readiness_test.go internal/cftunnel/runtime/http2_options.go internal/cftunnel/runtime/quic_runtime.go
git commit -m "feat: track single tunnel readiness"
```

---

### Task 3: Aggregate Multi-Tunnel Readiness

**Files:**
- Modify: `internal/cftunnel/multi_runner.go`
- Modify: `internal/cftunnel/multi_runner_test.go`

- [ ] **Step 1: Write failing aggregate readiness tests**

Update `TestMultiRunnerReadinessSummary` to expect `ready`, not `running`, and add pending coverage:

```go
func TestMultiRunnerReadyStatusRequiresAllTunnelsReady(t *testing.T) {
	t.Parallel()

	multi := &MultiRunner{
		runners: []tunnelRunner{
			fakeTunnelRunner{name: "a", run: func(context.Context) error { return nil }},
			fakeTunnelRunner{name: "b", run: func(context.Context) error { return nil }},
		},
		state: map[string]string{
			"a": "ready",
			"b": "starting",
		},
	}

	status := multi.ReadyStatus()
	if status.Ready {
		t.Fatalf("expected multi runner not ready: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=multi total=2 ready=1 failed=0") {
		t.Fatalf("unexpected summary header: %s", status.Summary)
	}
	if !strings.Contains(status.Summary, "a:ready") || !strings.Contains(status.Summary, "b:starting") {
		t.Fatalf("unexpected summary details: %s", status.Summary)
	}
}

func TestMultiRunnerReadyStatusAllReady(t *testing.T) {
	t.Parallel()

	multi := &MultiRunner{
		runners: []tunnelRunner{
			fakeTunnelRunner{name: "a", run: func(context.Context) error { return nil }},
			fakeTunnelRunner{name: "b", run: func(context.Context) error { return nil }},
		},
		state: map[string]string{
			"a": "ready",
			"b": "ready",
		},
	}

	status := multi.ReadyStatus()
	if !status.Ready {
		t.Fatalf("expected multi runner ready: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=multi total=2 ready=2 failed=0") {
		t.Fatalf("unexpected summary header: %s", status.Summary)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/cftunnel -run 'TestMultiRunnerReadyStatus|TestMultiRunnerReadinessSummary' -count=1
```

Expected: FAIL because `ReadyStatus` does not exist and the old summary counts `starting` as running.

- [ ] **Step 3: Implement aggregate readiness**

Add `ReadyStatus() health.ReadyStatus` to `MultiRunner`. Keep `ReadinessSummary()` only as a compatibility wrapper if needed:

```go
func (r *MultiRunner) ReadyStatus() health.ReadyStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.runners) == 0 {
		return health.ReadyStatus{Ready: false, Summary: "mode=multi total=0 ready=0 failed=0 details=[]"}
	}

	total := len(r.runners)
	readyCount := 0
	failed := 0
	details := make([]string, 0, total)
	for _, runner := range r.runners {
		name := runner.Name()
		status := r.state[name]
		if status == "" {
			status = "pending"
		}
		if status == "ready" {
			readyCount++
		}
		if status == "failed" {
			failed++
		}
		details = append(details, fmt.Sprintf("%s:%s", name, status))
	}
	return health.ReadyStatus{
		Ready: readyCount == total,
		Summary: fmt.Sprintf(
			"mode=multi total=%d ready=%d failed=%d details=[%s]",
			total,
			readyCount,
			failed,
			strings.Join(details, ","),
		),
	}
}

func (r *MultiRunner) ReadinessSummary() string {
	return r.ReadyStatus().Summary
}
```

When launching child runners, mark `starting`, then let child runner readiness update `ready` by callback or by wrapping each named runner with a shared tracker. On failure, set `failed`; on parent cancellation, set `stopped`; on clean exit without cancellation, set `exited`.

- [ ] **Step 4: Run multi-runner tests**

Run:

```bash
go test ./internal/cftunnel -run 'TestMultiRunnerReadyStatus|TestMultiRunnerIntegration' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cftunnel/multi_runner.go internal/cftunnel/multi_runner_test.go
git commit -m "feat: aggregate multi tunnel readiness"
```

---

### Task 4: Wire Health Runner in Application Bootstrap

**Files:**
- Modify: `cmd/app/main.go`
- Modify: `cmd/app/main_test.go`

- [ ] **Step 1: Write failing app wiring tests**

Add a single-tunnel health provider test to `cmd/app/main_test.go`:

```go
func TestBuildRunnersWiresHealthReadySummaryForSingleTunnel(t *testing.T) {
	t.Parallel()

	cfg := config.AppConfig{
		HealthListen: ":9090",
		CFTunnel: config.CFTunnelConfig{
			EdgeProtocol: config.EdgeProtocolHTTP2,
			Target:       "http://127.0.0.1:8081",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runners, err := buildRunners(cfg, logger)
	if err != nil {
		t.Fatalf("build runners: %v", err)
	}
	if len(runners) != 2 {
		t.Fatalf("unexpected runner count: %d", len(runners))
	}
	hr, ok := runners[0].(*health.Runner)
	if !ok {
		t.Fatalf("first runner should be health runner, got %T", runners[0])
	}
	status := hr.ReadyStatus()
	if status.Ready {
		t.Fatalf("single tunnel should not be ready before registration: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=single total=1 ready=0") {
		t.Fatalf("unexpected ready summary: %s", status.Summary)
	}
}
```

Update the existing multi-tunnel test to call `hr.ReadyStatus().Summary` and expect `ready=0`, not `running`.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./cmd/app -run TestBuildRunnersWiresHealth -count=1
```

Expected: FAIL because single-tunnel mode currently does not wire a readiness provider and multi mode still uses summary-only wiring.

- [ ] **Step 3: Implement app wiring**

In `cmd/app/main.go`, wire providers for both modes:

```go
if len(cfg.Tunnels) > 0 {
	multi, err := cftunnel.NewMultiRunner(cfg.Tunnels, logger)
	if err != nil {
		return nil, err
	}
	if healthRunner != nil {
		healthRunner.SetReadyProvider(multi.ReadyStatus)
	}
	runners = append(runners, multi)
	return runners, nil
}

tunnelRunner := cftunnel.NewRunner(cfg.CFTunnel, logger)
if healthRunner != nil {
	healthRunner.SetReadyProvider(tunnelRunner.ReadyStatus)
}
runners = append(runners, tunnelRunner)
return runners, nil
```

- [ ] **Step 4: Run app tests**

Run:

```bash
go test ./cmd/app -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/app/main.go cmd/app/main_test.go
git commit -m "feat: wire tunnel readiness into health endpoint"
```

---

### Task 5: Document and Verify Readiness Behavior

**Files:**
- Modify: `README.md`
- Optionally modify: `PROJECT_PLAN.md`

- [ ] **Step 1: Update README health endpoint section**

Document:

```markdown
## Health Endpoints

When `--health-listen` is non-empty, the app exposes:

- `/live`: process liveness. Returns `200 OK` while the health server is running.
- `/ready`: tunnel readiness. Returns `200 OK` only when every configured tunnel has completed edge registration. Returns `503 Service Unavailable` while tunnels are pending, starting, failed, stopped, or exited.

Readiness response bodies are text summaries such as:

```text
mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]
mode=multi total=2 ready=2 failed=0 details=[alpha:ready,beta:ready]
```
```

- [ ] **Step 2: Run focused tests**

Run:

```bash
go test ./internal/health ./internal/cftunnel ./cmd/app -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full verification**

Run:

```bash
go test ./... -count=1
go build -o /tmp/cf-quicktunnel-ipv6pool-readiness ./cmd/app
```

Expected: both commands succeed.

- [ ] **Step 4: Commit docs and final verification note**

```bash
git add README.md PROJECT_PLAN.md
git commit -m "docs: document tunnel readiness semantics"
```

---

## Self-Review

- Spec coverage: `/live`, `/ready`, single mode, multi mode, failed/stopped states, and registration-triggered readiness are covered.
- Placeholder scan: no `TBD`, `TODO`, or unspecified validation steps remain.
- Type consistency: plan consistently uses `health.ReadyStatus`, `SetReadyProvider`, `ReadyStatus()`, `ready`, `starting`, `failed`, `stopped`, and `exited`.
- Scope control: active origin probing, metrics, remote ingress, and reconnect policy changes are intentionally out of scope.
