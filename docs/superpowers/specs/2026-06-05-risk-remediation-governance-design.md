# cf-tunnel Risk Remediation Governance Design

**Date:** 2026-06-05

**Scope:** Only the risk fixes and engineering governance items already identified in the repository review.

## Goal

Build a phased remediation and governance spec for the current `cf-tunnel` project so the team can repair known risks in a controlled order, track execution status, and verify completion with explicit acceptance criteria.

## In Scope

- Go version strategy and build environment consistency
- Docker build and release artifact consistency
- Local CI script and GitHub release workflow alignment
- `--cf-route` CLI parsing defect remediation
- health readiness default behavior hardening
- regression test supplementation for the above items
- README, release, and execution documentation alignment
- task tracking and acceptance evidence recording

## Out of Scope

- New tunnel features
- Protocol behavior redesign
- Large-scale refactors unrelated to the identified risks
- Multi-quarter platform governance beyond the current repair set
- Performance tuning unless directly required by a known risk fix

## Confirmed Risk Baseline

### R1: Build and release chain depends on a pinned Go `1.26` toolchain

- Observed in `go.mod`, `Dockerfile`, local CI, and GitHub release workflow.
- Risk: if the toolchain or image tag is unavailable in an execution environment, build, CI, Docker packaging, and release publication all fail together.

### R2: Docker build flags are inconsistent with the documented release build

- Observed between `README.md`, `scripts/build-release.sh`, and `Dockerfile`.
- Risk: Docker image binaries can differ from release script binaries, reducing reproducibility and complicating debugging.

### R3: Docker build context includes repository metadata that should not participate in artifact construction

- Observed in `.dockerignore`.
- Risk: larger build context, avoidable metadata leakage, and possible divergence between local and container builds.

### R4: `--cf-route` parsing is not safe for target URLs containing commas

- Observed in `internal/config/config.go`.
- Risk: valid targets can be misparsed into route options, causing broken configuration through CLI while YAML may still work.

### R5: Health readiness defaults to ready when no provider is wired

- Observed in `internal/health/runner.go`.
- Risk: future wiring mistakes can silently expose false-positive readiness.

## Delivery Strategy

Use a four-phase execution model. Each phase has one primary objective and explicit boundaries to prevent scope drift.

### Phase P0: Baseline Confirmation

**Objective:** Freeze the remediation scope, evidence set, and acceptance model.

**Tasks:**

- Record the confirmed risk list and impacted files.
- Define execution boundaries and non-goals.
- Create the task ledger with standardized status fields.
- Record environmental blockers such as missing local `go` toolchain.

**Boundary Rules:**

- No production code changes in this phase.
- No hidden “quick fixes” before the ledger is initialized.

**Exit Criteria:**

- The risk list is documented.
- Each risk maps to a planned execution task.
- Status and verification fields are defined in the CSV ledger.

### Phase P1: Build and Release Chain Repair

**Objective:** Restore a coherent, reproducible build and release path.

**Tasks:**

- Normalize Go version policy across `go.mod`, `Dockerfile`, CI script, and GitHub Actions workflow.
- Align Docker build flags with the documented release build contract.
- Reduce Docker build context to required files only.
- Define the canonical build verification sequence for local and CI environments.

**Boundary Rules:**

- Do not change tunnel runtime behavior.
- Do not mix protocol or origin proxy logic into this phase.

**Exit Criteria:**

- One declared Go version strategy exists across all build entry points.
- Docker build semantics match release build semantics.
- Build context exclusions are documented and enforced.
- Local CI and release workflow reference the same build assumptions.

### Phase P2: Configuration and Runtime Safety Repair

**Objective:** Remove configuration parsing ambiguity and eliminate false readiness defaults.

**Tasks:**

- Redesign `--cf-route` CLI parsing so target URLs and route options are unambiguous.
- Keep CLI and YAML semantics aligned for route targets and per-route options.
- Change health readiness behavior so missing providers do not report healthy by default.
- Add regression tests for route parsing and readiness behavior.

**Boundary Rules:**

- Do not introduce new configuration features beyond what is needed to remove ambiguity.
- Do not refactor unrelated runtime packages.

**Exit Criteria:**

- Route parsing behavior is explicit and testable.
- Invalid and edge-case route inputs are covered by tests.
- Readiness cannot return a false-positive default state.
- Documentation reflects the final CLI behavior.

### Phase P3: Governance Closure

**Objective:** Close the loop with documentation, evidence, and maintainable task tracking.

**Tasks:**

- Update README, release notes guidance, and script usage text affected by the fixes.
- Record actual verification commands and outcomes per completed task.
- Mark blocked items explicitly if an external dependency prevents completion.
- Freeze the phase ledger as the authoritative implementation status snapshot.

**Boundary Rules:**

- No new functional scope is introduced in closure.
- Newly discovered issues are logged separately unless they block an in-scope fix.

**Exit Criteria:**

- All in-scope tasks have a terminal status.
- Each completed task has a verification standard and recorded evidence.
- Documentation and implementation no longer contradict each other.

## Construction Rules

- Evidence before modification: each task starts from a concrete observed defect.
- One phase, one class of change: avoid mixing build-chain work with runtime behavior work.
- No scope drift: only R1-R5 are part of this remediation cycle.
- Documentation parity: every user-visible behavior change must update the relevant docs.
- Verification before completion: no task is marked complete without a defined check.
- Explicit blockers: missing tools, unavailable external services, or CI constraints are recorded as blockers, not silently skipped.
- No opportunistic refactors: only direct supporting refactors are allowed.

## Status Model

All tasks in the CSV ledger use one of these states:

- `未开始`
- `进行中`
- `阻塞`
- `已完成`
- `已验证`

## Verification Model

Every execution task must include:

- verification command or inspection method
- expected result
- actual result field for execution time updates

If local execution is impossible, the ledger must state the blocking reason and required environment.

## Environment Constraints

- The current local environment reviewed on 2026-06-05 does not provide a `go` executable.
- Dynamic verification for `go test`, `go build`, and workflow parity therefore requires either:
  - a prepared local Go toolchain, or
  - CI execution evidence

This constraint must remain visible until cleared in the task ledger.

## File Outputs

This governance design produces:

- spec document: `docs/superpowers/specs/2026-06-05-risk-remediation-governance-design.md`
- task ledger: `docs/governance/2026-06-05-risk-remediation-task-ledger.csv`

## Approval Gate

This spec is intended to govern only the current remediation cycle. Any new issue discovered during execution must be evaluated separately and should not be merged into this scope without an explicit scope update.
