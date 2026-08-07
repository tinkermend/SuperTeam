# Project Delete Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add auditable project soft-delete with delete-preview, active-execution blockers, pending-approval cleanup, Temporal coordinator terminate-before-commit, runtime binding release, and detail-page Archive + Delete UI.

**Architecture:** Control Plane owns deletion as lock → blocker check → TerminateProjectCoordinator → single DB transaction (soft-delete project, deactivate members, cancel non-terminal tasks/approvals/inbox, delete runtime projection rows, write audit). Web detail header exposes Archive (moved from advanced aside) and Delete gated by `allowed_actions`, with a ConfirmDialog driven by `GET .../delete-preview`.

**Tech Stack:** Go + chi + pgx/v5 + sqlc + Atlas; Temporal Go SDK; React + TanStack Query/Router + Vitest + v3 UI; OpenAPI in `contracts/control-plane/openapi.yaml`.

**Spec:** `docs/superpowers/specs/2026-07-11-project-delete-design.md`

## Global Constraints

- Endpoints: `GET /api/v1/projects/{projectId}/delete-preview`, `DELETE /api/v1/projects/{projectId}`.
- Auth action: `project.delete` — tenant owner/admin **or** project `human_owner` only (same surface as `project.archive` / `project.update`); ordinary project members denied.
- Hard blockers: `project_tasks.status IN ('queued','running','in_progress')` and project-linked `task_runs.status IN ('queued','dispatching','running','cancelling')`.
- Warnings (preview only, cleaned on confirm): pending/requested decisions, `waiting_human`/`pending_review` tasks, open inbox items for the project, automation rules anchored to the project.
- Soft-delete: `projects.deleted_at`; list/get/schedule queries must exclude deleted rows; already-deleted → `404`.
- Cascade does **not** delete `digital_employees` rows; only `project_members` active → `removed`.
- Non-terminal `project_tasks` → `cancelled`; do **not** rewrite already-terminal statuses (`completed`,`failed`,`cancelled`,`done`,`success`).
- Pending `project_decision_requests.status_snapshot` / `approval_requests.status` → `cancelled`; open `inbox_items` for `source_project_id` **or** `digital_employee_run_recovery` anchored via run `tasks.params.metadata.anchor_project_id|project_id` → `cancelled`.
- Hard-delete projection rows: `project_runtime_nodes`, `project_employee_node_affinity`; release active `project_placements`.
- Terminate Temporal coordinator **before** DB commit; terminate failure → no DB changes; already-finished workflow = success.
- Audit: `audit_events.action = project.delete` with cascade counts; no secrets.
- Do not call Runtime Agent or delete local files.
- Archive remains separate; move Archive button to detail header; Delete only on detail.
- Migrations only under `apps/control-plane/internal/storage/migrations/`; update `atlas.sum`; `make -C apps/control-plane migrate-validate`.
- SQL under `apps/control-plane/internal/storage/queries/` → `make -C apps/control-plane generate-sqlc`.
- Contracts → `corepack pnpm generate:control-plane` / `verify:contracts` as required by repo scripts.
- Web follows `DESIGN.md`; tests via `corepack pnpm --filter ./apps/web run test` (never `npx vitest run`).
- Completion requires `$superteam-completion-check` and real Web/API/DB/Temporal smoke.

---

## File Structure

- `apps/control-plane/internal/authz/types.go` — `ActionProjectDelete`
- `apps/control-plane/internal/authz/authorizer.go` — include in `isProjectAction`; **not** in `projectActionAllowedForMember`
- `apps/control-plane/internal/authz/authorizer_test.go` — owner/admin allow, member deny
- `apps/control-plane/internal/storage/migrations/057_project_soft_delete.sql` (+ `atlas.sum`)
- `apps/control-plane/internal/storage/queries/project.sql` — `deleted_at` filters + delete SQL
- `apps/control-plane/internal/storage/queries/project_runtime_nodes.sql` / affinity / placement release queries as needed
- `apps/control-plane/internal/storage/queries/inbox.sql` — cancel open inbox by project
- Regenerated sqlc outputs
- `apps/control-plane/internal/project/types.go` — preview/blocker/cascade/audit types + errors
- `apps/control-plane/internal/project/repository.go` — delete repository methods
- `apps/control-plane/internal/project/pg_repository.go` — implementations via `withProjectQueries`
- `apps/control-plane/internal/project/coordination_signal.go` — `TerminateProjectCoordinator`
- `apps/control-plane/internal/project/service.go` — `GetProjectDeletePreview`, `DeleteProject`
- `apps/control-plane/internal/workflow/projectcoordination/client.go` — Temporal terminate
- `apps/control-plane/internal/project/handler.go` — HTTP + `allowed_actions` on project responses
- `apps/control-plane/internal/api/server.go` — routes
- `apps/control-plane/internal/api/project_routes_test.go` — route tests
- `contracts/control-plane/openapi.yaml` — schemas + paths
- `apps/web/src/lib/api/projects.ts` (+ tests)
- `apps/web/src/features/projects/components/project-operational-detail.tsx` (+ tests)
- `apps/web/src/features/projects/index.tsx` — wire mutations/dialogs

**OpenFGA note:** Existing `project.*` actions are DB-authorizer primary and are **not** mapped in `openFGARelationForAction` (same as `project.archive`). Do not invent a partial OpenFGA project model in this plan; keep parity with archive.

---

### Task 1: Authorization for `project.delete`

**Files:**
- Modify: `apps/control-plane/internal/authz/types.go`
- Modify: `apps/control-plane/internal/authz/authorizer.go`
- Test: `apps/control-plane/internal/authz/authorizer_test.go`

**Interfaces:**
- Produces: `authz.ActionProjectDelete = "project.delete"`
- Produces: `checkProjectAccess` allows tenant admin or `human_owner`; members denied via `projectActionAllowedForMember` omission

- [ ] **Step 1: Write failing authz tests**

Add cases to the existing project authz table test (or new `TestDBAuthorizerProjectDelete`):

```go
{name: "admin deletes project", action: ActionProjectDelete, /* ResourceProject + projectID */, tenantRole: RoleAdmin, allowed: true},
{name: "owner tenant deletes project", action: ActionProjectDelete, tenantRole: RoleOwner, allowed: true},
{name: "human_owner deletes project", action: ActionProjectDelete, asHumanOwner: true, tenantRole: RoleMember, allowed: true, matchedRule: "project.owner"},
{name: "member cannot delete project", action: ActionProjectDelete, asMember: true, tenantRole: RoleMember, denyReason: ReasonNoMembership},
```

Mirror the fixture style used for `ActionProjectArchive` in the same file.

- [ ] **Step 2: Run and confirm FAIL**

```bash
go test ./apps/control-plane/internal/authz -run 'ProjectDelete|ProjectArchive|ProjectAccess' -count=1
```

Expected: FAIL — `ActionProjectDelete` undefined.

- [ ] **Step 3: Implement**

In `types.go` next to `ActionProjectArchive`:

```go
ActionProjectDelete = "project.delete"
```

In `authorizer.go` `isProjectAction` switch, add `ActionProjectDelete` next to `ActionProjectArchive`.

Do **not** add `ActionProjectDelete` to `projectActionAllowedForMember`.

- [ ] **Step 4: Run tests PASS**

```bash
go test ./apps/control-plane/internal/authz -run 'ProjectDelete|ProjectAccess' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/authz
git commit -m "$(cat <<'EOF'
feat(authz): add project.delete for owner and tenant admin

EOF
)"
```

---

### Task 2: Migration `projects.deleted_at` + list/get filters

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/057_project_soft_delete.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/queries/project.sql` (`GetProject`, `ListProjects`, and any other `FROM projects` current-surface queries used by Console list/detail)
- Modify: `apps/control-plane/internal/storage/migrations_test.go` if it asserts projects columns
- Run: `make -C apps/control-plane generate-sqlc` and `make -C apps/control-plane migrate-validate`

**Interfaces:**
- Produces: `projects.deleted_at TIMESTAMPTZ NULL`
- Produces: `GetProject` / `ListProjects` return only `deleted_at IS NULL` rows

- [ ] **Step 1: Write migration**

```sql
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

COMMENT ON COLUMN projects.deleted_at IS '软删除时间；非空表示项目已从当前管理面移除';

CREATE INDEX IF NOT EXISTS idx_projects_tenant_deleted_created
    ON projects (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;
```

- [ ] **Step 2: Update atlas.sum and validate**

```bash
# follow repo convention used by prior migrations (atlas migrate hash / make target)
make -C apps/control-plane migrate-validate
```

Expected: PASS (or fix checksum per Makefile docs).

- [ ] **Step 3: Filter SQL**

Update:

```sql
-- name: GetProject :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListProjects :many
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  ...
```

Audit other Console-facing project selectors in `project.sql` (risk summaries / workflow instance project joins) and add `p.deleted_at IS NULL` where they drive current lists. Do not filter historical audit joins that must still resolve by id for forensics inside delete path — those use dedicated `FOR UPDATE` lock queries in Task 3.

- [ ] **Step 4: generate-sqlc**

```bash
make -C apps/control-plane generate-sqlc
```

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/storage
git commit -m "$(cat <<'EOF'
feat(db): add projects.deleted_at and filter current project queries

EOF
)"
```

---

### Task 3: Delete SQL — lock, blockers, preview, cascade

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify: `apps/control-plane/internal/storage/queries/project_runtime_nodes.sql` (or project.sql)
- Modify: `apps/control-plane/internal/storage/queries/inbox.sql`
- Modify: `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql` if placements live there
- Run: `make -C apps/control-plane generate-sqlc`

**Interfaces:**
- Produces sqlc methods used by repository in Task 4

- [ ] **Step 1: Add lock + soft-delete project**

```sql
-- name: GetProjectForDelete :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
FOR UPDATE;

-- name: SoftDeleteProject :one
UPDATE projects
SET deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz,
    coordination_status = 'terminated'
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 2: Blocker queries**

```sql
-- name: ListProjectDeleteTaskBlockers :many
SELECT
    'project_task'::text AS blocker_type,
    pt.id,
    pt.status,
    (COALESCE(NULLIF(pt.title, ''), pt.id::text))::text AS title
FROM project_tasks pt
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
  AND pt.status IN ('queued', 'running', 'in_progress')
ORDER BY pt.updated_at DESC
LIMIT 20;

-- name: ListProjectDeleteRunBlockers :many
SELECT
    'run'::text AS blocker_type,
    tr.id,
    tr.status,
    (COALESCE(NULLIF(tr.title, ''), tr.id::text))::text AS title
FROM task_runs tr
INNER JOIN project_tasks pt
  ON pt.tenant_id = tr.tenant_id
 AND pt.digital_employee_run_id = tr.id
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
ORDER BY tr.updated_at DESC
LIMIT 20;
```

Adjust join column if `task_runs` title field differs — match `ListDigitalEmployeeDeleteRunBlockers` in `employee_execution.sql`.

- [ ] **Step 3: Preview warning counts**

```sql
-- name: GetProjectDeletePreviewCounts :one
SELECT
  (SELECT COUNT(*)::int FROM project_decision_requests
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
      AND status_snapshot IN ('pending', 'requested')) AS pending_decision_count,
  (SELECT COUNT(*)::int FROM project_tasks
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
      AND status IN ('waiting_human', 'pending_review')) AS waiting_human_task_count,
  (SELECT COUNT(*)::int FROM inbox_items
    WHERE tenant_id = sqlc.arg('tenant_id') AND source_project_id = sqlc.arg('project_id')
      AND status = 'open') AS open_inbox_count,
  (SELECT COUNT(*)::int FROM project_members
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
      AND status = 'active') AS active_member_count,
  (SELECT COUNT(*)::int FROM project_members
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
      AND status = 'active' AND principal_type = 'digital_employee') AS digital_employee_member_count,
  (SELECT COUNT(*)::int FROM project_runtime_nodes
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')) AS runtime_node_binding_count,
  (SELECT COUNT(*)::int FROM project_employee_node_affinity
    WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')) AS affinity_count;
```

- [ ] **Step 4: Cascade updates/deletes**

```sql
-- name: DeactivateProjectMembersForDelete :many
UPDATE project_members
SET status = 'removed', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id') AND status = 'active'
RETURNING id;

-- name: CancelProjectTasksForDelete :many
UPDATE project_tasks
SET status = 'cancelled', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
  AND status NOT IN ('completed', 'failed', 'cancelled', 'done', 'success')
RETURNING id;

-- name: CancelProjectDecisionRequestsForDelete :many
UPDATE project_decision_requests
SET status_snapshot = 'cancelled', resolved_at = COALESCE(resolved_at, NOW()), updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
  AND status_snapshot IN ('pending', 'requested')
RETURNING id;

-- name: CancelApprovalRequestsForProjectDelete :many
UPDATE approval_requests
SET status = 'cancelled', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
  AND status = 'pending'
RETURNING id;

-- name: CancelInboxItemsForProjectDelete :many
UPDATE inbox_items
SET status = 'cancelled', resolved_at = COALESCE(resolved_at, NOW()), updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND source_project_id = sqlc.arg('project_id')
  AND status = 'open'
RETURNING id;

-- name: DeleteProjectRuntimeNodesForDelete :many
DELETE FROM project_runtime_nodes
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
RETURNING id;

-- name: DeleteProjectEmployeeNodeAffinitiesForDelete :many
DELETE FROM project_employee_node_affinity
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
RETURNING id;

-- name: ReleaseProjectPlacementsForDelete :many
UPDATE project_placements
SET state = 'released', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id') AND project_id = sqlc.arg('project_id')
  AND state = 'active'
RETURNING id;
```

Verify `approval_requests` / `project_placements` column names against migrations before finalizing; adjust to match schema exactly.

- [ ] **Step 5: generate-sqlc and commit**

```bash
make -C apps/control-plane generate-sqlc
git add apps/control-plane/internal/storage/queries
git commit -m "$(cat <<'EOF'
feat(db): add project delete lock, blocker, preview, and cascade queries

EOF
)"
```

---

### Task 4: Domain types + repository cascade

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go` memory repository stubs as needed
- Test: add focused pg or service tests in later task; here at least compile stubs

**Interfaces:**
- Produces:

```go
const (
  ProjectDeleteBlockedCode = "project_delete_blocked"
  ErrProjectDeleteBlocked = ... // sentinel
)

type ProjectDeleteBlocker struct {
  Type   string `json:"type"`
  ID     string `json:"id"`
  Status string `json:"status"`
  Title  string `json:"title"`
}

type ProjectDeleteBlockedError struct { Blockers []ProjectDeleteBlocker }

type ProjectDeletePreview struct {
  ProjectID   uuid.UUID
  ProjectName string
  CanDelete   bool
  Blockers    []ProjectDeleteBlocker
  Warnings    ProjectDeleteWarnings
  Message     string
}

type ProjectDeleteWarnings struct {
  PendingDecisionCount       int32 `json:"pending_decision_count"`
  WaitingHumanTaskCount      int32 `json:"waiting_human_task_count"`
  OpenInboxCount             int32 `json:"open_inbox_count"`
  ActiveMemberCount          int32 `json:"active_member_count"`
  DigitalEmployeeMemberCount int32 `json:"digital_employee_member_count"`
  RuntimeNodeBindingCount    int32 `json:"runtime_node_binding_count"`
  AffinityCount              int32 `json:"affinity_count"`
}

type ProjectDeleteCascadeResult struct {
  MemberCount, TaskCount, DecisionCount, ApprovalCount, InboxCount, RuntimeNodeCount, AffinityCount, PlacementCount int
}

type DeleteProjectRequest struct {
  TenantID, ProjectID, ActorUserID uuid.UUID
}
```

- Repository methods: `GetProjectForDelete`, `ListProjectDeleteBlockers`, `GetProjectDeletePreviewCounts`, `SoftDeleteProjectCascade`, `CreateProjectDeleteAuditEvent`

- [ ] **Step 1: Add types and repository interface methods**

- [ ] **Step 2: Implement `SoftDeleteProjectCascade` inside `withProjectQueries`**

Order inside tx: soft-delete project → deactivate members → cancel tasks → cancel decisions → cancel approvals → cancel inbox → delete runtime nodes → delete affinities → release placements → create audit event (or audit as separate repo call from service after cascade counts returned).

Audit details JSON shape:

```json
{
  "project_id": "...",
  "project_name": "...",
  "workflow_id": "...",
  "cascade": { "members": N, "tasks": N, "decisions": N, "approvals": N, "inbox": N, "runtime_nodes": N, "affinities": N, "placements": N }
}
```

Use `CreateAuditEvent` with `Action: "project.delete"`, `ResourceType: "project"`.

- [ ] **Step 3: Update memoryRepository in tests to stub new methods** (compile)

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(project): add delete cascade repository and domain types

EOF
)"
```

---

### Task 5: Temporal `TerminateProjectCoordinator`

**Files:**
- Modify: `apps/control-plane/internal/project/coordination_signal.go`
- Modify: `apps/control-plane/internal/project/coordination_signal.go` `NoopCoordinatorSignalClient`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/client.go`
- Test: unit test with fake temporal client if present; otherwise test Noop + service fake

**Interfaces:**
- Produces:

```go
type TerminateProjectCoordinatorSignal struct {
  TenantID   uuid.UUID
  ProjectID  uuid.UUID
  WorkflowID string
  Reason     string
}

// on CoordinatorSignalClient:
TerminateProjectCoordinator(ctx context.Context, signal TerminateProjectCoordinatorSignal) error
```

- [ ] **Step 1: Extend interface + Noop (return nil)**

- [ ] **Step 2: Implement on `projectcoordination.SignalClient`**

```go
func (c *SignalClient) TerminateProjectCoordinator(ctx context.Context, signal project.TerminateProjectCoordinatorSignal) error {
  wfID := workflowID(signal.WorkflowID, signal.ProjectID.String())
  err := c.client.TerminateWorkflow(ctx, wfID, "", signal.Reason)
  if err == nil {
    return nil
  }
  // Treat not-found / already-completed as success — use temporal/serviceerror helpers
  // matching patterns elsewhere in the repo if any; otherwise inspect error string/type.
  if isWorkflowMissingOrCompleted(err) {
    return nil
  }
  return err
}
```

Do **not** use `SignalWithStart` here (that would restart the coordinator).

- [ ] **Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(coordination): terminate project coordinator before delete

EOF
)"
```

---

### Task 6: Service `GetProjectDeletePreview` + `DeleteProject`

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: memory coordinator fake to record terminate calls

**Interfaces:**
- Produces: `(*ProjectDeletePreview, error)`, `error` from `DeleteProject`
- Consumes: repository + `CoordinatorSignalClient.TerminateProjectCoordinator`

- [ ] **Step 1: Failing service tests**

```go
func TestDeleteProjectBlockedByActiveTask(t *testing.T) { ... expect ErrProjectDeleteBlocked / ProjectDeleteBlockedError }
func TestDeleteProjectTerminatesThenCascades(t *testing.T) { ... expect terminate called, project soft-deleted, audit written }
func TestDeleteProjectAbortsWhenTerminateFails(t *testing.T) { ... terminate err => no cascade }
func TestGetProjectDeletePreviewIncludesWarnings(t *testing.T) { ... }
```

- [ ] **Step 2: Implement service methods**

```go
func (s *Service) GetProjectDeletePreview(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectDeletePreview, error)

func (s *Service) DeleteProject(ctx context.Context, req DeleteProjectRequest) error {
  // 1. validate IDs
  // 2. lock project (GetProjectForDelete) — outside or inside tx; blockers before terminate
  // 3. list blockers → return ProjectDeleteBlockedError
  // 4. s.coordinator.TerminateProjectCoordinator(...)
  // 5. SoftDeleteProjectCascade + audit in withProjectQueries / single tx
}
```

Preferred order matching spec: lock+blockers can be in a short read tx or same connection; **terminate outside DB tx**; then cascade tx. If cascade fails after terminate, return error (caller retries; terminate is idempotent success on missing workflow).

- [ ] **Step 3: Tests PASS**

```bash
go test ./apps/control-plane/internal/project -run 'DeleteProject|DeletePreview' -count=1
```

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(project): implement delete preview and delete service flow

EOF
)"
```

---

### Task 7: HTTP handlers, routes, OpenAPI, allowed_actions

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Run: `corepack pnpm generate:control-plane` and/or `corepack pnpm verify:contracts`

**Interfaces:**
- `GET .../delete-preview` → `200` JSON preview
- `DELETE .../{projectId}` → `204` / `404` / `409 project_delete_blocked` / terminate failure `503` or `409`
- `projectResponse.AllowedActions []string` injected on `GetProject` (and list items if cheap)

- [ ] **Step 1: Handler methods**

Authorize with `authorizeProjectScopedAction(..., authz.ActionProjectDelete)`.

Map `ProjectDeleteBlockedError` like employee delete:

```go
writeJSON(w, http.StatusConflict, map[string]any{
  "code": ProjectDeleteBlockedCode,
  "message": "该项目仍有排队或执行中的任务，停止或完成后再删除。",
  "blockers": blocked.Blockers,
})
```

Terminate failure → Chinese message + `503` (retryable).

- [ ] **Step 2: `allowedProjectActions` helper**

```go
actions := []string{authz.ActionProjectArchive, authz.ActionProjectDelete}
// Check each; append if Allowed
```

Attach on `GetProject` response (minimum). Optionally list.

- [ ] **Step 3: Register routes next to archive**

```go
r.Get("/projects/{projectId}/delete-preview", s.projectHandler.GetProjectDeletePreview)
r.Delete("/projects/{projectId}", s.projectHandler.DeleteProject)
```

- [ ] **Step 4: OpenAPI**

Document both paths, `ProjectDeletePreview`, `ProjectDeleteBlockedError`, and `Project.allowed_actions`.

- [ ] **Step 5: Route tests + generate/verify contracts**

```bash
go test ./apps/control-plane/internal/api -run Project -count=1
corepack pnpm verify:contracts
```

- [ ] **Step 6: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(api): expose project delete preview and delete endpoints

EOF
)"
```

---

### Task 8: Web API client

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`

**Interfaces:**
- `Project.allowed_actions?: string[]`
- `getProjectDeletePreview(options, projectId)`
- `deleteProject(options, projectId)` → void / throws `ApiRequestError` with JSON body

- [ ] **Step 1: Types + functions** mirroring employee delete client patterns (`deleteDigitalEmployee`, blocked error parsing)

- [ ] **Step 2: Unit tests for request paths and 409 payload**

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts
```

- [ ] **Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(web): add project delete API client helpers

EOF
)"
```

---

### Task 9: Detail UI — Archive in header + Delete confirm

**Files:**
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.test.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`
- Optionally extract small `project-delete-dialog.tsx` if detail file is already huge

**Interfaces:**
- Props: `onArchiveProject`, `onDeleteProject` (or internal dialog owned by index)
- Visibility: `allowed_actions?.includes("project.archive")` / `"project.delete"`; if archive action missing from API temporarily, keep archive visible for non-archived when caller passes handler (but prefer gating once Task 7 ships)

- [ ] **Step 1: Move Archive button into header action cluster** (next to 提交需求 / 配置项目); remove from advanced aside

- [ ] **Step 2: Add Delete button (destructive) when `project.delete` allowed

- [ ] **Step 3: ConfirmDialog flow**

On open → fetch `getProjectDeletePreview`:
- Show warnings counts
- If blockers → list + disable confirm
- Require typing `project.name`
- Confirm → `deleteProject` → navigate to `/projects` (or list without `project` search param) + invalidate queries
- Handle 409 blockers inline

Reuse `@/components/confirm-dialog` and patterns from `apps/web/src/features/employees/detail.tsx`.

- [ ] **Step 4: Component/page tests**

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/components/project-operational-detail.test.tsx
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(web): surface project archive and delete on detail header

EOF
)"
```

---

### Task 10: Verification gate

**Files:** none new

- [ ] **Step 1: Layer verifies**

```bash
corepack pnpm verify:contracts
corepack pnpm verify:control-plane
corepack pnpm verify:web
make -C apps/control-plane migrate-validate
```

- [ ] **Step 2: Real smoke**

1. `scripts/dev-services.sh status` — restart `control-plane` (and web if needed) so current code loads
2. Create or pick a project as human_owner
3. `GET .../delete-preview` shows warnings/can_delete
4. With a queued/running task → delete returns 409
5. Without blockers, with pending decision → preview warns; delete succeeds; project gone from list; members removed; runtime nodes gone; audit row `project.delete` present; Temporal workflow terminated
6. Archive button visible on detail header for non-archived project

- [ ] **Step 3: Run `$superteam-completion-check`** before claiming done

- [ ] **Step 4: Final commit only if smoke fixes needed**

---

## Spec coverage checklist

| Spec item | Task |
|---|---|
| `project.delete` authz | 1 |
| `deleted_at` + list filters | 2 |
| Blockers / warnings / cascade SQL | 3 |
| Soft-delete cascade + audit | 4, 6 |
| Terminate before commit | 5, 6 |
| delete-preview + DELETE API + OpenAPI | 7 |
| allowed_actions | 7, 8, 9 |
| Detail Archive + Delete UI | 9 |
| E2E smoke | 10 |
| No Runtime Agent calls | Global + 6 |
| Archive remains separate | Global + 9 |

## Placeholder / consistency self-review

- No TBD left; member status fixed to `removed`; task cancel excludes terminal statuses; OpenFGA explicitly deferred to match archive.
- Method names consistent: `TerminateProjectCoordinator`, `GetProjectDeletePreview`, `DeleteProject`, `ProjectDeleteBlockedCode`.
