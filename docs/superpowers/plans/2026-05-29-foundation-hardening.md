# Foundation Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore SuperTeam's platform foundation so Control Plane, Runtime Agent, contracts, and shared Web data boundaries form one verifiable execution baseline.

**Architecture:** Stabilize the backend first, then align contracts and clients, then make Runtime Agent's only product mode a managed daemon, then prove the loop with a fake-provider end-to-end test. Web work is limited to API-client and core/view readiness, not feature-page implementation.

**Tech Stack:** Go + chi + pgx + sqlc + PostgreSQL + Redis + S3-compatible object store, Rust + Tokio + reqwest, Next.js + React + TypeScript, Vitest, cargo test, Go test.

**Spec:** `docs/superpowers/specs/2026-05-29-foundation-hardening-design.md`

---

## File Structure

### Control Plane Boot And Wiring

- Modify: `apps/control-plane/cmd/control-plane/main.go`
  - Owns the single product startup path.
  - Loads config, opens storage clients, constructs repositories/services/handlers, and starts the API server.
- Modify: `apps/control-plane/cmd/server/main.go`
  - Either remove this entrypoint from product use or make it call the same app builder as `cmd/control-plane`.
- Modify: `apps/control-plane/internal/api/router.go`
  - Rename or narrow the health-only router so it is not mistaken for the product router.
- Modify: `apps/control-plane/internal/api/server.go`
  - Accept all required handlers and middleware dependencies explicitly.
  - Register the complete `/api/v1` route surface.
- Create: `apps/control-plane/internal/app/app.go`
  - Small composition root for constructing Control Plane dependencies.
  - Keeps `cmd/control-plane/main.go` short and testable.
- Test: `apps/control-plane/internal/app/app_test.go`
  - Verifies app construction fails clearly when dependencies are missing and exposes core routes when dependencies are present.

### Database And sqlc Generation

- Modify: `apps/control-plane/sqlc.yaml`
  - Keep generation rules explicit and reproducible.
- Modify: `apps/control-plane/Makefile`
  - Add a `generate-sqlc` target and make `generate` run both sqlc and OpenAPI generation.
- Modify: `apps/control-plane/internal/storage/queries/*.sql`
  - Ensure each query referenced by generated Go code has valid sqlc params and generated methods.
- Modify generated files under `apps/control-plane/internal/storage/queries/`
  - Regenerate with sqlc rather than manually editing generated code.
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`
  - Keep or repair existing test coverage for runtime nodes, tasks, auth, audit, events, artifacts.

### Contract And API Alignment

- Modify: `contracts/control-plane/openapi.yaml`
  - Add Control Plane health, tasks, runtime register, heartbeat, claim, lease, events, complete, fail endpoints.
- Modify: `contracts/runtime/openapi.yaml`
  - Keep only local Runtime Agent diagnostics that remain intentional.
  - Do not model business task dispatch into Runtime Agent.
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
  - Match Runtime-facing endpoints in the Control Plane contract.
- Modify: `apps/control-plane/internal/api/handlers/task.go`
  - Match Console-facing task endpoints in the Control Plane contract.
- Create: `apps/control-plane/internal/api/routes_test.go`
  - Smoke test that critical contract paths are registered.
- Modify: `packages/api-client/src/index.ts`
  - Export the public client surface.
- Create or modify: `packages/api-client/src/tasks.ts`, `packages/api-client/src/runtime.ts`
  - Hand-written minimal clients aligned to the contract until generated clients are introduced.

### Runtime Agent Daemon

- Modify: `apps/runtime-agent/src/config.rs`
  - Add Runtime auth token config.
  - Keep priority order: CLI > `RUNTIME_AGENT_*` env > config file > defaults.
- Modify: `apps/runtime-agent/config.example.toml`
  - Add token field with an explicit non-secret example value: `replace-with-runtime-token`.
- Modify: `apps/runtime-agent/.env.example`
  - Add token env var.
- Modify: `apps/runtime-agent/src/main.rs`
  - Default product execution should start daemon semantics.
  - Direct `run` command remains diagnostics.
- Modify: `apps/runtime-agent/src/daemon.rs`
  - Use token from config rather than requiring an out-of-band `run(token)` caller.
  - Heartbeat current load should eventually reflect active task count.
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
  - Align endpoint paths with the Control Plane contract.
- Test: `apps/runtime-agent/tests/daemon_test.rs`, `apps/runtime-agent/tests/controlplane_client_test.rs`
  - Verify config loading, token priority, and URL construction.

### Minimal Execution Loop

- Modify: `apps/control-plane/internal/task/service.go`
  - Add task event, completion, failure, lease, and load-release service boundaries as needed.
- Modify: `apps/control-plane/internal/task/repository.go`, `apps/control-plane/internal/task/pg_repository.go`
  - Add repository methods for event persistence and execution/result updates.
- Modify: `apps/control-plane/internal/runtime/service.go`
  - Add load release or reconcile behavior for terminal tasks.
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
  - Add missing runtime task event, complete, fail, and renew endpoints.
- Create: `apps/control-plane/internal/api/e2e_test.go`
  - Fake-provider-style end-to-end test through HTTP handlers or app router.

### Web Data Readiness

- Modify: `packages/api-client/src/health.ts`
  - Keep health client aligned with real response shape.
- Create: `packages/api-client/src/tasks.ts`
  - Minimal task list/create/detail/event client functions.
- Create: `packages/api-client/src/runtime.ts`
  - Minimal runtime node list/detail client functions.
- Modify: `packages/core/src/index.ts`
  - Export new domain summary helpers.
- Create: `packages/core/src/task-summary.ts`
  - Convert raw task records into UI-ready summaries.
- Create: `packages/core/src/runtime-node-summary.ts`
  - Convert raw runtime nodes into UI-ready summaries.
- Test: matching Vitest files in `packages/api-client/src` and `packages/core/src`.

### Documentation

- Modify: `README.md`
  - Update commands and current baseline after implementation.
- Modify: `docs/development.md`
  - Update local startup and verification flow.
- Modify: `docs/api.md`
  - Update endpoint examples after contract alignment.
- Modify: `docs/NEXT_STEPS.md`
  - Replace stale statements about Runtime execution gaps with verified state.
- Modify: `CHANGELOG.md`
  - Record each implementation batch.

---

## Task 1: Restore Go Compile And Generation Baseline

**Files:**
- Modify: `apps/control-plane/Makefile`
- Modify: `apps/control-plane/sqlc.yaml`
- Modify: `apps/control-plane/internal/storage/queries/*.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Capture the current compile failure**

Run:

```bash
go test ./apps/control-plane/...
```

Expected before implementation: FAIL with missing sqlc generated symbols such as `ListAuditEventsByActorParams`, `ListTaskArtifactsParams`, or missing methods referenced by `Querier`.

- [ ] **Step 2: Regenerate sqlc output from the checked-in queries**

Run:

```bash
cd apps/control-plane
sqlc generate
```

Expected: sqlc completes without errors and rewrites generated files under `internal/storage/queries`.

If `sqlc` is not installed, install or invoke it through the existing Go tooling path used by this repo, then re-run the same command. Do not manually patch generated files.

- [ ] **Step 3: Verify generated files no longer reference missing query params**

Run:

```bash
rg -n "ListAuditEventsByActorParams|ListAuditEventsByResourceParams|ListTaskArtifactsParams|ListTaskEventsParams|DeleteOldAuditEvents" apps/control-plane/internal/storage/queries
```

Expected: either no stale generated references remain, or each match has a corresponding generated type and method in the same package.

- [ ] **Step 4: Add reproducible sqlc generation target**

Modify `apps/control-plane/Makefile` so generation has explicit sqlc and OpenAPI targets:

```makefile
.PHONY: generate generate-sqlc generate-openapi test build clean migrate-up migrate-down migrate-status

generate: generate-sqlc generate-openapi

generate-sqlc:
	@echo "Generating sqlc code..."
	sqlc generate
	@echo "sqlc generation complete"

generate-openapi:
	@echo "Generating code from OpenAPI specs..."
	@mkdir -p internal/auth
	oapi-codegen -package auth -generate types,chi-server \
		-o internal/auth/generated.go \
		../../contracts/control-plane/auth.yaml
	@echo "OpenAPI code generation complete"
```

Keep existing `test`, `build`, `clean`, and migration targets. Update `clean` only if generated sqlc files are not committed by repository convention; otherwise do not delete sqlc generated files in `clean`.

- [ ] **Step 5: Verify sqlc package compiles**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries
```

Expected: PASS, or a real test failure unrelated to missing generated symbols. If tests fail because SQL queries and schema disagree, fix the SQL query file and regenerate with `sqlc generate`.

- [ ] **Step 6: Verify the full Control Plane compile baseline**

Run:

```bash
go test ./apps/control-plane/...
```

Expected after this task: any remaining failures should be real application compile errors, not sqlc generated-code drift. Continue only after generated-code drift is eliminated.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/Makefile apps/control-plane/sqlc.yaml apps/control-plane/internal/storage/queries
git commit -m "chore(control-plane): restore sqlc generation baseline"
```

---

## Task 2: Build One Control Plane Composition Root

**Files:**
- Create: `apps/control-plane/internal/app/app.go`
- Create: `apps/control-plane/internal/app/app_test.go`
- Modify: `apps/control-plane/cmd/control-plane/main.go`
- Modify: `apps/control-plane/cmd/server/main.go`
- Modify: `apps/control-plane/internal/api/router.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Write an app construction test**

Create `apps/control-plane/internal/app/app_test.go`:

```go
package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthOnlyRouterIsExplicit(t *testing.T) {
	router := NewHealthOnlyRouter()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", response.Code)
	}
}
```

This test captures the intended health-only helper before replacing ambiguous router names.

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/app -run TestHealthOnlyRouterIsExplicit -count=1
```

Expected: FAIL because `NewHealthOnlyRouter` does not exist yet.

- [ ] **Step 3: Create the health-only helper**

Create `apps/control-plane/internal/app/app.go` with this starting point:

```go
package app

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func NewHealthOnlyRouter() http.Handler {
	router := chi.NewRouter()
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:  "ok",
			Service: "control-plane",
		})
	})
	return router
}
```

- [ ] **Step 4: Run the app package test**

Run:

```bash
go test ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [ ] **Step 5: Move ambiguous health router usage**

Modify `apps/control-plane/internal/api/router.go` so it no longer exports a product-looking `NewRouter()` that only serves health. Either remove the file if only tests used it, or replace it with a small wrapper whose name is explicit:

```go
package api

import (
	"net/http"

	"github.com/superteam/control-plane/internal/app"
)

func NewHealthOnlyRouter() http.Handler {
	return app.NewHealthOnlyRouter()
}
```

Update `apps/control-plane/internal/api/health_test.go` to call `NewHealthOnlyRouter()`.

- [ ] **Step 6: Add dependency container types**

Extend `apps/control-plane/internal/app/app.go` with a composition struct that uses concrete handler types:

```go
type Dependencies struct {
	TaskHandler    *handlers.TaskHandler
	RuntimeHandler *handlers.RuntimeHandler
}
```

Import `github.com/superteam/control-plane/internal/api/handlers` in this file. Keep dependency direction one-way: `app` constructs handlers and `api` only registers handlers.

- [ ] **Step 7: Fix `cmd/server` compile drift**

Modify `apps/control-plane/cmd/server/main.go` so it does not call `api.NewServer()` without handlers. Either delete this command if the repo only supports `cmd/control-plane`, or make it delegate to the same command path. Preferred minimal content:

```go
package main

import controlplane "github.com/superteam/control-plane/cmd/control-plane"

func main() {
	controlplane.Main()
}
```

If Go package rules make importing a command package awkward, remove `cmd/server` from the supported command surface and document that `cmd/control-plane` is the only product entrypoint.

- [ ] **Step 8: Refactor `cmd/control-plane/main.go` to expose `Main()`**

Modify `apps/control-plane/cmd/control-plane/main.go`:

```go
package main

func main() {
	Main()
}

func Main() {
	// existing startup logic moves here
}
```

Keep command-line flags and config loading behavior unchanged.

- [ ] **Step 9: Wire real handlers in one place**

In the startup path, construct the dependency chain in this order:

```go
stores, err := storage.NewClients(ctx, storage.Config{...})
queries := queries.New(stores.Postgres)
taskRepo := task.NewPgRepository(queries)
taskService, err := task.NewService(taskRepo)
runtimeRepo := runtime.NewPgRepository(queries)
runtimeService, err := runtime.NewService(runtimeRepo)
poller := runtime.NewPoller()
taskHandler := handlers.NewTaskHandler(taskService)
runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller)
server := api.NewServer(taskHandler, runtimeHandler)
```

Use the actual constructor names in the repository. If a constructor differs, prefer the existing constructor rather than creating aliases.

- [ ] **Step 10: Verify Control Plane command compiles**

Run:

```bash
go test ./apps/control-plane/cmd/control-plane ./apps/control-plane/internal/api ./apps/control-plane/internal/app
```

Expected: PASS.

- [ ] **Step 11: Update changelog**

Add under `## [Unreleased] > ### Changed`:

```markdown
#### Control Plane 启动边界收敛 (2026-05-29)

- 收敛 Control Plane 主启动入口，明确 health-only router 与产品 API server 的边界，并通过统一装配路径连接存储、服务和 handlers。
```

- [ ] **Step 12: Commit**

```bash
git add apps/control-plane/cmd/control-plane apps/control-plane/cmd/server apps/control-plane/internal/app apps/control-plane/internal/api CHANGELOG.md
git commit -m "refactor(control-plane): unify startup composition"
```

---

## Task 3: Align Control Plane Contract And Runtime Routes

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `contracts/runtime/openapi.yaml`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Create: `apps/control-plane/internal/api/routes_test.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Write route smoke tests**

Create `apps/control-plane/internal/api/routes_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/superteam/control-plane/internal/api/handlers"
)

type noopTaskService struct{}
type noopRuntimeService struct{}
type noopPoller struct{}

func TestRuntimeRoutesAreRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(noopTaskService{}),
		handlers.NewRuntimeHandler(noopRuntimeService{}, noopTaskService{}, noopPoller{}),
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/runtime/register"},
		{http.MethodPost, "/api/v1/runtime/heartbeat"},
		{http.MethodPost, "/api/v1/runtime/tasks/claim"},
		{http.MethodPost, "/api/v1/runtime/tasks/1/events"},
		{http.MethodPost, "/api/v1/runtime/tasks/1/complete"},
		{http.MethodPost, "/api/v1/runtime/tasks/1/fail"},
		{http.MethodPost, "/api/v1/runtime/tasks/1/lease"},
	}

	for _, tc := range cases {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code == http.StatusNotFound {
			t.Fatalf("%s %s was not registered", tc.method, tc.path)
		}
	}
}
```

Adjust the noop test doubles to satisfy the exact handler interfaces in `handlers/task.go` and `handlers/runtime.go`. Keep their methods returning deterministic errors; the test only checks route registration, not business behavior.

- [ ] **Step 2: Run route smoke test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestRuntimeRoutesAreRegistered -count=1
```

Expected: FAIL because routes such as `/api/v1/runtime/tasks/claim` and terminal task endpoints are not registered yet.

- [ ] **Step 3: Update Control Plane OpenAPI paths**

Modify `contracts/control-plane/openapi.yaml` to include these path keys:

```yaml
paths:
  /health:
    get:
      operationId: getHealth
  /api/v1/tasks:
    get:
      operationId: listTasks
    post:
      operationId: createTask
  /api/v1/tasks/{taskId}:
    get:
      operationId: getTask
  /api/v1/tasks/{taskId}/status:
    put:
      operationId: updateTaskStatus
  /api/v1/tasks/{taskId}/cancel:
    post:
      operationId: cancelTask
  /api/v1/runtime/register:
    post:
      operationId: registerRuntimeNode
  /api/v1/runtime/heartbeat:
    post:
      operationId: heartbeatRuntimeNode
  /api/v1/runtime/tasks/claim:
    post:
      operationId: claimRuntimeTask
  /api/v1/runtime/tasks/{taskId}/events:
    post:
      operationId: appendRuntimeTaskEvents
  /api/v1/runtime/tasks/{taskId}/complete:
    post:
      operationId: completeRuntimeTask
  /api/v1/runtime/tasks/{taskId}/fail:
    post:
      operationId: failRuntimeTask
  /api/v1/runtime/tasks/{taskId}/lease:
    post:
      operationId: renewRuntimeTaskLease
  /api/v1/runtime/nodes:
    get:
      operationId: listRuntimeNodes
  /api/v1/runtime/nodes/{nodeId}:
    get:
      operationId: getRuntimeNode
```

Add schemas for each request and response used by these paths. Use existing JSON field names from Go/Rust models where possible: `node_id`, `supported_providers`, `max_slots`, `current_load`, `provider_type`, `params`, `events`, `result`, `error`.

- [ ] **Step 4: Register canonical runtime routes in Go**

Modify `apps/control-plane/internal/api/server.go` so runtime routes include canonical task paths:

```go
r.Route("/runtime", func(r chi.Router) {
	r.Post("/register", s.runtimeHandler.RegisterNode)
	r.Post("/heartbeat", s.runtimeHandler.Heartbeat)
	r.Post("/tasks/claim", s.runtimeHandler.ClaimTask)
	r.Post("/tasks/{id}/events", s.runtimeHandler.PushEvents)
	r.Post("/tasks/{id}/complete", s.runtimeHandler.CompleteTask)
	r.Post("/tasks/{id}/fail", s.runtimeHandler.FailTask)
	r.Post("/tasks/{id}/lease", s.runtimeHandler.RenewLease)
	r.Get("/nodes", s.runtimeHandler.ListNodes)
	r.Get("/nodes/{id}", s.runtimeHandler.GetNodeByID)
})
```

If temporary compatibility is needed, keep `/runtime/claim` as an alias, but mark the canonical route as `/runtime/tasks/claim`.

- [ ] **Step 5: Add stub handlers for new endpoints**

Modify `apps/control-plane/internal/api/handlers/runtime.go` with compilable stubs:

```go
func (h *RuntimeHandler) PushEvents(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "runtime task events persistence is not wired", http.StatusNotImplemented)
}

func (h *RuntimeHandler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "runtime task completion is not wired", http.StatusNotImplemented)
}

func (h *RuntimeHandler) FailTask(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "runtime task failure is not wired", http.StatusNotImplemented)
}

func (h *RuntimeHandler) RenewLease(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "runtime task lease renewal is not wired", http.StatusNotImplemented)
}
```

These stubs are replaced in Task 5. They exist here only to make route and contract alignment explicit.

- [ ] **Step 6: Align Runtime Agent client paths**

Modify `apps/runtime-agent/src/controlplane/client.rs`:

```rust
let url = format!("{}/api/v1/runtime/tasks/claim?timeout={}", self.base_url, timeout_secs);
```

Keep event, complete, fail, and lease paths aligned with the Go route names:

```rust
"/api/v1/runtime/tasks/{}/events"
"/api/v1/runtime/tasks/{}/complete"
"/api/v1/runtime/tasks/{}/fail"
"/api/v1/runtime/tasks/{}/lease"
```

- [ ] **Step 7: Run route and client tests**

Run:

```bash
go test ./apps/control-plane/internal/api -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane_client
```

Expected: Go route smoke test passes. Rust client tests pass or are updated to match the canonical paths.

- [ ] **Step 8: Update Runtime OpenAPI**

Modify `contracts/runtime/openapi.yaml` so business-dispatch endpoints under `/runtime/tasks/claim` and `/runtime/tasks/{taskId}/lease` are removed or labeled as Control Plane-facing only if this document remains for diagnostics. Keep diagnostics such as:

```yaml
paths:
  /health:
  /providers:
  /runs:
  /runs/{runId}:
  /runs/{runId}/events:
  /runs/{runId}/cancel:
```

Do not define Runtime Agent as a business task host.

- [ ] **Step 9: Update changelog**

Add under `## [Unreleased] > ### Changed`:

```markdown
#### Runtime API 契约路径收敛 (2026-05-29)

- 对齐 Control Plane OpenAPI、Go runtime routes 和 Rust Runtime Agent client 的任务 claim、事件、完成、失败和 lease 路径，并将 Runtime Agent 本地 API 收敛为诊断边界。
```

- [ ] **Step 10: Commit**

```bash
git add contracts/control-plane/openapi.yaml contracts/runtime/openapi.yaml apps/control-plane/internal/api apps/runtime-agent/src/controlplane/client.rs CHANGELOG.md
git commit -m "feat(contracts): align runtime task API paths"
```

---

## Task 4: Make Runtime Agent Daemon The Default Product Path

**Files:**
- Modify: `apps/runtime-agent/src/config.rs`
- Modify: `apps/runtime-agent/config.example.toml`
- Modify: `apps/runtime-agent/.env.example`
- Modify: `apps/runtime-agent/src/main.rs`
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/tests/daemon_test.rs`
- Modify: `apps/runtime-agent/tests/controlplane_client_test.rs`
- Modify: `package.json`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add failing config test for token priority**

Append to `apps/runtime-agent/tests/daemon_test.rs`:

```rust
#[test]
fn config_loads_runtime_token_from_env_and_cli_override() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.toml");
    std::fs::write(
        &config_path,
        r#"
[runtime]
node_id = "file-node"
control_plane_url = "http://localhost:8080"
auth_token = "file-token"
"#,
    )
    .expect("write config");

    let config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_AUTH_TOKEN", "env-token")],
        RuntimeConfigOverrides {
            auth_token: Some("cli-token".to_string()),
            ..Default::default()
        },
    )
    .expect("load config");

    assert_eq!(config.runtime.auth_token, "cli-token");
}
```

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml config_loads_runtime_token_from_env_and_cli_override
```

Expected: FAIL because `auth_token` and `RuntimeConfigOverrides.auth_token` do not exist yet.

- [ ] **Step 3: Add token to runtime config**

Modify `apps/runtime-agent/src/config.rs`:

```rust
pub struct RuntimeSection {
    pub node_id: String,
    pub control_plane_url: String,
    pub auth_token: String,
    pub heartbeat_interval: u64,
    pub max_concurrent_tasks: u16,
}

pub struct RuntimeConfigOverrides {
    pub node_id: Option<String>,
    pub auth_token: Option<String>,
    pub http_addr: Option<SocketAddr>,
    pub run_log_dir: Option<PathBuf>,
    pub claude_bin: Option<PathBuf>,
    pub opencode_bin: Option<PathBuf>,
}
```

Add file config:

```rust
struct FileRuntimeSection {
    node_id: Option<String>,
    control_plane_url: Option<String>,
    auth_token: Option<String>,
    heartbeat_interval: Option<u64>,
    max_concurrent_tasks: Option<u16>,
}
```

Apply file/env/overrides:

```rust
apply_string(&mut self.runtime.auth_token, runtime.auth_token);

"RUNTIME_AGENT_AUTH_TOKEN" => self.runtime.auth_token = value.to_string(),

apply_string(&mut self.runtime.auth_token, overrides.auth_token);
```

Validation:

```rust
if self.runtime.auth_token.trim().is_empty() {
    anyhow::bail!("runtime auth token is required");
}
```

Default:

```rust
auth_token: "local-dev-token".to_string(),
```

- [ ] **Step 4: Add CLI token option**

Modify `apps/runtime-agent/src/main.rs`:

```rust
#[arg(long)]
auth_token: Option<String>,
```

Pass it into overrides:

```rust
RuntimeConfigOverrides {
    node_id: args.node_id,
    auth_token: args.auth_token,
    http_addr: args.http_addr,
    run_log_dir: args.run_log_dir,
    claude_bin: args.claude_bin,
    opencode_bin: args.opencode_bin,
}
```

- [ ] **Step 5: Start daemon by default**

Modify `apps/runtime-agent/src/main.rs` after config load:

```rust
let daemon = RuntimeDaemon::new(config);
let snapshot = daemon.snapshot();
println!(
    "runtime-agent node={} status={}",
    snapshot.node_id, snapshot.status
);
if args.once {
    return Ok(());
}
daemon.run().await
```

Do not bind `RuntimeHttpServer` from the default product path. Keep direct `run` subcommand as a diagnostic command.

- [ ] **Step 6: Update daemon to read token from config**

Modify `apps/runtime-agent/src/daemon.rs`:

```rust
pub async fn run(self) -> Result<()> {
    let client = ControlPlaneClient::new(
        &self.config.runtime.control_plane_url,
        &self.config.runtime.auth_token,
    );
    // existing registration, heartbeat, executor startup
}
```

Remove the old `run(self, token: String)` signature.

- [ ] **Step 7: Update examples**

Modify `apps/runtime-agent/config.example.toml`:

```toml
[runtime]
node_id = "local-dev-node"
control_plane_url = "http://localhost:8080"
auth_token = "replace-with-runtime-token"
heartbeat_interval = 30
max_concurrent_tasks = 3
```

Modify `apps/runtime-agent/.env.example`:

```bash
RUNTIME_AGENT_AUTH_TOKEN=replace-with-runtime-token
```

Modify root `package.json`:

```json
"dev:runtime-agent": "cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.example.toml --once"
```

Keep `--once` in the dev script if the script is intended as a config smoke check. Add a separate documented command later for long-running daemon development.

- [ ] **Step 8: Run Runtime Agent tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 9: Update changelog**

Add under `## [Unreleased] > ### Changed`:

```markdown
#### Runtime Agent daemon 默认语义 (2026-05-29)

- 将 Runtime Agent 正式运行边界收敛为受 Control Plane 管理的 daemon，并补充认证 token 配置、环境变量和 CLI 覆盖。
```

- [ ] **Step 10: Commit**

```bash
git add apps/runtime-agent/src apps/runtime-agent/tests apps/runtime-agent/config.example.toml apps/runtime-agent/.env.example package.json CHANGELOG.md
git commit -m "feat(runtime-agent): default to managed daemon mode"
```

---

## Task 5: Implement Runtime Event, Completion, Failure, And Lease Endpoints

**Files:**
- Modify: `apps/control-plane/internal/task/models.go`
- Modify: `apps/control-plane/internal/task/repository.go`
- Modify: `apps/control-plane/internal/task/pg_repository.go`
- Modify: `apps/control-plane/internal/task/service.go`
- Modify: `apps/control-plane/internal/task/service_test.go`
- Modify: `apps/control-plane/internal/runtime/service.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime_test.go` or create it if missing
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add service tests for terminal status and event persistence**

Append focused tests to `apps/control-plane/internal/task/service_test.go`:

```go
func TestServiceAppendTaskEvent(t *testing.T) {
	repo := newMockRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	event := AppendTaskEventRequest{
		TaskID:    42,
		EventType: "text_delta",
		Payload:   []byte(`{"type":"text_delta","text":"hello"}`),
	}

	repo.On("CreateTaskEvent", mock.Anything, mock.Anything).Return(TaskEventRecord{
		ID:             1,
		TaskID:         42,
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload:        event.Payload,
	}, nil)

	created, err := service.AppendTaskEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, int64(42), created.TaskID)
	assert.Equal(t, "text_delta", created.EventType)
}
```

Use the existing mock style in the file. If the current mock repository is hand-written, add explicit methods for the new repository calls.

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/task -run TestServiceAppendTaskEvent -count=1
```

Expected: FAIL because `AppendTaskEventRequest`, `AppendTaskEvent`, or repository methods do not exist.

- [ ] **Step 3: Add task event models**

Modify `apps/control-plane/internal/task/models.go`:

```go
type TaskEvent struct {
	ID             int64
	TaskID         int64
	ExecutionID    *int64
	EventType      string
	SequenceNumber int32
	Payload        []byte
	CreatedAt      time.Time
}

type AppendTaskEventRequest struct {
	TaskID      int64
	ExecutionID *int64
	EventType   string
	Payload     []byte
}

type CompleteTaskRequest struct {
	TaskID int64
	Result []byte
}

type FailTaskRequest struct {
	TaskID int64
	Error  string
}
```

Use `json.RawMessage` instead of `[]byte` if current task models already use `json.RawMessage`.

- [ ] **Step 4: Extend repository interface**

Modify `apps/control-plane/internal/task/repository.go`:

```go
CreateTaskEvent(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error)
GetLatestTaskEventSequence(ctx context.Context, taskID int64) (int32, error)
```

Add matching param/record types if they do not exist in the task package.

- [ ] **Step 5: Implement PgRepository event methods**

Modify `apps/control-plane/internal/task/pg_repository.go`:

```go
func (r *PgRepository) GetLatestTaskEventSequence(ctx context.Context, taskID int64) (int32, error) {
	value, err := r.q.GetLatestTaskEventSequence(ctx, taskID)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case int32:
		return v, nil
	case int64:
		return int32(v), nil
	case int:
		return int32(v), nil
	default:
		return 0, nil
	}
}
```

Then implement `CreateTaskEvent` by delegating to `queries.CreateTaskEvent`.

- [ ] **Step 6: Implement task service methods**

Modify `apps/control-plane/internal/task/service.go`:

```go
func (s *Service) AppendTaskEvent(ctx context.Context, req AppendTaskEventRequest) (*TaskEvent, error) {
	if req.TaskID <= 0 {
		return nil, errors.New("task_id is required")
	}
	if strings.TrimSpace(req.EventType) == "" {
		return nil, errors.New("event_type is required")
	}
	if len(req.Payload) == 0 {
		return nil, errors.New("payload is required")
	}
	sequence, err := s.repository.GetLatestTaskEventSequence(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest event sequence: %w", err)
	}
	record, err := s.repository.CreateTaskEvent(ctx, CreateTaskEventParams{
		TaskID:         req.TaskID,
		ExecutionID:    int8FromInt64Ptr(req.ExecutionID),
		EventType:      req.EventType,
		SequenceNumber: sequence + 1,
		Payload:        req.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create task event: %w", err)
	}
	return s.recordToTaskEvent(record), nil
}
```

Add helper conversions using the same pgtype patterns already used in the task package.

- [ ] **Step 7: Replace runtime handler stubs**

Modify `apps/control-plane/internal/api/handlers/runtime.go`:

```go
func (h *RuntimeHandler) PushEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	var req struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, event := range req.Events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(event, &envelope); err != nil || envelope.Type == "" {
			http.Error(w, "event type is required", http.StatusBadRequest)
			return
		}
		if _, err := h.taskService.AppendTaskEvent(r.Context(), task.AppendTaskEventRequest{
			TaskID:    id,
			EventType: envelope.Type,
			Payload:   event,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusAccepted)
}
```

Extend the `TaskService` interface in `handlers/task.go` or `handlers/runtime.go` so runtime handlers can call `AppendTaskEvent`, `UpdateTaskStatus`, and later completion/failure methods.

- [ ] **Step 8: Implement complete and fail handlers**

In `runtime.go`, parse task id and call existing status transitions first:

```go
func (h *RuntimeHandler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	updated, err := h.taskService.UpdateTaskStatus(r.Context(), task.UpdateTaskStatusRequest{
		TaskID:    id,
		NewStatus: task.TaskStatusCompleted,
		Reason:    stringPtr("runtime completed task"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}
```

Implement `FailTask` similarly with `TaskStatusFailed` and request body `{ "error": "..." }`.

- [ ] **Step 9: Implement lease endpoint as explicit minimal acceptance**

For this foundation stage, `RenewLease` can be a state-validating no-op if lease persistence is not yet modeled:

```go
func (h *RuntimeHandler) RenewLease(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	if _, err := h.taskService.GetTask(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add a comment in the handler explaining that persistent lease records are a later task unless the schema already supports them.

- [ ] **Step 10: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/task ./apps/control-plane/internal/api/handlers ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

- [ ] **Step 11: Update changelog**

Add under `## [Unreleased] > ### Added`:

```markdown
#### Runtime 任务执行结果 API (2026-05-29)

- 补齐 Runtime task events、complete、fail 和 lease endpoint 的基础处理，支持 Runtime Agent 回传结构化执行事件和终态。
```

- [ ] **Step 12: Commit**

```bash
git add apps/control-plane/internal/task apps/control-plane/internal/runtime apps/control-plane/internal/api CHANGELOG.md
git commit -m "feat(control-plane): add runtime task result endpoints"
```

---

## Task 6: Add Fake-Provider End-To-End Proof

**Files:**
- Create: `apps/control-plane/internal/api/e2e_test.go`
- Modify: `apps/runtime-agent/tests/controlplane_client_test.rs`
- Optional Create: `apps/runtime-agent/tests/fixtures/fake_provider.sh`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Write Control Plane HTTP e2e test**

Create `apps/control-plane/internal/api/e2e_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFakeRuntimeTaskLifecycle(t *testing.T) {
	server := newTestServer(t)

	createBody := []byte(`{
		"title":"fake provider task",
		"provider_type":"fake",
		"params":{"prompt":"say hello"},
		"priority":5
	}`)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(createBody)))
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create task status=%d body=%s", createResp.Code, createResp.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	registerResp := httptest.NewRecorder()
	server.ServeHTTP(registerResp, httptest.NewRequest(http.MethodPost, "/api/v1/runtime/register", bytes.NewReader([]byte(`{
		"node_id":"fake-node",
		"name":"Fake Node",
		"supported_providers":["fake"],
		"max_slots":1,
		"metadata":{}
	}`))))
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registerResp.Code, registerResp.Body.String())
	}

	claimResp := httptest.NewRecorder()
	server.ServeHTTP(claimResp, httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil))
	if claimResp.Code != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", claimResp.Code, claimResp.Body.String())
	}

	eventsResp := httptest.NewRecorder()
	server.ServeHTTP(eventsResp, httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/1/events", bytes.NewReader([]byte(`{
		"events":[{"type":"text_delta","text":"hello"}]
	}`))))
	if eventsResp.Code != http.StatusAccepted {
		t.Fatalf("events status=%d body=%s", eventsResp.Code, eventsResp.Body.String())
	}

	completeResp := httptest.NewRecorder()
	server.ServeHTTP(completeResp, httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/1/complete", bytes.NewReader([]byte(`{"result":{"status":"success"}}`))))
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completeResp.Code, completeResp.Body.String())
	}
}
```

Implement `newTestServer(t)` using the repository's existing test patterns. If the existing services require Postgres, use the existing testcontainers helper from `queries_test.go`; do not create a second database harness unless the current helper cannot be reused.

- [ ] **Step 2: Run e2e test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestFakeRuntimeTaskLifecycle -count=1
```

Expected: FAIL until test app wiring and endpoint behavior are complete.

- [ ] **Step 3: Make e2e route use canonical claim**

Ensure the e2e test calls:

```text
POST /api/v1/runtime/tasks/claim?timeout=1
```

Do not use older `/api/v1/runtime/claim` in the e2e test.

- [ ] **Step 4: Add Runtime Agent client URL test**

Modify `apps/runtime-agent/tests/controlplane_client_test.rs` to use a local mock server and assert the claim request hits `/api/v1/runtime/tasks/claim`. If the test harness currently does not capture request paths, add a tiny axum or httpmock server:

```rust
#[tokio::test]
async fn claim_task_uses_canonical_runtime_path() {
    // Start mock server, register expectation:
    // method GET or POST must match the implementation contract.
    // path must be "/api/v1/runtime/tasks/claim".
}
```

Use the HTTP method chosen in the contract. If the contract uses POST, update `ControlPlaneClient::claim_task` to POST and adjust Go handler accordingly.

- [ ] **Step 5: Run all foundation verification commands**

Run:

```bash
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm -r --if-present test
```

Expected: all pass before this task is considered complete.

- [ ] **Step 6: Update changelog**

Add under `## [Unreleased] > ### Added`:

```markdown
#### Foundation fake provider 端到端验收 (2026-05-29)

- 新增 fake provider 风格的最小端到端验收，覆盖任务创建、Runtime 注册、claim、事件回传和完成状态。
```

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/api apps/runtime-agent/tests CHANGELOG.md
git commit -m "test: add foundation runtime e2e proof"
```

---

## Task 7: Prepare Web API Client And Core Data Boundaries

**Files:**
- Modify: `packages/api-client/src/index.ts`
- Modify: `packages/api-client/src/health.ts`
- Create: `packages/api-client/src/tasks.ts`
- Create: `packages/api-client/src/runtime.ts`
- Create: `packages/api-client/src/tasks.test.ts`
- Create: `packages/api-client/src/runtime.test.ts`
- Modify: `packages/core/src/index.ts`
- Create: `packages/core/src/task-summary.ts`
- Create: `packages/core/src/task-summary.test.ts`
- Create: `packages/core/src/runtime-node-summary.ts`
- Create: `packages/core/src/runtime-node-summary.test.ts`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add API client tests for URL construction**

Create `packages/api-client/src/tasks.test.ts`:

```ts
import { describe, expect, it, vi } from "vitest";

import { createTask, listTasks } from "./tasks";

describe("tasks api client", () => {
  it("lists tasks from the control plane API", async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify([]), { status: 200 }));

    await listTasks({ baseUrl: "http://localhost:8080", fetcher });

    expect(fetcher).toHaveBeenCalledWith("http://localhost:8080/api/v1/tasks", {
      headers: { accept: "application/json" },
      method: "GET",
    });
  });

  it("creates tasks with JSON payload", async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ id: 1 }), { status: 201 }));

    await createTask(
      { baseUrl: "http://localhost:8080", fetcher },
      { title: "Demo", provider_type: "fake", params: { prompt: "hello" }, priority: 5 },
    );

    expect(fetcher).toHaveBeenCalledWith("http://localhost:8080/api/v1/tasks", {
      body: JSON.stringify({ title: "Demo", provider_type: "fake", params: { prompt: "hello" }, priority: 5 }),
      headers: { "content-type": "application/json", accept: "application/json" },
      method: "POST",
    });
  });
});
```

- [ ] **Step 2: Run failing API client tests**

Run:

```bash
pnpm --filter @superteam/api-client test -- tasks.test.ts
```

Expected: FAIL because `tasks.ts` does not exist.

- [ ] **Step 3: Implement task client**

Create `packages/api-client/src/tasks.ts`:

```ts
export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export type CreateTaskInput = {
  title: string;
  provider_type: string;
  params: Record<string, unknown>;
  description?: string;
  priority?: number;
  target_node_id?: string;
  workspace_path?: string;
};

function trimBaseUrl(baseUrl: string) {
  return baseUrl.replace(/\/$/, "");
}

async function parseJson<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw new Error(`Control Plane request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function listTasks<T = unknown[]>(options: ApiClientOptions): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(`${trimBaseUrl(options.baseUrl)}/api/v1/tasks`, {
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<T>(response);
}

export async function createTask<T = unknown>(options: ApiClientOptions, input: CreateTaskInput): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(`${trimBaseUrl(options.baseUrl)}/api/v1/tasks`, {
    body: JSON.stringify(input),
    headers: { "content-type": "application/json", accept: "application/json" },
    method: "POST",
  });
  return parseJson<T>(response);
}
```

- [ ] **Step 4: Implement runtime client and tests**

Create `packages/api-client/src/runtime.ts`:

```ts
import type { ApiClientOptions } from "./tasks";

function trimBaseUrl(baseUrl: string) {
  return baseUrl.replace(/\/$/, "");
}

async function parseJson<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw new Error(`Control Plane request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function listRuntimeNodes<T = unknown[]>(options: ApiClientOptions): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(`${trimBaseUrl(options.baseUrl)}/api/v1/runtime/nodes`, {
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<T>(response);
}
```

Create `packages/api-client/src/runtime.test.ts` with the same fetcher pattern and assert `/api/v1/runtime/nodes`.

- [ ] **Step 5: Export API client functions**

Modify `packages/api-client/src/index.ts`:

```ts
export * from "./health";
export * from "./runtime";
export * from "./tasks";
```

- [ ] **Step 6: Add core task summary helper**

Create `packages/core/src/task-summary.ts`:

```ts
export type TaskLike = {
  id: number | string;
  title: string;
  status: string;
  provider_type?: string;
  providerType?: string;
};

export type TaskSummary = {
  id: string;
  title: string;
  status: string;
  providerLabel: string;
};

export function summarizeTask(task: TaskLike): TaskSummary {
  return {
    id: String(task.id),
    title: task.title,
    status: task.status,
    providerLabel: task.provider_type ?? task.providerType ?? "unknown",
  };
}
```

Create `packages/core/src/task-summary.test.ts`:

```ts
import { describe, expect, it } from "vitest";

import { summarizeTask } from "./task-summary";

describe("summarizeTask", () => {
  it("normalizes ids and provider labels", () => {
    expect(summarizeTask({ id: 1, title: "Demo", status: "pending", provider_type: "fake" })).toEqual({
      id: "1",
      title: "Demo",
      status: "pending",
      providerLabel: "fake",
    });
  });
});
```

- [ ] **Step 7: Add core runtime node summary helper**

Create `packages/core/src/runtime-node-summary.ts`:

```ts
export type RuntimeNodeLike = {
  node_id?: string;
  nodeId?: string;
  name: string;
  status: string;
  current_load?: number;
  currentLoad?: number;
  max_slots?: number;
  maxSlots?: number;
};

export type RuntimeNodeSummary = {
  id: string;
  name: string;
  status: string;
  loadLabel: string;
};

export function summarizeRuntimeNode(node: RuntimeNodeLike): RuntimeNodeSummary {
  const current = node.current_load ?? node.currentLoad ?? 0;
  const max = node.max_slots ?? node.maxSlots ?? 0;
  return {
    id: node.node_id ?? node.nodeId ?? node.name,
    name: node.name,
    status: node.status,
    loadLabel: `${current}/${max}`,
  };
}
```

Add a matching Vitest test for `loadLabel`.

- [ ] **Step 8: Export core helpers**

Modify `packages/core/src/index.ts`:

```ts
export * from "./health-summary";
export * from "./runtime-node-summary";
export * from "./task-summary";
```

- [ ] **Step 9: Run frontend package tests**

Run:

```bash
pnpm --filter @superteam/api-client test
pnpm --filter @superteam/core test
pnpm -r --if-present test
```

Expected: PASS.

- [ ] **Step 10: Update changelog**

Add under `## [Unreleased] > ### Added`:

```markdown
#### Web 真实数据接入底座 (2026-05-29)

- 为任务和 Runtime 节点补充最小 API client 与 core summary helper，后续页面可从 mock 数据平滑切换到真实接口。
```

- [ ] **Step 11: Commit**

```bash
git add packages/api-client/src packages/core/src CHANGELOG.md
git commit -m "feat(web): add foundation api client boundaries"
```

---

## Task 8: Update Docs And Final Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/development.md`
- Modify: `docs/api.md`
- Modify: `docs/NEXT_STEPS.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update README baseline**

Modify `README.md` current baseline to say:

```markdown
- `apps/control-plane` 通过统一启动入口装配存储、服务、handlers 和 API routes。
- `apps/runtime-agent` 默认作为受 Control Plane 管理的 daemon 运行；本地 provider run 仅用于诊断。
- `contracts/control-plane/openapi.yaml` 描述 Console 与 Runtime Agent 调用的 Control Plane API。
```

- [ ] **Step 2: Update development commands**

Modify `docs/development.md` so local verification includes:

```bash
pnpm install
cd apps/control-plane && make generate
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm -r --if-present test
```

Document Runtime Agent daemon startup with `RUNTIME_AGENT_AUTH_TOKEN` or `--auth-token`.

- [ ] **Step 3: Update API docs**

Modify `docs/api.md` so Runtime task paths use:

```text
POST /api/v1/runtime/tasks/claim
POST /api/v1/runtime/tasks/{taskId}/events
POST /api/v1/runtime/tasks/{taskId}/complete
POST /api/v1/runtime/tasks/{taskId}/fail
POST /api/v1/runtime/tasks/{taskId}/lease
```

Remove examples that instruct clients to dispatch business tasks directly to Runtime Agent local APIs.

- [ ] **Step 4: Update NEXT_STEPS**

Modify `docs/NEXT_STEPS.md` so it does not say Runtime Agent execution loop is missing if implementation is now present and verified. Replace stale status with:

```markdown
**当前 Foundation 状态**
- Control Plane 编译和统一启动入口已恢复。
- Runtime Agent 默认 daemon 语义已明确。
- Runtime 主链路已具备 fake-provider 端到端验收。
```

Only make this statement after Task 6 verification passes.

- [ ] **Step 5: Run final verification**

Run:

```bash
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm -r --if-present test
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 6: Update changelog**

Add under `## [Unreleased] > ### Changed`:

```markdown
#### Foundation 文档同步 (2026-05-29)

- 同步 README、开发指南、API 文档和下一步指引，使文档状态与已验证的 Foundation baseline 保持一致。
```

- [ ] **Step 7: Commit**

```bash
git add README.md docs/development.md docs/api.md docs/NEXT_STEPS.md CHANGELOG.md
git commit -m "docs: update foundation hardening handoff"
```

---

## Final Review Checklist

- [ ] `go test ./apps/control-plane/...` passes.
- [ ] `cargo test --manifest-path apps/runtime-agent/Cargo.toml` passes.
- [ ] `pnpm -r --if-present test` passes.
- [ ] `git diff --check` passes.
- [ ] `contracts/control-plane/openapi.yaml` includes the Control Plane task/runtime paths used by Go and Rust.
- [ ] `contracts/runtime/openapi.yaml` no longer presents Runtime Agent as a business dispatch host.
- [ ] Runtime Agent default product path is daemon semantics.
- [ ] Local HTTP/WS Runtime Agent server and direct provider run are documented only as diagnostics.
- [ ] Fake-provider style e2e proves task create, runtime register, claim, events, complete, and readback.
- [ ] CHANGELOG has entries for each implementation batch.

## Plan Self-Review

Spec coverage:

- Purpose and non-goals are covered by the file structure, implementation order, and final checklist.
- Control Plane boot boundary is covered by Task 2.
- Database generation boundary is covered by Task 1.
- Contract source of truth is covered by Task 3.
- Runtime Agent daemon boundary is covered by Task 4.
- Execution event boundary and minimal result loop are covered by Task 5 and Task 6.
- Web data readiness is covered by Task 7.
- Documentation synchronization and final verification are covered by Task 8.

Type consistency:

- Runtime task paths use `/api/v1/runtime/tasks/claim`, `/events`, `/complete`, `/fail`, and `/lease` consistently across contract, Go route, Rust client, docs, and tests.
- Runtime Agent uses `auth_token` in config and `RUNTIME_AGENT_AUTH_TOKEN` in env consistently.
- Web API client examples use snake_case payload keys to match the Control Plane JSON boundary.

Placeholder scan:

- The plan contains no unresolved `TBD`, `TODO`, `FIXME`, or implementation placeholders.
