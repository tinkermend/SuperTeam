# Team Management Employees and Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the team-scoped digital employees tab, quick create from team, and team-management audit records tab.

**Architecture:** This plan reuses existing digital employee APIs where possible, adding team filters and team detail integration. Team audit reads from existing `audit_events` and limits results to team-management events for the current team.

**Tech Stack:** Go, chi/net/http, pgx/sqlc, PostgreSQL, OpenAPI YAML, React, TanStack Query, shadcn/ui tables, Vitest Browser.

---

## Dependencies and Parallelism

Requires completion of `2026-06-03-team-management-foundation-lifecycle.md`.

Can run in parallel with:

- `2026-06-03-team-management-members-roles.md`
- `2026-06-03-team-management-governance-capabilities.md`

This plan becomes richer after Plans 2 and 3 because their writes produce team audit rows, but the API and UI can be implemented independently with seeded audit data.

## File Structure

- Modify `apps/control-plane/internal/storage/queries/employee_execution.sql`: ensure list digital employees can filter by team.
- Modify `apps/control-plane/internal/employee/service.go` and `handler.go`: preserve existing list API and support `team_id` query param if not already fully wired.
- Modify `apps/control-plane/internal/storage/queries/audit.sql`: add team audit query and regenerate via `cd apps/control-plane && make generate-sqlc`.
- Modify `apps/control-plane/internal/audit/service.go`: add tenant/team-filtered list method.
- Modify `apps/control-plane/internal/tenant/handler.go`: add team audit endpoint or delegate to audit service through tenant handler.
- Modify `apps/control-plane/internal/api/server.go`: register `GET /teams/{teamId}/audit`.
- Modify `apps/control-plane/internal/api/team_routes_test.go` and `employee_routes_test.go`.
- Modify `contracts/control-plane/openapi.yaml`.
- Modify `apps/web/src/lib/api/employees.ts` and tests: support `team_id` filter.
- Modify `apps/web/src/lib/api/teams.ts` and tests: add team audit API.
- Create `apps/web/src/features/teams/components/team-digital-employees-tab.tsx`.
- Create `apps/web/src/features/teams/components/team-audit-tab.tsx`.
- Modify `apps/web/src/features/teams/index.test.tsx`.
- Modify `CHANGELOG.md`.

## Task 1: Team-Filtered Digital Employee List

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Generated: `apps/control-plane/internal/storage/queries/employee_execution.sql.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Test: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`

- [ ] **Step 1: Write failing API tests**

Backend route test:

```go
func TestEmployeeListAcceptsTeamFilter(t *testing.T) {
	teamID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees?team_id="+teamID.String(), nil)
	// assert service.ListDigitalEmployees receives TeamID == teamID
}
```

Frontend API test:

```ts
await listDigitalEmployees({ baseUrl, fetcher }, { team_id: "team-1" });
expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/digital-employees?team_id=team-1", expect.any(Object));
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestEmployeeListAcceptsTeamFilter -count=1
pnpm --filter @superteam/web test src/lib/api/employees.test.ts
```

Expected: FAIL if team filter is not wired end-to-end.

- [ ] **Step 3: Wire team filter**

Ensure handler parses `team_id` query param into `ListDigitalEmployeesRequest.TeamID`. Ensure SQL filter includes:

```sql
AND (sqlc.narg('team_id')::uuid IS NULL OR team_id = sqlc.narg('team_id')::uuid)
```

Update frontend `listDigitalEmployees` signature to accept optional filters.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestEmployeeListAcceptsTeamFilter -count=1
pnpm --filter @superteam/web test src/lib/api/employees.test.ts
```

Expected: PASS.

## Task 2: Team Audit Query and Endpoint

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/audit.sql`
- Generated: `apps/control-plane/internal/storage/queries/audit.sql.go`
- Modify: `apps/control-plane/internal/audit/service.go`
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Test: `apps/control-plane/internal/api/team_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Write failing route test**

Test:

```go
func TestTeamAuditRouteUsesTeamAuditRead(t *testing.T) {
	teamID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID+"/audit", nil)
	// assert authz action is ActionTeamAuditRead and resource is team
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamAuditRouteUsesTeamAuditRead -count=1
```

Expected: FAIL because route does not exist.

- [ ] **Step 3: Add audit query**

Append:

```sql
-- name: ListTeamAuditEvents :many
SELECT *
FROM audit_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND resource_type = 'team'
  AND resource_id = sqlc.arg('team_id')::uuid::text
  AND action LIKE 'team.%'
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

Run:

```bash
cd apps/control-plane && make generate-sqlc
```

- [ ] **Step 4: Add route and OpenAPI**

Register:

```go
r.Get("/teams/{teamId}/audit", s.tenantHandler.ListTeamAudit)
```

Add OpenAPI schema `TeamAuditEvent` and path `GET /api/v1/teams/{teamId}/audit`.

- [ ] **Step 5: Run API test**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamAuditRouteUsesTeamAuditRead -count=1
```

Expected: PASS.

## Task 3: Web Digital Employees Tab

**Files:**
- Create: `apps/web/src/features/teams/components/team-digital-employees-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`
- Modify: `apps/web/src/lib/api/employees.ts`

- [ ] **Step 1: Write failing UI tests**

Cover:

- metrics `数字员工`, `active`, `draft`, `继承配置过期`, `未绑定 Runtime`.
- button `从此团队创建数字员工`.
- create form submits with current `team_id`.
- table shows `生效配置` and `执行实例`.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because tab is not implemented.

- [ ] **Step 3: Implement tab**

Use visual reference:

```text
docs/design/teamManager/03-team-digital-employees.png
```

Call `listDigitalEmployees({ team_id: teamId })`. Use existing `createDigitalEmployee` with `team_id` prefilled for quick create.

- [ ] **Step 4: Run UI tests**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: PASS.

## Task 4: Web Audit Tab

**Files:**
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/lib/api/teams.test.ts`
- Create: `apps/web/src/features/teams/components/team-audit-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing API and UI tests**

API:

```ts
await listTeamAuditEvents({ baseUrl, fetcher }, "team-1", { limit: 20, offset: 0 });
expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams/team-1/audit?limit=20&offset=0", expect.any(Object));
```

UI:

- summary strip renders `今日操作`, `成员变更`, `治理版本`, `能力绑定`, `被拒绝`.
- table renders `授权动作`.
- selected row detail displays `before/after`.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/web test src/lib/api/teams.test.ts src/features/teams/index.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Implement API and tab**

Use visual reference:

```text
docs/design/teamManager/07-team-audit-records.png
```

Limit tab contents to team-management audit events. Do not include task run, provider session, or capability invocation logs.

- [ ] **Step 4: Run tests**

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
- 新增团队详情中的数字员工入口和团队管理审计记录。
```

- [ ] **Step 2: Run verification**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries ./apps/control-plane/internal/employee ./apps/control-plane/internal/audit ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api
pnpm --filter @superteam/web test src/lib/api/employees.test.ts src/lib/api/teams.test.ts src/features/teams/index.test.tsx
git diff --check
```

Expected: all commands pass.

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/storage apps/control-plane/internal/employee apps/control-plane/internal/audit apps/control-plane/internal/tenant apps/control-plane/internal/api contracts/control-plane/openapi.yaml apps/web/src/lib/api apps/web/src/features/teams CHANGELOG.md
git commit -m "feat: add team employees and audit tabs"
```
