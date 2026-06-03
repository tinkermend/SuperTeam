# Team Management Members and Roles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement team-scoped human member management, ordinary role changes, privileged role requests, and the members tab UI.

**Architecture:** This plan uses the existing `tenant_members` table for team membership; `team_id IS NULL` remains tenant-level membership, and `team_id IS NOT NULL` is team-level membership. A new request table records privileged role change workflow. The Web members tab mirrors the selected roster-centered design.

**Tech Stack:** Go, chi/net/http, pgx/sqlc, PostgreSQL, OpenAPI YAML, React, TanStack Query, shadcn/ui table controls, Vitest Browser.

---

## Dependencies and Parallelism

Requires completion of `2026-06-03-team-management-foundation-lifecycle.md`.

Can run in parallel with:

- `2026-06-03-team-management-governance-capabilities.md`
- `2026-06-03-team-management-employees-audit.md`

Avoid editing lifecycle endpoints from Plan 1 while this plan is in progress.

## File Structure

- Modify `apps/control-plane/internal/storage/migrations/004_team_member_role_requests.sql`: create privileged role request table and indexes.
- Modify `apps/control-plane/internal/storage/queries/tenant_team_config.sql`: add member and role request sqlc queries.
- Regenerate `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`.
- Modify `apps/control-plane/internal/tenant/types.go`: add team member and role request types.
- Modify `apps/control-plane/internal/tenant/repository.go`: add member repository methods.
- Modify `apps/control-plane/internal/tenant/pg_repository.go`: map sqlc rows.
- Modify `apps/control-plane/internal/tenant/service.go`: implement member operations and last-owner protection.
- Modify `apps/control-plane/internal/tenant/service_test.go`: add member and request tests.
- Modify `apps/control-plane/internal/tenant/handler.go`: add member endpoints.
- Modify `apps/control-plane/internal/api/server.go`: register member routes.
- Modify `apps/control-plane/internal/api/team_routes_test.go`: cover action mapping.
- Modify `contracts/control-plane/openapi.yaml`: add schemas and paths.
- Modify `apps/web/src/lib/api/teams.ts` and `apps/web/src/lib/api/teams.test.ts`: add client APIs.
- Create `apps/web/src/features/teams/components/team-members-tab.tsx`: roster table and right rail.
- Modify `apps/web/src/features/teams/index.test.tsx`: cover members tab behavior.
- Modify `CHANGELOG.md`.

## Task 1: Privileged Role Request Migration

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/004_team_member_role_requests.sql`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Write failing storage test**

Add a test that creates a role request and expects it to be listed by team:

```go
func TestTeamMemberRoleRequestQueries(t *testing.T) {
	db := newQueriesTestDB(t)
	q := New(db)
	tenantID := seedTestTenant(t, db)
	teamID := seedTestTeam(t, db, tenantID, "ops", "运维团队")
	requesterID := seedTestAuthUser(t, db, "requester")
	targetID := seedTestAuthUser(t, db, "target")

	created, err := q.CreateTeamMemberRoleRequest(context.Background(), CreateTeamMemberRoleRequestParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		TargetUserID:  targetID,
		RequestedRole: "admin",
		RequestedBy:   requesterID,
		Reason:        "需要维护成员",
	})
	if err != nil {
		t.Fatalf("create role request: %v", err)
	}
	rows, err := q.ListTeamMemberRoleRequests(context.Background(), ListTeamMemberRoleRequestsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Limit:    20,
		Offset:   0,
	})
	if err != nil {
		t.Fatalf("list role requests: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != created.ID {
		t.Fatalf("expected created request in list")
	}
}
```

- [ ] **Step 2: Run storage test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestTeamMemberRoleRequestQueries -count=1
```

Expected: FAIL because the table and queries do not exist.

- [ ] **Step 3: Create migration**

Create `004_team_member_role_requests.sql`:

```sql
CREATE TABLE IF NOT EXISTS tenant_team_member_role_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    target_user_id UUID NOT NULL,
    requested_role VARCHAR(100) NOT NULL,
    requested_by UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    reason TEXT NOT NULL DEFAULT '',
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    decision_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_team_member_role_requests_team
    ON tenant_team_member_role_requests(tenant_id, team_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_tenant_team_member_role_requests_target
    ON tenant_team_member_role_requests(tenant_id, target_user_id, created_at DESC);

COMMENT ON TABLE tenant_team_member_role_requests IS '团队高权限角色变更申请表';
COMMENT ON COLUMN tenant_team_member_role_requests.id IS '角色变更申请ID';
COMMENT ON COLUMN tenant_team_member_role_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.team_id IS '团队ID';
COMMENT ON COLUMN tenant_team_member_role_requests.target_user_id IS '目标用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.requested_role IS '申请授予的团队角色';
COMMENT ON COLUMN tenant_team_member_role_requests.requested_by IS '申请人用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.status IS '申请状态：pending、approved、rejected';
COMMENT ON COLUMN tenant_team_member_role_requests.reason IS '申请原因';
COMMENT ON COLUMN tenant_team_member_role_requests.decided_by IS '审批人用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.decided_at IS '审批时间';
COMMENT ON COLUMN tenant_team_member_role_requests.decision_reason IS '审批说明';
COMMENT ON COLUMN tenant_team_member_role_requests.created_at IS '创建时间';
COMMENT ON COLUMN tenant_team_member_role_requests.updated_at IS '更新时间';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_tenant_team_member_role_requests_updated_at'
    ) THEN
        CREATE TRIGGER update_tenant_team_member_role_requests_updated_at
        BEFORE UPDATE ON tenant_team_member_role_requests
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;
```

- [ ] **Step 4: Add sqlc queries and regenerate**

Append query names:

```sql
-- name: ListTeamMembers :many
SELECT
  tm.id AS membership_id,
  tm.tenant_id,
  tm.team_id,
  tm.principal_id AS user_id,
  au.username,
  au.display_name,
  au.email,
  au.status AS account_status,
  tm.role,
  tm.status AS membership_status,
  tm.disabled_at,
  tm.created_at,
  tm.updated_at
FROM tenant_members tm
JOIN auth_users au ON au.id = tm.principal_id
WHERE tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.team_id = sqlc.arg('team_id')::uuid
  AND tm.principal_type = 'user'
  AND tm.disabled_at IS NULL
  AND au.deleted_at IS NULL
ORDER BY
  CASE tm.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 WHEN 'approver' THEN 3 WHEN 'member' THEN 4 WHEN 'viewer' THEN 5 ELSE 6 END,
  au.display_name NULLS LAST,
  au.username
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: AddTeamMember :one
INSERT INTO tenant_members (tenant_id, team_id, principal_type, principal_id, role, status)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('team_id')::uuid, 'user', sqlc.arg('user_id')::uuid, sqlc.arg('role')::varchar, 'active')
ON CONFLICT (tenant_id, team_id, principal_type, principal_id, role)
DO UPDATE SET status = 'active', disabled_at = NULL
RETURNING *;

-- name: DisableTeamMemberRole :one
UPDATE tenant_members
SET disabled_at = COALESCE(disabled_at, NOW()), status = 'disabled'
WHERE id = sqlc.arg('membership_id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
RETURNING *;

-- name: CountTeamOwners :one
SELECT COUNT(*)::integer
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = 'user'
  AND role = 'owner'
  AND status = 'active'
  AND disabled_at IS NULL;

-- name: CreateTeamMemberRoleRequest :one
INSERT INTO tenant_team_member_role_requests (tenant_id, team_id, target_user_id, requested_role, requested_by, reason)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('team_id')::uuid, sqlc.arg('target_user_id')::uuid, sqlc.arg('requested_role')::varchar, sqlc.arg('requested_by')::uuid, sqlc.arg('reason')::text)
RETURNING *;

-- name: ListTeamMemberRoleRequests :many
SELECT *
FROM tenant_team_member_role_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: DecideTeamMemberRoleRequest :one
UPDATE tenant_team_member_role_requests
SET status = sqlc.arg('status')::varchar,
    decided_by = sqlc.arg('decided_by')::uuid,
    decided_at = NOW(),
    decision_reason = sqlc.arg('decision_reason')::text
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND status = 'pending'
RETURNING *;
```

Run:

```bash
cd apps/control-plane && make generate-sqlc
```

Expected: generated Go includes member and role request methods.

- [ ] **Step 5: Run storage test**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run TestTeamMemberRoleRequestQueries -count=1
```

Expected: PASS.

## Task 2: Service Rules for Members and Privileged Requests

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`
- Modify: `apps/control-plane/internal/tenant/repository.go`
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`
- Modify: `apps/control-plane/internal/tenant/service.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests:

```go
func TestAddTeamMemberRejectsPrivilegedRole(t *testing.T) { /* role owner/admin/approver returns ErrInvalidInput */ }
func TestRemoveTeamMemberRejectsLastOwner(t *testing.T) { /* owner count 1 and target role owner returns ErrInvalidInput */ }
func TestApprovePrivilegedRoleRequestAddsRole(t *testing.T) { /* approving admin request calls AddTeamMember with admin */ }
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'Test(AddTeamMember|RemoveTeamMember|ApprovePrivileged)' -count=1
```

Expected: FAIL because methods do not exist.

- [ ] **Step 3: Add domain types**

Add roles and request structs:

```go
const (
	TeamRoleOwner    = "owner"
	TeamRoleAdmin    = "admin"
	TeamRoleApprover = "approver"
	TeamRoleMember   = "member"
	TeamRoleViewer   = "viewer"
)

type TeamMember struct {
	MembershipID     uuid.UUID
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	UserID           uuid.UUID
	Username         string
	DisplayName      string
	Email            string
	AccountStatus    string
	Role             string
	MembershipStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AddTeamMemberRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	UserID   uuid.UUID
	Role     string
}

type RemoveTeamMemberRequest struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	MembershipID uuid.UUID
}
```

Add `TeamMemberRoleRequest` and approve/reject request types.

- [ ] **Step 4: Implement service methods**

Methods:

```go
ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamMember, error)
AddTeamMember(ctx context.Context, req AddTeamMemberRequest) (*TeamMember, error)
RemoveTeamMember(ctx context.Context, req RemoveTeamMemberRequest) error
CreateRoleRequest(ctx context.Context, req CreateRoleRequestRequest) (*TeamMemberRoleRequest, error)
ApproveRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error)
RejectRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error)
```

Rules:

- Direct add/change permits only `member` and `viewer`.
- `owner`, `admin`, and `approver` require role request approval.
- Removing the final active owner returns `ErrInvalidInput`.
- Approving a role request adds the requested role through repository and marks the request approved.

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/tenant -count=1
```

Expected: PASS.

## Task 3: HTTP Endpoints and Authz Mapping

**Files:**
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/team_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write failing route tests**

Add route tests for:

- `GET /teams/{teamId}/members` uses `team.read`.
- `POST /teams/{teamId}/members` uses `team.member.add` with `Context["target_role"]`.
- `DELETE /teams/{teamId}/members/{memberId}` uses `team.member.remove`.
- `POST /teams/{teamId}/member-role-requests` uses `team.member.request_privileged_role`.
- approve/reject endpoints use `team.member.approve_privileged_role`.

- [ ] **Step 2: Run route tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: FAIL because endpoints do not exist.

- [ ] **Step 3: Implement handlers and routes**

Register:

```go
r.Get("/teams/{teamId}/members", s.tenantHandler.ListTeamMembers)
r.Post("/teams/{teamId}/members", s.tenantHandler.AddTeamMember)
r.Delete("/teams/{teamId}/members/{memberId}", s.tenantHandler.RemoveTeamMember)
r.Post("/teams/{teamId}/member-role-requests", s.tenantHandler.CreateTeamMemberRoleRequest)
r.Post("/teams/{teamId}/member-role-requests/{requestId}/approve", s.tenantHandler.ApproveTeamMemberRoleRequest)
r.Post("/teams/{teamId}/member-role-requests/{requestId}/reject", s.tenantHandler.RejectTeamMemberRoleRequest)
```

For direct member add, pass authorization context:

```go
Context: map[string]any{"target_role": req.Role}
```

- [ ] **Step 4: Update OpenAPI**

Add schemas:

- `TeamMember`
- `AddTeamMemberRequest`
- `TeamMemberRoleRequest`
- `CreateTeamMemberRoleRequest`
- `DecideTeamMemberRoleRequest`

- [ ] **Step 5: Run API tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutes -count=1
```

Expected: PASS.

## Task 4: Web Members Tab

**Files:**
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/lib/api/teams.test.ts`
- Create: `apps/web/src/features/teams/components/team-members-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing frontend tests**

Cover:

- renders counters `人类成员`, `负责人`, `管理员`, `审批人`, `直接生效角色`.
- direct add form only offers `普通成员` and `只读观察者`.
- pending privileged request cards show `拒绝` and `审批`.
- last-owner protection text is visible.

- [ ] **Step 2: Run frontend tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because members tab is not implemented.

- [ ] **Step 3: Add API functions**

Add:

```ts
listTeamMembers(options, teamId)
addTeamMember(options, teamId, input)
removeTeamMember(options, teamId, memberId)
createTeamMemberRoleRequest(options, teamId, input)
approveTeamMemberRoleRequest(options, teamId, requestId, input)
rejectTeamMemberRoleRequest(options, teamId, requestId, input)
```

- [ ] **Step 4: Implement members tab**

Use visual reference:

```text
docs/design/teamManager/02-team-members-roster.png
```

Render a grouped table by role and a right rail for privileged requests and final owner protection.

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
- 新增团队成员管理、普通角色直接变更和高权限角色申请审批流程。
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
git commit -m "feat: add team member role management"
```
