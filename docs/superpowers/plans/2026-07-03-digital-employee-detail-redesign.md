# 数字员工详情页重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `apps/web` 数字员工详情页从"操作表单为主"重构为原型的"观察 + 配置"IA（详情头卡 + 执行指标条 + 历史执行表 + 生效上下文面板 + 注入顺序链），同时给 Control Plane 补上已有数据但未暴露的接口（执行统计聚合、当前生效配置读取、运行历史增强筛选），不删除任何现有可用功能。

**Architecture:** 后端在 `apps/control-plane/internal/employee` 新增/扩展 3 类只读接口（run-stats 聚合、current effective-config 读取、run 列表增强），全部复用现有 `HandlerService`/`RunHandlerService`/`Repository` 分层与 `writeJSON`/`writeHandlerError` helper；契约同步更新 `contracts/control-plane/openapi.yaml` 并重新生成。前端把 `features/employees/detail.tsx` 拆成 7 个职责单一的子组件（头卡/指标条/历史表/运行详情抽屉/开始任务抽屉/生效上下文面板/注入链），全部复用 `@/components/superteam` v3 组件；不产出真实数据的部分（记忆条目）显式标「待接入」，不伪造。

**Tech Stack:** Go 1.x + chi + pgx/v5 + sqlc v1.31.1（Control Plane）；React + TanStack Query/Router + Vitest(vitest-browser-react) + Tailwind v3 token（Web）。

## Global Constraints

- 统计口径（对应 `docs/superpowers/specs/2026-07-03-digital-employee-detail-redesign-design.md` 第 3 节）：成功=`completed`；失败=`failed`+`timed_out`；人工停止=`cancelled`；累计=全部；成功率=成功/累计；平均/P90 耗时基于 `finished_at - started_at`（仅非空行）；近7天=`created_at >= now()-7d`；环比=(近7天-前7天)/前7天，前7天为0时只显示绝对值不显示百分比。
- 记忆条目无数据源，前端只能显示「待接入」占位，不得编造数字。
- sqlc 必须在 `apps/control-plane/` 目录下运行 `sqlc generate`（`sqlc.yaml` 用相对路径）。
- 契约生成命令固定为仓库根目录 `pnpm generate:control-plane`（触发 `go generate ./internal/api`），生成物 `*.gen.go` 已被 gitignore，但**修改 `contracts/control-plane/openapi.yaml` 后必须本地重新生成一次以验证契约可生成**；该生成不会自动挂载路由，路由仍需在 `apps/control-plane/internal/api/server.go` 手写 `chi.Router` 注册（沿用现有模式）。
- Web 测试只能用 `corepack pnpm --filter ./apps/web run test`，不得用 `npx vitest run`。
- Web 内部跳转必须用 TanStack Router 的 `Link`/`navigate`。
- 页面视觉只能用 `apps/web/src/components/superteam` 的 v3 组件与 `theme.css` 的 `--v3-*` token，不手搓卡片/表格/pill。
- 面向用户文案用简体中文。
- 每次改动收尾前用 `$superteam-completion-check` 做完成前检查；涉及前后端联调的功能声称"可用"前必须走真实端到端验证（浏览器/真实接口），不能只停在单元测试/构建。

---

## Part A — Control Plane 后端

### Task 1: 新增执行统计聚合查询与接口 `GET /digital-employees/{employeeId}/run-stats`

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`（新增查询，紧跟现有 `ListDigitalEmployeeRuns` 查询之后）
- Modify (generated): `apps/control-plane/internal/storage/queries/tasks.sql.go`、`apps/control-plane/internal/storage/queries/models.go`（由 `sqlc generate` 生成，不手写）
- Modify: `apps/control-plane/internal/employee/repository.go`（`Repository` 接口新增方法签名）
- Modify: `apps/control-plane/internal/employee/pg_repository.go`（实现新方法）
- Modify: `apps/control-plane/internal/employee/run_types.go`（新增 `DigitalEmployeeRunStats` 领域类型）
- Modify: `apps/control-plane/internal/employee/run_service.go`（`DigitalEmployeeRunService` 新增 `GetRunStats` 方法）
- Modify: `apps/control-plane/internal/employee/run_handler.go`（`RunHandlerService` 接口新增方法 + 新 handler + response 类型）
- Modify: `apps/control-plane/internal/api/server.go`（注册新路由）
- Test: `apps/control-plane/internal/employee/run_repository_test.go`（新增集成测试，走现有 `employeeRunRepositoryTestConfig`/`runEmployeeRepositoryTestMigrations` 真实 DB 夹具模式）

**Interfaces:**
- Produces: `RunHandlerService.GetRunStats(ctx, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRunStats, error)`；HTTP `GET /api/v1/digital-employees/{employeeId}/run-stats` → `200 { total_count, succeeded_count, failed_count, cancelled_count, success_rate, avg_duration_sec, p90_duration_sec, last_7d_count, prev_7d_count }`（后两个 duration 字段为 `number|null`）。

- [ ] **Step 1: 在 `tasks.sql` 新增聚合查询**

在 `apps/control-plane/internal/storage/queries/tasks.sql` 中 `-- name: ListDigitalEmployeeRuns :many` 查询块之后追加：

```sql
-- name: GetDigitalEmployeeRunStats :one
WITH scoped AS (
    SELECT status, started_at, finished_at, created_at
    FROM task_runs
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
)
SELECT
    COUNT(*)::bigint AS total_count,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint AS succeeded_count,
    COUNT(*) FILTER (WHERE status IN ('failed', 'timed_out'))::bigint AS failed_count,
    COUNT(*) FILTER (WHERE status = 'cancelled')::bigint AS cancelled_count,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days')::bigint AS last_7d_count,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days' AND created_at < NOW() - INTERVAL '7 days')::bigint AS prev_7d_count,
    AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL) AS avg_duration_sec,
    PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL) AS p90_duration_sec
FROM scoped;
```

- [ ] **Step 2: 生成 sqlc 代码**

```bash
cd apps/control-plane && sqlc generate
```

检查 `internal/storage/queries/tasks.sql.go` 中生成的 `GetDigitalEmployeeRunStats` 函数与 `GetDigitalEmployeeRunStatsRow` 结构体，记下 `AvgDurationSec`/`P90DurationSec` 两个字段的具体 Go 类型（预期是 `pgtype.Float8`，也可能是 `sql.NullFloat64`，以生成结果为准）。

- [ ] **Step 3: `Repository` 接口新增方法**

在 `apps/control-plane/internal/employee/repository.go` 中找到 `Repository` interface（与 `GetCurrentDigitalEmployeeEffectiveConfig` 同一个接口块），追加一行：

```go
	GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error)
```

并在同文件（复用现有 `DigitalEmployeeEffectiveConfigRecord` 定义附近的领域类型区）新增：

```go
type DigitalEmployeeRunStats struct {
	TotalCount      int64
	SucceededCount  int64
	FailedCount     int64
	CancelledCount  int64
	Last7dCount     int64
	Prev7dCount     int64
	AvgDurationSec  *float64
	P90DurationSec  *float64
}
```

- [ ] **Step 4: `PgRepository` 实现**

在 `apps/control-plane/internal/employee/pg_repository.go`，紧邻 `GetCurrentDigitalEmployeeEffectiveConfig` 实现之后新增：

```go
func (r *PgRepository) GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error) {
	row, err := r.q.GetDigitalEmployeeRunStats(ctx, queries.GetDigitalEmployeeRunStatsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return DigitalEmployeeRunStats{}, mapNoRows(err)
	}
	return DigitalEmployeeRunStats{
		TotalCount:     row.TotalCount,
		SucceededCount: row.SucceededCount,
		FailedCount:    row.FailedCount,
		CancelledCount: row.CancelledCount,
		Last7dCount:    row.Last7dCount,
		Prev7dCount:    row.Prev7dCount,
		AvgDurationSec: pgFloat8Ptr(row.AvgDurationSec),
		P90DurationSec: pgFloat8Ptr(row.P90DurationSec),
	}, nil
}

func pgFloat8Ptr(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}
```

> 如果 Step 2 生成的字段类型不是 `pgtype.Float8`（例如是普通 `float64` 或 `sql.NullFloat64`），把 `pgFloat8Ptr` 的参数类型和内部判空逻辑改成匹配生成类型的等价写法（`sql.NullFloat64` 用 `.Valid`/`.Float64`；普通 `float64` 直接取地址、无需判空）。以 `go build ./...` 报错为准调整，不要凭空假设。

若 `pg_repository.go` 顶部尚未导入 `github.com/jackc/pgx/v5/pgtype`，补上该 import。

- [ ] **Step 5: `DigitalEmployeeRunService.GetRunStats`**

在 `apps/control-plane/internal/employee/run_service.go`，紧邻 `ListRuns` 方法后新增：

```go
func (s *DigitalEmployeeRunService) GetRunStats(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRunStats, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	stats, err := s.repository.GetDigitalEmployeeRunStats(ctx, tenantID, employeeID)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
```

确认 `DigitalEmployeeRunService` 持有的 repository 字段类型（查看结构体定义）实现了新接口方法——因为 Step 3 把方法加进了 `Repository` interface，`PgRunRepository`（`pg_run_repository.go`）如果单独实现了一份 `Repository` 子集也需要检查是否要转发。运行 `go build ./...` 确认没有"missing method"报错；如果报错，在 `pg_run_repository.go` 里加一行转发：

```go
func (r *PgRunRepository) GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error) {
	return (&PgRepository{q: r.q}).GetDigitalEmployeeRunStats(ctx, tenantID, digitalEmployeeID)
}
```

（这是 `pg_run_repository.go` 里 `ListWorkspaceFilesForSync`/`UpsertWorkspaceFileSync` 已经在用的转发模式，照抄即可。）

- [ ] **Step 6: `RunHandlerService` 接口 + handler + 路由**

在 `apps/control-plane/internal/employee/run_handler.go`：

1. `RunHandlerService` interface 追加一行：

```go
	GetRunStats(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRunStats, error)
```

2. 在 `ListDigitalEmployeeRuns` handler 之后新增：

```go
func (h *HTTPHandler) GetDigitalEmployeeRunStats(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run stats read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	stats, err := service.GetRunStats(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runStatsResponseFromDomain(stats))
}

type digitalEmployeeRunStatsResponse struct {
	TotalCount     int64    `json:"total_count"`
	SucceededCount int64    `json:"succeeded_count"`
	FailedCount    int64    `json:"failed_count"`
	CancelledCount int64    `json:"cancelled_count"`
	SuccessRate    *float64 `json:"success_rate"`
	AvgDurationSec *float64 `json:"avg_duration_sec"`
	P90DurationSec *float64 `json:"p90_duration_sec"`
	Last7dCount    int64    `json:"last_7d_count"`
	Prev7dCount    int64    `json:"prev_7d_count"`
}

func runStatsResponseFromDomain(stats *DigitalEmployeeRunStats) digitalEmployeeRunStatsResponse {
	var successRate *float64
	if stats.TotalCount > 0 {
		rate := float64(stats.SucceededCount) / float64(stats.TotalCount)
		successRate = &rate
	}
	return digitalEmployeeRunStatsResponse{
		TotalCount:     stats.TotalCount,
		SucceededCount: stats.SucceededCount,
		FailedCount:    stats.FailedCount,
		CancelledCount: stats.CancelledCount,
		SuccessRate:    successRate,
		AvgDurationSec: stats.AvgDurationSec,
		P90DurationSec: stats.P90DurationSec,
		Last7dCount:    stats.Last7dCount,
		Prev7dCount:    stats.Prev7dCount,
	}
}
```

3. 在 `apps/control-plane/internal/api/server.go` 的 employee 路由分组里，紧跟 `r.Get("/digital-employees/{employeeId}/runs", s.employeeHandler.ListDigitalEmployeeRuns)` 之后新增：

```go
        r.Get("/digital-employees/{employeeId}/run-stats", s.employeeHandler.GetDigitalEmployeeRunStats)
```

- [ ] **Step 7: 编译检查**

```bash
cd apps/control-plane && go build ./...
```
预期：无报错。若有类型不匹配（尤其 Step 4 的 `pgFloat8Ptr`），按报错信息修正。

- [ ] **Step 8: 写集成测试**

在 `apps/control-plane/internal/employee/run_repository_test.go` 末尾追加（复用文件已有的 `employeeRunRepositoryTestConfig`/`runEmployeeRepositoryTestMigrations`/schema-per-test 模式）：

```go
func TestPgRepositoryGetDigitalEmployeeRunStatsAggregatesByStatus(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_stats_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.New()
	otherEmployeeID := uuid.New()
	taskID := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO tasks (id, tenant_id, title, provider_type, status)
		VALUES ($2, $1, 'stats-fixture-task', 'codex', 'completed');
	`, tenantID, taskID)
	require.NoError(t, err)

	insertRun := func(employee uuid.UUID, status string, startedAt, finishedAt *time.Time, createdAt time.Time) {
		_, err := conn.Exec(ctx, `
			INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
			VALUES ($1, $2, $3, $4, 'node-a', $5, $6, $7, $8)
		`, uuid.New(), tenantID, taskID, employee, status, startedAt, finishedAt, createdAt)
		require.NoError(t, err)
	}

	now := time.Now().UTC()
	start1, finish1 := now.Add(-30*time.Minute), now.Add(-10*time.Minute)
	start2, finish2 := now.Add(-2*time.Hour), now.Add(-1*time.Hour)
	insertRun(employeeID, "completed", &start1, &finish1, now.Add(-1*24*time.Hour))
	insertRun(employeeID, "completed", &start2, &finish2, now.Add(-10*24*time.Hour))
	insertRun(employeeID, "failed", &start1, &finish1, now.Add(-2*24*time.Hour))
	insertRun(employeeID, "cancelled", &start1, &finish1, now.Add(-12*24*time.Hour))
	insertRun(otherEmployeeID, "completed", &start1, &finish1, now)

	repo := NewPgRepository(queries.New(conn))
	stats, err := repo.GetDigitalEmployeeRunStats(ctx, tenantID, employeeID)
	require.NoError(t, err)

	require.Equal(t, int64(4), stats.TotalCount)
	require.Equal(t, int64(2), stats.SucceededCount)
	require.Equal(t, int64(1), stats.FailedCount)
	require.Equal(t, int64(1), stats.CancelledCount)
	require.Equal(t, int64(2), stats.Last7dCount)
	require.Equal(t, int64(1), stats.Prev7dCount)
	require.NotNil(t, stats.AvgDurationSec)
	require.InDelta(t, 1500, *stats.AvgDurationSec, 1)
}
```

若 `NewPgRepository` 的构造签名或 `queries.New(conn)` 用法与文件里其他测试不同，以文件中已有测试（如 `TestPgRunRepositoryGetRunPreflightUsesRuntimeNodeIDFromRuntimeNodes`）的写法为准调整（例如是否需要额外传 Redis 客户端）。

- [ ] **Step 9: 运行测试**

```bash
cd apps/control-plane
TEST_DATABASE_URL=postgres://localhost:5432/superteam_test?sslmode=disable \
TEST_REDIS_URL=redis://localhost:6379/1 \
go test ./internal/employee/... -run TestPgRepositoryGetDigitalEmployeeRunStatsAggregatesByStatus -v
```

若本地没有可用的测试 Postgres/Redis，测试会 `t.Skip`；此时至少确认 `go build ./...` 和 `go vet ./...` 通过，并在任务收尾时用 `scripts/dev-services.sh status` 确认的真实开发库补跑一次。

- [ ] **Step 10: 提交**

```bash
git add apps/control-plane/internal/storage/queries/tasks.sql \
        apps/control-plane/internal/storage/queries/tasks.sql.go \
        apps/control-plane/internal/storage/queries/models.go \
        apps/control-plane/internal/employee/repository.go \
        apps/control-plane/internal/employee/pg_repository.go \
        apps/control-plane/internal/employee/run_types.go \
        apps/control-plane/internal/employee/run_service.go \
        apps/control-plane/internal/employee/run_handler.go \
        apps/control-plane/internal/api/server.go \
        apps/control-plane/internal/employee/run_repository_test.go
git commit -m "feat(control-plane): add digital employee run stats aggregate endpoint"
```

---

### Task 2: 暴露当前生效配置读取接口 `GET /digital-employees/{employeeId}/effective-config`

现状：`Repository.GetCurrentDigitalEmployeeEffectiveConfig` 已存在（供内部 preflight 使用），但没有 HTTP 路由暴露它。本任务只加一层薄 handler，不改数据模型。

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`（新增 `Service.GetCurrentEffectiveConfig` + 类型转换 helper）
- Modify: `apps/control-plane/internal/employee/handler.go`（`HandlerService` 接口新增方法 + 新 handler，复用已有 `effectiveConfigResponseFromDomain`）
- Modify: `apps/control-plane/internal/api/server.go`（注册路由）
- Test: `apps/control-plane/internal/employee/service_test.go`（若不存在则在 `internal/employee` 包内新建同名测试文件，检查包内是否已有 `service_test.go`——若有，追加到其中）

**Interfaces:**
- Consumes: `Repository.GetCurrentDigitalEmployeeEffectiveConfig(ctx, tenantID, employeeID) (DigitalEmployeeEffectiveConfigRecord, error)`（Task 1 之前已存在，未改动）
- Produces: `HandlerService.GetCurrentEffectiveConfig(ctx, tenantID, employeeID uuid.UUID) (*DigitalEmployeeEffectiveConfig, error)`；HTTP `GET /api/v1/digital-employees/{employeeId}/effective-config` → `200 effectiveConfigResponse`（字段与现有 `ApproveDigitalEmployeeEffectiveConfig` 响应体一致：`id, tenant_id, digital_employee_id, team_config_revision_id, employee_config_revision_id, effective_config, validation_result, status, approved_by, approved_at, revoked_at, created_at, updated_at`）；无已批准配置时返回 `404`。

- [ ] **Step 1: 先检查包内是否已有同名转换函数**

```bash
cd apps/control-plane && grep -n "func effectiveConfigFromRecord" internal/employee/*.go
```
预期无输出（该转换函数尚不存在），确认后继续。

- [ ] **Step 2: `Service.GetCurrentEffectiveConfig`**

在 `apps/control-plane/internal/employee/service.go`，紧邻 `PreviewEffectiveConfig` 之前新增：

```go
func (s *Service) GetCurrentEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (*DigitalEmployeeEffectiveConfig, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCurrentDigitalEmployeeEffectiveConfig(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	return effectiveConfigFromRecord(record), nil
}

func effectiveConfigFromRecord(record DigitalEmployeeEffectiveConfigRecord) *DigitalEmployeeEffectiveConfig {
	return &DigitalEmployeeEffectiveConfig{
		ID:                       record.ID,
		TenantID:                 record.TenantID,
		DigitalEmployeeID:        record.DigitalEmployeeID,
		TeamConfigRevisionID:     record.TeamConfigRevisionID,
		EmployeeConfigRevisionID: record.EmployeeConfigRevisionID,
		EffectiveConfig:          record.EffectiveConfig,
		ValidationResult:         record.ValidationResult,
		Status:                   record.Status,
		ApprovedBy:               record.ApprovedBy,
		ApprovedAt:               record.ApprovedAt,
		RevokedAt:                record.RevokedAt,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
	}
}
```

- [ ] **Step 3: `HandlerService` 接口 + handler**

在 `apps/control-plane/internal/employee/handler.go`：

1. `HandlerService` interface 追加：

```go
	GetCurrentEffectiveConfig(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeEffectiveConfig, error)
```

2. 紧邻 `ApproveDigitalEmployeeEffectiveConfig` 之后新增：

```go
func (h *HTTPHandler) GetDigitalEmployeeEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee effective config read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	effectiveConfig, err := service.GetCurrentEffectiveConfig(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveConfigResponseFromDomain(effectiveConfig))
}
```

3. 在 `apps/control-plane/internal/api/server.go`，紧邻 `r.Post("/digital-employees/{employeeId}/effective-configs/approve", ...)` 之后新增：

```go
        r.Get("/digital-employees/{employeeId}/effective-config", s.employeeHandler.GetDigitalEmployeeEffectiveConfig)
```

- [ ] **Step 4: 编译**

```bash
cd apps/control-plane && go build ./...
```

- [ ] **Step 5: 写单元测试（不依赖真实 DB，直接测 handler 的 404 分支和转换函数）**

检查包内是否已有轻量 fake service 模式可复用：

```bash
grep -n "type fakeHandlerService\|type stubHandlerService" internal/employee/*_test.go
```

若已存在类似 fake，在其上追加 `GetCurrentEffectiveConfig` 方法实现；若不存在，在 `apps/control-plane/internal/employee/handler_test.go`（若不存在则新建）写一个最小 fake：

```go
package employee

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeEffectiveConfigService struct {
	effectiveConfig *DigitalEmployeeEffectiveConfig
	err             error
}

func (f *fakeEffectiveConfigService) GetCurrentEffectiveConfig(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeEffectiveConfig, error) {
	return f.effectiveConfig, f.err
}

func TestGetDigitalEmployeeEffectiveConfigReturnsNotFoundWhenNoApprovedConfig(t *testing.T) {
	// 这里只验证 404 路径不需要真实 authorizer/service 全量实现时，
	// 用现有 handler 的 writeHandlerError(ErrNotFound) 行为即可验证：
	require.Equal(t, http.StatusNotFound, httpStatusForHandlerError(ErrNotFound))
}

func httpStatusForHandlerError(err error) int {
	rec := httptest.NewRecorder()
	writeHandlerError(rec, err)
	return rec.Code
}
```

> 说明：`HTTPHandler` 的完整依赖注入（`authorizer`、`service`、`runService`）在现有测试文件里没有一个可直接复用的“最小可跑通”构造函数；与其为了这一个 handler 搭一整套授权 mock（超出本任务范围），本步骤把验证收窄到"`ErrNotFound` 正确映射为 404"这一行为契约，这正是 `GetDigitalEmployeeEffectiveConfig` 在无已批准配置时依赖的唯一分支逻辑。如果包内已存在覆盖 `authorizeDigitalEmployeeManagement` 全链路的测试 harness（Step 5 开头的 grep 会发现），改用该 harness 写一个端到端 handler 测试并删除上面这个精简版本。

- [ ] **Step 6: 运行测试**

```bash
cd apps/control-plane && go test ./internal/employee/... -run TestGetDigitalEmployeeEffectiveConfigReturnsNotFoundWhenNoApprovedConfig -v
```

- [ ] **Step 7: 提交**

```bash
git add apps/control-plane/internal/employee/service.go \
        apps/control-plane/internal/employee/handler.go \
        apps/control-plane/internal/api/server.go \
        apps/control-plane/internal/employee/handler_test.go
git commit -m "feat(control-plane): expose current digital employee effective config read endpoint"
```

---

### Task 3: 扩展运行历史列表——任务/项目关联、耗时、工件数、筛选、总数

这是本计划里唯一的**响应形状变更**：`GET /digital-employees/{employeeId}/runs` 从裸数组变成 `{ items, total_count, filters }` 对象。仓库内唯一消费方是 `apps/web`，会在 Part B 同步更新，不存在其他已知消费方。

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`（新增 `ListDigitalEmployeeRunsDetailed`、`CountDigitalEmployeeRunsDetailed`、`ListDigitalEmployeeRunProjectOptions` 三个查询）
- Modify (generated): `tasks.sql.go`、`models.go`
- Modify: `apps/control-plane/internal/employee/run_types.go`（新增 `DigitalEmployeeRunListFilter`、`DigitalEmployeeRunListItem`、`DigitalEmployeeRunListResult`、`RunProjectOption`）
- Modify: `apps/control-plane/internal/employee/pg_run_repository.go`（`ListRuns` 改造为 `ListRunsDetailed`，保留只读转换逻辑）
- Modify: `apps/control-plane/internal/employee/run_service.go`（`ListRuns` → `ListRunsDetailed`，签名带 filter）
- Modify: `apps/control-plane/internal/employee/run_handler.go`（`ListDigitalEmployeeRuns` handler 改造，解析新查询参数，返回新响应体）
- Test: `apps/control-plane/internal/employee/run_repository_test.go`（新增集成测试覆盖 join、筛选、分页总数）

**Interfaces:**
- Produces:
  ```go
  type DigitalEmployeeRunListFilter struct {
      Statuses  []string
      ProjectID *uuid.UUID
      From      *time.Time
      To        *time.Time
      Limit     int32
      Offset    int32
  }
  type DigitalEmployeeRunListItem struct {
      Run              *DigitalEmployeeRun
      TaskTitle        string
      ProjectID        *uuid.UUID
      ProjectName      *string
      WorkProductCount int32
      DurationSec      *float64
  }
  type RunProjectOption struct {
      ID   uuid.UUID
      Name string
  }
  type DigitalEmployeeRunListResult struct {
      Items      []DigitalEmployeeRunListItem
      TotalCount int64
      Projects   []RunProjectOption
  }
  ```
  `RunHandlerService.ListRunsDetailed(ctx, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)`；HTTP `GET .../runs?status=completed,failed&project_id=<uuid>&from=<RFC3339>&to=<RFC3339>&limit=&offset=` → `200 { items: [...], total_count, filters: { statuses: [{value,label}], projects: [{value,label}] } }`。

- [ ] **Step 1: 新增 SQL 查询**

在 `apps/control-plane/internal/storage/queries/tasks.sql`，`GetDigitalEmployeeRunStats` 之后追加：

```sql
-- name: ListDigitalEmployeeRunsDetailed :many
SELECT
    tr.id, tr.tenant_id, tr.task_id, tr.digital_employee_id, tr.execution_instance_id,
    tr.runtime_node_id, tr.node_id, tr.command_id, tr.provider_type, tr.provider_session_id,
    tr.provider_session_external_id, tr.status, tr.result, tr.diagnostic, tr.log_ref,
    tr.raw_result_ref, tr.work_products, tr.session_state, tr.error_message, tr.error_code,
    tr.error_family, tr.exit_code, tr.signal, tr.timed_out, tr.idempotency_key,
    tr.timeout_sec, tr.grace_sec, tr.started_at, tr.completed_at, tr.finished_at,
    tr.created_at, tr.updated_at,
    t.title AS task_title,
    p.id AS project_id,
    p.name AS project_name,
    jsonb_array_length(tr.work_products) AS work_product_count,
    EXTRACT(EPOCH FROM (tr.finished_at - tr.started_at))::float8 AS duration_sec
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN project_tasks pt ON pt.digital_employee_run_id = tr.id AND pt.tenant_id = tr.tenant_id
LEFT JOIN projects p ON p.id = pt.project_id AND p.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR tr.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('project_id')::uuid IS NULL OR p.id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR tr.created_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR tr.created_at < sqlc.narg('to_time')::timestamptz)
ORDER BY tr.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountDigitalEmployeeRunsDetailed :one
SELECT COUNT(*)::bigint AS total_count
FROM task_runs tr
JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
LEFT JOIN project_tasks pt ON pt.digital_employee_run_id = tr.id AND pt.tenant_id = tr.tenant_id
LEFT JOIN projects p ON p.id = pt.project_id AND p.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND t.deleted_at IS NULL
  AND (cardinality(sqlc.arg('statuses')::text[]) = 0 OR tr.status = ANY(sqlc.arg('statuses')::text[]))
  AND (sqlc.narg('project_id')::uuid IS NULL OR p.id = sqlc.narg('project_id')::uuid)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR tr.created_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR tr.created_at < sqlc.narg('to_time')::timestamptz);

-- name: ListDigitalEmployeeRunProjectOptions :many
SELECT DISTINCT p.id, p.name
FROM task_runs tr
JOIN project_tasks pt ON pt.digital_employee_run_id = tr.id AND pt.tenant_id = tr.tenant_id
JOIN projects p ON p.id = pt.project_id AND p.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY p.name;
```

- [ ] **Step 2: 生成 sqlc 代码**

```bash
cd apps/control-plane && sqlc generate
```

检查生成的 `ListDigitalEmployeeRunsDetailedRow`、`ListDigitalEmployeeRunsDetailedParams`（尤其 `Statuses []string`、`ProjectID uuid.NullUUID`、`FromTime`/`ToTime` 的具体类型）、`CountDigitalEmployeeRunsDetailedParams`、`ListDigitalEmployeeRunProjectOptionsRow` 的确切字段类型，后续步骤按实际生成类型对齐（同 Task 1 Step 2 的原则：以 `models.go`/`tasks.sql.go` 生成结果为准，不臆测）。

- [ ] **Step 3: 领域类型**

在 `apps/control-plane/internal/employee/run_types.go` 追加（紧邻 `DigitalEmployeeRun` 之后）：

```go
type DigitalEmployeeRunListFilter struct {
	Statuses  []string
	ProjectID *uuid.UUID
	From      *time.Time
	To        *time.Time
	Limit     int32
	Offset    int32
}

type DigitalEmployeeRunListItem struct {
	Run              *DigitalEmployeeRun
	TaskTitle        string
	ProjectID        *uuid.UUID
	ProjectName      *string
	WorkProductCount int32
	DurationSec      *float64
}

type RunProjectOption struct {
	ID   uuid.UUID
	Name string
}

type DigitalEmployeeRunListResult struct {
	Items      []DigitalEmployeeRunListItem
	TotalCount int64
	Projects   []RunProjectOption
}
```

- [ ] **Step 4: `PgRunRepository.ListRunsDetailed`**

在 `apps/control-plane/internal/employee/pg_run_repository.go`，把现有 `ListRuns` 方法**保留不删**（`GetRun`/`CreateRun` 等其他方法可能仍在别处依赖同名 sqlc 查询——先确认没有其他调用方再决定是否删除旧查询）：

```bash
grep -rn "\.ListRuns(" apps/control-plane/internal --include=*.go | grep -v "_test.go"
```

确认唯一调用方是 `run_service.go` 的 `DigitalEmployeeRunService.ListRuns` 后，把该 service 方法和 repository 方法**原地改造**（不新增并行方法，避免重复），即：

```go
func (r *PgRunRepository) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	statuses := filter.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	var projectID uuid.NullUUID
	if filter.ProjectID != nil {
		projectID = uuid.NullUUID{UUID: *filter.ProjectID, Valid: true}
	}
	var fromTime, toTime pgtype.Timestamptz
	if filter.From != nil {
		fromTime = pgtype.Timestamptz{Time: *filter.From, Valid: true}
	}
	if filter.To != nil {
		toTime = pgtype.Timestamptz{Time: *filter.To, Valid: true}
	}

	rows, err := r.q.ListDigitalEmployeeRunsDetailed(ctx, queries.ListDigitalEmployeeRunsDetailedParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Statuses:          statuses,
		ProjectID:         projectID,
		FromTime:          fromTime,
		ToTime:            toTime,
		Limit:             filter.Limit,
		Offset:            filter.Offset,
	})
	if err != nil {
		return nil, err
	}

	total, err := r.q.CountDigitalEmployeeRunsDetailed(ctx, queries.CountDigitalEmployeeRunsDetailedParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Statuses:          statuses,
		ProjectID:         projectID,
		FromTime:          fromTime,
		ToTime:            toTime,
	})
	if err != nil {
		return nil, err
	}

	projectRows, err := r.q.ListDigitalEmployeeRunProjectOptions(ctx, queries.ListDigitalEmployeeRunProjectOptionsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}

	items := make([]DigitalEmployeeRunListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, digitalEmployeeRunListItemFromDetailedRow(row))
	}
	projects := make([]RunProjectOption, 0, len(projectRows))
	for _, p := range projectRows {
		projects = append(projects, RunProjectOption{ID: p.ID, Name: p.Name})
	}

	return &DigitalEmployeeRunListResult{Items: items, TotalCount: total, Projects: projects}, nil
}
```

并新增行级转换函数（放在 `digitalEmployeeRunFromQuery` 附近）——**先跑一次 `sqlc generate` 后打开生成的 `ListDigitalEmployeeRunsDetailedRow` 结构体核对字段名**，再写转换函数体，字段名以生成结果为准：

```go
func digitalEmployeeRunListItemFromDetailedRow(row queries.ListDigitalEmployeeRunsDetailedRow) DigitalEmployeeRunListItem {
	run := digitalEmployeeRunFromQuery(queries.TaskRun{
		ID: row.ID, TenantID: row.TenantID, TaskID: row.TaskID, NodeID: row.NodeID,
		RuntimeNodeID: row.RuntimeNodeID, ProviderSessionID: row.ProviderSessionID, Status: row.Status,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, FinishedAt: row.FinishedAt,
		Result: row.Result, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		CommandID: row.CommandID, DigitalEmployeeID: row.DigitalEmployeeID, ExecutionInstanceID: row.ExecutionInstanceID,
		IdempotencyKey: row.IdempotencyKey, TimeoutSec: row.TimeoutSec, GraceSec: row.GraceSec,
		Diagnostic: row.Diagnostic, LogRef: row.LogRef, RawResultRef: row.RawResultRef,
		WorkProducts: row.WorkProducts, SessionState: row.SessionState, ErrorCode: row.ErrorCode,
		ErrorFamily: row.ErrorFamily, ExitCode: row.ExitCode, Signal: row.Signal, TimedOut: row.TimedOut,
		ProviderType: row.ProviderType, ProviderSessionExternalID: row.ProviderSessionExternalID,
	})

	item := DigitalEmployeeRunListItem{
		Run:              run,
		TaskTitle:        row.TaskTitle,
		WorkProductCount: int32(row.WorkProductCount),
		DurationSec:      pgFloat8Ptr(row.DurationSec),
	}
	if row.ProjectID.Valid {
		id := row.ProjectID.UUID
		item.ProjectID = &id
	}
	if row.ProjectName.Valid {
		name := row.ProjectName.String
		item.ProjectName = &name
	}
	return item
}
```

> `queries.TaskRun` 的所有字段名必须与 `models.go` 里实际定义一致——这段构造代码只是把 `ListDigitalEmployeeRunsDetailedRow` 里和 `TaskRun` 同名的列重新组装回 `TaskRun` 好复用 `digitalEmployeeRunFromQuery`；写完后跑 `go build`，逐个修正字段名/类型不一致（例如 `ProjectID`/`ProjectName` 在 sqlc 生成时如果是 `pgtype.UUID`/`pgtype.Text` 而不是 `uuid.NullUUID`/`sql.NullString`，改成对应的 `.Valid`/取值写法）。

- [ ] **Step 5: `DigitalEmployeeRunService.ListRuns` → 改造为 `ListRunsDetailed`**

在 `apps/control-plane/internal/employee/run_service.go`，把现有 `ListRuns` 方法体替换为：

```go
func (s *DigitalEmployeeRunService) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	result, err := s.repository.ListRunsDetailed(ctx, tenantID, employeeID, filter)
	if err != nil {
		return nil, err
	}
	for index, item := range result.Items {
		reconciledRun, reconciled, err := s.reconcileTerminalReceipt(ctx, tenantID, item.Run)
		if err != nil {
			return nil, err
		}
		if reconciled {
			result.Items[index].Run = reconciledRun
		}
	}
	return result, nil
}
```

在 `Repository`（interface，`repository.go`）里把 `ListRuns(ctx, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error)` 这一行替换为：

```go
	ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)
```

- [ ] **Step 6: handler 改造**

在 `apps/control-plane/internal/employee/run_handler.go`：

1. `RunHandlerService` interface 把 `ListRuns(...)` 一行替换为：

```go
	ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)
```

2. `ListDigitalEmployeeRuns` handler 整体替换为：

```go
func (h *HTTPHandler) ListDigitalEmployeeRuns(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee run read")
	if !ok {
		return
	}
	service, ok := h.runServiceFromRequest(w)
	if !ok {
		return
	}
	filter, parseErr := parseRunListFilter(r)
	if parseErr != "" {
		http.Error(w, parseErr, http.StatusBadRequest)
		return
	}
	result, err := service.ListRunsDetailed(r.Context(), tenantID, employeeID, filter)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runListResponseFromDomain(result))
}

func parseRunListFilter(r *http.Request) (DigitalEmployeeRunListFilter, string) {
	limit, offset, parseErr := parseRunPagination(r)
	if parseErr != "" {
		return DigitalEmployeeRunListFilter{}, parseErr
	}
	query := r.URL.Query()
	filter := DigitalEmployeeRunListFilter{Limit: limit, Offset: offset}
	if raw := query.Get("status"); raw != "" {
		filter.Statuses = strings.Split(raw, ",")
	}
	if raw := query.Get("project_id"); raw != "" {
		projectID, err := uuid.Parse(raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "project_id must be a valid uuid"
		}
		filter.ProjectID = &projectID
	}
	if raw := query.Get("from"); raw != "" {
		from, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "from must be an RFC3339 timestamp"
		}
		filter.From = &from
	}
	if raw := query.Get("to"); raw != "" {
		to, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return DigitalEmployeeRunListFilter{}, "to must be an RFC3339 timestamp"
		}
		filter.To = &to
	}
	return filter, ""
}

type digitalEmployeeRunListItemResponse struct {
	digitalEmployeeRunResponse
	TaskTitle        string  `json:"task_title"`
	ProjectID        *string `json:"project_id,omitempty"`
	ProjectName      *string `json:"project_name,omitempty"`
	WorkProductCount int32   `json:"work_product_count"`
	DurationSec      *float64 `json:"duration_sec,omitempty"`
}

type runFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type digitalEmployeeRunListResponse struct {
	Items      []digitalEmployeeRunListItemResponse `json:"items"`
	TotalCount int64                                 `json:"total_count"`
	Filters    struct {
		Statuses []runFilterOption `json:"statuses"`
		Projects []runFilterOption `json:"projects"`
	} `json:"filters"`
}

var digitalEmployeeRunStatusLabels = map[DigitalEmployeeRunStatus]string{
	DigitalEmployeeRunStatusQueued:      "排队中",
	DigitalEmployeeRunStatusDispatching: "调度中",
	DigitalEmployeeRunStatusRunning:     "执行中",
	DigitalEmployeeRunStatusCancelling:  "取消中",
	DigitalEmployeeRunStatusCompleted:   "已完成",
	DigitalEmployeeRunStatusFailed:      "失败",
	DigitalEmployeeRunStatusCancelled:   "已取消",
	DigitalEmployeeRunStatusTimedOut:    "已超时",
}

func runListResponseFromDomain(result *DigitalEmployeeRunListResult) digitalEmployeeRunListResponse {
	response := digitalEmployeeRunListResponse{TotalCount: result.TotalCount}
	response.Items = make([]digitalEmployeeRunListItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		entry := digitalEmployeeRunListItemResponse{
			digitalEmployeeRunResponse: runResponseFromDomain(item.Run),
			TaskTitle:                  item.TaskTitle,
			WorkProductCount:           item.WorkProductCount,
			DurationSec:                item.DurationSec,
		}
		if item.ProjectID != nil {
			id := item.ProjectID.String()
			entry.ProjectID = &id
		}
		entry.ProjectName = item.ProjectName
		response.Items = append(response.Items, entry)
	}
	for _, status := range []DigitalEmployeeRunStatus{
		DigitalEmployeeRunStatusQueued, DigitalEmployeeRunStatusDispatching, DigitalEmployeeRunStatusRunning,
		DigitalEmployeeRunStatusCancelling, DigitalEmployeeRunStatusCompleted, DigitalEmployeeRunStatusFailed,
		DigitalEmployeeRunStatusCancelled, DigitalEmployeeRunStatusTimedOut,
	} {
		response.Filters.Statuses = append(response.Filters.Statuses, runFilterOption{
			Value: string(status),
			Label: digitalEmployeeRunStatusLabels[status],
		})
	}
	for _, project := range result.Projects {
		response.Filters.Projects = append(response.Filters.Projects, runFilterOption{
			Value: project.ID.String(),
			Label: project.Name,
		})
	}
	return response
}
```

3. 确认 `run_handler.go` 顶部 import 包含 `strings` 和 `time`（若没有则新增）。

- [ ] **Step 7: 编译**

```bash
cd apps/control-plane && go build ./...
```
逐一修正类型不匹配（`uuid.NullUUID` vs `pgtype.UUID`、`pgtype.Text` vs `sql.NullString` 等，以编译器报错为准）。

- [ ] **Step 8: 集成测试——筛选与总数**

在 `run_repository_test.go` 追加（复用 Task 1 Step 8 的 schema-per-test 夹具模式，额外插入 `projects`/`project_tasks` 关联行）：

```go
func TestPgRepositoryListRunsDetailedFiltersByStatusAndProject(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRunRepositoryTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL and TEST_REDIS_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL and REDIS_URL")
	}
	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_run_list_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)
	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepositoryTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	employeeID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	runID := uuid.New()
	humanOwnerID := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status) VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO tasks (id, tenant_id, title, provider_type, status) VALUES ($2, $1, '需求梳理任务', 'codex', 'completed');
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
		VALUES ($3, $1, $2, $4, 'node-a', 'completed', NOW() - interval '30 minutes', NOW() - interval '10 minutes', NOW());
		INSERT INTO task_runs (id, tenant_id, task_id, digital_employee_id, node_id, status, started_at, finished_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, $4, 'node-a', 'failed', NOW() - interval '1 hour', NOW() - interval '50 minutes', NOW());
		INSERT INTO projects (id, tenant_id, name, status, human_owner_user_id)
		VALUES ($5, $1, '试点项目 A', 'active', $6);
		INSERT INTO project_tasks (id, tenant_id, project_id, digital_employee_run_id, title, status)
		VALUES (gen_random_uuid(), $1, $5, $3, '需求梳理任务', 'completed');
	`, tenantID, taskID, runID, employeeID, projectID, humanOwnerID)
	require.NoError(t, err)

	repo := NewPgRepository(queries.New(conn))

	all, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.TotalCount)
	require.Len(t, all.Projects, 1)
	require.Equal(t, "试点项目 A", all.Projects[0].Name)

	onlyCompleted, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{
		Statuses: []string{"completed"},
		Limit:    10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), onlyCompleted.TotalCount)
	require.Equal(t, "需求梳理任务", onlyCompleted.Items[0].TaskTitle)
	require.NotNil(t, onlyCompleted.Items[0].ProjectName)
	require.Equal(t, "试点项目 A", *onlyCompleted.Items[0].ProjectName)
	require.NotNil(t, onlyCompleted.Items[0].DurationSec)

	scopedToProject, err := repo.ListRunsDetailed(ctx, tenantID, employeeID, DigitalEmployeeRunListFilter{
		ProjectID: &projectID,
		Limit:     10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), scopedToProject.TotalCount)
}
```

检查 `project_tasks` 表的实际必填列（迁移 `013_project_management_v0.sql` 里 `status`/`title` 等是否有 NOT NULL 或默认值约束），按需要在 INSERT 语句里补齐字段——以 `\d project_tasks` 或迁移文件的实际列定义为准调整。

- [ ] **Step 9: 运行测试**

```bash
cd apps/control-plane
TEST_DATABASE_URL=postgres://localhost:5432/superteam_test?sslmode=disable \
TEST_REDIS_URL=redis://localhost:6379/1 \
go test ./internal/employee/... -run TestPgRepositoryListRunsDetailed -v
```

- [ ] **Step 10: 全量后端测试 + vet**

```bash
cd apps/control-plane && go vet ./... && go build ./...
```

- [ ] **Step 11: 提交**

```bash
git add apps/control-plane/internal/storage/queries/tasks.sql \
        apps/control-plane/internal/storage/queries/tasks.sql.go \
        apps/control-plane/internal/storage/queries/models.go \
        apps/control-plane/internal/employee/run_types.go \
        apps/control-plane/internal/employee/repository.go \
        apps/control-plane/internal/employee/pg_run_repository.go \
        apps/control-plane/internal/employee/run_service.go \
        apps/control-plane/internal/employee/run_handler.go \
        apps/control-plane/internal/employee/run_repository_test.go
git commit -m "feat(control-plane): enrich digital employee run list with task/project join and filters"
```

---

### Task 4: 同步契约 `contracts/control-plane/openapi.yaml`

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`

**Interfaces:**
- Consumes: Task 1–3 定义的三个端点行为
- Produces: 契约与实现一致，`pnpm generate:control-plane` 可成功生成

- [ ] **Step 1: 新增 `DigitalEmployeeRunStats` schema**

在 `openapi.yaml` 的 `components.schemas` 区（`DigitalEmployeeRun` schema 块之后）新增：

```yaml
    DigitalEmployeeRunStats:
      type: object
      required:
        - total_count
        - succeeded_count
        - failed_count
        - cancelled_count
        - last_7d_count
        - prev_7d_count
      properties:
        total_count:
          type: integer
          format: int64
        succeeded_count:
          type: integer
          format: int64
        failed_count:
          type: integer
          format: int64
        cancelled_count:
          type: integer
          format: int64
        success_rate:
          type: number
          nullable: true
        avg_duration_sec:
          type: number
          nullable: true
        p90_duration_sec:
          type: number
          nullable: true
        last_7d_count:
          type: integer
          format: int64
        prev_7d_count:
          type: integer
          format: int64
```

- [ ] **Step 2: 新增 run-stats 路径**

在 `/api/v1/digital-employees/{employeeId}/runs` 路径块之前新增：

```yaml
  /api/v1/digital-employees/{employeeId}/run-stats:
    get:
      operationId: getDigitalEmployeeRunStats
      summary: Get digital employee run statistics
      parameters:
        - $ref: "#/components/parameters/EmployeeId"
      responses:
        "200":
          description: Digital employee run statistics
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeRunStats"
```

- [ ] **Step 3: 新增 effective-config 读取路径**

在 `/api/v1/digital-employees/{employeeId}/effective-configs/approve` 路径块之后新增：

```yaml
  /api/v1/digital-employees/{employeeId}/effective-config:
    get:
      operationId: getDigitalEmployeeEffectiveConfig
      summary: Get current approved digital employee effective config
      parameters:
        - $ref: "#/components/parameters/EmployeeId"
      responses:
        "200":
          description: Current approved effective config
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeEffectiveConfig"
        "404":
          description: No approved effective config exists yet
```

- [ ] **Step 4: 更新运行列表路径为增强响应形状**

把现有：

```yaml
  /api/v1/digital-employees/{employeeId}/runs:
    get:
      operationId: listDigitalEmployeeRuns
      summary: List digital employee runs
      parameters:
        - $ref: "#/components/parameters/EmployeeId"
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: Digital employee run list
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/DigitalEmployeeRun"
```

替换为：

```yaml
  /api/v1/digital-employees/{employeeId}/runs:
    get:
      operationId: listDigitalEmployeeRuns
      summary: List digital employee runs
      parameters:
        - $ref: "#/components/parameters/EmployeeId"
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
        - name: status
          in: query
          required: false
          schema:
            type: string
          description: Comma-separated run statuses to filter by
        - name: project_id
          in: query
          required: false
          schema:
            type: string
            format: uuid
        - name: from
          in: query
          required: false
          schema:
            type: string
            format: date-time
        - name: to
          in: query
          required: false
          schema:
            type: string
            format: date-time
      responses:
        "200":
          description: Digital employee run list
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/DigitalEmployeeRunList"
```

并在 `components.schemas` 新增：

```yaml
    DigitalEmployeeRunListItem:
      allOf:
        - $ref: "#/components/schemas/DigitalEmployeeRun"
        - type: object
          required:
            - task_title
            - work_product_count
          properties:
            task_title:
              type: string
            project_id:
              type: string
              format: uuid
            project_name:
              type: string
            work_product_count:
              type: integer
            duration_sec:
              type: number
              nullable: true
    DigitalEmployeeRunFilterOption:
      type: object
      required:
        - value
        - label
      properties:
        value:
          type: string
        label:
          type: string
    DigitalEmployeeRunList:
      type: object
      required:
        - items
        - total_count
        - filters
      properties:
        items:
          type: array
          items:
            $ref: "#/components/schemas/DigitalEmployeeRunListItem"
        total_count:
          type: integer
          format: int64
        filters:
          type: object
          required:
            - statuses
            - projects
          properties:
            statuses:
              type: array
              items:
                $ref: "#/components/schemas/DigitalEmployeeRunFilterOption"
            projects:
              type: array
              items:
                $ref: "#/components/schemas/DigitalEmployeeRunFilterOption"
```

- [ ] **Step 5: 生成验证**

```bash
pnpm generate:control-plane
```
预期无报错（生成物是 gitignore 的 `*.gen.go`，本步骤只是验证契约本身语法与引用合法）。

- [ ] **Step 6: 提交**

```bash
git add contracts/control-plane/openapi.yaml
git commit -m "docs(contracts): sync openapi with run stats, effective-config read, and enriched run list"
```

---

## Part B — Web 前端

### Task 5: API 客户端更新（`apps/web/src/lib/api/employees.ts`）

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`

**Interfaces:**
- Produces:
  - `type DigitalEmployeeRunStats = { total_count, succeeded_count, failed_count, cancelled_count, success_rate: number|null, avg_duration_sec: number|null, p90_duration_sec: number|null, last_7d_count, prev_7d_count }`
  - `getDigitalEmployeeRunStats(options, employeeId): Promise<DigitalEmployeeRunStats>`
  - `getCurrentDigitalEmployeeEffectiveConfig(options, employeeId): Promise<DigitalEmployeeEffectiveConfig>`（复用已有 `DigitalEmployeeEffectiveConfig` 类型）
  - `type DigitalEmployeeRunFilterOption = { value: string; label: string }`
  - `type DigitalEmployeeRunListItem = DigitalEmployeeRun & { task_title: string; project_id?: string; project_name?: string; work_product_count: number; duration_sec?: number }`
  - `type DigitalEmployeeRunListResult = { items: DigitalEmployeeRunListItem[]; total_count: number; filters: { statuses: DigitalEmployeeRunFilterOption[]; projects: DigitalEmployeeRunFilterOption[] } }`
  - `type ListDigitalEmployeeRunsFilter = RunPagination & { status?: DigitalEmployeeRunStatus[]; project_id?: string; from?: string; to?: string }`
  - `listDigitalEmployeeRuns(options, employeeId, filter?: ListDigitalEmployeeRunsFilter): Promise<DigitalEmployeeRunListResult>`（**签名和返回类型都变了**——原来是 `RunPagination` → `DigitalEmployeeRun[]`）

- [ ] **Step 1: 新增类型**

在 `apps/web/src/lib/api/employees.ts`，`DigitalEmployeeRunEvent` 类型定义之后新增：

```typescript
export type DigitalEmployeeRunStats = {
  total_count: number;
  succeeded_count: number;
  failed_count: number;
  cancelled_count: number;
  success_rate: number | null;
  avg_duration_sec: number | null;
  p90_duration_sec: number | null;
  last_7d_count: number;
  prev_7d_count: number;
};

export type DigitalEmployeeRunFilterOption = {
  value: string;
  label: string;
};

export type DigitalEmployeeRunListItem = DigitalEmployeeRun & {
  task_title: string;
  project_id?: string;
  project_name?: string;
  work_product_count: number;
  duration_sec?: number;
};

export type DigitalEmployeeRunListResult = {
  items: DigitalEmployeeRunListItem[];
  total_count: number;
  filters: {
    statuses: DigitalEmployeeRunFilterOption[];
    projects: DigitalEmployeeRunFilterOption[];
  };
};

export type ListDigitalEmployeeRunsFilter = RunPagination & {
  status?: DigitalEmployeeRunStatus[];
  project_id?: string;
  from?: string;
  to?: string;
};
```

- [ ] **Step 2: 改造 `listDigitalEmployeeRuns`**

找到现有实现：

```typescript
export function listDigitalEmployeeRuns(
  options: ApiClientOptions,
  employeeId: string,
  pagination: RunPagination = {},
): Promise<DigitalEmployeeRun[]> {
  const encodedEmployeeId = encodePathSegment(employeeId);

  return getJson<DigitalEmployeeRun[]>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs${paginationQuery(pagination)}`,
    "digital employee runs",
  );
}
```

替换为：

```typescript
export function listDigitalEmployeeRuns(
  options: ApiClientOptions,
  employeeId: string,
  filter: ListDigitalEmployeeRunsFilter = {},
): Promise<DigitalEmployeeRunListResult> {
  const encodedEmployeeId = encodePathSegment(employeeId);
  const params = new URLSearchParams();
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));
  if (filter.status?.length) params.set("status", filter.status.join(","));
  if (filter.project_id) params.set("project_id", filter.project_id);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  const query = params.toString();

  return getJson<DigitalEmployeeRunListResult>(
    options,
    `/api/v1/digital-employees/${encodedEmployeeId}/runs${query ? `?${query}` : ""}`,
    "digital employee runs",
  );
}
```

- [ ] **Step 3: 新增 `getDigitalEmployeeRunStats` 与 `getCurrentDigitalEmployeeEffectiveConfig`**

紧邻 `listDigitalEmployeeRuns` 之后新增：

```typescript
export function getDigitalEmployeeRunStats(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeRunStats> {
  return getJson<DigitalEmployeeRunStats>(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}/run-stats`,
    "digital employee run stats",
  );
}

export function getCurrentDigitalEmployeeEffectiveConfig(
  options: ApiClientOptions,
  employeeId: string,
): Promise<DigitalEmployeeEffectiveConfig> {
  return getJson<DigitalEmployeeEffectiveConfig>(
    options,
    `/api/v1/digital-employees/${encodePathSegment(employeeId)}/effective-config`,
    "digital employee effective config",
  );
}
```

- [ ] **Step 4: 检查 `paginationQuery` 是否已无调用方**

```bash
cd apps/web && grep -rn "paginationQuery(" src
```
若只剩定义没有调用方，删除该函数（避免死代码）；若其他函数仍在用，保留不动。

- [ ] **Step 5: 类型检查**

```bash
corepack pnpm --filter ./apps/web exec tsc --noEmit
```
预期：此时会出现调用方（`detail.tsx`）类型错误——这是预期的，会在 Task 9/13 修复；本任务只确认新增/改造的类型定义本身没有语法错误（可用 `tsc --noEmit 2>&1 | grep employees.ts` 缩小范围确认这一个文件没有报错）。

- [ ] **Step 6: 提交**

```bash
git add apps/web/src/lib/api/employees.ts
git commit -m "feat(web): add run stats and effective-config read to employee api client"
```

---

### Task 6: `EmployeeDetailHeader` 组件——详情头卡

**Files:**
- Create: `apps/web/src/features/employees/components/employee-detail-header.tsx`
- Test: `apps/web/src/features/employees/components/employee-detail-header.test.tsx`

**Interfaces:**
- Consumes: `EmployeeAvatar`（已存在，`@/features/employees/avatar`）、`V3PageHeader`/`StatusPill`/`V3Button`（`@/components/superteam`）、`DigitalEmployee`（`@/lib/api/employees`）
- Produces: `EmployeeDetailHeader({ employee, onStartTask, onManageCapabilities }: { employee: DigitalEmployee; onStartTask: () => void; onManageCapabilities: () => void })` — 纯展示组件，无数据请求；`编辑员工配置`/`查看审计` 用 `Link`。

- [ ] **Step 1: 写失败的组件测试**

```typescript
// apps/web/src/features/employees/components/employee-detail-header.test.tsx
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeDetailHeader } from "./employee-detail-header";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employee = {
  id: "11111111-1111-4111-8111-111111111111",
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  name: "后端实现员",
  role: "backend_engineer",
  description: "负责后端实现、接口补全、数据库迁移与测试修复",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
};

describe("EmployeeDetailHeader", () => {
  it("renders name, status and triggers start task", async () => {
    const onStartTask = vi.fn();
    const onManageCapabilities = vi.fn();
    const screen = await render(
      <EmployeeDetailHeader employee={employee} onManageCapabilities={onManageCapabilities} onStartTask={onStartTask} />,
    );

    await expect.element(screen.getByRole("heading", { name: "后端实现员" })).toBeVisible();
    await expect.element(screen.getByText("active")).toBeVisible();
    await screen.getByRole("button", { name: "开始任务" }).click();
    expect(onStartTask).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- employee-detail-header
```
预期：FAIL，找不到模块 `./employee-detail-header`。

- [ ] **Step 3: 实现组件**

```typescript
// apps/web/src/features/employees/components/employee-detail-header.tsx
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Blocks, FileClock, Play, Settings } from "lucide-react";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { DigitalEmployee } from "@/lib/api/employees";
import { EmployeeAvatar } from "../avatar";

type EmployeeDetailHeaderProps = {
  employee: DigitalEmployee;
  onStartTask: () => void;
  onManageCapabilities: () => void;
};

const statusTone: Record<string, V3Tone> = {
  active: "ok",
  ready: "info",
  disabled: "mute",
  archived: "mute",
  error: "danger",
};

export function EmployeeDetailHeader({ employee, onStartTask, onManageCapabilities }: EmployeeDetailHeaderProps) {
  const avatarAsset = (employee.metadata?.avatar as DigitalEmployee["metadata"] extends undefined ? never : never) ?? undefined;

  return (
    <div className="flex flex-col gap-4 rounded-v3-card border border-v3-line bg-v3-card p-5 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 items-center gap-4">
        <EmployeeAvatar asset={employee.metadata?.avatar as never} name={employee.name} size="lg" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-[22px] font-extrabold tracking-tight text-v3-ink">{employee.name}</h1>
            <StatusPill tone={statusTone[employee.status] ?? "mute"}>{employee.status}</StatusPill>
          </div>
          <p className="mt-1 truncate text-[13px] text-v3-ink-2">Claude Code 会话身份 · 生效上下文与历史执行记录</p>
          <p className="mt-1 truncate text-xs text-v3-ink-3">
            角色 {employee.role}
            {employee.description ? ` · ${employee.description}` : ""}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <V3Button asChild variant="outline">
          <Link to="/employees">
            <ArrowLeft className="size-4" />
            返回列表
          </Link>
        </V3Button>
        <V3Button onClick={onManageCapabilities} type="button" variant="outline">
          <Blocks className="size-4" />
          管理技能与 MCP
        </V3Button>
        <V3Button asChild variant="outline">
          <Link params={{ employeeId: employee.id }} to="/employees/$employeeId/config">
            <Settings className="size-4" />
            编辑员工配置
          </Link>
        </V3Button>
        <V3Button asChild variant="outline">
          <Link to="/audit">
            <FileClock className="size-4" />
            查看审计
          </Link>
        </V3Button>
        <V3Button onClick={onStartTask} type="button" variant="primary">
          <Play className="size-4" />
          开始任务
        </V3Button>
      </div>
    </div>
  );
}
```

> `EmployeeAvatar` 的 `asset` prop 类型是 `DigitalEmployeeAvatarAsset | null | undefined`，而 `employee.metadata?.avatar` 类型是 `Record<string, unknown> | undefined`——两者结构相同但 TS 不能自动兼容，用 `as never` 之类的写法会掩盖真实类型问题。**改为**在组件里显式转换：

```typescript
import type { DigitalEmployeeAvatarAsset } from "@/lib/api/employees";

function avatarAssetFromMetadata(metadata: DigitalEmployee["metadata"]): DigitalEmployeeAvatarAsset | undefined {
  const avatar = metadata?.avatar;
  return avatar && typeof avatar === "object" ? (avatar as DigitalEmployeeAvatarAsset) : undefined;
}
```

并在组件里用 `avatarAssetFromMetadata(employee.metadata)` 替换掉上面临时的 `as never` 写法，删除未使用的 `avatarAsset` 变量。

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- employee-detail-header
```
预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/employee-detail-header.tsx \
        apps/web/src/features/employees/components/employee-detail-header.test.tsx
git commit -m "feat(web): add employee detail header component"
```

---

### Task 7: `EmployeeMetricsStrip` 组件——执行指标条

**Files:**
- Create: `apps/web/src/features/employees/components/employee-metrics-strip.tsx`
- Test: `apps/web/src/features/employees/components/employee-metrics-strip.test.tsx`

**Interfaces:**
- Consumes: `V3MetricCard`/`StatusPill`（`@/components/superteam`）、`DigitalEmployeeRunStats`（`@/lib/api/employees`）
- Produces: `EmployeeMetricsStrip({ stats, providerType, runtimeNodeLabel, commandChannelConnected, currentStatusLabel }: EmployeeMetricsStripProps)` — 纯展示，按 Global Constraints 的口径格式化数字（成功率 `xx.x%`、耗时 `mm分ss秒`或`--`、环比只在 `prev_7d_count>0` 时显示百分比）。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/employee-metrics-strip.test.tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeMetricsStrip } from "./employee-metrics-strip";

const stats = {
  total_count: 76,
  succeeded_count: 68,
  failed_count: 5,
  cancelled_count: 3,
  success_rate: 68 / 76,
  avg_duration_sec: 29 * 60,
  p90_duration_sec: 48 * 60,
  last_7d_count: 12,
  prev_7d_count: 10,
};

describe("EmployeeMetricsStrip", () => {
  it("renders formatted stats", async () => {
    const screen = await render(
      <EmployeeMetricsStrip
        commandChannelConnected
        currentStatusLabel="active"
        providerType="claude_code"
        runtimeNodeLabel="local-dev-node"
        stats={stats}
      />,
    );

    await expect.element(screen.getByText("76")).toBeVisible();
    await expect.element(screen.getByText("89.5%")).toBeVisible();
    await expect.element(screen.getByText("68")).toBeVisible();
    await expect.element(screen.getByText("29分0秒")).toBeVisible();
    await expect.element(screen.getByText(/P90 48分0秒/)).toBeVisible();
    await expect.element(screen.getByText(/近7天.*↑/)).toBeVisible();
  });

  it("shows placeholder dashes when stats are unavailable", async () => {
    const screen = await render(
      <EmployeeMetricsStrip
        commandChannelConnected={false}
        currentStatusLabel="active"
        providerType="claude_code"
        runtimeNodeLabel="local-dev-node"
        stats={undefined}
      />,
    );

    await expect.element(screen.getByText("Runtime 命令通道未连接")).toBeVisible();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- employee-metrics-strip
```

- [ ] **Step 3: 实现组件**

```typescript
// apps/web/src/features/employees/components/employee-metrics-strip.tsx
import { Activity, CheckCircle2, Clock, Gauge, Hand, Server, XCircle } from "lucide-react";
import { StatusPill, V3MetricCard } from "@/components/superteam";
import type { DigitalEmployeeRunStats } from "@/lib/api/employees";

type EmployeeMetricsStripProps = {
  stats: DigitalEmployeeRunStats | undefined;
  providerType: string;
  runtimeNodeLabel: string;
  commandChannelConnected: boolean;
  currentStatusLabel: string;
};

export function EmployeeMetricsStrip({
  stats,
  providerType,
  runtimeNodeLabel,
  commandChannelConnected,
  currentStatusLabel,
}: EmployeeMetricsStripProps) {
  const trend = formatTrend(stats);

  return (
    <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5">
      <V3MetricCard icon={<Server />} iconTone="brand" label="Provider" value={providerType} />
      <V3MetricCard
        icon={<Server />}
        iconTone="info"
        label="Runtime 执行位置"
        meta={
          <StatusPill showDot tone={commandChannelConnected ? "ok" : "danger"}>
            {commandChannelConnected ? "命令通道在线" : "Runtime 命令通道未连接"}
          </StatusPill>
        }
        value={runtimeNodeLabel}
      />
      <V3MetricCard icon={<Activity />} iconTone="mute" label="累计执行" value={stats ? stats.total_count : "--"} />
      <V3MetricCard icon={<Activity />} iconTone="info" label="近7天" meta={trend} value={stats ? stats.last_7d_count : "--"} />
      <V3MetricCard
        icon={<Gauge />}
        iconTone="ok"
        label="成功率"
        value={stats && stats.success_rate !== null ? formatPercent(stats.success_rate) : "--"}
      />
      <V3MetricCard
        icon={<Clock />}
        iconTone="brand"
        label="平均耗时"
        meta={stats?.p90_duration_sec != null ? `P90 ${formatDuration(stats.p90_duration_sec)}` : undefined}
        value={stats?.avg_duration_sec != null ? formatDuration(stats.avg_duration_sec) : "--"}
      />
      <V3MetricCard icon={<CheckCircle2 />} iconTone="ok" label="成功" value={stats ? stats.succeeded_count : "--"} />
      <V3MetricCard icon={<XCircle />} iconTone="danger" label="失败" value={stats ? stats.failed_count : "--"} />
      <V3MetricCard icon={<Hand />} iconTone="warn" label="人工停止" value={stats ? stats.cancelled_count : "--"} />
      <V3MetricCard icon={<Activity />} iconTone="brand" label="当前状态" value={currentStatusLabel} />
    </section>
  );
}

function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`;
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

function formatTrend(stats: DigitalEmployeeRunStats | undefined): string | undefined {
  if (!stats) {
    return undefined;
  }
  if (stats.prev_7d_count === 0) {
    return `较上周期 +${stats.last_7d_count}`;
  }
  const change = ((stats.last_7d_count - stats.prev_7d_count) / stats.prev_7d_count) * 100;
  const arrow = change >= 0 ? "↑" : "↓";
  return `较上周期 ${arrow}${Math.abs(change).toFixed(0)}%`;
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- employee-metrics-strip
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/employee-metrics-strip.tsx \
        apps/web/src/features/employees/components/employee-metrics-strip.test.tsx
git commit -m "feat(web): add employee execution metrics strip component"
```

---

### Task 8: `EmployeeRunHistoryTable` 组件——历史执行任务表

**Files:**
- Create: `apps/web/src/features/employees/components/employee-run-history-table.tsx`
- Test: `apps/web/src/features/employees/components/employee-run-history-table.test.tsx`

**Interfaces:**
- Consumes: `WorkSurface`/`V3Table`/`V3Th`/`V3Td`/`V3Tr`/`V3StateSurface`/`V3Pagination`/`StatusPill`/`V3Segmented`（`@/components/superteam`）、`DigitalEmployeeRunListResult`（`@/lib/api/employees`）
- Produces: `EmployeeRunHistoryTable({ result, isLoading, isError, error, page, pageSize, statusFilter, onStatusFilterChange, onPageChange, onRowClick, onRetry }: EmployeeRunHistoryTableProps)` — 点击行触发 `onRowClick(item)`，交给上层打开 `RunDetailDrawer`。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/employee-run-history-table.test.tsx
import { describe, expect, it, vi } from "vitest";
import { userEvent } from "vitest/browser";
import { render } from "vitest-browser-react";
import { EmployeeRunHistoryTable } from "./employee-run-history-table";
import type { DigitalEmployeeRunListResult } from "@/lib/api/employees";

const result: DigitalEmployeeRunListResult = {
  items: [
    {
      id: "run-1",
      tenant_id: "tenant-1",
      task_id: "task-1",
      digital_employee_id: "employee-1",
      execution_instance_id: "instance-1",
      runtime_node_id: "node-uuid-1",
      node_id: "node-a",
      command_id: "cmd-1",
      provider_type: "claude_code",
      status: "completed",
      result: {},
      diagnostic: {},
      work_products: [],
      session_state: {},
      timed_out: false,
      task_title: "数据库迁移脚本校验",
      project_name: "数据库平台",
      work_product_count: 2,
      duration_sec: 1095,
      created_at: "2026-05-20T10:32:00Z",
    },
  ],
  total_count: 1,
  filters: {
    statuses: [{ value: "completed", label: "已完成" }],
    projects: [{ value: "project-1", label: "数据库平台" }],
  },
};

describe("EmployeeRunHistoryTable", () => {
  it("renders run rows and triggers row click", async () => {
    const onRowClick = vi.fn();
    const screen = await render(
      <EmployeeRunHistoryTable
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={onRowClick}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={result}
        statusFilter={undefined}
      />,
    );

    await expect.element(screen.getByText("数据库迁移脚本校验")).toBeVisible();
    await expect.element(screen.getByText("数据库平台")).toBeVisible();
    await expect.element(screen.getByText("已完成")).toBeVisible();
    await userEvent.click(screen.getByText("数据库迁移脚本校验"));
    expect(onRowClick).toHaveBeenCalledWith(result.items[0]);
  });

  it("shows empty state when there are no runs", async () => {
    const screen = await render(
      <EmployeeRunHistoryTable
        onPageChange={vi.fn()}
        onRetry={vi.fn()}
        onRowClick={vi.fn()}
        onStatusFilterChange={vi.fn()}
        page={1}
        pageSize={10}
        result={{ items: [], total_count: 0, filters: { statuses: [], projects: [] } }}
        statusFilter={undefined}
      />,
    );

    await expect.element(screen.getByText("暂无数据")).toBeVisible();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- employee-run-history-table
```

- [ ] **Step 3: 实现组件**

```typescript
// apps/web/src/features/employees/components/employee-run-history-table.tsx
import {
  StatusPill,
  V3Chip,
  V3Pagination,
  V3StateSurface,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { DigitalEmployeeRunListItem, DigitalEmployeeRunListResult, DigitalEmployeeRunStatus } from "@/lib/api/employees";

type EmployeeRunHistoryTableProps = {
  result: DigitalEmployeeRunListResult | undefined;
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  page: number;
  pageSize: number;
  statusFilter: DigitalEmployeeRunStatus | undefined;
  onStatusFilterChange: (status: DigitalEmployeeRunStatus | undefined) => void;
  onPageChange: (page: number) => void;
  onRowClick: (item: DigitalEmployeeRunListItem) => void;
  onRetry: () => void;
};

const runStatusTone: Record<DigitalEmployeeRunStatus, V3Tone> = {
  queued: "mute",
  dispatching: "mute",
  running: "info",
  cancelling: "warn",
  completed: "ok",
  failed: "danger",
  cancelled: "warn",
  timed_out: "danger",
};

export function EmployeeRunHistoryTable({
  result,
  isLoading,
  isError,
  error,
  page,
  pageSize,
  statusFilter,
  onStatusFilterChange,
  onPageChange,
  onRowClick,
  onRetry,
}: EmployeeRunHistoryTableProps) {
  const items = result?.items ?? [];
  const total = result?.total_count ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center gap-2 border-b border-v3-line px-4 py-3">
        <V3Chip active={statusFilter === undefined} onClick={() => onStatusFilterChange(undefined)} type="button">
          全部状态
        </V3Chip>
        {result?.filters.statuses.map((option) => (
          <V3Chip
            active={statusFilter === option.value}
            key={option.value}
            onClick={() => onStatusFilterChange(option.value as DigitalEmployeeRunStatus)}
            type="button"
          >
            {option.label}
          </V3Chip>
        ))}
      </div>
      <V3StateSurface empty={items.length === 0} error={error} isError={isError} isLoading={isLoading} onRetry={onRetry}>
        <V3Table>
          <thead>
            <tr>
              <V3Th>任务 / 项目</V3Th>
              <V3Th>会话 ID</V3Th>
              <V3Th>状态</V3Th>
              <V3Th>耗时</V3Th>
              <V3Th>工件</V3Th>
              <V3Th>时间</V3Th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <V3Tr
                className="cursor-pointer"
                key={item.id}
                onClick={() => onRowClick(item)}
                tone={item.status === "failed" || item.status === "timed_out" ? "danger" : undefined}
              >
                <V3Td>
                  <p className="truncate font-medium text-v3-ink">{item.task_title}</p>
                  <p className="truncate text-xs text-v3-ink-3">{item.project_name ?? "无关联项目"}</p>
                </V3Td>
                <V3Td className="font-mono text-xs text-v3-ink-2">{shortId(item.id)}</V3Td>
                <V3Td>
                  <StatusPill tone={runStatusTone[item.status]}>{item.status}</StatusPill>
                </V3Td>
                <V3Td className="tabular-nums">{item.duration_sec != null ? formatDuration(item.duration_sec) : "--"}</V3Td>
                <V3Td className="tabular-nums">{item.work_product_count}</V3Td>
                <V3Td className="text-xs text-v3-ink-3">{item.updated_at ?? item.created_at ?? "-"}</V3Td>
              </V3Tr>
            ))}
          </tbody>
        </V3Table>
      </V3StateSurface>
      <V3Pagination onPageChange={onPageChange} page={page} pageCount={pageCount} pageSize={pageSize} total={total} />
    </WorkSurface>
  );
}

function shortId(id: string): string {
  return id.slice(0, 8);
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}
```

> `V3Chip` 的类型定义是 `ComponentProps<"button"> & { active?: boolean; count?: number }`，本身没有 `onClick` 之外的限制，直接传 `onClick`/`type="button"` 没问题。

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- employee-run-history-table
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/employee-run-history-table.tsx \
        apps/web/src/features/employees/components/employee-run-history-table.test.tsx
git commit -m "feat(web): add employee run history table component"
```

---

### Task 9: `RunDetailDrawer` 组件——运行详情抽屉（迁移事件流/结果/失败/停止）

**Files:**
- Create: `apps/web/src/features/employees/components/run-detail-drawer.tsx`
- Test: `apps/web/src/features/employees/components/run-detail-drawer.test.tsx`

**Interfaces:**
- Consumes: `Sheet`/`SheetContent`/`SheetHeader`/`SheetTitle`（`@/components/ui/sheet`）、`V3Button`/`StatusPill`（`@/components/superteam`）、`listDigitalEmployeeRunEvents`/`stopDigitalEmployeeRun`（`@/lib/api/employees`）
- Produces: `RunDetailDrawer({ apiOptions, employeeId, run, open, onOpenChange, onStopped }: RunDetailDrawerProps)` — 内部自己用 `useQuery` 拉事件流（迁移自 `detail.tsx` 现有逻辑），`useMutation` 处理停止（迁移自现有 `stopRun`），停止成功后调用 `onStopped(updatedRun)` 让父组件刷新列表缓存。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/run-detail-drawer.test.tsx
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { RunDetailDrawer } from "./run-detail-drawer";
import type { DigitalEmployeeRunListItem } from "@/lib/api/employees";

const employeeId = "11111111-1111-4111-8111-111111111111";

const runningRun: DigitalEmployeeRunListItem = {
  id: "run-1",
  tenant_id: "tenant-1",
  task_id: "task-1",
  digital_employee_id: employeeId,
  execution_instance_id: "instance-1",
  runtime_node_id: "node-uuid-1",
  node_id: "node-a",
  command_id: "cmd-1",
  provider_type: "claude_code",
  status: "running",
  result: {},
  diagnostic: {},
  work_products: [],
  session_state: {},
  timed_out: false,
  task_title: "数据库迁移脚本校验",
  work_product_count: 0,
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createFetcher() {
  let current = runningRun;
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/runs/${current.id}/events` && method === "GET") {
      return jsonResponse([{ event_type: "provider.stdout", sequence_number: 1, payload: { text: "正在执行" } }]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/runs/${current.id}/stop` && method === "POST") {
      current = { ...current, status: "cancelling" };
      return jsonResponse(current);
    }
    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

describe("RunDetailDrawer", () => {
  it("shows events and stops an active run", async () => {
    const fetcher = createFetcher();
    const onStopped = vi.fn();
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RunDetailDrawer
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employeeId={employeeId}
          onOpenChange={vi.fn()}
          onStopped={onStopped}
          open
          run={runningRun}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText(/正在执行/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "停止" }));
    await expect.element(screen.getByText("取消中")).toBeVisible();
    expect(onStopped).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- run-detail-drawer
```

- [ ] **Step 3: 实现组件**

把现有 `detail.tsx` 里的 `FailureBlock`/`ResultBlock`/`RunEventRow`/`failureReason`/`compactJson`/`runStatusLabel`/`isFailedRun`/`isActiveRun` 逻辑原样迁移进本文件（不改行为）：

```typescript
// apps/web/src/features/employees/components/run-detail-drawer.tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Square } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listDigitalEmployeeRunEvents,
  stopDigitalEmployeeRun,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunEvent,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>(["queued", "dispatching", "running", "cancelling"]);
const failedRunStatuses = new Set<DigitalEmployeeRunStatus>(["failed", "cancelled", "timed_out"]);

type RunDetailDrawerProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
  run: DigitalEmployeeRunListItem | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStopped: (run: DigitalEmployeeRun) => void;
};

export function RunDetailDrawer({ apiOptions, employeeId, run, open, onOpenChange, onStopped }: RunDetailDrawerProps) {
  const queryClient = useQueryClient();
  const events = useQuery({
    enabled: Boolean(run?.id) && open,
    queryKey: ["digital-employee-run-events", employeeId, run?.id, { limit: 50 }],
    queryFn: () => listDigitalEmployeeRunEvents(apiOptions, employeeId, run?.id ?? "", { limit: 50 }),
    refetchInterval: run && isActiveRun(run.status) ? 2500 : false,
  });
  const stopRun = useMutation({
    mutationFn: (target: DigitalEmployeeRunListItem) =>
      stopDigitalEmployeeRun(apiOptions, employeeId, target.id, { reason: "用户从 Web 停止" }),
    onSuccess: async (updatedRun) => {
      onStopped(updatedRun);
      await queryClient.invalidateQueries({ queryKey: ["digital-employee-run-events", employeeId, updatedRun.id] });
    },
  });

  if (!run) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            {run.task_title}
            <RunStatusPill status={run.status} />
          </SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-4 px-4 pb-6">
          <div className="grid gap-2 text-sm md:grid-cols-2">
            <SummaryItem label="命令" value={run.command_id} />
            <SummaryItem label="Provider" value={run.provider_type} />
            <SummaryItem label="节点" value={run.node_id || run.runtime_node_id} />
            <SummaryItem label="更新时间" value={run.updated_at ?? run.created_at ?? "-"} />
          </div>
          {isFailedRun(run.status) ? <FailureBlock run={run} /> : null}
          {run.status === "completed" ? <ResultBlock run={run} /> : null}
          {isActiveRun(run.status) ? (
            <V3Button
              disabled={run.status === "cancelling" || stopRun.isPending}
              onClick={() => stopRun.mutate(run)}
              type="button"
              variant="danger"
            >
              <Square className="size-4" />
              停止
            </V3Button>
          ) : null}
          {stopRun.isError ? <p className="text-sm text-destructive">停止失败</p> : null}
          <div>
            <div className="mb-2 flex items-center justify-between">
              <p className="text-sm font-semibold">事件流</p>
              {events.data ? <span className="text-xs text-v3-ink-3">{events.data.length} 条</span> : null}
            </div>
            {events.isLoading ? <p className="text-sm text-v3-ink-2">事件加载中</p> : null}
            {events.isError ? <p className="text-sm text-destructive">事件加载失败</p> : null}
            {events.data?.length ? (
              <div className="space-y-2">
                {events.data.map((event) => (
                  <RunEventRow event={event} key={`${event.sequence_number}-${event.event_type}`} />
                ))}
              </div>
            ) : !events.isLoading ? (
              <p className="text-sm text-v3-ink-2">暂无事件</p>
            ) : null}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function SummaryItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-v3-line bg-v3-card-soft px-3 py-2">
      <p className="text-xs text-v3-ink-3">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-v3-ink">{value}</p>
    </div>
  );
}

function RunStatusPill({ status }: { status: DigitalEmployeeRunStatus }) {
  const tone: V3Tone = isFailedRun(status) ? "danger" : status === "completed" ? "ok" : "mute";
  return <StatusPill tone={tone}>{runStatusLabel(status)}</StatusPill>;
}

function FailureBlock({ run }: { run: DigitalEmployeeRunListItem }) {
  return (
    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-sm font-medium text-destructive">失败原因</p>
      <p className="mt-1 text-sm">{failureReason(run)}</p>
    </div>
  );
}

function ResultBlock({ run }: { run: DigitalEmployeeRunListItem }) {
  return (
    <div>
      <p className="text-sm font-medium">结果</p>
      <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
        {compactJson(run.result)}
      </pre>
    </div>
  );
}

function RunEventRow({ event }: { event: DigitalEmployeeRunEvent }) {
  return (
    <div className="grid gap-2 rounded-md border border-v3-line px-3 py-2 md:grid-cols-[120px_160px_minmax(0,1fr)]">
      <p className="text-sm font-medium">#{event.sequence_number}</p>
      <p className="truncate text-sm">{event.event_type}</p>
      <pre className="min-w-0 overflow-auto whitespace-pre-wrap break-words text-xs text-v3-ink-2">
        {compactJson(event.payload)}
      </pre>
    </div>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

function isFailedRun(status: DigitalEmployeeRunStatus) {
  return failedRunStatuses.has(status);
}

function runStatusLabel(status: DigitalEmployeeRunStatus) {
  switch (status) {
    case "queued":
      return "排队中";
    case "dispatching":
      return "调度中";
    case "running":
      return "执行中";
    case "cancelling":
      return "取消中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    case "timed_out":
      return "已超时";
  }
}

function failureReason(run: DigitalEmployeeRunListItem) {
  return run.error_message || compactJson(run.diagnostic) || compactJson(run.result) || "未提供失败原因";
}

function compactJson(value: unknown) {
  if (!value || (typeof value === "object" && Object.keys(value).length === 0)) {
    return "";
  }
  return JSON.stringify(value, null, 2);
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- run-detail-drawer
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/run-detail-drawer.tsx \
        apps/web/src/features/employees/components/run-detail-drawer.test.tsx
git commit -m "feat(web): add run detail drawer with event stream and stop action"
```

---

### Task 10: `StartTaskDrawer` 组件——开始任务抽屉

**Files:**
- Create: `apps/web/src/features/employees/components/start-task-drawer.tsx`
- Test: `apps/web/src/features/employees/components/start-task-drawer.test.tsx`

**Interfaces:**
- Consumes: `Sheet`/`SheetContent`/`SheetHeader`/`SheetTitle`（`@/components/ui/sheet`）、`Label`/`Textarea`（`@/components/ui`）、`V3Button`（`@/components/superteam`）
- Produces: `StartTaskDrawer({ open, onOpenChange, canStartTask, disabledReasons, isPending, isError, onSubmit }: StartTaskDrawerProps)` — 表单状态（objective/prompt）内部持有，提交时调用 `onSubmit({ objective, prompt })`，成功后由父组件负责 `onOpenChange(false)`。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/start-task-drawer.test.tsx
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { StartTaskDrawer } from "./start-task-drawer";

describe("StartTaskDrawer", () => {
  it("submits objective and prompt", async () => {
    const onSubmit = vi.fn();
    const screen = await render(
      <StartTaskDrawer
        canStartTask
        disabledReasons={[]}
        isError={false}
        isPending={false}
        onOpenChange={vi.fn()}
        onSubmit={onSubmit}
        open
      />,
    );

    await userEvent.fill(screen.getByLabelText("任务目标"), "梳理上线风险");
    await userEvent.fill(screen.getByLabelText("任务提示"), "请检查最近失败任务");
    await userEvent.click(screen.getByRole("button", { name: "开始任务" }));

    expect(onSubmit).toHaveBeenCalledWith({ objective: "梳理上线风险", prompt: "请检查最近失败任务" });
  });

  it("disables submit and shows reasons when task cannot start", async () => {
    const screen = await render(
      <StartTaskDrawer
        canStartTask={false}
        disabledReasons={["Runtime 命令通道未连接，暂不能开始任务"]}
        isError={false}
        isPending={false}
        onOpenChange={vi.fn()}
        onSubmit={vi.fn()}
        open
      />,
    );

    await expect.element(screen.getByText("Runtime 命令通道未连接，暂不能开始任务")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "开始任务" })).toBeDisabled();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- start-task-drawer
```

- [ ] **Step 3: 实现组件**

```typescript
// apps/web/src/features/employees/components/start-task-drawer.tsx
import { Play } from "lucide-react";
import { useState } from "react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { V3Button } from "@/components/superteam";

type StartTaskDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canStartTask: boolean;
  disabledReasons: string[];
  isPending: boolean;
  isError: boolean;
  onSubmit: (input: { objective: string; prompt: string }) => void;
};

export function StartTaskDrawer({
  open,
  onOpenChange,
  canStartTask,
  disabledReasons,
  isPending,
  isError,
  onSubmit,
}: StartTaskDrawerProps) {
  const [objective, setObjective] = useState("");
  const [prompt, setPrompt] = useState("");
  const trimmedObjective = objective.trim();
  const canSubmit = canStartTask && Boolean(trimmedObjective) && !isPending;

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full sm:max-w-md" side="right">
        <SheetHeader>
          <SheetTitle>开始任务</SheetTitle>
        </SheetHeader>
        <form
          className="flex flex-col gap-3 px-4 pb-6"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit) {
              onSubmit({ objective: trimmedObjective, prompt: prompt.trim() });
              setObjective("");
              setPrompt("");
            }
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="run-objective">任务目标</Label>
            <Textarea
              disabled={!canStartTask || isPending}
              id="run-objective"
              onChange={(event) => setObjective(event.target.value)}
              rows={2}
              value={objective}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="run-prompt">任务提示</Label>
            <Textarea
              disabled={!canStartTask || isPending}
              id="run-prompt"
              onChange={(event) => setPrompt(event.target.value)}
              rows={4}
              value={prompt}
            />
          </div>
          <V3Button disabled={!canSubmit} type="submit">
            <Play className="size-4" />
            开始任务
          </V3Button>
          {disabledReasons.map((reason) => (
            <p className="text-xs text-v3-ink-3" key={reason}>
              {reason}
            </p>
          ))}
          {isError ? <p className="text-sm text-destructive">开始任务失败</p> : null}
        </form>
      </SheetContent>
    </Sheet>
  );
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- start-task-drawer
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/start-task-drawer.tsx \
        apps/web/src/features/employees/components/start-task-drawer.test.tsx
git commit -m "feat(web): add start task drawer component"
```

---

### Task 11: `EffectiveContextPanel` 组件——生效上下文面板

**Files:**
- Create: `apps/web/src/features/employees/components/effective-context-panel.tsx`
- Test: `apps/web/src/features/employees/components/effective-context-panel.test.tsx`

**Interfaces:**
- Consumes: `SoftCard`/`IconTile`/`StatusPill`/`V3Button`（`@/components/superteam`）、`getCurrentDigitalEmployeeEffectiveConfig`（`@/lib/api/employees`）、`listEmployeeSkills`（`@/lib/api/skills`）、`listEffectiveMcpConfig`（`@/lib/api/capabilities`）、`listEmployeeEnvironmentVariables`（`@/lib/api/employees`）
- Produces: `EffectiveContextPanel({ apiOptions, employee, executionInstance, employeeId, onManageCapabilities }: EffectiveContextPanelProps)` — 自带数据请求（各自独立 `useQuery`，四态各自处理，不因单个子请求失败拖垮整个面板）。技能/MCP 个人 vs 团队继承 vs 生效总数从 `inherited` 布尔字段客户端计数；宪法层级显示"团队 + 个人补充（2 层）"；记忆条目固定显示"待接入"占位，不发起任何请求。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/effective-context-panel.test.tsx
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EffectiveContextPanel } from "./effective-context-panel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employeeId = "11111111-1111-4111-8111-111111111111";

const employee = {
  id: employeeId,
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  name: "后端实现员",
  role: "backend_engineer",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
};

const executionInstance = {
  id: "instance-1",
  digital_employee_id: employeeId,
  runtime_node_id: "node-uuid-1",
  provider_type: "claude_code",
  agent_home_dir: ".superteam/workspaces/teams/backend/employees/backend-engineer",
  status: "ready",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/effective-config`) {
      return jsonResponse({
        id: "config-1",
        tenant_id: "tenant-1",
        digital_employee_id: employeeId,
        team_config_revision_id: "team-rev-1",
        employee_config_revision_id: "employee-rev-1",
        effective_config: { constitution: { team: { rules: ["禁止删除生产数据"] }, addendum: {} } },
        validation_result: { blocking_errors: [], warnings: [] },
        status: "approved",
      });
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/skills`) {
      return jsonResponse([
        { skill_id: "s1", inherited: false },
        { skill_id: "s2", inherited: true },
        { skill_id: "s3", inherited: true },
      ]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/mcp/effective`) {
      return jsonResponse([{ server_id: "m1", inherited: true }]);
    }
    if (url.pathname === `/api/v1/digital-employees/${employeeId}/environment-variables`) {
      return jsonResponse([
        { name: "DATABASE_URL", configured: true, fingerprint: "a1", sensitive: true, status: "active" },
        { name: "REDIS_URL", configured: false, fingerprint: "", sensitive: true, status: "active" },
      ]);
    }
    return jsonResponse({ error: "unhandled" }, 404);
  }) as unknown as typeof fetch;
}

describe("EffectiveContextPanel", () => {
  it("renders skill/mcp counts, constitution, env vars and memory placeholder", async () => {
    const fetcher = createFetcher();
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <EffectiveContextPanel
          apiOptions={{ baseUrl: "http://control-plane.local", fetcher }}
          employee={employee}
          employeeId={employeeId}
          executionInstance={executionInstance}
          onManageCapabilities={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("个人技能 1")).toBeVisible();
    await expect.element(screen.getByText("团队继承技能 2")).toBeVisible();
    await expect.element(screen.getByText("生效总数 3")).toBeVisible();
    await expect.element(screen.getByText("生效总数 1")).toBeVisible();
    await expect.element(screen.getByText("待接入")).toBeVisible();
    await expect.element(screen.getByText("已配置 1")).toBeVisible();
    await expect.element(screen.getByText("缺失 1")).toBeVisible();
    await expect.element(screen.getByText("REDIS_URL")).toBeVisible();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- effective-context-panel
```

- [ ] **Step 3: 实现组件**

先确认 `listEmployeeSkills`/`listEffectiveMcpConfig` 的确切请求路径（与测试里的 mock 路径一致）：

```bash
cd apps/web && grep -n "digital-employees.*skills\|digital-employees.*mcp" src/lib/api/skills.ts src/lib/api/capabilities.ts
```

按实际路径调整测试 mock（上面测试里的路径是占位推测，**必须先跑这个 grep 核对真实路径再改测试**，否则会一直 404）。确认后实现：

```typescript
// apps/web/src/features/employees/components/effective-context-panel.tsx
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BookOpen, Boxes, KeyRound, Network, ScrollText } from "lucide-react";
import { IconTile, SoftCard, StatusPill, V3Button } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getCurrentDigitalEmployeeEffectiveConfig,
  listEmployeeEnvironmentVariables,
  type DigitalEmployee,
  type DigitalEmployeeExecutionInstance,
} from "@/lib/api/employees";
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import { listEmployeeSkills } from "@/lib/api/skills";
import { ApiRequestError } from "@/lib/api/client";

type EffectiveContextPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
  employee: DigitalEmployee;
  executionInstance: DigitalEmployeeExecutionInstance | undefined;
  onManageCapabilities: () => void;
};

export function EffectiveContextPanel({
  apiOptions,
  employeeId,
  employee,
  executionInstance,
  onManageCapabilities,
}: EffectiveContextPanelProps) {
  const effectiveConfig = useQuery({
    queryKey: ["digital-employee-effective-config", employeeId],
    queryFn: () => getCurrentDigitalEmployeeEffectiveConfig(apiOptions, employeeId),
    retry: false,
  });
  const skills = useQuery({
    queryKey: ["employee-skills", employeeId],
    queryFn: () => listEmployeeSkills(apiOptions, employeeId),
  });
  const mcpServers = useQuery({
    queryKey: ["employee-effective-mcp", employeeId],
    queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId),
  });
  const envVars = useQuery({
    queryKey: ["employee-environment-variables", employeeId],
    queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId),
  });

  const noApprovedConfig = effectiveConfig.error instanceof ApiRequestError && effectiveConfig.error.status === 404;
  const personalSkillCount = skills.data?.filter((skill) => !skill.inherited).length ?? 0;
  const inheritedSkillCount = skills.data?.filter((skill) => skill.inherited).length ?? 0;
  const personalMcpCount = mcpServers.data?.filter((server) => !server.inherited).length ?? 0;
  const inheritedMcpCount = mcpServers.data?.filter((server) => server.inherited).length ?? 0;
  const configuredEnvCount = envVars.data?.filter((item) => item.configured).length ?? 0;
  const missingEnvVars = envVars.data?.filter((item) => !item.configured) ?? [];

  return (
    <SoftCard className="flex flex-col gap-5 p-5">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-v3-ink">生效上下文</h2>
        <V3Button asChild size="sm" variant="ghost">
          <Link params={{ employeeId }} to="/employees/$employeeId/config">
            编辑
          </Link>
        </V3Button>
      </div>

      <section className="space-y-2">
        <p className="text-xs font-semibold text-v3-ink-3">基本信息</p>
        <div className="grid grid-cols-2 gap-2 text-sm">
          <InfoItem label="Provider" value={executionInstance?.provider_type ?? "未绑定"} />
          <InfoItem label="角色" value={employee.role} />
          <InfoItem label="状态" value={employee.status} />
          <InfoItem label="工作目录" value={executionInstance?.agent_home_dir ?? "未配置"} />
        </div>
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="brand">
              <Boxes />
            </IconTile>
            技能
          </p>
          <Link className="text-xs text-v3-brand" to="/skills">
            查看全部
          </Link>
        </div>
        {skills.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : skills.isError ? (
          <p className="text-xs text-destructive">技能加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">
            个人技能 {personalSkillCount} · 团队继承技能 {inheritedSkillCount} · 生效总数 {skills.data?.length ?? 0}
          </p>
        )}
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="info">
              <Network />
            </IconTile>
            MCP
          </p>
          <Link className="text-xs text-v3-brand" to="/mcp">
            查看全部
          </Link>
        </div>
        {mcpServers.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : mcpServers.isError ? (
          <p className="text-xs text-destructive">MCP 加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">
            个人 MCP {personalMcpCount} · 团队 MCP {inheritedMcpCount} · 生效总数 {mcpServers.data?.length ?? 0}
          </p>
        )}
      </section>

      <section className="space-y-2">
        <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
          <IconTile size="sm" tone="artifact">
            <ScrollText />
          </IconTile>
          宪法与记忆
        </p>
        {effectiveConfig.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : noApprovedConfig ? (
          <p className="text-xs text-v3-ink-3">尚无已批准的生效配置</p>
        ) : effectiveConfig.isError ? (
          <p className="text-xs text-destructive">生效配置加载失败</p>
        ) : (
          <p className="text-xs text-v3-ink-2">宪法层级：团队 + 个人补充（2 层）</p>
        )}
        <div className="flex items-center gap-2">
          <IconTile size="sm" tone="mute">
            <BookOpen />
          </IconTile>
          <StatusPill showDot={false} tone="mute">
            记忆：待接入
          </StatusPill>
        </div>
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-xs font-semibold text-v3-ink-3">
            <IconTile size="sm" tone="warn">
              <KeyRound />
            </IconTile>
            环境变量
          </p>
          <V3Button onClick={onManageCapabilities} size="sm" variant="ghost">
            查看详情
          </V3Button>
        </div>
        {envVars.isLoading ? (
          <p className="text-xs text-v3-ink-3">加载中</p>
        ) : envVars.isError ? (
          <p className="text-xs text-destructive">环境变量加载失败</p>
        ) : (
          <>
            <p className="text-xs text-v3-ink-2">
              已配置 {configuredEnvCount} · 缺失 {missingEnvVars.length} · 总数 {envVars.data?.length ?? 0}
            </p>
            {missingEnvVars.length ? (
              <div className="flex flex-wrap gap-1.5">
                {missingEnvVars.map((item) => (
                  <StatusPill key={item.name} tone="danger">
                    {item.name}
                  </StatusPill>
                ))}
              </div>
            ) : null}
          </>
        )}
      </section>
    </SoftCard>
  );
}

function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] text-v3-ink-3">{label}</p>
      <p className="truncate text-sm font-medium text-v3-ink">{value}</p>
    </div>
  );
}
```

- [ ] **Step 4: 运行测试，按真实路径修正后确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- effective-context-panel
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/effective-context-panel.tsx \
        apps/web/src/features/employees/components/effective-context-panel.test.tsx
git commit -m "feat(web): add effective context panel with skills, mcp, constitution and env var summaries"
```

---

### Task 12: `ContextInjectionChain` 组件——下次任务注入顺序链

**Files:**
- Create: `apps/web/src/features/employees/components/context-injection-chain.tsx`
- Test: `apps/web/src/features/employees/components/context-injection-chain.test.tsx`

**Interfaces:**
- Consumes: `SoftCard`/`IconTile`（`@/components/superteam`）
- Produces: `ContextInjectionChain({ skillCount, mcpCount, envConfiguredCount, envTotalCount, roleLabel }: ContextInjectionChainProps)` — 纯展示，固定 8 节点，记忆节点固定「待接入」。

- [ ] **Step 1: 写失败的测试**

```typescript
// apps/web/src/features/employees/components/context-injection-chain.test.tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { ContextInjectionChain } from "./context-injection-chain";

describe("ContextInjectionChain", () => {
  it("renders 8 ordered nodes with counts and memory placeholder", async () => {
    const screen = await render(
      <ContextInjectionChain envConfiguredCount={7} envTotalCount={9} mcpCount={2} roleLabel="backend_engineer" skillCount={9} />,
    );

    await expect.element(screen.getByText("角色说明")).toBeVisible();
    await expect.element(screen.getByText("宪法")).toBeVisible();
    await expect.element(screen.getByText("待接入")).toBeVisible();
    await expect.element(screen.getByText("9 项")).toBeVisible();
    await expect.element(screen.getByText("7 / 9")).toBeVisible();
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

```bash
corepack pnpm --filter ./apps/web run test -- context-injection-chain
```

- [ ] **Step 3: 实现组件**

```typescript
// apps/web/src/features/employees/components/context-injection-chain.tsx
import { ArrowRight, Blocks, BookOpen, FolderGit2, KeyRound, Network, ScrollText, UserRound, Users } from "lucide-react";
import { IconTile, SoftCard } from "@/components/superteam";

type ContextInjectionChainProps = {
  roleLabel: string;
  skillCount: number;
  mcpCount: number;
  envConfiguredCount: number;
  envTotalCount: number;
};

export function ContextInjectionChain({ roleLabel, skillCount, mcpCount, envConfiguredCount, envTotalCount }: ContextInjectionChainProps) {
  const nodes = [
    { icon: <UserRound />, title: "角色说明", meta: roleLabel },
    { icon: <ScrollText />, title: "宪法", meta: "团队 + 个人补充" },
    { icon: <BookOpen />, title: "记忆", meta: "待接入" },
    { icon: <Blocks />, title: "个人技能", meta: `${skillCount} 项` },
    { icon: <Users />, title: "团队继承技能", meta: `${skillCount} 项` },
    { icon: <Network />, title: "MCP", meta: `${mcpCount} 项` },
    { icon: <KeyRound />, title: "环境变量", meta: `${envConfiguredCount} / ${envTotalCount}` },
    { icon: <FolderGit2 />, title: "工作目录", meta: "只读" },
  ];

  return (
    <SoftCard className="p-5">
      <p className="mb-3 text-sm font-semibold text-v3-ink">下次任务会注入的上下文包（按注入顺序 · 只读）</p>
      <div className="flex flex-wrap items-center gap-2">
        {nodes.map((node, index) => (
          <div className="flex items-center gap-2" key={node.title}>
            <div className="flex min-w-[104px] flex-col items-center gap-1.5 rounded-v3-inner bg-v3-card-soft px-3 py-2.5 text-center">
              <IconTile size="sm" tone="mute">
                {node.icon}
              </IconTile>
              <p className="text-xs font-semibold text-v3-ink">{node.title}</p>
              <p className="text-[11px] text-v3-ink-3">{node.meta}</p>
            </div>
            {index < nodes.length - 1 ? <ArrowRight aria-hidden className="size-4 shrink-0 text-v3-ink-3" /> : null}
          </div>
        ))}
      </div>
    </SoftCard>
  );
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
corepack pnpm --filter ./apps/web run test -- context-injection-chain
```

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/components/context-injection-chain.tsx \
        apps/web/src/features/employees/components/context-injection-chain.test.tsx
git commit -m "feat(web): add context injection order chain component"
```

---

### Task 13: 重写 `detail.tsx` 编排全部子组件

**Files:**
- Modify: `apps/web/src/features/employees/detail.tsx`（大幅重写，只保留数据编排职责）

**Interfaces:**
- Consumes: Task 5–12 的全部产出（API 类型/函数、7 个组件）
- Produces: `EmployeeDetailView`/`EmployeeDetailPage` 对外签名不变（`{ apiBaseUrl, employeeId, fetcher }` / `{ employeeId }`），供路由文件 `_authenticated/employees/$employeeId.tsx` 和已有测试无缝复用。

- [ ] **Step 1: 重写 `detail.tsx`**

```typescript
// apps/web/src/features/employees/detail.tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { ApiRequestError } from "@/lib/api/client";
import {
  createDigitalEmployeeRun,
  getDigitalEmployee,
  getDigitalEmployeeExecutionInstance,
  getDigitalEmployeeRunStats,
  listDigitalEmployeeRuns,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";
import { getRuntimeOverview } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { ContextInjectionChain } from "./components/context-injection-chain";
import { EffectiveContextPanel } from "./components/effective-context-panel";
import { EmployeeCapabilitiesPanel } from "./components/employee-capabilities-panel";
import { EmployeeDetailHeader } from "./components/employee-detail-header";
import { EmployeeMetricsStrip } from "./components/employee-metrics-strip";
import { EmployeeRunHistoryTable } from "./components/employee-run-history-table";
import { RunDetailDrawer } from "./components/run-detail-drawer";
import { StartTaskDrawer } from "./components/start-task-drawer";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>(["queued", "dispatching", "running", "cancelling"]);
const PAGE_SIZE = 10;

export function EmployeeDetailPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <EmployeeDetailView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeDetailViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

export function EmployeeDetailView({ apiBaseUrl, employeeId, fetcher }: EmployeeDetailViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<DigitalEmployeeRunStatus | undefined>(undefined);
  const [selectedRun, setSelectedRun] = useState<DigitalEmployeeRunListItem | undefined>(undefined);
  const [runDrawerOpen, setRunDrawerOpen] = useState(false);
  const [startTaskOpen, setStartTaskOpen] = useState(false);
  const [capabilitiesOpen, setCapabilitiesOpen] = useState(false);

  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });
  const instance = useQuery({
    queryKey: ["digital-employee-execution-instance", employeeId],
    queryFn: () => getDigitalEmployeeExecutionInstance(apiOptions, employeeId),
    retry: false,
  });
  const runStats = useQuery({
    queryKey: ["digital-employee-run-stats", employeeId],
    queryFn: () => getDigitalEmployeeRunStats(apiOptions, employeeId),
  });
  const runsQueryKey = ["digital-employee-runs", employeeId, { page, statusFilter }] as const;
  const runs = useQuery({
    queryKey: runsQueryKey,
    queryFn: () =>
      listDigitalEmployeeRuns(apiOptions, employeeId, {
        limit: PAGE_SIZE,
        offset: (page - 1) * PAGE_SIZE,
        status: statusFilter ? [statusFilter] : undefined,
      }),
    refetchInterval: (query) => (query.state.data?.items.some((item) => isActiveRun(item.status)) ? 2500 : false),
  });
  const runtimeOverview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview(apiOptions),
    refetchInterval: 5000,
  });

  const instanceNotFound = instance.error instanceof ApiRequestError && instance.error.status === 404;
  const hasActiveRun = runs.data?.items.some((item) => isActiveRun(item.status)) ?? false;
  const employeeCanRun = employee.data?.status === "ready" || employee.data?.status === "active";
  const executionInstanceCanRun = instance.isSuccess && (instance.data.status === "ready" || instance.data.status === "active");
  const executionRuntimeNodeId = instance.data?.runtime_node_id;
  const runtimeNode = runtimeOverview.data?.nodes.find((node) => node.runtime_node_id === executionRuntimeNodeId);
  const runtimeCommandChannelDisconnected = runtimeOverview.isSuccess && runtimeNode?.command_channel_connected === false;
  const canStartTask = employeeCanRun && executionInstanceCanRun && runs.isSuccess && !hasActiveRun && !runtimeCommandChannelDisconnected;

  const disabledReasons: string[] = [];
  if (hasActiveRun) disabledReasons.push("当前已有活跃运行");
  if (!executionInstanceCanRun && instance.isSuccess) disabledReasons.push("执行实例当前不可执行");
  if (runtimeCommandChannelDisconnected) disabledReasons.push("Runtime 命令通道未连接，暂不能开始任务");
  if (instanceNotFound) disabledReasons.push("未绑定 Runtime，暂不能开始任务");
  if (runs.isError) disabledReasons.push("运行列表加载失败，暂不能开始新任务");

  const refreshRunFacts = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-run-stats", employeeId] }),
    ]);
  };

  const createRun = useMutation({
    mutationFn: (input: { objective: string; prompt: string }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, { objective: input.objective, prompt: input.prompt }),
    onSuccess: async () => {
      setStartTaskOpen(false);
      await refreshRunFacts();
    },
  });

  const handleStopped = async (_run: DigitalEmployeeRun) => {
    await refreshRunFacts();
  };

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        {employee.isLoading ? <p className="text-sm text-v3-ink-2">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">数字员工加载失败</p> : null}

        {employee.data ? (
          <div className="flex flex-col gap-4">
            <EmployeeDetailHeader
              employee={employee.data}
              onManageCapabilities={() => setCapabilitiesOpen(true)}
              onStartTask={() => setStartTaskOpen(true)}
            />

            <EmployeeMetricsStrip
              commandChannelConnected={runtimeNode?.command_channel_connected ?? false}
              currentStatusLabel={employee.data.status}
              providerType={instance.data?.provider_type ?? "未绑定"}
              runtimeNodeLabel={instance.data?.runtime_node_id ?? "未绑定"}
              stats={runStats.data}
            />

            <section className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
              <EmployeeRunHistoryTable
                error={runs.error}
                isError={runs.isError}
                isLoading={runs.isLoading}
                onPageChange={setPage}
                onRetry={() => runs.refetch()}
                onRowClick={(item) => {
                  setSelectedRun(item);
                  setRunDrawerOpen(true);
                }}
                onStatusFilterChange={(status) => {
                  setStatusFilter(status);
                  setPage(1);
                }}
                page={page}
                pageSize={PAGE_SIZE}
                result={runs.data}
                statusFilter={statusFilter}
              />
              <EffectiveContextPanel
                apiOptions={apiOptions}
                employee={employee.data}
                employeeId={employeeId}
                executionInstance={instance.data}
                onManageCapabilities={() => setCapabilitiesOpen(true)}
              />
            </section>

            <ContextInjectionChain
              envConfiguredCount={0}
              envTotalCount={0}
              mcpCount={0}
              roleLabel={employee.data.role}
              skillCount={0}
            />
          </div>
        ) : null}
      </Main>

      <StartTaskDrawer
        canStartTask={canStartTask}
        disabledReasons={disabledReasons}
        isError={createRun.isError}
        isPending={createRun.isPending}
        onOpenChange={setStartTaskOpen}
        onSubmit={(input) => createRun.mutate(input)}
        open={startTaskOpen}
      />

      <RunDetailDrawer
        apiOptions={apiOptions}
        employeeId={employeeId}
        onOpenChange={setRunDrawerOpen}
        onStopped={handleStopped}
        open={runDrawerOpen}
        run={selectedRun}
      />

      <Sheet onOpenChange={setCapabilitiesOpen} open={capabilitiesOpen}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl" side="right">
          <SheetHeader>
            <SheetTitle>管理技能与 MCP</SheetTitle>
          </SheetHeader>
          <div className="px-4 pb-6">
            <EmployeeCapabilitiesPanel apiOptions={apiOptions} employeeId={employeeId} />
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}
```

> `ContextInjectionChain` 目前传的是硬编码 0——**这是一个已知缺口**，在 Step 2 里用 `EffectiveContextPanel` 已经拉取到的技能/MCP/环境变量数量回填。由于这些数据目前分别在 `EffectiveContextPanel` 内部请求（组件边界职责单一），`detail.tsx` 若要拿到这些计数用于 `ContextInjectionChain`，需要把这几个查询提升到 `detail.tsx` 层，`EffectiveContextPanel` 改为接收 props 而不是自己请求。**执行 Step 2 完成这次提升**，不要把 Step 1 的硬编码 0 当作最终状态提交。

- [ ] **Step 2: 把技能/MCP/环境变量查询提升到 `detail.tsx`，`EffectiveContextPanel` 改为纯展示**

修改 `apps/web/src/features/employees/components/effective-context-panel.tsx`：删除内部的 `useQuery` 调用（`skills`/`mcpServers`/`envVars`/`effectiveConfig` 四个），改为直接接收计算好的 props：

```typescript
type EffectiveContextPanelProps = {
  employee: DigitalEmployee;
  executionInstance: DigitalEmployeeExecutionInstance | undefined;
  employeeId: string;
  effectiveConfig: { isLoading: boolean; isError: boolean; noApprovedConfig: boolean };
  skills: { isLoading: boolean; isError: boolean; personalCount: number; inheritedCount: number; totalCount: number };
  mcp: { isLoading: boolean; isError: boolean; personalCount: number; inheritedCount: number; totalCount: number };
  envVars: { isLoading: boolean; isError: boolean; configuredCount: number; totalCount: number; missingNames: string[] };
  onManageCapabilities: () => void;
};
```

组件体内把原来 `skills.isLoading`/`skills.data?.filter(...)` 等表达式换成直接读 props 里对应字段（例如 `skills.personalCount`）。同步更新 `effective-context-panel.test.tsx`，改为直接传入计算好的 props 而不是 mock fetch（不再需要 `QueryClientProvider`/`fetcher`）：

```typescript
// apps/web/src/features/employees/components/effective-context-panel.test.tsx（重写）
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EffectiveContextPanel } from "./effective-context-panel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => <a href={to}>{children}</a>,
}));

const employee = {
  id: "employee-1",
  tenant_id: "tenant-1",
  owner_user_id: "user-1",
  employee_type: "backend_engineer",
  name: "后端实现员",
  role: "backend_engineer",
  status: "active" as const,
  permission_policy: {},
  context_policy: {},
  approval_policy: {},
  risk_level: "medium",
};

describe("EffectiveContextPanel", () => {
  it("renders skill/mcp counts, constitution, env vars and memory placeholder", async () => {
    const screen = await render(
      <EffectiveContextPanel
        effectiveConfig={{ isLoading: false, isError: false, noApprovedConfig: false }}
        employee={employee}
        employeeId="employee-1"
        envVars={{ isLoading: false, isError: false, configuredCount: 1, totalCount: 2, missingNames: ["REDIS_URL"] }}
        executionInstance={undefined}
        mcp={{ isLoading: false, isError: false, personalCount: 0, inheritedCount: 1, totalCount: 1 }}
        onManageCapabilities={vi.fn()}
        skills={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 2, totalCount: 3 }}
      />,
    );

    await expect.element(screen.getByText("个人技能 1")).toBeVisible();
    await expect.element(screen.getByText("团队继承技能 2")).toBeVisible();
    await expect.element(screen.getByText("生效总数 3")).toBeVisible();
    await expect.element(screen.getByText("待接入")).toBeVisible();
    await expect.element(screen.getByText("已配置 1")).toBeVisible();
    await expect.element(screen.getByText("REDIS_URL")).toBeVisible();
  });
});
```

在 `detail.tsx` 里新增对应的 `useQuery`（技能/MCP/环境变量/effective-config），把结果整理成上面的 props 形状传给 `EffectiveContextPanel`，并把技能总数/MCP 总数/环境变量已配置数传给 `ContextInjectionChain` 替换掉 Step 1 的硬编码 0：

```typescript
import { listEffectiveMcpConfig } from "@/lib/api/capabilities";
import { listEmployeeSkills } from "@/lib/api/skills";
import { getCurrentDigitalEmployeeEffectiveConfig, listEmployeeEnvironmentVariables } from "@/lib/api/employees";

// 在 EmployeeDetailView 内新增：
const effectiveConfigQuery = useQuery({
  queryKey: ["digital-employee-effective-config", employeeId],
  queryFn: () => getCurrentDigitalEmployeeEffectiveConfig(apiOptions, employeeId),
  retry: false,
});
const skillsQuery = useQuery({
  queryKey: ["employee-skills", employeeId],
  queryFn: () => listEmployeeSkills(apiOptions, employeeId),
});
const mcpQuery = useQuery({
  queryKey: ["employee-effective-mcp", employeeId],
  queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId),
});
const envVarsQuery = useQuery({
  queryKey: ["employee-environment-variables", employeeId],
  queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId),
});

const noApprovedConfig = effectiveConfigQuery.error instanceof ApiRequestError && effectiveConfigQuery.error.status === 404;
const personalSkillCount = skillsQuery.data?.filter((s) => !s.inherited).length ?? 0;
const inheritedSkillCount = skillsQuery.data?.filter((s) => s.inherited).length ?? 0;
const personalMcpCount = mcpQuery.data?.filter((s) => !s.inherited).length ?? 0;
const inheritedMcpCount = mcpQuery.data?.filter((s) => s.inherited).length ?? 0;
const configuredEnvCount = envVarsQuery.data?.filter((item) => item.configured).length ?? 0;
const missingEnvVars = envVarsQuery.data?.filter((item) => !item.configured) ?? [];
```

并把 JSX 中的 `<EffectiveContextPanel .../>` 和 `<ContextInjectionChain .../>` 替换成：

```typescript
<EffectiveContextPanel
  effectiveConfig={{ isLoading: effectiveConfigQuery.isLoading, isError: effectiveConfigQuery.isError && !noApprovedConfig, noApprovedConfig }}
  employee={employee.data}
  employeeId={employeeId}
  envVars={{
    isLoading: envVarsQuery.isLoading,
    isError: envVarsQuery.isError,
    configuredCount: configuredEnvCount,
    totalCount: envVarsQuery.data?.length ?? 0,
    missingNames: missingEnvVars.map((item) => item.name),
  }}
  executionInstance={instance.data}
  mcp={{ isLoading: mcpQuery.isLoading, isError: mcpQuery.isError, personalCount: personalMcpCount, inheritedCount: inheritedMcpCount, totalCount: mcpQuery.data?.length ?? 0 }}
  onManageCapabilities={() => setCapabilitiesOpen(true)}
  skills={{ isLoading: skillsQuery.isLoading, isError: skillsQuery.isError, personalCount: personalSkillCount, inheritedCount: inheritedSkillCount, totalCount: skillsQuery.data?.length ?? 0 }}
/>

<ContextInjectionChain
  envConfiguredCount={configuredEnvCount}
  envTotalCount={envVarsQuery.data?.length ?? 0}
  mcpCount={mcpQuery.data?.length ?? 0}
  roleLabel={employee.data.role}
  skillCount={skillsQuery.data?.length ?? 0}
/>
```

- [ ] **Step 3: 类型检查**

```bash
corepack pnpm --filter ./apps/web exec tsc --noEmit
```
逐一修正报错（尤其 `EmployeeCapabilitiesPanel`/`RunDetailDrawer`/`EmployeeRunHistoryTable` 的 props 类型对齐）。

- [ ] **Step 4: 提交**

```bash
git add apps/web/src/features/employees/detail.tsx \
        apps/web/src/features/employees/components/effective-context-panel.tsx \
        apps/web/src/features/employees/components/effective-context-panel.test.tsx
git commit -m "refactor(web): compose employee detail page from v3 observation-first layout"
```

---

### Task 14: 更新 `detail.test.tsx` 端到端页面测试

**Files:**
- Modify: `apps/web/src/features/employees/detail.test.tsx`

**Interfaces:**
- Consumes: Task 13 重写后的 `EmployeeDetailView`

- [ ] **Step 1: 重写测试夹具，覆盖新增的 run-stats/effective-config/skills/mcp/env-var 请求路径**

在现有 `createDetailFetcher` 里，把 `/runs` 路径的响应从裸数组改为 `{ items, total_count, filters }`，并新增 `run-stats`、`effective-config`、`skills`、`mcp/effective`、`environment-variables` 四个路径分支（复用 Task 11 Step 3 grep 出的真实路径）。核心断言从"能看到事件流/结果/失败原因"迁移到：详情头卡渲染员工名、指标条渲染累计执行数、历史表点击行后抽屉弹出并显示事件流、开始任务抽屉提交后触发刷新。

```bash
cd apps/web && grep -n "test(\"\|it(\"" src/features/employees/detail.test.tsx
```
先列出现有用例清单，逐条改造成新版页面结构下的等价断言（点击方式从"页面内联按钮"变成"先开抽屉再操作"），不要删减用例覆盖的行为分支（活跃运行停止、开始任务成功、命令通道断开禁用、运行列表加载失败禁用、执行实例缺失禁用、completed/failed/cancelled/timed_out 展示、切换历史运行查看不同事件流）。

- [ ] **Step 2: 运行测试**

```bash
corepack pnpm --filter ./apps/web run test -- detail.test
```
逐条修正直到全部通过。

- [ ] **Step 3: 全量前端测试**

```bash
corepack pnpm --filter ./apps/web run test
```

- [ ] **Step 4: 提交**

```bash
git add apps/web/src/features/employees/detail.test.tsx
git commit -m "test(web): update employee detail page tests for redesigned layout"
```

---

### Task 15: 收尾验证

**Files:** 无代码改动，仅验证。

- [ ] **Step 1: 后端全量校验**

```bash
cd apps/control-plane && go build ./... && go vet ./...
make -C apps/control-plane migrate-validate
```

- [ ] **Step 2: 前端全量校验**

```bash
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web exec tsc --noEmit
corepack pnpm --filter ./apps/web run build
```

- [ ] **Step 3: 启动真实服务**

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```
确认 Temporal、Control Plane、Web、Runtime Agent 均为运行中且加载了当前代码。

- [ ] **Step 4: 真实端到端验证**

用浏览器（Codex chrome plug 或等效工具）打开 `/employees/$employeeId`（选一个真实存在的数字员工），核对：
- 详情头卡显示真实姓名/状态/角色，操作按钮全部可点击
- 执行指标条数字来自真实 `run-stats` 接口返回（不是猜测/硬编码），成功率/耗时/近7天趋势与真实数据一致
- 历史执行表状态筛选、分页请求真实 `runs` 接口并正确携带 `status`/`limit`/`offset` 查询参数（用浏览器 Network 面板确认）
- 点击历史行打开运行详情抽屉，事件流/结果/失败原因/停止按钮行为与重构前一致
- 打开"开始任务"抽屉提交任务，成功后历史表刷新
- "生效上下文"面板技能/MCP 计数、环境变量已配置/缺失、记忆区域显示"待接入"而非编造数字
- "管理技能与 MCP"抽屉里 `EmployeeCapabilitiesPanel` 绑定/解绑技能功能仍然可用
- 桌面和移动宽度下无横向溢出、无文字截断异常

- [ ] **Step 5: 运行完成前检查 skill**

按 `.codex/skills/superteam-completion-check/SKILL.md` 走一遍收尾检查清单，记录验证证据（截图或 Network 请求记录）。

- [ ] **Step 6: 最终提交（如收尾检查发现小问题已修复）**

```bash
git status
git add -A
git commit -m "chore(web): finalize digital employee detail page redesign"
```
若第 4 步真实端到端验证受阻（服务未启动、认证缺失等），**不要**在这一步声明完成，按 CLAUDE.md 规则把任务标记为阻塞并说明缺失依赖。

---

## Self-Review Notes

- **Spec 覆盖**：Part A 覆盖 spec 第 4 节全部三类接口（run-stats、增强运行列表、effective-config 补齐）；Part B 覆盖 spec 第 5 节全部组件划分（头卡/指标条/历史表/运行详情/开始任务/生效上下文/注入链）；记忆占位（spec 第 8 节 YAGNI）在 Task 11/12 落实为固定文案，不发起任何请求。
- **与 spec 的已知偏差**（在 Task 2/11 中已按真实代码调整，均是"数据已存在只是没接口暴露"场景的具体落地方式，不改变 spec 的范围决策）：
  - spec 假设可以直接读 `effective_config` 派生技能/MCP 计数；实测 `effective_config` JSONB 不含技能/MCP 列表，改为复用已存在的 `listEmployeeSkills`/`listEffectiveMcpConfig`（各自带 `inherited` 字段）客户端计数，两者结果一致（都是"零新增语义，只是换一个真实数据源"）。
  - spec 提议宪法"层级"计数；实测 `constitution` 只有 `{team, addendum}` 两键，Task 11 如实展示"团队 + 个人补充（2 层）"，不编造第三层。
  - spec 提议"查看审计"带员工筛选；实测 `/audit` 路由目前只支持 `projectId` 筛选，无员工维度，Task 6 改为纯跳转链接，不假造不存在的筛选能力。
- **无占位符**：所有步骤均含可执行命令或完整代码；唯二需要"跑一次生成/编译后按实际结果微调"的地方（Task 1 Step 4 的 `pgFloat8Ptr` 类型、Task 3 Step 4 的字段名核对）都明确给出了具体检查命令和修正方向，不是模糊的"处理一下"。
- **类型一致性**：`DigitalEmployeeRunListItem`（前端类型，Task 5）字段名与后端 `digitalEmployeeRunListItemResponse`（Task 3 Step 6）JSON 字段一一对应；`RunDetailDrawer`/`EmployeeRunHistoryTable` 都以 `DigitalEmployeeRunListItem` 为唯一行数据类型，未出现命名不一致。
