# Team Management Governance and Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement governance draft editing, approval, rejection, JSON preview, difference summaries, and Skills/MCP/knowledge/external capability bindings as draft-only changes.

**Architecture:** This plan reuses `tenant_team_config_revisions` as the draft store. Active governance is immutable from the UI; all policy and binding edits produce or update a draft revision, then approval archives the old active revision and activates the draft. Capability bindings live in `capability_policy` JSONB in first version.

**Tech Stack:** Go, chi/net/http, pgx/sqlc, PostgreSQL JSONB, OpenAPI YAML, React forms, TanStack Query, shadcn/ui, Vitest Browser.

---

## Dependencies and Parallelism

Requires completion of `2026-06-03-team-management-foundation-lifecycle.md`.

Can run in parallel with:

- `2026-06-03-team-management-members-roles.md`
- `2026-06-03-team-management-employees-audit.md`

This plan does not add a migration if `tenant_team_config_revisions.status` remains a free `VARCHAR`. It adds status values in application code: `draft`, `active`, `rejected`, `archived`.

## File Structure

- Modify `apps/control-plane/internal/storage/queries/tenant_team_config.sql`: draft list/update/approve/reject queries.
- Regenerate `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go` via `cd apps/control-plane && make generate-sqlc`.
- Modify `apps/control-plane/internal/tenant/types.go`: governance draft and diff summary types.
- Modify `apps/control-plane/internal/tenant/repository.go`: add draft repository methods.
- Modify `apps/control-plane/internal/tenant/pg_repository.go`: map rows.
- Modify `apps/control-plane/internal/tenant/service.go`: implement draft save, validation, approve, reject, and diff summary.
- Modify `apps/control-plane/internal/tenant/service_test.go`: cover governance rules.
- Modify `apps/control-plane/internal/tenant/handler.go`: add governance endpoints.
- Modify `apps/control-plane/internal/api/server.go`: register routes.
- Modify `apps/control-plane/internal/api/team_routes_test.go`: cover route authz mapping.
- Modify `contracts/control-plane/openapi.yaml`: add governance schemas and paths.
- Modify `apps/web/src/lib/api/teams.ts` and `apps/web/src/lib/api/teams.test.ts`: governance API functions.
- Create `apps/web/src/features/teams/components/team-capabilities-tab.tsx`.
- Create `apps/web/src/features/teams/components/team-governance-tab.tsx`.
- Modify `apps/web/src/features/teams/index.test.tsx`.
- Modify `CHANGELOG.md`.

## Task 1: Draft SQL Queries

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
- Generated: `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Write failing query tests**

Add tests:

```go
func TestTeamGovernanceDraftLifecycleQueries(t *testing.T) {
	db := newQueriesTestDB(t)
	q := New(db)
	tenantID := seedTestTenant(t, db)
	teamID := seedTestTeam(t, db, tenantID, "ops", "运维团队")
	active := seedTestTeamConfigRevision(t, db, tenantID, teamID, "active", 1)
	draft := seedTestTeamConfigRevision(t, db, tenantID, teamID, "draft", 2)

	updated, err := q.UpdateTenantTeamConfigRevisionDraft(context.Background(), UpdateTenantTeamConfigRevisionDraftParams{
		ID:               draft.ID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Constitution:     []byte(`{"hard_rules":["禁止未审批生产写操作"]}`),
		CapabilityPolicy: []byte(`{"skill_bindings":["incident-diagnosis"]}`),
	})
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updated.RevisionNumber != 2 {
		t.Fatalf("expected draft revision 2")
	}

	if _, err := q.ArchiveActiveTenantTeamConfigRevision(context.Background(), ArchiveActiveTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	}); err != nil {
		t.Fatalf("archive active: %v", err)
	}
	approverID := seedTestAuthUser(t, db, "approver")
	approved, err := q.ActivateTenantTeamConfigRevision(context.Background(), ActivateTenantTeamConfigRevisionParams{
		ID:         draft.ID,
		TenantID:   tenantID,
		TeamID:     teamID,
		ApprovedBy: approverID,
	})
	if err != nil {
		t.Fatalf("activate draft: %v", err)
	}
	if approved.Status != "active" {
		t.Fatalf("expected active status, got %s", approved.Status)
	}
}
```

- [ ] **Step 2: Run query tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestTeamGovernanceDraftLifecycleQueries -count=1
```

Expected: FAIL because query methods do not exist.

- [ ] **Step 3: Add draft queries**

Append queries:

```sql
-- name: ListTenantTeamConfigDrafts :many
SELECT *
FROM tenant_team_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateTenantTeamConfigRevisionDraft :one
UPDATE tenant_team_config_revisions
SET
  constitution = COALESCE(sqlc.arg('constitution')::jsonb, constitution),
  capability_policy = COALESCE(sqlc.arg('capability_policy')::jsonb, capability_policy),
  context_policy = COALESCE(sqlc.arg('context_policy')::jsonb, context_policy),
  approval_policy = COALESCE(sqlc.arg('approval_policy')::jsonb, approval_policy),
  artifact_contract = COALESCE(sqlc.arg('artifact_contract')::jsonb, artifact_contract),
  internal_collaboration_policy = COALESCE(sqlc.arg('internal_collaboration_policy')::jsonb, internal_collaboration_policy),
  runtime_scope_policy = COALESCE(sqlc.arg('runtime_scope_policy')::jsonb, runtime_scope_policy),
  human_owner_user_id = sqlc.narg('human_owner_user_id')::uuid
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveActiveTenantTeamConfigRevision :many
UPDATE tenant_team_config_revisions
SET status = 'archived',
    archived_at = COALESCE(archived_at, NOW())
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
RETURNING *;

-- name: ActivateTenantTeamConfigRevision :one
UPDATE tenant_team_config_revisions
SET status = 'active',
    approved_by = sqlc.arg('approved_by')::uuid,
    approved_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: RejectTenantTeamConfigRevision :one
UPDATE tenant_team_config_revisions
SET status = 'rejected',
    archived_at = COALESCE(archived_at, NOW())
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;
```

- [ ] **Step 4: Regenerate and run query tests**

Run:

```bash
cd apps/control-plane && make generate-sqlc
go test ./apps/control-plane/internal/storage/queries -run TestTeamGovernanceDraftLifecycleQueries -count=1
```

Expected: PASS.

## Task 2: Governance Service Logic

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`
- Modify: `apps/control-plane/internal/tenant/repository.go`
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`
- Modify: `apps/control-plane/internal/tenant/service.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests:

```go
func TestApproveGovernanceDraftArchivesPreviousActive(t *testing.T) { /* active v7 archived, draft v8 active */ }
func TestApproveGovernanceDraftRejectsMissingHardRules(t *testing.T) { /* empty constitution.hard_rules returns ErrInvalidInput */ }
func TestUpdateGovernanceDraftStoresCapabilityBindings(t *testing.T) { /* skill/mcp/kb/external bindings remain in capability_policy */ }
```

- [ ] **Step 2: Run service tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'Test(ApproveGovernance|UpdateGovernance)' -count=1
```

Expected: FAIL because methods do not exist.

- [ ] **Step 3: Add service types**

Add:

```go
type GovernanceDraftInput struct {
	Constitution                map[string]any
	CapabilityPolicy            map[string]any
	ContextPolicy               map[string]any
	ApprovalPolicy              map[string]any
	ArtifactContract            map[string]any
	InternalCollaborationPolicy map[string]any
	RuntimeScopePolicy          map[string]any
	HumanOwnerUserID            *uuid.UUID
}

type GovernanceDiffSummary struct {
	AddedHardRules       int32
	ChangedCapabilities  int32
	ChangedApprovalRules int32
	Warnings             []ValidationIssue
	BlockingErrors       []ValidationIssue
}
```

Use the existing validation issue shape from employee if importing is not appropriate; keep it in tenant package to avoid cross-module coupling.

- [ ] **Step 4: Implement service methods**

Methods:

```go
ListGovernanceDrafts(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamConfigRevision, error)
CreateGovernanceDraft(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error)
UpdateGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID, input GovernanceDraftInput) (*TeamConfigRevision, error)
ApproveGovernanceDraft(ctx context.Context, tenantID, teamID, draftID, approvedBy uuid.UUID) (*TeamConfigRevision, error)
RejectGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*TeamConfigRevision, error)
PreviewGovernanceDiff(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*GovernanceDiffSummary, error)
```

Approval validation:

- `constitution.hard_rules` must be an array with at least one non-empty string.
- capability binding arrays must contain only strings.
- draft must belong to the same tenant and team.
- disabled or archived team must not approve new governance.

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/tenant -count=1
```

Expected: PASS.

## Task 3: HTTP Routes and OpenAPI

**Files:**
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/team_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write failing route tests**

Cover:

- `GET /teams/{teamId}/governance/current` uses `team.governance.read`.
- `GET /teams/{teamId}/governance/drafts` uses `team.governance.read`.
- draft create/update use `team.governance.edit`.
- approve/reject use `team.governance.approve`.

- [ ] **Step 2: Run route tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: FAIL because routes do not exist.

- [ ] **Step 3: Register routes**

```go
r.Get("/teams/{teamId}/governance/current", s.tenantHandler.GetCurrentTeamConfigRevision)
r.Get("/teams/{teamId}/governance/drafts", s.tenantHandler.ListGovernanceDrafts)
r.Post("/teams/{teamId}/governance/drafts", s.tenantHandler.CreateGovernanceDraft)
r.Patch("/teams/{teamId}/governance/drafts/{draftId}", s.tenantHandler.UpdateGovernanceDraft)
r.Post("/teams/{teamId}/governance/drafts/{draftId}/approve", s.tenantHandler.ApproveGovernanceDraft)
r.Post("/teams/{teamId}/governance/drafts/{draftId}/reject", s.tenantHandler.RejectGovernanceDraft)
```

Keep existing `/config-revisions/current` route as backward-compatible alias until all Web code migrates.

- [ ] **Step 4: Update OpenAPI**

Add `GovernanceDraftInput`, `GovernanceDiffSummary`, and draft path definitions.

- [ ] **Step 5: Run API tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: PASS.

## Task 4: Web Capabilities and Governance Tabs

**Files:**
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/lib/api/teams.test.ts`
- Create: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
- Create: `apps/web/src/features/teams/components/team-governance-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing UI tests**

Cover:

- capabilities tab shows Skills, MCP, 知识库, 外部能力 sections.
- binding action displays “绑定不会立即生效”.
- governance tab shows form sections and JSON preview.
- save draft calls draft endpoint, approve calls approve endpoint.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because the functional capabilities and governance tabs are not implemented.

- [ ] **Step 3: Add API functions**

Add:

```ts
getCurrentTeamGovernance(options, teamId)
listTeamGovernanceDrafts(options, teamId)
createTeamGovernanceDraft(options, teamId, input)
updateTeamGovernanceDraft(options, teamId, draftId, input)
approveTeamGovernanceDraft(options, teamId, draftId)
rejectTeamGovernanceDraft(options, teamId, draftId)
```

- [ ] **Step 4: Implement UI tabs**

Use references:

```text
docs/design/teamManager/04-team-capabilities-knowledge.png
docs/design/teamManager/05-team-governance-strategy.png
```

Capabilities tab writes binding changes into local draft state and then saves via governance draft API. Governance tab renders editable structured fields and a read-only JSON preview.

- [ ] **Step 5: Run frontend tests**

Run:

```bash
pnpm --filter @superteam/web test src/lib/api/teams.test.ts src/features/teams/index.test.tsx
```

Expected: PASS.

## Task 5: Verification and Commit

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

```md
- 新增团队治理草稿、能力与知识绑定、治理策略编辑和批准生效流程。
```

- [ ] **Step 2: Run verification**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api
pnpm --filter @superteam/web test src/lib/api/teams.test.ts src/features/teams/index.test.tsx
git diff --check
```

Expected: all commands pass.

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/storage apps/control-plane/internal/tenant apps/control-plane/internal/api contracts/control-plane/openapi.yaml apps/web/src/lib/api/teams.ts apps/web/src/lib/api/teams.test.ts apps/web/src/features/teams CHANGELOG.md
git commit -m "feat: add team governance draft workflow"
```
