# Digital Employee Provider And Project Runtime Dispatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Provider a fixed digital-employee identity fact, remove Runtime as a required employee creation binding, and dispatch ProjectTasks to Runtime nodes through Project placement and Runtime capabilities.

**Architecture:** Move `provider_type` from execution-instance authority to `digital_employees`, keep execution instances only as a compatibility/read model, and introduce a ProjectTask dispatch preflight that resolves `(project, digital_employee.provider_type)` to a Runtime node at attempt time. The first implementation keeps ordinary non-project employee runs on the legacy execution-instance path as workbench/debug behavior; ProjectTask dispatch becomes the durable business execution path that records Project, employee, Provider, and Runtime on each attempt.

**Tech Stack:** Go Control Plane, sqlc/Atlas Postgres migrations, React/TypeScript Web console, Rust Runtime Agent contract tests, `corepack pnpm` verification scripts.

---

## File Structure

- Create `apps/control-plane/internal/storage/migrations/045_digital_employee_provider_identity.sql`: add `digital_employees.provider_type`, backfill from active execution instances, add attempt audit columns, and preserve execution-instance compatibility.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: update Atlas migration checksums after adding migration `045`.
- Modify `apps/control-plane/internal/storage/queries/employee_execution.sql`: insert/select employee `provider_type`, add Provider-first create options, add ProjectTask dispatch preflight query, and keep legacy run preflight separate.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: add `digital_employee_id` and `provider_type` to ProjectTask attempt creation/list/read queries.
- Regenerate `apps/control-plane/internal/storage/queries/*.go`: run `make -C apps/control-plane generate-sqlc` or the repo-equivalent sqlc command.
- Modify `apps/control-plane/internal/employee/types.go`: expose `ProviderType` on `DigitalEmployee`.
- Modify `apps/control-plane/internal/employee/repository.go`: add `ProviderType` to create/record structs and add a ProjectTask dispatch preflight repository method.
- Modify `apps/control-plane/internal/employee/pg_repository.go`: map `provider_type`, implement the new preflight, and keep `BindExecutionInstance` from mutating employee Provider.
- Modify `apps/control-plane/internal/employee/service.go`: normalize Provider as required identity input, create employees without Runtime provisioning, and keep initial configs/environment/workspace facts.
- Modify `apps/control-plane/internal/employee/handler.go`: accept create requests without `runtime_node_id`, return employee `provider_type`, and reframe create options as Provider candidates plus Runtime dispatch preview.
- Modify `apps/control-plane/internal/employee/service_test.go`, `apps/control-plane/internal/employee/pg_repository_test.go`, and `apps/control-plane/internal/api/employee_routes_test.go`: cover create-without-Runtime, Provider immutability, and compatibility behavior.
- Modify `apps/control-plane/internal/employee/run_types.go`, `apps/control-plane/internal/employee/run_repository.go`, `apps/control-plane/internal/employee/run_service.go`, and `apps/control-plane/internal/employee/pg_run_repository.go`: add a ProjectTask-specific run-start path that accepts the resolved Runtime/Provider preflight instead of reading employee execution instance.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: call the ProjectTask dispatch run-start path and include resolved Provider in metadata/packet.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: verify ProjectTask dispatch resolves Runtime from Project placement and employee Provider, not execution instance.
- Modify `apps/control-plane/internal/project/types.go`, `apps/control-plane/internal/project/repository.go`, and `apps/control-plane/internal/project/pg_repository.go`: store and map `digital_employee_id` and `provider_type` on `ProjectTaskAttempt`.
- Modify `apps/control-plane/internal/project/service_test.go`, `apps/control-plane/internal/project/pg_repository_test.go`, and `apps/control-plane/internal/project/handler_test.go`: prove attempt audit facts survive queue/read/writeback paths.
- Modify `apps/web/src/lib/api/employees.ts`: make `runtime_node_id` optional or absent in create input and require `provider_type`.
- Modify `apps/web/src/features/employees/create.tsx`: replace Runtime binding with Provider selection and Runtime dispatch preview.
- Modify `apps/web/src/features/employees/create.test.tsx`: prove create submit contains Provider and omits Runtime.
- Modify `apps/runtime-agent/tests/runtime_command_executor_test.rs`: assert ProjectTask command payload still carries `project_id`, `project_task_attempt_id`, `digital_employee_id`, `provider_type`, and `runtime_node_id`.
- Do not modify Provider adapter implementations unless the command payload shape test exposes an actual mismatch.

## Review Corrections (2026-07-03)

A plan review against current code confirmed the core premise (ProjectTask dispatch today really does resolve Runtime through the legacy execution-instance path, not project placement) but found the following inaccuracies, which are corrected in the task sections below:

- **Unresolved blocker, not addressed by the original draft:** `buildStartSessionPayload` (`apps/control-plane/internal/employee/run_service.go:852,856`) sends `preflight.AgentHomeDir` and `preflight.ExecutionInstanceID` straight into the Runtime `start_session` command payload. Runtime Agent's `ensure_command_instance` (`apps/runtime-agent/src/commands/executor.rs:938-960`) hard-fails the command (`"agent_home_dir is required"` / `"agent_home_dir does not exist"`) unless `agent_home_dir` is a non-empty path that already exists on disk on that Runtime node. The new `GetProjectTaskRunPreflight`/`StartProjectTaskRunPreflight` this plan introduces has no source for `AgentHomeDir` (digital employees carry no execution-instance binding after Task 2), so a real ProjectTask dispatch built on Task 4 as originally drafted will fail at Runtime with that exact error. This is a genuine open design question (see project memory `project-code-workspace-runtime-affinity`, spec status 待评审/未落地) — Task 4 below now includes an explicit blocking step for it instead of silently shipping a broken payload.
- Task 3 Step 9's test asserted the wrong `CreationChecks` index, and Step 8's replacement check condition was a copy-paste of the unrelated `capability_policy` condition — both fixed below.
- Task 4's plan referenced `StartProjectTaskRunRequest`/`StartProjectTaskRunResult` as if they already existed in `apps/control-plane/internal/employee/`. They only exist in `apps/control-plane/internal/workflow/projectcoordination/types.go:319-340`, and `employee` has no such types today. Task 4 now defines distinct employee-package-local types and an explicit translation step in `app.go`, and adds `workflow/projectcoordination/types.go` to the file list.
- Several referenced helper names don't exist under those exact names (`previewEffectiveConfig` → `previewEffectiveConfigWithRepository`; `employeeConfigInputFromCreateRequest` → use `initialEmployeeConfigInput`/`initialEmployeeConfigParams`; `uuidValue` → does not exist in `employee` package, must be added locally; `mapFromJSON` → `mapFromJSONB`/`mapFromJSONValue`; `CreateDigitalEmployeeRun` as a service method → actual method is `DigitalEmployeeRunService.CreateRun`) — corrected inline below.
- Task 7's Rust assertions used a nonexistent `run_context.metadata` path. The real struct is `RuntimeCommandRunContext`, reached via `snapshot.command_context` in tests, and `digital_employee_id`/`provider_type`/`runtime_node_id` are top-level fields on it (populated from the payload in `executor.rs:296-303`), not nested under `.metadata`. Only `project_id`/`project_task_id`/`project_task_attempt_id`/`project_task_lease_token` live in `.metadata` today. Fixed below. Note this also means `digital_employee_id`/`provider_type` do **not** need new plumbing in `project_store.go`'s `runMetadata` map — `buildStartSessionPayload` already emits them as top-level payload keys once `preflight.ProviderType` is populated correctly by Task 4.
- Task 1's backfill silently defaults `provider_type` to `'codex'` for any employee with no matching execution instance and no `metadata.provider_type`. Since `provider_type` becomes immutable after Task 6, this is flagged as requiring explicit human confirmation against real data before running on a non-empty database, not a default to apply unattended.

## Current Baseline To Preserve

- ProjectDemand submission is already project-scoped and must stay the business task entry point.
- `project_members` already models a project digital-employee pool; dispatch must keep rejecting employees outside that pool.
- `project_placements` already supports one active Runtime placement per Project and many Projects per Runtime.
- Ordinary `POST /api/v1/digital-employees/{id}/runs` can remain a workbench/debug path for this implementation, but must not be described as ProjectTask execution.
- Runtime Agent remains an execution layer only. It does not choose employee, Project, Provider, or business policy.
- Control Plane does not execute local commands directly.

## Task 1: Provider Identity Migration And sqlc Models

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/045_digital_employee_provider_identity.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

- [ ] **Step 1: Add failing migration/query expectations**

Add this test to `apps/control-plane/internal/storage/migrations_test.go` near the existing table/column assertions:

```go
func TestDigitalEmployeeProviderIdentityMigrationObjects(t *testing.T) {
	sql := readMigration(t, "045_digital_employee_provider_identity.sql")
	require.Contains(t, sql, "ALTER TABLE digital_employees")
	require.Contains(t, sql, "ADD COLUMN provider_type VARCHAR(100)")
	require.Contains(t, sql, "UPDATE digital_employees de")
	require.Contains(t, sql, "project_task_attempts")
	require.Contains(t, sql, "digital_employee_id UUID")
	require.Contains(t, sql, "provider_type VARCHAR(100)")
}
```

- [ ] **Step 2: Run the migration object test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeProviderIdentityMigrationObjects -count=1
```

Expected: FAIL because migration `045_digital_employee_provider_identity.sql` does not exist yet.

- [ ] **Step 3: Create migration `045_digital_employee_provider_identity.sql`**

Create `apps/control-plane/internal/storage/migrations/045_digital_employee_provider_identity.sql`:

```sql
ALTER TABLE digital_employees
    ADD COLUMN provider_type VARCHAR(100);

WITH ranked_instances AS (
    SELECT
        digital_employee_id,
        provider_type,
        ROW_NUMBER() OVER (
            PARTITION BY tenant_id, digital_employee_id
            ORDER BY
                CASE status
                    WHEN 'ready' THEN 1
                    WHEN 'active' THEN 2
                    WHEN 'provisioning' THEN 3
                    ELSE 4
                END,
                updated_at DESC,
                created_at DESC
        ) AS rn
    FROM digital_employee_execution_instances
    WHERE deleted_at IS NULL
      AND COALESCE(provider_type, '') <> ''
)
UPDATE digital_employees de
SET provider_type = ranked_instances.provider_type
FROM ranked_instances
WHERE ranked_instances.digital_employee_id = de.id
  AND ranked_instances.rn = 1
  AND COALESCE(de.provider_type, '') = '';

UPDATE digital_employees
SET provider_type = COALESCE(NULLIF(metadata ->> 'provider_type', ''), 'codex')
WHERE provider_type IS NULL OR provider_type = '';

ALTER TABLE digital_employees
    ALTER COLUMN provider_type SET NOT NULL;

CREATE INDEX idx_digital_employees_provider_type
    ON digital_employees(tenant_id, provider_type, status)
    WHERE deleted_at IS NULL;

ALTER TABLE project_task_attempts
    ADD COLUMN digital_employee_id UUID,
    ADD COLUMN provider_type VARCHAR(100);

UPDATE project_task_attempts pta
SET
    digital_employee_id = pt.assigned_digital_employee_id,
    provider_type = COALESCE(
        NULLIF(pta.execution_context_packet ->> 'provider_type', ''),
        de.provider_type
    )
FROM project_tasks pt
LEFT JOIN digital_employees de
  ON de.tenant_id = pta.tenant_id
 AND de.id = pt.assigned_digital_employee_id
WHERE pt.tenant_id = pta.tenant_id
  AND pt.id = pta.project_task_id;

CREATE INDEX idx_project_task_attempts_employee
    ON project_task_attempts(tenant_id, digital_employee_id, status)
    WHERE digital_employee_id IS NOT NULL;

CREATE INDEX idx_project_task_attempts_provider
    ON project_task_attempts(tenant_id, provider_type, status)
    WHERE provider_type IS NOT NULL;

COMMENT ON COLUMN digital_employees.provider_type IS '数字员工创建时固定的 Provider 类型，例如 claude-code、opencode、codex；不表示 Runtime 绑定。';
COMMENT ON COLUMN project_task_attempts.digital_employee_id IS '本次项目任务尝试使用的数字员工 ID，冗余保存用于审计。';
COMMENT ON COLUMN project_task_attempts.provider_type IS '本次项目任务尝试使用的数字员工固定 Provider。';
```

Do not add a database check constraint for Provider names in this task. The design keeps Provider registration and server-side validation as the authority.

**Blocking check before running against real data:** the final fallback `COALESCE(NULLIF(metadata ->> 'provider_type', ''), 'codex')` silently assigns `codex` to any employee with no matching execution instance and no `metadata.provider_type`. Because `provider_type` becomes immutable once Task 6 lands, this default cannot be corrected later without a manual data fix. Before applying this migration to any non-empty database, run the backfill's `SELECT` portion read-only first and get explicit human sign-off on the count and identity of any employees that would fall through to the `'codex'` default — do not apply the `ALTER COLUMN ... SET NOT NULL` step until that is confirmed.

- [ ] **Step 4: Update employee create/select SQL**

In `apps/control-plane/internal/storage/queries/employee_execution.sql`, add `provider_type` to `CreateDigitalEmployee`:

```sql
-- name: CreateDigitalEmployee :one
INSERT INTO digital_employees (
    tenant_id,
    team_id,
    owner_user_id,
    employee_type,
    name,
    role,
    description,
    status,
    permission_policy,
    context_policy,
    approval_policy,
    risk_level,
    metadata,
    provider_type
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('owner_user_id')::uuid,
    sqlc.arg('employee_type')::varchar,
    sqlc.arg('name')::varchar,
    sqlc.arg('role')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('permission_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('risk_level')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.arg('provider_type')::varchar
) RETURNING *;
```

Keep `GetDigitalEmployee` and `ListDigitalEmployees` as `SELECT *`; sqlc will include the new column after regeneration.

- [ ] **Step 5: Update ProjectTask attempt SQL**

In `apps/control-plane/internal/storage/queries/project.sql`, find `CreateProjectTaskAttempt` and add the new columns to the insert:

```sql
digital_employee_id,
provider_type,
```

Add matching values:

```sql
sqlc.narg('digital_employee_id')::uuid,
sqlc.narg('provider_type')::varchar,
```

Keep all existing idempotency and active-attempt behavior unchanged.

- [ ] **Step 6: Regenerate sqlc outputs**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: generated Go files under `apps/control-plane/internal/storage/queries/` now expose `DigitalEmployee.ProviderType`, `CreateDigitalEmployeeParams.ProviderType`, `CreateProjectTaskAttemptParams.DigitalEmployeeID`, and `CreateProjectTaskAttemptParams.ProviderType`.

If `sqlc` is not on PATH, install or expose the repo-approved `sqlc v1.27.0` binary before continuing. Do not hand-edit generated files unless the environment cannot provide sqlc and the user explicitly accepts a manual generated-code patch.

- [ ] **Step 7: Update Atlas checksum**

Run:

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

Expected: `apps/control-plane/internal/storage/migrations/atlas.sum` changes and includes migration `045_digital_employee_provider_identity.sql`.

- [ ] **Step 8: Verify migration and generated code**

Run:

```bash
make -C apps/control-plane migrate-validate
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeProviderIdentityMigrationObjects -count=1
```

Expected: both commands PASS.

- [ ] **Step 9: Commit Task 1**

```bash
git add apps/control-plane/internal/storage/migrations/045_digital_employee_provider_identity.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/queries
git commit -m "db: add digital employee provider identity"
```

## Task 2: Control Plane Employee Creation Without Runtime Binding

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Test: `apps/control-plane/internal/employee/service_test.go`
- Test: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Add failing service test for creation without Runtime**

Add this test near existing create tests in `apps/control-plane/internal/employee/service_test.go`:

```go
func TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	repo.teams[teamID] = tenantID
	teamConfigID := uuid.New()
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:       &teamConfigID,
		TenantID: tenantID,
		TeamID:   teamID,
		Status:   TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{
			"allowed_employee_types": []any{"requirements_analyst"},
			"allowed_provider_types": []any{"codex"},
		},
		RuntimeScopePolicy: map[string]any{
			"provider_types": []any{"codex"},
		},
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	service := NewService(repo, nil)

	employee, err := service.CreateDigitalEmployee(ctx, CreateDigitalEmployeeRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		EmployeeType:  "requirements_analyst",
		Name:          "需求分析员",
		AvatarAssetID: "avatar-orb-blue",
		ProviderType:  "codex",
		Role:          "负责需求澄清",
	})

	require.NoError(t, err)
	require.Equal(t, DigitalEmployeeStatusReady, employee.Status)
	require.Equal(t, "codex", employee.ProviderType)
	require.Empty(t, repo.executionInstances)
	require.Empty(t, repo.runtimeCommandReceipts)
}
```

Keep the assertions: no Runtime, Provider persisted, no provisioning instance, no runtime command receipt.

- [ ] **Step 2: Run the service test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding -count=1
```

Expected: FAIL with `runtime_node_id is required` or runtime preflight/dispatcher errors.

- [ ] **Step 3: Add Provider to employee domain types**

In `apps/control-plane/internal/employee/types.go`, add `ProviderType string` to `DigitalEmployee`.

In `apps/control-plane/internal/employee/repository.go`, add `ProviderType string` to:

```go
type CreateDigitalEmployeeParams struct { ... }
type DigitalEmployeeRecord struct { ... }
```

- [ ] **Step 4: Map Provider in pg repository**

In `apps/control-plane/internal/employee/pg_repository.go`:

Add `ProviderType: params.ProviderType` to `queries.CreateDigitalEmployeeParams`.

Add this field in `digitalEmployeeRecordFromQuery`:

```go
ProviderType: employee.ProviderType,
```

Add `ProviderType` to any test memory repository create path:

```go
record.ProviderType = params.ProviderType
```

- [ ] **Step 5: Normalize create input without Runtime**

In `normalizeCreateDigitalEmployeeRequest` in `apps/control-plane/internal/employee/service.go`, remove the `runtime_node_id is required` validation. Keep `provider_type is required`.

Normalize provider names with a helper:

```go
func normalizeProviderType(value string) string {
	return strings.TrimSpace(value)
}
```

Use it as:

```go
providerType := normalizeProviderType(req.ProviderType)
if providerType == "" {
	return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
}
req.ProviderType = providerType
```

Do not normalize `claude_code` to `claude-code` in this task. Record current values as-is and decide naming separately only if tests or live data require it.

- [ ] **Step 6: Split local employee fact creation from provisioning**

Change `CreateDigitalEmployee` so it does not call `GetRuntimeProvisioningPreflight`, does not require `s.dispatcher`, and does not call `dispatchRuntimeProvisioningCommand`.

Add a new helper that returns only the employee record:

```go
func (s *Service) createReadyEmployeeIdentityFacts(ctx context.Context, repository Repository, req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) (DigitalEmployeeRecord, error) {
	record, err := repository.CreateDigitalEmployee(ctx, createDigitalEmployeeParams(req))
	if err != nil {
		return DigitalEmployeeRecord{}, fmt.Errorf("create digital employee: %w", err)
	}
	configRevision, err := s.createInitialActiveConfigRevision(ctx, repository, record, req, definition, teamConfig)
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	preview, err := s.previewEffectiveConfig(ctx, record, teamConfig, employeeConfigInputFromCreateRequest(req, definition), uuid.New())
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	if len(preview.Validation.BlockingErrors) != 0 {
		return DigitalEmployeeRecord{}, fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
	}
	if _, err := createApprovedEffectiveConfig(ctx, repository, record, teamConfig.ID, configRevision.ID, preview, req.OwnerUserID); err != nil {
		return DigitalEmployeeRecord{}, err
	}
	if err := s.createInitialEnvironmentVariables(ctx, repository, record, req); err != nil {
		return DigitalEmployeeRecord{}, err
	}
	return repository.UpdateDigitalEmployeeStatus(ctx, req.TenantID, record.ID, DigitalEmployeeStatusReady)
}
```

Then make `CreateDigitalEmployee` call it inside the existing transaction and return `employeeFromRecord(record)`.

The pseudocode above uses illustrative helper names that do not match current code exactly: the real preview helper is `previewEffectiveConfigWithRepository(ctx, repository Repository, teamConfig, employeeConfig EmployeeConfigInput)` (`service.go:662`, no `record`/`uuid.New()` args — it takes a `Repository` and a fully-built `EmployeeConfigInput`), and there is no `employeeConfigInputFromCreateRequest`; the existing `initialEmployeeConfigInput(req, definition, teamConfig, employeeID, configID, revisionNumber)` (`service.go:781`) builds the equivalent input from more arguments than shown here. Read `service.go:640-820` before writing this step and wire the real signatures rather than the args shown above.

Keep `createProvisioningInstanceAndReceipt`, `buildProvisionInstancePayload`, and provisioning writeback code in place for legacy bind/provisioning paths until a later cleanup task removes them.

- [ ] **Step 7: Include Provider in create params and domain mapping**

In `createDigitalEmployeeParams`, add:

```go
ProviderType: req.ProviderType,
```

In `employeeFromRecord`, add:

```go
ProviderType: record.ProviderType,
```

- [ ] **Step 8: Update HTTP create handler**

In `apps/control-plane/internal/employee/handler.go`, leave `RuntimeNodeID uuid.UUID` in the decoded struct as a deprecated optional compatibility field. Do not reject missing value.

Ensure `employeeResponseFromDomain` includes:

```go
ProviderType: employee.ProviderType,
```

If `digitalEmployeeResponse` does not have `ProviderType`, add:

```go
ProviderType string `json:"provider_type"`
```

- [ ] **Step 9: Add API route test for missing Runtime**

In `apps/control-plane/internal/api/employee_routes_test.go`, add a create-route test that posts no `runtime_node_id`:

```go
func TestCreateDigitalEmployeeRouteAcceptsProviderWithoutRuntime(t *testing.T) {
	service := &employeeRouteTestService{
		createEmployee: &employee.DigitalEmployee{
			ID:           employeeID,
			TenantID:     tenantID,
			TeamID:       &teamID,
			Name:         "需求分析员",
			ProviderType: "codex",
			Status:       employee.DigitalEmployeeStatusReady,
		},
	}
	handler := newEmployeeRoutesTestHandler(t, service)
	req := newAuthorizedJSONRequest(http.MethodPost, "/api/v1/digital-employees", `{
		"team_id":"`+teamID.String()+`",
		"employee_type":"requirements_analyst",
		"name":"需求分析员",
		"avatar_asset_id":"avatar-orb-blue",
		"provider_type":"codex",
		"role":"负责需求澄清"
	}`)
	resp := httptest.NewRecorder()

	handler.CreateDigitalEmployee(resp, req)

	require.Equal(t, http.StatusCreated, resp.Code)
	require.Equal(t, uuid.Nil, service.createReq.RuntimeNodeID)
	require.Equal(t, "codex", service.createReq.ProviderType)
	require.NotContains(t, resp.Body.String(), "runtime_node_id")
	require.Contains(t, resp.Body.String(), `"provider_type":"codex"`)
}
```

If the existing route test harness uses different helper names, update only the harness calls. The request body and assertions must stay equivalent.

- [ ] **Step 10: Run targeted employee/API tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding|TestCreateDigitalEmployeeParamsAndDomainMappingKeepOwnerAndType' -count=1
go test ./apps/control-plane/internal/api -run 'TestCreateDigitalEmployeeRoute' -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit Task 2**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/api
git commit -m "feat: create digital employees with fixed provider"
```

## Task 3: Provider-Based create-options And Web Creation Flow

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/features/employees/create.tsx`
- Modify: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Add failing Web test for Provider-only submit**

In `apps/web/src/features/employees/create.test.tsx`, add:

```tsx
it("creates an employee with provider_type and without runtime_node_id", async () => {
  const { requests, user } = renderCreateEmployeePage({
    createOptions: createOptionsFixture({ runtimeAvailability: "none" }),
  });

  await user.click(await screen.findByRole("button", { name: /使用模板|选择模板|开始配置/ }));
  await user.click(screen.getByRole("radio", { name: /codex/i }));
  await user.click(screen.getByRole("button", { name: /创建|确认创建/ }));

  const createRequest = requests.find((request) => request.url.endsWith("/api/v1/digital-employees"));
  expect(createRequest).toBeTruthy();
  const body = JSON.parse(createRequest!.body as string);
  expect(body.provider_type).toBe("codex");
  expect(body.runtime_node_id).toBeUndefined();
});
```

Adapt labels to the existing UI text in the test file. The assertion must remain: create request contains `provider_type` and omits `runtime_node_id`.

- [ ] **Step 2: Run the Web test and confirm it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: FAIL because current UI requires Runtime and sends `runtime_node_id`.

- [ ] **Step 3: Loosen frontend create input**

In `apps/web/src/lib/api/employees.ts`, change:

```ts
runtime_node_id: string;
provider_type: string;
```

to:

```ts
runtime_node_id?: string;
provider_type: string;
```

Update `assertReadyCreateInput` so it no longer requires `runtime_node_id`:

```ts
if (
  !("employee_type" in input) ||
  !input.employee_type ||
  !("avatar_asset_id" in input) ||
  !input.avatar_asset_id ||
  !("provider_type" in input) ||
  !input.provider_type
) {
  throw new Error("digital employee ready creation requires employee_type, avatar_asset_id, and provider_type");
}
```

- [ ] **Step 4: Derive Provider candidates from create options**

In `apps/web/src/features/employees/create.tsx`, add:

```tsx
function providerCandidates(options: DigitalEmployeeCreateOptions | undefined) {
  const fromPolicy = options?.team_config.allowed_provider_types ?? [];
  const fromCapabilities = options?.capability_options.provider_types ?? [];
  const fromRuntimePreview = options?.runtime_provider_options.map((option) => option.provider_type) ?? [];
  return Array.from(new Set([...fromPolicy, ...fromCapabilities, ...fromRuntimePreview].filter(Boolean))).sort();
}

function providerDispatchPreview(options: DigitalEmployeeCreateOptions | undefined, providerType: string) {
  const runtimeOptions = options?.runtime_provider_options ?? [];
  const matching = runtimeOptions.filter((option) => option.provider_type === providerType);
  const available = matching.filter((option) => option.available);
  return {
    matchingCount: matching.length,
    availableCount: available.length,
  };
}
```

- [ ] **Step 5: Replace Runtime selection state with Provider selection**

In `WizardDraft`, keep `runtime_binding` and `runtime_node_id` only if other code still references them during this task, but do not require them. Ensure `provider_type` is the selected field.

Replace the auto-select effect:

```tsx
const providers = providerCandidates(createOptions.data);
setDraft((current) => {
  if (!current.provider_type && providers.length === 1) {
    return { ...current, provider_type: providers[0], runtime_binding: "", runtime_node_id: "" };
  }
  if (current.provider_type && !providers.includes(current.provider_type)) {
    return { ...current, provider_type: "", runtime_binding: "", runtime_node_id: "" };
  }
  return current;
});
```

Replace `selectRuntime` with:

```tsx
function selectProvider(providerType: string) {
  updateDraft({ provider_type: providerType, runtime_binding: "", runtime_node_id: "" });
  setErrors((current) => ({ ...current, runtime: undefined }));
}
```

- [ ] **Step 6: Submit Provider-only payload**

In the mutation body, remove `findRuntimeOption` and remove `runtime_node_id`.

Use:

```tsx
provider_type: draft.provider_type,
```

Do not include `runtime_node_id` in the object sent to `createDigitalEmployee`.

- [ ] **Step 7: Replace Runtime step copy and validation**

Rename the step label from `"运行"` to `"Provider"` or `"执行器"` consistently in `configSteps`.

Replace `RuntimeStep` with a Provider step that renders one radio item per `providerCandidates(options)`. Each item should display the Provider name and a small dispatch preview:

```tsx
const preview = providerDispatchPreview(options, providerType);
const hint =
  preview.availableCount > 0
    ? `${preview.availableCount}/${preview.matchingCount} 个 Runtime 当前可调度`
    : "当前没有在线 Runtime 支持该 Provider，创建后需要配置项目 Runtime placement";
```

Validation becomes:

```tsx
if (step === "Provider" && !draft.provider_type) {
  return { runtime: "请选择 Provider" };
}
```

In `validateDraftForCreate`, require `provider_type` and no longer require `runtime_binding`.

- [ ] **Step 8: Update backend create-options wording**

In `apps/control-plane/internal/employee/service.go`, keep `RuntimeProviderOptions` in `CreateOptions` as a dispatch preview, but update the existing `runtime_provider` check in `createOptionChecks` (`service.go:303-308`) so lack of available Runtime is a warning, not a block. The condition must stay keyed on Runtime availability (`availableRuntimeCount`), not be copy-pasted from the `capability_policy` check above it:

```go
{
	Key:     "runtime_provider",
	Label:   "Runtime 调度预览",
	Status:  checkStatus(availableRuntimeCount > 0, true),
	Message: fmt.Sprintf("%d/%d 个运行绑定可用，Runtime 在线状态仅影响后续项目任务调度", availableRuntimeCount, len(runtimeOptions)),
}
```

`checkStatus(passed, warning)` returns `"warning"` (not `"blocked"`) when `passed` is false and `warning` is true (`service.go:312-320`) — that `warning=true` is what actually stops missing Runtime from blocking employee creation; reusing another check's condition with `warning=false` (as an earlier draft of this step did) would still block. Do not remove `runtime_provider_options` from the response in this task.

- [ ] **Step 9: Update API create-options tests**

In `apps/control-plane/internal/api/employee_routes_test.go`, update tests that expect `runtime_provider` to be blocking when no Runtime is online. They should now assert that the response still includes `runtime_provider_options` but creation checks do not block employee identity creation solely because `available_count` is `0`.

Use this assertion shape. Note `createOptionChecks` currently returns checks in the order `team_governance`(0), `employee_templates`(1), `capability_policy`(2), `runtime_provider`(3) (`service.go:284-309`), so the index is `3`, not `2`:

```go
require.Equal(t, "runtime_provider", body.CreationChecks[3].Key)
require.NotEqual(t, "blocked", body.CreationChecks[3].Status)
```

- [ ] **Step 10: Run targeted tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestGetCreateOptions|TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding' -count=1
go test ./apps/control-plane/internal/api -run 'TestGetDigitalEmployeeCreateOptions|TestCreateDigitalEmployeeRoute' -count=1
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS.

- [ ] **Step 11: Commit Task 3**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/api apps/web/src/lib/api/employees.ts apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git commit -m "feat: select provider when creating employees"
```

## Task 4: ProjectTask Dispatch Runtime Selection

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Modify: `apps/control-plane/internal/employee/run_types.go`
- Modify: `apps/control-plane/internal/employee/run_repository.go`
- Modify: `apps/control-plane/internal/employee/pg_run_repository.go`
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Test: `apps/control-plane/internal/employee/run_service_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Type-location note:** `StartProjectTaskRunRequest`/`StartProjectTaskRunResult` already exist today, but only in `apps/control-plane/internal/workflow/projectcoordination/types.go:319-340` — they are consumed through the `ProjectTaskRunStarter` interface (`project_store.go:189`) whose current implementation, `projectTaskRunStarterAdapter` in `app.go:158-188`, just forwards to `employee.DigitalEmployeeRunService.CreateRun` (the legacy execution-instance path) and translates the result. The `employee` package has no `StartProjectTaskRun*` types today, and `projectcoordination` has no import dependency on `employee` reversed — so the new method added to `DigitalEmployeeRunService` in this task must use its own, employee-package-local request/result types (named `StartProjectTaskRunRequest`/`StartProjectTaskRunResult` inside package `employee` is fine — Go scopes them separately from `projectcoordination`'s same-named types since they're different packages, but keep this distinction explicit while implementing so the two are not confused for the same type). `app.go`'s adapter must translate between `projectcoordination.StartProjectTaskRunRequest` (its incoming argument) and the new `employee.StartProjectTaskRunRequest` (what it passes to the run service), and between `employee.StartProjectTaskRunResult` and `projectcoordination.StartProjectTaskRunResult` (its return value) — see Step 9.

- [ ] **Step 1: Add failing ProjectTask dispatch test**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add:

```go
func TestProjectStoreDispatchProjectTaskUsesProjectPlacementAndEmployeeProvider(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := newProjectStoreMemoryRepository(tenantID, projectID)
	repo.projects[projectID] = project.Project{
		ID:               projectID,
		TenantID:         tenantID,
		HumanOwnerUserID: uuid.New(),
		Status:           project.ProjectStatusRunning,
	}
	repo.demands[demandID] = project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: project.ProjectDemandStatusPlanned}
	repo.projectTasks[taskID] = project.ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		Status:                    project.ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		Title:                     "实现接口",
	}
	starter := &projectTaskRunStarterFake{
		result: StartProjectTaskRunResult{
			RunID:         uuid.New(),
			RuntimeTaskID: uuid.New(),
			RuntimeNodeID: runtimeNodeID,
			NodeID:        "runtime-a",
			ProviderType:  "codex",
		},
	}
	store := NewProjectStore(repo)
	store.runStarter = starter

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, employeeID, starter.req.DigitalEmployeeID)
	require.Equal(t, projectID, starter.req.ProjectID)
	require.Equal(t, "codex", repo.projectTaskAttempts[0].ProviderType)
	require.Equal(t, employeeID, *repo.projectTaskAttempts[0].DigitalEmployeeID)
	require.Equal(t, "codex", repo.projectTaskAttempts[0].ExecutionContextPacket["provider_type"])
}
```

Adapt memory repository field names to the existing test fake. The key assertions are: run starter returns Provider/Runtime, attempt stores employee/provider, packet includes Provider.

- [ ] **Step 2: Run the test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreDispatchProjectTaskUsesProjectPlacementAndEmployeeProvider -count=1
```

Expected: FAIL because `StartProjectTaskRunResult` lacks `ProviderType` or attempt does not store provider/employee.

- [ ] **Step 3: Add dispatch preflight SQL**

In `apps/control-plane/internal/storage/queries/employee_execution.sql`, add:

```sql
-- name: GetProjectTaskRunPreflight :one
WITH active_placement AS (
    SELECT pp.runtime_node_id
    FROM project_placements pp
    WHERE pp.tenant_id = sqlc.arg('tenant_id')::uuid
      AND pp.project_id = sqlc.arg('project_id')::uuid
      AND pp.placement_status = 'active'
    LIMIT 1
),
approved_config AS (
    SELECT
        dec.id AS effective_config_id,
        dec.effective_config_snapshot
    FROM digital_employee_effective_configs dec
    WHERE dec.tenant_id = sqlc.arg('tenant_id')::uuid
      AND dec.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
      AND dec.status = 'approved'
      AND dec.revoked_at IS NULL
    ORDER BY dec.created_at DESC, dec.updated_at DESC
    LIMIT 1
),
latest_active_session AS (
    SELECT DISTINCT re.runtime_node_id
    FROM runtime_enrollments re
    JOIN runtime_sessions rs
      ON rs.tenant_id = re.tenant_id
     AND rs.runtime_node_id = re.runtime_node_id
     AND rs.revoked_at IS NULL
     AND rs.expires_at > NOW()
    WHERE re.tenant_id = sqlc.arg('tenant_id')::uuid
      AND re.status = 'approved'
)
SELECT
    de.tenant_id,
    de.team_id,
    de.id AS digital_employee_id,
    de.status AS digital_employee_status,
    de.provider_type,
    rn.id AS runtime_node_id,
    rn.node_id,
    COALESCE(approved_config.effective_config_snapshot -> 'budget_policy', '{}'::jsonb)::jsonb AS budget_policy,
    COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
    'Asia/Shanghai'::text AS business_timezone,
    (approved_config.effective_config_id IS NOT NULL)::boolean AS has_approved_effective_config,
    (latest_active_session.runtime_node_id IS NOT NULL)::boolean AS runtime_session_active,
    EXISTS (
        SELECT 1
        FROM runtime_capabilities rc
        WHERE rc.tenant_id = de.tenant_id
          AND rc.runtime_node_id = rn.id
          AND rc.capability_type = 'provider'
          AND rc.provider_type = de.provider_type
          AND rc.available = true
          AND rc.status = 'healthy'
          AND rc.health_status = 'healthy'
          AND rc.disabled_at IS NULL
          AND rc.archived_at IS NULL
    ) AS provider_healthy
FROM digital_employees de
JOIN active_placement ON TRUE
JOIN runtime_nodes rn
  ON rn.id = active_placement.runtime_node_id
 AND rn.tenant_id = de.tenant_id
 AND rn.status = 'online'
 AND rn.disabled_at IS NULL
 AND rn.archived_at IS NULL
LEFT JOIN approved_config ON TRUE
LEFT JOIN latest_active_session ON latest_active_session.runtime_node_id = rn.id
LEFT JOIN LATERAL (
    SELECT
        LEAST(COALESCE(SUM(
            CASE
                WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                ELSE 0
            END
        ), 0), 2147483647)::integer AS usage_tokens_today
    FROM task_runs tr
    WHERE tr.tenant_id = de.tenant_id
      AND tr.digital_employee_id = de.id
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
) today_usage ON TRUE
WHERE de.id = sqlc.arg('digital_employee_id')::uuid
  AND de.tenant_id = sqlc.arg('tenant_id')::uuid
  AND de.deleted_at IS NULL
  AND de.archived_at IS NULL;
```

This is the first implementation: one active placement per Project. Do not implement multi-candidate placement here.

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: generated `GetProjectTaskRunPreflight` query and params exist.

- [ ] **Step 5: Add ProjectTask run preflight types**

In `apps/control-plane/internal/employee/run_types.go`, add `ProjectID` and `ProviderType` where needed:

```go
type StartProjectTaskRunPreflight struct {
	TenantID                   uuid.UUID
	TeamID                     *uuid.UUID
	DigitalEmployeeID          uuid.UUID
	DigitalEmployeeStatus      DigitalEmployeeStatus
	RuntimeNodeID              uuid.UUID
	NodeID                     string
	ProviderType               string
	BudgetPolicy               map[string]any
	TodayTokenUsage            int32
	BusinessTimezone           string
	HasApprovedEffectiveConfig bool
	RuntimeSessionActive       bool
	ProviderHealthy            bool
}
```

Define new employee-package-local types `StartProjectTaskRunRequest` and `StartProjectTaskRunResult` in `run_types.go` (these are distinct Go types from `projectcoordination.StartProjectTaskRunRequest`/`Result` — see the type-location note in this task's Files section). `StartProjectTaskRunRequest` needs at least `TenantID`, `ProjectID`, `DigitalEmployeeID` (mirroring the fields `app.go`'s adapter will have available from `projectcoordination.StartProjectTaskRunRequest`). `StartProjectTaskRunResult` needs `RunID`, `RuntimeTaskID`, `RuntimeNodeID`, `NodeID`, and `ProviderType string`.

- [ ] **Step 6: Add repository method**

In `apps/control-plane/internal/employee/run_repository.go`, add:

```go
GetProjectTaskRunPreflight(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) (StartProjectTaskRunPreflight, error)
```

In `apps/control-plane/internal/employee/pg_run_repository.go`, implement it by mapping the generated query:

```go
func (r *PgRunRepository) GetProjectTaskRunPreflight(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) (StartProjectTaskRunPreflight, error) {
	row, err := r.q.GetProjectTaskRunPreflight(ctx, queries.GetProjectTaskRunPreflightParams{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return StartProjectTaskRunPreflight{}, mapNoRows(err)
	}
	budgetPolicy, err := mapFromJSONB(row.BudgetPolicy, "budget_policy")
	if err != nil {
		return StartProjectTaskRunPreflight{}, err
	}
	return StartProjectTaskRunPreflight{
		TenantID:                   row.TenantID,
		TeamID:                     uuidPtrFromNull(row.TeamID),
		DigitalEmployeeID:          row.DigitalEmployeeID,
		DigitalEmployeeStatus:      DigitalEmployeeStatus(row.DigitalEmployeeStatus),
		RuntimeNodeID:              row.RuntimeNodeID,
		NodeID:                     row.NodeID,
		ProviderType:               row.ProviderType,
		BudgetPolicy:               budgetPolicy,
		TodayTokenUsage:            row.TodayTokenUsage,
		BusinessTimezone:           row.BusinessTimezone,
		HasApprovedEffectiveConfig: row.HasApprovedEffectiveConfig,
		RuntimeSessionActive:       row.RuntimeSessionActive,
		ProviderHealthy:            row.ProviderHealthy,
	}, nil
}
```

Use the existing null UUID helper name in this file if it differs.

- [ ] **Step 7: Add ProjectTask run start method**

In `apps/control-plane/internal/employee/run_service.go`, add:

```go
func (s *DigitalEmployeeRunService) StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error) {
	preflight, err := s.repository.GetProjectTaskRunPreflight(ctx, req.TenantID, req.ProjectID, req.DigitalEmployeeID)
	if err != nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("get project task run preflight: %w", err)
	}
	if err := validateProjectTaskRunPreflight(preflight); err != nil {
		return StartProjectTaskRunResult{}, err
	}
	if s.dispatcher == nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: runtime command dispatcher is required", ErrRuntimeUnavailable)
	}
	if !s.dispatcher.IsConnected(preflight.NodeID) {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	legacyPreflight := RunPreflight{
		TenantID:                   preflight.TenantID,
		TeamID:                     uuidValue(preflight.TeamID),
		DigitalEmployeeID:          preflight.DigitalEmployeeID,
		DigitalEmployeeStatus:      preflight.DigitalEmployeeStatus,
		RuntimeNodeID:              preflight.RuntimeNodeID,
		NodeID:                     preflight.NodeID,
		ProviderType:               preflight.ProviderType,
		BudgetPolicy:               preflight.BudgetPolicy,
		TodayTokenUsage:            preflight.TodayTokenUsage,
		BusinessTimezone:           preflight.BusinessTimezone,
		HasApprovedEffectiveConfig: preflight.HasApprovedEffectiveConfig,
		ProviderHealthy:            preflight.ProviderHealthy,
		AgentHomeDir:               "",
	}
	return s.startProjectTaskRunWithPreflight(ctx, req, legacyPreflight)
}
```

`uuidValue` does not exist in the `employee` package today (it exists only as unrelated, unexported local helpers in `projectcoordination/project_store.go:2676` and `project/service.go:3788`) — add a small local `func uuidValue(v *uuid.UUID) uuid.UUID` helper (return `uuid.Nil` on `nil`) alongside this code, or drop `TeamID *uuid.UUID` from `StartProjectTaskRunPreflight` and dereference inline with a nil check.

Implement `startProjectTaskRunWithPreflight` by extracting the record creation and `dispatchStartSession` logic already used by `DigitalEmployeeRunService.CreateRun` (`run_service.go:101`; there is no method literally named `CreateDigitalEmployeeRun` — that name belongs to the HTTP handler in `run_handler.go:28`). It must not call `validateRunPreflight`; it uses `validateProjectTaskRunPreflight`.

- [ ] **Step 7b (blocking, do not skip): Resolve `AgentHomeDir`/`ExecutionInstanceID` for ProjectTask dispatch before wiring `dispatchStartSession`**

`legacyPreflight` above sets `AgentHomeDir: ""` and leaves `ExecutionInstanceID`, `RuntimeSelector`, `SessionPolicy`, and `WorkspacePolicy` at their zero values, because a digital employee created under Task 2 no longer has a bound execution instance to source them from. `dispatchStartSession` → `buildStartSessionPayload` (`run_service.go:213`, `:835-862`) sends `preflight.AgentHomeDir` and `preflight.ExecutionInstanceID` unchanged into the Runtime `start_session` command payload, and Runtime Agent's `ensure_command_instance` (`apps/runtime-agent/src/commands/executor.rs:938-960`) hard-fails the command — `"agent_home_dir is required"` if empty, `"agent_home_dir does not exist"` if the path isn't already a real directory on that Runtime node's disk — before any Provider process starts. Reusing `dispatchStartSession` unchanged for ProjectTask dispatch, as originally drafted, will fail every real dispatch at this check.

This is an open design question, not a coding gap: see project memory `project-code-workspace-runtime-affinity` (spec `docs/superpowers/specs/2026-06-29-project-code-workspace-runtime-affinity-design.md`, status 待评审/未落地) for the intended per-employee home-directory model. Do not silently invent a resolution here. Before continuing Task 4:

1. Confirm with a human whether ProjectTask dispatch is in scope to also provision/derive `AgentHomeDir` (and, if Runtime-side workspace materialization needs it, `WorkspacePolicy`/`SessionPolicy`/`RuntimeSelector`) independently of execution-instance binding, or whether this task's scope should be cut back so digital employees still require *some* one-time Runtime-side home-directory provisioning step (even though they no longer require a full execution-instance bind at creation time) before they are eligible for ProjectTask dispatch.
2. Once that's decided, update `GetProjectTaskRunPreflight`/`StartProjectTaskRunPreflight` to actually select a real `AgentHomeDir` value, and add a test asserting `dispatchStartSession`'s outgoing payload contains a non-empty `agent_home_dir` for a ProjectTask-dispatched run.
3. Do not mark Task 4 or Task 7's real-chain smoke as complete until a real Runtime Agent accepts a ProjectTask-dispatched `start_session` command without an `agent_home_dir` error.

- [ ] **Step 8: Add ProjectTask preflight validation**

Add:

```go
func validateProjectTaskRunPreflight(preflight StartProjectTaskRunPreflight) error {
	if preflight.TenantID == uuid.Nil {
		return fmt.Errorf("%w: preflight tenant_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: preflight digital_employee_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeStatus != DigitalEmployeeStatusReady && preflight.DigitalEmployeeStatus != DigitalEmployeeStatusActive {
		return fmt.Errorf("%w: digital employee must be ready or active", ErrInvalidInput)
	}
	if !preflight.HasApprovedEffectiveConfig {
		return fmt.Errorf("%w: approved effective config is required", ErrEffectiveConfigRequired)
	}
	if preflight.RuntimeNodeID == uuid.Nil || strings.TrimSpace(preflight.NodeID) == "" {
		return fmt.Errorf("%w: project runtime placement is required", ErrRuntimeUnavailable)
	}
	if strings.TrimSpace(preflight.ProviderType) == "" {
		return fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	if !preflight.RuntimeSessionActive {
		return fmt.Errorf("%w: runtime session is not active", ErrRuntimeUnavailable)
	}
	if !preflight.ProviderHealthy {
		return fmt.Errorf("%w: provider capability must be healthy", ErrProviderUnavailable)
	}
	return nil
}
```

- [ ] **Step 9: Wire adapter result Provider**

In `apps/control-plane/internal/app/app.go`, update `projectTaskRunStarterAdapter.StartProjectTaskRun` so it calls the new run-service method and returns `ProviderType`.

First, in `apps/control-plane/internal/workflow/projectcoordination/types.go`, add `ProviderType string` to `StartProjectTaskRunResult` (`types.go:335-340`) — this is the type the adapter returns to `project_store.go`, distinct from the new `employee.StartProjectTaskRunResult` added in Step 5.

Then in `app.go`, replace the adapter body (currently `app.go:162-188`, which calls `a.runService.CreateRun(...)`) so it instead calls the new `a.runService.StartProjectTaskRun(ctx, employee.StartProjectTaskRunRequest{TenantID: req.TenantID, ProjectID: req.ProjectID, DigitalEmployeeID: req.DigitalEmployeeID, ...})` and maps the `employee.StartProjectTaskRunResult` it gets back onto `projectcoordination.StartProjectTaskRunResult`. The result must include:

```go
ProviderType: run.ProviderType,
```

- [ ] **Step 10: Include Provider in dispatch metadata and packet**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, after run start succeeds, add:

```go
runMetadata["provider_type"] = run.ProviderType
```

Add to `executionContextPacket`:

```go
"provider_type": run.ProviderType,
```

Pass `DigitalEmployeeID` and `ProviderType` into `QueueProjectTaskRequest` in Task 5 after the request type is updated.

- [ ] **Step 11: Run targeted dispatch/run tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestStartProjectTaskRun|TestValidateProjectTaskRunPreflight' -count=1
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTaskUsesProjectPlacementAndEmployeeProvider|TestProjectStoreDispatchProjectTask' -count=1
```

Expected: PASS.

- [ ] **Step 12: Commit Task 4**

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/employee apps/control-plane/internal/app/app.go apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: dispatch project tasks through project runtime placement"
```

## Task 5: ProjectTask Attempt Audit Facts

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/project/pg_repository_test.go`
- Test: `apps/control-plane/internal/project/handler_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add failing project repository test**

In `apps/control-plane/internal/project/pg_repository_test.go`, add a test around `QueueProjectTaskWithAttempt`:

```go
func TestQueueProjectTaskWithAttemptPersistsEmployeeAndProviderAuditFacts(t *testing.T) {
	ctx := context.Background()
	repo, fixture := seedQueueProjectTaskWithAttemptFixture(t)
	employeeID := fixture.EmployeeID
	runtimeNodeID := fixture.RuntimeNodeID

	result, err := repo.QueueProjectTaskWithAttempt(ctx, QueueProjectTaskRequest{
		TenantID:               fixture.TenantID,
		ProjectID:              fixture.ProjectID,
		ProjectTaskID:          fixture.ProjectTaskID,
		ProjectTaskAttemptID:   uuidPtr(uuid.New()),
		DigitalEmployeeID:      employeeID,
		DigitalEmployeeRunID:   uuidPtr(uuid.New()),
		RuntimeTaskID:          uuidPtr(uuid.New()),
		RuntimeNodeID:          &runtimeNodeID,
		ProviderType:           "codex",
		IdempotencyKey:         "dispatch:" + fixture.ProjectTaskID.String(),
		LeaseToken:             "lease-token",
		ExecutionContextPacket: map[string]any{"project_id": fixture.ProjectID.String()},
	})

	require.NoError(t, err)
	require.Equal(t, employeeID, *result.Attempt.DigitalEmployeeID)
	require.Equal(t, "codex", *result.Attempt.ProviderType)
}
```

If no `seedQueueProjectTaskWithAttemptFixture` helper exists, extract it from the setup block used by `TestQueueProjectTaskWithAttemptMovesPlannedTaskToQueued` in the same file rather than creating unrelated fixtures.

- [ ] **Step 2: Run the test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestQueueProjectTaskWithAttemptPersistsEmployeeAndProviderAuditFacts -count=1
```

Expected: FAIL because `QueueProjectTaskRequest` or `ProjectTaskAttempt` does not expose the new fields yet.

- [ ] **Step 3: Add domain fields**

In `apps/control-plane/internal/project/types.go`, add to `ProjectTaskAttempt`:

```go
DigitalEmployeeID *uuid.UUID
ProviderType      *string
```

In `QueueProjectTaskRequest`, add:

```go
ProviderType string
```

It already has `DigitalEmployeeID uuid.UUID`; use that to populate the attempt field.

- [ ] **Step 4: Add repository mapping**

In `apps/control-plane/internal/project/pg_repository.go`, update `createProjectTaskAttemptWithQueries`:

```go
DigitalEmployeeID: nullUUIDFromValue(req.DigitalEmployeeID),
ProviderType:      textFromOptionalString(req.ProviderType),
```

Neither `nullUUIDFromValue` nor `textFromOptionalString` exists in `pg_repository.go` today (confirmed: no matches for either name in the `project` package) — these are illustrative names, not real helpers. Use:

```go
DigitalEmployeeID: uuid.NullUUID{UUID: req.DigitalEmployeeID, Valid: req.DigitalEmployeeID != uuid.Nil},
ProviderType:      pgtype.Text{String: req.ProviderType, Valid: req.ProviderType != ""},
```

or whatever small helper the existing null/text conversions in this file already use — `ptrUUID(value uuid.NullUUID) *uuid.UUID` and `ptrText(value pgtype.Text) *string` (`pg_repository.go:6177,6248`) are the existing helpers for the reverse direction (row → domain), used below; match that style for domain → params too. The test helper `uuidPtr(uuid.New())` used in Step 1's test snippet also does not exist yet in the `project` test files — add a small local `func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }` if the test file doesn't already have an equivalent.

In `projectTaskAttemptFromRecord`, add:

```go
DigitalEmployeeID: ptrUUID(row.DigitalEmployeeID),
ProviderType:      ptrText(row.ProviderType),
```

- [ ] **Step 5: Update memory repositories and handlers**

Update all in-memory test repositories that construct `ProjectTaskAttempt` so they set `DigitalEmployeeID` and `ProviderType` from `QueueProjectTaskRequest` where applicable.

When request/response JSON already derives attempt data for execution trace, include these fields only if the existing response structs expose attempt details. Do not create a new public API response shape unless the current handler already returns attempt fields.

- [ ] **Step 6: Pass Provider from ProjectStore**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, update `QueueProjectTaskRequest`:

```go
ProviderType: run.ProviderType,
```

Ensure `executionContextPacket` includes `provider_type`.

- [ ] **Step 7: Run targeted tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestQueueProjectTaskWithAttempt|TestProjectTaskAttempt|TestExecutionTrace' -count=1
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 5**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: persist project task attempt execution identity"
```

## Task 6: Execution Instance Compatibility And Provider Immutability

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Test: `apps/control-plane/internal/employee/service_test.go`
- Test: `apps/control-plane/internal/employee/pg_repository_test.go`

- [ ] **Step 1: Add failing Provider immutability test**

In `apps/control-plane/internal/employee/service_test.go`, add:

```go
func TestBindExecutionInstanceCannotChangeEmployeeProvider(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		TeamID:       &teamID,
		Status:       DigitalEmployeeStatusReady,
		ProviderType: "codex",
	}
	service := NewService(repo, fakeRuntimeDispatcher{connected: true})

	_, err := service.BindExecutionInstance(ctx, BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     runtimeNodeID,
		ProviderType:      "opencode",
		AgentHomeDir:      "/tmp/opencode",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Equal(t, "codex", repo.employees[employeeID].ProviderType)
}
```

Adapt fake dispatcher and constants to existing tests.

- [ ] **Step 2: Run the immutability test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestBindExecutionInstanceCannotChangeEmployeeProvider -count=1
```

Expected: FAIL because bind currently accepts changing `provider_type`.

- [ ] **Step 3: Guard bind/update paths**

In `BindExecutionInstance`, load the employee record before preflight and add:

```go
if strings.TrimSpace(record.ProviderType) != "" && strings.TrimSpace(record.ProviderType) != providerType {
	return DigitalEmployeeExecutionInstance{}, fmt.Errorf("%w: provider_type is fixed on digital employee", ErrInvalidInput)
}
```

Keep allowing bind with the same Provider for compatibility and skill-install workflows.

- [ ] **Step 4: Prevent SQL upsert from changing employee Provider**

Do not write `digital_employees.provider_type` in `UpsertDigitalEmployeeExecutionInstance`. It should continue to write only the compatibility execution-instance row. If a query or repository helper backfills employee Provider during upsert, remove that behavior.

- [ ] **Step 5: Keep legacy run preflight explicitly legacy**

Add a short code comment above `GetDigitalEmployeeRunPreflight` mapping or `validateRunPreflight`:

```go
// Legacy workbench runs still require a concrete execution instance. ProjectTask dispatch uses project placement preflight instead.
```

Do not weaken `validateRunPreflight` in this task; ordinary workbench run remains bound to execution instance until a later product decision.

- [ ] **Step 6: Run targeted tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestBindExecutionInstance|TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding|TestDigitalEmployeeRun' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 6**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/storage/queries
git commit -m "fix: keep employee provider immutable across runtime bindings"
```

## Task 7: Runtime Payload Contract And Real-Chain Verification

**Files:**
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add Runtime command payload regression test**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, add or update a ProjectTask dispatch test to assert the command context includes the audit facts. There is no `run_context` identifier in this codebase. The real struct is `RuntimeCommandRunContext` (`apps/runtime-agent/src/runs.rs:28-37`), reached in tests via `snapshot.command_context` (e.g. `let command_context = snapshot.command_context.expect(...)`):

```rust
pub struct RuntimeCommandRunContext {
    pub command_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub provider_type: String,
    pub session_policy: serde_json::Value,
    pub context_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    pub metadata: serde_json::Value,
}
```

`digital_employee_id` and `provider_type` are plain top-level `String` fields — assert them directly, not via `.metadata`. There is no `runtime_node_id` field on this struct; `runtime_node_id` is read out of `.metadata` by `executor.rs:1611-1612` (confirming it is already present in the metadata JSON alongside `project_id`/`project_task_id`/`project_task_attempt_id`/`project_task_lease_token`, per the existing `project_task_attempt_metadata()` fixture), so keep asserting it via `.metadata`:

```rust
assert_eq!(command_context.metadata.get("project_id").and_then(|v| v.as_str()), Some(project_id.as_str()));
assert_eq!(command_context.metadata.get("project_task_attempt_id").and_then(|v| v.as_str()), Some(attempt_id.as_str()));
assert_eq!(command_context.metadata.get("runtime_node_id").and_then(|v| v.as_str()), Some(runtime_node_id.as_str()));
assert_eq!(command_context.digital_employee_id, employee_id);
assert_eq!(command_context.provider_type, "codex");
```

Use the existing variable names in the closest ProjectTask dispatch test.

- [ ] **Step 2: Run Runtime targeted test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 3: Run full targeted verification stack**

Run:

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -count=1
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture
corepack pnpm verify:contracts
git diff --check
```

Expected: PASS.

- [ ] **Step 4: Add CHANGELOG entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add a `CHANGELOG.md` entry using the command output timestamp:

```markdown
- YYYY-MM-DD HH:MM: 将数字员工 Provider 提升为身份事实，创建数字员工不再要求 Runtime 绑定，并让 ProjectTask dispatch 基于项目 Runtime placement 记录员工、Provider 与 Runtime 执行事实。
```

- [ ] **Step 5: Real-chain smoke on current code**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh restart runtime-agent
```

Then perform one real ProjectTask path through the running stack:

1. Log in to the Web console with the local dev credentials used in this repo.
2. Create a digital employee with Provider `codex` and no Runtime selection.
3. Confirm the API response contains `provider_type:"codex"` and no employee-level Runtime binding is required for creation.
4. Add the employee to a Project digital-employee pool.
5. Ensure the Project has active `project_placements` pointing to an online Runtime that reports Provider `codex`.
6. Submit a ProjectDemand from the task launch page.
7. Confirm the resulting ProjectTask attempt/run can be audited with Project ID, ProjectTask ID, DigitalEmployee ID, Provider `codex`, and RuntimeNode ID.

If local Provider credentials or a safe real execution workspace are unavailable, stop and report:

```text
阻塞：Runtime/Provider 真实执行链路缺少 <exact dependency>；尚不能声明 ProjectTask execution path 完成。
```

Do not replace this smoke with unit tests when claiming the cross-layer feature is usable.

- [ ] **Step 6: Commit Task 7**

```bash
git add apps/runtime-agent/tests/runtime_command_executor_test.rs CHANGELOG.md
git commit -m "test: verify project task provider runtime payload"
```

## Final Verification Matrix

Run these before declaring implementation complete:

```bash
make -C apps/control-plane migrate-validate
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -count=1
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task --test runtime_command_executor_test -- --nocapture
corepack pnpm verify:contracts
git diff --check
```

Real-chain verification is required after the code changes because this plan crosses Web, Control Plane, database, Runtime dispatch, and Provider execution. The minimum real-chain evidence is one live ProjectDemand -> ProjectTask dispatch -> Runtime command -> attempt audit path on current code.

## Self-Review Notes

- Spec coverage: creation without Runtime is Task 2 and Task 3; fixed employee Provider is Task 1, Task 2, and Task 6; ProjectTask dispatch Runtime selection is Task 4; attempt audit facts are Task 5; Runtime payload verification and real-chain smoke are Task 7.
- Scope boundary: ordinary employee runs remain legacy workbench/debug behavior and still require execution instance; business ProjectTask execution moves to Project placement dispatch.
- Migration boundary: Provider names remain service-validated strings, with no database enum/check constraint in this plan.
- Risk: `sqlc` and `atlas` must be available. If either is missing, pause and install/expose the project-approved tool rather than hand-editing generated files.
- Risk (open, blocking): Task 4's new ProjectTask preflight has no source for `AgentHomeDir`, and Runtime Agent hard-fails `start_session` without a real, pre-existing home directory on disk (`executor.rs:938-960`). See "Review Corrections" at the top of this document and Task 4 Step 7b — this must be resolved with a human decision before Task 4's real-chain smoke can pass, and is the most likely reason this plan stalls at Task 7 if skipped.
