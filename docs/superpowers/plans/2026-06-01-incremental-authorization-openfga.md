# Incremental Authorization OpenFGA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建设 SuperTeam 第一阶段统一授权边界：新增 `internal/authz`，使用 DB-backed 最小实现接入 `/api/auth/me` 与 Runtime claim，为后续权限配置页面和 OpenFGA backend 保留稳定接口。

**Architecture:** 登录认证继续留在 `internal/auth`，授权判断集中到新的 `internal/authz`。第一版通过 PostgreSQL 查询 `tenant_members` 与 `runtime_node_scopes` 完成租户访问、团队访问、控制台访问和 Runtime claim 范围判断；业务 handler 只依赖 `Authorizer` 接口，不直接依赖 OpenFGA、Casbin 或散落角色判断。后续 OpenFGA 只替换 `Authorizer` 实现，不重写 Web/API 主链路。

**Tech Stack:** Go 1.25, chi/net/http, pgx/v5, sqlc, PostgreSQL, testify, existing auth/runtime/task handlers.

---

## Scope

本计划实现 spec 的第一阶段可验证范围：

- 新增 `apps/control-plane/internal/authz` 授权模块。
- 新增 DB-backed repository 查询和 `DBAuthorizer`。
- `/api/auth/me` 在登录成功后检查 `console.access`。
- Runtime claim 任务前检查 `task.claim`，拒绝超出 Runtime scope 的任务。
- 新增必要单元测试、handler 测试、sqlc 查询测试和回归测试。
- 更新 `CHANGELOG.md` 记录变更。

本计划不实现：

- 权限管理 Web 页面。
- OpenFGA 服务、store、model、tuple 同步。
- 完整数字员工、Capability、Artifact、Approval 授权页面。
- 面向业务用户的角色编辑流程。

## File Structure

- Create: `apps/control-plane/internal/authz/types.go`
  - 定义 `ActorRef`、`ResourceRef`、`CheckRequest`、`Decision`、动作常量、资源类型常量和错误。
- Create: `apps/control-plane/internal/authz/authorizer.go`
  - 定义 `Authorizer` 接口、`DBAuthorizer`、角色规则和动作分发。
- Create: `apps/control-plane/internal/authz/repository.go`
  - 定义授权查询 repository 接口与参数/结果类型。
- Create: `apps/control-plane/internal/authz/pg_repository.go`
  - 使用 sqlc queries 实现 DB-backed repository。
- Create: `apps/control-plane/internal/authz/authorizer_test.go`
  - 用内存 repository 测试授权语义。
- Modify: `apps/control-plane/internal/storage/queries/authz.sql`
  - 新增 `GetActiveTenantMembership`、`GetActiveTeamMembership`、`RuntimeNodeCoversTaskScope` 查询。
- Modify: `apps/control-plane/internal/storage/queries/queries_test.go`
  - 增加 tenant member 和 runtime scope 查询集成测试。
- Modify: generated files under `apps/control-plane/internal/storage/queries/*.go`
  - 运行 sqlc 生成。
- Modify: `apps/control-plane/internal/auth/handler.go`
  - 在 `GetCurrentUser` 中接入授权检查，未授权返回 403。
- Modify: `apps/control-plane/internal/auth/service.go`
  - 暴露当前用户 MVP 租户上下文查询方法，第一版固定使用默认租户。
- Modify: `apps/control-plane/internal/api/server.go`
  - 支持注入 `authz.Authorizer`，并传入 auth handler/runtime handler。
- Modify: `apps/control-plane/internal/app/app.go`
  - 构造 `authz.PgRepository` 和 `authz.DBAuthorizer`。
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
  - Runtime claim 筛选和 assign 前调用 `Authorizer.Check`。
- Modify: `apps/control-plane/internal/api/handlers/runtime_test.go`
  - 覆盖 scope 允许、scope 拒绝、poller 返回任务后仍二次检查。
- Modify: `apps/control-plane/internal/api/routes_test.go`
  - 覆盖 `/api/auth/me` 的授权拒绝和允许路径。
- Modify: `CHANGELOG.md`
  - 记录新增渐进式授权边界。

## Task 1: Define Authz Domain Contract

**Files:**
- Create: `apps/control-plane/internal/authz/types.go`
- Create: `apps/control-plane/internal/authz/authorizer.go`
- Test: `apps/control-plane/internal/authz/authorizer_test.go`

- [ ] **Step 1: Write failing contract tests**

Create `apps/control-plane/internal/authz/authorizer_test.go`:

```go
package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type memoryRepository struct {
	tenantRoles map[string]string
	teamRoles   map[string]string
	runtimeOK   bool
	err         error
}

func (r *memoryRepository) GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error) {
	if r.err != nil {
		return Membership{}, r.err
	}
	role, ok := r.tenantRoles[params.TenantID.String()+":"+params.PrincipalType+":"+params.PrincipalID.String()]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return Membership{TenantID: params.TenantID, PrincipalType: params.PrincipalType, PrincipalID: params.PrincipalID, Role: role, Status: "active"}, nil
}

func (r *memoryRepository) GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error) {
	if r.err != nil {
		return Membership{}, r.err
	}
	role, ok := r.teamRoles[params.TenantID.String()+":"+params.TeamID.String()+":"+params.PrincipalType+":"+params.PrincipalID.String()]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return Membership{TenantID: params.TenantID, TeamID: &params.TeamID, PrincipalType: params.PrincipalType, PrincipalID: params.PrincipalID, Role: role, Status: "active"}, nil
}

func (r *memoryRepository) RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return r.runtimeOK, nil
}

func TestDBAuthorizerAllowsTenantOwnerConsoleAccess(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	repo := &memoryRepository{
		tenantRoles: map[string]string{
			tenantID.String() + ":user:" + userID.String(): RoleOwner,
		},
	}
	authorizer := NewDBAuthorizer(repo)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected console access to be allowed, got %#v", decision)
	}
	if decision.MatchedRule != "tenant.owner" {
		t.Fatalf("expected tenant.owner rule, got %q", decision.MatchedRule)
	}
}

func TestDBAuthorizerDeniesConsoleAccessWithoutMembership(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	authorizer := NewDBAuthorizer(&memoryRepository{tenantRoles: map[string]string{}})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no repository error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected console access to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonNoMembership {
		t.Fatalf("expected no membership reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerAllowsRuntimeClaimWhenScopeCoversTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: true})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected runtime claim to be allowed, got %#v", decision)
	}
	if decision.MatchedRule != "runtime.scope" {
		t.Fatalf("expected runtime.scope rule, got %q", decision.MatchedRule)
	}
}

func TestDBAuthorizerDeniesRuntimeClaimWhenScopeDoesNotCoverTask(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	authorizer := NewDBAuthorizer(&memoryRepository{runtimeOK: false})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorRuntimeNode, ID: "node-1"},
		Action:   ActionTaskClaim,
		Resource: ResourceRef{Type: ResourceTask, ID: "task-1"},
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected runtime claim to be denied, got %#v", decision)
	}
	if decision.Reason != ReasonRuntimeScopeMissing {
		t.Fatalf("expected missing runtime scope reason, got %q", decision.Reason)
	}
}

func TestDBAuthorizerReturnsRepositoryErrors(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	repoErr := errors.New("db unavailable")
	authorizer := NewDBAuthorizer(&memoryRepository{err: repoErr})

	_, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if !errors.Is(err, repoErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -run TestDBAuthorizer -count=1
```

Expected: FAIL because package `internal/authz` does not exist or types such as `NewDBAuthorizer` are undefined.

- [ ] **Step 3: Add authz types**

Create `apps/control-plane/internal/authz/types.go`:

```go
package authz

import (
	"errors"

	"github.com/google/uuid"
)

const (
	ActorUser        = "user"
	ActorRuntimeNode = "runtime_node"
	ActorEmployee    = "employee"
	ActorService     = "service_account"
)

const (
	ResourceConsole = "console"
	ResourceTenant  = "tenant"
	ResourceTeam    = "team"
	ResourceTask    = "task"
)

const (
	ActionConsoleAccess = "console.access"
	ActionTenantAccess  = "tenant.access"
	ActionTeamAccess    = "team.access"
	ActionTaskClaim     = "task.claim"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

const (
	ReasonAllowed             = "allowed"
	ReasonNoMembership        = "no active membership"
	ReasonInvalidActor        = "invalid actor"
	ReasonUnsupportedAction   = "unsupported action"
	ReasonRuntimeScopeMissing = "runtime scope does not cover task"
)

var (
	ErrNoMembership      = errors.New("no active membership")
	ErrUnsupportedAction = errors.New("unsupported action")
)

type ActorRef struct {
	Type string
	ID   string
}

type ResourceRef struct {
	Type string
	ID   string
}

type CheckRequest struct {
	Actor       ActorRef
	Action      string
	Resource    ResourceRef
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Context     map[string]any
	AuditReason string
}

type Decision struct {
	Allowed       bool
	Reason        string
	MatchedRule   string
	RequiresAudit bool
	Snapshot      map[string]any
}

type Membership struct {
	TenantID      uuid.UUID
	TeamID        *uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	Role          string
	Status        string
}
```

- [ ] **Step 4: Add repository interface**

Create `apps/control-plane/internal/authz/repository.go`:

```go
package authz

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error)
	GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error)
	RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error)
}

type TenantMembershipParams struct {
	TenantID      uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
}

type TeamMembershipParams struct {
	TenantID      uuid.UUID
	TeamID        uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
}

type RuntimeScopeParams struct {
	TenantID uuid.UUID
	TeamID   *uuid.UUID
	NodeID   string
}
```

- [ ] **Step 5: Add DB-backed authorizer implementation**

Create `apps/control-plane/internal/authz/authorizer.go`:

```go
package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Authorizer interface {
	Check(ctx context.Context, req CheckRequest) (Decision, error)
}

type DBAuthorizer struct {
	repository Repository
}

func NewDBAuthorizer(repository Repository) *DBAuthorizer {
	return &DBAuthorizer{repository: repository}
}

func (a *DBAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	if a == nil || a.repository == nil {
		return Decision{Allowed: false, Reason: "authorizer is not configured", RequiresAudit: true}, nil
	}
	switch req.Action {
	case ActionConsoleAccess, ActionTenantAccess:
		return a.checkTenantAccess(ctx, req)
	case ActionTeamAccess:
		return a.checkTeamAccess(ctx, req)
	case ActionTaskClaim:
		return a.checkRuntimeTaskClaim(ctx, req)
	default:
		return Decision{Allowed: false, Reason: ReasonUnsupportedAction, RequiresAudit: true}, ErrUnsupportedAction
	}
}

func (a *DBAuthorizer) checkTenantAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID:      req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if roleAllowsTenantAccess(membership.Role) {
		return allow("tenant."+membership.Role, membership.Role), nil
	}
	return deny(ReasonNoMembership), nil
}

func (a *DBAuthorizer) checkTeamAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	if req.TeamID == nil {
		return a.checkTenantAccess(ctx, req)
	}
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTeamMembership(ctx, TeamMembershipParams{
		TenantID:      req.TenantID,
		TeamID:        *req.TeamID,
		PrincipalType: ActorUser,
		PrincipalID:   principalID,
	})
	if err == nil && roleAllowsTenantAccess(membership.Role) {
		return allow("team."+membership.Role, membership.Role), nil
	}
	if err != nil && !errors.Is(err, ErrNoMembership) {
		return Decision{}, err
	}
	return a.checkTenantAccess(ctx, req)
}

func (a *DBAuthorizer) checkRuntimeTaskClaim(ctx context.Context, req CheckRequest) (Decision, error) {
	if req.Actor.Type != ActorRuntimeNode || req.Actor.ID == "" {
		return deny(ReasonInvalidActor), nil
	}
	covered, err := a.repository.RuntimeNodeCoversTaskScope(ctx, RuntimeScopeParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		NodeID:   req.Actor.ID,
	})
	if err != nil {
		return Decision{}, err
	}
	if !covered {
		return deny(ReasonRuntimeScopeMissing), nil
	}
	return Decision{
		Allowed:     true,
		Reason:      ReasonAllowed,
		MatchedRule: "runtime.scope",
		Snapshot: map[string]any{
			"engine": "db",
			"action": req.Action,
		},
	}, nil
}

func parseUUIDActor(actor ActorRef, expectedType string) (uuid.UUID, bool) {
	if actor.Type != expectedType {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(actor.ID)
	return id, err == nil
}

func roleAllowsTenantAccess(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func allow(rule string, role string) Decision {
	return Decision{
		Allowed:     true,
		Reason:      ReasonAllowed,
		MatchedRule: rule,
		Snapshot: map[string]any{
			"engine": "db",
			"role":   role,
		},
	}
}

func deny(reason string) Decision {
	return Decision{
		Allowed:       false,
		Reason:        reason,
		RequiresAudit: true,
		Snapshot: map[string]any{
			"engine": "db",
		},
	}
}
```

- [ ] **Step 6: Run authz tests**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```bash
git add apps/control-plane/internal/authz
git commit -m "feat: add authz domain contract"
```

## Task 2: Add DB Queries And PgRepository

**Files:**
- Create: `apps/control-plane/internal/storage/queries/authz.sql`
- Modify: generated files under `apps/control-plane/internal/storage/queries/`
- Create: `apps/control-plane/internal/authz/pg_repository.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`
- Test: `apps/control-plane/internal/authz/pg_repository_test.go`

- [ ] **Step 1: Write failing sqlc query tests**

Append to `apps/control-plane/internal/storage/queries/queries_test.go`:

```go
func TestAuthzTenantMembershipQueries(t *testing.T) {
	cleanupTestData(t, testDB)
	ctx := context.Background()

	user, err := testQueries.CreateUser(ctx, queries.CreateUserParams{
		Username:     "authz-owner",
		PasswordHash: "hash",
		Status:       "active",
	})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO tenant_members (tenant_id, principal_type, principal_id, role, status)
		VALUES (
			'00000000-0000-0000-0000-000000000001'::uuid,
			'user',
			$1,
			'owner',
			'active'
		)
	`, user.ID)
	require.NoError(t, err)

	membership, err := testQueries.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		PrincipalType: "user",
		PrincipalID:   user.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "owner", membership.Role)
	assert.Equal(t, "active", membership.Status)
}

func TestAuthzRuntimeNodeCoversTaskScope(t *testing.T) {
	cleanupTestData(t, testDB)
	ctx := context.Background()

	providersJSON, err := json.Marshal([]string{"codex"})
	require.NoError(t, err)
	node, err := testQueries.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             "authz-node-1",
		Name:               "Authz Node 1",
		SupportedProviders: providersJSON,
		MaxSlots:           1,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO runtime_node_scopes (
			tenant_id,
			runtime_node_id,
			team_id,
			scope_type,
			scope_value,
			status
		)
		VALUES (
			'00000000-0000-0000-0000-000000000001'::uuid,
			$1,
			'00000000-0000-0000-0000-000000000101'::uuid,
			'team',
			'00000000-0000-0000-0000-000000000101',
			'active'
		)
	`, node.ID)
	require.NoError(t, err)

	covered, err := testQueries.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		TeamID: uuid.NullUUID{
			UUID:  uuid.MustParse("00000000-0000-0000-0000-000000000101"),
			Valid: true,
		},
		NodeID: "authz-node-1",
	})
	require.NoError(t, err)
	assert.True(t, covered)
}
```

- [ ] **Step 2: Run query tests to verify they fail**

Run:

```bash
cd apps/control-plane
go test ./internal/storage/queries -run 'TestAuthz' -count=1
```

Expected: FAIL if integration DB is configured because generated methods do not exist. If test env is not configured, expected output is the package skip message documented in `internal/storage/queries/README.md`; continue with sqlc generation and unit tests.

- [ ] **Step 3: Add authz SQL queries**

Create `apps/control-plane/internal/storage/queries/authz.sql`:

```sql
-- name: GetActiveTenantMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id IS NULL
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'member' THEN 3
    WHEN 'viewer' THEN 4
    ELSE 5
  END
LIMIT 1;

-- name: GetActiveTeamMembership :one
SELECT *
FROM tenant_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND team_id = sqlc.arg('team_id')::uuid
  AND principal_type = sqlc.arg('principal_type')::varchar
  AND principal_id = sqlc.arg('principal_id')::uuid
  AND status = 'active'
  AND disabled_at IS NULL
ORDER BY
  CASE role
    WHEN 'owner' THEN 1
    WHEN 'admin' THEN 2
    WHEN 'member' THEN 3
    WHEN 'viewer' THEN 4
    ELSE 5
  END
LIMIT 1;

-- name: RuntimeNodeCoversTaskScope :one
SELECT EXISTS (
  SELECT 1
  FROM runtime_nodes rn
  JOIN runtime_node_scopes rns ON rns.runtime_node_id = rn.id
  WHERE rn.node_id = sqlc.arg('node_id')::varchar
    AND rn.tenant_id = sqlc.arg('tenant_id')::uuid
    AND rn.disabled_at IS NULL
    AND rn.archived_at IS NULL
    AND rns.tenant_id = sqlc.arg('tenant_id')::uuid
    AND rns.status = 'active'
    AND rns.disabled_at IS NULL
    AND (
      rns.team_id IS NULL
      OR sqlc.narg('team_id')::uuid IS NULL
      OR rns.team_id = sqlc.narg('team_id')::uuid
    )
    AND (
      rns.scope_type = 'tenant'
      OR (
        rns.scope_type = 'team'
        AND sqlc.narg('team_id')::uuid IS NOT NULL
        AND rns.scope_value = sqlc.narg('team_id')::uuid::text
      )
    )
);
```

- [ ] **Step 4: Generate sqlc code**

Run:

```bash
cd apps/control-plane
make generate-sqlc
```

Expected: generated files in `apps/control-plane/internal/storage/queries/` include authz query methods. `apps/control-plane/Makefile` already defines `generate-sqlc` as `sqlc generate`, so this is the repo-native command for this task.

- [ ] **Step 5: Add PgRepository tests**

Create `apps/control-plane/internal/authz/pg_repository_test.go`:

```go
package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type fakeQueries struct {
	tenant queries.TenantMember
	team   queries.TenantMember
	scope  bool
	err    error
}

func (q *fakeQueries) GetActiveTenantMembership(ctx context.Context, params queries.GetActiveTenantMembershipParams) (queries.TenantMember, error) {
	if q.err != nil {
		return queries.TenantMember{}, q.err
	}
	return q.tenant, nil
}

func (q *fakeQueries) GetActiveTeamMembership(ctx context.Context, params queries.GetActiveTeamMembershipParams) (queries.TenantMember, error) {
	if q.err != nil {
		return queries.TenantMember{}, q.err
	}
	return q.team, nil
}

func (q *fakeQueries) RuntimeNodeCoversTaskScope(ctx context.Context, params queries.RuntimeNodeCoversTaskScopeParams) (bool, error) {
	if q.err != nil {
		return false, q.err
	}
	return q.scope, nil
}

func TestPgRepositoryMapsTenantMembership(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	repo := NewPgRepository(&fakeQueries{
		tenant: queries.TenantMember{
			TenantID:      tenantID,
			PrincipalType: "user",
			PrincipalID:   userID,
			Role:          "owner",
			Status:        "active",
		},
	})

	membership, err := repo.GetActiveTenantMembership(context.Background(), TenantMembershipParams{
		TenantID:      tenantID,
		PrincipalType: "user",
		PrincipalID:   userID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if membership.Role != "owner" || membership.PrincipalID != userID {
		t.Fatalf("unexpected membership: %#v", membership)
	}
}

func TestPgRepositoryMapsNoRowsToNoMembership(t *testing.T) {
	repo := NewPgRepository(&fakeQueries{err: pgx.ErrNoRows})

	_, err := repo.GetActiveTenantMembership(context.Background(), TenantMembershipParams{
		TenantID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		PrincipalType: "user",
		PrincipalID:   uuid.MustParse("00000000-0000-4000-8000-000000000002"),
	})

	if !errors.Is(err, ErrNoMembership) {
		t.Fatalf("expected ErrNoMembership, got %v", err)
	}
}
```

- [ ] **Step 6: Add PgRepository implementation**

Create `apps/control-plane/internal/authz/pg_repository.go`:

```go
package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type QueryStore interface {
	GetActiveTenantMembership(ctx context.Context, params queries.GetActiveTenantMembershipParams) (queries.TenantMember, error)
	GetActiveTeamMembership(ctx context.Context, params queries.GetActiveTeamMembershipParams) (queries.TenantMember, error)
	RuntimeNodeCoversTaskScope(ctx context.Context, params queries.RuntimeNodeCoversTaskScopeParams) (bool, error)
}

type PgRepository struct {
	q QueryStore
}

func NewPgRepository(q QueryStore) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) GetActiveTenantMembership(ctx context.Context, params TenantMembershipParams) (Membership, error) {
	row, err := r.q.GetActiveTenantMembership(ctx, queries.GetActiveTenantMembershipParams{
		TenantID:      params.TenantID,
		PrincipalType: params.PrincipalType,
		PrincipalID:   params.PrincipalID,
	})
	return membershipFromRow(row), mapNoRows(err)
}

func (r *PgRepository) GetActiveTeamMembership(ctx context.Context, params TeamMembershipParams) (Membership, error) {
	row, err := r.q.GetActiveTeamMembership(ctx, queries.GetActiveTeamMembershipParams{
		TenantID:      params.TenantID,
		TeamID:        params.TeamID,
		PrincipalType: params.PrincipalType,
		PrincipalID:   params.PrincipalID,
	})
	return membershipFromRow(row), mapNoRows(err)
}

func (r *PgRepository) RuntimeNodeCoversTaskScope(ctx context.Context, params RuntimeScopeParams) (bool, error) {
	teamID := uuid.NullUUID{}
	if params.TeamID != nil {
		teamID = uuid.NullUUID{UUID: *params.TeamID, Valid: true}
	}
	return r.q.RuntimeNodeCoversTaskScope(ctx, queries.RuntimeNodeCoversTaskScopeParams{
		TenantID: params.TenantID,
		TeamID:   teamID,
		NodeID:   params.NodeID,
	})
}

func membershipFromRow(row queries.TenantMember) Membership {
	var teamID *uuid.UUID
	if row.TeamID.Valid {
		teamID = &row.TeamID.UUID
	}
	return Membership{
		TenantID:      row.TenantID,
		TeamID:        teamID,
		PrincipalType: row.PrincipalType,
		PrincipalID:   row.PrincipalID,
		Role:          row.Role,
		Status:        row.Status,
	}
}

func mapNoRows(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoMembership
	}
	return err
}
```

- [ ] **Step 7: Run focused tests**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -count=1
go test ./internal/storage/queries -run 'TestAuthz' -count=1
```

Expected: `./internal/authz` passes. Query integration tests either pass with configured test DB or skip with documented message when no test DB is configured.

- [ ] **Step 8: Commit Task 2**

```bash
git add apps/control-plane/internal/authz apps/control-plane/internal/storage/queries
git commit -m "feat: add db-backed authz queries"
```

## Task 3: Gate Current User With Console Authorization

**Files:**
- Modify: `apps/control-plane/internal/auth/handler.go`
- Modify: `apps/control-plane/internal/auth/service.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Test: `apps/control-plane/internal/api/routes_test.go`

- [ ] **Step 1: Write failing route test for denied `/api/auth/me`**

Append to `apps/control-plane/internal/api/routes_test.go`:

```go
func TestCurrentUserRequiresConsoleAuthorization(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: false},
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(loginResp.Result().Cookies()[0])
	meResp := httptest.NewRecorder()
	server.ServeHTTP(meResp, meReq)

	if meResp.Code != http.StatusForbidden {
		t.Fatalf("expected current user to be forbidden, got %d: %s", meResp.Code, meResp.Body.String())
	}
}
```

Also add this helper type to the bottom of `routes_test.go`:

```go
type routeAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *routeAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "test.allow"}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership, RequiresAudit: true}, nil
}
```

Add import:

```go
	"github.com/superteam/control-plane/internal/authz"
```

- [ ] **Step 2: Run route test to verify it fails**

Run:

```bash
cd apps/control-plane
go test ./internal/api -run TestCurrentUserRequiresConsoleAuthorization -count=1
```

Expected: FAIL because `NewServerWithAuthz` does not exist and auth handler does not check authorization.

- [ ] **Step 3: Add current user tenant context to auth service**

Modify `apps/control-plane/internal/auth/service.go`:

```go
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

type CurrentUserContext struct {
	User     *User
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

func (s *Service) GetCurrentUserContext(ctx context.Context, token string) (*CurrentUserContext, error) {
	_, user, err := s.GetUserBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	tenantID := uuid.MustParse(DefaultTenantID)
	return &CurrentUserContext{
		User:     user,
		TenantID: tenantID,
	}, nil
}
```

This task intentionally uses the default tenant as the MVP bridge. Dynamic tenant selection belongs to the later permission configuration plan.

- [ ] **Step 4: Inject Authorizer into auth handler**

Modify `apps/control-plane/internal/auth/handler.go`:

```go
import (
	// existing imports...
	"github.com/superteam/control-plane/internal/authz"
)

type HTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewHandler(service *Service, authorizer ...authz.Authorizer) *HTTPHandler {
	var az authz.Authorizer
	if len(authorizer) > 0 {
		az = authorizer[0]
	}
	return &HTTPHandler{service: service, authorizer: az}
}
```

Replace `GetCurrentUser` with:

```go
func (h *HTTPHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	current, err := h.currentUserContext(r)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	if h.authorizer != nil {
		decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
			Actor: authz.ActorRef{
				Type: authz.ActorUser,
				ID:   current.User.ID.String(),
			},
			Action: authz.ActionConsoleAccess,
			Resource: authz.ResourceRef{
				Type: authz.ResourceConsole,
				ID:   "web",
			},
			TenantID:    current.TenantID,
			TeamID:      current.TeamID,
			AuditReason: "current user console access",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if !decision.Allowed {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	writeJSON(w, http.StatusOK, CurrentUserResponse{User: toGeneratedUserSummary(current.User)})
}
```

Add helper:

```go
func (h *HTTPHandler) currentUserContext(r *http.Request) (*CurrentUserContext, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return h.service.GetCurrentUserContext(r.Context(), cookie.Value)
}
```

Keep `currentSessionUser` for existing routes until those routes are authorized in a later task.

- [ ] **Step 5: Wire Authorizer through API server**

Modify `apps/control-plane/internal/api/server.go`:

```go
import (
	// existing imports...
	"github.com/superteam/control-plane/internal/authz"
)

type Server struct {
	// existing fields...
	authorizer authz.Authorizer
}

func NewServerWithAuthz(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	authService *auth.Service,
	runtimeAuthService middleware.AuthService,
	authorizer authz.Authorizer,
) *Server {
	server := NewServer(taskHandler, runtimeHandler, runtimeAuthService)
	server.authService = authService
	server.authorizer = authorizer
	if authService != nil {
		auth.HandlerFromMux(auth.NewHandler(authService, authorizer), server.router)
	}
	return server
}

func NewServerWithAuth(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, authService *auth.Service, runtimeAuthService ...middleware.AuthService) *Server {
	var runtimeAuth middleware.AuthService
	if len(runtimeAuthService) > 0 {
		runtimeAuth = runtimeAuthService[0]
	}
	return NewServerWithAuthz(taskHandler, runtimeHandler, authService, runtimeAuth, nil)
}
```

Use `NewServerWithAuthz` only where authorization is needed. Keep existing `NewServerWithAuth` as a compatibility wrapper.

- [ ] **Step 6: Wire container**

Modify `apps/control-plane/internal/app/app.go`:

```go
authzRepository := authz.NewPgRepository(q)
authorizer := authz.NewDBAuthorizer(authzRepository)
runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller)
server := api.NewServerWithAuthz(taskHandler, runtimeHandler, authService, authService, authorizer)
```

Add import:

```go
	"github.com/superteam/control-plane/internal/authz"
```

Add `Authorizer authz.Authorizer` to `Container`.

- [ ] **Step 7: Run focused tests**

Run:

```bash
cd apps/control-plane
go test ./internal/api -run 'TestAuthRoutesAreRegistered|TestCurrentUserRequiresConsoleAuthorization' -count=1
go test ./internal/auth -run 'TestLoginCreatesSessionAndReturnsCurrentUser' -count=1
```

Expected: PASS. Existing auth route test still succeeds when no authorizer is injected; new route test returns 403 when authorizer denies.

- [ ] **Step 8: Commit Task 3**

```bash
git add apps/control-plane/internal/auth apps/control-plane/internal/api apps/control-plane/internal/app
git commit -m "feat: gate current user with authz"
```

## Task 4: Gate Runtime Claim With Runtime Scope

**Files:**
- Modify: `apps/control-plane/internal/api/handlers/runtime.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Test: `apps/control-plane/internal/api/handlers/runtime_test.go`

- [ ] **Step 1: Write failing runtime claim denial test**

Append to `apps/control-plane/internal/api/handlers/runtime_test.go`:

```go
func TestClaimTaskSkipsTaskOutsideRuntimeScope(t *testing.T) {
	tenantID := handlerTestUUID(1)
	teamID := handlerTestUUID(101)
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	blockedTask := &task.Task{
		ID:           handlerTestUUID(200),
		TenantID:     tenantID,
		TeamID:       &teamID,
		ProviderType: "codex",
	}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {blockedTask},
		},
	}
	authorizer := &claimAuthorizer{allowed: false}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content when task is outside scope, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assignment, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	if authorizer.checks[0].Action != authz.ActionTaskClaim {
		t.Fatalf("expected task.claim action, got %#v", authorizer.checks[0])
	}
}
```

Add helper:

```go
type claimAuthorizer struct {
	allowed bool
	checks  []authz.CheckRequest
}

func (a *claimAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	return authz.Decision{Allowed: a.allowed, Reason: authz.ReasonAllowed}, nil
}
```

Add import:

```go
	"github.com/superteam/control-plane/internal/authz"
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd apps/control-plane
go test ./internal/api/handlers -run TestClaimTaskSkipsTaskOutsideRuntimeScope -count=1
```

Expected: FAIL because `NewRuntimeHandler` does not accept an authorizer and claim does not check Runtime scope.

- [ ] **Step 3: Inject authorizer into RuntimeHandler**

Modify `apps/control-plane/internal/api/handlers/runtime.go`:

```go
import (
	// existing imports...
	"github.com/superteam/control-plane/internal/authz"
)

type RuntimeHandler struct {
	runtimeService RuntimeService
	taskService    TaskService
	poller         Poller
	authorizer     authz.Authorizer
}

func NewRuntimeHandler(runtimeService RuntimeService, taskService TaskService, poller Poller, authorizer ...authz.Authorizer) *RuntimeHandler {
	var az authz.Authorizer
	if len(authorizer) > 0 {
		az = authorizer[0]
	}
	return &RuntimeHandler{
		runtimeService: runtimeService,
		taskService:    taskService,
		poller:         poller,
		authorizer:     az,
	}
}
```

Existing call sites continue to compile because the new argument is variadic.

- [ ] **Step 4: Add task claim authorization helper**

Add to `runtime.go`:

```go
func (h *RuntimeHandler) runtimeCanClaim(ctx context.Context, nodeID string, t *task.Task) (bool, error) {
	if h.authorizer == nil {
		return true, nil
	}
	decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorRuntimeNode,
			ID:   nodeID,
		},
		Action: authz.ActionTaskClaim,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTask,
			ID:   t.ID.String(),
		},
		TenantID:    t.TenantID,
		TeamID:      t.TeamID,
		AuditReason: "runtime task claim",
	})
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}
```

- [ ] **Step 5: Apply authorization while selecting candidate**

Replace the inner loop in `ClaimTask`:

```go
for _, t := range tasks {
	allowed, err := h.runtimeCanClaim(ctx, nodeID, t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		continue
	}
	if bestClaimCandidate(candidate, t) == t {
		candidate = t
	}
}
```

Also check the poller result before assign:

```go
allowed, err := h.runtimeCanClaim(ctx, nodeID, t)
if err != nil {
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return
}
if !allowed {
	w.WriteHeader(http.StatusNoContent)
	return
}
```

Place this after provider compatibility check and before `h.assignTask(ctx, w, t, nodeID)`.

- [ ] **Step 6: Wire container runtime handler with authorizer**

Modify `apps/control-plane/internal/app/app.go` so runtime handler receives the same authorizer:

```go
runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller, authorizer)
```

- [ ] **Step 7: Run focused runtime tests**

Run:

```bash
cd apps/control-plane
go test ./internal/api/handlers -run 'TestClaimTask' -count=1
go test ./internal/api -run 'TestRuntimeRoutes' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

```bash
git add apps/control-plane/internal/api/handlers apps/control-plane/internal/app apps/control-plane/internal/api
git commit -m "feat: gate runtime claim with authz scope"
```

## Task 5: Persist Authorization Decision Audit Records

**Files:**
- Modify: `apps/control-plane/internal/authz/types.go`
- Modify: `apps/control-plane/internal/authz/authorizer.go`
- Create: `apps/control-plane/internal/authz/decision_recorder.go`
- Test: `apps/control-plane/internal/authz/authorizer_test.go`
- Test: `apps/control-plane/internal/authz/decision_recorder_test.go`

- [ ] **Step 1: Write failing audit recorder test**

Append to `apps/control-plane/internal/authz/authorizer_test.go`:

```go
type memoryRecorder struct {
	records []DecisionRecord
}

func (r *memoryRecorder) RecordDecision(ctx context.Context, record DecisionRecord) error {
	r.records = append(r.records, record)
	return nil
}

func TestDBAuthorizerRecordsDeniedDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000004")
	recorder := &memoryRecorder{}
	authorizer := NewDBAuthorizer(&memoryRepository{tenantRoles: map[string]string{}}, recorder)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionConsoleAccess,
		Resource: ResourceRef{Type: ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denial, got %#v", decision)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("expected one decision record, got %#v", recorder.records)
	}
	record := recorder.records[0]
	if record.ActorType != ActorUser || record.Action != ActionConsoleAccess || record.Allowed {
		t.Fatalf("unexpected decision record: %#v", record)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -run TestDBAuthorizerRecordsDeniedDecision -count=1
```

Expected: FAIL because `DecisionRecord`, `DecisionRecorder`, and `NewDBAuthorizer` recorder support do not exist.

- [ ] **Step 3: Add decision recorder types**

Modify `apps/control-plane/internal/authz/types.go` imports:

```go
import (
	"context"
	"errors"

	"github.com/google/uuid"
)
```

Add these types:

```go
type DecisionRecord struct {
	TenantID     uuid.UUID
	TeamID       *uuid.UUID
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Allowed      bool
	Reason       string
	MatchedRule  string
	Engine       string
	Snapshot     map[string]any
}

type DecisionRecorder interface {
	RecordDecision(ctx context.Context, record DecisionRecord) error
}
```

- [ ] **Step 4: Record decisions from DBAuthorizer**

Modify `apps/control-plane/internal/authz/authorizer.go`:

```go
type DBAuthorizer struct {
	repository Repository
	recorder   DecisionRecorder
}

func NewDBAuthorizer(repository Repository, recorder ...DecisionRecorder) *DBAuthorizer {
	var r DecisionRecorder
	if len(recorder) > 0 {
		r = recorder[0]
	}
	return &DBAuthorizer{repository: repository, recorder: r}
}
```

Wrap each decision before returning from `Check`:

```go
func (a *DBAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	var decision Decision
	var err error
	if a == nil || a.repository == nil {
		decision = Decision{Allowed: false, Reason: "authorizer is not configured", RequiresAudit: true}
		return decision, nil
	}
	switch req.Action {
	case ActionConsoleAccess, ActionTenantAccess:
		decision, err = a.checkTenantAccess(ctx, req)
	case ActionTeamAccess:
		decision, err = a.checkTeamAccess(ctx, req)
	case ActionTaskClaim:
		decision, err = a.checkRuntimeTaskClaim(ctx, req)
	default:
		decision, err = Decision{Allowed: false, Reason: ReasonUnsupportedAction, RequiresAudit: true}, ErrUnsupportedAction
	}
	if err != nil {
		return decision, err
	}
	if recordErr := a.record(ctx, req, decision); recordErr != nil {
		return Decision{}, recordErr
	}
	return decision, nil
}

func (a *DBAuthorizer) record(ctx context.Context, req CheckRequest, decision Decision) error {
	if a.recorder == nil {
		return nil
	}
	return a.recorder.RecordDecision(ctx, DecisionRecord{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		ActorType:    req.Actor.Type,
		ActorID:      req.Actor.ID,
		Action:       req.Action,
		ResourceType: req.Resource.Type,
		ResourceID:   req.Resource.ID,
		Allowed:      decision.Allowed,
		Reason:       decision.Reason,
		MatchedRule:  decision.MatchedRule,
		Engine:       "db",
		Snapshot:     decision.Snapshot,
	})
	}
	```

- [ ] **Step 5: Write failing persistent recorder test**

Create `apps/control-plane/internal/authz/decision_recorder_test.go`:

```go
package authz

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type fakeOperationLogQueries struct {
	params queries.CreateWebOperationLogParams
	called bool
}

func (q *fakeOperationLogQueries) CreateWebOperationLog(ctx context.Context, params queries.CreateWebOperationLogParams) (queries.WebOperationLog, error) {
	q.params = params
	q.called = true
	return queries.WebOperationLog{}, nil
}

func TestOperationLogDecisionRecorderPersistsDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	query := &fakeOperationLogQueries{}
	recorder := NewOperationLogDecisionRecorder(query)

	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID:     tenantID,
		ActorType:    ActorUser,
		ActorID:      "00000000-0000-4000-8000-000000000001",
		Action:       ActionConsoleAccess,
		ResourceType: ResourceConsole,
		ResourceID:   "web",
		Allowed:      false,
		Reason:       ReasonNoMembership,
		MatchedRule:  "",
		Engine:       "db",
		Snapshot: map[string]any{
			"engine": "db",
		},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !query.called {
		t.Fatal("expected operation log query to be called")
	}
	if query.params.Module != ModuleAuthz {
		t.Fatalf("expected authz module, got %q", query.params.Module)
	}
	if query.params.Action != ActionConsoleAccess {
		t.Fatalf("expected console access action, got %q", query.params.Action)
	}
	if query.params.Result != ResultFailed {
		t.Fatalf("expected failed result for denied decision, got %q", query.params.Result)
	}
	if len(query.params.Details) == 0 {
		t.Fatal("expected details json to be present")
	}
	var details map[string]any
	if err := json.Unmarshal(query.params.Details, &details); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	if details["reason"] != ReasonNoMembership || details["engine"] != "db" {
		t.Fatalf("unexpected details: %#v", details)
	}
}

func TestOperationLogDecisionRecorderUsesSucceededResultForAllowedDecision(t *testing.T) {
	query := &fakeOperationLogQueries{}
	recorder := NewOperationLogDecisionRecorder(query)

	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID:     uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ActorType:    ActorRuntimeNode,
		ActorID:      "node-1",
		Action:       ActionTaskClaim,
		ResourceType: ResourceTask,
		ResourceID:   "00000000-0000-4000-8000-000000000042",
		Allowed:      true,
		Reason:       ReasonAllowed,
		MatchedRule:  "runtime.scope",
		Engine:       "db",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if query.params.Result != ResultSucceeded {
		t.Fatalf("expected succeeded result, got %q", query.params.Result)
	}
}

func TestOperationLogDecisionRecorderNilQueryIsNoop(t *testing.T) {
	recorder := NewOperationLogDecisionRecorder(nil)
	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Action:   ActionConsoleAccess,
	})
	if err != nil {
		t.Fatalf("expected nil query recorder to be a no-op, got %v", err)
	}
}

```

- [ ] **Step 6: Run persistent recorder test to verify it fails**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -run TestOperationLogDecisionRecorder -count=1
```

Expected: FAIL because `NewOperationLogDecisionRecorder`, `ModuleAuthz`, `ResultFailed`, and `ResultSucceeded` do not exist.

- [ ] **Step 7: Add persistent recorder implementation**

Create `apps/control-plane/internal/authz/decision_recorder.go`:

```go
package authz

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const (
	ModuleAuthz     = "authz"
	ResultSucceeded = "succeeded"
	ResultFailed    = "failed"
)

type OperationLogQueries interface {
	CreateWebOperationLog(ctx context.Context, params queries.CreateWebOperationLogParams) (queries.WebOperationLog, error)
}

type OperationLogDecisionRecorder struct {
	q OperationLogQueries
}

func NewOperationLogDecisionRecorder(q OperationLogQueries) *OperationLogDecisionRecorder {
	return &OperationLogDecisionRecorder{q: q}
}

func (r *OperationLogDecisionRecorder) RecordDecision(ctx context.Context, record DecisionRecord) error {
	if r == nil || r.q == nil {
		return nil
	}
	details, err := json.Marshal(map[string]any{
		"allowed":      record.Allowed,
		"reason":       record.Reason,
		"matched_rule": record.MatchedRule,
		"engine":       record.Engine,
		"snapshot":     record.Snapshot,
	})
	if err != nil {
		return err
	}
	return r.q.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID:     uuid.NullUUID{UUID: record.TenantID, Valid: record.TenantID != uuid.Nil},
		UserID:       userIDFromRecord(record),
		Username:     pgtype.Text{},
		Module:       ModuleAuthz,
		ResourceType: text(record.ResourceType),
		ResourceID:   text(record.ResourceID),
		Action:       record.Action,
		Result:       result(record.Allowed),
		RequestID:    pgtype.Text{},
		ClientIp:     pgtype.Text{},
		UserAgent:    pgtype.Text{},
		Details:      details,
	})
}

func userIDFromRecord(record DecisionRecord) uuid.NullUUID {
	if record.ActorType != ActorUser {
		return uuid.NullUUID{}
	}
	id, err := uuid.Parse(record.ActorID)
	if err != nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func result(allowed bool) string {
	if allowed {
		return ResultSucceeded
	}
	return ResultFailed
}
```

- [ ] **Step 8: Wire recorder in app container**

Modify `apps/control-plane/internal/app/app.go`:

```go
authzRepository := authz.NewPgRepository(q)
authzRecorder := authz.NewOperationLogDecisionRecorder(q)
authorizer := authz.NewDBAuthorizer(authzRepository, authzRecorder)
```

- [ ] **Step 9: Run authz tests**

Run:

```bash
cd apps/control-plane
go test ./internal/authz -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit Task 5**

```bash
git add apps/control-plane/internal/authz apps/control-plane/internal/app
git commit -m "feat: persist authz decision logs"
```

## Task 6: Changelog And Regression Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update changelog**

Add an entry near the top of `CHANGELOG.md`:

```markdown
## 2026-06-01

- 新增 Control Plane 渐进式授权边界：`internal/authz` 统一 `Authorizer` 接口，第一版使用 PostgreSQL 权限事实判断 Web 控制台访问和 Runtime claim 范围。
- `/api/auth/me` 登录后增加 `console.access` 授权检查，认证和授权保持分层。
- Runtime claim 任务前增加 `task.claim` 范围检查，Runtime 节点不能领取超出 `runtime_node_scopes` 的任务。
```

If `CHANGELOG.md` already has a 2026-06-01 section, append these bullets to that section instead of creating a duplicate date section.

- [ ] **Step 2: Run focused Go tests**

Run:

```bash
cd apps/control-plane
go test ./internal/authz ./internal/auth ./internal/api ./internal/api/handlers -count=1
```

Expected: PASS.

- [ ] **Step 3: Run storage query tests**

Run:

```bash
cd apps/control-plane
go test ./internal/storage/queries -run 'TestAuthz' -count=1
```

Expected: PASS if `TEST_DATABASE_URL` and `TEST_REDIS_URL` are configured. If not configured, expected output is the documented skip message:

```text
skipping storage query integration tests: set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL
```

- [ ] **Step 4: Run broader control-plane tests**

Run:

```bash
cd apps/control-plane
go test ./... -count=1
```

Expected: PASS, except storage integration package may skip when external test DB/Redis env is absent.

- [ ] **Step 5: Inspect git diff**

Run:

```bash
git status --short
git diff --stat
```

Expected: only authz implementation, generated sqlc files, API/app wiring, tests, and changelog are modified. No unrelated `.env`, `.scratch`, local config, or ROADMAP changes should be staged.

- [ ] **Step 6: Commit Task 6**

```bash
git add CHANGELOG.md
git commit -m "docs: record authz rollout"
```

## Final Verification

Run from repo root:

```bash
git log --oneline -6
git status --short
cd apps/control-plane && go test ./internal/authz ./internal/auth ./internal/api ./internal/api/handlers -count=1
```

Expected:

- Recent commits show each task boundary.
- Working tree contains no unstaged files from this implementation except pre-existing user changes.
- Focused Go tests pass.
- If storage query integration env is configured, `go test ./internal/storage/queries -run 'TestAuthz' -count=1` passes; otherwise the package skip is reported explicitly.

## Follow-Up Plans

Create separate specs/plans for:

- 权限管理 Web 页面和配置 API。
- Capability authorization 表与调用链路。
- OpenFGA store/model/tuple 同步 backend。
- 授权决策审计视图。
