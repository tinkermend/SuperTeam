# 数字员工配置破坏性重构 Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the old digital-employee personal governance config model with the final model: `persona_memory_markdown`, `capability_bindings`, and `budget_policy`, while destructively clearing incompatible development data.

**Architecture:** This is a breaking, cross-layer refactor. The database migration first clears incompatible digital-employee rows and reshapes `digital_employee_config_revisions`; then sqlc/OpenAPI/domain types are regenerated around the final fields; then Control Plane, Web, and Runtime payloads are updated to stop reading or writing the removed fields.

**Tech Stack:** Go Control Plane, sqlc/pgx, Atlas migrations, OpenAPI/oapi-codegen, React + TanStack Router/Query + Vitest, Rust Runtime Agent, Cargo tests.

## Global Constraints

- This is a breaking development-environment refactor: do not preserve old digital employees if they conflict with the new model.
- Do not migrate old `role_profile` or `constitution_addendum` content into `persona_memory_markdown`.
- Remove these old config fields from product, API, DB, and runtime payloads: `role_profile`, `constitution_addendum`, `capability_selection`, `context_policy_override`, `approval_policy_override`, `output_contract_addendum`.
- Keep `provider_type` as the existing create-flow fact; do not reframe this task as a Provider selection redesign.
- Runtime matching is project-owned; do not add Runtime matching config to digital employees.
- Do not add memory-library strategy settings.
- Web tests must run with `corepack pnpm --filter ./apps/web run test`; do not use raw `npx vitest` or `npx playwright install`.
- Contract changes require `corepack pnpm generate:control-plane` and `corepack pnpm verify:contracts`.
- Migration changes live in `apps/control-plane/internal/storage/migrations/`; run `atlas migrate hash --dir file://internal/storage/migrations` from `apps/control-plane` and prefer `make -C apps/control-plane migrate-validate`.
- Before claiming completion, run `$superteam-completion-check` and perform the real-chain smoke in Task 9.

---

## File Structure

- `apps/control-plane/internal/storage/migrations/051_digital_employee_config_final_model.sql`: destructive dev-data cleanup plus schema reshape for `digital_employee_config_revisions`.
- `apps/control-plane/internal/storage/migrations/atlas.sum`: Atlas checksum update.
- `apps/control-plane/internal/storage/migrations_test.go`: migration string tests for the destructive cleanup and final columns.
- `apps/control-plane/internal/storage/queries/digital_employee_config.sql`: sqlc query source for final config revision columns.
- `apps/control-plane/internal/storage/queries/employee_execution.sql`: overview/run/readiness queries that currently read old config fields.
- `apps/control-plane/internal/storage/queries/*.sql.go`, `models.go`: generated sqlc outputs.
- `contracts/control-plane/openapi.yaml`: API contract for create employee, config revision, create options, and template shapes.
- `apps/control-plane/gen/control_plane.gen.go`: generated OpenAPI output.
- `apps/control-plane/internal/employee/types.go`: final domain request/response/config types.
- `apps/control-plane/internal/employee/repository.go`: repository params/records for final config fields.
- `apps/control-plane/internal/employee/pg_repository.go`: JSONB mapping for final fields.
- `apps/control-plane/internal/employee/service.go`: create/config revision/effective preview/runtime payload logic.
- `apps/control-plane/internal/employee/handler.go`: request/response DTOs and old-field rejection.
- `apps/control-plane/internal/employee/*_test.go`: focused service, handler, repository, and route tests.
- `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`: planning profile reads identity and final capability bindings instead of `role_profile` / `capability_selection`.
- `apps/web/src/lib/api/employees.ts`: final TypeScript API types and payloads.
- `apps/web/src/lib/api/employee-templates.ts`: template final fields.
- `apps/web/src/features/employees/create.tsx`: create wizard IA and payload.
- `apps/web/src/features/employees/config.tsx`: config revision UI replaced with persona/capability/budget fields.
- `apps/web/src/features/employees/detail.tsx`: detail display uses final fields.
- `apps/web/src/features/employees/templates.tsx`, `template-utils.ts`: template editing/summary uses persona/capability/budget.
- `apps/web/src/features/employees/*.test.tsx`, `apps/web/src/lib/api/*.test.ts`: updated tests.
- `apps/runtime-agent/src/commands/payload.rs`: provision/session payload parsing of persona and capability bindings.
- `apps/runtime-agent/src/commands/executor.rs`: materialize `人格记忆.md` as controlled projection if included.
- `apps/runtime-agent/tests/runtime_command_payload_test.rs`, `runtime_command_executor_test.rs`, `workspace_files_test.rs`: Runtime validation.
- `CHANGELOG.md`: final feature completion entry after implementation and verification, not during early tasks.

---

### Task 1: Destructive Migration To Final Config Columns

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/051_digital_employee_config_final_model.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`

**Interfaces:**
- Produces DB columns on `digital_employee_config_revisions`: `persona_memory_markdown TEXT NOT NULL DEFAULT ''`, `capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb`, existing `budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb`.
- Removes DB columns: `role_profile`, `constitution_addendum`, `capability_selection`, `context_policy_override`, `approval_policy_override`, `output_contract_addendum`.
- Clears all rows in `digital_employees` and dependent employee-owned tables in the development schema.

- [ ] **Step 1: Write the failing migration test**

Add this test to `apps/control-plane/internal/storage/migrations_test.go` near `TestDigitalEmployeeBudgetPolicyMigration`:

```go
func TestDigitalEmployeeConfigFinalModelMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/051_digital_employee_config_final_model.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, expected := range []string{
		"DELETE FROM digital_employee_environment_variables",
		"DELETE FROM skill_installations",
		"DELETE FROM digital_employee_mcp_bindings_v2",
		"DELETE FROM digital_employee_mcp_bindings",
		"DELETE FROM digital_employee_workspace_file_syncs",
		"DELETE FROM digital_employee_workspace_file_revisions",
		"DELETE FROM digital_employee_workspace_files",
		"DELETE FROM digital_employee_execution_instances",
		"DELETE FROM digital_employee_config_revisions",
		"DELETE FROM digital_employees",
		"ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb",
		"DROP COLUMN IF EXISTS role_profile",
		"DROP COLUMN IF EXISTS constitution_addendum",
		"DROP COLUMN IF EXISTS capability_selection",
		"DROP COLUMN IF EXISTS context_policy_override",
		"DROP COLUMN IF EXISTS approval_policy_override",
		"DROP COLUMN IF EXISTS output_contract_addendum",
		"COMMENT ON COLUMN digital_employee_config_revisions.persona_memory_markdown IS '数字员工人格记忆 Markdown，描述人格画像、专业边界、工作方式和表达偏好'",
		"COMMENT ON COLUMN digital_employee_config_revisions.capability_bindings IS '数字员工能力绑定，保存 Skill、MCP、外部能力和环境变量引用'",
	} {
		require.Contains(t, sql, expected)
	}

	for _, forbidden := range []string{
		"INSERT INTO digital_employee_config_revisions",
		"role_profile JSONB",
		"constitution_addendum JSONB",
		"capability_selection JSONB",
		"context_policy_override JSONB",
		"approval_policy_override JSONB",
		"output_contract_addendum JSONB",
	} {
		require.NotContains(t, sql, forbidden)
	}
}
```

- [ ] **Step 2: Run the new test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeConfigFinalModelMigration -count=1
```

Expected: FAIL because `migrations/051_digital_employee_config_final_model.sql` does not exist.

- [ ] **Step 3: Add the migration**

Create `apps/control-plane/internal/storage/migrations/051_digital_employee_config_final_model.sql`:

```sql
-- Development-only destructive reset for the final digital-employee config model.
-- Current development data is intentionally discarded instead of migrated from the
-- old personal-governance config shape.

DELETE FROM digital_employee_environment_variables;
DELETE FROM skill_installations;
DELETE FROM digital_employee_mcp_bindings_v2;
DELETE FROM digital_employee_mcp_bindings;
DELETE FROM skill_agent_bindings;
DELETE FROM project_employee_node_affinity;

UPDATE project_task_attempts
SET digital_employee_id = NULL
WHERE digital_employee_id IS NOT NULL;

UPDATE project_tasks
SET assigned_digital_employee_id = NULL,
    digital_employee_run_id = NULL
WHERE assigned_digital_employee_id IS NOT NULL
   OR digital_employee_run_id IS NOT NULL;

DELETE FROM task_runs
WHERE digital_employee_id IS NOT NULL;

DELETE FROM digital_employee_workspace_file_syncs;
DELETE FROM digital_employee_workspace_file_revisions
WHERE file_id IN (SELECT id FROM digital_employee_workspace_files);
DELETE FROM digital_employee_workspace_files;
DELETE FROM runtime_command_receipts
WHERE resource_type = 'digital_employee_execution_instance';
DELETE FROM provider_session_events
WHERE digital_employee_id IS NOT NULL;
DELETE FROM provider_sessions
WHERE digital_employee_id IS NOT NULL;
DELETE FROM digital_employee_execution_instances;
DELETE FROM digital_employee_config_revisions;
DELETE FROM digital_employees;

ALTER TABLE digital_employee_config_revisions
    ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE digital_employee_config_revisions
    DROP COLUMN IF EXISTS role_profile,
    DROP COLUMN IF EXISTS constitution_addendum,
    DROP COLUMN IF EXISTS capability_selection,
    DROP COLUMN IF EXISTS context_policy_override,
    DROP COLUMN IF EXISTS approval_policy_override,
    DROP COLUMN IF EXISTS output_contract_addendum;

COMMENT ON TABLE digital_employee_config_revisions IS '数字员工个人配置版本表，保存人格记忆、能力绑定和预算策略';
COMMENT ON COLUMN digital_employee_config_revisions.persona_memory_markdown IS '数字员工人格记忆 Markdown，描述人格画像、专业边界、工作方式和表达偏好';
COMMENT ON COLUMN digital_employee_config_revisions.capability_bindings IS '数字员工能力绑定，保存 Skill、MCP、外部能力和环境变量引用';
COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限';
```

Before keeping any destructive `DELETE`, verify the table exists in the post-migration dev schema, not only in historical migration files. A table can be created in an early migration and dropped later.

Prefer a live schema check against the intended development database:

```bash
DEV_DATABASE_URL="$(
  awk '
    /^[A-Za-z]/ { section=$1 }
    section=="postgres:" && $1=="url:" {
      line=$0
      sub(/^[^"]*"/, "", line)
      sub(/".*$/, "", line)
      print line
      exit
    }
  ' apps/control-plane/config/config.yaml
)"
test -n "$DEV_DATABASE_URL"
psql "$DEV_DATABASE_URL" -Atc "
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name IN (
    'digital_employee_environment_variables',
    'skill_installations',
    'digital_employee_mcp_bindings_v2',
    'digital_employee_mcp_bindings',
    'skill_agent_bindings',
    'project_employee_node_affinity',
    'project_task_attempts',
    'project_tasks',
    'task_runs',
    'digital_employee_workspace_file_syncs',
    'digital_employee_workspace_file_revisions',
    'digital_employee_workspace_files',
    'runtime_command_receipts',
    'provider_session_events',
    'provider_sessions',
    'digital_employee_execution_instances',
    'digital_employee_config_revisions',
    'digital_employees'
  )
ORDER BY table_name;"
```

If a live database is unavailable during Task 1 implementation, validate net existence by reading both creates and drops:

```bash
rg -n "CREATE TABLE.*digital_employee_|DROP TABLE IF EXISTS digital_employee_|CREATE TABLE.*(skill_installations|skill_agent_bindings|project_employee_node_affinity|project_task_attempts|project_tasks|task_runs|runtime_command_receipts|provider_session_events|provider_sessions)|DROP TABLE IF EXISTS (skill_installations|skill_agent_bindings|project_employee_node_affinity|project_task_attempts|project_tasks|task_runs|runtime_command_receipts|provider_session_events|provider_sessions)" apps/control-plane/internal/storage/migrations
```

Then remove a `DELETE` when the table is genuinely absent after the latest migration. Do not add `DELETE FROM digital_employee_effective_configs`; that table was removed by migration `047_drop_team_governance_and_effective_config.sql`.

- [ ] **Step 4: Run the migration test**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeConfigFinalModelMigration -count=1
```

Expected: PASS.

- [ ] **Step 5: Update Atlas checksum and validate replay**

Run:

```bash
cd apps/control-plane
atlas migrate hash --dir file://internal/storage/migrations
make migrate-validate
```

Expected: both commands exit 0. If `make migrate-validate` is blocked because Docker or Atlas is unavailable, record the exact blocker and still run `atlas migrate hash`.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/051_digital_employee_config_final_model.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat(control-plane): reshape digital employee config schema"
```

---

### Task 2: sqlc Queries And Repository Shape

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Generated: `apps/control-plane/internal/storage/queries/digital_employee_config.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/employee_execution.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/models.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Test: `apps/control-plane/internal/employee/pg_repository_test.go`

**Interfaces:**
- Produces `EmployeeConfigInput`/record mappings with only `PersonaMemoryMarkdown string`, `CapabilityBindings map[string]any`, and `BudgetPolicy map[string]any`.
- Produces sqlc rows with JSONB `capability_bindings` and text `persona_memory_markdown`.

**Execution Boundary:**
- Task 2 and Task 3 are one Go package compile unit. The `apps/control-plane/internal/employee` package cannot stay green after Task 2 alone because generated sqlc/repository types are consumed by service and handler files in the same package.
- Execute Task 2 immediately followed by Task 3 before requiring the package-level Go tests to pass or creating the final Control Plane commit. Task 2 still owns query/repository mapping and TDD red evidence; Task 3 owns service/handler/domain alignment and the green package verification.
- Do not leave the branch at a committed state where Task 2 has changed generated repository types but Task 3 consumers still reference removed old fields.

- [ ] **Step 1: Write repository mapping tests**

Add to `apps/control-plane/internal/employee/pg_repository_test.go`:

```go
func TestDigitalEmployeeConfigRevisionQueryMappingUsesFinalFields(t *testing.T) {
	row := queries.DigitalEmployeeConfigRevision{
		ID:                    uuid.New(),
		TenantID:              uuid.New(),
		DigitalEmployeeID:     uuid.New(),
		RevisionNumber:        3,
		PersonaMemoryMarkdown: "# 人格画像\n证据优先",
		CapabilityBindings:    []byte(`{"skills":["incident-diagnosis"],"mcp_servers":["postgres-readonly"],"environment_variable_refs":["PG_DSN"]}`),
		BudgetPolicy:          []byte(`{"daily_token_limit":50000}`),
		Status:                string(ConfigRevisionStatusActive),
		CreatedAt:             pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	input, err := employeeConfigInputFromQueryRow(row)
	require.NoError(t, err)
	require.Equal(t, "# 人格画像\n证据优先", input.PersonaMemoryMarkdown)
	require.Equal(t, []any{"incident-diagnosis"}, input.CapabilityBindings["skills"])
	require.Equal(t, float64(50000), input.BudgetPolicy["daily_token_limit"])

	record, err := configRevisionRecordFromQuery(row)
	require.NoError(t, err)
	require.Equal(t, input.PersonaMemoryMarkdown, record.PersonaMemoryMarkdown)
	require.Equal(t, input.CapabilityBindings, record.CapabilityBindings)
	require.Equal(t, input.BudgetPolicy, record.BudgetPolicy)
}
```

If `queries.DigitalEmployeeConfigRevision` has a different generated struct name after Step 3, update only the type name, not the asserted field names.

- [ ] **Step 2: Run the test and confirm it fails**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestDigitalEmployeeConfigRevisionQueryMappingUsesFinalFields -count=1
```

Expected: FAIL because final fields do not exist.

- [ ] **Step 3: Replace digital employee config SQL**

Rewrite `apps/control-plane/internal/storage/queries/digital_employee_config.sql` so every select/returning list uses:

```sql
id,
tenant_id,
digital_employee_id,
revision_number,
persona_memory_markdown,
capability_bindings,
budget_policy,
status,
approved_by,
approved_at,
archived_at,
created_at,
updated_at
```

The create query must use this insert shape:

```sql
-- name: CreateDigitalEmployeeConfigRevision :one
INSERT INTO digital_employee_config_revisions (
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('persona_memory_markdown')::text, ''),
    COALESCE(sqlc.arg('capability_bindings')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('budget_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING id,
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at,
    archived_at,
    created_at,
    updated_at;
```

Update `apps/control-plane/internal/storage/queries/employee_execution.sql`:

- Replace `decr.capability_selection` with `decr.capability_bindings`.
- Replace JSON path `{enabled_mcp_servers}` with `{mcp_servers}`.
- Replace any selected config state old field with final `persona_memory_markdown`, `capability_bindings`, and `budget_policy` only.

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: generated query files no longer contain `RoleProfile`, `ConstitutionAddendum`, `CapabilitySelection`, `ContextPolicyOverride`, `ApprovalPolicyOverride`, or `OutputContractAddendum` for digital employee config revisions.

- [ ] **Step 5: Update repository domain structs**

In `apps/control-plane/internal/employee/repository.go`, replace config params and records with:

```go
type CreateConfigRevisionParams struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy           map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
}

type DigitalEmployeeConfigRevisionRecord struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy           map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
```

In `apps/control-plane/internal/employee/pg_repository.go`, update `CreateDigitalEmployeeConfigRevision` to serialize `CapabilityBindings` and `BudgetPolicy` only:

```go
capabilityBindings, err := jsonbFromMap(params.CapabilityBindings, "capability_bindings")
if err != nil {
	return DigitalEmployeeConfigRevisionRecord{}, err
}
budgetPolicy, err := jsonbFromMap(params.BudgetPolicy, "budget_policy")
if err != nil {
	return DigitalEmployeeConfigRevisionRecord{}, err
}
```

Map generated rows with:

```go
capabilityBindings, err := mapFromJSONB(revision.CapabilityBindings, "capability_bindings")
if err != nil {
	return EmployeeConfigInput{}, err
}
budgetPolicy, err := mapFromJSONB(revision.BudgetPolicy, "budget_policy")
if err != nil {
	return EmployeeConfigInput{}, err
}
```

- [ ] **Step 6: Run focused Go tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestDigitalEmployeeConfigRevisionQueryMappingUsesFinalFields|TestDigitalEmployeeConfigRevisionQueryMappingKeepsBudgetPolicy' -count=1
```

Expected: final-fields test may still be blocked by service/handler compile references until Task 3 is implemented in the same package. If compile errors reference removed fields in `service.go`, `handler.go`, `service_test.go`, `handler_test.go`, or route tests, continue directly into Task 3 and do not commit Task 2 separately. After Task 3, this command must pass or be replaced by the nearest focused final-field repository test if the generated sqlc row type name changed.

- [ ] **Step 7: Commit or continue into Task 3**

```bash
git add apps/control-plane/internal/storage/queries/digital_employee_config.sql \
  apps/control-plane/internal/storage/queries/employee_execution.sql \
  apps/control-plane/internal/storage/queries/digital_employee_config.sql.go \
  apps/control-plane/internal/storage/queries/employee_execution.sql.go \
  apps/control-plane/internal/storage/queries/models.go \
  apps/control-plane/internal/employee/repository.go \
  apps/control-plane/internal/employee/pg_repository.go \
  apps/control-plane/internal/employee/pg_repository_test.go
git commit -m "feat(control-plane): map final employee config fields"
```

Only create this commit if the `employee` package compiles at the end of Task 2. If it does not compile because Task 3 consumers still reference old config fields, keep the Task 2 changes uncommitted, implement Task 3 immediately, then create one combined Control Plane commit after the Task 3 green verification.

---

### Task 3: Control Plane Domain, Service, And Handler

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/handler_test.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`

**Interfaces:**
- Consumes Task 2 repository fields.
- Produces service requests:
  - `CreateDigitalEmployeeRequest.PersonaMemoryMarkdown string`
  - `CreateDigitalEmployeeRequest.CapabilityBindings map[string]any`
  - `CreateDigitalEmployeeConfigRevisionRequest.PersonaMemoryMarkdown *string`
  - `CreateDigitalEmployeeConfigRevisionRequest.CapabilityBindings map[string]any`
  - `BudgetPolicy map[string]any`
- Produces HTTP JSON fields: `persona_memory_markdown`, `capability_bindings`, `budget_policy`.

**Execution Boundary:**
- If Task 2 left sqlc/repository changes uncommitted because `apps/control-plane/internal/employee` could not compile independently, complete Task 3 in the same working tree and create one combined commit after the focused Go tests pass.
- The combined commit message should be `feat(control-plane): use final employee config fields` when it includes both Task 2 repository mapping and Task 3 domain/service/handler alignment.

- [ ] **Step 1: Write service tests for final create/config behavior**

Replace old config revision tests in `apps/control-plane/internal/employee/service_test.go` with tests shaped like:

```go
func TestCreateConfigRevisionStoresFinalFields(t *testing.T) {
	repo := newEmployeeServiceTestRepository()
	employee := seedConfigRevisionEmployee(t, repo)
	service := NewService(repo, nil, nil, nil, ServiceOptions{})

	persona := "# 人格画像\n证据优先"
	revision, err := service.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:                employee.TenantID,
		DigitalEmployeeID:       employee.ID,
		PersonaMemoryMarkdown:  &persona,
		CapabilityBindings:     map[string]any{"skills": []any{"incident-diagnosis"}, "mcp_servers": []any{"postgres-readonly"}},
		BudgetPolicy:            map[string]any{"daily_token_limit": float64(12000)},
	})
	require.NoError(t, err)
	require.Equal(t, persona, revision.PersonaMemoryMarkdown)
	require.Equal(t, []any{"incident-diagnosis"}, revision.CapabilityBindings["skills"])
	require.Equal(t, float64(12000), revision.BudgetPolicy["daily_token_limit"])
	require.Equal(t, persona, repo.createdConfigRevision.PersonaMemoryMarkdown)
}
```

Add a handler or API test that sends old fields and expects 400:

```go
func TestCreateDigitalEmployeeConfigRevisionRejectsOldFields(t *testing.T) {
	server := newEmployeeRouteTestServer(t)
	employeeID := server.seedEmployee(t)

	body := strings.NewReader(`{"role_profile":{"title":"legacy"},"persona_memory_markdown":"# 人格画像"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/digital-employees/"+employeeID.String()+"/config-revisions", body)
	req.Header.Set("Content-Type", "application/json")
	req = server.withAuth(req)
	rec := httptest.NewRecorder()

	server.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "role_profile is no longer supported")
}
```

Adapt helper names to the existing test fixture names in the file.

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateConfigRevisionStoresFinalFields|TestCreateDigitalEmployeeConfigRevisionRejectsOldFields' -count=1
```

Expected: FAIL because the domain and handler still use old fields.

- [ ] **Step 3: Update domain types**

In `apps/control-plane/internal/employee/types.go`, replace config fields:

```go
type EmployeeConfigInput struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy           map[string]any
}

type DigitalEmployeeConfigRevision struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	PersonaMemoryMarkdown string
	CapabilityBindings    map[string]any
	BudgetPolicy           map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID                uuid.UUID
	DigitalEmployeeID       uuid.UUID
	PersonaMemoryMarkdown  *string
	CapabilityBindings     map[string]any
	BudgetPolicy            map[string]any
	Status                  ConfigRevisionStatus
	ApprovedBy              *uuid.UUID
}
```

Add to `CreateDigitalEmployeeRequest`:

```go
PersonaMemoryMarkdown string
CapabilityBindings    map[string]any
```

Remove from `CreateDigitalEmployeeRequest` and all config request/response types:

```go
RoleProfile
ConstitutionAddendum
CapabilitySelection
ContextPolicyOverride
ApprovalPolicyOverride
OutputContractAddendum
```

- [ ] **Step 4: Update service logic**

Replace `initialEmployeeConfigParams` and `initialEmployeeConfigInput` field population with:

```go
PersonaMemoryMarkdown: strings.TrimSpace(req.PersonaMemoryMarkdown),
CapabilityBindings:    normalizeCapabilityBindings(req.CapabilityBindings),
BudgetPolicy:           cloneMap(req.BudgetPolicy),
```

Add a helper in `service.go`:

```go
func normalizeCapabilityBindings(input map[string]any) map[string]any {
	bindings := cloneMap(input)
	if bindings == nil {
		bindings = map[string]any{}
	}
	for _, key := range []string{"skills", "mcp_servers", "external_capabilities", "environment_variable_refs"} {
		if _, ok := bindings[key]; !ok {
			bindings[key] = []any{}
		}
	}
	return bindings
}
```

Replace `CreateConfigRevision` inheritance with:

```go
personaMemoryMarkdown := ""
if latestConfig != nil {
	personaMemoryMarkdown = latestConfig.PersonaMemoryMarkdown
}
if req.PersonaMemoryMarkdown != nil {
	personaMemoryMarkdown = strings.TrimSpace(*req.PersonaMemoryMarkdown)
}
capabilityBindings := inheritedConfigMap(req.CapabilityBindings, latestConfig, func(config EmployeeConfigInput) map[string]any {
	return config.CapabilityBindings
})
capabilityBindings = normalizeCapabilityBindings(capabilityBindings)
```

Update `PreviewEffectiveConfig` to return only:

```go
effectiveConfig := map[string]any{
	"employee_config_revision_id": req.EmployeeConfig.ID.String(),
	"persona_memory_markdown":     req.EmployeeConfig.PersonaMemoryMarkdown,
	"capability_bindings":         cloneMap(req.EmployeeConfig.CapabilityBindings),
	"budget_policy":               cloneMap(req.EmployeeConfig.BudgetPolicy),
}
```

Delete `initialRoleProfile`, `initialCapabilitySelection`, `initialContextPolicyOverride`, `constrainedDefaultContextPolicyOverride`, `validateCapabilitySelection`, `validateContextSubset`, and `validateApprovalOverride` if no remaining code uses them.

- [ ] **Step 5: Update handler old-field rejection**

In `CreateDigitalEmployeeConfigRevision`, decode into `map[string]json.RawMessage` first:

```go
var raw map[string]json.RawMessage
if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
	http.Error(w, err.Error(), http.StatusBadRequest)
	return
}
for _, field := range []string{
	"role_profile",
	"constitution_addendum",
	"capability_selection",
	"context_policy_override",
	"approval_policy_override",
	"output_contract_addendum",
} {
	if _, exists := raw[field]; exists {
		http.Error(w, field+" is no longer supported", http.StatusBadRequest)
		return
	}
}
```

Then unmarshal the accepted fields:

```go
var req struct {
	PersonaMemoryMarkdown *string              `json:"persona_memory_markdown"`
	CapabilityBindings    map[string]any       `json:"capability_bindings"`
	BudgetPolicy           map[string]any       `json:"budget_policy"`
	Status                 ConfigRevisionStatus `json:"status"`
}
payload, _ := json.Marshal(raw)
if err := json.Unmarshal(payload, &req); err != nil {
	http.Error(w, err.Error(), http.StatusBadRequest)
	return
}
```

Update `configRevisionResponse` and `configRevisionResponseFromDomain` to final fields:

```go
PersonaMemoryMarkdown string         `json:"persona_memory_markdown"`
CapabilityBindings    map[string]any `json:"capability_bindings"`
BudgetPolicy           map[string]any `json:"budget_policy"`
```

- [ ] **Step 6: Run focused Go tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateConfigRevisionStoresFinalFields|TestCreateDigitalEmployeeConfigRevisionRejectsOldFields|TestCreateDigitalEmployee' -count=1
go test ./apps/control-plane/internal/api -run 'TestCreateDigitalEmployee|TestCreateDigitalEmployeeConfigRevision' -count=1
```

Expected: tests pass after updating existing old-field assertions.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/employee/types.go \
  apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/handler.go \
  apps/control-plane/internal/employee/service_test.go \
  apps/control-plane/internal/employee/handler_test.go \
  apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat(control-plane): use final employee config domain"
```

---

### Task 4: OpenAPI Contract And Generated Types

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/gen/control_plane.gen.go`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employee-templates.ts`
- Test: `apps/web/src/lib/api/employees.test.ts`

**Interfaces:**
- Consumes Task 3 JSON fields.
- Produces TypeScript types:
  - `persona_memory_markdown: string`
  - `capability_bindings: CapabilityBindings`
  - `budget_policy: BudgetPolicy`
- Removes old fields from contract and generated clients.

- [ ] **Step 1: Update API tests first**

In `apps/web/src/lib/api/employees.test.ts`, change config revision fixtures to:

```ts
const revision: DigitalEmployeeConfigRevision = {
  id: "revision-1",
  tenant_id: "tenant-1",
  digital_employee_id: "employee-1",
  revision_number: 1,
  persona_memory_markdown: "# 人格画像\n证据优先",
  capability_bindings: {
    skills: ["incident-diagnosis"],
    mcp_servers: ["postgres-readonly"],
    external_capabilities: [],
    environment_variable_refs: ["PG_DSN"],
  },
  budget_policy: { daily_token_limit: 12000 },
  status: "active",
};
```

Update create input assertion:

```ts
expect(body).toMatchObject({
  persona_memory_markdown: "# 人格画像\n证据优先",
  capability_bindings: {
    skills: ["incident-diagnosis"],
    mcp_servers: ["postgres-readonly"],
    external_capabilities: [],
    environment_variable_refs: ["PG_DSN"],
  },
  budget_policy: { daily_token_limit: 12000 },
});
expect(body).not.toHaveProperty("role_profile");
expect(body).not.toHaveProperty("constitution_addendum");
expect(body).not.toHaveProperty("capability_selection");
```

- [ ] **Step 2: Run API tests and confirm failure**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts
```

Expected: FAIL because API types still expose old fields.

- [ ] **Step 3: Update OpenAPI schemas**

In `contracts/control-plane/openapi.yaml`:

- In create employee request schema, remove old fields and add:

```yaml
persona_memory_markdown:
  type: string
capability_bindings:
  type: object
  additionalProperties: true
budget_policy:
  type: object
  additionalProperties: true
```

- In config revision create and response schemas, remove:

```yaml
role_profile
constitution_addendum
capability_selection
context_policy_override
approval_policy_override
output_contract_addendum
```

- Add required response fields:

```yaml
- persona_memory_markdown
- capability_bindings
- budget_policy
```

- Keep `provider_type` wherever it currently exists; do not add new provider semantics.

- [ ] **Step 4: Regenerate OpenAPI output**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: both exit 0; `apps/control-plane/gen/control_plane.gen.go` no longer has config revision old fields.

- [ ] **Step 5: Update Web API types**

In `apps/web/src/lib/api/employees.ts`, define:

```ts
export type CapabilityBindings = {
  skills?: string[];
  mcp_servers?: string[];
  external_capabilities?: string[];
  environment_variable_refs?: string[];
  [key: string]: unknown;
};

export type DigitalEmployeeConfigRevision = {
  id: string;
  tenant_id: string;
  digital_employee_id: string;
  revision_number: number;
  persona_memory_markdown: string;
  capability_bindings: CapabilityBindings;
  budget_policy: BudgetPolicy;
  status: ConfigRevisionStatus;
  approved_by?: string;
  approved_at?: string;
  archived_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateDigitalEmployeeConfigRevisionInput = {
  persona_memory_markdown?: string;
  capability_bindings?: CapabilityBindings;
  budget_policy?: BudgetPolicy;
  status?: ConfigRevisionStatus;
};
```

Update `CreateDigitalEmployeeInput` to include:

```ts
persona_memory_markdown?: string;
capability_bindings?: CapabilityBindings;
budget_policy?: BudgetPolicy;
```

Remove old fields from TypeScript request/response types.

- [ ] **Step 6: Update template API types**

In `apps/web/src/lib/api/employee-templates.ts`, replace `default_capability_selection` and `default_context_policy_override` with:

```ts
persona_memory_markdown?: string;
capability_bindings?: CapabilityBindings;
budget_policy?: BudgetPolicy;
```

If backend template endpoints are updated in Task 5, keep this file aligned with that shape.

- [ ] **Step 7: Run contract and API tests**

Run:

```bash
corepack pnpm verify:contracts
corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add contracts/control-plane/openapi.yaml \
  apps/control-plane/gen/control_plane.gen.go \
  apps/web/src/lib/api/employees.ts \
  apps/web/src/lib/api/employee-templates.ts \
  apps/web/src/lib/api/employees.test.ts
git commit -m "feat(contracts): expose final employee config fields"
```

---

### Task 5: Templates And Create Options Use Persona/Bindings/Budget

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Modify: `apps/control-plane/internal/employee/template_types.go`
- Modify: `apps/control-plane/internal/employee/template_repository.go`
- Modify: `apps/control-plane/internal/employee/template_service.go`
- Modify: `apps/control-plane/internal/employee/template_handler.go`
- Modify: `apps/control-plane/internal/storage/queries/digital_employee_templates.sql`
- Generated: `apps/control-plane/internal/storage/queries/digital_employee_templates.sql.go`
- Modify: `apps/control-plane/internal/employee/template_service_test.go`
- Modify: `apps/web/src/features/employees/template-utils.ts`
- Modify: `apps/web/src/features/employees/templates.tsx`
- Modify: `apps/web/src/features/employees/templates.test.tsx`

**Interfaces:**
- Produces template fields `persona_memory_markdown`, `capability_bindings`, `budget_policy`.
- Removes template fields `default_capability_selection`, `default_context_policy_override`, and `default_approval_policy`.

**Execution Boundary:**
- Task 4 updates create employee and config revision contract/type surfaces. Task 5 owns the remaining template contract/backend/UI surface and must remove template OpenAPI fields `default_capability_selection`, `default_context_policy_override`, and `default_approval_policy` from create/update/response schemas.
- The generated Control Plane API file in the current repo is `apps/control-plane/internal/api/gen/control_plane.gen.go`.
- Run `corepack pnpm generate:control-plane` and `corepack pnpm verify:contracts` after changing template OpenAPI schemas.

- [ ] **Step 1: Write failing backend template test**

In `apps/control-plane/internal/employee/template_service_test.go`, add:

```go
func TestCreateEmployeeTemplateStoresFinalConfigDefaults(t *testing.T) {
	repo := newTemplateServiceTestRepository()
	service := NewTemplateService(repo)

	template, err := service.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateRequest{
		TenantID:                uuid.New(),
		Type:                    "evidence_worker",
		Label:                   "证据整理员工",
		Description:             "整理证据",
		DefaultRole:             "evidence_worker",
		PersonaMemoryMarkdown:  "# 人格画像\n证据优先",
		CapabilityBindings:     map[string]any{"skills": []any{"code-reading"}},
		BudgetPolicy:            map[string]any{"daily_token_limit": float64(10000)},
	})
	require.NoError(t, err)
	require.Equal(t, "# 人格画像\n证据优先", template.PersonaMemoryMarkdown)
	require.Equal(t, []any{"code-reading"}, template.CapabilityBindings["skills"])
	require.Equal(t, float64(10000), template.BudgetPolicy["daily_token_limit"])
}
```

- [ ] **Step 2: Run focused template tests and confirm failure**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestCreateEmployeeTemplateStoresFinalConfigDefaults -count=1
```

Expected: FAIL because template domain still uses old default fields.

- [ ] **Step 3: Add template migration or extend Task 1 migration**

If `digital_employee_templates` already exists from migration `050`, create `052_digital_employee_templates_final_config.sql`:

```sql
ALTER TABLE digital_employee_templates
    ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE digital_employee_templates
    DROP COLUMN IF EXISTS default_capability_selection,
    DROP COLUMN IF EXISTS default_context_policy_override,
    DROP COLUMN IF EXISTS default_approval_policy;

COMMENT ON COLUMN digital_employee_templates.persona_memory_markdown IS '模板预填的人格记忆 Markdown';
COMMENT ON COLUMN digital_employee_templates.capability_bindings IS '模板预填的数字员工能力绑定';
COMMENT ON COLUMN digital_employee_templates.budget_policy IS '模板预填的预算策略';
```

Update `atlas.sum` with:

```bash
cd apps/control-plane
atlas migrate hash --dir file://internal/storage/migrations
```

- [ ] **Step 4: Update sqlc template queries and regenerate**

In `apps/control-plane/internal/storage/queries/digital_employee_templates.sql`, replace old default columns with:

```sql
persona_memory_markdown,
capability_bindings,
budget_policy,
```

Run:

```bash
make -C apps/control-plane generate-sqlc
```

- [ ] **Step 5: Update backend template domain/handler**

Replace template request/record fields with:

```go
PersonaMemoryMarkdown string
CapabilityBindings    map[string]any
BudgetPolicy           map[string]any
```

Handler JSON fields:

```go
PersonaMemoryMarkdown string         `json:"persona_memory_markdown"`
CapabilityBindings    map[string]any `json:"capability_bindings"`
BudgetPolicy           map[string]any `json:"budget_policy"`
```

Reject old fields in template create/update handlers using the same raw-map check from Task 3 for:

```go
[]string{"default_capability_selection", "default_context_policy_override", "default_approval_policy"}
```

Update `contracts/control-plane/openapi.yaml` template create/update/response schemas:

- Remove:

```yaml
default_capability_selection
default_context_policy_override
default_approval_policy
```

- Add:

```yaml
persona_memory_markdown:
  type: string
capability_bindings:
  type: object
  additionalProperties: true
budget_policy:
  type: object
  additionalProperties: true
```

Then regenerate:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

- [ ] **Step 6: Update Web template UI**

In `apps/web/src/features/employees/templates.tsx`, replace the JSON editors for old defaults with:

- `personaMemoryMarkdown` textarea labelled `人格记忆.md`
- `capabilityBindings` JSON textarea labelled `能力绑定`
- `budgetPolicy` JSON textarea labelled `预算策略`

Payload:

```ts
persona_memory_markdown: personaMemoryMarkdown.trim(),
capability_bindings: capabilityBindings,
budget_policy: budgetPolicy,
```

In `template-utils.ts`, make summaries read `template.capability_bindings` instead of `default_capability_selection`.

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateEmployeeTemplate|TestUpdateEmployeeTemplate|TestCreateOptions' -count=1
corepack pnpm --filter ./apps/web run test -- src/features/employees/templates.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/052_digital_employee_templates_final_config.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/queries/digital_employee_templates.sql \
  apps/control-plane/internal/storage/queries/digital_employee_templates.sql.go \
  contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/gen/control_plane.gen.go \
  apps/control-plane/internal/employee/template_types.go \
  apps/control-plane/internal/employee/template_repository.go \
  apps/control-plane/internal/employee/template_service.go \
  apps/control-plane/internal/employee/template_handler.go \
  apps/control-plane/internal/employee/template_service_test.go \
  apps/web/src/features/employees/template-utils.ts \
  apps/web/src/features/employees/templates.tsx \
  apps/web/src/features/employees/templates.test.tsx
git commit -m "feat: align employee templates with final config model"
```

---

### Task 6: Web Create And Config Screens

**Files:**
- Modify: `apps/web/src/features/employees/create.tsx`
- Modify: `apps/web/src/features/employees/config.tsx`
- Modify: `apps/web/src/features/employees/detail.tsx`
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/config.test.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`

**Interfaces:**
- Consumes final API types from Task 4 and template fields from Task 5.
- Produces create payload with `persona_memory_markdown`, `capability_bindings`, and `budget_policy`.

- [ ] **Step 1: Read design rules**

Run:

```bash
sed -n '1,220p' DESIGN.md
```

Expected: developer has current frontend design constraints before editing employee UI.

- [ ] **Step 2: Write failing create-flow test**

In `apps/web/src/features/employees/create.test.tsx`, update the successful submit assertion:

```ts
expect(body).toMatchObject({
  persona_memory_markdown: expect.stringContaining("# 人格画像"),
  capability_bindings: {
    skills: ["incident-diagnosis"],
    mcp_servers: ["postgres-readonly"],
    external_capabilities: [],
    environment_variable_refs: ["PG_DSN"],
  },
  budget_policy: { daily_token_limit: 200000 },
});
expect(body).not.toHaveProperty("role_profile");
expect(body).not.toHaveProperty("constitution_addendum");
expect(body).not.toHaveProperty("capability_selection");
expect(body).not.toHaveProperty("context_policy_override");
expect(body).not.toHaveProperty("approval_policy_override");
expect(body).not.toHaveProperty("output_contract_addendum");
```

Add a visible-copy assertion:

```ts
expect(screen.queryByText("角色配置")).toBeNull();
expect(screen.queryByText("能力与策略")).toBeNull();
expect(screen.getByText("人格记忆.md")).toBeVisible();
expect(screen.getByText("能力绑定")).toBeVisible();
```

- [ ] **Step 3: Run create tests and confirm failure**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: FAIL because current UI submits old fields.

- [ ] **Step 4: Update create draft and payload helpers**

In `create.tsx`, add draft fields:

```ts
persona_memory_markdown: string;
capability_bindings_json: string;
```

Default persona markdown:

```ts
const defaultPersonaMemoryMarkdown = `# 人格画像

# 专业边界

# 工作方式

# 表达偏好

# 协作习惯

# 不应做的事
`;
```

Add parser:

```ts
function capabilityBindingsFromDraft(draft: WizardDraft): CapabilityBindings {
  const parsed = parseJsonObject(draft.capability_bindings_json, "capability_bindings");
  if (!parsed.ok) {
    return {
      skills: [],
      mcp_servers: [],
      external_capabilities: [],
      environment_variable_refs: [],
    };
  }
  return {
    skills: Array.isArray(parsed.value.skills) ? parsed.value.skills : [],
    mcp_servers: Array.isArray(parsed.value.mcp_servers) ? parsed.value.mcp_servers : [],
    external_capabilities: Array.isArray(parsed.value.external_capabilities) ? parsed.value.external_capabilities : [],
    environment_variable_refs: Array.isArray(parsed.value.environment_variable_refs) ? parsed.value.environment_variable_refs : [],
    ...parsed.value,
  };
}
```

Final create payload:

```ts
persona_memory_markdown: draft.persona_memory_markdown.trim(),
capability_bindings: capabilityBindingsFromDraft(draft),
budget_policy: budgetPolicyFromDraft(draft),
```

Remove payload properties:

```ts
role_profile
constitution_addendum
capability_selection
context_policy_override
approval_policy_override
output_contract_addendum
```

- [ ] **Step 5: Update create UI copy and panels**

Replace the old configuration step content with fields:

- Textarea label `人格记忆.md`
- JSON textarea label `能力绑定`
- Budget input label remains budget-specific

Do not add a landing page or explanatory feature text. Keep controls dense and consistent with existing console UI.

- [ ] **Step 6: Update config/detail screens**

In `config.tsx`, replace old JSON editors with:

```ts
persona_memory_markdown
capability_bindings
budget_policy
```

In `detail.tsx`, render final sections:

- `人格记忆.md`
- `能力绑定`
- `预算策略`
- runtime/cache state as read-only if available

Remove references to:

```ts
role_profile
constitution_addendum
capability_selection
context_policy_override
approval_policy_override
output_contract_addendum
```

- [ ] **Step 7: Run focused web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/features/employees/config.test.tsx src/features/employees/index.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/features/employees/create.tsx \
  apps/web/src/features/employees/config.tsx \
  apps/web/src/features/employees/detail.tsx \
  apps/web/src/features/employees/create.test.tsx \
  apps/web/src/features/employees/config.test.tsx \
  apps/web/src/features/employees/index.test.tsx
git commit -m "feat(web): use persona memory employee config"
```

---

### Task 7: Runtime Payload And Persona Projection

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- Modify: `apps/runtime-agent/tests/workspace_files_test.rs`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`

**Interfaces:**
- Consumes final fields from Control Plane payload.
- Produces Runtime structs with:
  - `persona_memory_markdown: Option<String>`
  - `capability_bindings: serde_json::Value`
  - existing `budget_policy` metadata when applicable

- [ ] **Step 1: Write Runtime payload test**

In `apps/runtime-agent/tests/runtime_command_payload_test.rs`, add:

```rust
#[test]
fn parses_persona_memory_and_capability_bindings_for_provision_payload() {
    let mut command = valid_provision_payload();
    command.payload["persona_memory_markdown"] = serde_json::json!("# 人格画像\n证据优先");
    command.payload["capability_bindings"] = serde_json::json!({
        "skills": ["incident-diagnosis"],
        "mcp_servers": ["postgres-readonly"],
        "external_capabilities": [],
        "environment_variable_refs": ["PG_DSN"]
    });

    let payload = RuntimeProvisionInstanceCommandPayload::from_command(&command).unwrap();
    assert_eq!(payload.persona_memory_markdown.as_deref(), Some("# 人格画像\n证据优先"));
    assert_eq!(payload.capability_bindings["skills"][0], "incident-diagnosis");
}
```

- [ ] **Step 2: Run Runtime test and confirm failure**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_persona_memory_and_capability_bindings_for_provision_payload
```

Expected: FAIL because fields do not exist.

- [ ] **Step 3: Update Runtime payload structs**

In `payload.rs`, add to provision and session payloads:

```rust
#[serde(default)]
pub persona_memory_markdown: Option<String>,
#[serde(default)]
pub capability_bindings: serde_json::Value,
```

Add default helper if needed:

```rust
fn default_json_object() -> serde_json::Value {
    serde_json::json!({})
}
```

Use:

```rust
#[serde(default = "default_json_object")]
pub capability_bindings: serde_json::Value,
```

- [ ] **Step 4: Project persona memory as controlled file**

In `executor.rs`, during provision/workspace materialization, if `persona_memory_markdown` is non-empty, write a workspace file named `人格记忆.md` under the employee home using the same atomic/safe write utility used for workspace files. Expected behavior:

```rust
if let Some(markdown) = payload.persona_memory_markdown.as_deref() {
    if !markdown.trim().is_empty() {
        let target = payload.agent_home_dir.join("人格记忆.md");
        atomic_write(&target, markdown.as_bytes())?;
    }
}
```

If `agent_home_dir` is a `String`, convert it with `PathBuf::from(&payload.agent_home_dir)`. Use existing atomic write helper if it is private; otherwise add a small local helper in executor tests only through the production-safe function already used by workspace materialization.

- [ ] **Step 5: Update Control Plane payload builders**

In `service.go` `buildProvisionInstancePayload`, replace old payload fields:

```go
"role_profile": ...
"context_policy_override": ...
"approval_policy_override": ...
"capability_selection": ...
"output_contract_addendum": ...
```

with:

```go
"persona_memory_markdown": configInput.PersonaMemoryMarkdown,
"capability_bindings":     cloneMap(configInput.CapabilityBindings),
"budget_policy":           cloneMap(configInput.BudgetPolicy),
"mcp_servers":             runtimeMCPServersPayload(configInput.CapabilityBindings),
```

In `run_service.go`, make project-task start payload include `persona_memory_markdown` and `capability_bindings`, and make MCP extraction read `capability_bindings.mcp_servers` if that helper still depends on config JSON.

- [ ] **Step 6: Run Runtime and Control Plane focused tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_persona_memory_and_capability_bindings_for_provision_payload
go test ./apps/control-plane/internal/employee -run 'TestBuildStartSessionPayload|TestBuildProvisionInstancePayload|TestBuildStartSessionPayloadIncludesEffectiveMCPServers' -count=1
```

Expected: PASS after updating test fixtures to final fields.

- [ ] **Step 7: Commit**

```bash
git add apps/runtime-agent/src/commands/payload.rs \
  apps/runtime-agent/src/commands/executor.rs \
  apps/runtime-agent/tests/runtime_command_payload_test.rs \
  apps/runtime-agent/tests/runtime_command_executor_test.rs \
  apps/runtime-agent/tests/workspace_files_test.rs \
  apps/control-plane/internal/employee/service.go \
  apps/control-plane/internal/employee/run_service.go \
  apps/control-plane/internal/employee/run_service_test.go
git commit -m "feat(runtime): project persona memory into employee workspace"
```

---

### Task 8: Remove Old Field References Across Codebase

**Files:**
- Modify any remaining files reported by the scans below.
- Likely files: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`, `apps/control-plane/internal/app/planning_profile_adapter.go`, `apps/web/src/features/employees/*`, `apps/web/src/lib/api/*`, generated query files, tests.

**Interfaces:**
- Produces a clean repo scan where old digital employee config fields are absent outside migration history and this plan/spec.

- [ ] **Step 1: Run old-field scan**

Run:

```bash
rg -n "role_profile|constitution_addendum|capability_selection|context_policy_override|approval_policy_override|output_contract_addendum|default_capability_selection|default_context_policy_override|default_approval_policy" \
  apps/control-plane apps/web apps/runtime-agent contracts/control-plane/openapi.yaml \
  -g '*.{go,ts,tsx,rs,sql,yaml}'
```

Expected before cleanup: matches remain. Expected after cleanup: no matches except historical migration files that intentionally document old schema before migration `051`.

- [ ] **Step 2: Update planning profile**

In `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`, replace `RoleProfile` reads with base employee fields and final config fields:

```go
roleProfile := PlanningRoleProfile{
	PrimaryRole:  normalizePlanningString(source.Role),
	Description:  strings.TrimSpace(source.Description),
	EmployeeType: normalizePlanningString(source.EmployeeType),
	SourceRole:   strings.TrimSpace(source.Role),
}
```

If planner needs persona, add `PersonaMemoryMarkdown string` to the source record and include a short excerpt only:

```go
PersonaSummary: firstNonEmptyLine(source.PersonaMemoryMarkdown),
```

Do not reintroduce a role config map.

- [ ] **Step 3: Update capability count/readiness queries**

Any query that counts MCP servers from `capability_selection.enabled_mcp_servers` must read `capability_bindings.mcp_servers`.

SQL replacement:

```sql
CASE
    WHEN jsonb_typeof(decr.capability_bindings -> 'mcp_servers') = 'array'
    THEN jsonb_array_length(decr.capability_bindings -> 'mcp_servers')
    ELSE 0
END AS effective_mcp_server_count
```

- [ ] **Step 4: Update tests and fixtures**

Replace old fixture objects with:

```json
{
  "persona_memory_markdown": "# 人格画像\n证据优先",
  "capability_bindings": {
    "skills": ["incident-diagnosis"],
    "mcp_servers": ["postgres-readonly"],
    "external_capabilities": [],
    "environment_variable_refs": ["PG_DSN"]
  },
  "budget_policy": {
    "daily_token_limit": 12000
  }
}
```

- [ ] **Step 5: Run full local verification gates**

Run:

```bash
corepack pnpm verify:contracts
go test ./apps/control-plane/...
corepack pnpm --filter ./apps/web run test
cargo test --manifest-path apps/runtime-agent/Cargo.toml
rg -n "role_profile|constitution_addendum|capability_selection|context_policy_override|approval_policy_override|output_contract_addendum|default_capability_selection|default_context_policy_override|default_approval_policy" \
  apps/control-plane apps/web apps/runtime-agent contracts/control-plane/openapi.yaml \
  -g '*.{go,ts,tsx,rs,sql,yaml}'
```

Expected: commands pass. The final `rg` should have no active code/contract matches; if migration history matches, list those exact historical files in the task notes and confirm no generated/runtime/web code still references old fields.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane apps/web apps/runtime-agent contracts/control-plane/openapi.yaml
git commit -m "refactor: remove legacy employee config fields"
```

---

### Task 9: Real Development Smoke And Completion Gate

**Files:**
- Modify: `CHANGELOG.md`
- No code changes unless smoke reveals bugs.

**Interfaces:**
- Consumes all prior tasks.
- Produces evidence that current Web, Control Plane, DB, Runtime, and Provider path use final fields.

- [ ] **Step 1: Apply migration to intended dev database**

Check services:

```bash
scripts/dev-services.sh status
```

If Control Plane is running through the dev script, restart it so migrations run:

```bash
scripts/dev-services.sh restart control-plane
```

If applying manually, derive the same development DB URL used by the dev script, then run migrations against that explicit value without printing it:

```bash
DEV_DATABASE_URL="$(
  awk '
    /^[A-Za-z]/ { section=$1 }
    section=="postgres:" && $1=="url:" {
      line=$0
      sub(/^[^"]*"/, "", line)
      sub(/".*$/, "", line)
      print line
      exit
    }
  ' apps/control-plane/config/config.yaml
)"
test -n "$DEV_DATABASE_URL"
DATABASE_URL="$DEV_DATABASE_URL" make -C apps/control-plane migrate-status
DATABASE_URL="$DEV_DATABASE_URL" make -C apps/control-plane migrate-up
```

Expected: migration `051` and `052` if created are applied. Confirm old columns are gone:

```bash
psql "$DEV_DATABASE_URL" -c "\d digital_employee_config_revisions"
```

Expected: table has `persona_memory_markdown`, `capability_bindings`, `budget_policy`; it does not have old config columns.

- [ ] **Step 2: Start current stack**

Run:

```bash
scripts/dev-services.sh restart all
scripts/dev-services.sh status
```

Expected: Temporal, Control Plane, Web, and Runtime Agent are running. If `openfga` is needed for the local auth path, start it explicitly:

```bash
scripts/dev-services.sh start openfga
```

- [ ] **Step 3: Create a digital employee through the real Web**

Use Chrome plug or in-app browser against the running Web. Perform:

1. Open `/employees/new`.
2. Fill identity.
3. Select Provider using the existing Provider step.
4. Fill `人格记忆.md`.
5. Bind at least one available capability or leave empty bindings if no capability exists, but verify payload contains `capability_bindings`.
6. Configure budget.
7. Submit.

Expected: create request returns 2xx/201 and response contains `persona_memory_markdown`, `capability_bindings`, `budget_policy`.

- [ ] **Step 4: Confirm API rejects old fields**

From the authenticated browser tab that is already on the newly created employee detail page, run this in the browser console or Codex browser evaluation context:

```js
const employeeId = location.pathname.match(/\/employees\/([^/?#]+)/)?.[1];
if (!employeeId) {
  throw new Error(`Cannot derive employee id from ${location.pathname}`);
}

const response = await fetch(`/api/v1/digital-employees/${employeeId}/config-revisions`, {
  method: "POST",
  credentials: "include",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    role_profile: { title: "legacy" },
    persona_memory_markdown: "# 人格画像",
  }),
});
const body = await response.text();
console.log(response.status, body);
if (response.status !== 400 || !body.includes("role_profile is no longer supported")) {
  throw new Error(`Expected old-field rejection, got ${response.status}: ${body}`);
}
```

Expected: console prints HTTP status `400` and a message containing `role_profile is no longer supported`.

- [ ] **Step 5: Run a real task smoke**

From the employee detail or project task dispatch path, start a real low-risk task with the new employee. Confirm:

- Runtime receives payload with `persona_memory_markdown`.
- Runtime receives `capability_bindings`.
- MCP/environment projections still appear if configured.
- Employee home contains `人格记忆.md` and not a generated employee `AGENTS.md`.

Use logs or API response bodies to capture evidence. Do not claim Provider execution works unless a real Provider command reaches terminal success.

- [ ] **Step 6: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add to `CHANGELOG.md` under the current section:

```md
- YYYY-MM-DD HH:MM 完成数字员工配置破坏性重构：删除旧个人治理配置字段，改为人格记忆、能力绑定和预算策略，并通过真实创建与 Runtime 投影链路验证。
```

Replace the timestamp with command output.

- [ ] **Step 7: Run completion check**

Use project skill `$superteam-completion-check`. Minimum commands before final:

```bash
git diff --check
corepack pnpm verify:contracts
go test ./apps/control-plane/...
corepack pnpm --filter ./apps/web run test
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: all pass, plus real-chain evidence from Steps 1-5.

- [ ] **Step 8: Commit verification/changelog**

```bash
git add CHANGELOG.md
git commit -m "chore: record employee config refactor verification"
```

---

## Self-Review Checklist

- Spec coverage: covered destructive data cleanup, final fields, old-field removal, Web IA, Runtime projection, governance boundary, tests, and real-chain verification.
- Placeholder scan: no red-flag wording remains.
- Type consistency: final field names are `persona_memory_markdown`, `capability_bindings`, and `budget_policy` in DB/API/Web/Runtime payloads.
- Provider boundary: plan treats `provider_type` as existing create-flow fact only.
- Risk note: Task 1 destructive migration must be run only against intended development databases; production-safe migration is explicitly out of scope.
