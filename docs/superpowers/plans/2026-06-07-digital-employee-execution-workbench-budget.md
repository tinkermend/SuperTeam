# Digital Employee Execution Workbench Budget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert `/employees` into the confirmed execution workbench and add governed daily token budgets that are configurable, visible, and enforced before run dispatch.

**Architecture:** Evolve the existing `/api/v1/digital-employees/overview` read model instead of adding a parallel endpoint. Add `budget_policy` to digital employee config revisions and approved effective config snapshots, use the approved snapshot for daily budget preflight, and render the Web workbench from one overview response.

**Tech Stack:** Go + chi/net/http + pgx + sqlc + Atlas migrations + OpenAPI for Control Plane; React + TanStack Query + TanStack Router + shadcn/ui + SuperTeam liquid components + Vitest for Web.

---

## Scope And Guardrails

- Do not modify team-management files unless a later user request explicitly asks for it. At plan-writing time the worktree had unrelated changes under `apps/web/src/features/teams/...`.
- Do not edit shared initial migrations. Add a forward migration after the current highest migration.
- Do not implement a名册 view toggle. `/employees` is a single workbench view in this plan.
- Do not add run/start/stop controls to the employee cards.
- Do not store budget limits in `digital_employees.metadata`.
- Do not fake a budget limit in the frontend. `null` means no upper limit.

## File Structure

Backend contract and domain:

- Modify: `contracts/control-plane/openapi.yaml`
  - Extend `DigitalEmployeeOverview`, `DigitalEmployeeOverviewSummary`, `DigitalEmployeeOverviewItem`, `DigitalEmployeeBudgetSummary`, create/config revision schemas, effective config schemas.
- Modify: `apps/control-plane/internal/employee/types.go`
  - Add `BudgetPolicy`, workbench status, queue summary, recent event summaries, and budget summary fields.
- Modify: `apps/control-plane/internal/employee/handler.go`
  - Decode/encode `budget_policy`; encode queue summary, workbench status, recent events, and daily budget summary.
- Modify: `apps/control-plane/internal/employee/service.go`
  - Validate `budget_policy`, include it in config/effective snapshots.
- Modify: `apps/control-plane/internal/employee/repository.go`
  - Add budget policy params and daily usage repository methods.
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
  - Map SQL rows into new overview fields and config budget fields.
- Modify: `apps/control-plane/internal/employee/run_repository.go`
  - Extend `RunPreflight` with approved effective config snapshot and today token usage.
- Modify: `apps/control-plane/internal/employee/pg_run_repository.go`
  - Read approved budget policy and Asia/Shanghai daily usage in preflight.
- Modify: `apps/control-plane/internal/employee/run_service.go`
  - Enforce daily budget before active-run conflict and dispatch.

Backend storage:

- Create: `apps/control-plane/internal/storage/migrations/011_digital_employee_budget_policy.sql`
  - Add `digital_employee_config_revisions.budget_policy`.
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
  - Refresh Atlas checksums after migration.
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  - Assert forward migration content and comments.
- Modify: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`
  - Include `budget_policy` in create/list/get config revision queries.
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
  - Extend overview summary/items/filter queries.
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
  - Add or extend run preflight query and daily token usage query if current preflight is generated there.
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`, `apps/control-plane/internal/storage/queries/models.go`, `apps/control-plane/internal/storage/queries/querier.go`
  - Regenerate with the repo sqlc command.

Backend tests:

- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/pg_repository_test.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`
- Modify: `apps/control-plane/internal/employee/run_repository_test.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`

Web:

- Modify: `apps/web/src/lib/api/employees.ts`
  - Add `BudgetPolicy`, queue summary, workbench status, recent events, daily budget summary fields.
- Modify: `apps/web/src/lib/api/employees.test.ts`
  - Cover budget policy payloads and overview response parsing.
- Modify: `apps/web/src/features/employees/create.tsx`
  - Add daily token budget input in governance step.
- Modify: `apps/web/src/features/employees/create.test.tsx`
  - Cover empty/unlimited and positive budget submission.
- Modify: `apps/web/src/features/employees/config.tsx`
  - Add budget policy editor and save to config revision.
- Modify: `apps/web/src/features/employees/config.test.tsx`
  - Cover budget policy save.
- Modify: `apps/web/src/features/employees/index.tsx`
  - Replace table with execution workbench cards and right rail.
- Modify: `apps/web/src/features/employees/index.test.tsx`
  - Cover workbench cards, state labels, queue rail, selected employee events, and budget display.

Docs:

- Modify: `CHANGELOG.md`
  - Add Asia/Shanghai timestamped feature entry after implementation.

### Task 1: Add Budget Policy To Backend Domain And Migration

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/011_digital_employee_budget_policy.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/storage/queries/digital_employee_config.sql`
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`

- [ ] **Step 1: Write the migration test**

Add this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestDigitalEmployeeBudgetPolicyMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/011_digital_employee_budget_policy.sql")
	if err != nil {
		t.Fatalf("read budget policy migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"ALTER TABLE digital_employee_config_revisions",
		"ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"ALTER TABLE digital_employees",
		"metadata",
		"approval_policy_override",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("budget policy migration must not use %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeBudgetPolicyMigration -count=1
```

Expected: fails with `read budget policy migration`.

- [ ] **Step 3: Create the migration**

Create `apps/control-plane/internal/storage/migrations/011_digital_employee_budget_policy.sql`:

```sql
ALTER TABLE digital_employee_config_revisions
    ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限';
```

- [ ] **Step 4: Run the migration test and verify it passes**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestDigitalEmployeeBudgetPolicyMigration -count=1
```

Expected: PASS.

- [ ] **Step 5: Add backend domain types**

In `apps/control-plane/internal/employee/types.go`, add this struct near the config types:

```go
type BudgetPolicy struct {
	DailyTokenLimit *int32
}
```

Then add `BudgetPolicy map[string]any` to:

```go
type EmployeeConfigInput struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
}

type DigitalEmployeeConfigRevision struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RevisionNumber         int32
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
	ApprovedAt             *time.Time
	ArchivedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CreateDigitalEmployeeRequest struct {
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	OwnerUserID            uuid.UUID
	EmployeeType           string
	Name                   string
	AvatarAssetID          string
	Role                   string
	Description            *string
	PermissionPolicy       map[string]any
	ContextPolicy          map[string]any
	ApprovalPolicy         map[string]any
	RiskLevel              string
	Metadata               map[string]any
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	RuntimeNodeID          uuid.UUID
	ProviderType           string
	SessionPolicy          map[string]any
	WorkspacePolicy        map[string]any
}

type CreateDigitalEmployeeConfigRevisionRequest struct {
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	RoleProfile            map[string]any
	ConstitutionAddendum   map[string]any
	CapabilitySelection    map[string]any
	ContextPolicyOverride  map[string]any
	ApprovalPolicyOverride map[string]any
	BudgetPolicy           map[string]any
	OutputContractAddendum map[string]any
	Status                 ConfigRevisionStatus
	ApprovedBy             *uuid.UUID
}
```

- [ ] **Step 6: Update config SQL**

In `apps/control-plane/internal/storage/queries/digital_employee_config.sql`, update config revision insert/select statements so every selected config revision includes `budget_policy`, and create uses:

```sql
COALESCE(sqlc.arg('budget_policy')::jsonb, '{}'::jsonb)
```

The `INSERT INTO digital_employee_config_revisions (...)` column list must include:

```sql
budget_policy,
```

The `RETURNING` and `SELECT` projections must include:

```sql
budget_policy,
```

- [ ] **Step 7: Regenerate sqlc**

Run the repo's SQL generation command:

```bash
make generate-sqlc
```

If the repo does not expose `make generate-sqlc`, run:

```bash
cd apps/control-plane && sqlc generate
```

Expected: generated query structs include `BudgetPolicy`.

- [ ] **Step 8: Commit task 1**

```bash
git add apps/control-plane/internal/storage/migrations/011_digital_employee_budget_policy.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/employee/types.go \
  apps/control-plane/internal/storage/queries/digital_employee_config.sql \
  apps/control-plane/internal/storage/queries
git commit -m "feat(control-plane): add employee budget policy storage"
```

### Task 2: Persist Budget Policy Through Create, Config Revision, Preview, And Approval

**Files:**
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`

- [ ] **Step 1: Write service tests for budget validation and snapshot**

Add tests to `apps/control-plane/internal/employee/service_test.go`:

```go
func TestCreateConfigRevisionStoresBudgetPolicy(t *testing.T) {
	svc, repo := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()

	revision, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RoleProfile:       map[string]any{"role": "analyst"},
		BudgetPolicy:      map[string]any{"daily_token_limit": float64(12000)},
		Status:            ConfigRevisionStatusDraft,
	})

	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}
	if revision.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy on revision, got %#v", revision.BudgetPolicy)
	}
	if repo.createdConfigRevision.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy persisted, got %#v", repo.createdConfigRevision.BudgetPolicy)
	}
}

func TestCreateConfigRevisionRejectsInvalidBudgetPolicy(t *testing.T) {
	svc, _ := newEmployeeServiceForTest(t)
	_, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		BudgetPolicy:      map[string]any{"daily_token_limit": float64(0)},
		Status:            ConfigRevisionStatusDraft,
	})

	if err == nil || !strings.Contains(err.Error(), "budget_policy.daily_token_limit") {
		t.Fatalf("expected budget policy validation error, got %v", err)
	}
}

func TestPreviewEffectiveConfigIncludesBudgetPolicy(t *testing.T) {
	svc, _ := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		TeamConfig: TeamConfigInput{
			ID:       uuid.New(),
			TenantID: tenantID,
			TeamID:   uuid.New(),
			Status:   TeamConfigRevisionStatusActive,
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                uuid.New(),
			TenantID:          tenantID,
			DigitalEmployeeID: employeeID,
			BudgetPolicy:      map[string]any{"daily_token_limit": float64(9000)},
		},
	})

	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	budgetPolicy, ok := preview.EffectiveConfig["budget_policy"].(map[string]any)
	if !ok || budgetPolicy["daily_token_limit"] != float64(9000) {
		t.Fatalf("expected budget policy in effective config, got %#v", preview.EffectiveConfig["budget_policy"])
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateConfigRevision.*Budget|TestPreviewEffectiveConfigIncludesBudgetPolicy' -count=1
```

Expected: FAIL because `BudgetPolicy` is not yet mapped/validated.

- [ ] **Step 3: Add budget policy validation helpers**

In `apps/control-plane/internal/employee/service.go`, add:

```go
func normalizeBudgetPolicy(input map[string]any) (map[string]any, error) {
	policy := cloneMap(input)
	if policy == nil {
		return map[string]any{}, nil
	}
	value, exists := policy["daily_token_limit"]
	if !exists || value == nil || value == "" {
		delete(policy, "daily_token_limit")
		return policy, nil
	}

	var limit int64
	switch typed := value.(type) {
	case int:
		limit = int64(typed)
	case int32:
		limit = int64(typed)
	case int64:
		limit = typed
	case float64:
		if typed != float64(int64(typed)) {
			return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
		}
		limit = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
		}
		limit = parsed
	default:
		return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
	}
	if limit <= 0 || limit > int64(^uint32(0)>>1) {
		return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
	}
	policy["daily_token_limit"] = float64(limit)
	return policy, nil
}
```

Ensure `service.go` imports `encoding/json` if it does not already.

- [ ] **Step 4: Use validation in create/config paths**

In `CreateDigitalEmployee`, before repository create config revision call, normalize:

```go
budgetPolicy, err := normalizeBudgetPolicy(req.BudgetPolicy)
if err != nil {
	return nil, err
}
```

Pass `BudgetPolicy: budgetPolicy` into `CreateDigitalEmployeeConfigRevisionRequest` or repository params.

In `CreateConfigRevision`, normalize:

```go
budgetPolicy, err := normalizeBudgetPolicy(req.BudgetPolicy)
if err != nil {
	return nil, err
}
```

Pass `BudgetPolicy: budgetPolicy` into repository params and returned domain object.

- [ ] **Step 5: Include budget policy in effective config preview**

In the function building the effective config map, include:

```go
"budget_policy": cloneMap(configInput.BudgetPolicy),
```

The resulting approved effective config snapshot must contain `budget_policy`.

- [ ] **Step 6: Map budget policy in pg repository**

In `apps/control-plane/internal/employee/pg_repository.go`, wherever config revision params are encoded, add:

```go
budgetPolicy, err := jsonbFromMap(params.BudgetPolicy, "budget_policy")
if err != nil {
	return nil, err
}
```

Pass it to sqlc params:

```go
BudgetPolicy: budgetPolicy,
```

When mapping SQL rows back to domain:

```go
budgetPolicy, err := mapFromJSONB(revision.BudgetPolicy, "budget_policy")
if err != nil {
	return nil, err
}
```

Set:

```go
BudgetPolicy: budgetPolicy,
```

- [ ] **Step 7: Decode and encode budget policy in handler**

In `CreateDigitalEmployee`, `CreateDigitalEmployeeConfigRevision`, and response structs in `handler.go`, add:

```go
BudgetPolicy map[string]any `json:"budget_policy"`
```

Pass it through request structs and response mappers.

- [ ] **Step 8: Run backend employee tests**

```bash
go test ./apps/control-plane/internal/employee -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit task 2**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/api/employee_routes_test.go
git commit -m "feat(control-plane): include employee budget policy in configs"
```

### Task 3: Enforce Daily Token Budget In Run Preflight

**Files:**
- Modify: `apps/control-plane/internal/employee/run_types.go`
- Modify: `apps/control-plane/internal/employee/run_repository.go`
- Modify: `apps/control-plane/internal/employee/pg_run_repository.go`
- Modify: `apps/control-plane/internal/employee/run_service.go`
- Modify: `apps/control-plane/internal/employee/run_service_test.go`
- Modify: `apps/control-plane/internal/employee/run_repository_test.go`
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`

- [ ] **Step 1: Write run service budget tests**

Add tests to `apps/control-plane/internal/employee/run_service_test.go`:

```go
func TestCreateRunRejectsWhenDailyTokenBudgetExceeded(t *testing.T) {
	repo := newFakeDigitalEmployeeRunRepo()
	dispatcher := &fakeRuntimeDispatcher{connected: true}
	service, err := NewDigitalEmployeeRunService(repo, dispatcher, nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	userID := uuid.New()
	repo.preflight = validRunPreflight(tenantID, employeeID)
	repo.preflight.BudgetPolicy = map[string]any{"daily_token_limit": float64(1000)}
	repo.preflight.TodayTokenUsage = 1000

	_, err = service.CreateRun(context.Background(), CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
		Objective:         "预算验证",
		Prompt:            "执行一次验证",
	})

	if err == nil || !strings.Contains(err.Error(), "employee daily token budget exceeded") {
		t.Fatalf("expected budget exceeded error, got %v", err)
	}
	if dispatcher.dispatchedCommands != 0 {
		t.Fatalf("budget exceeded run must not dispatch command")
	}
	if repo.createdRun != nil {
		t.Fatalf("budget exceeded run must not create run record")
	}
}

func TestCreateRunAllowsWhenDailyTokenBudgetUnset(t *testing.T) {
	repo := newFakeDigitalEmployeeRunRepo()
	dispatcher := &fakeRuntimeDispatcher{connected: true}
	service, err := NewDigitalEmployeeRunService(repo, dispatcher, nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	userID := uuid.New()
	repo.preflight = validRunPreflight(tenantID, employeeID)
	repo.preflight.BudgetPolicy = map[string]any{}
	repo.preflight.TodayTokenUsage = 999999

	_, err = service.CreateRun(context.Background(), CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
		Objective:         "无预算上限验证",
		Prompt:            "执行一次验证",
	})

	if err != nil {
		t.Fatalf("expected run allowed without budget limit, got %v", err)
	}
	if dispatcher.dispatchedCommands != 1 {
		t.Fatalf("expected command dispatch")
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateRun.*DailyTokenBudget' -count=1
```

Expected: FAIL because `RunPreflight` has no budget fields and service does not enforce budget.

- [ ] **Step 3: Extend RunPreflight**

In `apps/control-plane/internal/employee/run_types.go` or the file where `RunPreflight` is defined, add:

```go
BudgetPolicy     map[string]any
TodayTokenUsage  int32
BusinessTimezone string
```

Set `BusinessTimezone` to `"Asia/Shanghai"` in repository results.

- [ ] **Step 4: Add budget enforcement helper**

In `apps/control-plane/internal/employee/run_service.go`, add:

```go
func validateDailyTokenBudget(preflight RunPreflight) error {
	policy, err := normalizeBudgetPolicy(preflight.BudgetPolicy)
	if err != nil {
		return err
	}
	value, ok := policy["daily_token_limit"].(float64)
	if !ok || value <= 0 {
		return nil
	}
	limit := int32(value)
	if preflight.TodayTokenUsage >= limit {
		return fmt.Errorf("%w: employee daily token budget exceeded", ErrInvalidInput)
	}
	return nil
}
```

Call it in `CreateRun` immediately after `validateRunPreflight(preflight)` and before `dispatcher.IsConnected`:

```go
if err := validateDailyTokenBudget(preflight); err != nil {
	return nil, err
}
```

- [ ] **Step 5: Extend SQL preflight query**

Find the current `GetRunPreflight` query in `apps/control-plane/internal/storage/queries/tasks.sql` or `employee_execution.sql`. Add:

```sql
COALESCE(ec.effective_config_snapshot -> 'budget_policy', '{}'::jsonb) AS budget_policy,
COALESCE(today_usage.usage_tokens_today, 0)::integer AS today_token_usage,
'Asia/Shanghai'::text AS business_timezone
```

Add a lateral join for today usage:

```sql
LEFT JOIN LATERAL (
    SELECT
        LEAST(
            SUM(
                CASE
                    WHEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '') ~ '^[0-9]+$'
                    THEN COALESCE(tr.result #>> '{usage,total_tokens}', tr.result ->> 'total_tokens', '')::bigint
                    ELSE 0
                END
            ),
            2147483647
        )::integer AS usage_tokens_today
    FROM task_runs tr
    WHERE tr.tenant_id = de.tenant_id
      AND tr.digital_employee_id = de.id
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
) today_usage ON true
```

- [ ] **Step 6: Regenerate sqlc and map fields**

Run:

```bash
make generate-sqlc
```

In `pg_run_repository.go`, map:

```go
budgetPolicy, err := mapFromJSONB(row.BudgetPolicy, "budget_policy")
if err != nil {
	return RunPreflight{}, err
}
preflight.BudgetPolicy = budgetPolicy
preflight.TodayTokenUsage = row.TodayTokenUsage
preflight.BusinessTimezone = row.BusinessTimezone
```

- [ ] **Step 7: Add repository test for Asia/Shanghai boundary**

In `apps/control-plane/internal/employee/run_repository_test.go`, add a focused test that inserts two `task_runs` for one employee: one just before Asia/Shanghai midnight and one after midnight. Assert only the after-midnight run counts for today's usage. Use concrete UTC timestamps for China midnight:

```go
beforeBusinessDay := time.Date(2026, 6, 6, 15, 59, 0, 0, time.UTC)
insideBusinessDay := time.Date(2026, 6, 6, 16, 1, 0, 0, time.UTC)
```

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestRunPreflightUsesAsiaShanghaiDailyTokenUsage -count=1
```

Expected: PASS after SQL mapping is correct.

- [ ] **Step 8: Run run tests**

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateRun|TestRunPreflight' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit task 3**

```bash
git add apps/control-plane/internal/employee apps/control-plane/internal/storage/queries
git commit -m "feat(control-plane): enforce employee daily token budget"
```

### Task 4: Expand Overview Contract For Workbench Cards

**Files:**
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository_test.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/internal/storage/queries/*.sql.go`

- [ ] **Step 1: Write overview route contract test**

Extend `TestDigitalEmployeeOverviewRouteUsesConsoleTenantAndFilters` in `apps/control-plane/internal/api/employee_routes_test.go` to assert:

```go
if body.Summary.ReadyCount != 1 ||
	body.Summary.PendingRuntimeBindingCount != 0 ||
	body.Summary.PendingConfigApprovalCount != 0 ||
	body.Summary.FailedRecentRunCount != 0 {
	t.Fatalf("unexpected workbench summary: %#v", body.Summary)
}
if body.QueueSummary.PendingRuntimeBindingCount != 0 || body.QueueSummary.StaleConfigCount != 0 || body.QueueSummary.FailedRecentRunCount != 0 {
	t.Fatalf("unexpected queue summary: %#v", body.QueueSummary)
}
if body.Items[0].WorkbenchStatus != "ready" {
	t.Fatalf("expected ready workbench status, got %#v", body.Items[0].WorkbenchStatus)
}
if len(body.Items[0].RecentEvents) != 3 || body.Items[0].RecentEvents[0].Label != "命令已下发" {
	t.Fatalf("expected recent events, got %#v", body.Items[0].RecentEvents)
}
if body.Items[0].BudgetSummary.DailyTokenLimit == nil || *body.Items[0].BudgetSummary.DailyTokenLimit != 10000 {
	t.Fatalf("expected daily token limit, got %#v", body.Items[0].BudgetSummary)
}
```

Add the corresponding fields to the local response struct in the test.

- [ ] **Step 2: Run overview route test and verify it fails**

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeOverviewRouteUsesConsoleTenantAndFilters -count=1
```

Expected: FAIL because response lacks new fields.

- [ ] **Step 3: Add overview domain fields**

In `types.go`, add:

```go
type WorkbenchStatus string

const (
	WorkbenchStatusReady          WorkbenchStatus = "ready"
	WorkbenchStatusPendingBinding WorkbenchStatus = "pending_binding"
	WorkbenchStatusError          WorkbenchStatus = "error"
)

type DigitalEmployeeOverviewQueueSummary struct {
	PendingRuntimeBindingCount int32
	StaleConfigCount          int32
	FailedRecentRunCount      int32
}

type DigitalEmployeeRecentEventSummary struct {
	Label      string
	Status     string
	OccurredAt *time.Time
}
```

Extend existing structs:

```go
type DigitalEmployeeOverview struct {
	Summary      DigitalEmployeeOverviewSummary
	QueueSummary DigitalEmployeeOverviewQueueSummary
	Items        []DigitalEmployeeOverviewItem
	Filters      DigitalEmployeeOverviewFilters
	Pagination   OverviewPagination
}

type DigitalEmployeeOverviewSummary struct {
	TotalCount                  int32
	RunnableCount               int32
	RunningCount                int32
	WaitingRuntimeCount         int32
	ErrorCount                  int32
	HighRiskCount               int32
	ReadyCount                  int32
	PendingRuntimeBindingCount  int32
	PendingConfigApprovalCount  int32
	FailedRecentRunCount        int32
}

type DigitalEmployeeOverviewItem struct {
	IdentitySummary   DigitalEmployeeIdentitySummary
	ExecutionSummary  DigitalEmployeeExecutionSummary
	LatestRunSummary  *DigitalEmployeeLatestRunSummary
	GovernanceSummary DigitalEmployeeGovernanceSummary
	BudgetSummary     DigitalEmployeeBudgetSummary
	WorkbenchStatus   WorkbenchStatus
	RecentEvents      []DigitalEmployeeRecentEventSummary
}

type DigitalEmployeeBudgetSummary struct {
	DailyTokenLimit   *int32
	UsageTokensToday int32
	UsagePercentToday *int32
	LimitExceeded    bool
	UsageTokens30d   *int32
	RunCount30d      int32
	CostAmount30d    *float64
	Currency         string
	Source           string
}
```

- [ ] **Step 4: Add mapper helpers**

In `pg_repository.go`, add:

```go
func overviewWorkbenchStatus(identityStatus DigitalEmployeeStatus, executionStatus OverviewExecutionStatus, runtimeStatus, providerStatus, healthStatus string, governanceStatus string, runStatus OverviewRunStatus) WorkbenchStatus {
	if identityStatus == DigitalEmployeeStatusError ||
		executionStatus == OverviewExecutionStatusError ||
		runStatus == OverviewRunStatusFailed ||
		runStatus == OverviewRunStatusTimedOut {
		return WorkbenchStatusError
	}
	if executionStatus == OverviewExecutionStatusMissing ||
		executionStatus == OverviewExecutionStatusProvisioning ||
		runtimeStatus == "" ||
		providerStatus == "" {
		return WorkbenchStatusPendingBinding
	}
	if governanceStatus == "missing" || governanceStatus == "pending_approval" || governanceStatus == "stale" {
		return WorkbenchStatusPendingBinding
	}
	if runtimeStatus != "online" || providerStatus != "healthy" || healthStatus != "healthy" {
		return WorkbenchStatusError
	}
	return WorkbenchStatusReady
}

func overviewUsagePercent(today int32, limit *int32) *int32 {
	if limit == nil || *limit <= 0 {
		return nil
	}
	percent := int32((int64(today) * 100) / int64(*limit))
	if percent > 100 {
		percent = 100
	}
	return &percent
}
```

- [ ] **Step 5: Extend SQL overview query**

In `employee_execution.sql`, add effective-config budget extraction:

```sql
ec.effective_config_snapshot -> 'budget_policy' AS budget_policy,
NULLIF(ec.effective_config_snapshot #>> '{budget_policy,daily_token_limit}', '') AS daily_token_limit_text,
```

Add today's token CTE similar to Task 3 and select:

```sql
today_budget_usage_tokens,
```

Add recent event CTE:

```sql
recent_events AS (
    SELECT
        ranked.tenant_id,
        ranked.digital_employee_id,
        jsonb_agg(
            jsonb_build_object(
                'label', ranked.event_label,
                'status', ranked.event_status,
                'occurred_at', ranked.occurred_at
            )
            ORDER BY ranked.occurred_at DESC NULLS LAST, ranked.sequence_number DESC
        ) AS recent_events_json
    FROM (
        SELECT
            tr.tenant_id,
            tr.digital_employee_id,
            te.sequence_number,
            CASE
                WHEN te.event_type = 'run_dispatched' THEN '命令已下发'
                WHEN te.event_type ILIKE '%provider%' THEN 'Provider 输出中'
                WHEN te.event_type ILIKE '%complete%' THEN '等待结果回写'
                ELSE te.event_type
            END AS event_label,
            CASE
                WHEN te.event_type ILIKE '%fail%' THEN 'failed'
                WHEN te.event_type ILIKE '%complete%' THEN 'completed'
                ELSE 'running'
            END AS event_status,
            COALESCE(te.created_at, tr.updated_at, tr.created_at) AS occurred_at,
            ROW_NUMBER() OVER (
                PARTITION BY tr.tenant_id, tr.digital_employee_id
                ORDER BY COALESCE(te.created_at, tr.updated_at, tr.created_at) DESC, te.sequence_number DESC
            ) AS row_number
        FROM task_runs tr
        JOIN task_events te
          ON te.tenant_id = tr.tenant_id
         AND te.run_id = tr.id
        JOIN overview_args args ON args.tenant_id = tr.tenant_id
        WHERE tr.digital_employee_id IS NOT NULL
    ) ranked
    WHERE ranked.row_number <= 3
    GROUP BY ranked.tenant_id, ranked.digital_employee_id
)
```

Select:

```sql
COALESCE(re.recent_events_json, '[]'::jsonb) AS recent_events_json,
```

- [ ] **Step 6: Regenerate sqlc and map overview fields**

Run:

```bash
make generate-sqlc
```

In `overviewItemFromQuery`, parse:

```go
dailyTokenLimit := int32PtrFromJSONString(row.DailyTokenLimitText)
usagePercent := overviewUsagePercent(row.TodayBudgetUsageTokens, dailyTokenLimit)
recentEvents := recentEventsFromJSON(row.RecentEventsJson)
workbenchStatus := overviewWorkbenchStatus(
	DigitalEmployeeStatus(row.Status),
	overviewExecutionStatus(row.ExecutionStatus),
	row.RuntimeStatus,
	row.ProviderStatus,
	row.HealthStatus,
	row.GovernanceStatus,
	overviewRunStatus(row.LatestRunStatus),
)
```

Set `BudgetSummary`:

```go
BudgetSummary: DigitalEmployeeBudgetSummary{
	DailyTokenLimit:    dailyTokenLimit,
	UsageTokensToday:  row.TodayBudgetUsageTokens,
	UsagePercentToday: usagePercent,
	LimitExceeded:     dailyTokenLimit != nil && row.TodayBudgetUsageTokens >= *dailyTokenLimit,
	UsageTokens30d:    budgetUsage,
	RunCount30d:       row.BudgetRunCount30d,
	Currency:          "USD",
	Source:            overviewBudgetSource(row.BudgetRunCount30d, budgetUsageValue),
},
WorkbenchStatus: workbenchStatus,
RecentEvents:    recentEvents,
```

Add JSON parser:

```go
func recentEventsFromJSON(raw []byte) []DigitalEmployeeRecentEventSummary {
	if len(raw) == 0 {
		return []DigitalEmployeeRecentEventSummary{}
	}
	var payload []struct {
		Label      string     `json:"label"`
		Status     string     `json:"status"`
		OccurredAt *time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []DigitalEmployeeRecentEventSummary{}
	}
	events := make([]DigitalEmployeeRecentEventSummary, 0, len(payload))
	for _, item := range payload {
		events = append(events, DigitalEmployeeRecentEventSummary{
			Label:      item.Label,
			Status:     item.Status,
			OccurredAt: item.OccurredAt,
		})
	}
	return events
}
```

- [ ] **Step 7: Encode response fields**

In `handler.go`, add response structs and mapping:

```go
type digitalEmployeeOverviewResponse struct {
	Summary      digitalEmployeeOverviewSummaryResponse      `json:"summary"`
	QueueSummary digitalEmployeeOverviewQueueSummaryResponse `json:"queue_summary"`
	Items        []digitalEmployeeOverviewItemResponse       `json:"items"`
	Filters      digitalEmployeeOverviewFiltersResponse      `json:"filters"`
	Pagination   overviewPaginationResponse                  `json:"pagination"`
}

type digitalEmployeeOverviewQueueSummaryResponse struct {
	PendingRuntimeBindingCount int32 `json:"pending_runtime_binding_count"`
	StaleConfigCount          int32 `json:"stale_config_count"`
	FailedRecentRunCount      int32 `json:"failed_recent_run_count"`
}
```

Add summary fields:

```go
ReadyCount                 int32 `json:"ready_count"`
PendingRuntimeBindingCount int32 `json:"pending_runtime_binding_count"`
PendingConfigApprovalCount int32 `json:"pending_config_approval_count"`
FailedRecentRunCount       int32 `json:"failed_recent_run_count"`
```

Add item fields:

```go
WorkbenchStatus WorkbenchStatus                              `json:"workbench_status"`
RecentEvents    []digitalEmployeeRecentEventSummaryResponse `json:"recent_events"`
```

Add recent event response:

```go
type digitalEmployeeRecentEventSummaryResponse struct {
	Label      string  `json:"label"`
	Status     string  `json:"status"`
	OccurredAt *string `json:"occurred_at,omitempty"`
}
```

Add budget fields:

```go
DailyTokenLimit    *int32 `json:"daily_token_limit,omitempty"`
UsageTokensToday   int32  `json:"usage_tokens_today"`
UsagePercentToday  *int32 `json:"usage_percent_today,omitempty"`
LimitExceeded      bool   `json:"limit_exceeded"`
```

- [ ] **Step 8: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, add matching schema properties for:

- `queue_summary`
- `ready_count`
- `pending_runtime_binding_count`
- `pending_config_approval_count`
- `failed_recent_run_count`
- `workbench_status`
- `recent_events`
- `budget_summary.daily_token_limit`
- `budget_summary.usage_tokens_today`
- `budget_summary.usage_percent_today`
- `budget_summary.limit_exceeded`
- request/response `budget_policy`

- [ ] **Step 9: Run backend tests**

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeOverviewRouteUsesConsoleTenantAndFilters -count=1
go test ./apps/control-plane/internal/employee -run 'TestOverview|TestDigitalEmployeeOverview' -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit task 4**

```bash
git add contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/employee_routes_test.go \
  apps/control-plane/internal/employee \
  apps/control-plane/internal/storage/queries
git commit -m "feat(control-plane): expand employee overview workbench model"
```

### Task 5: Update Web API Types And Tests

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`

- [ ] **Step 1: Write Web API tests**

In `apps/web/src/lib/api/employees.test.ts`, add assertions to the existing overview test:

```ts
expect(fetcher).toHaveBeenCalledWith(
  "http://control-plane.local/api/v1/digital-employees/overview?q=%E9%9C%80%E6%B1%82&team_id=team-1&status=active&employee_type=requirements_analyst&provider_type=codex&runtime_node_id=runtime-1&risk_level=medium&execution_status=missing&run_status=none&limit=25&offset=5",
  expect.objectContaining({ credentials: "include", method: "GET" }),
)
expect(overview.queue_summary.pending_runtime_binding_count).toBe(2)
expect(overview.items[0].workbench_status).toBe("ready")
expect(overview.items[0].recent_events[0].label).toBe("命令已下发")
expect(overview.items[0].budget_summary.daily_token_limit).toBe(10000)
```

Add create/config request tests:

```ts
expect(JSON.parse(String(createRequestBody))).toMatchObject({
  budget_policy: { daily_token_limit: 12000 },
})
```

- [ ] **Step 2: Run Web API tests and verify they fail**

```bash
pnpm --filter @superteam/web test -- employees.ts
```

Expected: FAIL because types do not include new fields.

- [ ] **Step 3: Add API types**

In `apps/web/src/lib/api/employees.ts`, add:

```ts
export type BudgetPolicy = {
  daily_token_limit?: number | null;
};

export type DigitalEmployeeWorkbenchStatus = "ready" | "pending_binding" | "error";

export type DigitalEmployeeRecentEventSummary = {
  label: string;
  status: string;
  occurred_at?: string;
};
```

Extend:

```ts
export type DigitalEmployeeOverview = {
  summary: {
    total_count: number;
    runnable_count: number;
    running_count: number;
    waiting_runtime_count: number;
    error_count: number;
    high_risk_count: number;
    ready_count: number;
    pending_runtime_binding_count: number;
    pending_config_approval_count: number;
    failed_recent_run_count: number;
  };
  queue_summary: {
    pending_runtime_binding_count: number;
    stale_config_count: number;
    failed_recent_run_count: number;
  };
  items: DigitalEmployeeOverviewItem[];
  filters: { /* keep existing filter shape */ };
  pagination: { limit: number; offset: number; total_count: number };
};
```

Extend item:

```ts
workbench_status: DigitalEmployeeWorkbenchStatus;
recent_events: DigitalEmployeeRecentEventSummary[];
```

Extend budget summary:

```ts
daily_token_limit?: number | null;
usage_tokens_today: number;
usage_percent_today?: number | null;
limit_exceeded: boolean;
```

Add `budget_policy?: BudgetPolicy` to create input and config revision input.

- [ ] **Step 4: Run Web API tests**

```bash
pnpm --filter @superteam/web test -- employees.ts
```

Expected: PASS.

- [ ] **Step 5: Commit task 5**

```bash
git add apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts
git commit -m "feat(web): type employee workbench overview budget fields"
```

### Task 6: Add Budget Inputs To Create And Config Pages

**Files:**
- Modify: `apps/web/src/features/employees/create.tsx`
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/config.tsx`
- Modify: `apps/web/src/features/employees/config.test.tsx`

- [ ] **Step 1: Write create-page budget tests**

In `apps/web/src/features/employees/create.test.tsx`, add tests using the same interaction style as the existing four-step wizard tests:

```tsx
it("submits an optional daily token budget when creating a digital employee", async () => {
  const fetcher = createWizardFetcher();
  const screen = await renderCreateEmployeeView(fetcher);

  await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
  await userEvent.fill(screen.getByLabelText("描述"), "负责生产数据库变更和恢复验证");
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), "12000");
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await expect.element(screen.getByLabelText("客户侧执行机 A / codex")).toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));

  const createCall = fetcher.mock.calls.find(
    ([input, init]) => String(input).endsWith("/api/v1/digital-employees") && init?.method === "POST",
  );
  expect(createCall).toBeTruthy();
  const body = JSON.parse(String(createCall?.[1]?.body));
  expect(body.budget_policy).toEqual({ daily_token_limit: 12000 });
});

it("omits daily token budget when the field is empty", async () => {
  const fetcher = createWizardFetcher();
  const screen = await renderCreateEmployeeView(fetcher);

  await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await expect.element(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" })).toHaveValue(null);
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: "创建数字员工" }));

  const createCall = fetcher.mock.calls.find(
    ([input, init]) => String(input).endsWith("/api/v1/digital-employees") && init?.method === "POST",
  );
  expect(createCall).toBeTruthy();
  const body = JSON.parse(String(createCall?.[1]?.body));
  expect(body.budget_policy).toEqual({});
});
```

- [ ] **Step 2: Run create tests and verify they fail**

```bash
pnpm --filter @superteam/web test -- create
```

Expected: FAIL because budget input is missing.

- [ ] **Step 3: Add create draft field**

In `create.tsx`, add to `WizardDraft`:

```ts
daily_token_limit: string;
```

Set in `emptyDraft`:

```ts
daily_token_limit: "",
```

Extend `ValidationErrors`:

```ts
type ValidationErrors = Partial<
  Record<
    "avatar_asset_id" | "daily_token_limit" | "employee_type" | "name" | "role" | "runtime" | "team_id",
    string
  >
>;
```

Add payload helper:

```ts
function budgetPolicyFromDraft(draft: WizardDraft) {
  const trimmed = draft.daily_token_limit.trim();
  if (!trimmed) return {};
  return { daily_token_limit: Number(trimmed) };
}
```

Pass into `createDigitalEmployee`:

```ts
budget_policy: budgetPolicyFromDraft(draft),
```

- [ ] **Step 4: Add create form input**

In the governance step render block, add:

```tsx
<div className="space-y-2">
  <Label htmlFor="daily-token-limit">每日 Token 预算上限</Label>
  <Input
    id="daily-token-limit"
    inputMode="numeric"
    min={1}
    type="number"
    value={draft.daily_token_limit}
    onChange={(event) => updateDraft({ daily_token_limit: event.target.value })}
    placeholder="不填写表示无预算上限"
  />
  <p className="text-xs text-muted-foreground">不填写表示无预算上限。填写后，达到当日上限会阻止发起新的运行。</p>
</div>
```

Extend validation for governance step:

```ts
if (draft.daily_token_limit.trim()) {
  const parsed = Number(draft.daily_token_limit)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    nextErrors.daily_token_limit = "每日 Token 预算上限必须是正整数"
  }
}
```

- [ ] **Step 5: Write config-page budget tests**

In `config.test.tsx`, add:

```tsx
it("submits budget policy as part of a config revision", async () => {
  const queryClient = createQueryClient();
  const fetcher = vi.fn(async (input: RequestInfo | URL, options?: RequestInit) => {
    const url = requestUrl(input);
    const method = requestMethod(input, options);
    if (url.includes("/digital-employees/") && method === "POST") {
      return new Response(JSON.stringify({ id: "revision-1", status: "draft" }), {
        status: 201,
        headers: { "content-type": "application/json" },
      });
    }
    if (url.includes("/digital-employees/")) {
      return new Response(JSON.stringify(employee), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    return new Response(JSON.stringify({}), { status: 404 });
  });

  const screen = await render(
    <QueryClientProvider client={queryClient}>
      <EmployeeConfigView
        apiBaseUrl="http://localhost:8080"
        employeeId={employee.id}
        fetcher={fetcher}
      />
    </QueryClientProvider>,
  );

  await expect.element(screen.getByRole("button", { name: /保存配置/ })).toBeVisible();
  await userEvent.type(screen.getByRole("spinbutton", { name: "每日 Token 预算上限" }), "15000");
  await userEvent.click(screen.getByRole("button", { name: /保存配置/ }));

  const postCall = fetcher.mock.calls.find(([input, init]) => requestUrl(input).includes("/config-revisions") && init?.method === "POST");
  expect(postCall).toBeTruthy();
  const body = JSON.parse(String(postCall?.[1]?.body));
  expect(body.budget_policy).toEqual({ daily_token_limit: 15000 });
});
```

- [ ] **Step 6: Add config-page budget editor**

In `config.tsx`, add state:

```ts
const [dailyTokenLimit, setDailyTokenLimit] = useState("");
```

In submit payload:

```ts
budget_policy: dailyTokenLimit.trim()
  ? { daily_token_limit: Number(dailyTokenLimit.trim()) }
  : {},
```

Add a card before submit buttons:

```tsx
<Card>
  <CardHeader>
    <CardTitle>预算策略</CardTitle>
  </CardHeader>
  <CardContent className="space-y-2">
    <Label htmlFor="config-daily-token-limit">每日 Token 预算上限</Label>
    <Input
      id="config-daily-token-limit"
      inputMode="numeric"
      min={1}
      type="number"
      value={dailyTokenLimit}
      onChange={(event) => setDailyTokenLimit(event.target.value)}
      placeholder="不填写表示无预算上限"
    />
    <p className="text-xs text-muted-foreground">预算会进入新的配置版本，批准后生效。</p>
  </CardContent>
</Card>
```

- [ ] **Step 7: Run Web create/config tests**

```bash
pnpm --filter @superteam/web test -- create config
```

Expected: PASS.

- [ ] **Step 8: Commit task 6**

```bash
git add apps/web/src/features/employees/create.tsx \
  apps/web/src/features/employees/create.test.tsx \
  apps/web/src/features/employees/config.tsx \
  apps/web/src/features/employees/config.test.tsx
git commit -m "feat(web): configure employee daily token budgets"
```

### Task 7: Build The Employee Execution Workbench UI

**Files:**
- Modify: `apps/web/src/features/employees/index.tsx`
- Modify: `apps/web/src/features/employees/index.test.tsx`

- [ ] **Step 1: Write workbench rendering tests**

In `index.test.tsx`, update the overview mock to include:

```ts
queue_summary: {
  pending_runtime_binding_count: 2,
  stale_config_count: 4,
  failed_recent_run_count: 1,
},
summary: {
  total_count: 18,
  runnable_count: 14,
  running_count: 5,
  waiting_runtime_count: 2,
  error_count: 1,
  high_risk_count: 3,
  ready_count: 14,
  pending_runtime_binding_count: 2,
  pending_config_approval_count: 4,
  failed_recent_run_count: 1,
},
```

Add assertions:

```tsx
await expect.element(screen.getByText("就绪")).toBeVisible()
await expect.element(screen.getByText("待绑定")).toBeVisible()
await expect.element(screen.getByText("异常")).toBeVisible()
await expect.element(screen.getByText("配置待审批")).toBeVisible()
await expect.element(screen.getByText("运行失败")).toBeVisible()
await expect.element(screen.getByText("local-dev-node · Claude Code")).toBeVisible()
await expect.element(screen.getByText("成功 · 2 分钟前")).toBeVisible()
await expect.element(screen.getByText("无预算上限")).toBeVisible()
await expect.element(screen.getByText("待处理队列")).toBeVisible()
await expect.element(screen.getByText("最近运行失败")).toBeVisible()
await expect.element(screen.getByText("命令已下发")).toBeVisible()
expect(screen.queryByText("执行实例 ready")).not.toBeInTheDocument()
expect(screen.queryByText("Server")).not.toBeInTheDocument()
expect(screen.queryByRole("button", { name: /启动|停止/ })).not.toBeInTheDocument()
```

- [ ] **Step 2: Run employee page test and verify it fails**

```bash
pnpm --filter @superteam/web test -- index
```

Expected: FAIL because current page renders table.

- [ ] **Step 3: Replace table with workbench state**

In `index.tsx`, remove `EmployeeOverviewTable` usage and introduce selected id state:

```tsx
const [selectedEmployeeId, setSelectedEmployeeId] = useState<string>();
const selectedItem = useMemo(() => {
  if (items.length === 0) return undefined;
  return items.find((item) => item.identity_summary.id === selectedEmployeeId) ?? items[0];
}, [items, selectedEmployeeId]);
```

Use `useEffect` to keep selection valid:

```tsx
useEffect(() => {
  if (items.length === 0) {
    setSelectedEmployeeId(undefined);
    return;
  }
  if (!selectedEmployeeId || !items.some((item) => item.identity_summary.id === selectedEmployeeId)) {
    setSelectedEmployeeId(items[0].identity_summary.id);
  }
}, [items, selectedEmployeeId]);
```

- [ ] **Step 4: Add queue metric cards**

Replace `SummaryMetrics` with:

```tsx
function WorkbenchMetrics({ overview }: { overview: DigitalEmployeeOverview }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <MetricCard title="就绪" value={formatNumber(overview.summary.ready_count)} icon={<Check />} iconTone="success" />
      <MetricCard title="待绑定" value={formatNumber(overview.summary.pending_runtime_binding_count)} icon={<LinkIcon />} iconTone="warning" />
      <MetricCard title="异常" value={formatNumber(overview.summary.error_count)} icon={<AlertTriangle />} iconTone="danger" />
      <MetricCard title="配置待审批" value={formatNumber(overview.summary.pending_config_approval_count)} icon={<ClipboardCheck />} iconTone="artifact" />
      <MetricCard title="运行失败" value={formatNumber(overview.summary.failed_recent_run_count)} icon={<XCircle />} iconTone="danger" />
    </div>
  );
}
```

Import `ClipboardCheck`, `Link as LinkIcon`, and `XCircle` from `lucide-react`; import `useEffect` and `useMemo` from React; import `cn` from `@/lib/utils`; import `DigitalEmployeeOverview`, `DigitalEmployeeOverviewItem`, and `DigitalEmployeeWorkbenchStatus` from `@/lib/api/employees`.

- [ ] **Step 5: Add employee card component**

Add:

```tsx
function EmployeeWorkbenchCard({ item, selected, onSelect }: { item: DigitalEmployeeOverviewItem; selected: boolean; onSelect: () => void }) {
  const identity = item.identity_summary;
  const avatarAsset = overviewAvatarAsset(item);
  return (
    <article
      role="button"
      tabIndex={0}
      className={cn(
        "group relative flex min-h-[260px] cursor-pointer flex-col overflow-hidden rounded-lg border bg-card/90 p-4 text-left shadow-sm transition hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected && "ring-2 ring-[var(--superteam-menu-accent)]",
      )}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect();
        }
      }}
    >
      <span className={cn("absolute inset-y-0 left-0 w-1", workbenchRailClass(item.workbench_status))} />
      <div className="flex items-start gap-3 pl-1">
        <EmployeeAvatar asset={avatarAsset} name={identity.name} size="md" />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate font-semibold text-foreground">{identity.name}</p>
              <p className="truncate text-xs text-muted-foreground">{identity.employee_type_label || identity.role}</p>
            </div>
            <StatusBadge tone={workbenchTone(item.workbench_status)}>{workbenchStatusLabel(item.workbench_status)}</StatusBadge>
          </div>
          <span className="mt-2 inline-flex max-w-full rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">{identity.team_name || "未分组"}</span>
        </div>
      </div>
      <div className="mt-4 space-y-3 text-sm">
        <div className="border-t pt-3 font-medium text-foreground">{runtimeProviderLine(item)}</div>
        <div>
          <p className="text-xs text-muted-foreground">最近运行</p>
          <p className={cn("mt-1 font-medium", latestRunToneClass(item.latest_run_summary?.status))}>{latestRunCompact(item)}</p>
        </div>
        <p className="text-xs text-muted-foreground">{governanceLine(item)}</p>
        <BudgetBar summary={item.budget_summary} />
      </div>
      <div className="mt-auto grid grid-cols-2 gap-2 border-t pt-3">
        <Button asChild size="sm" variant="ghost" onClick={(event) => event.stopPropagation()}>
          <Link params={{ employeeId: identity.id }} to="/employees/$employeeId">详情</Link>
        </Button>
        <Button asChild size="sm" variant="ghost" onClick={(event) => event.stopPropagation()}>
          <Link params={{ employeeId: identity.id }} to="/employees/$employeeId/config">配置</Link>
        </Button>
      </div>
    </article>
  );
}
```

Ensure there is no run/start/stop button.

- [ ] **Step 6: Add helper functions**

Add:

```ts
function workbenchStatusLabel(status: DigitalEmployeeWorkbenchStatus) {
  return status === "ready" ? "就绪" : status === "pending_binding" ? "待绑定" : "异常";
}

function workbenchTone(status: DigitalEmployeeWorkbenchStatus): Tone {
  return status === "ready" ? "success" : status === "pending_binding" ? "warning" : "danger";
}

function workbenchRailClass(status: DigitalEmployeeWorkbenchStatus) {
  if (status === "ready") return "bg-emerald-500";
  if (status === "pending_binding") return "bg-amber-500";
  return "bg-destructive";
}

function runtimeProviderLine(item: DigitalEmployeeOverviewItem) {
  const execution = item.execution_summary;
  if (item.workbench_status === "pending_binding" || !execution.runtime_node_id) {
    return "等待绑定 Runtime Agent";
  }
  const runtime = execution.node_id || execution.runtime_name || "Runtime Agent";
  const provider = providerLabel(execution.provider_type);
  return `${runtime} · ${provider}`;
}

function providerLabel(value: string) {
  const labels: Record<string, string> = {
    claude_code: "Claude Code",
    claude: "Claude Code",
    opencode: "OpenCode",
    open_code: "OpenCode",
    codex: "Codex",
  };
  return labels[value] ?? value;
}

function latestRunCompact(item: DigitalEmployeeOverviewItem) {
  const run = item.latest_run_summary;
  if (!run || run.status === "none") return "-";
  const label = run.status === "completed" ? "成功" : "失败";
  return `${label} · ${runTimeLabel(run)}`;
}

function latestRunToneClass(status?: string) {
  if (status === "completed") return "text-emerald-600";
  if (status === "failed" || status === "timed_out") return "text-destructive";
  return "text-muted-foreground";
}

function governanceLine(item: DigitalEmployeeOverviewItem) {
  const governance = item.governance_summary;
  const revision = governance.employee_revision_number ?? governance.team_revision_number;
  const revisionText = revision ? `配置 v${revision}` : "配置";
  return `${revisionText} ${governanceStatusCompact(governance.status)} · skills ${formatNumber(governance.skills_count)} · MCP ${formatNumber(governance.mcp_servers_count)}`;
}

function governanceStatusCompact(status: string) {
  if (status === "approved") return "已审批";
  if (status === "pending_approval" || status === "draft" || status === "stale") return "待审批";
  return "未配置";
}
```

- [ ] **Step 7: Add BudgetBar**

Add:

```tsx
function BudgetBar({ summary }: { summary: DigitalEmployeeOverviewItem["budget_summary"] }) {
  if (!summary.daily_token_limit) {
    return <p className="text-xs text-muted-foreground">Token 预算：无预算上限</p>;
  }
  const percent = Math.min(summary.usage_percent_today ?? 0, 100);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Token 预算</span>
        <span>{formatNumber(summary.usage_tokens_today)} / {formatNumber(summary.daily_token_limit)}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div className={cn("h-full rounded-full", summary.limit_exceeded ? "bg-destructive" : "bg-[var(--superteam-menu-accent)]")} style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}
```

- [ ] **Step 8: Add right rail**

Add:

```tsx
function QueueRow({ action, label, tone, value }: { action: string; label: string; tone: Tone; value: number }) {
  return (
    <div className="flex items-center justify-between gap-3 border-t py-3 first:border-t-0 first:pt-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <StatusBadge tone={tone}>{formatNumber(value)}</StatusBadge>
          <p className="truncate text-sm font-medium">{label}</p>
        </div>
        <p className="text-xs text-muted-foreground">{formatNumber(value)} 个数字员工</p>
      </div>
      <Button size="sm" variant="outline" type="button">
        {action}
      </Button>
    </div>
  );
}

function SelectedEmployeePanel({ item }: { item: DigitalEmployeeOverviewItem }) {
  const identity = item.identity_summary;
  const avatarAsset = overviewAvatarAsset(item);
  return (
    <div className="space-y-4">
      <div>
        <h2 className="font-semibold">选中员工</h2>
      </div>
      <div className="flex items-start gap-3">
        <EmployeeAvatar asset={avatarAsset} name={identity.name} size="lg" />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="truncate font-semibold">{identity.name}</p>
            <StatusBadge tone={workbenchTone(item.workbench_status)}>{workbenchStatusLabel(item.workbench_status)}</StatusBadge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{identity.employee_type_label || identity.role} · {identity.team_name || "未分组"}</p>
        </div>
      </div>
      <div className="space-y-1 text-sm">
        <p className="text-xs text-muted-foreground">绑定</p>
        <p className="font-medium">{runtimeProviderLine(item)}</p>
      </div>
      <div className="space-y-3">
        <p className="text-xs text-muted-foreground">最新事件</p>
        {item.recent_events.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无最近事件</p>
        ) : (
          <ol className="space-y-3">
            {item.recent_events.map((event, index) => (
              <li className="flex items-start gap-3" key={`${event.label}-${event.occurred_at ?? index}`}>
                <span className={cn("mt-1 size-2 rounded-full", event.status === "failed" ? "bg-destructive" : "bg-[var(--superteam-menu-accent)]")} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm">{event.label}</p>
                  <p className="text-xs text-muted-foreground">{event.occurred_at ? eventTimeLabel(event.occurred_at) : "-"}</p>
                </div>
              </li>
            ))}
          </ol>
        )}
      </div>
      <Button className="w-full" type="button" variant="outline">
        查看审计
      </Button>
    </div>
  );
}

function WorkbenchRail({ overview, selectedItem }: { overview: DigitalEmployeeOverview; selectedItem?: DigitalEmployeeOverviewItem }) {
  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <LiquidCard className="rounded-xl p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-semibold">待处理队列</h2>
        </div>
        <QueueRow label="待绑定 Runtime" value={overview.queue_summary.pending_runtime_binding_count} action="绑定" tone="warning" />
        <QueueRow label="配置过期" value={overview.queue_summary.stale_config_count} action="审批" tone="artifact" />
        <QueueRow label="最近运行失败" value={overview.queue_summary.failed_recent_run_count} action="查看" tone="danger" />
      </LiquidCard>
      <LiquidCard className="rounded-xl p-4">
        {selectedItem ? <SelectedEmployeePanel item={selectedItem} /> : <p className="text-sm text-muted-foreground">暂无选中员工</p>}
      </LiquidCard>
    </aside>
  );
}
```

Add this helper for event times:

```ts
function eventTimeLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}
```

- [ ] **Step 9: Compose layout**

In the main loaded state, render:

```tsx
{overview.data ? <WorkbenchMetrics overview={overview.data} /> : null}
...
{items.length > 0 ? (
  <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
    <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
      {items.map((item) => (
        <EmployeeWorkbenchCard
          key={item.identity_summary.id}
          item={item}
          selected={selectedItem?.identity_summary.id === item.identity_summary.id}
          onSelect={() => setSelectedEmployeeId(item.identity_summary.id)}
        />
      ))}
    </div>
    <WorkbenchRail overview={overview.data} selectedItem={selectedItem} />
  </div>
) : null}
```

- [ ] **Step 10: Run Web employee tests**

```bash
pnpm --filter @superteam/web test -- employees
```

Expected: PASS.

- [ ] **Step 11: Commit task 7**

```bash
git add apps/web/src/features/employees/index.tsx apps/web/src/features/employees/index.test.tsx
git commit -m "feat(web): render employee execution workbench"
```

### Task 8: Final Verification, Changelog, And Browser Check

**Files:**
- Modify: `CHANGELOG.md`
- Generated/changed files from earlier tasks

- [ ] **Step 1: Add changelog entry**

Get Asia/Shanghai time:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add an entry to `CHANGELOG.md` using that exact timestamp:

```markdown
- YYYY-MM-DD HH:mm 数字员工首页改为执行工作台卡片视图，新增每日 Token 预算治理配置、overview 预算摘要和运行前预算拦截。
```

- [ ] **Step 2: Run backend tests**

```bash
go test ./apps/control-plane/internal/...
```

Expected: PASS.

- [ ] **Step 3: Run Web tests**

```bash
pnpm --filter @superteam/web test -- employees
```

Expected: PASS.

- [ ] **Step 4: Run Web typecheck**

```bash
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 5: Run diff whitespace check**

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 6: Browser verify `/employees`**

Start the app using the repo's normal commands if not already running:

```bash
pnpm dev:control-plane
pnpm dev:web
```

Open `http://127.0.0.1:3000/employees` in the browser. Verify:

- The page shows cards, not the old 10-column table.
- Only `就绪`、`待绑定`、`异常` appear as card primary statuses.
- Card Runtime row is only `Runtime Agent · Provider` or `等待绑定 Runtime Agent`.
- No `Server` chip appears.
- No `执行实例 ready` text appears.
- No start/play/stop buttons appear.
- Right rail shows `待处理队列` and selected employee events.
- Desktop has no overlap or horizontal overflow.
- Mobile/narrow viewport stacks the right rail below the cards.

- [ ] **Step 7: Commit final verification artifacts**

```bash
git add CHANGELOG.md
git commit -m "docs: record employee workbench budget rollout"
```

- [ ] **Step 8: Final status check**

```bash
git status --short
```

Expected: clean except unrelated user changes that existed before execution.

## Self-Review Notes

- Spec coverage: backend overview expansion is covered by Task 4; budget config and effective snapshots by Tasks 1-2; budget preflight by Task 3; create/config Web inputs by Task 6; workbench UI by Task 7; verification/changelog by Task 8.
- Type consistency: use `budget_policy`, `daily_token_limit`, `workbench_status`, `queue_summary`, `recent_events`, `usage_tokens_today`, `usage_percent_today`, and `limit_exceeded` consistently across backend, OpenAPI, and Web.
- Scope check: this remains one coherent vertical slice because the UI depends on the expanded overview contract and the budget display depends on the same backend policy used by run preflight.
