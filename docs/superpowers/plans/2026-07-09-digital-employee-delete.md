# Digital Employee Delete Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an auditable digital employee deletion flow that blocks active work, soft-deletes the employee and current projection state, hides the employee from current team/project selection surfaces, and preserves historical runs, project tasks, artifacts, and audit records.

**Architecture:** Control Plane owns the business deletion as a single transactional service method: lock the current employee, query run/project-task blockers, collect cleanup hints, soft-delete/disable current projection rows, write one audit event, then commit. The HTTP route returns `204` on success and a structured `409 digital_employee_delete_blocked` body on blockers. The Web detail page exposes a destructive confirm dialog gated by `allowed_actions` and displays blocker details from the API.

**Tech Stack:** Go + chi + pgx/v5 + sqlc + Atlas migrations/checksum for Control Plane; React + TanStack Query/Router + Vitest browser tests + v3 SuperTeam UI components for Web.

## Global Constraints

- Deletion endpoint is `DELETE /api/v1/digital-employees/{employeeId}`.
- Delete blocking statuses are exactly `task_runs.status IN ('queued','dispatching','running','cancelling')` and `project_tasks.status IN ('queued','running','in_progress')`.
- Any blocker returns `409 Conflict` with `code: "digital_employee_delete_blocked"`, a Chinese user-facing `message`, and `blockers[]` containing at least `type`, `id`, `status`, and a readable `title`.
- Already deleted employees return `404 Not Found`.
- Successful deletion is soft-delete/current-state cleanup; do not physically delete Runtime directories, runs, project tasks, artifacts, execution ledger, provider sessions, or audit records.
- Current `project_employee_node_affinity` rows for the deleted employee are removable projection cache and must be deleted; historical `project_tasks`, route decisions, attempts, reports, and attestations must be retained.
- Successful deletion must write `audit_events` action `digital_employee.delete` with cleanup candidates and cascade counts, and must not include environment variable plaintext, secrets, tokens, credential secret values, or MCP credential values.
- `employee.delete` is tenant owner/admin only; employee owners with only member-level tenant membership must not be allowed to delete.
- Console and Control Plane must not call Runtime Agent, stop running tasks, or delete local files as part of this request.
- Production migrations live only in `apps/control-plane/internal/storage/migrations/`; if a migration is added, update `atlas.sum` and run `make -C apps/control-plane migrate-validate`.
- SQL changes under `apps/control-plane/internal/storage/queries/` must run `make -C apps/control-plane generate-sqlc`.
- Web page/layout/style changes must follow `DESIGN.md`; use existing v3 components/tokens and TanStack Router `navigate`/`Link` for internal navigation.
- Web tests must run through `corepack pnpm --filter ./apps/web run test`; do not use `npx vitest run`.
- Before claiming feature completion, use `$superteam-completion-check` and run a real current-code smoke through Web/API; do not present unit/component/build success as end-to-end availability.

---

## File Structure

- `apps/control-plane/internal/authz/types.go`: define `ActionEmployeeDelete = "employee.delete"`.
- `apps/control-plane/internal/authz/authorizer.go`: route `employee.delete` to tenant admin access against `ResourceEmployee`.
- `apps/control-plane/internal/authz/openfga_mapping.go`: include `employee.delete` in admin relation mapping for shadow/OpenFGA parity.
- `apps/control-plane/internal/authz/authorizer_test.go`: cover tenant admin allow, member deny, invalid resource, and employee owner deny.
- `apps/control-plane/internal/storage/queries/employee_execution.sql`: add lock/query/cascade SQL for the business delete path, including project affinity cleanup, and replace the existing single-table `DeleteDigitalEmployee` query with a safer `SoftDeleteDigitalEmployeeForDelete` query.
- `apps/control-plane/internal/storage/queries/*.sql.go`, `querier.go`, `models.go`: regenerated sqlc output.
- `apps/control-plane/internal/employee/types.go`: request, blocker, cascade result, audit cleanup structs, and typed blocked error.
- `apps/control-plane/internal/employee/repository.go`: repository methods for deletion lock, blockers, cascade, and audit.
- `apps/control-plane/internal/employee/pg_repository.go`: repository implementations and JSON audit details mapping.
- `apps/control-plane/internal/employee/service.go`: `Service.DeleteDigitalEmployee` transaction.
- `apps/control-plane/internal/employee/service_test.go`: service rollback/success tests using `memoryRepository` snapshots.
- `apps/control-plane/internal/employee/handler.go`: `HandlerService.DeleteDigitalEmployee`, HTTP handler, allowed action injection, JSON blocker error response.
- `apps/control-plane/internal/api/server.go`: register `DELETE /digital-employees/{employeeId}`.
- `apps/control-plane/internal/api/employee_routes_test.go`: route/auth/error response tests and route fake service updates.
- `contracts/control-plane/openapi.yaml`: document delete endpoint and blocker response schema.
- `apps/web/src/lib/api/client.ts`: retain parsed error JSON payload on `ApiRequestError`.
- `apps/web/src/lib/api/client.test.ts`: cover JSON error payload parsing.
- `apps/web/src/lib/api/employees.ts`: deletion API function and blocker response types; `DigitalEmployee.allowed_actions`.
- `apps/web/src/lib/api/employees.test.ts`: deletion request and blocker error tests.
- `apps/web/src/features/employees/components/employee-detail-header.tsx`: destructive delete action slot.
- `apps/web/src/features/employees/components/employee-detail-header.test.tsx`: delete button visibility and callback tests.
- `apps/web/src/features/employees/detail.tsx`: confirm dialog, mutation, cache invalidation, blocker rendering, redirect.
- `apps/web/src/features/employees/detail.test.tsx`: success and 409 blocker UI tests.

---

### Task 1: Authorization Contract for `employee.delete`

**Files:**
- Modify: `apps/control-plane/internal/authz/types.go`
- Modify: `apps/control-plane/internal/authz/authorizer.go`
- Modify: `apps/control-plane/internal/authz/openfga_mapping.go`
- Test: `apps/control-plane/internal/authz/authorizer_test.go`

**Interfaces:**
- Produces: `authz.ActionEmployeeDelete` constant with value `"employee.delete"`.
- Produces: `DBAuthorizer.Check` allows `employee.delete` only for tenant owner/admin, with `ResourceRef{Type: authz.ResourceEmployee, ID: employeeID}`.
- Consumed by later tasks: HTTP route authorization and Web `allowed_actions` checks use the same string.

- [ ] **Step 1: Write the failing authz test**

In `apps/control-plane/internal/authz/authorizer_test.go`, extend `TestDBAuthorizerEmployeeActionsUseBusinessActionSurface` table with these cases, immediately after the existing status update case:

```go
{name: "admin deletes employee", action: ActionEmployeeDelete, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleAdmin, allowed: true, matchedRule: "tenant.admin", resourceType: ResourceEmployee, resourceID: employeeID.String()},
{name: "owner deletes employee", action: ActionEmployeeDelete, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleOwner, allowed: true, matchedRule: "tenant.owner", resourceType: ResourceEmployee, resourceID: employeeID.String()},
{name: "member cannot delete employee", action: ActionEmployeeDelete, resource: ResourceRef{Type: ResourceEmployee, ID: employeeID.String()}, tenantRole: RoleMember, denyReason: ReasonNoMembership, resourceType: ResourceEmployee, resourceID: employeeID.String()},
```

Then extend `TestDBAuthorizerEmployeeOwnerCanUsePersonalEmployeeActions` after the existing `ActionEmployeeStatusUpdate` denial block:

```go

decision, err = authorizer.Check(context.Background(), CheckRequest{
	Actor:    ActorRef{Type: ActorUser, ID: ownerID.String()},
	Action:   ActionEmployeeDelete,
	Resource: ownerResource,
	TenantID: tenantID,
})
if err != nil {
	t.Fatalf("expected no error, got %v", err)
}
if decision.Allowed {
	t.Fatalf("expected employee owner not to delete through personal owner rule, got %#v", decision)
}
```

- [ ] **Step 2: Run authz tests and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/authz -run 'TestDBAuthorizerEmployee(ActionsUseBusinessActionSurface|OwnerCanUsePersonalEmployeeActions)' -count=1
```

Expected: FAIL because `ActionEmployeeDelete` is undefined.

- [ ] **Step 3: Add the action constant**

In `apps/control-plane/internal/authz/types.go`, add the constant next to other employee actions:

```go
	ActionEmployeeDelete         = "employee.delete"
```

Keep it near `ActionEmployeeStatusUpdate` so the destructive management action is visible in the employee action group.

- [ ] **Step 4: Wire DB authorizer**

In `apps/control-plane/internal/authz/authorizer.go`, include `ActionEmployeeDelete` in the tenant-admin-only employee action case:

```go
	case ActionEmployeeStatusUpdate,
		ActionEmployeeDelete,
		ActionEmployeeExecutionBind,
		ActionEmployeeRunCreate,
		ActionEmployeeRunStop,
		ActionEmployeeRunLogRead:
		if !validUUIDResource(req.Resource, ResourceEmployee) {
			decision = deny(ReasonInvalidResource)
			break
		}
		decision, err = a.checkTenantAdminAccess(ctx, req)
```

- [ ] **Step 5: Wire OpenFGA relation mapping**

In `apps/control-plane/internal/authz/openfga_mapping.go`, add `ActionEmployeeDelete` to the admin relation case:

```go
	case ActionRuntimeScopeManage, ActionAuthzCenterRead, ActionUserProjectTeamScopeRead, ActionUserProjectTeamScopeManage, ActionEmployeeCreate, ActionEmployeeDelete, ActionTeamCreate,
```

`openFGAObjectForRequest` currently does not map `ResourceEmployee`; do not invent a new object mapping unless an existing OpenFGA test fails. This mapping is still useful for relation classification and shadow parity where the object is tenant-scoped.

- [ ] **Step 6: Run authz tests and verify pass**

Run:

```bash
go test ./apps/control-plane/internal/authz -run 'TestDBAuthorizerEmployee(ActionsUseBusinessActionSurface|OwnerCanUsePersonalEmployeeActions)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/authz/types.go apps/control-plane/internal/authz/authorizer.go apps/control-plane/internal/authz/openfga_mapping.go apps/control-plane/internal/authz/authorizer_test.go
git commit -m "feat(authz): add digital employee delete action"
```

---

### Task 2: SQL and Repository Deletion Primitives

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Generated: `apps/control-plane/internal/storage/queries/employee_execution.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`
- Generated: `apps/control-plane/internal/storage/queries/models.go` when `make -C apps/control-plane generate-sqlc` updates shared model types
- Modify: `apps/control-plane/internal/employee/types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`
- Modify: `apps/control-plane/internal/employee/pg_repository.go`
- Test: `apps/control-plane/internal/employee/pg_repository_test.go`

**Interfaces:**
- Produces: `Repository.GetDigitalEmployeeForDelete(ctx, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)`.
- Produces: `Repository.ListDigitalEmployeeDeleteBlockers(ctx, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error)`.
- Produces: `Repository.SoftDeleteDigitalEmployeeCascade(ctx, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error)`.
- Produces: `Repository.CreateDigitalEmployeeDeleteAuditEvent(ctx, params DigitalEmployeeDeleteAuditEventParams) error`.
- Consumes later: `Service.DeleteDigitalEmployee` will call these methods inside `WithTransaction`.

- [ ] **Step 1: Add domain types before writing implementation**

In `apps/control-plane/internal/employee/types.go`, after the existing error var block, add:

```go
const DigitalEmployeeDeleteBlockedCode = "digital_employee_delete_blocked"

var ErrDigitalEmployeeDeleteBlocked = errors.New("digital employee delete blocked")

type DigitalEmployeeDeleteBlockerType string

const (
	DigitalEmployeeDeleteBlockerTypeRun         DigitalEmployeeDeleteBlockerType = "run"
	DigitalEmployeeDeleteBlockerTypeProjectTask DigitalEmployeeDeleteBlockerType = "project_task"
)

type DigitalEmployeeDeleteBlocker struct {
	Type      DigitalEmployeeDeleteBlockerType
	ID        uuid.UUID
	Status    string
	Title     string
	RunID     *uuid.UUID
	ProjectID *uuid.UUID
}

type DigitalEmployeeDeleteBlockedError struct {
	Blockers []DigitalEmployeeDeleteBlocker
}

func (e *DigitalEmployeeDeleteBlockedError) Error() string {
	return ErrDigitalEmployeeDeleteBlocked.Error()
}

func (e *DigitalEmployeeDeleteBlockedError) Unwrap() error {
	return ErrDigitalEmployeeDeleteBlocked
}

type SoftDeleteDigitalEmployeeCascadeParams struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	DeletedAt         time.Time
}

type DigitalEmployeeDeleteCascadeResult struct {
	ExecutionInstances     int64
	EnvironmentVariables   int64
	MCPBindings            int64
	MCPBindingsV2          int64
	SkillBindings          int64
	ConfigRevisions        int64
	WorkspaceFiles         int64
	ProjectAffinities      int64
	WorkspaceFileIDs       []uuid.UUID
	MCPBindingIDs          []uuid.UUID
	MCPBindingV2IDs        []uuid.UUID
	SkillBindingIDs        []uuid.UUID
	ExecutionInstanceID    *uuid.UUID
	RuntimeNodeID          *uuid.UUID
	AgentHomeDir           string
	ProviderType           string
}

type DigitalEmployeeDeleteAuditEventParams struct {
	TenantID      uuid.UUID
	ActorUserID   uuid.UUID
	Employee      DigitalEmployeeRecord
	CascadeResult DigitalEmployeeDeleteCascadeResult
	DeletedAt     time.Time
}
```

- [ ] **Step 2: Extend `Repository` interface**

In `apps/control-plane/internal/employee/repository.go`, add these methods to `type Repository interface` after `GetDigitalEmployee`:

```go
	GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error)
	ListDigitalEmployeeDeleteBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error)
	SoftDeleteDigitalEmployeeCascade(ctx context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error)
	CreateDigitalEmployeeDeleteAuditEvent(ctx context.Context, params DigitalEmployeeDeleteAuditEventParams) error
```

- [ ] **Step 3: Replace/add SQL queries**

In `apps/control-plane/internal/storage/queries/employee_execution.sql`, replace the existing `-- name: DeleteDigitalEmployee :exec` query with the following queries near `GetDigitalEmployee`/`UpdateDigitalEmployeeStatus`:

```sql
-- name: GetDigitalEmployeeForDelete :one
SELECT *
FROM digital_employees
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
FOR UPDATE;

-- name: ListDigitalEmployeeDeleteRunBlockers :many
SELECT
    'run'::text AS blocker_type,
    tr.id,
    tr.status,
    COALESCE(t.title, tr.task_id::text) AS title,
    tr.id AS run_id,
    pt.project_id
FROM task_runs tr
LEFT JOIN tasks t
  ON t.tenant_id = tr.tenant_id
 AND t.id = tr.task_id
LEFT JOIN project_tasks pt
  ON pt.tenant_id = tr.tenant_id
 AND pt.digital_employee_run_id = tr.id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
ORDER BY tr.updated_at DESC, tr.created_at DESC
LIMIT 20;

-- name: ListDigitalEmployeeDeleteProjectTaskBlockers :many
SELECT
    'project_task'::text AS blocker_type,
    pt.id,
    pt.status,
    COALESCE(NULLIF(pt.title, ''), pt.id::text) AS title,
    NULL::uuid AS run_id,
    pt.project_id
FROM project_tasks pt
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.assigned_digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND pt.status IN ('queued', 'running', 'in_progress')
ORDER BY pt.updated_at DESC, pt.created_at DESC
LIMIT 20;

-- name: SoftDeleteDigitalEmployeeForDelete :one
UPDATE digital_employees
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDigitalEmployeeExecutionInstancesForDelete :many
UPDATE digital_employee_execution_instances
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id, runtime_node_id, provider_type, agent_home_dir;

-- name: SoftDeleteDigitalEmployeeEnvironmentVariablesForDelete :many
UPDATE digital_employee_environment_variables
SET status = 'disabled',
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: SoftDeleteDigitalEmployeeMCPBindingsForDelete :many
UPDATE digital_employee_mcp_bindings
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: SoftDeleteDigitalEmployeeMCPBindingsV2ForDelete :many
UPDATE digital_employee_mcp_bindings_v2
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: DisableSkillAgentBindingsForDelete :many
UPDATE skill_agent_bindings
SET status = 'disabled',
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'enabled'
RETURNING id;

-- name: ArchiveDigitalEmployeeConfigRevisionsForDelete :many
UPDATE digital_employee_config_revisions
SET status = 'archived',
    archived_at = COALESCE(archived_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND archived_at IS NULL
RETURNING id;

-- name: SoftDeleteDigitalEmployeeWorkspaceFilesForDelete :many
UPDATE digital_employee_workspace_files
SET status = 'deleted',
    archived_at = COALESCE(archived_at, sqlc.arg('deleted_at')::timestamptz),
    deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: DeleteProjectEmployeeNodeAffinitiesForEmployeeDelete :many
DELETE FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
RETURNING id;
```

Keep this compatibility query at the end of the same file because `apps/control-plane/internal/employee/pg_run_repository.go` uses `queries.DeleteDigitalEmployee` for provisioning-abort cleanup. This compatibility query is not the business delete path and must not write audit events:

```sql
-- name: DeleteDigitalEmployee :exec
UPDATE digital_employees
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid;
```

- [ ] **Step 4: Run sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: sqlc generation completes. Generated code includes all query methods listed in Step 3.

- [ ] **Step 5: Implement repository methods**

In `apps/control-plane/internal/employee/pg_repository.go`, add helpers near `GetDigitalEmployee`:

```go
func (r *PgRepository) GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	employee, err := r.q.GetDigitalEmployeeForDelete(ctx, queries.GetDigitalEmployeeForDeleteParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) ListDigitalEmployeeDeleteBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error) {
	runRows, err := r.q.ListDigitalEmployeeDeleteRunBlockers(ctx, queries.ListDigitalEmployeeDeleteRunBlockersParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	projectRows, err := r.q.ListDigitalEmployeeDeleteProjectTaskBlockers(ctx, queries.ListDigitalEmployeeDeleteProjectTaskBlockersParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	blockers := make([]DigitalEmployeeDeleteBlocker, 0, len(runRows)+len(projectRows))
	for _, row := range runRows {
		blockers = append(blockers, DigitalEmployeeDeleteBlocker{
			Type:      DigitalEmployeeDeleteBlockerTypeRun,
			ID:        row.ID,
			Status:    row.Status,
			Title:     row.Title,
			RunID:     uuidPtr(row.RunID),
			ProjectID: uuidPtrFromNull(row.ProjectID),
		})
	}
	for _, row := range projectRows {
		blockers = append(blockers, DigitalEmployeeDeleteBlocker{
			Type:      DigitalEmployeeDeleteBlockerTypeProjectTask,
			ID:        row.ID,
			Status:    row.Status,
			Title:     row.Title,
			RunID:     uuidPtrFromNull(row.RunID),
			ProjectID: uuidPtrFromNull(row.ProjectID),
		})
	}
	return blockers, nil
}
```

Under this repo's sqlc configuration, nullable UUID columns are generated as `uuid.NullUUID`. Add these helpers in `pg_repository.go`:

```go
func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	v := value.UUID
	return &v
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	v := value
	return &v
}
```

Then add cascade/audit methods:

```go
func (r *PgRepository) SoftDeleteDigitalEmployeeCascade(ctx context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error) {
	deletedAt := pgtype.Timestamptz{Time: params.DeletedAt, Valid: true}
	cascade := DigitalEmployeeDeleteCascadeResult{}

	instances, err := r.q.SoftDeleteDigitalEmployeeExecutionInstancesForDelete(ctx, queries.SoftDeleteDigitalEmployeeExecutionInstancesForDeleteParams{
		TenantID:          params.TenantID,
		DigitalEmployeeID: params.DigitalEmployeeID,
		DeletedAt:         deletedAt,
	})
	if err != nil {
		return cascade, err
	}
	cascade.ExecutionInstances = int64(len(instances))
	if len(instances) > 0 {
		first := instances[0]
		cascade.ExecutionInstanceID = uuidPtr(first.ID)
		cascade.RuntimeNodeID = uuidPtr(first.RuntimeNodeID)
		cascade.ProviderType = first.ProviderType
		cascade.AgentHomeDir = first.AgentHomeDir
	}

	envRows, err := r.q.SoftDeleteDigitalEmployeeEnvironmentVariablesForDelete(ctx, queries.SoftDeleteDigitalEmployeeEnvironmentVariablesForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.EnvironmentVariables = int64(len(envRows))

	mcpRows, err := r.q.SoftDeleteDigitalEmployeeMCPBindingsForDelete(ctx, queries.SoftDeleteDigitalEmployeeMCPBindingsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.MCPBindings = int64(len(mcpRows))
	cascade.MCPBindingIDs = appendUUIDs(cascade.MCPBindingIDs, mcpRows)

	mcpV2Rows, err := r.q.SoftDeleteDigitalEmployeeMCPBindingsV2ForDelete(ctx, queries.SoftDeleteDigitalEmployeeMCPBindingsV2ForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.MCPBindingsV2 = int64(len(mcpV2Rows))
	cascade.MCPBindingV2IDs = appendUUIDs(cascade.MCPBindingV2IDs, mcpV2Rows)

	skillRows, err := r.q.DisableSkillAgentBindingsForDelete(ctx, queries.DisableSkillAgentBindingsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.SkillBindings = int64(len(skillRows))
	cascade.SkillBindingIDs = appendUUIDs(cascade.SkillBindingIDs, skillRows)

	configRows, err := r.q.ArchiveDigitalEmployeeConfigRevisionsForDelete(ctx, queries.ArchiveDigitalEmployeeConfigRevisionsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.ConfigRevisions = int64(len(configRows))

	workspaceRows, err := r.q.SoftDeleteDigitalEmployeeWorkspaceFilesForDelete(ctx, queries.SoftDeleteDigitalEmployeeWorkspaceFilesForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil { return cascade, err }
	cascade.WorkspaceFiles = int64(len(workspaceRows))
	cascade.WorkspaceFileIDs = appendUUIDs(cascade.WorkspaceFileIDs, workspaceRows)

	affinityRows, err := r.q.DeleteProjectEmployeeNodeAffinitiesForEmployeeDelete(ctx, queries.DeleteProjectEmployeeNodeAffinitiesForEmployeeDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID})
	if err != nil { return cascade, err }
	cascade.ProjectAffinities = int64(len(affinityRows))

	_, err = r.q.SoftDeleteDigitalEmployeeForDelete(ctx, queries.SoftDeleteDigitalEmployeeForDeleteParams{
		ID:        params.DigitalEmployeeID,
		TenantID:  params.TenantID,
		DeletedAt: deletedAt,
	})
	if err != nil {
		return cascade, mapNoRows(err)
	}
	return cascade, nil
}

func (r *PgRepository) CreateDigitalEmployeeDeleteAuditEvent(ctx context.Context, params DigitalEmployeeDeleteAuditEventParams) error {
	details, err := json.Marshal(digitalEmployeeDeleteAuditDetails(params))
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "digital_employee_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "digital_employee", Valid: true},
		ResourceID:   pgtype.Text{String: params.Employee.ID.String(), Valid: true},
		Action:       "digital_employee.delete",
		Details:      details,
	})
	return err
}
```

Use this helper for the `RETURNING id` query results, which generate `[]uuid.UUID`:

```go
func appendUUIDs(dst []uuid.UUID, src []uuid.UUID) []uuid.UUID {
	return append(dst, src...)
}
```

Add audit detail helper:

```go
func digitalEmployeeDeleteAuditDetails(params DigitalEmployeeDeleteAuditEventParams) map[string]any {
	teamID := ""
	if params.Employee.TeamID != nil {
		teamID = params.Employee.TeamID.String()
	}
	executionInstanceID := ""
	if params.CascadeResult.ExecutionInstanceID != nil {
		executionInstanceID = params.CascadeResult.ExecutionInstanceID.String()
	}
	runtimeNodeID := ""
	if params.CascadeResult.RuntimeNodeID != nil {
		runtimeNodeID = params.CascadeResult.RuntimeNodeID.String()
	}
	return map[string]any{
		"digital_employee_id": params.Employee.ID.String(),
		"name":                params.Employee.Name,
		"team_id":             teamID,
		"provider_type":       coalesceString(params.CascadeResult.ProviderType, params.Employee.ProviderType),
		"runtime_node_id":     runtimeNodeID,
		"execution_instance_id": executionInstanceID,
		"agent_home_dir":      params.CascadeResult.AgentHomeDir,
		"cascade_counts": map[string]any{
			"execution_instances":   params.CascadeResult.ExecutionInstances,
			"environment_variables": params.CascadeResult.EnvironmentVariables,
			"mcp_bindings":          params.CascadeResult.MCPBindings,
			"mcp_bindings_v2":       params.CascadeResult.MCPBindingsV2,
			"skill_bindings":        params.CascadeResult.SkillBindings,
			"config_revisions":      params.CascadeResult.ConfigRevisions,
			"workspace_files":       params.CascadeResult.WorkspaceFiles,
			"project_affinities":    params.CascadeResult.ProjectAffinities,
		},
		"cleanup_candidates": map[string]any{
			"agent_home_dir":     params.CascadeResult.AgentHomeDir,
			"workspace_file_ids": uuidStrings(params.CascadeResult.WorkspaceFileIDs),
			"mcp_binding_ids":    uuidStrings(append(params.CascadeResult.MCPBindingIDs, params.CascadeResult.MCPBindingV2IDs...)),
			"skill_binding_ids":  uuidStrings(params.CascadeResult.SkillBindingIDs),
		},
		"deleted_at": params.DeletedAt.UTC().Format(time.RFC3339Nano),
	}
}
```

Add these string helpers:

```go
func coalesceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uuidStrings(values []uuid.UUID) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			out = append(out, value.String())
		}
	}
	return out
}
```

- [ ] **Step 6: Compile and verify run repository compatibility**

Run:

```bash
go test ./apps/control-plane/internal/employee -run '^$' -count=1
```

Expected: compile success. The presence of `queries.DeleteDigitalEmployee` confirms the compatibility query from Step 3 stayed available for provisioning aborts. Do not route provisioning aborts through `Service.DeleteDigitalEmployee`, because abort cleanup must not create a user delete audit event.

- [ ] **Step 7: Add DB-backed repository tests**

Add the tests to `apps/control-plane/internal/employee/pg_repository_test.go`. Reuse that file's existing DB fixture and migration setup; do not create a second database harness.

Add two tests:

```go
func TestPgRepositoryListDigitalEmployeeDeleteBlockers(t *testing.T) {
	ctx, repo, q, tenantID := newEmployeeRepositoryDBFixture(t)
	employeeID := seedDigitalEmployee(t, ctx, q, tenantID, "删除阻断员工")
	seedTaskRun(t, ctx, q, tenantID, employeeID, "running", "运行中的任务")
	seedProjectTask(t, ctx, q, tenantID, employeeID, "queued", "排队项目任务")
	seedTaskRun(t, ctx, q, tenantID, employeeID, "completed", "已完成任务")
	seedProjectTask(t, ctx, q, tenantID, employeeID, "completed", "已完成项目任务")

	blockers, err := repo.ListDigitalEmployeeDeleteBlockers(ctx, tenantID, employeeID)
	require.NoError(t, err)
	require.Len(t, blockers, 2)
	require.ElementsMatch(t, []DigitalEmployeeDeleteBlockerType{DigitalEmployeeDeleteBlockerTypeRun, DigitalEmployeeDeleteBlockerTypeProjectTask}, []DigitalEmployeeDeleteBlockerType{blockers[0].Type, blockers[1].Type})
}

func TestPgRepositorySoftDeleteDigitalEmployeeCascadeAndAudit(t *testing.T) {
	ctx, repo, q, tenantID := newEmployeeRepositoryDBFixture(t)
	employeeID := seedDigitalEmployeeWithProjectionRows(t, ctx, q, tenantID)
	actorID := seedAuthUser(t, ctx, q, tenantID)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)

	employee, err := repo.GetDigitalEmployeeForDelete(ctx, tenantID, employeeID)
	require.NoError(t, err)
	cascade, err := repo.SoftDeleteDigitalEmployeeCascade(ctx, SoftDeleteDigitalEmployeeCascadeParams{TenantID: tenantID, DigitalEmployeeID: employeeID, DeletedAt: deletedAt})
	require.NoError(t, err)
	require.Equal(t, int64(1), cascade.ExecutionInstances)
	require.Equal(t, int64(1), cascade.EnvironmentVariables)
	require.Equal(t, int64(1), cascade.WorkspaceFiles)
	require.Equal(t, int64(1), cascade.SkillBindings)
	require.Equal(t, int64(1), cascade.ProjectAffinities)
	require.NotEmpty(t, cascade.AgentHomeDir)

	err = repo.CreateDigitalEmployeeDeleteAuditEvent(ctx, DigitalEmployeeDeleteAuditEventParams{TenantID: tenantID, ActorUserID: actorID, Employee: employee, CascadeResult: cascade, DeletedAt: deletedAt})
	require.NoError(t, err)

	_, err = repo.GetDigitalEmployee(ctx, tenantID, employeeID)
	require.ErrorIs(t, err, ErrNotFound)
	assertLatestAuditHasNoSecrets(t, ctx, q, tenantID, employeeID)
}
```

Use real insert helpers for required foreign keys. Keep helper names concrete in the implementation and assert these row effects:

```sql
digital_employees.deleted_at IS NOT NULL AND status = 'disabled'
digital_employee_execution_instances.deleted_at IS NOT NULL AND status = 'disabled'
digital_employee_environment_variables.deleted_at IS NOT NULL AND status = 'disabled'
digital_employee_workspace_files.deleted_at IS NOT NULL AND status = 'deleted'
skill_agent_bindings.status = 'disabled'
digital_employee_config_revisions.status = 'archived' AND archived_at IS NOT NULL
project_employee_node_affinity has no row for the deleted employee
```

- [ ] **Step 8: Run repository tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestPgRepository(ListDigitalEmployeeDeleteBlockers|SoftDeleteDigitalEmployeeCascadeAndAudit)' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/control-plane/internal/storage/queries apps/control-plane/internal/employee/types.go apps/control-plane/internal/employee/repository.go apps/control-plane/internal/employee/pg_repository.go apps/control-plane/internal/employee/pg_repository_test.go
git commit -m "feat(employee): add delete repository primitives"
```

---

### Task 3: Service Transaction and Rollback Semantics

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/service_test.go`

**Interfaces:**
- Produces: `Service.DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error`.
- Consumes: repository methods from Task 2.
- Produces for handler: typed `*DigitalEmployeeDeleteBlockedError` preserving blockers.

- [ ] **Step 1: Add request type**

In `apps/control-plane/internal/employee/types.go`, near other request structs, add:

```go
type DeleteDigitalEmployeeRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	ActorUserID       uuid.UUID
}
```

- [ ] **Step 2: Write service rollback tests**

In `apps/control-plane/internal/employee/service_test.go`, add tests before `type memoryRepository struct`:

```go
func TestServiceDeleteDigitalEmployeeBlocksActiveWorkAndRollsBack(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	employeeID := uuid.New()
	actorID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{ID: employeeID, TenantID: tenantID, OwnerUserID: actorID, EmployeeType: "devops_engineer", ProviderType: "codex", Name: "阻断员工", Role: "devops", Status: DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now}
	repo.deleteBlockers = []DigitalEmployeeDeleteBlocker{{Type: DigitalEmployeeDeleteBlockerTypeRun, ID: uuid.New(), Status: "running", Title: "运行中的任务"}}

	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorID})
	require.ErrorIs(t, err, ErrDigitalEmployeeDeleteBlocked)
	var blocked *DigitalEmployeeDeleteBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Len(t, blocked.Blockers, 1)
	require.Nil(t, repo.employees[employeeID].DeletedAt)
	require.Equal(t, 0, repo.deleteCascadeCount)
	require.Len(t, repo.deleteAuditEvents, 0)
	require.Equal(t, 0, repo.transactionCommitCount)
}

func TestServiceDeleteDigitalEmployeeSoftDeletesCascadeAndAudits(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	actorID := uuid.New()
	executionInstanceID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{ID: employeeID, TenantID: tenantID, TeamID: &teamID, OwnerUserID: actorID, EmployeeType: "devops_engineer", ProviderType: "codex", Name: "可删除员工", Role: "devops", Status: DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now}
	repo.deleteCascadeResult = DigitalEmployeeDeleteCascadeResult{ExecutionInstances: 1, EnvironmentVariables: 2, MCPBindings: 1, MCPBindingsV2: 1, SkillBindings: 1, ConfigRevisions: 1, WorkspaceFiles: 1, ProjectAffinities: 1, ExecutionInstanceID: &executionInstanceID, RuntimeNodeID: &runtimeNodeID, AgentHomeDir: "/srv/superteam/agents/emp", ProviderType: "codex", WorkspaceFileIDs: []uuid.UUID{uuid.New()}}

	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorID})
	require.NoError(t, err)
	require.NotNil(t, repo.employees[employeeID].DeletedAt)
	require.Equal(t, DigitalEmployeeStatusDisabled, repo.employees[employeeID].Status)
	require.Equal(t, 1, repo.deleteCascadeCount)
	require.Len(t, repo.deleteAuditEvents, 1)
	require.Equal(t, actorID, repo.deleteAuditEvents[0].ActorUserID)
	require.Equal(t, employeeID, repo.deleteAuditEvents[0].Employee.ID)
	require.Equal(t, int64(1), repo.deleteAuditEvents[0].CascadeResult.ExecutionInstances)
	require.Equal(t, 1, repo.transactionCommitCount)
}

func TestServiceDeleteDigitalEmployeeValidatesRequiredIDs(t *testing.T) {
	svc, err := NewService(newMemoryRepository())
	require.NoError(t, err)
	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{})
	require.ErrorIs(t, err, ErrInvalidInput)
}
```

- [ ] **Step 3: Run service tests and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestServiceDeleteDigitalEmployee' -count=1
```

Expected: FAIL because `DeleteDigitalEmployee` and memory repository delete methods are not implemented.

- [ ] **Step 4: Implement `Service.DeleteDigitalEmployee`**

In `apps/control-plane/internal/employee/service.go`, add after `GetDigitalEmployee` or near status management:

```go
func (s *Service) DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.ActorUserID == uuid.Nil {
		return fmt.Errorf("%w: actor_user_id is required", ErrInvalidInput)
	}
	deletedAt := time.Now().UTC()
	return s.repository.WithTransaction(ctx, func(repository Repository) error {
		employee, err := repository.GetDigitalEmployeeForDelete(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return err
		}
		blockers, err := repository.ListDigitalEmployeeDeleteBlockers(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			return &DigitalEmployeeDeleteBlockedError{Blockers: append([]DigitalEmployeeDeleteBlocker(nil), blockers...)}
		}
		cascade, err := repository.SoftDeleteDigitalEmployeeCascade(ctx, SoftDeleteDigitalEmployeeCascadeParams{
			TenantID:          req.TenantID,
			DigitalEmployeeID: req.DigitalEmployeeID,
			DeletedAt:         deletedAt,
		})
		if err != nil {
			return err
		}
		return repository.CreateDigitalEmployeeDeleteAuditEvent(ctx, DigitalEmployeeDeleteAuditEventParams{
			TenantID:      req.TenantID,
			ActorUserID:   req.ActorUserID,
			Employee:      employee,
			CascadeResult: cascade,
			DeletedAt:     deletedAt,
		})
	})
}
```

- [ ] **Step 5: Extend `memoryRepository`**

In `apps/control-plane/internal/employee/service_test.go`, add fields to `memoryRepository`:

```go
	deleteBlockers      []DigitalEmployeeDeleteBlocker
	deleteCascadeResult DigitalEmployeeDeleteCascadeResult
	deleteCascadeCount  int
	deleteAuditEvents   []DigitalEmployeeDeleteAuditEventParams
```

Add to `memoryRepositorySnapshot`:

```go
	deleteCascadeCount int
	deleteAuditEvents  []DigitalEmployeeDeleteAuditEventParams
```

Update snapshot/restore functions to copy those fields so rollback assertions work.

Add methods:

```go
func (r *memoryRepository) GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	return r.GetDigitalEmployee(ctx, tenantID, employeeID)
}

func (r *memoryRepository) ListDigitalEmployeeDeleteBlockers(_ context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error) {
	return append([]DigitalEmployeeDeleteBlocker(nil), r.deleteBlockers...), nil
}

func (r *memoryRepository) SoftDeleteDigitalEmployeeCascade(_ context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error) {
	employee, ok := r.employees[params.DigitalEmployeeID]
	if !ok || employee.TenantID != params.TenantID || employee.DeletedAt != nil {
		return DigitalEmployeeDeleteCascadeResult{}, ErrNotFound
	}
	employee.Status = DigitalEmployeeStatusDisabled
	employee.DisabledAt = &params.DeletedAt
	employee.DeletedAt = &params.DeletedAt
	employee.UpdatedAt = params.DeletedAt
	r.employees[params.DigitalEmployeeID] = employee
	r.deleteCascadeCount++
	return r.deleteCascadeResult, nil
}

func (r *memoryRepository) CreateDigitalEmployeeDeleteAuditEvent(_ context.Context, params DigitalEmployeeDeleteAuditEventParams) error {
	r.deleteAuditEvents = append(r.deleteAuditEvents, params)
	return nil
}
```

- [ ] **Step 6: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestServiceDeleteDigitalEmployee' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/employee/service.go apps/control-plane/internal/employee/service_test.go apps/control-plane/internal/employee/types.go
git commit -m "feat(employee): add delete service transaction"
```

---

### Task 4: HTTP Route, JSON 409, Allowed Actions, and OpenAPI

**Files:**
- Modify: `apps/control-plane/internal/employee/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

**Interfaces:**
- Produces: `DELETE /api/v1/digital-employees/{employeeId}` returns `204`, `404`, `403`, or structured `409`.
- Produces: detail response includes `allowed_actions`, with `employee.delete` only when current user can delete.
- Consumed by Web Task 6: `DigitalEmployee.allowed_actions` and delete blocker response.

- [ ] **Step 1: Add route tests first**

In `apps/control-plane/internal/api/employee_routes_test.go`, add `deleteReq employee.DeleteDigitalEmployeeRequest` and `deleteErr error` fields to `routeEmployeeService`.

Add these methods:

```go
func (s *routeEmployeeService) DeleteDigitalEmployee(ctx context.Context, req employee.DeleteDigitalEmployeeRequest) error {
	s.deleteReq = req
	return s.deleteErr
}
```

Add tests:

```go
func TestDeleteDigitalEmployeeRouteReturnsNoContent(t *testing.T) {
	server, service, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNoContent, resp.Code)
	require.Equal(t, employeeID, service.deleteReq.DigitalEmployeeID)
	require.Equal(t, platform.DefaultTenantID, service.deleteReq.TenantID)
	require.NotEqual(t, uuid.Nil, service.deleteReq.ActorUserID)
}

func TestDeleteDigitalEmployeeRouteReturnsBlockers(t *testing.T) {
	blockerID := uuid.New()
	projectID := uuid.New()
	blockedErr := &employee.DigitalEmployeeDeleteBlockedError{Blockers: []employee.DigitalEmployeeDeleteBlocker{{Type: employee.DigitalEmployeeDeleteBlockerTypeProjectTask, ID: blockerID, Status: "queued", Title: "项目任务 A", ProjectID: &projectID}}}
	server, _, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true}, func(s *routeEmployeeService) { s.deleteErr = blockedErr })
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	require.Equal(t, http.StatusConflict, resp.Code)
	var body struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		Blockers []struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			Status    string `json:"status"`
			Title     string `json:"title"`
			ProjectID string `json:"project_id"`
		} `json:"blockers"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, employee.DigitalEmployeeDeleteBlockedCode, body.Code)
	require.Contains(t, body.Message, "仍有排队或执行中的工作")
	require.Len(t, body.Blockers, 1)
	require.Equal(t, "project_task", body.Blockers[0].Type)
	require.Equal(t, "项目任务 A", body.Blockers[0].Title)
	require.Equal(t, projectID.String(), body.Blockers[0].ProjectID)
}
```

Create this helper in the same test file near the existing route test helpers:

```go
func newEmployeeRouteTestServer(t *testing.T, authorizer *routeAuthorizer, configure ...func(*routeEmployeeService)) (*Server, *routeEmployeeService, *http.Cookie) {
	t.Helper()
	authService, err := auth.NewService(newRouteAuthRepo())
	require.NoError(t, err)
	_, err = authService.CreateUser(context.Background(), "admin", "admin")
	require.NoError(t, err)
	service := &routeEmployeeService{}
	for _, fn := range configure { fn(service) }
	server := NewServerWithAuthz(handlers.NewTaskHandler(&routeTaskService{}), handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}), authService, nil, authorizer)
	server.SetEmployeeHandler(employee.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	return server, service, cookie
}
```

Also add delete to `TestEmployeeRoutesUseAuthzActions` table:

```go
{name: "delete", method: http.MethodDelete, path: "/api/v1/digital-employees/" + employeeID, action: authz.ActionEmployeeDelete, resourceType: authz.ResourceEmployee, resourceID: employeeID},
```

- [ ] **Step 2: Run route tests and verify failure**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'Test(DeleteDigitalEmployeeRoute|EmployeeRoutesUseAuthzActions)' -count=1
```

Expected: FAIL because route and service interface are missing.

- [ ] **Step 3: Extend handler service interface and response types**

In `apps/control-plane/internal/employee/handler.go`, add to `HandlerService`:

```go
	DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error
```

Add response types near `errorResponse`:

```go
type digitalEmployeeDeleteBlockedResponse struct {
	Code     string                                  `json:"code"`
	Message  string                                  `json:"message"`
	Blockers []digitalEmployeeDeleteBlockerResponse `json:"blockers"`
}

type digitalEmployeeDeleteBlockerResponse struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	RunID     string `json:"run_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}
```

- [ ] **Step 4: Implement delete handler and structured blocker writer**

In `apps/control-plane/internal/employee/handler.go`, add after `GetDigitalEmployee`:

```go
func (h *HTTPHandler) DeleteDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeDelete, &employeeID, "digital employee delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	actorUserID := middleware.GetUserID(r.Context())
	if err := service.DeleteDigitalEmployee(r.Context(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorUserID}); err != nil {
		writeDeleteDigitalEmployeeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDeleteDigitalEmployeeError(w http.ResponseWriter, err error) {
	var blocked *DigitalEmployeeDeleteBlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, digitalEmployeeDeleteBlockedResponse{
			Code:     DigitalEmployeeDeleteBlockedCode,
			Message:  "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
			Blockers: deleteBlockerResponses(blocked.Blockers),
		})
		return
	}
	writeHandlerError(w, err)
}

func deleteBlockerResponses(blockers []DigitalEmployeeDeleteBlocker) []digitalEmployeeDeleteBlockerResponse {
	responses := make([]digitalEmployeeDeleteBlockerResponse, 0, len(blockers))
	for _, blocker := range blockers {
		responses = append(responses, digitalEmployeeDeleteBlockerResponse{
			Type:      string(blocker.Type),
			ID:        blocker.ID.String(),
			Status:    blocker.Status,
			Title:     blocker.Title,
			RunID:     uuidStringValue(blocker.RunID),
			ProjectID: uuidStringValue(blocker.ProjectID),
		})
	}
	return responses
}

func uuidStringValue(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}
```

- [ ] **Step 5: Register route**

In `apps/control-plane/internal/api/server.go`, add immediately after `r.Get("/digital-employees/{employeeId}", ...)`:

```go
				r.Delete("/digital-employees/{employeeId}", s.employeeHandler.DeleteDigitalEmployee)
```

- [ ] **Step 6: Add allowed actions to employee detail response**

In `apps/control-plane/internal/employee/handler.go`, add field to `digitalEmployeeResponse`:

```go
	AllowedActions []string `json:"allowed_actions,omitempty"`
```

Change `GetDigitalEmployee` handler to compute allowed actions after successful service fetch:

```go
	response := employeeResponseFromDomain(employee)
	response.AllowedActions = h.allowedEmployeeActions(r.Context(), tenantID, employeeID)
	writeJSON(w, http.StatusOK, response)
```

Add helper:

```go
func (h *HTTPHandler) allowedEmployeeActions(ctx context.Context, tenantID, employeeID uuid.UUID) []string {
	if h.authorizer == nil {
		return nil
	}
	userID := middleware.GetUserID(ctx)
	if userID == uuid.Nil {
		return nil
	}
	actions := []string{authz.ActionEmployeeDelete}
	allowed := make([]string, 0, len(actions))
	for _, action := range actions {
		decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
			Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
			Action:   action,
			Resource: authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()},
			TenantID: tenantID,
		})
		if err == nil && decision.Allowed {
			allowed = append(allowed, action)
		}
	}
	return allowed
}
```

Route tests should assert `allowed_actions` on GET detail in a small new test:

```go
func TestGetDigitalEmployeeIncludesAllowedDeleteAction(t *testing.T) {
	server, _, cookie := newEmployeeRouteTestServer(t, &routeAuthorizer{allowed: true})
	employeeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/"+employeeID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	var body struct { AllowedActions []string `json:"allowed_actions"` }
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Contains(t, body.AllowedActions, authz.ActionEmployeeDelete)
}
```

- [ ] **Step 7: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, find the existing `/api/v1/digital-employees/{employeeId}` path object and add this `delete` operation beside the existing `get` operation:

```yaml
    delete:
      operationId: deleteDigitalEmployee
      summary: Delete a digital employee
      responses:
        '204':
          description: Digital employee deleted
        '404':
          description: Digital employee not found
        '409':
          description: Digital employee has active work and cannot be deleted
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/DigitalEmployeeDeleteBlockedError'
```

Add schemas:

```yaml
    DigitalEmployeeDeleteBlockedError:
      type: object
      required: [code, message, blockers]
      properties:
        code:
          type: string
          enum: [digital_employee_delete_blocked]
        message:
          type: string
        blockers:
          type: array
          items:
            $ref: '#/components/schemas/DigitalEmployeeDeleteBlocker'
    DigitalEmployeeDeleteBlocker:
      type: object
      required: [type, id, status, title]
      properties:
        type:
          type: string
          enum: [run, project_task]
        id:
          type: string
          format: uuid
        status:
          type: string
        title:
          type: string
        run_id:
          type: string
          format: uuid
        project_id:
          type: string
          format: uuid
```

Find the existing `DigitalEmployee` schema in `contracts/control-plane/openapi.yaml` and add `allowed_actions` to its properties:

```yaml
        allowed_actions:
          type: array
          items:
            type: string
```

- [ ] **Step 8: Generate/verify contract**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: both commands pass. Generated Go files may be ignored by git; still run this to prove contract generation.

- [ ] **Step 9: Run route tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run 'Test(DeleteDigitalEmployeeRoute|GetDigitalEmployeeIncludesAllowedDeleteAction|EmployeeRoutesUseAuthzActions)' -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add apps/control-plane/internal/employee/handler.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/employee_routes_test.go contracts/control-plane/openapi.yaml
git commit -m "feat(api): expose digital employee delete endpoint"
```

---

### Task 5: Web API Client Error Payload and Delete Function

**Files:**
- Modify: `apps/web/src/lib/api/client.ts`
- Test: `apps/web/src/lib/api/client.test.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
- Test: `apps/web/src/lib/api/employees.test.ts`

**Interfaces:**
- Produces: `ApiRequestError.payload?: unknown` carrying parsed JSON error bodies.
- Produces: `deleteDigitalEmployee(options, employeeId): Promise<void>`.
- Produces types: `DigitalEmployeeDeleteBlocker`, `DigitalEmployeeDeleteBlockedErrorResponse`.

- [ ] **Step 1: Add API client tests first**

Create `apps/web/src/lib/api/client.test.ts` with this content:

```ts
import { describe, expect, it } from "vitest";
import { ApiRequestError, parseJson } from "./client";

describe("ApiRequestError", () => {
  it("keeps parsed JSON error payload", async () => {
    const payload = {
      code: "digital_employee_delete_blocked",
      message: "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
      blockers: [{ type: "run", id: "run-1", status: "running", title: "运行中" }],
    };

    await expect(
      parseJson(new Response(JSON.stringify(payload), { status: 409, headers: { "content-type": "application/json" } }), "delete digital employee"),
    ).rejects.toMatchObject({
      status: 409,
      code: "digital_employee_delete_blocked",
      detail: payload.message,
      payload,
    });
  });
});
```

- [ ] **Step 2: Run client test and verify failure**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/client.test.ts
```

Expected: FAIL because `payload` is not present.

- [ ] **Step 3: Extend `ApiRequestError` and `readErrorDetail`**

In `apps/web/src/lib/api/client.ts`, change the class:

```ts
export class ApiRequestError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly detail?: string;
  readonly payload?: unknown;

  constructor(resource: string, status: number, detail?: string, code?: string, payload?: unknown) {
    super(`${resource} request failed with status ${status}${detail ? `: ${detail}` : ""}`);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.payload = payload;
  }
}
```

Change `parseJson` error throw:

```ts
throw new ApiRequestError(resource, response.status, errorDetail.detail, errorDetail.code, errorDetail.payload);
```

Change `ParsedErrorDetail`:

```ts
type ParsedErrorDetail = {
  detail?: string;
  code?: string;
  payload?: unknown;
};
```

Change the JSON branch of `readErrorDetail`:

```ts
const parsed = JSON.parse(body) as { code?: unknown; error?: unknown; message?: unknown };
const detail =
  typeof parsed.message === "string" && parsed.message
    ? parsed.message
    : typeof parsed.error === "string" && parsed.error
      ? parsed.error
      : body;
return {
  detail,
  code: typeof parsed.code === "string" && parsed.code ? parsed.code : undefined,
  payload: parsed,
};
```

- [ ] **Step 4: Add employee API delete tests**

In `apps/web/src/lib/api/employees.test.ts`, add import `deleteDigitalEmployee`.

Add tests near `getDigitalEmployee` tests:

```ts
it("deletes a digital employee with encoded employee id", async () => {
  const fetcher = vi.fn(async () => new Response(null, { status: 204 }));

  await expect(
    deleteDigitalEmployee({ baseUrl: "http://control-plane.local", fetcher }, "employee 1/primary"),
  ).resolves.toBeUndefined();

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary",
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "DELETE",
    },
  );
});

it("surfaces delete blockers from the API payload", async () => {
  const payload = {
    code: "digital_employee_delete_blocked",
    message: "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
    blockers: [{ type: "project_task", id: "task-1", status: "queued", title: "项目任务" }],
  };
  const fetcher = vi.fn(async () => new Response(JSON.stringify(payload), { status: 409, headers: { "content-type": "application/json" } }));

  await expect(
    deleteDigitalEmployee({ baseUrl: "http://control-plane.local", fetcher }, "employee-1"),
  ).rejects.toMatchObject({ status: 409, code: "digital_employee_delete_blocked", payload });
});
```

- [ ] **Step 5: Implement employee API types/function**

In `apps/web/src/lib/api/employees.ts`, add optional field to `DigitalEmployee`:

```ts
  allowed_actions?: string[];
```

Add types near other employee API types:

```ts
export type DigitalEmployeeDeleteBlocker = {
  type: "run" | "project_task";
  id: string;
  status: string;
  title: string;
  run_id?: string;
  project_id?: string;
};

export type DigitalEmployeeDeleteBlockedErrorResponse = {
  code: "digital_employee_delete_blocked";
  message: string;
  blockers: DigitalEmployeeDeleteBlocker[];
};
```

Add function after `getDigitalEmployee`:

```ts
export async function deleteDigitalEmployee(
  options: ApiClientOptions,
  employeeId: string,
): Promise<void> {
  return deleteJson(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}`,
    "delete digital employee",
  );
}
```

- [ ] **Step 6: Run API tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/client.test.ts src/lib/api/employees.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/lib/api/client.ts apps/web/src/lib/api/client.test.ts apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts
git commit -m "feat(web): support digital employee delete API errors"
```

---

### Task 6: Web Detail Delete UI and Blocker Feedback

**Files:**
- Modify: `apps/web/src/features/employees/components/employee-detail-header.tsx`
- Test: `apps/web/src/features/employees/components/employee-detail-header.test.tsx`
- Modify: `apps/web/src/features/employees/detail.tsx`
- Test: `apps/web/src/features/employees/detail.test.tsx`

**Interfaces:**
- Consumes: `DigitalEmployee.allowed_actions` from Task 4 and `deleteDigitalEmployee`/blocker payload from Task 5.
- Produces: details page destructive delete action, name confirmation, success redirect to `/employees`, cache invalidation, and structured blocker rendering.

- [ ] **Step 1: Update header tests first**

In `apps/web/src/features/employees/components/employee-detail-header.test.tsx`, update render props in existing test to pass no-op delete fields:

```tsx
<EmployeeDetailHeader
  canDelete={false}
  employee={employee}
  isDeleting={false}
  onDeleteEmployee={vi.fn()}
  onManageCapabilities={onManageCapabilities}
  onStartTask={onStartTask}
/>
```

Add a test:

```tsx
it("renders destructive delete action only when allowed", async () => {
  const onDeleteEmployee = vi.fn();
  const allowedScreen = await render(
    <EmployeeDetailHeader
      canDelete
      employee={employee}
      isDeleting={false}
      onDeleteEmployee={onDeleteEmployee}
      onManageCapabilities={vi.fn()}
      onStartTask={vi.fn()}
    />,
  );

  const deleteButton = allowedScreen.getByRole("button", { name: "删除数字员工" });
  await expect.element(deleteButton).toBeVisible();
  await deleteButton.click();
  expect(onDeleteEmployee).toHaveBeenCalledTimes(1);

  const deniedScreen = await render(
    <EmployeeDetailHeader
      canDelete={false}
      employee={employee}
      isDeleting={false}
      onDeleteEmployee={vi.fn()}
      onManageCapabilities={vi.fn()}
      onStartTask={vi.fn()}
    />,
  );
  await expect.element(deniedScreen.queryByRole("button", { name: "删除数字员工" })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Implement header delete button**

In `apps/web/src/features/employees/components/employee-detail-header.tsx`, import `Trash2` from `lucide-react`.

Extend props:

```ts
type EmployeeDetailHeaderProps = {
  employee: DigitalEmployee;
  onStartTask: () => void;
  onManageCapabilities: () => void;
  onDeleteEmployee: () => void;
  canDelete: boolean;
  isDeleting?: boolean;
};
```

Update function args and add button before `开始任务`:

```tsx
{canDelete ? (
  <V3Button disabled={isDeleting} onClick={onDeleteEmployee} type="button" variant="danger">
    <Trash2 className="size-4" />
    删除数字员工
  </V3Button>
) : null}
```

- [ ] **Step 3: Update detail tests for delete flow**

In `apps/web/src/features/employees/detail.test.tsx`, extend the TanStack Router mock with `useNavigate`:

```tsx
const navigateMock = vi.fn();
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, search, to }: { children: ReactNode; search?: Record<string, string | undefined>; to: string }) => { ... },
  useNavigate: () => navigateMock,
}));
```

Reset `navigateMock` before each test:

```ts
beforeEach(() => {
  navigateMock.mockReset();
});
```

Set the fixture employee to include `allowed_actions: ["employee.delete"]` for delete-enabled tests, or override per fetcher. Add `employeeOverride` and delete behavior to `createDetailFetcher`:

```ts
employeeOverride?: Record<string, unknown>;
deleteStatus?: number;
deletePayload?: Record<string, unknown>;
```

In the GET employee branch, return `{ ...employee, ...employeeOverride }`. Add DELETE branch:

```ts
if (path === `/api/v1/digital-employees/${employee.id}` && method === "DELETE") {
  if (deleteStatus && deleteStatus >= 400) {
    return jsonResponse(deletePayload ?? { error: "delete failed" }, deleteStatus);
  }
  return new Response(null, { status: 204 });
}
```

Add success test:

```tsx
it("confirms by employee name, deletes, invalidates caches and navigates to employees", async () => {
  const fetcher = createDetailFetcher({ run: runFixture({ status: "completed" }), employeeOverride: { allowed_actions: ["employee.delete"] } });
  const screen = await renderEmployeeDetail(fetcher);

  await userEvent.click(screen.getByRole("button", { name: "删除数字员工" }));
  await expect.element(screen.getByRole("heading", { name: "删除数字员工" })).toBeVisible();
  await expect.element(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();

  await userEvent.fill(screen.getByLabelText("输入员工名称确认"), "需求分析员工");
  await userEvent.click(screen.getByRole("button", { name: "确认删除" }));

  expect(fetchCallCount(fetcher, `/api/v1/digital-employees/${employee.id}`, "DELETE")).toBe(1);
  await expect.poll(() => navigateMock).toHaveBeenCalledWith({ to: "/employees" });
});
```

Add blocker test:

```tsx
it("renders delete blockers from conflict response", async () => {
  const fetcher = createDetailFetcher({
    run: runFixture({ status: "completed" }),
    employeeOverride: { allowed_actions: ["employee.delete"] },
    deleteStatus: 409,
    deletePayload: {
      code: "digital_employee_delete_blocked",
      message: "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
      blockers: [
        { type: "run", id: "run-1", status: "running", title: "线上排障运行", run_id: "run-1" },
        { type: "project_task", id: "task-1", status: "queued", title: "项目排队任务", project_id: "project-1" },
      ],
    },
  });
  const screen = await renderEmployeeDetail(fetcher);

  await userEvent.click(screen.getByRole("button", { name: "删除数字员工" }));
  await userEvent.fill(screen.getByLabelText("输入员工名称确认"), "需求分析员工");
  await userEvent.click(screen.getByRole("button", { name: "确认删除" }));

  await expect.element(screen.getByText("该数字员工仍有排队或执行中的工作，停止或完成后再删除。")).toBeVisible();
  await expect.element(screen.getByText("线上排障运行")).toBeVisible();
  await expect.element(screen.getByText("running")).toBeVisible();
  await expect.element(screen.getByText("项目排队任务")).toBeVisible();
  expect(navigateMock).not.toHaveBeenCalled();
});
```

- [ ] **Step 4: Run UI tests and verify failure**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/components/employee-detail-header.test.tsx src/features/employees/detail.test.tsx
```

Expected: FAIL because UI delete flow is not implemented.

- [ ] **Step 5: Implement detail delete state/mutation**

In `apps/web/src/features/employees/detail.tsx`, add imports:

```ts
import { useNavigate } from "@tanstack/react-router";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Input } from "@/components/ui/input";
import { deleteDigitalEmployee, type DigitalEmployeeDeleteBlockedErrorResponse } from "@/lib/api/employees";
```

Add state and navigate:

```ts
const navigate = useNavigate();
const [deleteOpen, setDeleteOpen] = useState(false);
const [deleteConfirmName, setDeleteConfirmName] = useState("");
const [deleteBlocked, setDeleteBlocked] = useState<DigitalEmployeeDeleteBlockedErrorResponse | undefined>(undefined);
```

Add type guard near bottom:

```ts
function deleteBlockedPayload(error: unknown): DigitalEmployeeDeleteBlockedErrorResponse | undefined {
  if (!(error instanceof ApiRequestError) || error.code !== "digital_employee_delete_blocked") {
    return undefined;
  }
  const payload = error.payload as DigitalEmployeeDeleteBlockedErrorResponse | undefined;
  if (!payload || !Array.isArray(payload.blockers)) {
    return undefined;
  }
  return payload;
}
```

Add mutation:

```ts
const deleteEmployee = useMutation({
  mutationFn: () => deleteDigitalEmployee(apiOptions, employeeId),
  onMutate: () => {
    setDeleteBlocked(undefined);
  },
  onSuccess: async () => {
    setDeleteOpen(false);
    setDeleteConfirmName("");
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employees"] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-overview"] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employees-overview"] }),
      queryClient.invalidateQueries({ queryKey: ["team-digital-employees"] }),
      queryClient.invalidateQueries({ queryKey: ["project-create", "digital-employees"] }),
      queryClient.invalidateQueries({ queryKey: ["unassigned-digital-employees"] }),
    ]);
    await navigate({ to: "/employees" });
  },
  onError: (error) => {
    const blocked = deleteBlockedPayload(error);
    if (blocked) {
      setDeleteBlocked(blocked);
    }
  },
});
```

Compute:

```ts
const canDeleteEmployee = employee.data?.allowed_actions?.includes("employee.delete") ?? false;
const deleteConfirmMatches = deleteConfirmName.trim() === (employee.data?.name ?? "");
```

Pass to header:

```tsx
<EmployeeDetailHeader
  canDelete={canDeleteEmployee}
  employee={employee.data}
  isDeleting={deleteEmployee.isPending}
  onDeleteEmployee={() => setDeleteOpen(true)}
  onManageCapabilities={() => setCapabilitiesOpen(true)}
  onStartTask={() => setStartTaskOpen(true)}
/>
```

Add `ConfirmDialog` near other overlays:

```tsx
<ConfirmDialog
  cancelBtnText="取消"
  confirmText="确认删除"
  desc="删除后该员工不会再出现在数字员工列表、团队数字员工列表、项目创建和项目调度候选中。历史运行、项目任务和审计记录会保留；Runtime 节点上的目录不会由本次请求直接物理删除，只写入审计和清理线索。"
  destructive
  disabled={!deleteConfirmMatches}
  handleConfirm={() => deleteEmployee.mutate()}
  isLoading={deleteEmployee.isPending}
  onOpenChange={(open) => {
    setDeleteOpen(open);
    if (!open) {
      setDeleteConfirmName("");
      setDeleteBlocked(undefined);
      deleteEmployee.reset();
    }
  }}
  open={deleteOpen}
  title="删除数字员工"
>
  <div className="space-y-3">
    <label className="block text-sm font-medium text-v3-ink" htmlFor="delete-employee-name-confirm">
      输入员工名称确认
    </label>
    <Input
      id="delete-employee-name-confirm"
      value={deleteConfirmName}
      onChange={(event) => setDeleteConfirmName(event.target.value)}
      aria-describedby="delete-employee-name-confirm-help"
    />
    <p id="delete-employee-name-confirm-help" className="text-xs text-v3-ink-3">
      必须输入完整员工名称：{employee.data?.name}
    </p>
    {deleteBlocked ? (
      <div className="rounded-v3-inner border border-destructive/25 bg-destructive/5 p-3 text-sm text-v3-ink">
        <p className="font-semibold text-destructive">{deleteBlocked.message}</p>
        <ul className="mt-2 space-y-2">
          {deleteBlocked.blockers.map((blocker) => (
            <li key={`${blocker.type}-${blocker.id}`} className="rounded-v3-inner bg-white/80 p-2 text-v3-ink-2">
              <span className="font-medium text-v3-ink">{blocker.title}</span>
              <span className="ml-2 text-xs tabular-nums text-v3-ink-3">{blocker.type} · {blocker.status}</span>
            </li>
          ))}
        </ul>
      </div>
    ) : null}
    {deleteEmployee.isError && !deleteBlocked ? (
      <p className="text-sm text-destructive">删除失败，请稍后重试。</p>
    ) : null}
  </div>
</ConfirmDialog>
```

- [ ] **Step 6: Run UI tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/components/employee-detail-header.test.tsx src/features/employees/detail.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/employees/components/employee-detail-header.tsx apps/web/src/features/employees/components/employee-detail-header.test.tsx apps/web/src/features/employees/detail.tsx apps/web/src/features/employees/detail.test.tsx
git commit -m "feat(web): add digital employee delete action"
```

---

### Task 7: Integration Verification and Real Smoke

**Files:**
- Verify only unless a previous task exposes a defect.
- Possible modify: any file from Tasks 1-6 if verification finds a bug.

**Interfaces:**
- Produces evidence that current code honors block/success/audit/list-removal behavior through real service paths.

- [ ] **Step 1: Run backend focused tests**

Run:

```bash
go test ./apps/control-plane/internal/authz -run 'TestDBAuthorizerEmployee(ActionsUseBusinessActionSurface|OwnerCanUsePersonalEmployeeActions)' -count=1
go test ./apps/control-plane/internal/employee -run 'Test(ServiceDeleteDigitalEmployee|PgRepositoryListDigitalEmployeeDeleteBlockers|PgRepositorySoftDeleteDigitalEmployeeCascadeAndAudit)' -count=1
go test ./apps/control-plane/internal/api -run 'Test(DeleteDigitalEmployeeRoute|GetDigitalEmployeeIncludesAllowedDeleteAction|EmployeeRoutesUseAuthzActions)' -count=1
```

Expected: all PASS.

- [ ] **Step 2: Run Web focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/client.test.ts src/lib/api/employees.test.ts src/features/employees/components/employee-detail-header.test.tsx src/features/employees/detail.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run contract and SQL generation checks**

Run:

```bash
make -C apps/control-plane generate-sqlc
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: commands complete without diff except checked-in sqlc/OpenAPI contract files intentionally changed by the feature.

- [ ] **Step 4: Check current-service status and restart changed services**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected: Control Plane and Web are running current code. Do not restart Runtime Agent unless a real Runtime path must be smoke-tested.

- [ ] **Step 5: Real API smoke for blocker behavior**

Use the repo's existing authenticated curl pattern. If no helper exists, log in with the local dev credentials already used by this repo's tests/manual smokes and save cookies under `/tmp/superteam-delete-smoke-cookies.txt`.

Create or select a digital employee with an active run row and call:

```bash
curl -i -b /tmp/superteam-delete-smoke-cookies.txt -X DELETE "http://127.0.0.1:8080/api/v1/digital-employees/${EMPLOYEE_ID}"
```

Expected:

```http
HTTP/1.1 409 Conflict
```

Response JSON contains:

```json
{
  "code": "digital_employee_delete_blocked",
  "blockers": [
    { "type": "run", "status": "running" }
  ]
}
```

If the local database cannot safely seed active runs, mark this smoke blocked with the missing seed path and do not claim end-to-end completion.

- [ ] **Step 6: Real API smoke for successful deletion and audit**

Create a throwaway employee with no active run/project-task blockers, then call DELETE.

Expected DELETE:

```http
HTTP/1.1 204 No Content
```

Then verify list/detail/audit through real endpoints:

```bash
curl -s -b /tmp/superteam-delete-smoke-cookies.txt "http://127.0.0.1:8080/api/v1/digital-employees" | jq '.[] | select(.id == env.EMPLOYEE_ID)'
curl -i -b /tmp/superteam-delete-smoke-cookies.txt "http://127.0.0.1:8080/api/v1/digital-employees/${EMPLOYEE_ID}"
curl -s -b /tmp/superteam-delete-smoke-cookies.txt "http://127.0.0.1:8080/api/v1/audit-events?resource_type=digital_employee&resource_id=${EMPLOYEE_ID}" | jq '.items[0]'
```

Expected:

- list query prints no row for the deleted employee.
- detail returns `404`.
- latest audit item has `action == "digital_employee.delete"`, `details.cleanup_candidates.agent_home_dir` when an execution instance existed, and no plaintext env var values.

- [ ] **Step 7: Real Web smoke**

Using Chrome plugin or the in-app browser, open the running Web app at the current local URL from `scripts/dev-services.sh status`. If the app shows the login screen, log in with the local dev admin account used by this repository's existing browser smokes, then navigate to a throwaway employee detail page and verify:

- Delete button is visible only for an admin/owner session with `employee.delete` allowed.
- Confirm button is disabled until the exact employee name is typed.
- A blocked employee shows the blocker list and stays on the detail page.
- A deletable employee returns to `/employees`, and the employee is absent from the list and any team/project selection surface that reads current digital employees.

- [ ] **Step 8: Run completion gate**

Read and follow `.codex/skills/superteam-completion-check/SKILL.md`.

At minimum run:

```bash
git diff --check
```

Then record the exact verification evidence from Steps 1-7. If real Web/API smoke was blocked, state the blocker and do not mark the feature fully complete.

- [ ] **Step 9: Final commit if verification changed files**

If verification required fixes:

```bash
git add <fixed-files>
git commit -m "fix(employee): complete digital employee delete verification fixes"
```

If no files changed, no commit is needed for this task.
