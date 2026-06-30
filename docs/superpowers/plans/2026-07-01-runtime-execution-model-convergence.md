# Runtime Execution Model Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop Runtime Agent from defaulting into the broken legacy `/runtime/tasks/claim` polling path and make the command-driven `start_session -> ProjectTaskAttempt writeback` path the only default execution model.

**Architecture:** First make the legacy polling path impossible to hit by default, then make Control Plane claim return no work unless an explicit legacy compatibility flag is enabled, then mark the legacy runtime task contract deprecated. ProjectTask execution remains on the existing DigitalEmployee Run plus RuntimeCommand plus ProjectTaskAttempt path.

**Tech Stack:** Rust `apps/runtime-agent`, Go `apps/control-plane`, OpenAPI `contracts/control-plane/openapi.yaml`, pnpm contract verification, Go tests, Cargo tests.

---

## File Structure

- Modify `apps/runtime-agent/src/daemon.rs`: stop starting `TaskExecutor` by default.
- Modify `apps/runtime-agent/src/config.rs`: add an optional default-off `enable_legacy_task_polling` runtime config field only if preserving a temporary switch is easier than deleting the call site immediately.
- Modify `apps/runtime-agent/src/executor/mod.rs`: keep the module compiling behind the default-off switch in phase 1; do not change provider execution behavior here.
- Modify `apps/runtime-agent/tests/daemon_test.rs`: add regression coverage that a normal daemon run does not call `/api/v1/runtime/tasks/claim`.
- Modify `apps/control-plane/internal/api/handlers/runtime.go`: make `ClaimTask` return `204 No Content` by default before listing or assigning tasks.
- Modify `apps/control-plane/internal/api/handlers/runtime_test.go`: update claim tests to assert default no-op and command-driven task skip behavior.
- Modify `apps/control-plane/internal/api/routes_test.go`: keep route existence expectations if the endpoint remains, but ensure no legacy `/api/v1/runtime/claim` path returns.
- Modify `contracts/control-plane/openapi.yaml`: mark `/api/v1/runtime/tasks/*` endpoints as deprecated and point readers to RuntimeCommand and ProjectTaskAttempt writeback.
- Modify generated contract files only through the repo generator if `pnpm generate:control-plane` changes them.
- Modify documentation only where existing docs still describe `/runtime/tasks/*` as the active execution path.

## Task 1: Runtime Agent Default Stops Legacy Polling

**Files:**
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify if needed: `apps/runtime-agent/src/config.rs`
- Test: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Write a failing daemon regression test**

Add a test in `apps/runtime-agent/tests/daemon_test.rs` that starts a minimal fake Control Plane and proves Runtime Agent does not request `/api/v1/runtime/tasks/claim` during a short run. Use the existing daemon test helpers in that file for fake HTTP handling. The test should fail on current code because `TaskExecutor` starts `polling_loop`.

```rust
#[tokio::test]
async fn daemon_default_does_not_start_legacy_task_claim_loop() {
    let observed_paths = Arc::new(Mutex::new(Vec::<String>::new()));
    let observed_paths_for_server = observed_paths.clone();
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("addr");

    let server = tokio::spawn(async move {
        loop {
            let (mut socket, _) = listener.accept().await.expect("accept");
            let request = read_http_request(&mut socket).await;
            let path = request
                .lines()
                .next()
                .and_then(|line| line.split_whitespace().nth(1))
                .unwrap_or("")
                .to_string();
            observed_paths_for_server.lock().unwrap().push(path.clone());

            if path == "/api/v1/runtime/enrollments/hello" || path == "/api/v1/runtime/enroll/hello" {
                write_json_response(
                    &mut socket,
                    serde_json::json!({
                        "enrollment": {
                            "id": "11111111-1111-4111-8111-111111111111",
                            "tenant_id": "00000000-0000-4000-8000-000000000001",
                            "runtime_node_id": "22222222-2222-4222-8222-222222222222",
                            "node_id": "node-1",
                            "bootstrap_key_id": "33333333-3333-4333-8333-333333333333",
                            "status": "approved"
                        },
                        "session": {
                            "id": "44444444-4444-4444-8444-444444444444",
                            "tenant_id": "00000000-0000-4000-8000-000000000001",
                            "runtime_node_id": "22222222-2222-4222-8222-222222222222",
                            "expires_at": "2999-01-01T00:00:00Z"
                        },
                        "session_token": "session-token"
                    }),
                )
                .await;
                continue;
            }

            if path == "/api/v1/runtime/nodes/node-1/capabilities" {
                write_json_response(&mut socket, serde_json::json!({"capabilities": []})).await;
                continue;
            }

            if path == "/api/v1/runtime/heartbeat" {
                write_json_response(
                    &mut socket,
                    serde_json::json!({
                        "node_id": "node-1",
                        "name": "node-1",
                        "supported_providers": ["codex"],
                        "max_slots": 1,
                        "current_load": 0,
                        "status": "online",
                        "required_tools": []
                    }),
                )
                .await;
                continue;
            }

            write_status_response(&mut socket, "204 No Content", serde_json::json!({})).await;
        }
    });

    let mut config = RuntimeConfig::new("node-1").expect("config");
    config.runtime.control_plane_url = format!("http://{addr}");
    config.runtime.bootstrap_key = "bootstrap".to_string();
    config.runtime.heartbeat_interval = 60;

    let daemon = RuntimeDaemon::new(config);
    let run = tokio::spawn(async move {
        let _ = tokio::time::timeout(Duration::from_millis(600), daemon.run()).await;
    });
    let _ = run.await;
    server.abort();

    let paths = observed_paths.lock().unwrap().clone();
    assert!(
        !paths.iter().any(|path| path.starts_with("/api/v1/runtime/tasks/claim")),
        "legacy claim loop should not run by default; observed paths: {paths:?}"
    );
}
```

If existing helper names differ, reuse the exact helper functions already present in `daemon_test.rs`; do not add a second HTTP helper stack.

- [ ] **Step 2: Run the failing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_default_does_not_start_legacy_task_claim_loop --test daemon_test -- --nocapture
```

Expected before implementation: FAIL because `/api/v1/runtime/tasks/claim` appears, or the daemon blocks inside `TaskExecutor`.

- [ ] **Step 3: Disable TaskExecutor by default**

In `apps/runtime-agent/src/daemon.rs`, replace the default executor startup:

```rust
let executor = TaskExecutor::new(self.config, control_plane);
executor.run().await?;

Ok(())
```

with a wait loop that keeps the daemon alive after spawning WS command loop and heartbeat:

```rust
std::future::pending::<()>().await;
#[allow(unreachable_code)]
Ok(())
```

If the compiler requires the imported `TaskExecutor` to be removed, delete this import from `daemon.rs`:

```rust
use crate::executor::TaskExecutor;
```

Do not delete the executor module in this task.

- [ ] **Step 4: Run the daemon regression test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_default_does_not_start_legacy_task_claim_loop --test daemon_test -- --nocapture
```

Expected: PASS and no observed `/api/v1/runtime/tasks/claim` path.

- [ ] **Step 5: Run nearby Runtime Agent tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_ws_url_uses_runtime_ws_endpoint handle_text_command_ignores_unsupported_command_types --lib -- --nocapture
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane_client_claim_task_sends_runtime_identity_headers --test controlplane_client_test -- --nocapture
```

Expected: PASS. The controlplane client legacy method may still pass as an isolated legacy client test in phase 1.

- [ ] **Step 6: Commit Task 1**

```bash
git add apps/runtime-agent/src/daemon.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "fix(runtime-agent): disable legacy task polling by default"
```

## Task 2: Control Plane Runtime Claim Becomes Default No-Op

**Files:**
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/handlers/runtime_test.go`

- [ ] **Step 1: Write failing default no-op claim test**

In `apps/control-plane/internal/api/handlers/runtime_test.go`, add or update a test that creates a supported pending regular task and asserts `ClaimTask` returns `204` and never calls `AssignTask`.

```go
func TestClaimTaskDefaultsToNoContentWithoutLegacyCompatibility(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	supportedTask := &task.Task{
		ID:           handlerTestUUID(200),
		ProviderType: "codex",
		Priority:     1,
		Params:       []byte(`{"kind":"legacy-task"}`),
	}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {supportedTask},
		},
	}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected default claim to return 204, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assigned task, got %s", taskService.assignedTaskID)
	}
	if len(taskService.listedProviders) != 0 {
		t.Fatalf("expected no task scan by default, got providers %#v", taskService.listedProviders)
	}
}
```

Keep existing tests that verify command-driven tasks are skipped, but adjust their expected status to `204` if they now hit the default no-op guard.

- [ ] **Step 2: Run the failing handler test**

Run:

```bash
go test ./apps/control-plane/internal/api/handlers -run 'TestClaimTaskDefaultsToNoContentWithoutLegacyCompatibility|TestClaimTaskSkipsRuntimeCommandDrivenTask' -count=1
```

Expected before implementation: FAIL because current handler assigns a supported pending regular task.

- [ ] **Step 3: Implement the default no-op guard**

In `apps/control-plane/internal/api/handlers/runtime.go`, place this guard in `ClaimTask` after validating `nodeID` but before reading timeout, loading node, listing tasks, or waiting on poller:

```go
func (h *RuntimeHandler) ClaimTask(w http.ResponseWriter, r *http.Request) {
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	return

	// Legacy runtime task polling is intentionally disabled by default.
	// Project and digital-employee execution must use RuntimeCommand start_session
	// and ProjectTaskAttempt writeback instead.
```

Then remove the now-unreachable old body in the same task to avoid dead code. The final function should be:

```go
func (h *RuntimeHandler) ClaimTask(w http.ResponseWriter, r *http.Request) {
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

After removing the old body, remove imports that become unused, usually `strconv` and `time` if no other function in `runtime.go` uses them.

- [ ] **Step 4: Run handler tests**

Run:

```bash
go test ./apps/control-plane/internal/api/handlers -run 'TestClaimTask' -count=1
```

Expected: PASS after updating claim tests to the new default no-op semantics.

- [ ] **Step 5: Run route tests for runtime endpoints**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'TestRuntimeRoutes|TestLegacyRuntimeClaimRouteIsNotRegistered|TestRuntimeClaim' -count=1
```

Expected: PASS. If a route test expects successful assignment, update it to expect `204` and no assignment.

- [ ] **Step 6: Commit Task 2**

```bash
git add apps/control-plane/internal/api/handlers/runtime.go apps/control-plane/internal/api/handlers/runtime_test.go apps/control-plane/internal/api/routes_test.go
git commit -m "fix(control-plane): disable legacy runtime task claim"
```

## Task 3: Mark Legacy Runtime Task Contract Deprecated

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated files if generator changes them
- Modify docs that mention `/api/v1/runtime/tasks/claim` as active execution path

- [ ] **Step 1: Mark OpenAPI operations deprecated**

In `contracts/control-plane/openapi.yaml`, add `deprecated: true` to these operations:

```yaml
  /api/v1/runtime/tasks/claim:
    post:
      operationId: claimRuntimeTask
      deprecated: true
```

Repeat for:

```yaml
  /api/v1/runtime/tasks/{taskId}/events:
    post:
      deprecated: true
  /api/v1/runtime/tasks/{taskId}/complete:
    post:
      deprecated: true
  /api/v1/runtime/tasks/{taskId}/fail:
    post:
      deprecated: true
  /api/v1/runtime/tasks/{taskId}/lease:
    post:
      deprecated: true
```

Update summaries to include the replacement path. Example:

```yaml
summary: Deprecated legacy Runtime task claim; use RuntimeCommand start_session and ProjectTaskAttempt writeback
```

- [ ] **Step 2: Generate and verify contracts**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: both commands pass. If generated files change, include them in this task commit. Do not hand-edit generated files.

- [ ] **Step 3: Search docs for legacy active wording**

Run:

```bash
rg -n "/api/v1/runtime/tasks/claim|/runtime/tasks/claim|claimRuntimeTask|runtime task claim|TaskExecutor" docs contracts apps --glob '!**/target/**' --glob '!**/node_modules/**'
```

For docs that describe `/runtime/tasks/*` as the current active execution path, replace the wording with:

```markdown
Legacy `/api/v1/runtime/tasks/*` polling is deprecated and disabled by default. New execution must use RuntimeCommand `start_session`; ProjectTask execution must close through `/api/v1/runtime/project-task-attempts/{attemptId}` writeback endpoints.
```

Do not edit historical design docs unless they are used as current operator guidance.

- [ ] **Step 4: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Commit Task 3**

```bash
git add contracts/control-plane/openapi.yaml docs
git add apps/control-plane/internal/authzcenter/generated.go apps/control-plane/internal/authzcenter/generated.yaml 2>/dev/null || true
git commit -m "docs: deprecate legacy runtime task contract"
```

If generated files differ under other paths, stage only the files changed by `corepack pnpm generate:control-plane`.

## Task 4: Preserve ProjectTaskAttempt Command-Driven Closure

**Files:**
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify only if tests expose a regression: `apps/runtime-agent/src/commands/executor.rs`, `apps/control-plane/internal/project/service.go`, `apps/control-plane/internal/project/pg_repository.go`

- [ ] **Step 1: Run existing attempt writeback tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'Test(StartProjectTaskAttempt|CompleteProjectTaskAttempt|SubmitProjectTaskAttemptResult|FailProjectTaskAttempt|WaitHumanProjectTaskAttempt)' -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture
```

Expected: PASS. If failures occur, inspect whether they are due to the legacy polling removal or pre-existing local environment.

- [ ] **Step 2: Add regression test for command-driven project task metadata**

If no existing Rust test asserts all three writebacks after `project_task_dispatch`, add one in `apps/runtime-agent/tests/runtime_command_executor_test.rs` that:

- builds a `RuntimeSessionCommandPayload` with metadata:

```json
{
  "source": "project_task_dispatch",
  "project_task_id": "55555555-5555-4555-8555-555555555555",
  "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
  "project_task_lease_token": "lease-token-1",
  "runtime_node_id": "44444444-4444-4444-8444-444444444444",
  "handoff_contract": {"completion_path": "project_task_attempt_writeback"}
}
```

- handles a `start_session` command through `RuntimeCommandExecutor`.
- asserts the fake Control Plane receives:
  - `/api/v1/runtime/project-task-attempts/66666666-6666-4666-8666-666666666666/started`
  - `/api/v1/runtime/commands/{commandId}/complete`
  - `/api/v1/runtime/project-task-attempts/66666666-6666-4666-8666-666666666666/complete`

Use the fake HTTP server patterns already present in `runtime_command_executor_test.rs`.

- [ ] **Step 3: Run the command-driven regression test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task_dispatch --test runtime_command_executor_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 4: Run ProjectTask service tests again**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'Test(StartProjectTaskAttempt|CompleteProjectTaskAttempt|FailProjectTaskAttempt|WaitHumanProjectTaskAttempt|ResolveProjectTaskHumanWait)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 4 if files changed**

If this task only ran tests and changed nothing, do not create an empty commit. If tests or small code fixes were added:

```bash
git add apps/runtime-agent/tests/runtime_command_executor_test.rs apps/runtime-agent/src/commands/executor.rs apps/control-plane/internal/project
git commit -m "test: protect project task attempt command writeback"
```

## Task 5: Legacy Data Risk Check

**Files:**
- Create: `docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md`

- [ ] **Step 1: Inspect configured dev service status**

Run:

```bash
scripts/dev-services.sh status
```

Expected: report whether Control Plane and database are running. Do not start or restart services in this task unless the user explicitly asks.

- [ ] **Step 2: Identify the development database URL**

Check existing repo config and environment in read-only fashion:

```bash
rg -n "DATABASE_URL|postgres|control_plane" apps/control-plane/config .env* docker-compose* scripts --glob '!**/node_modules/**'
```

Expected: identify the safe local development database URL, or state that it cannot be confirmed.

- [ ] **Step 3: Run read-only SQL if a safe dev database is confirmed**

Use the repo's preferred DB tool if documented. The query should count non-command-driven active legacy tasks:

```sql
SELECT status, count(*) AS count
FROM tasks
WHERE deleted_at IS NULL
  AND status IN ('pending', 'claimed', 'running')
  AND COALESCE(params->>'provider_run_protocol', '') <> 'provider-run/v1'
GROUP BY status
ORDER BY status;
```

If no safe database can be confirmed, skip SQL and record the blocker.

- [ ] **Step 4: Write the risk report**

Create `docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md` with this structure:

```markdown
# Runtime Legacy Task Risk Check

日期：2026-07-01

## Scope

Read-only check for active non-command-driven rows in `tasks` before disabling legacy Runtime polling.

## Service Status

- Control Plane: Record the exact status line from `scripts/dev-services.sh status`.
- Database: Record the confirmed development database source, or state that no safe database URL was confirmed.

## Query

```sql
SELECT status, count(*) AS count
FROM tasks
WHERE deleted_at IS NULL
  AND status IN ('pending', 'claimed', 'running')
  AND COALESCE(params->>'provider_run_protocol', '') <> 'provider-run/v1'
GROUP BY status
ORDER BY status;
```

## Result

- Use one of these exact result shapes:
  - `No active non-command-driven legacy tasks were found.`
  - `Active non-command-driven legacy tasks were found: pending=N, claimed=N, running=N.`
  - `Blocked: no safe development database URL was confirmed, so SQL was not run.`

## Recommendation

- If zero rows: proceed with default no-op.
- If rows exist: decide whether to cancel, migrate to DigitalEmployee Run, or temporarily enable explicit legacy compatibility.
```
```

Replace the instruction sentences above with actual observed facts before saving the report.

- [ ] **Step 5: Run report self-check**

Run:

```bash
rg -n "Record the exact status|Use one of these exact result shapes|TODO|TBD|待定" docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md
git diff --check -- docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md
```

Expected: first command has no output; second command has no output.

- [ ] **Step 6: Commit Task 5**

```bash
git add docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md
git commit -m "docs: record legacy runtime task risk check"
```

## Task 6: Final Verification and Real-Chain Smoke

**Files:**
- No planned code changes
- Update report only if smoke evidence needs to be recorded

- [ ] **Step 1: Run static hygiene**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 2: Run targeted Rust tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_default_does_not_start_legacy_task_claim_loop --test daemon_test -- --nocapture
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 3: Run targeted Go tests**

Run:

```bash
go test ./apps/control-plane/internal/api/handlers -run 'TestClaimTask' -count=1
go test ./apps/control-plane/internal/api -run 'TestRuntimeRoutes|TestLegacyRuntimeClaimRouteIsNotRegistered|TestRuntimeClaim' -count=1
go test ./apps/control-plane/internal/project -run 'Test(StartProjectTaskAttempt|CompleteProjectTaskAttempt|SubmitProjectTaskAttemptResult|FailProjectTaskAttempt|WaitHumanProjectTaskAttempt|ResolveProjectTaskHumanWait)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Run contract verification**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 5: Verify running services are current before real-chain smoke**

Run:

```bash
scripts/dev-services.sh status
```

If Control Plane or Runtime Agent is running stale code, restart only the affected service:

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
```

Expected: services needed for the smoke are running current code.

- [ ] **Step 6: Run real ProjectTask execution smoke**

Use the existing project creation, demand submission, and dispatch path already used in this repo's runtime/project-task smoke tests. The smoke must prove:

- Runtime Agent receives `start_session` over `/api/v1/runtime/ws`.
- Runtime Agent does not call `/api/v1/runtime/tasks/claim`.
- ProjectTaskAttempt reaches `running`.
- Provider run reaches terminal.
- ProjectTaskAttempt reaches `succeeded`, `failed`, or `waiting_human`.
- `runtime_command_receipts.status` and `task_runs.status` reach compatible terminal states.

If a real Provider binary, credentials, auth token, runtime-ready digital employee, or safe workspace is unavailable, stop and report blocked. Do not replace this with a fake Provider and call the chain usable.

- [ ] **Step 7: Record verification evidence**

If smoke succeeds, record the exact commands, API calls, and final statuses in the final response. If smoke is blocked, record the missing dependency and the strongest local tests that passed.

- [ ] **Step 8: Final commit if verification docs changed**

If Task 6 adds smoke evidence to a report:

```bash
git add docs/superpowers/reports/2026-07-01-runtime-legacy-task-risk-check.md
git commit -m "docs: record runtime execution smoke evidence"
```

Do not commit if no files changed.

## Completion Criteria

- Runtime Agent default startup does not execute legacy polling.
- Control Plane `/api/v1/runtime/tasks/claim` returns no task by default.
- OpenAPI marks legacy runtime task endpoints deprecated.
- ProjectTaskAttempt command-driven path has passing targeted tests.
- Legacy active task data risk is checked or explicitly blocked by unknown database configuration.
- Real-chain smoke succeeds before claiming the execution path is usable; otherwise the work is reported as blocked, not complete.
