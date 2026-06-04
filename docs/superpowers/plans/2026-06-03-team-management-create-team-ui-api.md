# Team Management Create Team UI API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the real Team Management list and right-side two-step Create Team flow backed by Control Plane APIs and the live database.

**Architecture:** Backend owns filtering, validation, authorization, and the atomic team-plus-members transaction. Frontend owns query state, high-density list presentation, and a two-step drawer that only submits real API payloads. The feature stays inside the existing tenant/team and web feature boundaries.

**Tech Stack:** Go `net/http`/chi, pgx/sqlc/Atlas, OpenAPI, React, TanStack Query, shadcn/ui/Radix, Tailwind CSS, Vitest, Playwright.

---

## Scope Check

This plan covers one coherent vertical slice: team list filtering, create-team transaction, API contract, frontend list UI, create-team drawer, and real smoke verification. It does not implement the broader team detail Tab redesign, governance editor redesign, user invitation, or team hard-delete.

## File Structure

- Modify `apps/control-plane/internal/auth/types.go`: add `Q` to `ListUsersFilter`.
- Modify `apps/control-plane/internal/auth/service.go`: normalize the user search query.
- Modify `apps/control-plane/internal/auth/handler.go`: bind `q` from `/api/auth/users`.
- Modify `apps/control-plane/internal/auth/pg_repository.go`: pass `q` into sqlc.
- Modify `apps/control-plane/internal/storage/queries/auth.sql`: add `q` filtering to `ListUsers`.
- Regenerate `apps/control-plane/internal/storage/queries/auth.sql.go` with `make -C apps/control-plane generate-sqlc`.
- Regenerate `apps/control-plane/internal/auth/generated.go` with `make -C apps/control-plane generate-openapi` after updating `contracts/control-plane/auth.yaml`; the oapi-codegen generated `ListUsersParams` will then include the `Q` field.
- Modify `apps/control-plane/internal/tenant/types.go`: add `InitialTeamMemberInput`, `CreateTeamRequest.InitialMembers`, and membership validation helpers.
- Modify `apps/control-plane/internal/tenant/repository.go`: add `CreateTeamWithInitialMembers`.
- Modify `apps/control-plane/internal/tenant/pg_repository.go`: implement the atomic transaction.
- Modify `apps/control-plane/internal/storage/queries/tenant_team_config.sql`: add active same-tenant user lookup and owner/member insert queries against `tenant_members`.
- Regenerate `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`.
- Modify `apps/control-plane/internal/tenant/service.go`: validate atomic create input and return `TeamOverview`.
- Modify `apps/control-plane/internal/tenant/handler.go`: decode `initial_members`, return `TeamOverview`, and keep backend authorization.
- Modify `apps/control-plane/internal/tenant/types.go` and repository params: carry the current actor user ID into team creation so audit events can be written inside the same repository transaction.
- Modify `apps/control-plane/internal/tenant/pg_repository.go`: write `team.create` and `team.member.add` audit events inside the same transaction that creates the team and memberships.
- Modify `apps/control-plane/internal/api/team_routes_test.go`: route and response coverage.
- Modify `apps/control-plane/internal/tenant/service_test.go`: domain validation coverage.
- Modify `contracts/control-plane/openapi.yaml`: document create payload, overview response, and team list filters.
- Modify `apps/web/src/lib/api/auth.ts` and `apps/web/src/lib/api/auth.test.ts`: add `q` to `listUsers`.
- Modify `apps/web/src/lib/api/teams.ts` and `apps/web/src/lib/api/teams.test.ts`: add `initial_members`, `allowed_actions`, pagination filters, and `TeamOverview` create response.
- Create `apps/web/src/features/teams/components/team-management-toolbar.tsx`: list filters.
- Modify `apps/web/src/features/teams/components/team-list-table.tsx`: high-density visual table, row actions, owner display, highlighted row.
- Create `apps/web/src/features/teams/components/create-team-drawer.tsx`: drawer shell and step controller.
- Create `apps/web/src/features/teams/components/create-team-basic-step.tsx`: name, slug, owner search.
- Create `apps/web/src/features/teams/components/create-team-members-step.tsx`: create a minimal stub in Task 5 so typecheck passes, then replace it with user search, role assignment, and selected members in Task 6.
- Modify `apps/web/src/features/teams/index.tsx`: query filters, drawer mutation, success refresh, highlight.
- Modify `apps/web/src/features/teams/index.test.tsx`: frontend workflow coverage.
- Modify `CHANGELOG.md`: add a timestamped Unreleased entry after implementation.
- Create smoke artifacts under `.scratch/team-ui-audit/` during execution; do not commit them.

## Task 1: User Search API

**Files:**
- Modify: `apps/control-plane/internal/auth/types.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Modify: `apps/control-plane/internal/auth/handler.go`
- Modify: `apps/control-plane/internal/auth/pg_repository.go`
- Modify: `apps/control-plane/internal/storage/queries/auth.sql`
- Modify after generation: `apps/control-plane/internal/storage/queries/auth.sql.go`
- Modify: `contracts/control-plane/auth.yaml`
- Modify after generation: `apps/control-plane/internal/auth/generated.go`
- Test: `apps/control-plane/internal/auth/service_test.go`
- Test: `apps/control-plane/internal/api/routes_test.go`
- Test: `apps/web/src/lib/api/auth.test.ts`
- Modify: `apps/web/src/lib/api/auth.ts`

- [ ] **Step 1: Write failing auth service test**

Add this test to `apps/control-plane/internal/auth/service_test.go`:

```go
func TestListUsersNormalizesSearchQuery(t *testing.T) {
	repo := &mockRepo{}
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.ListUsers(context.Background(), ListUsersFilter{
		Q:      "  owner@example.com  ",
		Status: UserStatusActive,
		Limit:  200,
		Offset: -5,
	}); err != nil {
		t.Fatalf("list users: %v", err)
	}

	if repo.lastListUsersFilter.Q != "owner@example.com" {
		t.Fatalf("expected trimmed query, got %q", repo.lastListUsersFilter.Q)
	}
	if repo.lastListUsersFilter.Status != UserStatusActive {
		t.Fatalf("expected active status filter, got %q", repo.lastListUsersFilter.Status)
	}
	if repo.lastListUsersFilter.Limit != 20 || repo.lastListUsersFilter.Offset != 0 {
		t.Fatalf("expected normalized pagination 20/0, got %d/%d", repo.lastListUsersFilter.Limit, repo.lastListUsersFilter.Offset)
	}
}
```

Add this field and assignment to the auth test fake so the assertion can read the normalized filter:

```go
lastListUsersFilter ListUsersFilter
```

```go
m.lastListUsersFilter = filter
```

- [ ] **Step 2: Run service test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/auth -run TestListUsersNormalizesSearchQuery -count=1
```

Expected: FAIL because `ListUsersFilter` has no `Q` field.

- [ ] **Step 3: Add backend search filter plumbing**

In `apps/control-plane/internal/auth/types.go`, change `ListUsersFilter` to:

```go
type ListUsersFilter struct {
	Q      string
	Status string
	Limit  int32
	Offset int32
}
```

In `apps/control-plane/internal/auth/service.go`, import `strings` and normalize `Q`:

```go
func (s *Service) ListUsers(ctx context.Context, filter ListUsersFilter) ([]*User, error) {
	filter.Q = strings.TrimSpace(filter.Q)
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListUsers(ctx, filter)
}
```

In `apps/control-plane/internal/auth/handler.go`, update `ListUsers` to read the generated `params.Q`:

```go
q := ""
if params.Q != nil {
    q = *params.Q
}
users, err := h.service.ListUsers(r.Context(), ListUsersFilter{
    Q:      q,
    Status: status,
    Limit:  valueOrDefault(params.Limit, 20),
    Offset: valueOrDefault(params.Offset, 0),
})
```

In `apps/control-plane/internal/storage/queries/auth.sql`, change `ListUsers` to:

```sql
-- name: ListUsers :many
SELECT * FROM auth_users
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (
    sqlc.narg('q')::text IS NULL
    OR username ILIKE '%' || sqlc.narg('q')::text || '%'
    OR COALESCE(display_name, '') ILIKE '%' || sqlc.narg('q')::text || '%'
    OR COALESCE(email, '') ILIKE '%' || sqlc.narg('q')::text || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

In `apps/control-plane/internal/auth/pg_repository.go`, pass `Q`:

```go
rows, err := r.q.ListUsers(ctx, queries.ListUsersParams{
	Q:      pgtype.Text{String: filter.Q, Valid: filter.Q != ""},
	Status: pgtype.Text{String: filter.Status, Valid: filter.Status != ""},
	Limit:  filter.Limit,
	Offset: filter.Offset,
})
```

- [ ] **Step 4: Regenerate sqlc and update the public auth contract**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: sqlc regeneration succeeds.

Update `contracts/control-plane/auth.yaml` — add an optional `q` query parameter to `GET /api/auth/users`:

```yaml
- name: q
  in: query
  required: false
  schema:
    type: string
    minLength: 1
  description: 按用户名、显示名称或邮箱搜索
```

Regenerate the auth OpenAPI code:

```bash
make -C apps/control-plane generate-openapi
```

Expected: `apps/control-plane/internal/auth/generated.go` now has `Q *string` in `ListUsersParams`. The handler will read `params.Q` instead of bypassing the generated types.

- [ ] **Step 5: Add route coverage for `q`**

In `apps/control-plane/internal/api/routes_test.go`, update the existing `/api/auth/users` route test to request:

```go
listReq := httptest.NewRequest(http.MethodGet, "/api/auth/users?q=operator&status=active&limit=10&offset=0", nil)
```

Assert the route fake receives `Q == "operator"`:

```go
if repo.lastListUsersFilter.Q != "operator" || repo.lastListUsersFilter.Status != auth.UserStatusActive {
	t.Fatalf("expected user list q/status operator/active, got %#v", repo.lastListUsersFilter)
}
```

- [ ] **Step 6: Add frontend API test for `q`**

In `apps/web/src/lib/api/auth.test.ts`, add:

```ts
it("lists users with search query and active filter", async () => {
  const fetcher = vi.fn(async () =>
    new Response(JSON.stringify({ items: [] }), {
      headers: { "content-type": "application/json" },
      status: 200,
    }),
  );

  await listUsers({
    baseUrl: "http://control-plane.local",
    fetcher,
    limit: 20,
    offset: 0,
    q: "owner",
    status: "active",
  });

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/auth/users?q=owner&status=active&limit=20&offset=0",
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );
});
```

- [ ] **Step 7: Implement frontend `q` option**

In `apps/web/src/lib/api/auth.ts`, update `ListUsersOptions`:

```ts
export type ListUsersOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
  q?: string;
  status?: UserSummary["status"];
};
```

Update `listUsers` before `status`:

```ts
const q = options.q?.trim();
if (q) {
  params.set("q", q);
}
```

- [ ] **Step 8: Verify Task 1**

Run:

```bash
go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/api -run 'TestListUsers|TestAuth|TestRoutes|TestTeamRoutes' -count=1
pnpm --filter @superteam/web test src/lib/api/auth.test.ts
```

Expected: all targeted tests pass.

- [ ] **Step 9: Commit Task 1**

```bash
git add apps/control-plane/internal/auth apps/control-plane/internal/storage/queries/auth.sql apps/control-plane/internal/storage/queries/auth.sql.go apps/control-plane/internal/api/routes_test.go apps/web/src/lib/api/auth.ts apps/web/src/lib/api/auth.test.ts contracts/control-plane/auth.yaml
git commit -m "feat: add searchable active user listing"
```

## Task 2: Atomic Create Team Domain And Repository

**Files:**
- Modify: `apps/control-plane/internal/tenant/types.go`
- Modify: `apps/control-plane/internal/tenant/repository.go`
- Modify: `apps/control-plane/internal/tenant/pg_repository.go`
- Modify: `apps/control-plane/internal/storage/queries/tenant_team_config.sql`
- Modify after generation: `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`
- Test: `apps/control-plane/internal/tenant/service_test.go`

- [ ] **Step 1: Write failing domain tests**

Add tests to `apps/control-plane/internal/tenant/service_test.go`:

```go
func TestCreateTeamCreatesOwnerAndInitialMembers(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	memberID := uuid.New()
	viewerID := uuid.New()
	repo.activeUsers[ownerID] = true
	repo.activeUsers[memberID] = true
	repo.activeUsers[viewerID] = true

	overview, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Slug:             "security",
		Name:             "安全团队",
		HumanOwnerUserID: &ownerID,
		InitialMembers: []InitialTeamMemberInput{
			{UserID: memberID, Role: TeamRoleMember},
			{UserID: viewerID, Role: TeamRoleViewer},
		},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if overview.Team == nil || overview.Team.Slug != "security" {
		t.Fatalf("expected created overview team, got %#v", overview.Team)
	}
	if repo.createdTeamWithMembers.OwnerUserID != ownerID {
		t.Fatalf("expected owner %s, got %s", ownerID, repo.createdTeamWithMembers.OwnerUserID)
	}
	if got := repo.createdTeamWithMembers.InitialMembers; !reflect.DeepEqual(got, []InitialTeamMemberInput{
		{UserID: memberID, Role: TeamRoleMember},
		{UserID: viewerID, Role: TeamRoleViewer},
	}) {
		t.Fatalf("expected initial members preserved, got %#v", got)
	}
}

func TestCreateTeamRejectsPrivilegedInitialMemberRoles(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()
	repo.activeUsers[ownerID] = true
	repo.activeUsers[targetID] = true

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         uuid.New(),
		ActorUserID:      actorID,
		Slug:             "security",
		Name:             "安全团队",
		HumanOwnerUserID: &ownerID,
		InitialMembers:  []InitialTeamMemberInput{{UserID: targetID, Role: TeamRoleAdmin}},
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for admin initial member, got %v", err)
	}
	if repo.createTeamWithMembersCalled {
		t.Fatalf("expected invalid request not to reach repository")
	}
}

func TestCreateTeamRejectsOwnerDuplicatedAsInitialMember(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	actorID := uuid.New()
	repo.activeUsers[ownerID] = true

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         uuid.New(),
		ActorUserID:      actorID,
		Slug:             "security",
		Name:             "安全团队",
		HumanOwnerUserID: &ownerID,
		InitialMembers:  []InitialTeamMemberInput{{UserID: ownerID, Role: TeamRoleMember}},
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for duplicated owner, got %v", err)
	}
}
```

Extend `memoryRepository` with `activeUsers`, `CreateTeamWithInitialMembers`, and enough `GetTeamSummary` behavior for the tests. The key requirement: `CreateTeamWithInitialMembers` must write the created team into `memoryRepository.teams` and the owner/member memberships into the existing `memoryRepository.teamMembers` map so that the subsequent `GetOverview` call can read them back:

```go
activeUsers                 map[uuid.UUID]bool
createTeamWithMembersCalled bool
createdTeamWithMembers      CreateTeamWithInitialMembersParams
```

Initialize `activeUsers` in `newMemoryRepository`:

```go
activeUsers: map[uuid.UUID]bool{},
```

```go
func (m *memoryRepository) CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error) {
	m.createTeamWithMembersCalled = true
	m.createdTeamWithMembers = params
	if !m.activeUsers[params.OwnerUserID] {
		return TeamRecord{}, ErrNotFound
	}
	for _, mbr := range params.InitialMembers {
		if !m.activeUsers[mbr.UserID] {
			return TeamRecord{}, ErrNotFound
		}
	}
	// store team so GetTeamSummary can find it
	team := TeamRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		Slug:             params.Slug,
		Name:             params.Name,
		Status:           params.Status,
		HumanOwnerUserID: &params.OwnerUserID,
		Metadata:         params.Metadata,
	}
	m.teams[team.ID] = team
	// store owner membership so summary aggregation picks it up
	ownerMembership := TeamMemberRecord{
		MembershipID: uuid.New(),
		TenantID:     params.TenantID,
		TeamID:       team.ID,
		UserID:       params.OwnerUserID,
		Role:         TeamRoleOwner,
		MembershipStatus: "active",
	}
	m.teamMembers[ownerMembership.MembershipID] = ownerMembership
	// store initial members
	for _, mbr := range params.InitialMembers {
		membership := TeamMemberRecord{
			MembershipID: uuid.New(),
			TenantID:     params.TenantID,
			TeamID:       team.ID,
			UserID:       mbr.UserID,
			Role:         mbr.Role,
			MembershipStatus: "active",
		}
		m.teamMembers[membership.MembershipID] = membership
	}
	return team, nil
}
```

Also update `GetTeamSummary` in `memoryRepository` so it returns a valid `TeamListItemRecord` for the stored team with `MemberCount` set to the number of active entries in `teamMembers` for that `tenantID/teamID`.

- [ ] **Step 2: Run domain tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'TestCreateTeamCreatesOwnerAndInitialMembers|TestCreateTeamRejects' -count=1
```

Expected: FAIL because `InitialTeamMemberInput` and `CreateTeamWithInitialMembers` do not exist.

- [ ] **Step 3: Add domain request and repository types**

In `apps/control-plane/internal/tenant/types.go`, add:

```go
type InitialTeamMemberInput struct {
	UserID uuid.UUID
	Role   string
}
```

Change `CreateTeamRequest`:

```go
type CreateTeamRequest struct {
	TenantID         uuid.UUID
	ActorUserID      uuid.UUID
	Slug             string
	Name             string
	Status           TeamStatus
	HumanOwnerUserID *uuid.UUID
	InitialMembers   []InitialTeamMemberInput
	Metadata         map[string]any
}
```

Update every existing `CreateTeamRequest` test fixture in `apps/control-plane/internal/tenant/service_test.go` and `apps/control-plane/internal/api/team_routes_test.go` to pass a non-nil `ActorUserID`. This includes tests that create a team only as setup for governance/config revision assertions.

In `apps/control-plane/internal/tenant/repository.go`, add to `Repository`:

```go
CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error)
```

Add the params type:

```go
type CreateTeamWithInitialMembersParams struct {
	TenantID       uuid.UUID
	ActorUserID    uuid.UUID
	Slug           string
	Name           string
	Status         TeamStatus
	OwnerUserID    uuid.UUID
	InitialMembers []InitialTeamMemberInput
	Metadata       map[string]any
}
```

- [ ] **Step 4: Implement service validation**

In `apps/control-plane/internal/tenant/service.go`, change `CreateTeam` to return `*TeamOverview` and use this validation shape:

```go
func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamOverview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.ActorUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: actor_user_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	initialMembers, err := normalizeInitialMembers(*req.HumanOwnerUserID, req.InitialMembers)
	if err != nil {
		return nil, err
	}

	team, err := s.repository.CreateTeamWithInitialMembers(ctx, CreateTeamWithInitialMembersParams{
		TenantID:       req.TenantID,
		ActorUserID:    req.ActorUserID,
		Slug:           slug,
		Name:           name,
		Status:         status,
		OwnerUserID:    *req.HumanOwnerUserID,
		InitialMembers: initialMembers,
		Metadata:       cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create team with initial members: %w", err)
	}
	return s.GetOverview(ctx, team.TenantID, team.ID)
}
```

Add helper:

```go
func normalizeInitialMembers(ownerUserID uuid.UUID, members []InitialTeamMemberInput) ([]InitialTeamMemberInput, error) {
	seen := map[uuid.UUID]struct{}{ownerUserID: {}}
	normalized := make([]InitialTeamMemberInput, 0, len(members))
	for _, member := range members {
		if member.UserID == uuid.Nil {
			return nil, fmt.Errorf("%w: initial member user_id is required", ErrInvalidInput)
		}
		if member.Role != TeamRoleMember && member.Role != TeamRoleViewer {
			return nil, fmt.Errorf("%w: initial member role must be member or viewer", ErrInvalidInput)
		}
		if _, ok := seen[member.UserID]; ok {
			return nil, fmt.Errorf("%w: duplicate initial member", ErrInvalidInput)
		}
		seen[member.UserID] = struct{}{}
		normalized = append(normalized, member)
	}
	return normalized, nil
}
```

- [ ] **Step 5: Add sqlc queries for transaction**

> **Design note — cross-domain query:** `GetActiveTenantUserForTeamCreate` queries `auth_users` and `tenant_members` but lives in `tenant_team_config.sql`. This is intentional: the atomic team-creation transaction runs inside a single sqlc `Queries` instance (via `WithTx`), so all queries the transaction needs must reside in the same sqlc package. sqlc does not support cross-package `WithTx`. The generated Go code in `tenant_team_config.sql.go` will include auth-user row types, which is acceptable for this transactional boundary.

In `apps/control-plane/internal/storage/queries/tenant_team_config.sql`, add:

```sql
-- name: GetActiveTenantUserForTeamCreate :one
SELECT au.id, au.username, au.display_name, au.email, au.status
FROM auth_users au
JOIN tenant_members tm ON tm.principal_id = au.id
WHERE au.id = sqlc.arg('id')::uuid
  AND au.status = 'active'
  AND au.deleted_at IS NULL
  AND tm.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tm.principal_type = 'user'
  AND tm.status = 'active'
  AND tm.disabled_at IS NULL
LIMIT 1;

-- name: AddTeamOwnerMembership :one
INSERT INTO tenant_members (
    tenant_id,
    team_id,
    principal_type,
    principal_id,
    role,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    'user',
    sqlc.arg('user_id')::uuid,
    'owner',
    'active'
)
ON CONFLICT (tenant_id, team_id, principal_type, principal_id, role)
DO UPDATE SET
    status = 'active',
    disabled_at = NULL,
    updated_at = NOW()
RETURNING *;
```

Use the existing `AddTeamMember` query for `member` and `viewer`; it already inserts into `tenant_members` with `principal_type = 'user'`.

- [ ] **Step 6: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: sqlc generation succeeds and updates `tenant_team_config.sql.go`.

- [ ] **Step 7: Implement repository transaction**

> **Design note — `AddTeamMember` ON CONFLICT behavior:** The existing `AddTeamMember` sqlc query uses `ON CONFLICT DO UPDATE SET status='active'`. In the initial-members loop, this means a previously-disabled member will be reactivated rather than inserted as a new row. This is the desired behavior: if a user was previously a member and was later disabled, adding them as an initial member should restore their membership to active. If conflicting memberships need to be rejected with an error instead, replace the query with `ON CONFLICT DO NOTHING` and check `RowsAffected`.

In `apps/control-plane/internal/tenant/pg_repository.go`, implement:

```go
func (r *PgRepository) CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error) {
	if r.db == nil {
		return TeamRecord{}, fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return TeamRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	qtx := r.q.WithTx(tx)
	if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
		ID:       params.OwnerUserID,
		TenantID: params.TenantID,
	}); err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	for _, member := range params.InitialMembers {
		if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
			ID:       member.UserID,
			TenantID: params.TenantID,
		}); err != nil {
			return TeamRecord{}, mapNoRows(err)
		}
	}
	team, err := qtx.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:         params.TenantID,
		Slug:             params.Slug,
		Name:             params.Name,
		Status:           string(params.Status),
		HumanOwnerUserID: nullUUIDFromPtr(&params.OwnerUserID),
		Metadata:         metadata,
	})
	if err != nil {
		return TeamRecord{}, mapConstraintError(err)
	}
	if _, err := qtx.AddTeamOwnerMembership(ctx, queries.AddTeamOwnerMembershipParams{
		TenantID: params.TenantID,
		TeamID:   team.ID,
		UserID:   params.OwnerUserID,
	}); err != nil {
		return TeamRecord{}, mapConstraintError(err)
	}
	for _, member := range params.InitialMembers {
		if _, err := qtx.AddTeamMember(ctx, queries.AddTeamMemberParams{
			TenantID: params.TenantID,
			TeamID:   team.ID,
			UserID:   member.UserID,
			Role:     member.Role,
		}); err != nil {
			return TeamRecord{}, mapConstraintError(err)
		}
	}
	record, err := teamRecordFromQuery(team)
	if err != nil {
		return TeamRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamRecord{}, err
	}
	committed = true
	return record, nil
}
```

- [ ] **Step 8: Add required audit writing inside the transaction**

The spec requires team creation and initial member additions to produce audit events as part of the create flow. Add audit writing inside `CreateTeamWithInitialMembers` before `tx.Commit(ctx)` by using the same transaction-bound `qtx`. The event details must include at least `team_id`, `slug`, `human_owner_user_id`, and the `initial_members` count.

Audit actions:

```text
team.create
team.member.add
```

Repository transaction shape:

```go
if _, err := qtx.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
	TenantID:     params.TenantID,
	EventType:    "team_management",
	ActorType:    "user",
	ActorID:      params.ActorUserID.String(),
	ResourceType: "team",
	ResourceID:   team.ID.String(),
	Action:       "team.create",
	Details:      auditDetailsJSON,
}); err != nil {
	return TeamRecord{}, err
}
```

For each owner/member membership inserted in the same transaction, create a `team.member.add` audit event with `resource_type = "team_member"` and `resource_id` set to the membership ID.

Add service or route tests that assert successful create emits audit events:

```go
if len(repo.auditEvents) < 2 {
	t.Fatalf("expected team create and member audit events, got %#v", repo.auditEvents)
}
```

Do not make audit optional for this create path. Unit tests that do not exercise create-team auditing may keep a no-op fake, but the create-team tests in this task must assert the audit events.

- [ ] **Step 9: Verify Task 2**

Run:

```bash
go test ./apps/control-plane/internal/tenant -run 'TestCreateTeam|TestTeamStatus' -count=1
```

Expected: all targeted tenant tests pass.

- [ ] **Step 10: Commit Task 2**

```bash
git add apps/control-plane/internal/tenant apps/control-plane/internal/storage/queries/tenant_team_config.sql apps/control-plane/internal/storage/queries/tenant_team_config.sql.go
git commit -m "feat: create teams with initial members atomically"
```

## Task 3: Team Create Route, Contract, And Web API Client

**Files:**
- Modify: `apps/control-plane/internal/tenant/handler.go`
- Modify: `apps/control-plane/internal/api/team_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/lib/api/teams.test.ts`

- [ ] **Step 1: Write failing route test**

In `apps/control-plane/internal/api/team_routes_test.go`, update `TestTeamRoutesUseConsoleTenant` create request body:

```go
memberID := uuid.New()
viewerID := uuid.New()
createBody := `{
  "slug":"platform",
  "name":"Platform",
  "human_owner_user_id":"` + ownerID.String() + `",
  "initial_members":[
    {"user_id":"` + memberID.String() + `","role":"member"},
    {"user_id":"` + viewerID.String() + `","role":"viewer"}
  ],
  "metadata":{"cost_center":"r-and-d"}
}`
createReq := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(createBody))
```

Assert service receives members and response is overview:

```go
if !reflect.DeepEqual(service.createReq.InitialMembers, []tenant.InitialTeamMemberInput{
	{UserID: memberID, Role: tenant.TeamRoleMember},
	{UserID: viewerID, Role: tenant.TeamRoleViewer},
}) {
	t.Fatalf("expected initial members in create request, got %#v", service.createReq.InitialMembers)
}
var created struct {
	Team struct {
		ID               string `json:"id"`
		TenantID         string `json:"tenant_id"`
		HumanOwnerUserID string `json:"human_owner_user_id"`
	} `json:"team"`
	AllowedActions []string `json:"allowed_actions"`
}
```

- [ ] **Step 2: Run route test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestTeamRoutesUseConsoleTenant -count=1
```

Expected: FAIL because handler does not decode `initial_members` and still returns a plain team response.

- [ ] **Step 3: Update handler interface and response**

In `apps/control-plane/internal/tenant/handler.go`, change:

```go
CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamOverview, error)
```

Update `CreateTeam` request struct:

```go
var req struct {
	Slug             string                   `json:"slug"`
	Name             string                   `json:"name"`
	Status           TeamStatus               `json:"status"`
	HumanOwnerUserID *uuid.UUID               `json:"human_owner_user_id"`
	InitialMembers   []InitialTeamMemberInput `json:"initial_members"`
	Metadata         map[string]any           `json:"metadata"`
}
```

Return overview:

```go
overview, err := service.CreateTeam(r.Context(), CreateTeamRequest{
	TenantID:         tenantID,
	ActorUserID:      middleware.GetUserID(r.Context()),
	Slug:             req.Slug,
	Name:             req.Name,
	Status:           req.Status,
	HumanOwnerUserID: req.HumanOwnerUserID,
	InitialMembers:   req.InitialMembers,
	Metadata:         req.Metadata,
})
if err != nil {
	writeHandlerError(w, err)
	return
}
overview.AllowedActions = h.allowedTeamActions(r, tenantID, overview.Team.ID)
writeJSON(w, http.StatusCreated, teamOverviewResponseFromDomain(overview))
```

Update `apps/control-plane/internal/tenant/handler.go` `HandlerService` interface — change:
```go
CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error)
```
to:
```go
CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamOverview, error)
```

Update `apps/control-plane/internal/api/team_routes_test.go` `routeTeamService` mock:
- Change `CreateTeam` return type from `(*tenant.Team, error)` to `(*tenant.TeamOverview, error)`.
- Return `&tenant.TeamOverview{Team: &tenant.Team{...}, AllowedActions: []tenant.AllowedTeamAction{"team.update"}}` so existing assertions on `team.id`/`team.tenant_id` fields continue to pass under the `overview.team` nesting.
- Update the existing `TestTeamRoutesUseConsoleTenant` response decode to unwrap the overview envelope before checking the team fields. The test already decodes `var created struct { Team struct { ... } }` into a `created` variable; ensure the assertions access `created.Team.ID` etc.

- [ ] **Step 4: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, update `POST /api/v1/teams` request schema to include:

```yaml
initial_members:
  type: array
  items:
    type: object
    required:
      - user_id
      - role
    properties:
      user_id:
        type: string
        format: uuid
      role:
        type: string
        enum: [member, viewer]
```

Change the `201` response schema to `TeamOverviewResponse`.

Keep `allowed_actions` in `TeamOverviewResponse` for this slice. Do not add row-level `allowed_actions` to `TeamListItem`; list row writes still call authorized endpoints and receive backend authorization.

- [ ] **Step 5: Write failing frontend API test**

In `apps/web/src/lib/api/teams.test.ts`, replace the create-team test with:

```ts
it("creates team with owner and initial members and parses overview", async () => {
  const overview = {
    team: {
      id: "11111111-1111-4111-8111-111111111111",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      slug: "security",
      name: "安全团队",
      status: "active",
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      human_owner: {
        user_id: "33333333-3333-4333-8333-333333333333",
        username: "owner",
        display_name: "负责人",
        email: "owner@example.com",
        status: "active",
      },
    },
    member_count: 3,
    digital_employee_count: 0,
    capability_count: 0,
    pending_draft_count: 0,
    pending_item_count: 0,
    allowed_actions: ["team.update"],
  };
  const fetcher = vi.fn(async () =>
    new Response(JSON.stringify(overview), {
      headers: { "content-type": "application/json" },
      status: 201,
    }),
  );

  await expect(
    createTeam(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        slug: "security",
        name: "安全团队",
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        initial_members: [
          { user_id: "44444444-4444-4444-8444-444444444444", role: "member" },
          { user_id: "55555555-5555-4555-8555-555555555555", role: "viewer" },
        ],
      },
    ),
  ).resolves.toEqual(overview);

  expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams", {
    body: JSON.stringify({
      slug: "security",
      name: "安全团队",
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      initial_members: [
        { user_id: "44444444-4444-4444-8444-444444444444", role: "member" },
        { user_id: "55555555-5555-4555-8555-555555555555", role: "viewer" },
      ],
    }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
});
```

- [ ] **Step 6: Update frontend team client types**

In `apps/web/src/lib/api/teams.ts`, add:

```ts
export type InitialTeamMemberInput = {
  user_id: string;
  role: Extract<TeamMemberRole, "member" | "viewer">;
};
```

Update `CreateTeamInput`:

```ts
export type CreateTeamInput = {
  slug: string;
  name: string;
  human_owner_user_id: string;
  initial_members?: InitialTeamMemberInput[];
  status?: TeamStatus;
  metadata?: Record<string, unknown>;
};
```

Change `createTeam` return type:

```ts
export function createTeam(options: ApiClientOptions, input: CreateTeamInput): Promise<TeamOverview> {
  return postJson<TeamOverview>(options, "/api/v1/teams", input, "create team");
}
```

- [ ] **Step 7: Verify Task 3**

Run:

```bash
go test ./apps/control-plane/internal/api ./apps/control-plane/internal/tenant -run 'TestTeamRoutesUseConsoleTenant|TestCreateTeam' -count=1
pnpm --filter @superteam/web test src/lib/api/teams.test.ts src/lib/api/auth.test.ts
pnpm verify:contracts
```

Expected: Go tests and Vitest pass. `pnpm verify:contracts` must pass or report only an already documented unrelated route mismatch; if it fails for team create or user search, fix the contract before continuing.

- [ ] **Step 8: Commit Task 3**

```bash
git add apps/control-plane/internal/tenant/handler.go apps/control-plane/internal/api/team_routes_test.go contracts/control-plane/openapi.yaml apps/web/src/lib/api/teams.ts apps/web/src/lib/api/teams.test.ts
git commit -m "feat: expose create team overview contract"
```

## Task 4: Team List Toolbar And Table UX

**Files:**
- Create: `apps/web/src/features/teams/components/team-management-toolbar.tsx`
- Modify: `apps/web/src/features/teams/components/team-list-table.tsx`
- Modify: `apps/web/src/features/teams/index.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing UI test for filters**

In `apps/web/src/features/teams/index.test.tsx`, add:

```tsx
it("filters team summaries through the real list endpoint", async () => {
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/teams") {
      return jsonResponse([]);
    }
    return jsonResponse({});
  }) as unknown as typeof fetch;

  const screen = render(
    <QueryClientProvider client={createQueryClient()}>
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );

  await expect.element(screen.getByText("团队管理")).toBeInTheDocument();
  await userEvent.type(screen.getByPlaceholder("搜索团队名称、slug、负责人"), "安全");
  await userEvent.selectOptions(screen.getByLabelText("团队状态"), "active");
  await userEvent.selectOptions(screen.getByLabelText("治理状态"), "draft_pending");

  await expect.poll(() => fetcher.mock.calls.map(([url]) => String(url))).toContain(
    "http://control-plane.local/api/v1/teams?status=active&governance_status=draft_pending&q=%E5%AE%89%E5%85%A8",
  );
});
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because the toolbar does not exist.

- [ ] **Step 3: Create toolbar component**

Create `apps/web/src/features/teams/components/team-management-toolbar.tsx`:

```tsx
import { RotateCcw, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { GovernanceSummaryStatus, TeamStatus } from "@/lib/api/teams";

export type TeamListFilters = {
  governance_status?: GovernanceSummaryStatus;
  q: string;
  status?: TeamStatus;
};

type TeamManagementToolbarProps = {
  filters: TeamListFilters;
  onChange: (filters: TeamListFilters) => void;
  onReset: () => void;
};

export function TeamManagementToolbar({ filters, onChange, onReset }: TeamManagementToolbarProps) {
  return (
    <div className="mb-4 grid gap-3 rounded-md border bg-card/95 p-3 shadow-sm md:grid-cols-[minmax(260px,1fr)_180px_180px_auto]">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="pl-9"
          onChange={(event) => onChange({ ...filters, q: event.target.value })}
          placeholder="搜索团队名称、slug、负责人"
          value={filters.q}
        />
      </div>
      <Select
        value={filters.status ?? "all"}
        onValueChange={(value) => onChange({ ...filters, status: value === "all" ? undefined : (value as TeamStatus) })}
      >
        <SelectTrigger aria-label="团队状态">
          <SelectValue placeholder="团队状态" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">全部状态</SelectItem>
          <SelectItem value="active">活跃</SelectItem>
          <SelectItem value="disabled">已禁用</SelectItem>
          <SelectItem value="archived">已归档</SelectItem>
        </SelectContent>
      </Select>
      <Select
        value={filters.governance_status ?? "all"}
        onValueChange={(value) =>
          onChange({ ...filters, governance_status: value === "all" ? undefined : (value as GovernanceSummaryStatus) })
        }
      >
        <SelectTrigger aria-label="治理状态">
          <SelectValue placeholder="治理状态" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">全部治理</SelectItem>
          <SelectItem value="not_configured">未配置</SelectItem>
          <SelectItem value="draft_pending">草案待批准</SelectItem>
          <SelectItem value="active">已生效</SelectItem>
          <SelectItem value="needs_update">需更新</SelectItem>
        </SelectContent>
      </Select>
      <Button onClick={onReset} type="button" variant="outline">
        <RotateCcw data-icon="inline-start" />
        重置
      </Button>
    </div>
  );
}
```

- [ ] **Step 4: Wire filters into `TeamsView`**

In `apps/web/src/features/teams/index.tsx`, add state and filtered query:

```tsx
const [filters, setFilters] = useState<TeamListFilters>({ q: "" });
const teams = useQuery({
  queryKey: ["team-summaries", filters],
  queryFn: () =>
    listTeamSummaries(
      { baseUrl: apiBaseUrl, fetcher },
      {
        governance_status: filters.governance_status,
        q: filters.q,
        status: filters.status,
      },
    ),
});
```

Render toolbar before the table:

```tsx
<TeamManagementToolbar
  filters={filters}
  onChange={setFilters}
  onReset={() => setFilters({ q: "" })}
/>
```

- [ ] **Step 5: Update table visual and states**

In `team-list-table.tsx`, update the component's props type/interface to include `highlightedTeamId?: string`. Add the prop and render:

```tsx
<TableRow
  className={team.id === highlightedTeamId ? "bg-[var(--superteam-menu-accent-soft)]" : undefined}
  key={team.id}
>
```

Update owner cell to two-line display:

```tsx
<TableCell>
  <div className="font-medium">{teamOwnerLabel(team)}</div>
  {team.human_owner?.email ? <div className="text-xs text-muted-foreground">{team.human_owner.email}</div> : null}
</TableCell>
```

Add the updated-time column:

```tsx
<TableHead>更新时间</TableHead>
```

```tsx
<TableCell>{team.updated_at ? new Date(team.updated_at).toLocaleString("zh-CN") : "-"}</TableCell>
```

- [ ] **Step 6: Verify Task 4**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx src/lib/api/teams.test.ts
pnpm --filter @superteam/web typecheck
```

Expected: tests and typecheck pass.

- [ ] **Step 7: Commit Task 4**

```bash
git add apps/web/src/features/teams/components/team-management-toolbar.tsx apps/web/src/features/teams/components/team-list-table.tsx apps/web/src/features/teams/index.tsx apps/web/src/features/teams/index.test.tsx
git commit -m "feat: add team management filters"
```

## Task 5: Create Team Drawer Basic Step

**Files:**
- Create: `apps/web/src/features/teams/components/create-team-drawer.tsx`
- Create: `apps/web/src/features/teams/components/create-team-basic-step.tsx`
- Create: `apps/web/src/features/teams/components/create-team-members-step.tsx`
- Modify: `apps/web/src/features/teams/index.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing drawer basic-step test**

Add to `apps/web/src/features/teams/index.test.tsx`:

```tsx
it("opens create team drawer and requires name slug and owner before next step", async () => {
  const fetcher = createTeamsFetcher();
  const screen = render(
    <QueryClientProvider client={createQueryClient()}>
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );

  await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
  await expect.element(screen.getByRole("heading", { name: "新建团队" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await expect.element(screen.getByText("团队名称不能为空")).toBeInTheDocument();
  await expect.element(screen.getByText("团队标识不能为空")).toBeInTheDocument();
  await expect.element(screen.getByText("请选择负责人")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because the drawer and button do not exist.

- [ ] **Step 3: Create drawer shell**

Create `create-team-drawer.tsx` with step state:

```tsx
import { useMemo, useState } from "react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import type { UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import { CreateTeamBasicStep } from "./create-team-basic-step";
import { CreateTeamMembersStep } from "./create-team-members-step";

export type CreateTeamDraft = {
  name: string;
  slug: string;
  owner?: UserSummary;
  initial_members: InitialTeamMemberInput[];
};

type CreateTeamDrawerProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (draft: CreateTeamDraft) => void;
  open: boolean;
  submitError?: string;
};

const emptyDraft: CreateTeamDraft = { name: "", slug: "", initial_members: [] };

export function CreateTeamDrawer(props: CreateTeamDrawerProps) {
  const [step, setStep] = useState<"basic" | "members">("basic");
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const canSubmit = useMemo(() => draft.name.trim() && draft.slug.trim() && draft.owner, [draft]);

  function nextStep() {
    const nextErrors: Record<string, string> = {};
    if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
    if (!draft.slug.trim()) nextErrors.slug = "团队标识不能为空";
    if (!draft.owner) nextErrors.owner = "请选择负责人";
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) setStep("members");
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-[520px]">
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle>新建团队</SheetTitle>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto p-6">
          {step === "basic" ? (
            <CreateTeamBasicStep
              apiBaseUrl={props.apiBaseUrl}
              errors={errors}
              fetcher={props.fetcher}
              value={draft}
              onChange={setDraft}
            />
          ) : (
            <CreateTeamMembersStep draft={draft} onChange={setDraft} apiBaseUrl={props.apiBaseUrl} fetcher={props.fetcher} />
          )}
        </div>
        <div className="flex justify-between gap-3 border-t p-4">
          <button className="rounded-md border px-4 py-2" onClick={() => props.onOpenChange(false)} type="button">
            取消
          </button>
          {step === "members" ? (
            <button className="rounded-md border px-4 py-2" onClick={() => setStep("basic")} type="button">
              上一步
            </button>
          ) : null}
          {step === "basic" ? (
            <button className="rounded-md bg-primary px-4 py-2 text-primary-foreground" onClick={nextStep} type="button">
              下一步
            </button>
          ) : (
            <button
              className="rounded-md bg-primary px-4 py-2 text-primary-foreground disabled:opacity-60"
              disabled={!canSubmit || props.isSubmitting}
              onClick={() => props.onSubmit(draft)}
              type="button"
            >
              创建团队
            </button>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
```

Use shadcn `Button` instead of raw `button` when imports are available in the implementation pass; keep labels and disabled behavior identical.

- [ ] **Step 4: Create minimal members step stub so Task 5 typecheck passes**

Create `apps/web/src/features/teams/components/create-team-members-step.tsx`:

```tsx
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamMembersStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

export function CreateTeamMembersStep(_props: CreateTeamMembersStepProps) {
  return (
    <div className="rounded-md border bg-muted/30 p-4 text-sm text-muted-foreground">
      初始成员将在下一步接入真实用户搜索。
    </div>
  );
}
```

Task 6 replaces this stub with the full candidate-user list and role assignment UI.

- [ ] **Step 5: Create basic step with owner search**

> **UX note:** The owner search triggers on every keystroke in the team name field. For a production-quality implementation, add a `useDebounce` hook (e.g. 300ms) on the search query to avoid hammering the API on every keystroke. Show a loading spinner (`owners.isLoading`) while the user list is fetching, and an empty-state message when no active users match the query.

Create `create-team-basic-step.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Input } from "@/components/ui/input";
import { listUsers } from "@/lib/api/auth";
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamBasicStepProps = {
  apiBaseUrl: string;
  errors: Record<string, string>;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
  value: CreateTeamDraft;
};

export function CreateTeamBasicStep({ apiBaseUrl, errors, fetcher, onChange, value }: CreateTeamBasicStepProps) {
  const owners = useQuery({
    queryKey: ["team-owner-candidates", value.name],
    queryFn: () => listUsers({ baseUrl: apiBaseUrl, fetcher, limit: 20, offset: 0, q: value.name, status: "active" }),
  });

  return (
    <div className="space-y-5">
      <label className="grid gap-2">
        <span className="text-sm font-medium">团队名称</span>
        <Input value={value.name} onChange={(event) => onChange({ ...value, name: event.target.value })} />
        {errors.name ? <span className="text-sm text-destructive">{errors.name}</span> : null}
      </label>
      <label className="grid gap-2">
        <span className="text-sm font-medium">团队标识 slug</span>
        <Input value={value.slug} onChange={(event) => onChange({ ...value, slug: event.target.value })} />
        {errors.slug ? <span className="text-sm text-destructive">{errors.slug}</span> : null}
      </label>
      <div className="grid gap-2">
        <span className="text-sm font-medium">负责人</span>
        <div className="rounded-md border">
          {(owners.data?.items ?? []).map((user) => (
            <button
              className="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-muted"
              key={user.id}
              onClick={() => onChange({ ...value, owner: user })}
              type="button"
            >
              <span>{user.username}</span>
              <span className="text-muted-foreground">{user.status}</span>
            </button>
          ))}
        </div>
        {value.owner ? <span className="text-sm text-muted-foreground">已选择：{value.owner.username}</span> : null}
        {errors.owner ? <span className="text-sm text-destructive">{errors.owner}</span> : null}
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Add button and drawer to TeamsView**

In `index.tsx`, render the primary button and drawer:

```tsx
const [createOpen, setCreateOpen] = useState(false);
```

```tsx
<Button onClick={() => setCreateOpen(true)}>
  <Plus data-icon="inline-start" />
  新建团队
</Button>
```

```tsx
<CreateTeamDrawer
  apiBaseUrl={apiBaseUrl}
  fetcher={fetcher}
  open={createOpen}
  onOpenChange={setCreateOpen}
  onSubmit={() => undefined}
/>
```

- [ ] **Step 7: Verify Task 5**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: test and typecheck pass.

- [ ] **Step 8: Commit Task 5**

```bash
git add apps/web/src/features/teams/components/create-team-drawer.tsx apps/web/src/features/teams/components/create-team-basic-step.tsx apps/web/src/features/teams/components/create-team-members-step.tsx apps/web/src/features/teams/index.tsx apps/web/src/features/teams/index.test.tsx
git commit -m "feat: add create team drawer basic step"
```

## Task 6: Initial Members Step And Submit

**Files:**
- Modify: `apps/web/src/features/teams/components/create-team-members-step.tsx`
- Modify: `apps/web/src/features/teams/components/create-team-drawer.tsx`
- Modify: `apps/web/src/features/teams/index.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx`

- [ ] **Step 1: Write failing workflow test**

Add to `apps/web/src/features/teams/index.test.tsx`:

```tsx
it("creates a team with selected owner and initial members", async () => {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    if (url.pathname === "/api/v1/teams" && (init?.method ?? "GET") === "GET") return jsonResponse([]);
    if (url.pathname === "/api/auth/users") {
      return jsonResponse({
        items: [
          { id: "owner-user", username: "owner", status: "active" },
          { id: "member-user", username: "member", status: "active" },
          { id: "viewer-user", username: "viewer", status: "active" },
        ],
      });
    }
    if (url.pathname === "/api/v1/teams" && init?.method === "POST") {
      expect(JSON.parse(String(init.body))).toEqual({
        name: "安全团队",
        slug: "security",
        human_owner_user_id: "owner-user",
        initial_members: [
          { user_id: "member-user", role: "member" },
          { user_id: "viewer-user", role: "viewer" },
        ],
      });
      return jsonResponse({
        team: { id: "team-security", tenant_id: "tenant-1", name: "安全团队", slug: "security", status: "active" },
        member_count: 3,
        digital_employee_count: 0,
        capability_count: 0,
        pending_draft_count: 0,
        pending_item_count: 0,
        allowed_actions: [],
      }, 201);
    }
    return jsonResponse({});
  }) as unknown as typeof fetch;

  const screen = render(
    <QueryClientProvider client={createQueryClient()}>
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );

  await userEvent.click(screen.getByRole("button", { name: "新建团队" }));
  await userEvent.type(screen.getByLabelText("团队名称"), "安全团队");
  await userEvent.type(screen.getByLabelText("团队标识 slug"), "security");
  await userEvent.click(screen.getByRole("button", { name: "owner" }));
  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: "添加 member 为普通成员" }));
  await userEvent.click(screen.getByRole("button", { name: "添加 viewer 为只读观察者" }));
  await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

  await expect.poll(() => fetcher.mock.calls.some(([url, init]) => String(url).endsWith("/api/v1/teams") && init?.method === "POST")).toBe(true);
});
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx
```

Expected: FAIL because members step and submit are incomplete.

- [ ] **Step 3: Create members step**

> **UX note — duplicate feedback:** The `addMember` function silently skips users who are already the owner or already in `initial_members`. Consider adding a visual hint (e.g. disabling the "add" button with a tooltip "已是团队成员") instead of silently no-opping, so the user understands why clicking has no effect.

Create `create-team-members-step.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { listUsers } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamMembersStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

export function CreateTeamMembersStep({ apiBaseUrl, draft, fetcher, onChange }: CreateTeamMembersStepProps) {
  const users = useQuery({
    queryKey: ["team-member-candidates"],
    queryFn: () => listUsers({ baseUrl: apiBaseUrl, fetcher, limit: 50, offset: 0, status: "active" }),
  });

  function addMember(member: InitialTeamMemberInput) {
    if (member.user_id === draft.owner?.id) return;
    if (draft.initial_members.some((item) => item.user_id === member.user_id)) return;
    onChange({ ...draft, initial_members: [...draft.initial_members, member] });
  }

  return (
    <div className="space-y-5">
      <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-900">
        负责人、管理员、审批人需创建后发起特权角色申请。
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">候选用户</h3>
        {(users.data?.items ?? []).map((user) => {
          const isOwner = user.id === draft.owner?.id;
          return (
            <div className="flex items-center justify-between rounded-md border px-3 py-2" key={user.id}>
              <div>
                <div className="text-sm font-medium">{user.username}</div>
                <div className="text-xs text-muted-foreground">{isOwner ? "负责人" : user.status}</div>
              </div>
              <div className="flex gap-2">
                <button disabled={isOwner} onClick={() => addMember({ user_id: user.id, role: "member" })} type="button">
                  添加 {user.username} 为普通成员
                </button>
                <button disabled={isOwner} onClick={() => addMember({ user_id: user.id, role: "viewer" })} type="button">
                  添加 {user.username} 为只读观察者
                </button>
              </div>
            </div>
          );
        })}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">已选择的初始成员（{draft.initial_members.length}）</h3>
        {draft.initial_members.map((member) => (
          <div className="flex items-center justify-between rounded-md border px-3 py-2" key={member.user_id}>
            <span className="text-sm">{member.user_id}</span>
            <span className="text-sm text-muted-foreground">{member.role === "member" ? "普通成员" : "只读观察者"}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
```

Use project `Button`, `Badge`, and richer user display during implementation; keep role labels and disabled owner behavior identical.

- [ ] **Step 4: Wire submit mutation**

In `apps/web/src/features/teams/index.tsx`, add:

```tsx
const [highlightedTeamId, setHighlightedTeamId] = useState<string>();
const createMutation = useMutation({
  mutationFn: (draft: CreateTeamDraft) =>
    createTeam(
      { baseUrl: apiBaseUrl, fetcher },
      {
        name: draft.name.trim(),
        slug: draft.slug.trim(),
        human_owner_user_id: draft.owner?.id ?? "",
        initial_members: draft.initial_members,
      },
    ),
  onSuccess: (overview) => {
    setCreateOpen(false);
    setHighlightedTeamId(overview.team.id);
    void teams.refetch();
  },
});
```

Pass to drawer:

```tsx
isSubmitting={createMutation.isPending}
onSubmit={(draft) => createMutation.mutate(draft)}
submitError={createMutation.error instanceof Error ? createMutation.error.message : undefined}
```

Pass highlight to table:

```tsx
<TeamListTable highlightedTeamId={highlightedTeamId} ... />
```

- [ ] **Step 5: Verify Task 6**

Run:

```bash
pnpm --filter @superteam/web test src/features/teams/index.test.tsx src/lib/api/teams.test.ts src/lib/api/auth.test.ts
pnpm --filter @superteam/web typecheck
```

Expected: tests and typecheck pass.

- [ ] **Step 6: Commit Task 6**

```bash
git add apps/web/src/features/teams/components/create-team-members-step.tsx apps/web/src/features/teams/components/create-team-drawer.tsx apps/web/src/features/teams/index.tsx apps/web/src/features/teams/index.test.tsx
git commit -m "feat: create teams from drawer"
```

## Task 7: End-To-End Verification And Changelog

**Files:**
- Modify: `CHANGELOG.md`
- Use, do not commit: `.scratch/team-ui-audit/`

- [ ] **Step 1: Add changelog entry**

Under `## [Unreleased]` in `CHANGELOG.md`, add:

```md
### Added

- 2026-06-03 HH:MM：团队管理页补齐真实筛选工具条和右侧两步新建团队抽屉，创建团队时通过 Control Plane 事务一次性写入团队、负责人和初始成员，并以真实接口刷新列表。
```

Use the current Asia/Shanghai time for `HH:MM`.

- [ ] **Step 2: Run focused verification**

Run:

```bash
go test ./apps/control-plane/internal/auth ./apps/control-plane/internal/tenant ./apps/control-plane/internal/api ./apps/control-plane/internal/storage/queries
pnpm --filter @superteam/web test src/lib/api/auth.test.ts src/lib/api/teams.test.ts src/features/teams/index.test.tsx
pnpm --filter @superteam/web typecheck
pnpm verify:contracts
git diff --check
```

Expected: all pass. When `pnpm verify:contracts` reports route drift, classify every failing path before completion. Fix any failure involving `/api/v1/teams`, `/api/auth/users`, or generated types touched by this plan. Only report a remaining failure as residual risk when it is unrelated to this plan and was already present before execution.

- [ ] **Step 3: Start services**

Run in separate terminal sessions or background commands:

```bash
pnpm dev:control-plane -- --config apps/control-plane/config/config.yaml
pnpm --filter @superteam/web dev -- --host 127.0.0.1 --port 3000
```

Expected: Control Plane listens on `:8081`, Web listens on `http://127.0.0.1:3000`.

- [ ] **Step 4: Check and seed real data**

Check the local DB or `/api/auth/users?status=active&limit=20&offset=0` for at least three active users. Insert missing active users through the existing user API after login or through the repository-supported development path. Use active users with display names and emails so owner/member selectors are meaningful.

API-based seed shape after login:

```bash
curl -i -X POST http://127.0.0.1:8081/api/auth/users \
  -H 'content-type: application/json' \
  --cookie 'superteam_session=<browser-cookie>' \
  --data '{"username":"security-owner","password":"secret123"}'
```

Do not commit seed data or local DB dumps.

- [ ] **Step 5: Browser smoke test with real backend**

Use Browser or Playwright against `http://127.0.0.1:3000`:

1. Log in as `admin / admin`.
2. Open `/teams`.
3. Verify search, team status filter, governance status filter, and reset call `GET /api/v1/teams` with real query params.
4. Open “新建团队”.
5. Select active owner from `/api/auth/users?q=&status=active`.
6. Move to “初始成员”.
7. Add one `member` and one `viewer`.
8. Submit.
9. Verify `POST /api/v1/teams` contains `initial_members`.
10. Verify list refresh shows the new team and owner summary.

Save screenshots:

```text
.scratch/team-ui-audit/team-list-after-create-real.png
.scratch/team-ui-audit/create-team-drawer-basic-real.png
.scratch/team-ui-audit/create-team-drawer-members-real.png
```

- [ ] **Step 6: Verify database facts**

Use the configured database connection and run read-only checks:

```sql
SELECT slug, name, human_owner_user_id, status
FROM tenant_teams
WHERE slug = 'security';

SELECT role, principal_id, status
FROM tenant_members
WHERE team_id = (
  SELECT id FROM tenant_teams WHERE slug = 'security'
)
ORDER BY role, principal_id;

SELECT action, resource_type, resource_id
FROM audit_events
WHERE resource_type = 'team'
ORDER BY created_at DESC
LIMIT 10;
```

Expected: one team row, one `owner` membership, selected `member`/`viewer` memberships, and audit rows for `team.create` and `team.member.add`.

- [ ] **Step 7: Stop services**

Stop all dev servers started in Step 3. Confirm ports are free:

```bash
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:8081 -sTCP:LISTEN
```

Expected: no listener output for both ports.

- [ ] **Step 8: Commit Task 7**

```bash
git add CHANGELOG.md
git commit -m "docs: record team creation UI API update"
```

## Final Review Checklist

- [ ] Spec coverage: list filters, owner summary, right-side drawer, active user search, one-shot create, real API, errors, and tests are each covered by at least one task.
- [ ] No mock-only completion evidence remains.
- [ ] No team detail Tab redesign has entered the implementation scope.
- [ ] No user invitation or user creation flow has entered the drawer.
- [ ] No uncommitted generated files are left after sqlc or OpenAPI generation.
- [ ] `git status --short` contains only intentional files before each commit.
