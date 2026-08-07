# ProjectTask Runtime Attempt Contract Implementation Plan

> 复核状态：06-20 ProjectTask durable closure基础落地

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch Runtime project-task writeback from project-task-id endpoints to attempt-aware endpoints with `attempt_id`, `lease_token`, and Runtime node validation.

**Architecture:** This is phase 2 of `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md` and depends on `docs/superpowers/plans/2026-06-20-project-task-durable-closure-control-plane.md`. Control Plane exposes attempt endpoints and removes old project-task writeback routes. Runtime Agent consumes attempt metadata and writes started, lease, complete, and fail events through attempt endpoints.

**Tech Stack:** Go chi handlers, OpenAPI/oapi-codegen, sqlc-backed project service, Rust Runtime Agent, reqwest, Axum tests, `corepack pnpm verify:contracts`.

---

## Source Spec

Implement this plan against:

- `docs/superpowers/specs/2026-06-20-project-task-durable-closure-design.md`
- `docs/superpowers/plans/2026-06-20-project-task-durable-closure-control-plane.md`

## File Structure

Modify:

- `contracts/control-plane/openapi.yaml`
  - Removes old Runtime project-task writeback paths.
  - Adds attempt started, lease, complete, and fail endpoints.
- `apps/control-plane/internal/api/server.go`
  - Registers attempt routes under `/api/v1/runtime/project-task-attempts/{attemptId}`.
- `apps/control-plane/internal/project/handler.go`
  - Adds attempt-aware handler methods and request bodies.
- `apps/control-plane/internal/project/handler_test.go`
  - Tests route parsing and service request construction.
- `apps/control-plane/internal/project/types.go`
  - Adds attempt writeback request types.
- `apps/control-plane/internal/project/repository.go`
  - Adds repository methods for started, lease, complete, and fail attempt writeback.
- `apps/control-plane/internal/project/pg_repository.go`
  - Implements attempt writeback transactions.
- `apps/control-plane/internal/project/service.go`
  - Validates active attempt, lease token, runtime node, idempotency key, and terminal idempotency.
- `apps/control-plane/internal/project/service_test.go`
  - Tests successful started/lease/complete/fail and mismatch rejection.
- `apps/control-plane/internal/storage/queries/project.sql`
  - Adds attempt update queries.
- `apps/control-plane/internal/storage/queries/*.go`
  - Regenerate through `make -C apps/control-plane generate-sqlc`.
- `apps/runtime-agent/src/controlplane/client.rs`
  - Replaces `complete_project_task` and `fail_project_task` URLs with attempt endpoints.
- `apps/runtime-agent/src/commands/executor.rs`
  - Extends project task context with `attempt_id`, `lease_token`, and context packet version.
- `apps/runtime-agent/tests/runtime_command_executor_test.rs`
  - Updates fake server routes and assertions.
- `CHANGELOG.md`
  - Adds dated Runtime contract entry.

## Task 1: Change OpenAPI And Route Registration

**Files:**

- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`

- [ ] **Step 1: Add failing route test**

In `apps/control-plane/internal/api/routes_test.go`, replace old project-task runtime route assertions with:

```go
func TestRuntimeProjectTaskAttemptRoutesRegistered(t *testing.T) {
	attemptID := "11111111-1111-4111-8111-111111111111"
	routes := routeFixtures(t)
	require.Contains(t, routes, http.MethodPost+" /api/v1/runtime/project-task-attempts/"+attemptID+"/started")
	require.Contains(t, routes, http.MethodPost+" /api/v1/runtime/project-task-attempts/"+attemptID+"/lease")
	require.Contains(t, routes, http.MethodPost+" /api/v1/runtime/project-task-attempts/"+attemptID+"/complete")
	require.Contains(t, routes, http.MethodPost+" /api/v1/runtime/project-task-attempts/"+attemptID+"/fail")
	require.NotContains(t, routes, http.MethodPost+" /api/v1/runtime/project-tasks/"+attemptID+"/complete")
	require.NotContains(t, routes, http.MethodPost+" /api/v1/runtime/project-tasks/"+attemptID+"/fail")
}
```

If `routeFixtures` is not the helper name in this file, adapt to the existing route enumeration helper and keep the same assertions.

- [ ] **Step 2: Run route test and verify it fails**

Run:

```bash
cd apps/control-plane && go test ./internal/api -run TestRuntimeProjectTaskAttemptRoutesRegistered -count=1
```

Expected: FAIL because new routes are not registered and old routes still exist.

- [ ] **Step 3: Update OpenAPI paths**

In `contracts/control-plane/openapi.yaml`, delete:

```yaml
/api/v1/runtime/project-tasks/{projectTaskId}/complete:
/api/v1/runtime/project-tasks/{projectTaskId}/fail:
/api/v1/runtime/project-tasks/{projectTaskId}/transfer-requests:
```

Add:

```yaml
  /api/v1/runtime/project-task-attempts/{attemptId}/started:
    post:
      operationId: startProjectTaskAttempt
      summary: Mark a ProjectTask attempt as started by Runtime
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/StartProjectTaskAttemptRequest"
      responses:
        "202":
          description: Attempt started
  /api/v1/runtime/project-task-attempts/{attemptId}/lease:
    post:
      operationId: renewProjectTaskAttemptLease
      summary: Renew a ProjectTask attempt lease
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/RenewProjectTaskAttemptLeaseRequest"
      responses:
        "204":
          description: Lease renewed
  /api/v1/runtime/project-task-attempts/{attemptId}/complete:
    post:
      operationId: completeProjectTaskAttempt
      summary: Complete a ProjectTask attempt
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CompleteProjectTaskAttemptRequest"
      responses:
        "202":
          description: Attempt completion accepted
  /api/v1/runtime/project-task-attempts/{attemptId}/fail:
    post:
      operationId: failProjectTaskAttempt
      summary: Fail a ProjectTask attempt
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/FailProjectTaskAttemptRequest"
      responses:
        "202":
          description: Attempt failure accepted
```

Add schemas:

```yaml
    ProjectTaskAttemptRuntimeFields:
      type: object
      required:
        - project_task_id
        - lease_token
        - runtime_node_id
        - idempotency_key
      properties:
        project_task_id:
          type: string
          format: uuid
        lease_token:
          type: string
        runtime_node_id:
          type: string
          format: uuid
        idempotency_key:
          type: string
        provider_session_id:
          type: string
    StartProjectTaskAttemptRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
    RenewProjectTaskAttemptLeaseRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
        - type: object
          properties:
            lease_expires_at:
              type: string
              format: date-time
    CompleteProjectTaskAttemptRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
        - type: object
          required:
            - conclusion
          properties:
            conclusion:
              type: string
            evidence_refs:
              type: array
              items: {}
            artifact_refs:
              type: array
              items: {}
            confidence_factors:
              type: object
              additionalProperties: true
            uncertainty:
              type: string
            missing_information:
              type: array
              items: {}
            recommended_next_action:
              type: string
            requires_human_review:
              type: boolean
    FailProjectTaskAttemptRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
        - type: object
          required:
            - failure_summary
            - failure_family
          properties:
            failure_summary:
              type: string
            failure_family:
              type: string
            retryable:
              type: boolean
```

- [ ] **Step 4: Register routes**

In `apps/control-plane/internal/api/server.go`, replace old project-task runtime routes with:

```go
r.Route("/project-task-attempts/{attemptId}", func(r chi.Router) {
	r.Post("/started", s.projectHandler.StartProjectTaskAttempt)
	r.Post("/lease", s.projectHandler.RenewProjectTaskAttemptLease)
	r.Post("/complete", s.projectHandler.CompleteProjectTaskAttempt)
	r.Post("/fail", s.projectHandler.FailProjectTaskAttempt)
})
```

- [ ] **Step 5: Run route and contract verification**

Run:

```bash
cd apps/control-plane && go test ./internal/api -run TestRuntimeProjectTaskAttemptRoutesRegistered -count=1
corepack pnpm verify:contracts
```

Expected: route test PASS; contract verification fails until handlers and generated code are updated, or passes if contract verifier only checks route shape.

- [ ] **Step 6: Commit route contract**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/server.go apps/control-plane/internal/api/routes_test.go
git commit -m "feat(api): add project task attempt runtime routes"
```

## Task 2: Add Control Plane Attempt Handlers And Service Methods

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add handler service interface methods**

In `apps/control-plane/internal/project/handler.go`, extend `HandlerService`:

```go
	StartProjectTaskAttempt(ctx context.Context, req StartProjectTaskAttemptRequest) (*ProjectTaskAttempt, error)
	RenewProjectTaskAttemptLease(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) error
	CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error)
	FailProjectTaskAttempt(ctx context.Context, req FailProjectTaskAttemptRequest) (*ProjectTask, error)
```

- [ ] **Step 2: Add request types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type ProjectTaskAttemptRuntimeRequest struct {
	TenantID       uuid.UUID
	AttemptID      uuid.UUID
	ProjectTaskID  uuid.UUID
	RuntimeNodeID  uuid.UUID
	LeaseToken     string
	IdempotencyKey string
	ProviderSessionID *string
}

type StartProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
}

type RenewProjectTaskAttemptLeaseRequest struct {
	ProjectTaskAttemptRuntimeRequest
	LeaseExpiresAt *time.Time
}

type CompleteProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID     uuid.UUID
	Conclusion            string
	EvidenceRefs          []any
	ArtifactRefs          []any
	ConfidenceFactors     map[string]any
	Uncertainty           string
	MissingInformation    []any
	RecommendedNextAction string
	RequiresHumanReview   bool
}

type FailProjectTaskAttemptRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID uuid.UUID
	FailureSummary    string
	FailureFamily     string
	Retryable         *bool
}
```

- [ ] **Step 3: Add handler tests**

In `apps/control-plane/internal/project/handler_test.go`, add tests for one endpoint and mirror the pattern for the other three:

```go
func TestStartProjectTaskAttemptHandlerBuildsServiceRequest(t *testing.T) {
	tenantID := uuid.New()
	attemptID := uuid.New()
	taskID := uuid.New()
	nodeID := uuid.New()
	service := &handlerTestService{}
	handler := NewHTTPHandler(service)
	body := strings.NewReader(`{
		"project_task_id":"` + taskID.String() + `",
		"runtime_node_id":"` + nodeID.String() + `",
		"lease_token":"lease-token-1",
		"idempotency_key":"attempt-start-1"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/started", body)
	req = req.WithContext(context.WithValue(req.Context(), tenantIDContextKey{}, tenantID))
	resp := httptest.NewRecorder()

	handler.StartProjectTaskAttempt(resp, req)

	require.Equal(t, http.StatusAccepted, resp.Code)
	require.Equal(t, attemptID, service.startProjectTaskAttemptReq.AttemptID)
	require.Equal(t, taskID, service.startProjectTaskAttemptReq.ProjectTaskID)
	require.Equal(t, nodeID, service.startProjectTaskAttemptReq.RuntimeNodeID)
	require.Equal(t, "lease-token-1", service.startProjectTaskAttemptReq.LeaseToken)
}
```

- [ ] **Step 4: Implement handler methods**

In `apps/control-plane/internal/project/handler.go`, add methods that parse `attemptId`, decode JSON, read tenant/runtime context, and call service. Return `202 Accepted` for started/complete/fail and `204 No Content` for lease.

- [ ] **Step 5: Add service validation tests**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestStartProjectTaskAttemptRejectsWrongLeaseToken(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	nodeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{ID: taskID, TenantID: tenantID, Status: ProjectTaskStatusQueued, CurrentAttemptID: &attemptID})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		Status:        ProjectTaskAttemptStatusQueued,
		LeaseToken:    "expected-token",
	})

	_, err = service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       tenantID,
			AttemptID:      attemptID,
			ProjectTaskID:  taskID,
			RuntimeNodeID:  nodeID,
			LeaseToken:     "wrong-token",
			IdempotencyKey: "start-1",
		},
	})
	require.ErrorIs(t, err, ErrProjectConflict)
}
```

- [ ] **Step 6: Implement service methods**

In `apps/control-plane/internal/project/service.go`, add validation helper:

```go
func (s *Service) validateAttemptRuntimeRequest(ctx context.Context, req ProjectTaskAttemptRuntimeRequest) (ProjectTask, ProjectTaskAttempt, error) {
	if req.TenantID == uuid.Nil || req.AttemptID == uuid.Nil || req.ProjectTaskID == uuid.Nil || req.RuntimeNodeID == uuid.Nil {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	if strings.TrimSpace(req.LeaseToken) == "" || strings.TrimSpace(req.IdempotencyKey) == "" {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrInvalidProject
	}
	task, err := s.repo.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	attempt, err := s.repo.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID)
	if err != nil {
		return ProjectTask{}, ProjectTaskAttempt{}, err
	}
	if attempt.ProjectTaskID != req.ProjectTaskID || attempt.LeaseToken != req.LeaseToken {
		return ProjectTask{}, ProjectTaskAttempt{}, ErrProjectConflict
	}
	return task, attempt, nil
}
```

Implement each service method by calling this helper and then repository writeback methods from Task 3.

- [ ] **Step 7: Run handler and service tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'ProjectTaskAttempt|StartProjectTaskAttempt|RenewProjectTaskAttemptLease|CompleteProjectTaskAttempt|FailProjectTaskAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit handlers and service methods**

Run:

```bash
git add apps/control-plane/internal/project
git commit -m "feat(control-plane): handle project task attempt writebacks"
```

## Task 3: Add Repository Attempt Writeback Transactions

**Files:**

- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`

- [ ] **Step 1: Add repository tests**

In `apps/control-plane/internal/project/pg_repository_test.go`, add:

```go
func TestStartProjectTaskAttemptAdvancesTaskAndAttempt(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectForRepositoryTest(t, repo, tenantID)
	task, attempt := createQueuedProjectTaskAttemptForRepositoryTest(t, repo, tenantID, projectID)
	nodeID := uuid.New()

	started, err := repo.StartProjectTaskAttemptWriteback(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       tenantID,
			AttemptID:      attempt.ID,
			ProjectTaskID:  task.ID,
			RuntimeNodeID:  nodeID,
			LeaseToken:     attempt.LeaseToken,
			IdempotencyKey: "start-" + attempt.ID.String(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Attempt.Status)
	require.Equal(t, ProjectTaskStatusRunning, started.Task.Status)
}
```

- [ ] **Step 2: Add sqlc update queries**

Append queries:

```sql
-- name: StartProjectTaskAttempt :one
UPDATE project_task_attempts
SET status = 'running',
    runtime_node_id = sqlc.arg('runtime_node_id')::uuid,
    provider_session_id = COALESCE(sqlc.narg('provider_session_id')::varchar, provider_session_id),
    started_at = COALESCE(started_at, NOW()),
    renewed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status = 'queued'
RETURNING *;

-- name: RenewProjectTaskAttemptLease :one
UPDATE project_task_attempts
SET lease_expires_at = sqlc.narg('lease_expires_at')::timestamptz,
    renewed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status IN ('queued', 'running')
RETURNING *;

-- name: FinishProjectTaskAttempt :one
UPDATE project_task_attempts
SET status = sqlc.arg('status')::varchar,
    provider_session_id = COALESCE(sqlc.narg('provider_session_id')::varchar, provider_session_id),
    finished_at = NOW(),
    retryable = sqlc.narg('retryable')::boolean,
    failure_family = sqlc.narg('failure_family')::varchar,
    failure_message = sqlc.narg('failure_message')::text,
    terminal_event_id = sqlc.narg('terminal_event_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status IN ('queued', 'running')
RETURNING *;
```

- [ ] **Step 3: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: exits 0.

- [ ] **Step 4: Implement repository methods**

Add repository methods:

```go
	StartProjectTaskAttemptWriteback(ctx context.Context, req StartProjectTaskAttemptRequest) (ProjectTaskAttemptWritebackResult, error)
	RenewProjectTaskAttemptLeaseWriteback(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) (ProjectTaskAttempt, error)
	CompleteProjectTaskAttemptWriteback(ctx context.Context, req CompleteProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error)
	FailProjectTaskAttemptWriteback(ctx context.Context, req FailProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error)
```

Use one transaction per method. `StartProjectTaskAttemptWriteback` updates attempt to `running`, then updates task status to `running` only from `queued`. Complete/fail should reuse existing event and execution summary creation logic, but gate by current attempt and lease token.

- [ ] **Step 5: Run repository tests**

Run:

```bash
cd apps/control-plane && go test ./internal/project -run 'ProjectTaskAttemptWriteback|StartProjectTaskAttemptAdvancesTaskAndAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit repository writebacks**

Run:

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/project
git commit -m "feat(control-plane): persist attempt writebacks"
```

## Task 4: Switch Runtime Agent To Attempt Endpoints

**Files:**

- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Update test fake routes**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, change fake server routes from:

```rust
"/api/v1/runtime/project-tasks/{project_task_id}/complete"
"/api/v1/runtime/project-tasks/{project_task_id}/fail"
```

to:

```rust
"/api/v1/runtime/project-task-attempts/{attempt_id}/complete"
"/api/v1/runtime/project-task-attempts/{attempt_id}/fail"
```

Update `CapturedProjectTaskWriteback` to:

```rust
struct CapturedProjectTaskWriteback {
    attempt_id: String,
    project_task_id: String,
    lease_token: String,
    runtime_node_id: String,
    idempotency_key: String,
    body: serde_json::Value,
}
```

- [ ] **Step 2: Update test payload metadata**

In project-task runtime command payloads, replace metadata:

```json
{
  "source": "project_task_dispatch",
  "project_task_id": "55555555-5555-4555-8555-555555555555",
  "handoff_contract": {"completion_path": "project_task_writeback"}
}
```

with:

```json
{
  "source": "project_task_dispatch",
  "project_task_id": "55555555-5555-4555-8555-555555555555",
  "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
  "project_task_lease_token": "lease-token-1",
  "runtime_node_id": "77777777-7777-4777-8777-777777777777",
  "execution_context_packet_version": "v1",
  "handoff_contract": {"completion_path": "project_task_attempt_writeback"}
}
```

- [ ] **Step 3: Run Runtime test and verify it fails**

Run:

```bash
cd apps/runtime-agent && cargo test --test runtime_command_executor_test start_session_completes_project_task_when_metadata_requests_writeback -- --nocapture
```

Expected: FAIL because runtime client still calls old project-task endpoints.

- [ ] **Step 4: Update Runtime writeback context**

In `apps/runtime-agent/src/commands/executor.rs`, extend `ProjectTaskWritebackContext`:

```rust
struct ProjectTaskWritebackContext {
    project_task_id: String,
    attempt_id: String,
    lease_token: String,
    runtime_node_id: String,
    digital_employee_id: String,
    expected_outputs: Vec<serde_json::Value>,
    handoff_contract: serde_json::Value,
    execution_context_packet_version: String,
}
```

Update `project_task_writeback_context_from_metadata` to require:

- `project_task_id`
- `project_task_attempt_id`
- `project_task_lease_token`
- `runtime_node_id`

and to accept only `completion_path = "project_task_attempt_writeback"` or an omitted completion path.

- [ ] **Step 5: Update Runtime client methods**

In `apps/runtime-agent/src/controlplane/client.rs`, replace old methods with:

```rust
pub async fn complete_project_task_attempt(
    &self,
    attempt_id: &str,
    body: &ProjectTaskCompleteWriteback,
) -> Result<()> {
    let url = format!("{}/api/v1/runtime/project-task-attempts/{}/complete", self.base_url, attempt_id);
    self.post_json(&url, body).await
}

pub async fn fail_project_task_attempt(
    &self,
    attempt_id: &str,
    body: &ProjectTaskFailWriteback,
) -> Result<()> {
    let url = format!("{}/api/v1/runtime/project-task-attempts/{}/fail", self.base_url, attempt_id);
    self.post_json(&url, body).await
}
```

If `post_json` is not available, use the existing request-building pattern from `complete_project_task` and `fail_project_task`.

- [ ] **Step 6: Add required writeback fields**

In `project_task_complete_writeback` and `project_task_fail_writeback`, include:

```rust
project_task_id: context.project_task_id.clone(),
lease_token: context.lease_token.clone(),
runtime_node_id: context.runtime_node_id.clone(),
idempotency_key: format!("project-task-attempt:{}:complete:{}", context.attempt_id, command_id),
```

For fail:

```rust
idempotency_key: format!("project-task-attempt:{}:fail:{}", context.attempt_id, command_id),
failure_family: "runtime_agent_failure".to_string(),
```

- [ ] **Step 7: Run Runtime tests**

Run:

```bash
cd apps/runtime-agent && cargo test --test runtime_command_executor_test project_task -- --nocapture
```

Expected: PASS.

- [ ] **Step 8: Commit Runtime switch**

Run:

```bash
git add apps/runtime-agent/src/controlplane/client.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat(runtime-agent): write back project task attempts"
```

## Task 5: Remove Old ProjectTask Writeback Paths

**Files:**

- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Delete old handler methods**

Remove:

```go
CompleteProjectTask(ctx context.Context, req CompleteProjectTaskRequest) (*ExecutionSummary, error)
FailProjectTask(ctx context.Context, req FailProjectTaskRequest) (*ProjectTask, error)
RequestProjectTaskTransfer(ctx context.Context, req RequestProjectTaskTransferRequest) (*TransferRequest, error)
```

from the Runtime-facing handler service surface. Keep project-domain methods only if still used by non-Runtime workflows; if retained, do not register them under `/api/v1/runtime/project-tasks`.

- [ ] **Step 2: Remove old Runtime client methods**

Remove Rust client methods:

```rust
complete_project_task
fail_project_task
```

and all tests that assert old `/api/v1/runtime/project-tasks/{id}` paths.

- [ ] **Step 3: Search for old paths**

Run:

```bash
rg -n "/api/v1/runtime/project-tasks|project-tasks/\\{projectTaskId\\}/complete|project-tasks/\\{projectTaskId\\}/fail|transfer-requests" contracts apps/control-plane apps/runtime-agent
```

Expected: no old Runtime project-task writeback path remains. Project transfer request APIs outside Runtime may still exist under project routes; review each match and keep only non-Runtime routes.

- [ ] **Step 4: Run route, contract, Go, and Rust tests**

Run:

```bash
corepack pnpm verify:contracts
cd apps/control-plane && go test ./internal/api ./internal/project -count=1
cd apps/runtime-agent && cargo test --test runtime_command_executor_test project_task -- --nocapture
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit old path removal**

Run:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane apps/runtime-agent
git commit -m "refactor(runtime): remove legacy project task writeback paths"
```

## Task 6: Phase Verification

**Files:**

- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add:

```markdown
- [YYYY-MM-DD HH:MM] Runtime ProjectTask writeback now uses attempt-aware endpoints with lease token and runtime node validation.
```

- [ ] **Step 2: Run full phase gates**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
cd apps/control-plane && go test ./internal/api ./internal/project -count=1
cd apps/runtime-agent && cargo test --test runtime_command_executor_test project_task -- --nocapture
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit verification changes**

Run:

```bash
git add CHANGELOG.md apps/web/src/lib/api/generated apps/control-plane/internal/api
git commit -m "chore: verify project task attempt runtime contract"
```

If generated API files did not change after `corepack pnpm generate:control-plane`, include only files with actual diffs.
