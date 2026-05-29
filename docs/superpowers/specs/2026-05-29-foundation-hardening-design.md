# SuperTeam Foundation Hardening Design

Date: 2026-05-29
Status: Approved for specification (revised)
Scope: Platform foundation hardening, not feature-module implementation

## 1. Purpose

SuperTeam already has meaningful foundation assets: a Web console shell, Go Control Plane domain code, Rust Runtime Agent, shared packages, and contract documents. The current risk is that these pieces are not yet a stable engineering baseline. Some code paths cannot compile, contracts cover only ~10% of the actual API surface, and the Control Plane has no working startup path.

This design defines the foundation-hardening work needed before building larger business modules such as task center, approval center, employee management, capability gateway, workflow orchestration, or audit dashboards.

The goal is to make the platform skeleton trustworthy:

- Control Plane compiles, starts through one primary entrypoint, and wires configuration, storage, repositories, services, handlers, middleware, and routes in one place.
- Control Plane OpenAPI, Runtime OpenAPI, Go handlers, Rust Runtime client, and TypeScript client boundaries agree on paths and payloads.
- Database migrations, sqlc queries, generated code, and tests form a repeatable generation and verification loop.
- Runtime Agent has one formal product mode: a managed daemon connected to Control Plane.
- Runtime registration, heartbeat, task claim, lease renewal, event reporting, completion, and failure reporting form a minimal verifiable execution loop.
- Web remains a console shell for this phase, but has stable client and domain-state boundaries for later real-data pages.

## 2. Non-Goals

This design does not implement:

- Full task center pages.
- Approval policy engines or complex human approval flows.
- Temporal workflows.
- Multi-tenant authorization and OpenFGA integration.
- Production deployment orchestration.
- Full provider session governance.
- Full Web dashboard data binding.
- A Runtime Agent host mode that accepts business task dispatch outside Control Plane.

The work should stay focused on foundation readiness. Product modules should start only after this baseline is stable.

## 3. Current State Assessment

The repository is not an empty scaffold. It already contains:

- `apps/web`: Next.js App Router entry, console shell, shared visual components, and tests.
- `apps/control-plane`: Go domain packages for tasks, runtime nodes, auth, audit, storage, and API handlers.
- `apps/runtime-agent`: Rust daemon crate, provider adapters, workspace handling, executor loops, Control Plane client, local diagnostics, and tests.
- `packages/ui`, `packages/views`, `packages/core`, `packages/api-client`: shared UI and frontend boundaries.
- `contracts/control-plane`, `contracts/runtime`, `contracts/provider`: API and provider contract material.

### 3.1 Control Plane — Cannot Compile (Critical)

The Control Plane is the most fragile component. Neither entrypoint can produce a running binary.

**Duplicate entrypoints, both broken:**

- `cmd/server/main.go` calls `api.NewServer()` without providing required handler parameters — compile error.
- `cmd/control-plane/main.go` calls `api.NewRouter()` which only registers `GET /health` — functionally incomplete.

**Duplicate routers:**

- `internal/api/router.go` — health-only router (`NewRouter`).
- `internal/api/server.go` — full API router (`NewServer`) with task and runtime routes, but not wired into any compilable entrypoint.

**SQLC querier interface is broken:**

- The `querier` interface references parameter structs that do not exist (`ListAuditEventsByActorParams`, `ListTaskEventsParams`, `ListTaskArtifactsParams`).
- Missing query method `DeleteOldAuditEvents`.
- All packages that depend on the storage layer (auth, runtime, task, api/handlers) cannot compile tests.

**Implementation state by package:**

| Package | State |
|---------|-------|
| `config` | Real implementation, tests pass |
| `storage` | SQLC mismatch, cannot compile |
| `auth` | Real implementation, blocked by storage |
| `task` | Real implementation, blocked by storage |
| `runtime` | Real implementation, blocked by storage |
| `audit` | Real implementation, blocked by storage |
| `api/handlers` | Real implementation, blocked by storage + no wiring |
| `approval` | Stub — interface only |
| `artifact` | Stub — interface only |
| `workflow` | Stub — interface only |

### 3.2 Contracts — ~10% Coverage (Critical)

| Contract | Defined Endpoints | Actual API Surface | Coverage |
|----------|------------------|--------------------|----------|
| `contracts/control-plane/openapi.yaml` | 1 (`GET /health`) | ~10 (tasks CRUD, runtime register/heartbeat/claim, health) | ~10% |
| `contracts/runtime/openapi.yaml` | 9 | 6 local diagnostics + 3 misattributed CP endpoints | Mixed |
| `packages/api-client` | 1 (`GET /health`) | Same as control-plane contract | ~10% |
| `packages/core` | 1 helper function | 0 domain models | N/A |

The runtime contract mixes concerns: it includes heartbeat (`POST /runtime/nodes/{nodeId}/heartbeat`), claim (`POST /runtime/tasks/claim`), and lease (`PATCH /runtime/tasks/{taskId}/lease`) endpoints that are served by the Control Plane, not by the Runtime Agent. These should be in the control-plane contract.

### 3.3 Runtime Agent — Mostly Complete (Low Risk)

The Runtime Agent is the most mature component. The daemon mode described in the product architecture already exists and is functional.

**Implemented capabilities:**

- Daemon mode: default when no subcommand is provided. Registers with Control Plane, polls for tasks, executes with concurrent slot management, maintains heartbeats, streams events, handles lease renewal, supports graceful shutdown.
- Control Plane client: all 8 API methods implemented (register, heartbeat, claim_task, update_task_status, push_event, complete_task, fail_task, renew_lease).
- Provider adapters: Claude Code and OpenCode with event normalization.
- Task execution: three concurrent loops (polling, execution with semaphore, lease renewal), workspace management, retry with exponential backoff.
- Local diagnostics: HTTP server with `/health`, `/providers`, `/runs/*`, WebSocket `/ws`.
- One-shot `run` subcommand: local development diagnostic, not a product dispatch path.
- Tests: 10 test files covering CLI, client, daemon, health, HTTP server, provider spawn/events/exit, run registry.

**Remaining gaps:**

- Rust client paths may not align with actual Go handler routes (contract drift).
- Auth token configuration needs explicit support in config/CLI/env.
- No fake provider adapter for deterministic end-to-end testing.

### 3.4 Web — Mock Only (Expected)

The Web console uses 100% mock data. This is expected for this phase. The shared packages (`api-client`, `core`, `views`) have the right structure but minimal content. No action is needed beyond expanding `api-client` and `core` to match the Control Plane contract once it stabilizes.

### 3.5 Local Development Environment

The Control Plane startup path depends on PostgreSQL, Redis, and S3-compatible storage. The current project has:

- PostgreSQL: migration files exist, connection info documented.
- Redis: referenced in config but no local setup documented.
- S3: referenced in config but no local setup documented.

A local development story for all three dependencies is needed before the Control Plane can start reliably.

## 4. Design Overview

Foundation hardening is organized into six work packages. They are ordered by dependency: each package unblocks or reduces ambiguity for the next one.

### 4.1 SQLC and Storage Layer Repair

**Goal:** Make the storage layer compile and all storage-dependent tests pass.

The SQLC querier interface is the single highest-priority blocker. Without a working storage layer, nothing else in the Control Plane can compile or be tested.

Tasks:

- Fix SQLC query files to match the querier interface, or regenerate the querier interface from the query files. Pick the direction that produces the least rework.
- Add missing query `DeleteOldAuditEvents`.
- Ensure query parameter structs exist for all multi-parameter queries.
- Run `sqlc generate` and verify zero diffs in committed generated code.
- Verify `go test ./apps/control-plane/internal/storage/...` passes.
- Verify all packages that depend on storage (auth, runtime, task, audit, api/handlers) can compile.

Acceptance:

- `go build ./apps/control-plane/...` succeeds.
- `go test ./apps/control-plane/...` passes (excluding integration tests requiring live DB).

### 4.2 Contract Source of Truth

**Goal:** Define the target API surface before building the startup wiring, so that handler interfaces are known upfront.

Contracts must describe the API surface that clients actually use:

- `contracts/control-plane/openapi.yaml` must include:
  - Health: `GET /health`
  - Tasks: `POST /api/v1/tasks`, `GET /api/v1/tasks`, `GET /api/v1/tasks/{id}`, `PUT /api/v1/tasks/{id}/status`, `POST /api/v1/tasks/{id}/cancel`
  - Runtime: `POST /api/v1/runtime/register`, `POST /api/v1/runtime/heartbeat`, `POST /api/v1/runtime/claim`, `GET /api/v1/runtime/nodes`
  - Task events: `POST /api/v1/tasks/{id}/events`, `GET /api/v1/tasks/{id}/events`
  - Task completion: `POST /api/v1/tasks/{id}/complete`, `POST /api/v1/tasks/{id}/fail`
  - Auth basics if enabled
- `contracts/runtime/openapi.yaml` must only describe Runtime Agent local diagnostics:
  - `GET /health`, `GET /providers`, `POST /runs`, `GET /runs/{runId}`, `GET /runs/{runId}/events`, `POST /runs/{runId}/cancel`, `GET /ws`
  - Remove heartbeat, claim, and lease endpoints (these are Control Plane endpoints)
- `contracts/provider/` remains the language-neutral provider contract.
- Run `oapi-codegen` and commit generated server stubs for type-safe handler interfaces.
- Go handler paths and Rust client paths must align with the updated contracts.
- `packages/api-client` should align to the same contract for frontend access.

Acceptance:

- Contract smoke test: every Go handler route matches an OpenAPI path.
- Contract smoke test: every Rust client method calls a path that exists in the Control Plane contract.
- OpenAPI coverage ≥ 90% of actual handler endpoints.

### 4.3 Control Plane Primary Startup Path

**Goal:** One compilable, runnable entrypoint that wires the full server.

Control Plane must have one primary startup path. That path is responsible for:

- Loading YAML configuration and environment overrides.
- Validating required database, Redis, and S3 settings. Fail fast (< 5 seconds) on missing required config or broken storage initialization.
- Opening Postgres, Redis, and S3 clients.
- Creating sqlc query handles.
- Creating repositories for task, runtime, auth, audit modules.
- Creating domain services.
- Creating API handlers (using contract-generated interfaces where applicable).
- Applying middleware.
- Registering all routes.
- Starting the HTTP server.

Implementation decisions:

- **Pick one entrypoint.** `cmd/control-plane/main.go` is the canonical location per CLAUDE.md directory conventions. Remove or repurpose `cmd/server/main.go`.
- **Use `server.go` as the router** (it already has the full route set). Remove or rename `router.go` to clearly mark it as test-only or delete it if unused.
- **Stub packages** (approval, artifact, workflow): keep as interface-only stubs during this phase. Do not add implementations. Wire no-op or placeholder handlers if route registration requires them.
- **Hidden no-op dependencies are forbidden.** The service must not appear healthy while critical dependencies are unavailable.

Acceptance:

- `go build ./apps/control-plane/cmd/control-plane/` produces a binary.
- Running the binary with valid config starts and exposes all contract-defined routes.
- Running the binary with missing config fails fast with a clear error message.
- `go test ./apps/control-plane/...` passes.

### 4.4 Runtime Agent Contract Alignment

**Goal:** Align the already-implemented Runtime Agent with the Control Plane contract and add missing config support.

The Runtime Agent daemon is functionally complete. This work package focuses on alignment, not new features.

Tasks:

- Verify all Rust Control Plane client paths match the updated `contracts/control-plane/openapi.yaml` paths exactly.
- Add auth token support: config file field, `--token` CLI flag, `SUPTEAM_TOKEN` environment variable.
- Verify event model alignment: Rust `ProviderEvent` variants match the Control Plane event storage schema.
- Remove or relocate any runtime contract references to Control Plane endpoints.
- Ensure the local `run` subcommand is clearly documented as a development diagnostic, not a product dispatch path.

Acceptance:

- Every Rust client HTTP call targets a path that exists in the Control Plane contract.
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml` passes.
- Local development command (`cargo run`) starts with daemon semantics by default.

### 4.5 Execution Event Boundary and End-to-End Proof

**Goal:** Verify the full task lifecycle with a deterministic fake provider.

Execution events must flow through a predictable pipeline:

```text
Provider output
  -> Runtime Agent ProviderEvent normalization
  -> Runtime Agent retry/batch sender
  -> Control Plane task_events
  -> task_state_history and audit_events where appropriate
  -> Web/API query surface
```

Event model for the foundation phase:

- session started
- turn started
- text delta or log chunk
- tool started
- tool completed
- artifact produced
- blocker or decision request if emitted by a provider
- execution completed
- execution failed

The Control Plane should store unknown but valid provider event payloads without hardcoding a closed provider-specific enum in business logic.

**Fake provider implementation:**

- Implement a `FakeProvider` adapter that implements the existing `ProviderAdapter` trait in the Runtime Agent.
- The fake provider emits a deterministic sequence of events (session started → text delta → tool started → tool completed → execution completed).
- It does not spawn external processes. It produces output from a configurable script embedded in the test.
- The fake provider is gated behind a `--provider fake` flag or test-only feature flag, and is never registered as a production provider type.

**End-to-end test flow:**

1. Create task through Control Plane API.
2. Register Runtime node with fake provider capability.
3. Runtime Agent claims task.
4. Fake provider executes and emits deterministic events.
5. Events are persisted in Control Plane.
6. Task completes with result.
7. Node load is released.
8. Read task and events back through API and verify.

Acceptance:

- The end-to-end test passes deterministically without real Claude Code or OpenCode.
- `go test ./apps/control-plane/...` passes.
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml` passes.
- All events in the pipeline are queryable through the Control Plane API.

### 4.6 Web Data Readiness Boundary

**Goal:** Prepare shared packages for real data without building feature pages.

Web should not expand into full feature pages during this hardening phase. It should become ready for real data:

- `packages/api-client` exposes health, task, runtime node, and task event calls that match the Control Plane contract.
- `packages/core` owns small domain summaries (task status, runtime node status, event types) and state-composition helpers.
- `packages/views` remains framework-neutral and does not depend directly on Next.js or Tauri routing.
- Existing mock homepage data can remain, but should be easy to replace with API-backed data.
- Loading, empty, error, and unauthorized states should be defined at the shared view/core boundary.

This prevents future feature pages from baking mock data into component structure.

Acceptance:

- `pnpm -r --if-present test` passes.
- `api-client` type signatures match the Control Plane contract.
- `core` exports domain types for task, runtime node, and task event.

## 5. Target Data Flow

The minimal foundation loop is:

1. A user, test client, or future Web page creates a task through Control Plane.
2. Control Plane persists the task in `tasks` with status `pending`.
3. Runtime Agent registers and reports supported providers and available slots.
4. Control Plane selects an eligible Runtime node by provider support, node health, and capacity.
5. Runtime Agent claims the task and receives a lease.
6. Runtime Agent creates a local workspace and starts the matching provider adapter.
7. Provider output is normalized into `ProviderEvent` records.
8. Runtime Agent sends events to Control Plane with retry behavior.
9. Control Plane persists events and updates task state history.
10. Runtime Agent reports completion or failure.
11. Control Plane updates task status, records result/error, releases node load, and exposes task/event/result queries.

The Control Plane is the long-lived state and policy authority. Runtime Agent is the managed execution node. Provider adapters execute work but do not own platform state.

## 6. Error Handling

Errors should be classified by boundary.

### 6.1 Startup Errors

Missing required configuration, invalid DSNs, storage initialization failures, and object-store setup failures should fail fast (< 5 seconds). The service should not expose a healthy API while critical dependencies required for the MVP loop are absent.

Hidden no-op dependencies are forbidden. If a service cannot connect to its database, it must not silently serve health-only responses while all functional APIs return errors.

### 6.2 Generation And Migration Errors

Migration, query, and generated-code mismatch is a foundation failure. It should block completion. The project should not rely on stale generated code that compiles only in a subset of packages.

The project should expose:
- One documented command for code generation (`make generate` or equivalent).
- One documented command for verification (`make verify` or equivalent, which runs generation and checks for diffs).

### 6.3 Runtime Communication Errors

Runtime registration, heartbeat, claim, lease renewal, event reporting, completion, and failure reporting need clear retry behavior.

Default behavior (already implemented in Runtime Agent, needs verification against Control Plane responses):

- Registration failure: retry with backoff, do not claim tasks.
- Heartbeat failure: continue retrying, report local degraded state.
- Claim failure: back off and retry.
- Lease renewal failure: stop extending affected task; cancel or fail task according to lease semantics.
- Event reporting failure: retry bounded attempts; if still failing, fail the task with a structured error.

The Control Plane must return error codes that the Runtime Agent can classify as retryable vs terminal.

### 6.4 Provider Errors

Provider spawn failure, non-zero exit, stream parse failure, cancellation, and timeout should become structured task failure events. Raw stderr can be stored as diagnostics, but task state should not depend on reading unstructured logs.

### 6.5 Contract Errors

Invalid payloads, unsupported provider types, illegal state transitions, missing node identity, and unauthorized runtime tokens should return stable error codes. Runtime Agent and Web should be able to tell retryable errors from terminal errors.

The Control Plane should use a consistent error response format across all endpoints (e.g., `{"error": {"code": "string", "message": "string", "retryable": bool}}`).

## 7. Local Development Environment

Before the Control Plane can start locally, three dependencies must be available:

| Dependency | Local Development Option |
|-----------|------------------------|
| PostgreSQL | Docker Compose or local install. Migration files exist at `internal/storage/migrations/`. |
| Redis | Docker Compose or local install. Used for cache and lightweight queue. |
| S3-compatible storage | MinIO via Docker Compose. Used for logs, artifacts, and execution output. |

The project should include a `docker-compose.dev.yaml` (or similar) that starts all three dependencies with the correct ports and default credentials for local development. This file should be documented in `docs/development.md`.

## 8. Stub Package Policy

The following packages are interface-only stubs and should remain stubs during foundation hardening:

- `internal/approval` — no implementation needed until approval center feature work begins.
- `internal/artifact` — no implementation needed until artifact management feature work begins.
- `internal/workflow` — no implementation needed until Temporal integration begins.

These stubs should not be wired into the primary startup path with real handlers. If route registration requires a placeholder, use a `501 Not Implemented` handler. Do not add no-op implementations that silently succeed.

## 9. Testing And Acceptance Criteria

Foundation hardening is complete only when these checks pass:

### 9.1 Compilation Gates

- `go build ./apps/control-plane/...` succeeds.
- `cargo build --manifest-path apps/runtime-agent/Cargo.toml` succeeds.
- `pnpm -r --if-present build` succeeds.

### 9.2 Test Gates

- `go test ./apps/control-plane/...` passes.
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml` passes.
- `pnpm -r --if-present test` passes.

### 9.3 Contract Consistency Gates

- Every Go handler route has a matching OpenAPI path definition.
- Every Rust client HTTP call targets a path that exists in the Control Plane contract.
- Every `api-client` TypeScript method calls a path that exists in the Control Plane contract.
- OpenAPI coverage ≥ 90% of actual handler endpoints.

### 9.4 Startup Behavior Gates

- Control Plane starts with one documented command.
- Control Plane fails fast (< 5 seconds) with a clear error message when required config is missing.
- Control Plane does not expose a healthy `/health` response while critical dependencies (DB, Redis) are unavailable.
- Runtime Agent local development starts with daemon semantics.

### 9.5 End-to-End Proof

A fake-provider end-to-end test passes:

1. Create task
2. Register Runtime node
3. Claim task
4. Execute fake provider
5. Persist events
6. Complete task
7. Release node load
8. Read task and events back

The test must be deterministic and must not depend on real Claude Code, OpenCode, or external network access.

### 9.6 Documentation Gates

- `README.md` does not contradict the verified runtime state.
- `docs/development.md` documents how to start Control Plane and Runtime Agent locally.
- `docs/api.md` documents the Control Plane API surface.

## 10. Implementation Order

### Phase 1: Compilable Baseline (blocks everything)

1. **Fix SQLC generation mismatch**
   - Fix querier interface or regenerate from query files.
   - Add missing `DeleteOldAuditEvents` query.
   - Ensure all parameter structs exist.
   - Run `sqlc generate`, verify zero diffs.
   - Verify `go build ./apps/control-plane/...` succeeds.

2. **Define Control Plane contract**
   - Update `contracts/control-plane/openapi.yaml` to cover all task and runtime endpoints.
   - Clean up `contracts/runtime/openapi.yaml` to only cover local diagnostics.
   - Run `oapi-codegen` and commit generated types.
   - Add route smoke tests (Go route paths === OpenAPI paths).

3. **Wire primary startup path**
   - Use `cmd/control-plane/main.go` as the single entrypoint.
   - Wire storage, repositories, services, handlers, middleware, and routes.
   - Remove or repurpose `cmd/server/main.go`.
   - Remove `router.go` or rename to `router_test.go` if used for test helpers.
   - Add fail-fast validation for required config.
   - Wire stub packages as `501 Not Implemented` handlers if needed.

### Phase 2: Contract Alignment and Runtime Verification

4. **Align Runtime Agent with Control Plane contract**
   - Verify Rust client paths match Go handler routes.
   - Add auth token config/CLI/env support.
   - Verify event model alignment.
   - Ensure daemon mode is the default local development experience.

5. **Implement missing Control Plane handlers for execution lifecycle**
   - Add task event persistence handler (`POST /api/v1/tasks/{id}/events`).
   - Add task complete handler (`POST /api/v1/tasks/{id}/complete`).
   - Add task fail handler (`POST /api/v1/tasks/{id}/fail`).
   - Add lease renewal handler (`PATCH /api/v1/runtime/tasks/{id}/lease`).
   - Persist task state transitions and release node load on terminal states.

### Phase 3: End-to-End Verification

6. **Add fake provider and end-to-end proof**
   - Implement `FakeProvider` adapter (implements `ProviderAdapter` trait).
   - Write end-to-end test covering the full lifecycle from Section 9.5.
   - Verify all events are queryable.

### Phase 4: Web Preparation

7. **Prepare Web real-data boundaries**
   - Expand `packages/api-client` for MVP foundation APIs (tasks, runtime nodes, events, health).
   - Add `packages/core` domain types (task status, runtime node status, event types).
   - Keep `packages/views` platform-neutral.
   - Do not build feature pages.

### Phase 5: Documentation and Cleanup

8. **Update documentation**
   - Update `README.md` to reflect verified runtime state.
   - Write `docs/development.md` with startup commands and local environment setup.
   - Write `docs/api.md` documenting the Control Plane API surface.
   - Add `docker-compose.dev.yaml` for PostgreSQL, Redis, and MinIO.
   - Remove dead entrypoints and router files.

## 11. Risks And Mitigations

### 11.1 Expanding Into Feature Work Too Early

Risk: building task center, approval UI, or employee pages before the backend loop is stable.

Mitigation: use the compilation and test gates (Section 9.1–9.2) as the entry condition for feature-module work.

### 11.2 Treating Diagnostics As Dispatch

Risk: local Runtime HTTP/WS or direct provider commands become a shadow control path.

Mitigation: document them as diagnostics only. All business tasks must be claimed from Control Plane. The `run` subcommand is explicitly a development diagnostic.

### 11.3 Contract Drift Returning

Risk: handlers, Rust client, TS client, and docs drift again.

Mitigation: route smoke tests (Go paths === OpenAPI paths === Rust client paths === TS client paths) are part of the acceptance criteria. Require contract updates with API changes.

### 11.4 Hidden Mock Data Coupling

Risk: Web components keep growing around hard-coded dashboard data.

Mitigation: define domain summaries and API-client boundaries before building more pages. The acceptance gate in Section 9.6 ensures `api-client` and `core` are ready.

### 11.5 Overfitting To Current Providers

Risk: Control Plane business logic hardcodes Claude Code or OpenCode.

Mitigation: keep provider type as registry/config data and validate server-side without creating a closed business enum in core workflow logic. The fake provider adapter (Section 4.5) proves the provider abstraction is genuine.

### 11.6 SQLC Regression

Risk: future sqlc changes break the querier interface again.

Mitigation: add `make verify` command that runs `sqlc generate` and fails on diffs. Include in CI once CI is set up.

### 11.7 Stub Packages Becoming Dependencies

Risk: stub packages (approval, artifact, workflow) gain partial implementations that become load-bearing without proper testing.

Mitigation: explicit policy in Section 8 — stubs remain interface-only with `501 Not Implemented` handlers until their feature phase begins.

## 12. Completion Definition

This foundation-hardening effort is complete when:

- The repository has a passing backend, runtime, and frontend test baseline (Sections 9.1–9.2).
- Contract consistency tests pass (Section 9.3).
- Startup behavior meets fail-fast requirements (Section 9.4).
- A fake-provider task can complete through the full Control Plane and Runtime path (Section 9.5).
- Web shared layers are ready for real API data without requiring full feature-page implementation (Section 9.6).
- A new developer can start Control Plane and Runtime Agent using documented commands.
- Runtime Agent behaves as a managed daemon in local and deployed contexts.

After this point, the project can safely proceed to feature specs for task center, approval center, employee management, capability integration, and audit views.
