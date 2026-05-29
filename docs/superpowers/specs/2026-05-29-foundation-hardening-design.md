# SuperTeam Foundation Hardening Design

Date: 2026-05-29
Status: Approved for specification
Scope: Platform foundation hardening, not feature-module implementation

## 1. Purpose

SuperTeam already has meaningful foundation assets: a Web console shell, Go Control Plane domain code, Rust Runtime Agent, shared packages, and contract documents. The current risk is that these pieces are not yet a stable engineering baseline. Some code paths are partially implemented, some contracts lag behind code, and the current Control Plane verification does not pass.

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

The current foundation gaps are architectural rather than purely feature-level:

1. **Runnable baseline drift**
   - Control Plane has more than one startup/router path.
   - One router exposes only `/health`.
   - Another server path expects handlers but is not fully wired by the command entrypoint.
   - Current Go verification fails, which blocks reliable backend extension.

2. **Contract drift**
   - Control Plane contract is behind the implemented API surface.
   - Runtime contract, Runtime Agent client, and Go handler paths do not consistently describe the same endpoints.
   - Runtime task polling, claim, lease, events, complete, and fail paths need one source of truth.

3. **Database generation drift**
   - sqlc generated code and query interfaces are not in a verified reproducible state.
   - Generated code mismatch should be treated as a foundation failure.

4. **Runtime mode ambiguity**
   - Runtime Agent has local HTTP/WS diagnostics and direct provider-run commands.
   - Product architecture should not treat these as a task-dispatch host mode.
   - The formal mode is a managed daemon that receives tasks only through Control Plane.

5. **End-to-end proof gap**
   - There is not yet a minimal automated proof that a task can move through create, claim, execute, event persistence, completion, and node-load release.

6. **Web data-readiness gap**
   - The Web shell and components exist, but data is still mock-oriented.
   - Later pages need stable API-client and domain-state boundaries before feature pages expand.

## 4. Design Overview

Foundation hardening is organized into six work packages. They should be implemented in order because each package reduces ambiguity for the next one.

### 4.1 Control Plane Boot Boundary

Control Plane should have one primary startup path. That path is responsible for:

- Loading YAML configuration and environment overrides.
- Validating required database, Redis, and object-store settings.
- Opening Postgres, Redis, and S3 clients.
- Creating sqlc query handles.
- Creating repositories for task, runtime, auth, audit, artifact, and later modules.
- Creating domain services.
- Creating API handlers.
- Applying middleware.
- Registering all routes.
- Starting the HTTP server.

`/health` should remain available, but it must not be the only route exposed by the main command. If a lightweight router is useful for unit tests, it should be named and scoped as a test helper or health-only router so it cannot be mistaken for the product server.

The startup path should fail fast on missing required config or broken storage initialization. It should avoid hidden no-op dependencies that make the service look alive while core APIs are unavailable.

### 4.2 Database Generation Boundary

Database work must become reproducible:

- Migrations define the schema.
- SQL query files define data access.
- sqlc generates Go code from the query files.
- Generated code is committed when the repository convention requires it.
- Tests verify the generated query layer.

The hardening task should fix current generation mismatch first. After that, the project should expose a single documented command for generation and a single documented command for verification.

Suggested acceptance gates:

- `go test ./apps/control-plane/...` passes.
- Running the generation command after a clean checkout does not leave unexpected diffs.
- Query tests cover tasks, runtime nodes, auth runtime tokens, task events, task artifacts, and audit events that are part of the MVP loop.

### 4.3 Contract Source Of Truth

Contracts should describe the API surface that clients actually use. During this phase:

- `contracts/control-plane/openapi.yaml` should include health, auth basics if enabled, tasks, runtime registration, heartbeat, claim, lease, events, complete, and fail endpoints.
- `contracts/runtime/openapi.yaml` should only describe local Runtime Agent diagnostics that are still intentionally supported.
- Runtime business dispatch must be described as Runtime Agent to Control Plane, not external caller to Runtime Agent.
- Go handlers and Rust client paths must align with the contract.
- `packages/api-client` should align to the same contract for frontend access.

Short term, the clients may remain hand-written if that is faster. The design direction is still contract-first: contract updates should precede or accompany handler/client changes, and smoke tests should catch route drift.

### 4.4 Runtime Agent Daemon Boundary

Runtime Agent has one formal product mode: `daemon`.

Whether it runs on a server node, a developer machine, or a customer-side execution node, it is a managed daemon controlled by the Control Plane. It must not accept business tasks directly from Web, external systems, or local diagnostic endpoints.

Daemon responsibilities:

- Read `node_id`, Control Plane URL, auth token, workspace settings, provider adapter settings, and logging settings from config, environment, or CLI.
- Register itself with Control Plane.
- Report heartbeat, provider availability, slot capacity, and current load.
- Claim tasks through Control Plane.
- Maintain task leases.
- Execute tasks through local provider adapters.
- Normalize provider output into structured events.
- Report events, final status, result, and artifact references back to Control Plane.
- Keep only necessary local runtime state and temporary workspaces.

Existing local HTTP/WS capabilities should be treated as diagnostics, health, and observation facilities. Direct provider-run commands are development diagnostics. Neither is a product dispatch path.

The default local development command should also use daemon semantics. Local development may connect to a local Control Plane, but the architectural shape is the same as a deployed node.

### 4.5 Execution Event Boundary

Execution events must flow through a predictable pipeline:

```text
Provider output
  -> Runtime Agent ProviderEvent normalization
  -> Runtime Agent retry/batch sender
  -> Control Plane task_events
  -> task_state_history and audit_events where appropriate
  -> Web/API query surface
```

For the foundation phase, the event model should be small but structured:

- session started
- turn started
- text delta or log chunk
- tool started
- tool completed
- artifact produced
- blocker or decision request if emitted by a provider
- execution completed
- execution failed

Not every provider must emit every event. The Control Plane should store unknown but valid provider event payloads without hardcoding a closed provider-specific enum in business logic.

WebSocket, NATS, and Temporal are not required for the foundation loop. They can be introduced later when the durable storage and synchronous API path are stable.

### 4.6 Web Data Readiness Boundary

Web should not expand into full feature pages during this hardening phase. It should become ready for real data:

- `packages/api-client` exposes health, task, runtime node, and task event calls that match the Control Plane contract.
- `packages/core` owns small domain summaries and state-composition helpers.
- `packages/views` remains framework-neutral and does not depend directly on Next.js or Tauri routing.
- Existing mock homepage data can remain, but should be easy to replace with API-backed data.
- Loading, empty, error, and unauthorized states should be defined at the shared view/core boundary.

This prevents future feature pages from baking mock data into component structure.

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

Missing required configuration, invalid DSNs, storage initialization failures, and object-store setup failures should fail fast. The service should not expose a healthy API while critical dependencies required for the MVP loop are absent.

### 6.2 Generation And Migration Errors

Migration, query, and generated-code mismatch is a foundation failure. It should block completion. The project should not rely on stale generated code that compiles only in a subset of packages.

### 6.3 Runtime Communication Errors

Runtime registration, heartbeat, claim, lease renewal, event reporting, completion, and failure reporting need clear retry behavior.

Suggested default:

- Registration failure: retry with backoff and do not claim tasks.
- Heartbeat failure: continue retrying, report local degraded state.
- Claim failure: back off and retry.
- Lease renewal failure: stop extending affected task; cancel or fail task according to lease semantics.
- Event reporting failure: retry bounded attempts; if still failing, fail the task with a structured error.

### 6.4 Provider Errors

Provider spawn failure, non-zero exit, stream parse failure, cancellation, and timeout should become structured task failure events. Raw stderr can be stored as diagnostics, but task state should not depend on reading unstructured logs.

### 6.5 Contract Errors

Invalid payloads, unsupported provider types, illegal state transitions, missing node identity, and unauthorized runtime tokens should return stable error codes. Runtime Agent and Web should be able to tell retryable errors from terminal errors.

## 7. Testing And Acceptance Criteria

Foundation hardening is complete only when these checks pass:

- `go test ./apps/control-plane/...`
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml`
- `pnpm -r --if-present test`
- Contract smoke tests verify that Go routes, Rust Runtime client paths, and OpenAPI paths agree.
- A fake-provider end-to-end test passes:
  - create task
  - register Runtime node
  - claim task
  - execute fake provider
  - persist events
  - complete task
  - release node load
  - read task and events back
- Control Plane has one documented primary startup command.
- Runtime Agent local development starts with daemon semantics.
- Web shared packages expose real-data boundaries without requiring full feature pages.

Docs should also be updated so `README.md`, `docs/development.md`, `docs/api.md`, and `docs/NEXT_STEPS.md` do not contradict the verified runtime state.

## 8. Implementation Order

1. **Restore the compilable baseline**
   - Regenerate or repair sqlc output.
   - Fix missing generated query methods.
   - Fix command entrypoint compile errors.
   - Verify `go test ./apps/control-plane/...`.

2. **Unify Control Plane boot**
   - Pick the primary command.
   - Wire storage, repositories, services, handlers, middleware, and routes.
   - Keep health-only router clearly separated if still needed for tests.

3. **Align contracts and API paths**
   - Update Control Plane OpenAPI.
   - Update Runtime-facing endpoint paths.
   - Align Rust client methods and Go handlers.
   - Add route smoke tests.

4. **Clarify Runtime Agent daemon startup**
   - Add token config/CLI/env support.
   - Make local default command express daemon behavior.
   - Keep local HTTP/WS and direct provider run as diagnostics only.

5. **Implement minimal execution result loop**
   - Add missing events, complete, fail, and lease handlers.
   - Persist task events and state transitions.
   - Release node load on terminal states.

6. **Add fake-provider end-to-end proof**
   - Avoid relying on real Claude Code or OpenCode.
   - Verify the control loop with deterministic test output.

7. **Prepare Web real-data boundaries**
   - Expand `packages/api-client` for MVP foundation APIs.
   - Add `packages/core` summaries.
   - Keep `packages/views` platform-neutral.

## 9. Risks And Mitigations

### 9.1 Expanding Into Feature Work Too Early

Risk: building task center, approval UI, or employee pages before the backend loop is stable.

Mitigation: use the foundation acceptance gates as the entry condition for feature-module work.

### 9.2 Treating Diagnostics As Dispatch

Risk: local Runtime HTTP/WS or direct provider commands become a shadow control path.

Mitigation: document them as diagnostics only. All business tasks must be claimed from Control Plane.

### 9.3 Contract Drift Returning

Risk: handlers, Rust client, TS client, and docs drift again.

Mitigation: add route smoke tests and require contract updates with API changes.

### 9.4 Hidden Mock Data Coupling

Risk: Web components keep growing around hard-coded dashboard data.

Mitigation: define domain summaries and API-client boundaries before building more pages.

### 9.5 Overfitting To Current Providers

Risk: Control Plane business logic hardcodes Claude Code or OpenCode.

Mitigation: keep provider type as registry/config data and validate server-side without creating a closed business enum in core workflow logic.

## 10. Completion Definition

This foundation-hardening effort is complete when:

- The repository has a passing backend, runtime, and frontend test baseline.
- A new developer can start Control Plane and Runtime Agent using documented commands.
- Runtime Agent behaves as a managed daemon in local and deployed contexts.
- Contracts match the implementation and client calls.
- A fake-provider task can complete through the full Control Plane and Runtime path.
- Web shared layers are ready for real API data without requiring full feature-page implementation.

After this point, the project can safely proceed to feature specs for task center, approval center, employee management, capability integration, and audit views.
