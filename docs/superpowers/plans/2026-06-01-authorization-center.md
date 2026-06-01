# Authorization Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 SuperTeam Web 一级菜单“权限中心”，提供真实授权审计、Runtime 范围配置、成员角色只读视图和权限诊断。

**Architecture:** 保持 `internal/authz` 只负责授权判断，新增 `internal/authzcenter` 作为权限中心 API 的应用服务层，聚合 `web_operation_logs`、`runtime_nodes`、`runtime_node_scopes`、`auth_users`、`tenant_members` 等数据。Web 使用现有 React/Vite + TanStack Query + shadcn-admin 布局，通过真实 `/api/authz/*` API 渲染五个 Tab；OpenFGA 仍是后端演进方向，不暴露到首版业务 UI。

**Tech Stack:** Go 1.25, chi/net/http, pgx/v5, sqlc, PostgreSQL, oapi-codegen, React 19, Vite, TanStack Router, TanStack Query, TanStack Table, shadcn/ui, lucide-react, Vitest browser tests.

---

## Scope

本计划实现 spec 中的混合 MVP：

- 新增 Web 一级菜单“权限中心”，路径 `/permissions`。
- 实现五个 Tab：授权概览、授权审计、Runtime 范围、成员角色、权限诊断。
- 授权审计读取真实 `web_operation_logs` 中 `module = 'authz'` 的记录。
- Runtime 范围读取和写入真实 `runtime_node_scopes`。
- 成员角色首版只读，展示用户、账号状态、租户/团队角色和最近拒绝。
- 权限诊断通过统一 `Authorizer.Check` dry-run 返回 decision。
- 所有 Runtime scope 写操作写入 `web_operation_logs`。
- 更新 `CHANGELOG.md`。

本计划不实现：

- 完整 IAM。
- OpenFGA 模型编辑器或 tuple 管理。
- 用户邀请、成员移除和任务转交。
- 数字员工授权编辑。
- Capability 授权编辑。
- 审计导出报表。
- 登录认证链路重构。

## File Structure

### Contracts and Generated Code

- Create: `contracts/control-plane/authz.yaml`
  - 权限中心 API 契约，包含 overview、decisions、runtime scopes、members、check。
- Modify: `apps/control-plane/Makefile`
  - `generate-openapi` 同时生成 `internal/auth/generated.go` 和 `internal/authzcenter/generated.go`。
- Regenerate: `apps/control-plane/internal/authzcenter/generated.go`
  - oapi-codegen 生成的权限中心 server/types。

### SQL and Backend

- Create: `apps/control-plane/internal/storage/queries/authz_center.sql`
  - 权限中心查询和 Runtime scope 写入 sqlc queries。
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
  - sqlc 生成代码。
- Create: `apps/control-plane/internal/authzcenter/types.go`
  - 权限中心领域类型和 action 常量。
- Create: `apps/control-plane/internal/authzcenter/repository.go`
  - Repository 接口。
- Create: `apps/control-plane/internal/authzcenter/pg_repository.go`
  - sqlc-backed repository。
- Create: `apps/control-plane/internal/authzcenter/service.go`
  - overview、decision 查询、runtime scope 写入、members、dry-run check。
- Create: `apps/control-plane/internal/authzcenter/handler.go`
  - HTTP handler，实现 generated server interface。
- Create: `apps/control-plane/internal/authzcenter/service_test.go`
  - 使用内存 repository 和 fake authorizer 做服务层测试。
- Modify: `apps/control-plane/internal/api/server.go`
  - 注入并注册 authzcenter handler。
- Modify: `apps/control-plane/internal/app/app.go`
  - 构造 authzcenter repository/service/handler。
- Modify: `apps/control-plane/internal/api/routes_test.go`
  - 覆盖权限中心路由注册、认证拒绝和写操作授权检查。

### Web

- Create: `apps/web/src/lib/api/authz.ts`
  - 权限中心 API client 和类型。
- Create: `apps/web/src/lib/api/authz.test.ts`
  - API client URL、method、credentials、body 测试。
- Modify: `apps/web/src/lib/api/index.ts`
  - 导出 authz client。
- Create: `apps/web/src/routes/_authenticated/permissions/index.tsx`
  - TanStack Router route。
- Create: `apps/web/src/features/permissions/index.tsx`
  - 权限中心页面容器。
- Create: `apps/web/src/features/permissions/components/authorization-overview.tsx`
- Create: `apps/web/src/features/permissions/components/authorization-audit-table.tsx`
- Create: `apps/web/src/features/permissions/components/runtime-scopes.tsx`
- Create: `apps/web/src/features/permissions/components/member-roles.tsx`
- Create: `apps/web/src/features/permissions/components/permission-diagnostics.tsx`
- Create: `apps/web/src/features/permissions/index.test.tsx`
  - 渲染五个 Tab、空态、错误态和诊断表单基本行为。
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
  - 增加“权限中心”一级菜单。
- Regenerate: `apps/web/src/routeTree.gen.ts`
  - Vite/TanStack Router 插件生成。

### Docs

- Modify: `CHANGELOG.md`
  - 记录权限中心 Web + Control Plane API 变更。

---

## Task 1: Add Authz Center OpenAPI Contract

**Files:**
- Create: `contracts/control-plane/authz.yaml`
- Modify: `apps/control-plane/Makefile`
- Regenerate: `apps/control-plane/internal/authzcenter/generated.go`

- [ ] **Step 1: Write the authz center contract**

Create `contracts/control-plane/authz.yaml`:

```yaml
openapi: 3.0.3
info:
  title: SuperTeam Control Plane - Authz Center API
  version: 1.0.0
  description: 权限中心 API

servers:
  - url: http://localhost:8080
    description: 本地开发环境

paths:
  /api/authz/overview:
    get:
      summary: 获取权限中心概览
      operationId: getAuthzOverview
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/AuthzOverviewResponse"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"

  /api/authz/decisions:
    get:
      summary: 查询授权审计记录
      operationId: listAuthzDecisions
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      parameters:
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
        - name: result
          in: query
          required: false
          schema:
            type: string
            enum: [succeeded, failed]
        - name: action
          in: query
          required: false
          schema:
            type: string
        - name: actor_type
          in: query
          required: false
          schema:
            type: string
        - name: actor_id
          in: query
          required: false
          schema:
            type: string
        - name: resource_type
          in: query
          required: false
          schema:
            type: string
        - name: resource_id
          in: query
          required: false
          schema:
            type: string
        - name: request_id
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/AuthzDecisionListResponse"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"

  /api/authz/runtime-scopes:
    get:
      summary: 查询 Runtime 节点授权范围
      operationId: listRuntimeScopes
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeScopeListResponse"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
    post:
      summary: 新增 Runtime 节点授权范围
      operationId: createRuntimeScope
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateRuntimeScopeRequest"
      responses:
        "201":
          description: 创建成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeScopeResponse"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"

  /api/authz/runtime-scopes/{scope_id}:
    patch:
      summary: 启用或禁用 Runtime 节点授权范围
      operationId: updateRuntimeScope
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      parameters:
        - name: scope_id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/UpdateRuntimeScopeRequest"
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/RuntimeScopeResponse"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
        "404":
          $ref: "#/components/responses/NotFound"

  /api/authz/members:
    get:
      summary: 查询成员角色只读视图
      operationId: listAuthzMembers
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      parameters:
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/AuthzMemberListResponse"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"

  /api/authz/check:
    post:
      summary: 权限诊断 dry-run
      operationId: checkPermission
      tags: [AuthzCenter]
      security:
        - cookieAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CheckPermissionRequest"
      responses:
        "200":
          description: 成功
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/CheckPermissionResponse"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"

components:
  securitySchemes:
    cookieAuth:
      type: apiKey
      in: cookie
      name: session_token

  parameters:
    Limit:
      name: limit
      in: query
      required: false
      schema:
        type: integer
        format: int32
        minimum: 1
        maximum: 100
        default: 20
    Offset:
      name: offset
      in: query
      required: false
      schema:
        type: integer
        format: int32
        minimum: 0
        default: 0

  schemas:
    AuthzOverviewResponse:
      type: object
      required: [engine, totals, top_denied_actions, recent_events]
      properties:
        engine:
          $ref: "#/components/schemas/AuthzEngineStatus"
        totals:
          $ref: "#/components/schemas/AuthzTotals"
        top_denied_actions:
          type: array
          items:
            $ref: "#/components/schemas/AuthzActionCount"
        recent_events:
          type: array
          items:
            $ref: "#/components/schemas/AuthzDecisionRecord"

    AuthzEngineStatus:
      type: object
      required: [engine, status]
      properties:
        engine:
          type: string
          example: db
        status:
          type: string
          example: healthy
        engine_version:
          type: string
          nullable: true
          example: db-authorizer-v1

    AuthzTotals:
      type: object
      required: [total, allowed, denied, denied_rate]
      properties:
        total:
          type: integer
          format: int64
        allowed:
          type: integer
          format: int64
        denied:
          type: integer
          format: int64
        denied_rate:
          type: number
          format: double

    AuthzActionCount:
      type: object
      required: [action, count]
      properties:
        action:
          type: string
        count:
          type: integer
          format: int64

    AuthzDecisionListResponse:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: "#/components/schemas/AuthzDecisionRecord"

    AuthzDecisionRecord:
      type: object
      required: [id, tenant_id, module, action, result, created_at]
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        user_id:
          type: string
          format: uuid
          nullable: true
        username:
          type: string
          nullable: true
        module:
          type: string
        action:
          type: string
        result:
          type: string
          enum: [succeeded, failed]
        resource_type:
          type: string
          nullable: true
        resource_id:
          type: string
          nullable: true
        request_id:
          type: string
          nullable: true
        engine:
          type: string
          nullable: true
        reason:
          type: string
          nullable: true
        matched_rule:
          type: string
          nullable: true
        actor_type:
          type: string
          nullable: true
        actor_id:
          type: string
          nullable: true
        details:
          type: object
          additionalProperties: true
        created_at:
          type: string
          format: date-time

    RuntimeScopeListResponse:
      type: object
      required: [nodes]
      properties:
        nodes:
          type: array
          items:
            $ref: "#/components/schemas/RuntimeScopeNode"

    RuntimeScopeNode:
      type: object
      required: [runtime_node_id, node_id, name, supported_providers, max_slots, current_load, status, scopes]
      properties:
        runtime_node_id:
          type: string
          format: uuid
        node_id:
          type: string
        name:
          type: string
        supported_providers:
          type: array
          items:
            type: string
        max_slots:
          type: integer
          format: int32
        current_load:
          type: integer
          format: int32
        status:
          type: string
        last_heartbeat_at:
          type: string
          format: date-time
          nullable: true
        recent_denied_reason:
          type: string
          nullable: true
        scopes:
          type: array
          items:
            $ref: "#/components/schemas/RuntimeScope"

    RuntimeScope:
      type: object
      required: [id, tenant_id, runtime_node_id, scope_type, scope_value, status, created_at, updated_at]
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        runtime_node_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
          nullable: true
        scope_type:
          type: string
          enum: [tenant, team]
        scope_value:
          type: string
        status:
          type: string
          enum: [active, disabled]
        disabled_at:
          type: string
          format: date-time
          nullable: true
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time

    CreateRuntimeScopeRequest:
      type: object
      required: [runtime_node_id, tenant_id, scope_type, scope_value]
      properties:
        runtime_node_id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
          nullable: true
        scope_type:
          type: string
          enum: [tenant, team]
        scope_value:
          type: string
          minLength: 1

    UpdateRuntimeScopeRequest:
      type: object
      required: [status]
      properties:
        status:
          type: string
          enum: [active, disabled]

    RuntimeScopeResponse:
      type: object
      required: [scope]
      properties:
        scope:
          $ref: "#/components/schemas/RuntimeScope"

    AuthzMemberListResponse:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: "#/components/schemas/AuthzMemberRecord"

    AuthzMemberRecord:
      type: object
      required: [user_id, username, account_status, memberships, console_access]
      properties:
        user_id:
          type: string
          format: uuid
        username:
          type: string
        email:
          type: string
          nullable: true
        display_name:
          type: string
          nullable: true
        account_status:
          type: string
        memberships:
          type: array
          items:
            $ref: "#/components/schemas/AuthzMembershipRecord"
        console_access:
          type: boolean
        recent_denied_reason:
          type: string
          nullable: true

    AuthzMembershipRecord:
      type: object
      required: [tenant_id, principal_type, principal_id, role, status]
      properties:
        tenant_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
          nullable: true
        principal_type:
          type: string
        principal_id:
          type: string
          format: uuid
        role:
          type: string
        status:
          type: string

    CheckPermissionRequest:
      type: object
      required: [actor, action, resource, tenant_id]
      properties:
        actor:
          $ref: "#/components/schemas/AuthzRef"
        action:
          type: string
          enum: [console.access, tenant.access, team.access, task.claim]
        resource:
          $ref: "#/components/schemas/AuthzRef"
        tenant_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
          nullable: true

    AuthzRef:
      type: object
      required: [type, id]
      properties:
        type:
          type: string
        id:
          type: string

    CheckPermissionResponse:
      type: object
      required: [allowed, reason, matched_rule, engine]
      properties:
        allowed:
          type: boolean
        reason:
          type: string
        matched_rule:
          type: string
        engine:
          type: string
        snapshot:
          type: object
          additionalProperties: true

    Error:
      type: object
      required: [error]
      properties:
        error:
          type: string

  responses:
    BadRequest:
      description: 请求参数错误
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
    Unauthorized:
      description: 未认证
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
    Forbidden:
      description: 无权限
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
    NotFound:
      description: 资源不存在
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
```

- [ ] **Step 2: Add oapi-codegen target**

Modify `apps/control-plane/Makefile` so `generate-openapi` creates the authzcenter package:

```makefile
generate-openapi:
	@echo "Generating code from OpenAPI specs..."
	@mkdir -p internal/auth
	oapi-codegen -package auth -generate types,chi-server \
		-o internal/auth/generated.go \
		../../contracts/control-plane/auth.yaml
	@mkdir -p internal/authzcenter
	oapi-codegen -package authzcenter -generate types,chi-server \
		-o internal/authzcenter/generated.go \
		../../contracts/control-plane/authz.yaml
	@echo "OpenAPI code generation complete"
```

- [ ] **Step 3: Generate OpenAPI code**

Run:

```bash
make -C apps/control-plane generate-openapi
```

Expected: exit 0, and `apps/control-plane/internal/authzcenter/generated.go` exists.

- [ ] **Step 4: Commit contract slice**

```bash
git add contracts/control-plane/authz.yaml apps/control-plane/Makefile apps/control-plane/internal/authzcenter/generated.go
git commit -m "feat: add authz center api contract"
```

---

## Task 2: Add Authz Center SQL Queries

**Files:**
- Create: `apps/control-plane/internal/storage/queries/authz_center.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Test: `apps/control-plane/internal/storage/queries/queries_test.go`

- [ ] **Step 1: Add failing query tests**

Append focused tests to `apps/control-plane/internal/storage/queries/queries_test.go`. Use the existing test DB helper pattern in the file. Add one test for decision filtering and one for runtime scope mutation:

```go
func TestAuthzCenterDecisionQueries(t *testing.T) {
	q, cleanup := setupTestQueries(t)
	defer cleanup()

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.New()
	_, err := q.CreateWebOperationLog(context.Background(), CreateWebOperationLogParams{
		TenantID: uuid.NullUUID{UUID: tenantID, Valid: true},
		UserID:   uuid.NullUUID{UUID: userID, Valid: true},
		Username: pgtype.Text{String: "authz-auditor", Valid: true},
		Module:   "authz",
		Action:   "task.claim",
		Result:   "failed",
		Details:  []byte(`{"engine":"db","reason":"runtime scope does not cover task","actor_type":"runtime_node","actor_id":"node-1"}`),
	})
	if err != nil {
		t.Fatalf("create operation log: %v", err)
	}

	rows, err := q.ListAuthzDecisions(context.Background(), ListAuthzDecisionsParams{
		Result: pgtype.Text{String: "failed", Valid: true},
		Action: pgtype.Text{String: "task.claim", Valid: true},
		Limit:  20,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one authz decision, got %#v", rows)
	}
	if rows[0].Action != "task.claim" || rows[0].Result != "failed" {
		t.Fatalf("unexpected decision row: %#v", rows[0])
	}
}

func TestAuthzCenterRuntimeScopeQueries(t *testing.T) {
	q, cleanup := setupTestQueries(t)
	defer cleanup()

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	node, err := q.CreateRuntimeNode(context.Background(), CreateRuntimeNodeParams{
		NodeID:             "authz-center-node",
		Name:               "Authz Center Node",
		SupportedProviders: []byte(`["codex"]`),
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             "online",
		Metadata:           []byte(`{}`),
		LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		t.Fatalf("create runtime node: %v", err)
	}

	scope, err := q.CreateRuntimeNodeScope(context.Background(), CreateRuntimeNodeScopeParams{
		TenantID:      tenantID,
		RuntimeNodeID: node.ID,
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})
	if err != nil {
		t.Fatalf("create runtime scope: %v", err)
	}
	if scope.Status != "active" {
		t.Fatalf("expected active scope, got %#v", scope)
	}

	updated, err := q.UpdateRuntimeNodeScopeStatus(context.Background(), UpdateRuntimeNodeScopeStatusParams{
		ID:     scope.ID,
		Status: "disabled",
	})
	if err != nil {
		t.Fatalf("disable runtime scope: %v", err)
	}
	if updated.Status != "disabled" || !updated.DisabledAt.Valid {
		t.Fatalf("expected disabled scope with disabled_at, got %#v", updated)
	}

	nodes, err := q.ListRuntimeNodesWithScopes(context.Background())
	if err != nil {
		t.Fatalf("list runtime nodes with scopes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatalf("expected runtime node scope rows")
	}
}
```

- [ ] **Step 2: Run query tests and confirm failure**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run 'TestAuthzCenter' -count=1
```

Expected: FAIL because `ListAuthzDecisions`, `CreateRuntimeNodeScope`, `UpdateRuntimeNodeScopeStatus`, and `ListRuntimeNodesWithScopes` are not generated yet.

- [ ] **Step 3: Add authz center SQL queries**

Create `apps/control-plane/internal/storage/queries/authz_center.sql`:

```sql
-- name: ListAuthzDecisions :many
SELECT *
FROM web_operation_logs
WHERE module = 'authz'
  AND (sqlc.narg('result')::varchar IS NULL OR result = sqlc.narg('result')::varchar)
  AND (sqlc.narg('action')::varchar IS NULL OR action = sqlc.narg('action')::varchar)
  AND (sqlc.narg('actor_type')::varchar IS NULL OR details->>'actor_type' = sqlc.narg('actor_type')::varchar OR details->'actor'->>'type' = sqlc.narg('actor_type')::varchar)
  AND (sqlc.narg('actor_id')::varchar IS NULL OR details->>'actor_id' = sqlc.narg('actor_id')::varchar OR details->'actor'->>'id' = sqlc.narg('actor_id')::varchar)
  AND (sqlc.narg('resource_type')::varchar IS NULL OR resource_type = sqlc.narg('resource_type')::varchar)
  AND (sqlc.narg('resource_id')::varchar IS NULL OR resource_id = sqlc.narg('resource_id')::varchar)
  AND (sqlc.narg('request_id')::varchar IS NULL OR request_id = sqlc.narg('request_id')::varchar)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountAuthzDecisionsSince :one
SELECT
  COUNT(*)::bigint AS total,
  COUNT(*) FILTER (WHERE result = 'succeeded')::bigint AS allowed,
  COUNT(*) FILTER (WHERE result = 'failed')::bigint AS denied
FROM web_operation_logs
WHERE module = 'authz'
  AND created_at >= sqlc.arg('since')::timestamptz;

-- name: ListTopDeniedAuthzActionsSince :many
SELECT action, COUNT(*)::bigint AS count
FROM web_operation_logs
WHERE module = 'authz'
  AND result = 'failed'
  AND created_at >= sqlc.arg('since')::timestamptz
GROUP BY action
ORDER BY count DESC, action ASC
LIMIT sqlc.arg('limit');

-- name: ListRuntimeNodesWithScopes :many
SELECT
  rn.id AS runtime_node_id,
  rn.tenant_id AS runtime_tenant_id,
  rn.node_id,
  rn.name,
  rn.supported_providers,
  rn.max_slots,
  rn.current_load,
  rn.status AS runtime_status,
  rn.last_heartbeat_at,
  rns.id AS scope_id,
  rns.tenant_id AS scope_tenant_id,
  rns.runtime_node_id AS scope_runtime_node_id,
  rns.team_id AS scope_team_id,
  rns.scope_type,
  rns.scope_value,
  rns.status AS scope_status,
  rns.disabled_at AS scope_disabled_at,
  rns.created_at AS scope_created_at,
  rns.updated_at AS scope_updated_at
FROM runtime_nodes rn
LEFT JOIN runtime_node_scopes rns ON rns.runtime_node_id = rn.id
WHERE rn.archived_at IS NULL
ORDER BY rn.created_at DESC, rns.created_at DESC;

-- name: CreateRuntimeNodeScope :one
INSERT INTO runtime_node_scopes (
  tenant_id,
  runtime_node_id,
  team_id,
  scope_type,
  scope_value,
  status
) VALUES (
  sqlc.arg('tenant_id')::uuid,
  sqlc.arg('runtime_node_id')::uuid,
  sqlc.narg('team_id')::uuid,
  sqlc.arg('scope_type')::varchar,
  sqlc.arg('scope_value')::varchar,
  'active'
)
ON CONFLICT (runtime_node_id, scope_type, scope_value) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  team_id = EXCLUDED.team_id,
  status = 'active',
  disabled_at = NULL,
  updated_at = NOW()
RETURNING *;

-- name: UpdateRuntimeNodeScopeStatus :one
UPDATE runtime_node_scopes
SET status = sqlc.arg('status')::varchar,
    disabled_at = CASE
      WHEN sqlc.arg('status')::varchar = 'disabled' THEN COALESCE(disabled_at, NOW())
      WHEN sqlc.arg('status')::varchar = 'active' THEN NULL
      ELSE disabled_at
    END,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ListAuthzMembers :many
SELECT
  u.id AS user_id,
  u.username,
  u.display_name,
  u.email,
  u.status AS account_status,
  tm.tenant_id,
  tm.team_id,
  tm.principal_type,
  tm.principal_id,
  tm.role,
  tm.status AS membership_status
FROM auth_users u
LEFT JOIN tenant_members tm
  ON tm.principal_type = 'user'
 AND tm.principal_id = u.id
 AND tm.disabled_at IS NULL
WHERE u.deleted_at IS NULL
ORDER BY u.created_at DESC, tm.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

- [ ] **Step 4: Regenerate sqlc**

Run:

```bash
make -C apps/control-plane generate-sqlc
```

Expected: exit 0 and generated methods appear in `apps/control-plane/internal/storage/queries/authz_center.sql.go`.

- [ ] **Step 5: Run query tests**

Run:

```bash
go test ./apps/control-plane/internal/storage/queries -run 'TestAuthzCenter' -count=1
```

Expected: PASS when the query test DB is configured; otherwise existing query tests should skip with the repository's documented skip message.

- [ ] **Step 6: Commit SQL slice**

```bash
git add apps/control-plane/internal/storage/queries
git commit -m "feat: add authz center storage queries"
```

---

## Task 3: Implement Authz Center Service and Handler

**Files:**
- Create: `apps/control-plane/internal/authzcenter/types.go`
- Create: `apps/control-plane/internal/authzcenter/repository.go`
- Create: `apps/control-plane/internal/authzcenter/pg_repository.go`
- Create: `apps/control-plane/internal/authzcenter/service.go`
- Create: `apps/control-plane/internal/authzcenter/handler.go`
- Create: `apps/control-plane/internal/authzcenter/service_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/api/routes_test.go`

- [ ] **Step 1: Add failing service tests**

Create `apps/control-plane/internal/authzcenter/service_test.go`:

```go
package authzcenter

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
)

type memoryRepo struct {
	decisions []DecisionRecord
	scopes    []RuntimeScope
	members   []MemberRecord
	logs      []OperationLogInput
}

func (r *memoryRepo) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	return r.decisions, nil
}

func (r *memoryRepo) CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error) {
	var totals DecisionTotals
	for _, decision := range r.decisions {
		totals.Total++
		if decision.Result == "failed" {
			totals.Denied++
		} else {
			totals.Allowed++
		}
	}
	return totals, nil
}

func (r *memoryRepo) ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error) {
	return []ActionCount{{Action: "task.claim", Count: 2}}, nil
}

func (r *memoryRepo) ListRuntimeScopeNodes(ctx context.Context) ([]RuntimeScopeNode, error) {
	return []RuntimeScopeNode{{
		RuntimeNodeID: uuid.MustParse("00000000-0000-4000-8000-000000000010"),
		NodeID:        "node-1",
		Name:          "Node 1",
		Status:        "online",
		Scopes:        r.scopes,
	}}, nil
}

func (r *memoryRepo) CreateRuntimeScope(ctx context.Context, input RuntimeScopeInput) (RuntimeScope, error) {
	scope := RuntimeScope{
		ID:            uuid.New(),
		TenantID:      input.TenantID,
		RuntimeNodeID: input.RuntimeNodeID,
		TeamID:        input.TeamID,
		ScopeType:     input.ScopeType,
		ScopeValue:    input.ScopeValue,
		Status:        "active",
	}
	r.scopes = append(r.scopes, scope)
	return scope, nil
}

func (r *memoryRepo) UpdateRuntimeScopeStatus(ctx context.Context, scopeID uuid.UUID, status string) (RuntimeScope, error) {
	for idx, scope := range r.scopes {
		if scope.ID == scopeID {
			r.scopes[idx].Status = status
			return r.scopes[idx], nil
		}
	}
	return RuntimeScope{}, ErrNotFound
}

func (r *memoryRepo) ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error) {
	return r.members, nil
}

func (r *memoryRepo) RecordOperationLog(ctx context.Context, input OperationLogInput) error {
	r.logs = append(r.logs, input)
	return nil
}

type fakeAuthorizer struct {
	checks  []authz.CheckRequest
	allowed bool
}

func (a *fakeAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	return authz.Decision{Allowed: a.allowed, Reason: authz.ReasonAllowed, MatchedRule: "test.allow", Snapshot: map[string]any{"engine": "db"}}, nil
}

func TestServiceOverviewUsesDecisionData(t *testing.T) {
	repo := &memoryRepo{decisions: []DecisionRecord{
		{ID: uuid.New(), Action: "task.claim", Result: "failed", CreatedAt: time.Now().UTC()},
		{ID: uuid.New(), Action: "console.access", Result: "succeeded", CreatedAt: time.Now().UTC()},
	}}
	service := NewService(repo, &fakeAuthorizer{allowed: true})

	overview, err := service.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Totals.Total != 2 || overview.Totals.Denied != 1 || overview.Totals.Allowed != 1 {
		t.Fatalf("unexpected totals: %#v", overview.Totals)
	}
	if overview.Engine.Engine != "db" {
		t.Fatalf("expected db engine, got %#v", overview.Engine)
	}
}

func TestServiceCreateRuntimeScopeRequiresAuthorizationAndLogs(t *testing.T) {
	repo := &memoryRepo{}
	authorizer := &fakeAuthorizer{allowed: true}
	service := NewService(repo, authorizer)
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	actor := Actor{UserID: uuid.New(), Username: "admin", TenantID: tenantID}

	scope, err := service.CreateRuntimeScope(context.Background(), actor, RuntimeScopeInput{
		TenantID:      tenantID,
		RuntimeNodeID: uuid.New(),
		ScopeType:     "tenant",
		ScopeValue:    tenantID.String(),
	})
	if err != nil {
		t.Fatalf("create runtime scope: %v", err)
	}
	if scope.Status != "active" {
		t.Fatalf("expected active scope, got %#v", scope)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != ActionRuntimeScopeManage {
		t.Fatalf("expected runtime scope manage check, got %#v", authorizer.checks)
	}
	if len(repo.logs) != 1 || repo.logs[0].Action != OperationActionRuntimeScopeCreate {
		t.Fatalf("expected operation log, got %#v", repo.logs)
	}
}

func TestServiceCheckPermissionUsesAuthorizerDryRun(t *testing.T) {
	repo := &memoryRepo{}
	authorizer := &fakeAuthorizer{allowed: false}
	service := NewService(repo, authorizer)
	tenantID := uuid.MustParse(auth.DefaultTenantID)

	decision, err := service.CheckPermission(context.Background(), CheckPermissionInput{
		Actor:    authz.ActorRef{Type: authz.ActorUser, ID: uuid.NewString()},
		Action:   authz.ActionConsoleAccess,
		Resource: authz.ResourceRef{Type: authz.ResourceConsole, ID: "web"},
		TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision, got %#v", decision)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].AuditReason != "authz center dry-run" {
		t.Fatalf("expected dry-run check, got %#v", authorizer.checks)
	}
}
```

- [ ] **Step 2: Run service tests and confirm failure**

Run:

```bash
go test ./apps/control-plane/internal/authzcenter -run TestService -count=1
```

Expected: FAIL because `internal/authzcenter` types and service do not exist.

- [ ] **Step 3: Add domain types**

Create `apps/control-plane/internal/authzcenter/types.go`:

```go
package authzcenter

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

const (
	ActionRuntimeScopeManage = "runtime_scope.manage"

	OperationModuleAuthz              = "authz"
	OperationActionRuntimeScopeCreate = "runtime_scope.create"
	OperationActionRuntimeScopeUpdate = "runtime_scope.update"
	OperationResultSucceeded          = "succeeded"
	OperationResultFailed             = "failed"
)

var ErrForbidden = errors.New("forbidden")
var ErrNotFound = errors.New("not found")
var ErrInvalidInput = errors.New("invalid input")

type Actor struct {
	UserID   uuid.UUID
	Username string
	TenantID uuid.UUID
}

type EngineStatus struct {
	Engine        string
	Status        string
	EngineVersion string
}

type DecisionTotals struct {
	Total   int64
	Allowed int64
	Denied  int64
}

func (t DecisionTotals) DeniedRate() float64 {
	if t.Total == 0 {
		return 0
	}
	return float64(t.Denied) / float64(t.Total)
}

type ActionCount struct {
	Action string
	Count  int64
}

type Overview struct {
	Engine           EngineStatus
	Totals           DecisionTotals
	TopDeniedActions []ActionCount
	RecentEvents     []DecisionRecord
}

type DecisionFilter struct {
	Result       string
	Action       string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	RequestID    string
	Limit        int32
	Offset       int32
}

type DecisionRecord struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UserID       *uuid.UUID
	Username     string
	Module       string
	Action       string
	Result       string
	ResourceType string
	ResourceID   string
	RequestID    string
	Engine       string
	Reason       string
	MatchedRule  string
	ActorType    string
	ActorID      string
	Details      map[string]any
	CreatedAt    time.Time
}

type RuntimeScopeNode struct {
	RuntimeNodeID     uuid.UUID
	NodeID            string
	Name              string
	SupportedProviders []string
	MaxSlots          int32
	CurrentLoad       int32
	Status            string
	LastHeartbeatAt   *time.Time
	RecentDeniedReason string
	Scopes            []RuntimeScope
}

type RuntimeScope struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	TeamID        *uuid.UUID
	ScopeType     string
	ScopeValue    string
	Status        string
	DisabledAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RuntimeScopeInput struct {
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	TeamID        *uuid.UUID
	ScopeType     string
	ScopeValue    string
}

type MemberFilter struct {
	Limit  int32
	Offset int32
}

type MemberRecord struct {
	UserID             uuid.UUID
	Username           string
	DisplayName        string
	Email              string
	AccountStatus      string
	Memberships        []MembershipRecord
	ConsoleAccess      bool
	RecentDeniedReason string
}

type MembershipRecord struct {
	TenantID      uuid.UUID
	TeamID        *uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	Role          string
	Status        string
}

type CheckPermissionInput struct {
	Actor    authz.ActorRef
	Action   string
	Resource authz.ResourceRef
	TenantID uuid.UUID
	TeamID   *uuid.UUID
}

type OperationLogInput struct {
	UserID       uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	Details      map[string]any
}
```

- [ ] **Step 4: Add repository interface**

Create `apps/control-plane/internal/authzcenter/repository.go`:

```go
package authzcenter

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error)
	CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error)
	ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error)
	ListRuntimeScopeNodes(ctx context.Context) ([]RuntimeScopeNode, error)
	CreateRuntimeScope(ctx context.Context, input RuntimeScopeInput) (RuntimeScope, error)
	UpdateRuntimeScopeStatus(ctx context.Context, scopeID uuid.UUID, status string) (RuntimeScope, error)
	ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error)
	RecordOperationLog(ctx context.Context, input OperationLogInput) error
}
```

- [ ] **Step 5: Add service implementation**

Create `apps/control-plane/internal/authzcenter/service.go`:

```go
package authzcenter

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type Service struct {
	repo       Repository
	authorizer authz.Authorizer
	now        func() time.Time
}

func NewService(repo Repository, authorizer authz.Authorizer) *Service {
	return &Service{repo: repo, authorizer: authorizer, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) GetOverview(ctx context.Context) (Overview, error) {
	since := s.now().Add(-24 * time.Hour)
	totals, err := s.repo.CountDecisionsSince(ctx, since)
	if err != nil {
		return Overview{}, err
	}
	topDenied, err := s.repo.ListTopDeniedActionsSince(ctx, since, 5)
	if err != nil {
		return Overview{}, err
	}
	recent, err := s.repo.ListDecisions(ctx, DecisionFilter{Limit: 10, Offset: 0})
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		Engine: EngineStatus{Engine: "db", Status: "healthy", EngineVersion: "db-authorizer-v1"},
		Totals: totals,
		TopDeniedActions: topDenied,
		RecentEvents: recent,
	}, nil
}

func (s *Service) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	filter.Limit = normalizeLimit(filter.Limit)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListDecisions(ctx, filter)
}

func (s *Service) ListRuntimeScopes(ctx context.Context) ([]RuntimeScopeNode, error) {
	return s.repo.ListRuntimeScopeNodes(ctx)
}

func (s *Service) CreateRuntimeScope(ctx context.Context, actor Actor, input RuntimeScopeInput) (RuntimeScope, error) {
	if err := validateRuntimeScopeInput(input); err != nil {
		return RuntimeScope{}, err
	}
	if err := s.ensureRuntimeScopeManage(ctx, actor, input.TenantID); err != nil {
		_ = s.recordRuntimeScopeOperation(ctx, actor, "", OperationActionRuntimeScopeCreate, OperationResultFailed, map[string]any{"reason": err.Error()})
		return RuntimeScope{}, err
	}
	scope, err := s.repo.CreateRuntimeScope(ctx, input)
	if err != nil {
		_ = s.recordRuntimeScopeOperation(ctx, actor, "", OperationActionRuntimeScopeCreate, OperationResultFailed, map[string]any{"reason": err.Error()})
		return RuntimeScope{}, err
	}
	_ = s.recordRuntimeScopeOperation(ctx, actor, scope.ID.String(), OperationActionRuntimeScopeCreate, OperationResultSucceeded, map[string]any{
		"runtime_node_id": scope.RuntimeNodeID.String(),
		"tenant_id": scope.TenantID.String(),
		"team_id": uuidString(scope.TeamID),
		"scope_type": scope.ScopeType,
		"scope_value": scope.ScopeValue,
	})
	return scope, nil
}

func (s *Service) UpdateRuntimeScopeStatus(ctx context.Context, actor Actor, scopeID uuid.UUID, status string) (RuntimeScope, error) {
	if status != "active" && status != "disabled" {
		return RuntimeScope{}, ErrInvalidInput
	}
	if err := s.ensureRuntimeScopeManage(ctx, actor, actor.TenantID); err != nil {
		_ = s.recordRuntimeScopeOperation(ctx, actor, scopeID.String(), OperationActionRuntimeScopeUpdate, OperationResultFailed, map[string]any{"reason": err.Error(), "status": status})
		return RuntimeScope{}, err
	}
	scope, err := s.repo.UpdateRuntimeScopeStatus(ctx, scopeID, status)
	if err != nil {
		_ = s.recordRuntimeScopeOperation(ctx, actor, scopeID.String(), OperationActionRuntimeScopeUpdate, OperationResultFailed, map[string]any{"reason": err.Error(), "status": status})
		return RuntimeScope{}, err
	}
	_ = s.recordRuntimeScopeOperation(ctx, actor, scope.ID.String(), OperationActionRuntimeScopeUpdate, OperationResultSucceeded, map[string]any{"status": status})
	return scope, nil
}

func (s *Service) ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error) {
	filter.Limit = normalizeLimit(filter.Limit)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.ListMembers(ctx, filter)
}

func (s *Service) CheckPermission(ctx context.Context, input CheckPermissionInput) (authz.Decision, error) {
	return s.authorizer.Check(ctx, authz.CheckRequest{
		Actor: input.Actor,
		Action: input.Action,
		Resource: input.Resource,
		TenantID: input.TenantID,
		TeamID: input.TeamID,
		AuditReason: "authz center dry-run",
	})
}

func (s *Service) ensureRuntimeScopeManage(ctx context.Context, actor Actor, tenantID uuid.UUID) error {
	decision, err := s.authorizer.Check(ctx, authz.CheckRequest{
		Actor: authz.ActorRef{Type: authz.ActorUser, ID: actor.UserID.String()},
		Action: ActionRuntimeScopeManage,
		Resource: authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
		AuditReason: "runtime scope management",
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) recordRuntimeScopeOperation(ctx context.Context, actor Actor, resourceID, action, result string, details map[string]any) error {
	return s.repo.RecordOperationLog(ctx, OperationLogInput{
		UserID: actor.UserID,
		Username: actor.Username,
		Module: OperationModuleAuthz,
		ResourceType: "runtime_node_scope",
		ResourceID: resourceID,
		Action: action,
		Result: result,
		Details: details,
	})
}

func validateRuntimeScopeInput(input RuntimeScopeInput) error {
	if input.TenantID == uuid.Nil || input.RuntimeNodeID == uuid.Nil || strings.TrimSpace(input.ScopeValue) == "" {
		return ErrInvalidInput
	}
	if input.ScopeType != "tenant" && input.ScopeType != "team" {
		return ErrInvalidInput
	}
	if input.ScopeType == "team" && input.TeamID == nil {
		return ErrInvalidInput
	}
	return nil
}

func normalizeLimit(limit int32) int32 {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func uuidString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}
```

- [ ] **Step 6: Extend authz authorizer for runtime scope management**

Modify `apps/control-plane/internal/authz/types.go` to add:

```go
const (
	ActionRuntimeScopeManage = "runtime_scope.manage"
)
```

Modify `apps/control-plane/internal/authz/authorizer.go` switch to allow tenant owner/admin only:

```go
case ActionRuntimeScopeManage:
	if !resourceMatchesUUID(req.Resource, ResourceTenant, req.TenantID) {
		decision = deny(ReasonInvalidResource)
		break
	}
	decision, err = a.checkTenantAdminAccess(ctx, req)
```

Add helper in the same file:

```go
func (a *DBAuthorizer) checkTenantAdminAccess(ctx context.Context, req CheckRequest) (Decision, error) {
	principalID, ok := parseUUIDActor(req.Actor, ActorUser)
	if !ok {
		return deny(ReasonInvalidActor), nil
	}
	membership, err := a.repository.GetActiveTenantMembership(ctx, TenantMembershipParams{
		TenantID: req.TenantID,
		PrincipalType: ActorUser,
		PrincipalID: principalID,
	})
	if err != nil {
		if errors.Is(err, ErrNoMembership) {
			return deny(ReasonNoMembership), nil
		}
		return Decision{}, err
	}
	if membership.Role == RoleOwner || membership.Role == RoleAdmin {
		return allow("tenant."+membership.Role, membership.Role), nil
	}
	return deny(ReasonNoMembership), nil
}
```

Add a focused test in `apps/control-plane/internal/authz/authorizer_test.go` that member role is denied and admin role is allowed for `runtime_scope.manage`.

- [ ] **Step 7: Add pg repository implementation**

Create `apps/control-plane/internal/authzcenter/pg_repository.go`. It should:

- map `queries.WebOperationLog` to `DecisionRecord`;
- parse `details` JSON into `map[string]any`;
- read reason/matched_rule/engine/actor from either flat keys or nested `actor`;
- group `ListRuntimeNodesWithScopes` rows by runtime node;
- map `tenant_members` rows into member records grouped by user;
- use `queries.CreateWebOperationLog` for `RecordOperationLog`.

Use this skeleton and fill all mapping helpers in the same file:

```go
package authzcenter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	rows, err := r.q.ListAuthzDecisions(ctx, queries.ListAuthzDecisionsParams{
		Result: nullableText(filter.Result),
		Action: nullableText(filter.Action),
		ActorType: nullableText(filter.ActorType),
		ActorID: nullableText(filter.ActorID),
		ResourceType: nullableText(filter.ResourceType),
		ResourceID: nullableText(filter.ResourceID),
		RequestID: nullableText(filter.RequestID),
		Limit: filter.Limit,
		Offset: filter.Offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]DecisionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, decisionFromLog(row))
	}
	return items, nil
}

func (r *PgRepository) CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error) {
	row, err := r.q.CountAuthzDecisionsSince(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return DecisionTotals{}, err
	}
	return DecisionTotals{Total: row.Total, Allowed: row.Allowed, Denied: row.Denied}, nil
}

func (r *PgRepository) ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error) {
	rows, err := r.q.ListTopDeniedAuthzActionsSince(ctx, queries.ListTopDeniedAuthzActionsSinceParams{
		Since: pgtype.Timestamptz{Time: since, Valid: true},
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ActionCount, 0, len(rows))
	for _, row := range rows {
		items = append(items, ActionCount{Action: row.Action, Count: row.Count})
	}
	return items, nil
}
```

The implementation must include the remaining repository methods and compile. Keep helpers small and local to this file.

- [ ] **Step 8: Add HTTP handler**

Create `apps/control-plane/internal/authzcenter/handler.go` implementing the generated interface:

```go
package authzcenter

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/auth"
)

type HTTPHandler struct {
	service *Service
	authService *auth.Service
}

func NewHandler(service *Service, authService *auth.Service) *HTTPHandler {
	return &HTTPHandler{service: service, authService: authService}
}

func (h *HTTPHandler) GetAuthzOverview(w http.ResponseWriter, r *http.Request) {
	if _, err := h.currentActor(r); err != nil {
		h.writeAuthError(w, err)
		return
	}
	overview, err := h.service.GetOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, toGeneratedOverview(overview))
}
```

Implement all generated methods:

- `ListAuthzDecisions`
- `ListRuntimeScopes`
- `CreateRuntimeScope`
- `UpdateRuntimeScope`
- `ListAuthzMembers`
- `CheckPermission`

Use `auth.SessionCookieName` and `auth.Service.GetCurrentUserContext` to build `Actor`. Reuse local `writeJSON` / `writeError` helpers in this package. Translate:

- `auth.ErrUnauthorized`, `auth.ErrSessionNotFound`, `auth.ErrSessionExpired` -> 401
- `auth.ErrUserDisabled` -> 403
- `ErrForbidden` -> 403
- `ErrInvalidInput` -> 400
- `ErrNotFound` -> 404

- [ ] **Step 9: Register routes in server and app**

Modify `apps/control-plane/internal/api/server.go`:

- Add `authzCenterHandler *authzcenter.HTTPHandler` to `Server`.
- Add parameter to `NewServerWithAuthz`:

```go
authzCenterHandler *authzcenter.HTTPHandler,
```

- If handler is non-nil, call:

```go
authzcenter.HandlerFromMux(authzCenterHandler, server.router)
```

Modify `apps/control-plane/internal/app/app.go`:

```go
authzCenterRepository := authzcenter.NewPgRepository(q)
authzCenterService := authzcenter.NewService(authzCenterRepository, authorizer)
authzCenterHandler := authzcenter.NewHandler(authzCenterService, authService)
server := api.NewServerWithAuthz(taskHandler, runtimeHandler, authService, authService, authorizer, authzCenterHandler)
```

- [ ] **Step 10: Add route tests**

Append tests to `apps/control-plane/internal/api/routes_test.go`:

```go
func TestAuthzCenterRoutesRejectUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	center := authzcenter.NewHandler(authzcenter.NewService(&routeAuthzCenterRepo{}, &routeAuthorizer{allowed: true}), authService)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
		center,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", resp.Code)
	}
}
```

Add `routeAuthzCenterRepo` in the same test file implementing `authzcenter.Repository`, with empty lists and operation log capture. Add a positive route test for:

- login as admin;
- `GET /api/authz/overview` returns 200;
- `POST /api/authz/runtime-scopes` records one `runtime_scope.manage` check and one operation log.

- [ ] **Step 11: Run backend tests**

Run:

```bash
go test ./apps/control-plane/internal/authz ./apps/control-plane/internal/authzcenter ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

- [ ] **Step 12: Commit backend service slice**

```bash
git add apps/control-plane/internal/authz apps/control-plane/internal/authzcenter apps/control-plane/internal/api/server.go apps/control-plane/internal/api/routes_test.go apps/control-plane/internal/app/app.go
git commit -m "feat: implement authz center backend"
```

---

## Task 4: Add Web Authz API Client

**Files:**
- Create: `apps/web/src/lib/api/authz.ts`
- Create: `apps/web/src/lib/api/authz.test.ts`
- Modify: `apps/web/src/lib/api/index.ts`

- [ ] **Step 1: Add failing API client tests**

Create `apps/web/src/lib/api/authz.test.ts`:

```ts
import { describe, expect, it, vi } from "vitest";
import {
  checkPermission,
  createRuntimeScope,
  listAuthzDecisions,
  listAuthzMembers,
  listRuntimeScopes,
  updateRuntimeScope,
} from "./authz";

describe("authz center api client", () => {
  it("lists authz decisions with filters and cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(
      listAuthzDecisions({
        baseUrl: "http://control-plane.local/",
        fetcher,
        result: "failed",
        action: "task.claim",
        limit: 10,
        offset: 5,
      }),
    ).resolves.toEqual({ items: [] });

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/authz/decisions?result=failed&action=task.claim&limit=10&offset=5",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates and updates runtime scopes", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          scope: {
            id: "00000000-0000-4000-8000-000000000001",
            tenant_id: "00000000-0000-0000-0000-000000000001",
            runtime_node_id: "00000000-0000-4000-8000-000000000010",
            team_id: null,
            scope_type: "tenant",
            scope_value: "00000000-0000-0000-0000-000000000001",
            status: "active",
            disabled_at: null,
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    );

    await createRuntimeScope(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        tenant_id: "00000000-0000-0000-0000-000000000001",
        runtime_node_id: "00000000-0000-4000-8000-000000000010",
        scope_type: "tenant",
        scope_value: "00000000-0000-0000-0000-000000000001",
      },
    );

    await updateRuntimeScope(
      { baseUrl: "http://control-plane.local", fetcher },
      "00000000-0000-4000-8000-000000000001",
      "disabled",
    );

    expect(fetcher).toHaveBeenNthCalledWith(1, "http://control-plane.local/api/authz/runtime-scopes", expect.objectContaining({ method: "POST", credentials: "include" }));
    expect(fetcher).toHaveBeenNthCalledWith(2, "http://control-plane.local/api/authz/runtime-scopes/00000000-0000-4000-8000-000000000001", expect.objectContaining({ method: "PATCH", credentials: "include" }));
  });

  it("loads runtime scopes and members and checks permission", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/authz/runtime-scopes")) {
        return new Response(JSON.stringify({ nodes: [] }), { status: 200, headers: { "content-type": "application/json" } });
      }
      if (url.includes("/api/authz/members")) {
        return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "content-type": "application/json" } });
      }
      return new Response(JSON.stringify({ allowed: true, reason: "allowed", matched_rule: "tenant.owner", engine: "db", snapshot: {} }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    });

    await expect(listRuntimeScopes({ baseUrl: "http://control-plane.local", fetcher })).resolves.toEqual({ nodes: [] });
    await expect(listAuthzMembers({ baseUrl: "http://control-plane.local", fetcher, limit: 20, offset: 0 })).resolves.toEqual({ items: [] });
    await expect(
      checkPermission(
        { baseUrl: "http://control-plane.local", fetcher },
        {
          actor: { type: "user", id: "00000000-0000-4000-8000-000000000001" },
          action: "console.access",
          resource: { type: "console", id: "web" },
          tenant_id: "00000000-0000-0000-0000-000000000001",
        },
      ),
    ).resolves.toMatchObject({ allowed: true });
  });
});
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/authz.test.ts
```

Expected: FAIL because `apps/web/src/lib/api/authz.ts` does not exist.

- [ ] **Step 3: Implement API client**

Create `apps/web/src/lib/api/authz.ts` with exported types and functions:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type AuthzDecisionResult = "succeeded" | "failed";
export type RuntimeScopeStatus = "active" | "disabled";
export type RuntimeScopeType = "tenant" | "team";

export type AuthzDecisionRecord = {
  id: string;
  tenant_id: string;
  user_id?: string | null;
  username?: string | null;
  module: string;
  action: string;
  result: AuthzDecisionResult;
  resource_type?: string | null;
  resource_id?: string | null;
  request_id?: string | null;
  engine?: string | null;
  reason?: string | null;
  matched_rule?: string | null;
  actor_type?: string | null;
  actor_id?: string | null;
  details?: Record<string, unknown>;
  created_at: string;
};

export type AuthzOverviewResponse = {
  engine: { engine: string; status: string; engine_version?: string | null };
  totals: { total: number; allowed: number; denied: number; denied_rate: number };
  top_denied_actions: { action: string; count: number }[];
  recent_events: AuthzDecisionRecord[];
};

export type AuthzDecisionListResponse = { items: AuthzDecisionRecord[] };

export type RuntimeScope = {
  id: string;
  tenant_id: string;
  runtime_node_id: string;
  team_id?: string | null;
  scope_type: RuntimeScopeType;
  scope_value: string;
  status: RuntimeScopeStatus;
  disabled_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type RuntimeScopeNode = {
  runtime_node_id: string;
  node_id: string;
  name: string;
  supported_providers: string[];
  max_slots: number;
  current_load: number;
  status: string;
  last_heartbeat_at?: string | null;
  recent_denied_reason?: string | null;
  scopes: RuntimeScope[];
};

export type RuntimeScopeListResponse = { nodes: RuntimeScopeNode[] };
export type RuntimeScopeResponse = { scope: RuntimeScope };

export type CreateRuntimeScopeRequest = {
  runtime_node_id: string;
  tenant_id: string;
  team_id?: string | null;
  scope_type: RuntimeScopeType;
  scope_value: string;
};

export type AuthzMemberRecord = {
  user_id: string;
  username: string;
  display_name?: string | null;
  email?: string | null;
  account_status: string;
  console_access: boolean;
  recent_denied_reason?: string | null;
  memberships: Array<{
    tenant_id: string;
    team_id?: string | null;
    principal_type: string;
    principal_id: string;
    role: string;
    status: string;
  }>;
};

export type AuthzMemberListResponse = { items: AuthzMemberRecord[] };

export type CheckPermissionRequest = {
  actor: { type: string; id: string };
  action: "console.access" | "tenant.access" | "team.access" | "task.claim";
  resource: { type: string; id: string };
  tenant_id: string;
  team_id?: string | null;
};

export type CheckPermissionResponse = {
  allowed: boolean;
  reason: string;
  matched_rule: string;
  engine: string;
  snapshot?: Record<string, unknown>;
};

export type ListAuthzDecisionsOptions = ApiClientOptions & {
  result?: AuthzDecisionResult;
  action?: string;
  actor_type?: string;
  actor_id?: string;
  resource_type?: string;
  resource_id?: string;
  request_id?: string;
  limit?: number;
  offset?: number;
};

export type ListAuthzMembersOptions = ApiClientOptions & {
  limit?: number;
  offset?: number;
};

function queryPath(path: string, params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `${path}?${query}` : path;
}

async function getJson<T>(options: ApiClientOptions, path: string, label: string): Promise<T> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, path), {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  });
  return parseJson<T>(response, label);
}

export function getAuthzOverview(options: ApiClientOptions): Promise<AuthzOverviewResponse> {
  return getJson(options, "/api/authz/overview", "authz overview");
}

export function listAuthzDecisions(options: ListAuthzDecisionsOptions): Promise<AuthzDecisionListResponse> {
  return getJson(
    options,
    queryPath("/api/authz/decisions", {
      result: options.result,
      action: options.action,
      actor_type: options.actor_type,
      actor_id: options.actor_id,
      resource_type: options.resource_type,
      resource_id: options.resource_id,
      request_id: options.request_id,
      limit: options.limit,
      offset: options.offset,
    }),
    "authz decisions",
  );
}

export function listRuntimeScopes(options: ApiClientOptions): Promise<RuntimeScopeListResponse> {
  return getJson(options, "/api/authz/runtime-scopes", "runtime scopes");
}

export function listAuthzMembers(options: ListAuthzMembersOptions): Promise<AuthzMemberListResponse> {
  return getJson(options, queryPath("/api/authz/members", { limit: options.limit, offset: options.offset }), "authz members");
}

export async function createRuntimeScope(options: ApiClientOptions, input: CreateRuntimeScopeRequest): Promise<RuntimeScopeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/authz/runtime-scopes"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<RuntimeScopeResponse>(response, "create runtime scope");
}

export async function updateRuntimeScope(options: ApiClientOptions, scopeID: string, status: RuntimeScopeStatus): Promise<RuntimeScopeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/authz/runtime-scopes/${scopeID}`), {
    body: JSON.stringify({ status }),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "PATCH",
  });
  return parseJson<RuntimeScopeResponse>(response, "update runtime scope");
}

export async function checkPermission(options: ApiClientOptions, input: CheckPermissionRequest): Promise<CheckPermissionResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/authz/check"), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });
  return parseJson<CheckPermissionResponse>(response, "authz check");
}
```

Modify `apps/web/src/lib/api/index.ts`:

```ts
export * from "./auth";
export * from "./authz";
export * from "./client";
export * from "./health";
export * from "./runtime";
export * from "./tasks";
```

- [ ] **Step 4: Run client tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/authz.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit client slice**

```bash
git add apps/web/src/lib/api/authz.ts apps/web/src/lib/api/authz.test.ts apps/web/src/lib/api/index.ts
git commit -m "feat: add authz center web api client"
```

---

## Task 5: Build Permissions Center Web Page

**Files:**
- Create: `apps/web/src/routes/_authenticated/permissions/index.tsx`
- Create: `apps/web/src/features/permissions/index.tsx`
- Create: `apps/web/src/features/permissions/components/authorization-overview.tsx`
- Create: `apps/web/src/features/permissions/components/authorization-audit-table.tsx`
- Create: `apps/web/src/features/permissions/components/runtime-scopes.tsx`
- Create: `apps/web/src/features/permissions/components/member-roles.tsx`
- Create: `apps/web/src/features/permissions/components/permission-diagnostics.tsx`
- Create: `apps/web/src/features/permissions/index.test.tsx`
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Regenerate: `apps/web/src/routeTree.gen.ts`

- [ ] **Step 1: Add failing page tests**

Create `apps/web/src/features/permissions/index.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { PermissionsCenter } from "./index";

function renderPermissions(fetcher: typeof fetch) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <PermissionsCenter apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("PermissionsCenter", () => {
  it("renders the five permission center tabs", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/authz/overview")) {
        return new Response(JSON.stringify({ engine: { engine: "db", status: "healthy" }, totals: { total: 2, allowed: 1, denied: 1, denied_rate: 0.5 }, top_denied_actions: [], recent_events: [] }), { status: 200, headers: { "content-type": "application/json" } });
      }
      if (url.includes("/api/authz/decisions")) return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "content-type": "application/json" } });
      if (url.endsWith("/api/authz/runtime-scopes")) return new Response(JSON.stringify({ nodes: [] }), { status: 200, headers: { "content-type": "application/json" } });
      if (url.includes("/api/authz/members")) return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "content-type": "application/json" } });
      return new Response(JSON.stringify({ error: "not found" }), { status: 404, headers: { "content-type": "application/json" } });
    });

    const screen = await renderPermissions(fetcher as typeof fetch);

    await expect.element(screen.getByRole("heading", { name: "权限中心" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "授权概览" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "授权审计" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "Runtime 范围" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "成员角色" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "权限诊断" })).toBeVisible();
    await expect.element(screen.getByText("db")).toBeVisible();
  });

  it("submits permission diagnostics", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/authz/overview")) return new Response(JSON.stringify({ engine: { engine: "db", status: "healthy" }, totals: { total: 0, allowed: 0, denied: 0, denied_rate: 0 }, top_denied_actions: [], recent_events: [] }), { status: 200, headers: { "content-type": "application/json" } });
      if (url.includes("/api/authz/decisions")) return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "content-type": "application/json" } });
      if (url.endsWith("/api/authz/runtime-scopes")) return new Response(JSON.stringify({ nodes: [] }), { status: 200, headers: { "content-type": "application/json" } });
      if (url.includes("/api/authz/members")) return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "content-type": "application/json" } });
      return new Response(JSON.stringify({ allowed: true, reason: "allowed", matched_rule: "tenant.owner", engine: "db", snapshot: {} }), { status: 200, headers: { "content-type": "application/json" } });
    });

    const screen = await renderPermissions(fetcher as typeof fetch);
    await screen.getByRole("tab", { name: "权限诊断" }).click();
    await screen.getByLabelText("Actor ID").fill("00000000-0000-4000-8000-000000000001");
    await screen.getByLabelText("租户 ID").fill("00000000-0000-0000-0000-000000000001");
    await screen.getByRole("button", { name: "开始诊断" }).click();

    await expect.element(screen.getByText("允许")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run page tests and confirm failure**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/permissions/index.test.tsx
```

Expected: FAIL because the permissions feature does not exist.

- [ ] **Step 3: Add route and sidebar entry**

Create `apps/web/src/routes/_authenticated/permissions/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { PermissionsCenter } from "@/features/permissions";

export const Route = createFileRoute("/_authenticated/permissions/")({
  component: PermissionsCenter,
});
```

Modify `apps/web/src/components/layout/data/sidebar-data.ts`:

- Import `KeyRound` from `lucide-react`.
- Add item after “Runtime 节点”:

```ts
{
  title: "权限中心",
  url: "/permissions",
  icon: KeyRound,
},
```

- [ ] **Step 4: Add page container**

Create `apps/web/src/features/permissions/index.tsx`:

```tsx
import { KeyRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { AuthorizationAuditTable } from "./components/authorization-audit-table";
import { AuthorizationOverview } from "./components/authorization-overview";
import { MemberRoles } from "./components/member-roles";
import { PermissionDiagnostics } from "./components/permission-diagnostics";
import { RuntimeScopes } from "./components/runtime-scopes";

type PermissionsCenterProps = {
  apiBaseUrl?: string;
  fetcher?: typeof fetch;
};

export function PermissionsCenter({ apiBaseUrl = resolveControlPlaneUrl(), fetcher }: PermissionsCenterProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main fluid>
        <div className="mb-4 flex items-center gap-3">
          <KeyRound className="h-6 w-6 text-muted-foreground" />
          <div>
            <h1 className="text-2xl font-bold tracking-tight">权限中心</h1>
            <p className="text-sm text-muted-foreground">查看授权决策、配置 Runtime 服务范围，并诊断访问问题。</p>
          </div>
        </div>
        <Tabs defaultValue="overview" className="space-y-4">
          <TabsList>
            <TabsTrigger value="overview">授权概览</TabsTrigger>
            <TabsTrigger value="audit">授权审计</TabsTrigger>
            <TabsTrigger value="runtime">Runtime 范围</TabsTrigger>
            <TabsTrigger value="members">成员角色</TabsTrigger>
            <TabsTrigger value="diagnostics">权限诊断</TabsTrigger>
          </TabsList>
          <TabsContent value="overview"><AuthorizationOverview apiOptions={apiOptions} /></TabsContent>
          <TabsContent value="audit"><AuthorizationAuditTable apiOptions={apiOptions} /></TabsContent>
          <TabsContent value="runtime"><RuntimeScopes apiOptions={apiOptions} /></TabsContent>
          <TabsContent value="members"><MemberRoles apiOptions={apiOptions} /></TabsContent>
          <TabsContent value="diagnostics"><PermissionDiagnostics apiOptions={apiOptions} /></TabsContent>
        </Tabs>
      </Main>
    </>
  );
}
```

- [ ] **Step 5: Implement tab components**

Each component should use real query functions from `@/lib/api/authz`.

Required behavior:

- `authorization-overview.tsx`
  - `useQuery(["authz-overview"], () => getAuthzOverview(apiOptions))`
  - Show cards for engine, total, denied, denied rate.
  - Show recent events list.
- `authorization-audit-table.tsx`
  - `useQuery(["authz-decisions"], () => listAuthzDecisions({ ...apiOptions, limit: 50, offset: 0 }))`
  - Use a compact table with 时间、结果、动作、Actor、资源、原因.
  - Empty state text: `暂无授权审计记录。`
- `runtime-scopes.tsx`
  - `useQuery(["runtime-scopes"], () => listRuntimeScopes(apiOptions))`
  - Show nodes and scopes.
  - Provide create form with runtime node ID, tenant ID, optional team ID, scope type, scope value.
  - Use `useMutation` for `createRuntimeScope` and `updateRuntimeScope`.
  - On success invalidate `["runtime-scopes"]`.
- `member-roles.tsx`
  - `useQuery(["authz-members"], () => listAuthzMembers({ ...apiOptions, limit: 50, offset: 0 }))`
  - Read-only table, no edit buttons.
- `permission-diagnostics.tsx`
  - Controlled form.
  - Default actor type `user`, action `console.access`, resource type `console`, resource ID `web`.
  - Button label `开始诊断`.
  - Require Actor ID and tenant ID before submit.
  - Result badge text `允许` or `拒绝`.

Use existing `Card`, `Badge`, `Button`, `Input`, `Label`, `Select`, and `Table` components. Keep layout dense and operational.

- [ ] **Step 6: Run tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/permissions/index.test.tsx src/lib/api/authz.test.ts
```

Expected: PASS.

- [ ] **Step 7: Regenerate route tree and typecheck**

Run:

```bash
pnpm --filter @superteam/web typecheck
```

Expected: PASS. This should refresh `apps/web/src/routeTree.gen.ts` through the Vite/TanStack Router tooling if needed; if it does not, run `pnpm --filter @superteam/web build` and include the generated route tree.

- [ ] **Step 8: Commit web slice**

```bash
git add apps/web/src/routes/_authenticated/permissions apps/web/src/features/permissions apps/web/src/components/layout/data/sidebar-data.ts apps/web/src/routeTree.gen.ts
git commit -m "feat: add permissions center web page"
```

---

## Task 6: End-to-End Verification and Changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add near the top of `CHANGELOG.md`:

```md
- 新增 Web“权限中心”一级菜单和 `/api/authz/*` 权限中心 API，支持授权概览、授权审计、Runtime 范围配置、成员角色只读视图和权限诊断。
```

- [ ] **Step 2: Run backend verification**

Run:

```bash
go test ./apps/control-plane/internal/authz ./apps/control-plane/internal/authzcenter ./apps/control-plane/internal/api ./apps/control-plane/internal/storage/queries -count=1
```

Expected: PASS, with query integration tests either passing or skipping only when required test DB env vars are not configured.

- [ ] **Step 3: Run web verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/authz.test.ts src/features/permissions/index.test.tsx
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected: all commands exit 0.

- [ ] **Step 4: Run contract/generation verification**

Run:

```bash
make -C apps/control-plane generate-sqlc
make -C apps/control-plane generate-openapi
git diff --exit-code -- apps/control-plane/internal/storage/queries apps/control-plane/internal/auth/generated.go apps/control-plane/internal/authzcenter/generated.go
```

Expected: generation commands exit 0 and `git diff --exit-code` exits 0, proving generated files are committed and reproducible.

- [ ] **Step 5: Run full targeted verification**

Run:

```bash
pnpm verify:web
go test ./apps/control-plane/...
```

Expected: PASS.

- [ ] **Step 6: Optional browser smoke test**

Start services if not already running:

```bash
pnpm dev:control-plane
pnpm dev:web
```

Open `http://127.0.0.1:3000/permissions` in the in-app browser. Verify:

- unauthenticated users are redirected to login;
- after login, sidebar shows “权限中心”;
- five tabs are visible;
- Runtime 范围 and 权限诊断 render without layout overlap on desktop width.

- [ ] **Step 7: Commit changelog and verification cleanup**

```bash
git add CHANGELOG.md
git commit -m "docs: record permissions center changes"
```

---

## Self-Review Checklist

- Spec coverage:
  - 一级菜单 `/permissions`: Task 5.
  - 五个 Tabs: Task 5.
  - 授权审计读取真实 `web_operation_logs`: Task 2 and Task 3.
  - Runtime 范围真实读写 `runtime_node_scopes`: Task 2 and Task 3.
  - 成员角色只读: Task 3 and Task 5.
  - 权限诊断 dry-run `Authorizer.Check`: Task 3 and Task 5.
  - 写操作审计: Task 3.
  - 不暴露 OpenFGA DSL: Task 5 UI copy requirements.
  - CHANGELOG: Task 6.
- Placeholder scan:
  - No `TBD`, `TODO`, or “implement later”.
  - Steps that require code include concrete file paths and code skeletons.
- Type consistency:
  - API paths use `/api/authz/*` consistently.
  - Web route uses `/permissions`.
  - Backend app service package is `internal/authzcenter`; authorization engine remains `internal/authz`.
  - Runtime scope statuses are `active` and `disabled`.
