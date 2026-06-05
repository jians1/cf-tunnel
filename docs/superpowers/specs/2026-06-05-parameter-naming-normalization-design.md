# cf-tunnel Parameter Naming Normalization Design

**Date:** 2026-06-05

**Scope:** Only the parameter naming normalization, documentation alignment, and regression coverage required to eliminate the currently inconsistent CLI/config naming scheme. No backward compatibility layer is included.

## Goal

Normalize all user-facing configuration names in `cf-tunnel` so CLI flags, YAML fields, route option keys, help text, tests, and README examples follow one explicit naming convention with no legacy aliases.

## Problem Statement

The current project exposes the same origin TLS concepts through multiple naming schemes:

- CLI top-level flags use kebab-case with `origin` prefix, such as `--cf-origin-server-name` and `--cf-origin-insecure-skip-verify`.
- YAML top-level fields use snake_case, but one field drops the `origin` prefix: `origin_server_name` and `insecure_skip_verify`.
- CLI `--cf-route` inline options use shorter keys: `server_name` and `insecure_skip_verify`.
- YAML route objects use `origin_server_name` and `insecure_skip_verify`.
- README examples describe the behaviors separately, but do not clearly document the naming mapping.

This is not a runtime defect today, but it is a clear interface-governance defect. It increases migration cost between CLI and YAML, creates avoidable misconfiguration risk, and makes the external contract harder to maintain.

## Naming Policy

The normalized naming policy is mandatory for all user-facing configuration surfaces in this scope.

### Rule N1: CLI Flags

All CLI flags use kebab-case.

Examples:

- `--cf-origin-server-name`
- `--cf-origin-insecure-skip-verify`

### Rule N2: YAML Fields

All YAML fields use snake_case.

Examples:

- `origin_server_name`
- `origin_insecure_skip_verify`

### Rule N3: Route Option Keys Must Keep Full Semantic Prefixes

Route-specific option names must preserve the same semantic word roots as the top-level fields instead of using shortened variants.

Normalized route keys:

- CLI `--cf-route` options:
  - `origin_server_name=...`
  - `origin_insecure_skip_verify=true|false`
- YAML `routes[]` fields:
  - `origin_server_name`
  - `origin_insecure_skip_verify`

### Rule N4: No Compatibility Layer

Legacy parameter names are removed rather than aliased.

Removed names in this cycle include:

- route CLI option `server_name`
- route CLI option `insecure_skip_verify`
- YAML top-level field `insecure_skip_verify`
- YAML route field `insecure_skip_verify`

If users provide removed names, parsing must fail with explicit errors rather than silently accepting both formats.

## Final Target Contract

After normalization, the user-facing contract becomes:

### Single-Tunnel CLI

- `--cf-origin-server-name=<name>`
- `--cf-origin-insecure-skip-verify`
- `--cf-route=/path=url[,host=...][,origin_server_name=...][,origin_insecure_skip_verify=true|false]`

### YAML Single-Tunnel / Multi-Tunnel

```yaml
cf_tunnel:
  target: https://127.0.0.1:9443
  origin_server_name: secure.internal
  origin_insecure_skip_verify: false
  routes:
    - path: /secure/*
      target: https://127.0.0.1:9443
      origin_server_name: secure.internal
      origin_insecure_skip_verify: true
```

## In Scope

- top-level `cf_tunnel` YAML field renaming for origin TLS verification semantics
- `routes[]` YAML field renaming for origin TLS verification semantics
- `--cf-route` inline option key renaming
- CLI usage text updates
- README Chinese and English example updates
- parser validation and error behavior updates
- regression test updates covering accepted names and rejected legacy names
- task governance CSV for staged execution tracking

## Out of Scope

- unrelated runtime feature work
- protocol redesign
- config file format expansion beyond YAML
- automatic migration tooling
- compatibility aliases or deprecation windows
- renaming internal runtime JSON/log field names unless directly required by the parser contract in this scope

## Affected Surfaces

### Primary Code Paths

- `internal/config/config.go`
- `internal/config/config_test.go`
- `README.md`

### Secondary Verification Paths

- any tests asserting route string serialization
- any tests asserting config-file field acceptance or rejection
- governance documents tracking this remediation cycle

## Delivery Strategy

Use four execution phases. Each phase ends with explicit exit criteria and does not borrow work from later phases.

### Phase P0: Contract Freeze

**Objective:** Freeze the final naming contract and record exact scope boundaries before code changes.

**Tasks:**

- document the normalized naming rules
- list removed legacy names
- map in-scope files and verification expectations
- initialize the execution ledger

**Boundary Rules:**

- no production code changes
- no opportunistic renames outside the agreed parameter surfaces

**Exit Criteria:**

- normalized naming policy is written
- removed names are explicitly listed
- task ledger exists with status and verification fields

### Phase P1: Parser and Validation Normalization

**Objective:** Make the parser accept only the normalized names.

**Tasks:**

- rename YAML field tags for top-level origin TLS verification
- rename YAML route allowed fields and decode struct fields
- rename CLI route option parsing and serialization keys
- update CLI usage/help strings
- ensure removed names fail fast with clear parse errors

**Boundary Rules:**

- do not add alias handling
- do not change unrelated runtime behavior
- do not rename logging/output fields unless parser correctness depends on it

**Exit Criteria:**

- only normalized names are accepted on user-facing config surfaces in scope
- removed names are rejected deterministically
- help output reflects the final contract

### Phase P2: Documentation and Example Normalization

**Objective:** Make all published examples and prose match the new contract.

**Tasks:**

- update README Chinese examples
- update README English examples
- add an explicit naming rule section describing CLI/YAML/route conventions
- remove stale examples using legacy route option keys

**Boundary Rules:**

- no new feature documentation
- no undocumented behavioral changes beyond naming normalization

**Exit Criteria:**

- README contains only normalized names for in-scope parameters
- naming policy is understandable without reading code

### Phase P3: Regression Closure and Governance Recording

**Objective:** Lock the normalized contract with tests and execution evidence.

**Tasks:**

- update config parser tests to the normalized names
- add rejection tests for removed names
- run targeted and full test verification
- record actual results and status transitions in the CSV ledger

**Boundary Rules:**

- no additional interface redesign after test closure begins
- newly discovered unrelated issues are logged separately, not absorbed into this scope

**Exit Criteria:**

- tests cover accepted and rejected naming forms
- verification results are recorded in the ledger
- all in-scope tasks reach terminal status

## Construction Rules

- one naming policy per concept: the same semantic concept must use the same word roots across CLI, YAML, and route option surfaces
- no hidden compatibility: removed names must fail loudly
- docs ship with code: no parser change is complete until README and usage strings match
- tests are contract guards: every removed name should have a rejection test, and every supported name should have an acceptance test
- scope discipline: only parameter naming, docs, and directly related tests are included

## Verification Model

Each task in the ledger must define:

- inspection command or test command
- expected acceptance condition
- actual result field updated during execution

Recommended verification commands for this cycle:

- `PATH=/usr/local/go/bin:$PATH go test ./internal/config/...`
- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH go run ./cmd/app --help`
- targeted grep/inspection for README examples and field names

## Risks and Controls

### R1: Breaking Existing User Commands

This is intentional scope, not an accidental regression.

**Control:** make the breaking contract explicit in README and parser errors.

### R2: Partial Rename Leaves Split Semantics

A partial rename would preserve the same governance defect under a new surface.

**Control:** treat CLI top-level flags, CLI route options, YAML top-level fields, YAML route fields, tests, and docs as one atomic change set.

### R3: Docs Drift From Implementation

**Control:** P2 is mandatory before closure, and P3 cannot complete until README inspection passes.

## File Outputs

This normalization governance cycle produces:

- spec: `docs/superpowers/specs/2026-06-05-parameter-naming-normalization-design.md`
- task ledger: `docs/governance/2026-06-05-parameter-naming-normalization-task-ledger.csv`

## Approval Boundary

Any further renaming outside this user-facing parameter scope requires a separate decision. This spec only governs the current normalization cycle.
