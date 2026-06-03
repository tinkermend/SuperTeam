# Team Management Foundation and Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared team-management foundation: richer team list, detail overview, lifecycle actions, allowed actions, and the frontend shell that later team tabs attach to.

**Architecture:** This plan extends the existing `tenant` module rather than creating a new module. Backend service methods own lifecycle and overview rules; handlers perform console auth and per-action authorization; the Web feature consumes a new team API client and renders `/teams` plus `/teams/:teamId`.

**Tech Stack:** Go, chi/net/http, pgx/sqlc, PostgreSQL JSONB, OpenAPI YAML, React, TanStack Router, TanStack Query, shadcn/ui, lucide-react, Vitest Browser.

---

## Dependency Graph

This plan is the prerequisite for the other team-management plans:

- Plan 2 `team-management-members-roles` depends on the detail shell, team status semantics, and `allowed_actions`.
- Plan 3 `team-management-governance-capabilities` depends on the detail shell and overview governance summaries.
- Plan 4 `team-management-employees-audit` depends on the detail shell and team-scoped routing.

After this plan is merged, Plans 2, 3, and 4 can run in parallel in separate branches or worktrees.

## File Structure

- Modify `apps/control-plane/internal/tenant/types.go`: add `archived` team status, overview/list summary types, update/lifecycle requests, and allowed action fields.
- Modify `apps/control-plane/internal/tenant/repository.go`: add repository methods for search/list summaries, lifecycle mutations, and overview aggregate reads.
- Modify `apps/control-plane/internal/storage/queries/tenant_team_config.sql`: add sqlc queries for list summaries, update, disable, archive, restore, current draft count, member count, digital employee count, and capability count extraction.
- Regenerate `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go` via `cd apps/control-plane && make generate-sqlc`.
- Modify `apps/control-plane/internal/tenant/pg_repository.go`: map sqlc rows to tenant records and summary structs.
- Modify `apps/control-plane/internal/tenant/service.go`: implement lifecycle and overview orchestration.
- Modify `apps/control-plane/internal/tenant/service_test.go`: add focused service tests for validation and lifecycle state.
- Modify `apps/control-plane/internal/tenant/handler.go`: add routes, request/response DTOs, and `allowed_actions` calculation.
- Modify `apps/control-plane/internal/api/server.go`: register new team routes.
- Modify `apps/control-plane/internal/api/team_routes_test.go`: cover new routes and action mapping.
- Modify `contracts/control-plane/openapi.yaml`: document new endpoints and schemas.
- Modify `apps/web/src/lib/api/teams.ts`: add frontend API types and functions.
- Modify `apps/web/src/lib/api/teams.test.ts`: cover new API paths and methods.
- Modify `apps/web/src/routes/_authenticated/teams/index.tsx`: keep list route.
- Create `apps/web/src/routes/_authenticated/teams/$teamId.tsx`: detail route.
- Replace `apps/web/src/features/teams/index.tsx` with a split feature shell or extract components under `apps/web/src/features/teams/components/`.
- Modify `apps/web/src/features/teams/index.test.tsx`: cover list and detail shell.
- Modify `CHANGELOG.md`: add a short entry under `Unreleased`.

## Task 1: Backend Status and Overview Types

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write failing service tests for archived status and lifecycle validation**

Add tests that prove the domain accepts `archived`, rejects unknown status, and keeps `active` as the default:

```go
func TestTeamStatusAllowsArchived(t *testing.T) {
	if !TeamStatusArchived.IsValid() {
		t.Fatalf("expected archived team status to be valid")
	}
	if TeamStatus("paused").IsValid() {
		t.Fatalf("expected unknown team status to be invalid")
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run TestTeamStatusAllowsArchived -count=1
```

Expected: FAIL because `TeamStatusArchived` is not defined.

- [ ] **Step 3: Add domain types**

Add these status and request/response types in `apps/control-plane/internal/tenant/types.go`:

```go
const (
	TeamStatusActive   TeamStatus = "active"
	TeamStatusDisabled TeamStatus = "disabled"
	TeamStatusArchived TeamStatus = "archived"
)

type GovernanceSummaryStatus string

const (
	GovernanceSummaryNotConfigured GovernanceSummaryStatus = "not_configured"
	GovernanceSummaryDraftPending  GovernanceSummaryStatus = "draft_pending"
	GovernanceSummaryActive        GovernanceSummaryStatus = "active"
	GovernanceSummaryNeedsUpdate   GovernanceSummaryStatus = "needs_update"
)

type AllowedTeamAction string

type TeamListItem struct {
	Team
	MemberCount          int32
	DigitalEmployeeCount int32
	CapabilityCount      int32
	GovernanceStatus     GovernanceSummaryStatus
	CurrentRevision      *int32
	PendingDraftCount    int32
	RiskSummary          string
}

type TeamOverview struct {
	Team                 *Team
	MemberCount          int32
	DigitalEmployeeCount int32
	CapabilityCount      int32
	CurrentRevision      *TeamConfigRevision
	PendingDraftCount    int32
	PendingItemCount     int32
	AllowedActions       []AllowedTeamAction
}

type UpdateTeamRequest struct {
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	Name             string
	Slug             string
	HumanOwnerUserID *uuid.UUID
	Metadata         map[string]any
}

type ChangeTeamStatusRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	Status   TeamStatus
}
```

Update `TeamStatus.IsValid()` to include `TeamStatusArchived`.

- [ ] **Step 4: Re-run the focused test**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run TestTeamStatusAllowsArchived -count=1
```

Expected: PASS.

## Task 2: SQL Queries for List Summary and Lifecycle

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
- Generated: `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Add a failing query integration test**

In `queries_test.go`, add a test that inserts one team, one team member, one digital employee, and one active config revision, then calls `ListTenantTeamSummaries` and expects counts:

```go
func TestListTenantTeamSummariesReturnsGovernanceCounts(t *testing.T) {
	db := newQueriesTestDB(t)
	q := New(db)
	tenantID := seedTestTenant(t, db)
	teamID := seedTestTeam(t, db, tenantID, "ops", "运维团队")
	seedTestTenantMember(t, db, tenantID, &teamID, "member")
	seedTestDigitalEmployee(t, db, tenantID, &teamID, "数据库运维员工")
	seedTestTeamConfigRevision(t, db, tenantID, teamID, "active", 1)

	rows, err := q.ListTenantTeamSummaries(context.Background(), ListTenantTeamSummariesParams{
		TenantID: tenantID,
		Limit:    20,
		Offset:   0,
	})
	if err != nil {
		t.Fatalf("list team summaries: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].MemberCount != 1 || rows[0].DigitalEmployeeCount != 1 {
		t.Fatalf("unexpected counts: members=%d employees=%d", rows[0].MemberCount, rows[0].DigitalEmployeeCount)
	}
}
```

Use existing helper patterns in `queries_test.go`; if helpers do not exist, add local helper functions in the same test file that execute explicit `INSERT` statements.

- [ ] **Step 2: Run the query test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestListTenantTeamSummariesReturnsGovernanceCounts -count=1
```

Expected: FAIL because `ListTenantTeamSummaries` is not generated.

- [ ] **Step 3: Add SQL queries**

Append these query names to `tenant_team_config.sql`:

```sql
-- name: ListTenantTeamSummaries :many
WITH current_config AS (
  SELECT DISTINCT ON (tenant_id, team_id)
    tenant_id,
    team_id,
    revision_number,
    capability_policy,
    approval_policy,
    status
  FROM tenant_team_config_revisions
  WHERE status = 'active'
    AND archived_at IS NULL
  ORDER BY tenant_id, team_id, revision_number DESC
),
draft_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS pending_draft_count
  FROM tenant_team_config_revisions
  WHERE status = 'draft'
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
),
member_counts AS (
  SELECT tenant_id, team_id, COUNT(DISTINCT principal_id)::integer AS member_count
  FROM tenant_members
  WHERE team_id IS NOT NULL
    AND principal_type = 'user'
    AND status = 'active'
    AND disabled_at IS NULL
  GROUP BY tenant_id, team_id
),
employee_counts AS (
  SELECT tenant_id, team_id, COUNT(*)::integer AS digital_employee_count
  FROM digital_employees
  WHERE team_id IS NOT NULL
    AND deleted_at IS NULL
    AND archived_at IS NULL
  GROUP BY tenant_id, team_id
)
SELECT
  tt.*,
  COALESCE(mc.member_count, 0)::integer AS member_count,
  COALESCE(ec.digital_employee_count, 0)::integer AS digital_employee_count,
  (
    COALESCE(jsonb_array_length(cc.capability_policy->'skill_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'mcp_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'knowledge_base_bindings'), 0) +
    COALESCE(jsonb_array_length(cc.capability_policy->'external_capability_bindings'), 0)
  )::integer AS capability_count,
  cc.revision_number AS current_revision,
  COALESCE(dc.pending_draft_count, 0)::integer AS pending_draft_count,
  CASE
    WHEN cc.team_id IS NULL THEN 'not_configured'
    WHEN COALESCE(dc.pending_draft_count, 0) > 0 THEN 'draft_pending'
    ELSE 'active'
  END::varchar AS governance_status,
  COALESCE(cc.approval_policy->>'risk_summary', '')::varchar AS risk_summary
FROM tenant_teams tt
LEFT JOIN current_config cc ON cc.tenant_id = tt.tenant_id AND cc.team_id = tt.id
LEFT JOIN draft_counts dc ON dc.tenant_id = tt.tenant_id AND dc.team_id = tt.id
LEFT JOIN member_counts mc ON mc.tenant_id = tt.tenant_id AND mc.team_id = tt.id
LEFT JOIN employee_counts ec ON ec.tenant_id = tt.tenant_id AND ec.team_id = tt.id
WHERE tt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tt.deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR tt.status = sqlc.narg('status')::varchar)
  AND (
    sqlc.narg('q')::varchar IS NULL
    OR tt.name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR tt.slug ILIKE '%' || sqlc.narg('q')::varchar || '%'
  )
ORDER BY tt.updated_at DESC, tt.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateTenantTeam :one
UPDATE tenant_teams
SET
  slug = sqlc.arg('slug')::varchar,
  name = sqlc.arg('name')::varchar,
  human_owner_user_id = sqlc.narg('human_owner_user_id')::uuid,
  metadata = COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: SetTenantTeamStatus :one
UPDATE tenant_teams
SET
  status = sqlc.arg('status')::varchar,
  disabled_at = CASE
    WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
    WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
    ELSE disabled_at
  END,
  archived_at = CASE
    WHEN sqlc.arg('status')::varchar = 'archived' THEN COALESCE(archived_at, NOW())
    WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
    ELSE archived_at
  END
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
RETURNING *;
```

- [ ] **Step 4: Regenerate sqlc outputs**

Run the repo’s sqlc generation command:

```bash
cd apps/control-plane && make generate-sqlc
```

Expected: `internal/storage/queries/tenant_team_config.sql.go` contains `ListTenantTeamSummaries`, `UpdateTenantTeam`, and `SetTenantTeamStatus`.

- [ ] **Step 5: Run query tests**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestListTenantTeamSummariesReturnsGovernanceCounts -count=1
```

Expected: PASS.

## Task 3: Repository and Service Methods

**Files:**
- Modify: `apps/control-plane/internal/tenant/repository.go`
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`
- Modify: `apps/control-plane/internal/tenant/service.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests for:

```go
func TestUpdateTeamRejectsEmptyName(t *testing.T) { /* call UpdateTeam with blank name and expect ErrInvalidInput */ }
func TestChangeTeamStatusRejectsInvalidStatus(t *testing.T) { /* call ChangeTeamStatus with TeamStatus("paused") */ }
func TestListTeamSummariesDefaultsLimit(t *testing.T) { /* call ListTeamSummaries with Limit 0 and assert repository received 50 */ }
```

- [ ] **Step 2: Run focused service tests**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'Test(UpdateTeam|ChangeTeamStatus|ListTeamSummaries)' -count=1
```

Expected: FAIL because methods do not exist.

- [ ] **Step 3: Extend repository interface**

Add methods:

```go
ListTeamSummaries(ctx context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error)
UpdateTeam(ctx context.Context, params UpdateTeamParams) (TeamRecord, error)
SetTeamStatus(ctx context.Context, params SetTeamStatusParams) (TeamRecord, error)
```

Define params and record aliases in `repository.go`.

- [ ] **Step 4: Implement service methods**

Add methods in `service.go`:

```go
func (s *Service) ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error)
func (s *Service) UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error)
func (s *Service) ChangeTeamStatus(ctx context.Context, req ChangeTeamStatusRequest) (*Team, error)
func (s *Service) GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error)
```

Validation rules:

- `tenant_id` and `team_id` must be non-nil UUIDs where applicable.
- `name` and `slug` must be trimmed and non-empty for update.
- status must be one of active, disabled, archived.
- list limit defaults to 50 and caps at 100.

- [ ] **Step 5: Implement pg repository mapping**

Use generated sqlc methods in `pg_repository.go`. Map nullable `current_revision` to `*int32`, and map `governance_status` to `GovernanceSummaryStatus`.

- [ ] **Step 6: Run tenant service tests**

Run:

```bash
go test ./apps/control-plane/internal/tenant -count=1
```

Expected: PASS.

## Task 4: HTTP Routes, OpenAPI, and Allowed Actions

**Files:**
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/team_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write failing route tests**

Add cases that verify:

- `GET /api/v1/teams/{teamId}/overview` checks `team.read`.
- `PATCH /api/v1/teams/{teamId}` checks `team.update`.
- `POST /api/v1/teams/{teamId}/disable` checks `team.disable`.
- `POST /api/v1/teams/{teamId}/archive` checks `team.archive`.
- `POST /api/v1/teams/{teamId}/restore` checks `team.restore`.

- [ ] **Step 2: Run route tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: FAIL because routes do not exist.

- [ ] **Step 3: Extend handler service interface**

Add to `HandlerService`:

```go
ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error)
UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error)
ChangeTeamStatus(ctx context.Context, req ChangeTeamStatusRequest) (*Team, error)
GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error)
```

- [ ] **Step 4: Implement handlers**

Add handler methods:

```go
func (h *HTTPHandler) GetTeamOverview(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) UpdateTeam(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) DisableTeam(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ArchiveTeam(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) RestoreTeam(w http.ResponseWriter, r *http.Request)
```

For overview, compute `allowed_actions` by checking these actions against the team resource and ignoring denied decisions:

```go
var overviewActions = []string{
	authz.ActionTeamUpdate,
	authz.ActionTeamDisable,
	authz.ActionTeamArchive,
	authz.ActionTeamRestore,
	authz.ActionTeamMemberAdd,
	authz.ActionTeamMemberRequestPrivilegedRole,
	authz.ActionTeamGovernanceEdit,
	authz.ActionTeamGovernanceApprove,
	authz.ActionTeamCapabilityBind,
	authz.ActionTeamCapabilityUnbind,
	authz.ActionTeamAuditRead,
}
```

- [ ] **Step 5: Register routes**

In `server.go`, add:

```go
r.Get("/teams/{teamId}/overview", s.tenantHandler.GetTeamOverview)
r.Patch("/teams/{teamId}", s.tenantHandler.UpdateTeam)
r.Post("/teams/{teamId}/disable", s.tenantHandler.DisableTeam)
r.Post("/teams/{teamId}/archive", s.tenantHandler.ArchiveTeam)
r.Post("/teams/{teamId}/restore", s.tenantHandler.RestoreTeam)
```

- [ ] **Step 6: Update OpenAPI**

Add schemas:

- `TeamListItem`
- `TeamOverview`
- `AllowedTeamAction`
- `UpdateTeamRequest`

Add the five new path entries and update `GET /api/v1/teams` response item to `TeamListItem`.

- [ ] **Step 7: Run API tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: PASS.

## Task 5: Web API Client

**Files:**
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/lib/api/teams.test.ts`

- [ ] **Step 1: Write failing API client tests**

Add tests for these functions:

```ts
listTeamSummaries({ baseUrl, fetcher }, { status: "active", q: "ops" })
getTeamOverview({ baseUrl, fetcher }, "team-1")
updateTeam({ baseUrl, fetcher }, "team-1", { name: "运维团队", slug: "ops", human_owner_user_id: "user-1" })
disableTeam({ baseUrl, fetcher }, "team-1")
archiveTeam({ baseUrl, fetcher }, "team-1")
restoreTeam({ baseUrl, fetcher }, "team-1")
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/lib/api/teams.test.ts
```

Expected: FAIL because functions do not exist.

- [ ] **Step 3: Add TypeScript types and functions**

Update `TeamStatus`:

```ts
export type TeamStatus = "active" | "disabled" | "archived";
```

Add `AllowedTeamAction`, `TeamListItem`, `TeamOverview`, `UpdateTeamInput`, and functions for the six API operations.

- [ ] **Step 4: Run API client tests**

Run:

```bash
pnpm --filter @superteam/web test src/lib/api/teams.test.ts
```

Expected: PASS.

## Task 6: Web List and Detail Shell

**Files:**
- Modify: `apps/web/src/features/teams/index.tsx`
- Create: `apps/web/src/features/teams/components/team-list-table.tsx`
- Create: `apps/web/src/features/teams/components/team-detail-layout.tsx`
- Create: `apps/web/src/features/teams/components/team-overview-tab.tsx`
- Create: `apps/web/src/routes/_authenticated/teams/$teamId.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing UI tests**

Cover:

- list renders columns `负责人`, `成员`, `数字员工`, `能力`, `治理状态`, `当前版本`, `待批准`.
- detail route renders tabs `概览`, `成员`, `数字员工`, `能力与知识`, `治理策略`, `审计记录`.
- disabled team hides or disables add-member and create-governance actions.

- [ ] **Step 2: Run UI tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because the new layout does not exist.

- [ ] **Step 3: Implement list table**

Use the saved design image as visual reference:

```text
docs/design/teamManager/01-team-overview-action-first.png
```

Keep the table dense and use shadcn primitives already present in the repo. Avoid card-per-row layout.

- [ ] **Step 4: Implement detail shell**

Create a detail route that calls `getTeamOverview`, renders the compact team header, and renders scoped empty-state panels for members, governance, employees, and audit. Each empty-state panel must name the dependent plan that will replace it.

- [ ] **Step 5: Run UI tests**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: PASS.

## Task 7: Verification and Commit

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add under `Unreleased`:

```md
- 新增团队管理控制台基础计划的列表摘要、详情概览、生命周期操作和前端详情框架。
```

- [ ] **Step 2: Run verification**

Run:

```bash
go test ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api
pnpm --filter @superteam/web test src/lib/api/teams.test.ts src/features/teams/index.test.tsx
git diff --check
```

Expected: all commands pass.

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/tenant apps/control-plane/internal/api apps/control-plane/internal/storage/queries contracts/control-plane/openapi.yaml apps/web/src/lib/api/teams.ts apps/web/src/lib/api/teams.test.ts apps/web/src/features/teams apps/web/src/routes/_authenticated/teams CHANGELOG.md
git commit -m "feat: add team management foundation"
```
