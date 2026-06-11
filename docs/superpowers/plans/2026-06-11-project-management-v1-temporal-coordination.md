# 项目管理 V1 Temporal 协调 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 V0 项目事实源之上接入 Temporal 虚拟协调线程，让需求、路由决策、项目任务、执行回写、转派请求和人类决策形成可审计的运行闭环。

**Architecture:** V1 保持 Project 作为业务事实入口：项目服务负责写事实、事件和信号桥接，Temporal Workflow 只串行编排项目内协调决策，数据库读写、规则规划、审批创建和任务分派全部放在 Activity 中。Control Plane 使用小接口隔离 Temporal client、审批服务和项目 repository，前端继续复用 V0 项目运行详情与配置治理页面，只增量展示 Workflow、RouteDecision、DecisionRequest、TransferRequest 与执行摘要。

**Tech Stack:** Go + chi/net/http + pgx/sqlc + Atlas migrations + Temporal Go SDK + testify；React + TanStack Query + TanStack Router + shadcn/ui + lucide-react + Vitest browser；PostgreSQL + Redis + S3 兼容存储。

---

## 参考规格

- 主规格：`docs/superpowers/specs/2026-06-10-project-management-v1-temporal-coordination-design.md`
- V0 基线计划：`docs/superpowers/plans/2026-06-11-project-management-v0-foundation.md`
- 数据库规范：`DATABASE_DESIGN.md`
- 前端设计规范：`DESIGN.md`
- 项目协调设计参考：`docs/design/projectManager/temporal-project-coordination-design.md`

## 当前 Worktree 与分支策略

计划执行前复查到当前工作树位于 `codex/project-management-v1-temporal-coordination` 分支，HEAD 为 `56522da`，从 `main` 的 `2da5012` 分出。`main` 历史中已经包含项目管理 V0 的后端、前端、契约与修复提交，`codex/project-management-v0-foundation` 指向其中一个 V0 祖先提交。当前唯一未提交改动是 `apps/control-plane/internal/storage/migrations_test.go` 中的 V1 迁移失败测试；该改动属于 Task 1 的 RED 阶段，不能回滚。

执行顺序：

1. 执行时先用 `git status --short --branch`、`git log --oneline --decorate -8` 和 `git branch --contains 0e595fb` 确认当前 HEAD 包含 V0。
2. 如果当前分支已经是 `codex/project-management-v1-temporal-coordination`，继续在该分支执行，不再创建同名分支。
3. 如果后续在其他机器上执行且发现 V0 尚未合入当前基线，应先把 `codex/project-management-v0-foundation` 合并或变基进 V1 分支，再执行 Task 1；不要在缺少 V0 project 模块的基线上硬写 V1。
4. V1 每个任务独立提交。不要回滚 V0 文件；如果遇到 V0 未提交文件，只能在确认属于 V0 收尾后先提交 V0 checkpoint，再继续 V1。

## 范围边界

包含：

- 最小全局 `approval` 核心表、repository、service，用作项目人类决策事实源。
- V1 项目协调表：`project_coordination_jobs`、`project_route_decisions`、`project_execution_summaries`、`project_transfer_requests`、`project_decision_requests`。
- Temporal project coordinator Workflow、signals、activities、worker 注册和信号客户端。
- `SubmitDemand`、项目配置保存、成员替换、执行完成、执行失败、转派请求、人类决策处理到 Workflow signal 的桥接。
- RouteDecision 结构化校验，任务分派只能选择项目数字员工池内 active executor。
- 前端展示规划中状态、RouteDecision、真实项目任务状态、人类决策队列、TransferRequest、最近 Workflow 事件和 signal 状态。

不包含：

- 复杂流程图编辑器。
- 跨项目资源排班。
- 自动学习路由策略。
- 高级证据评分。
- 成本核算和预算预测。
- 项目最终验收归档。

## 文件结构

### Control Plane 存储层

- 新建：`apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql`
  - 新增 approval 核心表与 project coordination V1 表，所有表 UUID-first、tenant-first、中文注释、应用层关系校验优先。
- 修改：`apps/control-plane/internal/storage/migrations/atlas.sum`
  - 通过 Atlas 或现有 migration 校验流程更新。
- 修改：`apps/control-plane/internal/storage/migrations_test.go`
  - 增加 V1 表、索引、中文注释、无封闭 DB enum、无跨模块重 FK 的 schema 守卫。
- 新建：`apps/control-plane/internal/storage/queries/approval.sql`
  - `approval_requests` 与 `approval_decisions` 的 sqlc 查询。
- 修改：`apps/control-plane/internal/storage/queries/project.sql`
  - 增加 coordination job、route decision、execution summary、transfer request、decision projection、project task status 更新查询。
- 重新生成：`apps/control-plane/internal/storage/queries/*.sql.go`
  - 由 `sqlc generate` 生成。

### Control Plane 审批模块

- 修改：`apps/control-plane/internal/approval/service.go`
  - 从空壳升级为最小审批核心。
- 新建：`apps/control-plane/internal/approval/types.go`
  - `ApprovalRequest`、`ApprovalDecision`、status、decision 枚举字符串和请求 DTO。
- 新建：`apps/control-plane/internal/approval/repository.go`
  - repository 接口。
- 新建：`apps/control-plane/internal/approval/pg_repository.go`
  - sqlc-backed repository。
- 修改：`apps/control-plane/internal/approval/service_test.go`
  - 覆盖创建审批、处理审批、重复处理拒绝、租户隔离。
- 新建：`apps/control-plane/internal/approval/pg_repository_test.go`
  - 覆盖 mapper 和 JSON payload。

### Control Plane 项目模块

- 修改：`apps/control-plane/internal/project/types.go`
  - 增加 V1 类型、事件类型、状态字符串、signal payload、API request/response DTO。
- 修改：`apps/control-plane/internal/project/repository.go`
  - 增加 V1 repository 方法。
- 修改：`apps/control-plane/internal/project/pg_repository.go`
  - 实现 V1 持久化、mapper、事务边界。
- 修改：`apps/control-plane/internal/project/service.go`
  - 接入 `CoordinatorSignalClient`、`ApprovalCreator`、V1 writeback 和 decision resolve。
- 新建：`apps/control-plane/internal/project/coordination_signal.go`
  - 定义项目服务依赖的 Temporal 信号接口、noop/fake 可替换边界。
- 修改：`apps/control-plane/internal/project/service_test.go`
  - 覆盖 SubmitDemand signal、成员池约束、writeback signal、转派请求、人类决策处理。
- 修改：`apps/control-plane/internal/project/handler.go`
  - 增加 V1 Console API 和 Runtime writeback request body。
- 修改：`apps/control-plane/internal/project/handler_test.go`
  - 覆盖新 handler 行为。
- 修改：`apps/control-plane/internal/api/server.go`
  - 增加 V1 project routes 与 runtime session writeback routes。
- 修改：`apps/control-plane/internal/api/project_routes_test.go`
  - 覆盖 console auth、runtime session auth、route ordering、tenant scoping。

### Control Plane Workflow 模块

- 修改：`apps/control-plane/go.mod`
  - 添加 Temporal Go SDK。
- 修改：`apps/control-plane/internal/config/config.go`
  - 增加 Temporal 配置。
- 修改：`apps/control-plane/config/config.example.yaml`
  - 增加 Temporal 示例配置。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/types.go`
  - Workflow signal、activity input/output、structured planner 类型。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/workflow.go`
  - ProjectCoordinator Workflow 串行 signal loop。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/activities.go`
  - Activity 接口、Activity struct、业务调用边界。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/planner.go`
  - 第一版 deterministic structured planner 与校验函数。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/client.go`
  - Temporal signal/start client，实现 project 包 `CoordinatorSignalClient`。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/worker.go`
  - worker 注册函数。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
  - Temporal workflow test suite 覆盖主要 signals 与串行处理。
- 新建：`apps/control-plane/internal/workflow/projectcoordination/planner_test.go`
  - 覆盖 RouteDecision 结构化校验与员工池约束。
- 修改：`apps/control-plane/internal/app/app.go`
  - wiring Temporal client、workflow activities、approval service、project service。

### 契约

- 修改：`contracts/control-plane/openapi.yaml`
  - 补充 V1 API paths、schemas、request/response examples。
- 修改：`scripts/verify-foundation-contracts.mjs`
  - 如果脚本已枚举 project routes，则加入 V1 route guard；如果脚本按 OpenAPI 全量检查，则无需新增逻辑。

### Web 前端

- 修改：`apps/web/src/lib/api/projects.ts`
  - 增加 V1 API 类型和 client functions。
- 修改：`apps/web/src/lib/api/projects.test.ts`
  - 覆盖 V1 endpoints 路径、方法、body、cookie credentials。
- 修改：`apps/web/src/features/projects/index.tsx`
  - 增加 V1 queries/mutations，保留旧数据刷新。
- 修改：`apps/web/src/features/projects/components/project-operational-detail.tsx`
  - 展示规划中、RouteDecision、真实任务状态、决策队列、转派请求和 Workflow 状态。
- 修改：`apps/web/src/features/projects/components/project-config-page.tsx`
  - 配置或成员保存后展示影响 Workflow 的提示，并刷新 coordination job/event。
- 修改：`apps/web/src/features/projects/index.test.tsx`
  - 覆盖运行态新增区域、需求提交后的 planning 状态、人类决策处理、转派提示、执行完成状态。
- 修改：`apps/web/src/features/projects/config.test.tsx`
  - 覆盖配置/成员保存后的 Workflow 影响提示。

### 文档

- 修改：`CHANGELOG.md`
  - V1 开发完成时追加 Asia/Shanghai 时间戳。

## Task 0: Confirm V0 Baseline And V1 Branch

**Files:**
- Read: `AGENTS.md`
- Read: `CHANGELOG.md`
- Read: `apps/control-plane/internal/project/*`
- Read: `apps/web/src/features/projects/*`
- Modify: git branch state only when the V1 branch does not already exist

- [ ] **Step 1: Inspect current branch and dirty state**

Run:

```bash
git status --short --branch
git diff --name-only
git log --oneline --decorate -8
git branch --contains 0e595fb
```

Expected:

- Current branch is `codex/project-management-v1-temporal-coordination`, or another explicitly chosen V1 feature branch.
- `git branch --contains 0e595fb` includes the current branch, proving the V0 project management page stabilization commit is in the V1 baseline.
- Dirty files are either this plan file or Task 1 RED-stage files such as `apps/control-plane/internal/storage/migrations_test.go`.

- [ ] **Step 2: Run focused V0 verification before branching**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -run 'Project|Migration|Migrations' -count=1
pnpm --filter @superteam/web test -- src/features/projects src/lib/api/projects.test.ts src/routes/_authenticated/projects/-project-route.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: project/api/web V0 checks exit 0. If `internal/storage` fails only because `TestProjectManagementV1TemporalCoordinationMigration` cannot find migration `014_project_management_v1_temporal_coordination.sql`, record that as the expected Task 1 RED failure and continue.

- [ ] **Step 3: Create or reuse the stacked V1 branch**

Run:

```bash
current_branch="$(git branch --show-current)"
if [ "$current_branch" = "codex/project-management-v1-temporal-coordination" ]; then
  git status --short --branch
else
  git switch -c codex/project-management-v1-temporal-coordination
  git status --short --branch
fi
```

Expected: branch is `codex/project-management-v1-temporal-coordination`; worktree is clean except this plan file and/or Task 1 RED-stage migration test.

- [ ] **Step 4: Commit V0 checkpoint only if V0 source files are unexpectedly dirty**

If Step 1 shows dirty V0 source files unrelated to the V1 migration test, run:

```bash
git add AGENTS.md CHANGELOG.md apps/control-plane/internal/project apps/control-plane/internal/api apps/control-plane/internal/app apps/control-plane/internal/storage apps/web/src/features/projects apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/routes/_authenticated/projects docs/design/projectManager docs/prototypes docs/superpowers/plans/2026-06-11-project-management-v0-foundation.md
git commit -m "feat: add project management v0 foundation"
```

Expected: commit succeeds only when actual V0 files were dirty. If dirty state contains only `apps/control-plane/internal/storage/migrations_test.go` with V1 assertions, skip this step because that file belongs to Task 1.

## Task 1: Add V1 Database Tables And sqlc Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Create: `apps/control-plane/internal/storage/queries/approval.sql`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.sql.go`

- [x] **Step 1: Write the failing migration test**

Append this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestProjectManagementV1TemporalCoordinationMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/014_project_management_v1_temporal_coordination.sql")
	if err != nil {
		t.Fatalf("read project management v1 migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE approval_requests",
		"CREATE TABLE approval_decisions",
		"CREATE TABLE project_coordination_jobs",
		"CREATE TABLE project_route_decisions",
		"candidate_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"selected_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE TABLE project_execution_summaries",
		"CREATE TABLE project_transfer_requests",
		"CREATE TABLE project_decision_requests",
		"approval_request_id UUID NOT NULL",
		"CREATE INDEX idx_project_route_decisions_tenant_project_created",
		"CREATE INDEX idx_project_decision_requests_tenant_project_status",
		"COMMENT ON TABLE approval_requests IS",
		"COMMENT ON COLUMN project_decision_requests.approval_request_id IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected v1 migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE approval_status",
		"CREATE TYPE project_coordination_job_status",
		"BIGSERIAL PRIMARY KEY",
		"REFERENCES digital_employees",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("v1 migration must not contain %q", forbidden)
		}
	}
}
```

- [x] **Step 2: Run the migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectManagementV1TemporalCoordinationMigration -count=1
```

Expected: FAIL because `014_project_management_v1_temporal_coordination.sql` does not exist yet.

- [x] **Step 3: Create the V1 migration**

Create `apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql` with:

```sql
-- 014_project_management_v1_temporal_coordination.sql
-- 项目管理 V1：Temporal 协调、路由决策、执行回写、转派和人类决策投影

CREATE TABLE approval_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    requester_type VARCHAR(50) NOT NULL,
    requester_id UUID,
    target_user_id UUID NOT NULL,
    decision_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    risk_level VARCHAR(50),
    status VARCHAR(50) NOT NULL,
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    context_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

COMMENT ON TABLE approval_requests IS '全局审批请求事实表，保存人类决策请求的事实源';
COMMENT ON COLUMN approval_requests.id IS '审批请求ID';
COMMENT ON COLUMN approval_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN approval_requests.resource_type IS '审批关联资源类型，由应用层校验';
COMMENT ON COLUMN approval_requests.resource_id IS '审批关联资源ID';
COMMENT ON COLUMN approval_requests.requester_type IS '发起者类型，例如 project_coordinator、human_user 或 system';
COMMENT ON COLUMN approval_requests.requester_id IS '发起者ID，可为空表示系统发起';
COMMENT ON COLUMN approval_requests.target_user_id IS '目标处理人用户ID';
COMMENT ON COLUMN approval_requests.decision_type IS '决策类型，由应用层注册和校验';
COMMENT ON COLUMN approval_requests.title IS '审批标题快照';
COMMENT ON COLUMN approval_requests.summary IS '审批摘要快照';
COMMENT ON COLUMN approval_requests.risk_level IS '风险等级快照';
COMMENT ON COLUMN approval_requests.status IS '审批状态：pending、approved、rejected、needs_more_evidence、cancelled';
COMMENT ON COLUMN approval_requests.options IS '可选决策项 JSON 数组';
COMMENT ON COLUMN approval_requests.context_payload IS '审批上下文快照';
COMMENT ON COLUMN approval_requests.created_at IS '审批创建时间';
COMMENT ON COLUMN approval_requests.updated_at IS '审批更新时间';
COMMENT ON COLUMN approval_requests.resolved_at IS '审批处理完成时间';

CREATE INDEX idx_approval_requests_tenant_status_created ON approval_requests(tenant_id, status, created_at DESC);
CREATE INDEX idx_approval_requests_tenant_resource ON approval_requests(tenant_id, resource_type, resource_id);
CREATE TRIGGER update_approval_requests_updated_at BEFORE UPDATE ON approval_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE approval_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    approval_request_id UUID NOT NULL,
    decided_by_user_id UUID NOT NULL,
    decision VARCHAR(100) NOT NULL,
    comment TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE approval_decisions IS '全局审批处理记录表，保存人类处理动作和意见';
COMMENT ON COLUMN approval_decisions.id IS '审批处理记录ID';
COMMENT ON COLUMN approval_decisions.tenant_id IS '租户ID';
COMMENT ON COLUMN approval_decisions.approval_request_id IS '关联审批请求ID';
COMMENT ON COLUMN approval_decisions.decided_by_user_id IS '处理审批的人类用户ID';
COMMENT ON COLUMN approval_decisions.decision IS '处理结论：approved、rejected、needs_more_evidence';
COMMENT ON COLUMN approval_decisions.comment IS '处理意见';
COMMENT ON COLUMN approval_decisions.payload IS '处理时提交的结构化补充信息';
COMMENT ON COLUMN approval_decisions.created_at IS '处理记录创建时间';

CREATE INDEX idx_approval_decisions_tenant_request_created ON approval_decisions(tenant_id, approval_request_id, created_at DESC);

CREATE TABLE project_coordination_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    workflow_id VARCHAR(255) NOT NULL,
    trigger_event_id UUID,
    job_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    input_snapshot_ref JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_coordination_jobs IS '项目协调作业记录，追踪一次 Workflow 协调决策的输入、状态和输出事件';
COMMENT ON COLUMN project_coordination_jobs.id IS '协调作业ID';
COMMENT ON COLUMN project_coordination_jobs.tenant_id IS '租户ID';
COMMENT ON COLUMN project_coordination_jobs.project_id IS '所属项目ID';
COMMENT ON COLUMN project_coordination_jobs.workflow_id IS 'Temporal Workflow ID';
COMMENT ON COLUMN project_coordination_jobs.trigger_event_id IS '触发该作业的项目事件ID';
COMMENT ON COLUMN project_coordination_jobs.job_type IS '协调作业类型，例如 demand_route、transfer_review、human_decision';
COMMENT ON COLUMN project_coordination_jobs.status IS '协调作业状态：running、completed、failed、noop';
COMMENT ON COLUMN project_coordination_jobs.input_snapshot_ref IS '输入快照引用或小型快照 JSON';
COMMENT ON COLUMN project_coordination_jobs.output_event_ids IS '该作业产生的项目事件ID列表';
COMMENT ON COLUMN project_coordination_jobs.started_at IS '协调作业开始时间';
COMMENT ON COLUMN project_coordination_jobs.finished_at IS '协调作业结束时间';
COMMENT ON COLUMN project_coordination_jobs.created_at IS '协调作业创建时间';

CREATE INDEX idx_project_coordination_jobs_tenant_project_created ON project_coordination_jobs(tenant_id, project_id, created_at DESC);

CREATE TABLE project_route_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID NOT NULL,
    demand_id UUID,
    candidate_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    selected_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    reason TEXT NOT NULL,
    input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb,
    budget_estimate JSONB NOT NULL DEFAULT '{}'::jsonb,
    requires_human_review BOOLEAN NOT NULL DEFAULT false,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_route_decisions IS '项目需求路由决策表，保存虚拟协调线程选择执行员工和输出契约的结构化结论';
COMMENT ON COLUMN project_route_decisions.id IS '路由决策ID';
COMMENT ON COLUMN project_route_decisions.tenant_id IS '租户ID';
COMMENT ON COLUMN project_route_decisions.project_id IS '所属项目ID';
COMMENT ON COLUMN project_route_decisions.coordination_job_id IS '产生该决策的协调作业ID';
COMMENT ON COLUMN project_route_decisions.demand_id IS '关联需求ID';
COMMENT ON COLUMN project_route_decisions.candidate_digital_employee_ids IS '候选数字员工ID数组';
COMMENT ON COLUMN project_route_decisions.selected_digital_employee_ids IS '选中的数字员工ID数组';
COMMENT ON COLUMN project_route_decisions.reason IS '路由理由';
COMMENT ON COLUMN project_route_decisions.input_requirements IS '任务输入要求';
COMMENT ON COLUMN project_route_decisions.expected_outputs IS '期望输出契约数组';
COMMENT ON COLUMN project_route_decisions.budget_estimate IS '预算估算快照';
COMMENT ON COLUMN project_route_decisions.requires_human_review IS '是否需要人类先审核该决策';
COMMENT ON COLUMN project_route_decisions.created_event_id IS '创建该决策时产生的项目事件ID';
COMMENT ON COLUMN project_route_decisions.created_at IS '路由决策创建时间';

CREATE INDEX idx_project_route_decisions_tenant_project_created ON project_route_decisions(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_route_decisions_tenant_demand ON project_route_decisions(tenant_id, demand_id);

CREATE TABLE project_execution_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    conclusion TEXT NOT NULL,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence_factors JSONB NOT NULL DEFAULT '{}'::jsonb,
    uncertainty TEXT,
    missing_information JSONB NOT NULL DEFAULT '[]'::jsonb,
    recommended_next_action TEXT,
    requires_human_review BOOLEAN NOT NULL DEFAULT false,
    transfer_request_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_execution_summaries IS '项目任务执行摘要表，保存数字员工回写的结论、证据、工件和不确定性';
COMMENT ON COLUMN project_execution_summaries.id IS '执行摘要ID';
COMMENT ON COLUMN project_execution_summaries.tenant_id IS '租户ID';
COMMENT ON COLUMN project_execution_summaries.project_id IS '所属项目ID';
COMMENT ON COLUMN project_execution_summaries.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_execution_summaries.digital_employee_id IS '回写摘要的数字员工ID';
COMMENT ON COLUMN project_execution_summaries.conclusion IS '执行结论';
COMMENT ON COLUMN project_execution_summaries.evidence_refs IS '证据引用数组';
COMMENT ON COLUMN project_execution_summaries.artifact_refs IS '工件引用数组';
COMMENT ON COLUMN project_execution_summaries.confidence_factors IS '置信度因素快照';
COMMENT ON COLUMN project_execution_summaries.uncertainty IS '不确定性说明';
COMMENT ON COLUMN project_execution_summaries.missing_information IS '缺失信息数组';
COMMENT ON COLUMN project_execution_summaries.recommended_next_action IS '建议下一步动作';
COMMENT ON COLUMN project_execution_summaries.requires_human_review IS '是否需要人类复核';
COMMENT ON COLUMN project_execution_summaries.transfer_request_id IS '关联转派请求ID';
COMMENT ON COLUMN project_execution_summaries.created_event_id IS '创建该摘要时产生的项目事件ID';
COMMENT ON COLUMN project_execution_summaries.created_at IS '执行摘要创建时间';

CREATE INDEX idx_project_execution_summaries_tenant_project_created ON project_execution_summaries(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_execution_summaries_tenant_task ON project_execution_summaries(tenant_id, project_task_id);

CREATE TABLE project_transfer_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    requested_by_digital_employee_id UUID NOT NULL,
    reason TEXT NOT NULL,
    suggested_employee_type VARCHAR(100),
    suggested_digital_employee_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    missing_context_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_transfer_requests IS '项目任务转派请求表，保存数字员工发起的结构化转派事实';
COMMENT ON COLUMN project_transfer_requests.id IS '转派请求ID';
COMMENT ON COLUMN project_transfer_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN project_transfer_requests.project_id IS '所属项目ID';
COMMENT ON COLUMN project_transfer_requests.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_transfer_requests.requested_by_digital_employee_id IS '发起转派请求的数字员工ID';
COMMENT ON COLUMN project_transfer_requests.reason IS '转派理由';
COMMENT ON COLUMN project_transfer_requests.suggested_employee_type IS '建议员工类型';
COMMENT ON COLUMN project_transfer_requests.suggested_digital_employee_ids IS '建议数字员工ID数组';
COMMENT ON COLUMN project_transfer_requests.missing_context_refs IS '缺失上下文引用数组';
COMMENT ON COLUMN project_transfer_requests.status IS '转派请求状态：requested、accepted、rejected、cancelled';
COMMENT ON COLUMN project_transfer_requests.created_event_id IS '创建该请求时产生的项目事件ID';
COMMENT ON COLUMN project_transfer_requests.created_at IS '转派请求创建时间';
COMMENT ON COLUMN project_transfer_requests.updated_at IS '转派请求更新时间';

CREATE INDEX idx_project_transfer_requests_tenant_project_status ON project_transfer_requests(tenant_id, project_id, status, created_at DESC);
CREATE INDEX idx_project_transfer_requests_tenant_task ON project_transfer_requests(tenant_id, project_task_id);
CREATE TRIGGER update_project_transfer_requests_updated_at BEFORE UPDATE ON project_transfer_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE project_decision_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    approval_request_id UUID NOT NULL,
    coordination_job_id UUID,
    project_task_id UUID,
    target_user_id UUID NOT NULL,
    decision_type VARCHAR(100) NOT NULL,
    title_snapshot VARCHAR(255) NOT NULL,
    summary_snapshot TEXT,
    risk_level_snapshot VARCHAR(50),
    status_snapshot VARCHAR(50) NOT NULL,
    created_event_id UUID,
    resolved_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

COMMENT ON TABLE project_decision_requests IS '项目侧人类决策查询投影，审批事实源归 approval_requests 与 approval_decisions';
COMMENT ON COLUMN project_decision_requests.id IS '项目决策请求投影ID';
COMMENT ON COLUMN project_decision_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN project_decision_requests.project_id IS '所属项目ID';
COMMENT ON COLUMN project_decision_requests.approval_request_id IS '全局审批请求ID，审批事实源引用';
COMMENT ON COLUMN project_decision_requests.coordination_job_id IS '关联协调作业ID';
COMMENT ON COLUMN project_decision_requests.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_decision_requests.target_user_id IS '目标处理人用户ID';
COMMENT ON COLUMN project_decision_requests.decision_type IS '决策类型';
COMMENT ON COLUMN project_decision_requests.title_snapshot IS '决策标题快照';
COMMENT ON COLUMN project_decision_requests.summary_snapshot IS '决策摘要快照';
COMMENT ON COLUMN project_decision_requests.risk_level_snapshot IS '风险等级快照';
COMMENT ON COLUMN project_decision_requests.status_snapshot IS '审批状态快照';
COMMENT ON COLUMN project_decision_requests.created_event_id IS '创建该投影时产生的项目事件ID';
COMMENT ON COLUMN project_decision_requests.resolved_event_id IS '处理该投影时产生的项目事件ID';
COMMENT ON COLUMN project_decision_requests.created_at IS '投影创建时间';
COMMENT ON COLUMN project_decision_requests.updated_at IS '投影更新时间';
COMMENT ON COLUMN project_decision_requests.resolved_at IS '投影处理完成时间';

CREATE INDEX idx_project_decision_requests_tenant_project_status ON project_decision_requests(tenant_id, project_id, status_snapshot, created_at DESC);
CREATE INDEX idx_project_decision_requests_tenant_approval ON project_decision_requests(tenant_id, approval_request_id);
CREATE TRIGGER update_project_decision_requests_updated_at BEFORE UPDATE ON project_decision_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

- [x] **Step 4: Add approval sqlc queries**

Create `apps/control-plane/internal/storage/queries/approval.sql` with:

```sql
-- name: CreateApprovalRequest :one
INSERT INTO approval_requests (
    tenant_id,
    resource_type,
    resource_id,
    requester_type,
    requester_id,
    target_user_id,
    decision_type,
    title,
    summary,
    risk_level,
    status,
    options,
    context_payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.arg('requester_type')::varchar,
    sqlc.narg('requester_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('decision_type')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('options')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetApprovalRequest :one
SELECT * FROM approval_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ResolveApprovalRequest :one
UPDATE approval_requests
SET status = sqlc.arg('status')::varchar,
    resolved_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending'
RETURNING *;

-- name: CreateApprovalDecision :one
INSERT INTO approval_decisions (
    tenant_id,
    approval_request_id,
    decided_by_user_id,
    decision,
    comment,
    payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('approval_request_id')::uuid,
    sqlc.arg('decided_by_user_id')::uuid,
    sqlc.arg('decision')::varchar,
    sqlc.narg('comment')::text,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListApprovalDecisionsForRequest :many
SELECT * FROM approval_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND approval_request_id = sqlc.arg('approval_request_id')::uuid
ORDER BY created_at DESC;
```

- [x] **Step 5: Add project sqlc queries**

Append these queries to `apps/control-plane/internal/storage/queries/project.sql`:

```sql
-- name: CreateProjectCoordinationJob :one
INSERT INTO project_coordination_jobs (
    tenant_id,
    project_id,
    workflow_id,
    trigger_event_id,
    job_type,
    status,
    input_snapshot_ref,
    started_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('workflow_id')::varchar,
    sqlc.narg('trigger_event_id')::uuid,
    sqlc.arg('job_type')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('input_snapshot_ref')::jsonb, '{}'::jsonb),
    NOW()
) RETURNING *;

-- name: FinishProjectCoordinationJob :one
UPDATE project_coordination_jobs
SET status = sqlc.arg('status')::varchar,
    output_event_ids = COALESCE(sqlc.narg('output_event_ids')::jsonb, output_event_ids),
    finished_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ListProjectCoordinationJobs :many
SELECT * FROM project_coordination_jobs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectRouteDecision :one
INSERT INTO project_route_decisions (
    tenant_id,
    project_id,
    coordination_job_id,
    demand_id,
    candidate_digital_employee_ids,
    selected_digital_employee_ids,
    reason,
    input_requirements,
    expected_outputs,
    budget_estimate,
    requires_human_review,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('coordination_job_id')::uuid,
    sqlc.narg('demand_id')::uuid,
    COALESCE(sqlc.narg('candidate_digital_employee_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('selected_digital_employee_ids')::jsonb, '[]'::jsonb),
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('input_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('expected_outputs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('budget_estimate')::jsonb, '{}'::jsonb),
    sqlc.arg('requires_human_review')::boolean,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectRouteDecisions :many
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectTaskStatus :one
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectExecutionSummary :one
INSERT INTO project_execution_summaries (
    tenant_id,
    project_id,
    project_task_id,
    digital_employee_id,
    conclusion,
    evidence_refs,
    artifact_refs,
    confidence_factors,
    uncertainty,
    missing_information,
    recommended_next_action,
    requires_human_review,
    transfer_request_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('conclusion')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('confidence_factors')::jsonb, '{}'::jsonb),
    sqlc.narg('uncertainty')::text,
    COALESCE(sqlc.narg('missing_information')::jsonb, '[]'::jsonb),
    sqlc.narg('recommended_next_action')::text,
    sqlc.arg('requires_human_review')::boolean,
    sqlc.narg('transfer_request_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectExecutionSummaries :many
SELECT * FROM project_execution_summaries
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectTransferRequest :one
INSERT INTO project_transfer_requests (
    tenant_id,
    project_id,
    project_task_id,
    requested_by_digital_employee_id,
    reason,
    suggested_employee_type,
    suggested_digital_employee_ids,
    missing_context_refs,
    status,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('requested_by_digital_employee_id')::uuid,
    sqlc.arg('reason')::text,
    sqlc.narg('suggested_employee_type')::varchar,
    COALESCE(sqlc.narg('suggested_digital_employee_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('missing_context_refs')::jsonb, '[]'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectTransferRequests :many
SELECT * FROM project_transfer_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectDecisionRequest :one
INSERT INTO project_decision_requests (
    tenant_id,
    project_id,
    approval_request_id,
    coordination_job_id,
    project_task_id,
    target_user_id,
    decision_type,
    title_snapshot,
    summary_snapshot,
    risk_level_snapshot,
    status_snapshot,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('approval_request_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('decision_type')::varchar,
    sqlc.arg('title_snapshot')::varchar,
    sqlc.narg('summary_snapshot')::text,
    sqlc.narg('risk_level_snapshot')::varchar,
    sqlc.arg('status_snapshot')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ResolveProjectDecisionRequest :one
UPDATE project_decision_requests
SET status_snapshot = sqlc.arg('status_snapshot')::varchar,
    resolved_event_id = sqlc.narg('resolved_event_id')::uuid,
    resolved_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status_snapshot = 'pending'
RETURNING *;

-- name: ListProjectDecisionRequests :many
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

- [x] **Step 6: Generate sqlc code**

Run:

```bash
cd apps/control-plane && sqlc generate
```

Expected: generated Go files include `CreateApprovalRequest`, `CreateProjectCoordinationJob`, `CreateProjectRouteDecision`, `CreateProjectExecutionSummary`, `CreateProjectTransferRequest`, and `CreateProjectDecisionRequest`.

- [x] **Step 7: Run database verification**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestProjectManagementV1TemporalCoordinationMigration|TestMigrations' -count=1
```

Expected: PASS.

- [x] **Step 8: Commit database foundation**

Run:

```bash
git add apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries
git commit -m "feat: add project coordination v1 schema"
```

Expected: commit succeeds.

## Task 2: Build Minimal Global Approval Core

**Files:**
- Modify: `apps/control-plane/internal/approval/service.go`
- Create: `apps/control-plane/internal/approval/types.go`
- Create: `apps/control-plane/internal/approval/repository.go`
- Create: `apps/control-plane/internal/approval/pg_repository.go`
- Modify: `apps/control-plane/internal/approval/service_test.go`
- Create: `apps/control-plane/internal/approval/pg_repository_test.go`

- [x] **Step 1: Write the failing service test**

Replace the current empty-shell test content in `apps/control-plane/internal/approval/service_test.go` with:

```go
package approval

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestApprovalServiceCreatesAndResolvesRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	targetUserID := uuid.New()
	resourceID := uuid.New()

	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID:      tenantID,
		ResourceType:  "project_decision",
		ResourceID:    resourceID,
		RequesterType: "project_coordinator",
		TargetUserID:  targetUserID,
		DecisionType:  "route_review",
		Title:         "确认高风险路由",
		Summary:       "需要负责人确认是否继续",
		RiskLevel:     "high",
		Options:       []any{"approved", "rejected", "needs_more_evidence"},
		ContextPayload: map[string]any{"project_id": resourceID.String()},
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if request.Status != ApprovalStatusPending {
		t.Fatalf("expected pending request, got %s", request.Status)
	}

	decision, err := service.ResolveRequest(context.Background(), ResolveRequestInput{
		TenantID:          tenantID,
		ApprovalRequestID: request.ID,
		DecidedByUserID:   targetUserID,
		Decision:          ApprovalDecisionApproved,
		Comment:           "同意继续",
		Payload:           map[string]any{"accepted": true},
	})
	if err != nil {
		t.Fatalf("resolve request: %v", err)
	}
	if decision.Decision != ApprovalDecisionApproved {
		t.Fatalf("expected approved decision, got %s", decision.Decision)
	}
	resolved, err := service.GetRequest(context.Background(), tenantID, request.ID)
	if err != nil {
		t.Fatalf("get resolved request: %v", err)
	}
	if resolved.Status != ApprovalStatusApproved {
		t.Fatalf("expected approved request status, got %s", resolved.Status)
	}
}

func TestApprovalServiceRejectsInvalidAndDuplicateResolution(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID: uuid.New(),
		Title:    "缺少字段",
	})
	if !errors.Is(err, ErrInvalidApprovalRequest) {
		t.Fatalf("expected invalid request, got %v", err)
	}

	tenantID := uuid.New()
	request, err := service.CreateRequest(context.Background(), CreateRequestInput{
		TenantID:      tenantID,
		ResourceType:  "project_decision",
		ResourceID:    uuid.New(),
		RequesterType: "project_coordinator",
		TargetUserID:  uuid.New(),
		DecisionType:  "route_review",
		Title:         "确认",
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	input := ResolveRequestInput{
		TenantID:          tenantID,
		ApprovalRequestID: request.ID,
		DecidedByUserID:   request.TargetUserID,
		Decision:          ApprovalDecisionRejected,
	}
	if _, err := service.ResolveRequest(context.Background(), input); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := service.ResolveRequest(context.Background(), input); !errors.Is(err, ErrApprovalAlreadyResolved) {
		t.Fatalf("expected duplicate resolve error, got %v", err)
	}
}
```

Also include a small memory repository in the same test file with methods matching `Repository`. Use maps keyed by `uuid.UUID`, update status only when current status is `pending`, and return `ErrApprovalAlreadyResolved` on duplicate resolution.

- [x] **Step 2: Run the failing approval test**

Run:

```bash
go test ./apps/control-plane/internal/approval -count=1
```

Expected: FAIL with undefined `CreateRequestInput`, `ApprovalStatusPending`, and service methods.

- [x] **Step 3: Implement approval domain types**

Create `apps/control-plane/internal/approval/types.go`:

```go
package approval

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidApprovalRequest = errors.New("invalid approval request")
	ErrApprovalNotFound       = errors.New("approval request not found")
	ErrApprovalAlreadyResolved = errors.New("approval request already resolved")
)

type ApprovalStatus string

const (
	ApprovalStatusPending           ApprovalStatus = "pending"
	ApprovalStatusApproved          ApprovalStatus = "approved"
	ApprovalStatusRejected          ApprovalStatus = "rejected"
	ApprovalStatusNeedsMoreEvidence ApprovalStatus = "needs_more_evidence"
	ApprovalStatusCancelled         ApprovalStatus = "cancelled"
)

type ApprovalDecision string

const (
	ApprovalDecisionApproved          ApprovalDecision = "approved"
	ApprovalDecisionRejected          ApprovalDecision = "rejected"
	ApprovalDecisionNeedsMoreEvidence ApprovalDecision = "needs_more_evidence"
)

type ApprovalRequest struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ResourceType   string
	ResourceID     uuid.UUID
	RequesterType  string
	RequesterID    *uuid.UUID
	TargetUserID   uuid.UUID
	DecisionType   string
	Title          string
	Summary        *string
	RiskLevel      *string
	Status         ApprovalStatus
	Options        []any
	ContextPayload map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ResolvedAt     *time.Time
}

type ApprovalDecisionRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ApprovalRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          ApprovalDecision
	Comment           *string
	Payload           map[string]any
	CreatedAt         time.Time
}

type CreateRequestInput struct {
	TenantID       uuid.UUID
	ResourceType   string
	ResourceID     uuid.UUID
	RequesterType  string
	RequesterID    *uuid.UUID
	TargetUserID   uuid.UUID
	DecisionType   string
	Title          string
	Summary        string
	RiskLevel      string
	Options        []any
	ContextPayload map[string]any
}

type ResolveRequestInput struct {
	TenantID          uuid.UUID
	ApprovalRequestID uuid.UUID
	DecidedByUserID   uuid.UUID
	Decision          ApprovalDecision
	Comment           string
	Payload           map[string]any
}
```

- [x] **Step 4: Implement repository interface and service**

Create `apps/control-plane/internal/approval/repository.go`:

```go
package approval

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateApprovalRequest(ctx context.Context, input CreateRequestInput, status ApprovalStatus) (ApprovalRequest, error)
	GetApprovalRequest(ctx context.Context, tenantID, requestID uuid.UUID) (ApprovalRequest, error)
	ResolveApprovalRequest(ctx context.Context, input ResolveRequestInput, status ApprovalStatus) (ApprovalRequest, error)
	CreateApprovalDecision(ctx context.Context, input ResolveRequestInput) (ApprovalDecisionRecord, error)
}
```

Replace `apps/control-plane/internal/approval/service.go` with:

```go
package approval

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("approval repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateRequest(ctx context.Context, input CreateRequestInput) (*ApprovalRequest, error) {
	input.ResourceType = strings.TrimSpace(input.ResourceType)
	input.RequesterType = strings.TrimSpace(input.RequesterType)
	input.DecisionType = strings.TrimSpace(input.DecisionType)
	input.Title = strings.TrimSpace(input.Title)
	if input.TenantID == uuid.Nil || input.ResourceID == uuid.Nil || input.TargetUserID == uuid.Nil || input.ResourceType == "" || input.RequesterType == "" || input.DecisionType == "" || input.Title == "" {
		return nil, ErrInvalidApprovalRequest
	}
	request, err := s.repository.CreateApprovalRequest(ctx, input, ApprovalStatusPending)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *Service) GetRequest(ctx context.Context, tenantID, requestID uuid.UUID) (*ApprovalRequest, error) {
	if tenantID == uuid.Nil || requestID == uuid.Nil {
		return nil, ErrInvalidApprovalRequest
	}
	request, err := s.repository.GetApprovalRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *Service) ResolveRequest(ctx context.Context, input ResolveRequestInput) (*ApprovalDecisionRecord, error) {
	if input.TenantID == uuid.Nil || input.ApprovalRequestID == uuid.Nil || input.DecidedByUserID == uuid.Nil || !validDecision(input.Decision) {
		return nil, ErrInvalidApprovalRequest
	}
	status := statusFromDecision(input.Decision)
	if _, err := s.repository.ResolveApprovalRequest(ctx, input, status); err != nil {
		return nil, err
	}
	decision, err := s.repository.CreateApprovalDecision(ctx, input)
	if err != nil {
		return nil, err
	}
	return &decision, nil
}

func validDecision(decision ApprovalDecision) bool {
	switch decision {
	case ApprovalDecisionApproved, ApprovalDecisionRejected, ApprovalDecisionNeedsMoreEvidence:
		return true
	default:
		return false
	}
}

func statusFromDecision(decision ApprovalDecision) ApprovalStatus {
	switch decision {
	case ApprovalDecisionApproved:
		return ApprovalStatusApproved
	case ApprovalDecisionNeedsMoreEvidence:
		return ApprovalStatusNeedsMoreEvidence
	default:
		return ApprovalStatusRejected
	}
}
```

- [x] **Step 5: Implement PostgreSQL repository**

Create `apps/control-plane/internal/approval/pg_repository.go` with mapper helpers mirroring `project.PgRepository` JSON handling. Required methods:

```go
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateApprovalRequest(ctx context.Context, input CreateRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	options, err := jsonbArray(input.Options, "options")
	if err != nil {
		return ApprovalRequest{}, err
	}
	payload, err := jsonbObject(input.ContextPayload, "context_payload")
	if err != nil {
		return ApprovalRequest{}, err
	}
	row, err := r.q.CreateApprovalRequest(ctx, queries.CreateApprovalRequestParams{
		TenantID:       input.TenantID,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		RequesterType:  input.RequesterType,
		RequesterID:    nullUUID(input.RequesterID),
		TargetUserID:   input.TargetUserID,
		DecisionType:   input.DecisionType,
		Title:          input.Title,
		Summary:        textOrNull(input.Summary),
		RiskLevel:      textOrNull(input.RiskLevel),
		Status:         string(status),
		Options:        options,
		ContextPayload: payload,
	})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) GetApprovalRequest(ctx context.Context, tenantID, requestID uuid.UUID) (ApprovalRequest, error) {
	row, err := r.q.GetApprovalRequest(ctx, queries.GetApprovalRequestParams{TenantID: tenantID, ID: requestID})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) ResolveApprovalRequest(ctx context.Context, input ResolveRequestInput, status ApprovalStatus) (ApprovalRequest, error) {
	row, err := r.q.ResolveApprovalRequest(ctx, queries.ResolveApprovalRequestParams{
		TenantID: input.TenantID,
		ID:       input.ApprovalRequestID,
		Status:   string(status),
	})
	if err != nil {
		return ApprovalRequest{}, err
	}
	return requestFromRecord(row)
}

func (r *PgRepository) CreateApprovalDecision(ctx context.Context, input ResolveRequestInput) (ApprovalDecisionRecord, error) {
	payload, err := jsonbObject(input.Payload, "payload")
	if err != nil {
		return ApprovalDecisionRecord{}, err
	}
	row, err := r.q.CreateApprovalDecision(ctx, queries.CreateApprovalDecisionParams{
		TenantID:          input.TenantID,
		ApprovalRequestID: input.ApprovalRequestID,
		DecidedByUserID:   input.DecidedByUserID,
		Decision:          string(input.Decision),
		Comment:           textOrNull(input.Comment),
		Payload:           payload,
	})
	if err != nil {
		return ApprovalDecisionRecord{}, err
	}
	return decisionFromRecord(row)
}

func requestFromRecord(row queries.ApprovalRequest) (ApprovalRequest, error) {
	options := []any{}
	if len(row.Options) > 0 {
		if err := json.Unmarshal(row.Options, &options); err != nil {
			return ApprovalRequest{}, fmt.Errorf("options: %w", err)
		}
	}
	payload, err := mapFromJSON(row.ContextPayload)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("context_payload: %w", err)
	}
	return ApprovalRequest{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ResourceType:   row.ResourceType,
		ResourceID:     row.ResourceID,
		RequesterType:  row.RequesterType,
		RequesterID:    ptrUUID(row.RequesterID),
		TargetUserID:   row.TargetUserID,
		DecisionType:   row.DecisionType,
		Title:          row.Title,
		Summary:        ptrText(row.Summary),
		RiskLevel:      ptrText(row.RiskLevel),
		Status:         ApprovalStatus(row.Status),
		Options:        options,
		ContextPayload: payload,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		ResolvedAt:     ptrTime(row.ResolvedAt),
	}, nil
}

func decisionFromRecord(row queries.ApprovalDecision) (ApprovalDecisionRecord, error) {
	payload, err := mapFromJSON(row.Payload)
	if err != nil {
		return ApprovalDecisionRecord{}, fmt.Errorf("payload: %w", err)
	}
	return ApprovalDecisionRecord{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ApprovalRequestID: row.ApprovalRequestID,
		DecidedByUserID:   row.DecidedByUserID,
		Decision:          ApprovalDecision(row.Decision),
		Comment:           ptrText(row.Comment),
		Payload:           payload,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}
```

Also add helper functions in this file: `textOrNull`, `ptrText`, `nullUUID`, `ptrUUID`, `ptrTime`, `jsonbObject`, `jsonbArray`, `mapFromJSON`. Copy their behavior from the existing `project` repository helpers and keep them unexported in `approval`.

- [x] **Step 6: Run approval tests**

Run:

```bash
go test ./apps/control-plane/internal/approval -count=1
```

Expected: PASS.

- [x] **Step 7: Commit approval core**

Run:

```bash
git add apps/control-plane/internal/approval
git commit -m "feat: add minimal approval core"
```

Expected: commit succeeds.

## Task 3: Add Project V1 Domain Types, Repository Methods, And Structured Planner Validation

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/planner.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/planner_test.go`

- [x] **Step 1: Write failing planner tests**

Create `apps/control-plane/internal/workflow/projectcoordination/planner_test.go`:

```go
package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
)

func TestPlanDemandRouteSelectsOnlyActiveExecutorPoolMembers(t *testing.T) {
	employeeID := uuid.New()
	reviewerID := uuid.New()
	snapshot := CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "补充回归证据",
			Content: "整理日志并给出结论",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: employeeID, ProjectRole: "executor", Status: "active", DisplayName: "执行员工"},
			{PrincipalID: reviewerID, ProjectRole: "reviewer", Status: "active", DisplayName: "复核员工"},
		},
	}

	decision, err := PlanDemandRoute(snapshot)
	if err != nil {
		t.Fatalf("plan demand route: %v", err)
	}
	if len(decision.SelectedDigitalEmployeeIDs) != 1 || decision.SelectedDigitalEmployeeIDs[0] != employeeID {
		t.Fatalf("expected only executor selected, got %#v", decision.SelectedDigitalEmployeeIDs)
	}
	if decision.RequiresHumanReview {
		t.Fatalf("ordinary demand should not require human review")
	}
}

func TestValidateRouteDecisionRejectsOutOfPoolSelection(t *testing.T) {
	poolID := uuid.New()
	decision := RouteDecisionPlan{
		CandidateDigitalEmployeeIDs: []uuid.UUID{poolID},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{uuid.New()},
		Reason:                      "错误选择",
		ExpectedOutputs:             []string{"执行摘要"},
	}
	err := ValidateRouteDecision(decision, []uuid.UUID{poolID})
	if err == nil {
		t.Fatal("expected out-of-pool route decision to fail validation")
	}
}
```

- [x] **Step 2: Run failing planner tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestPlanDemandRoute|TestValidateRouteDecision' -count=1
```

Expected: FAIL because the package and planner types do not exist.

- [x] **Step 3: Add project V1 types**

Append these types and event constants to `apps/control-plane/internal/project/types.go`:

```go
const (
	ProjectEventWorkflowSignaled       ProjectEventType = "workflow.signaled"
	ProjectEventCoordinationJobCreated ProjectEventType = "coordination_job.created"
	ProjectEventRouteDecisionCreated   ProjectEventType = "route_decision.created"
	ProjectEventTaskCreated            ProjectEventType = "project_task.created"
	ProjectEventTaskDispatched         ProjectEventType = "project_task.dispatched"
	ProjectEventTaskCompleted          ProjectEventType = "project_task.completed"
	ProjectEventTaskFailed             ProjectEventType = "project_task.failed"
	ProjectEventTransferRequested      ProjectEventType = "transfer.requested"
	ProjectEventDecisionRequested      ProjectEventType = "decision.requested"
	ProjectEventDecisionSubmitted      ProjectEventType = "decision.submitted"
)

type CoordinationJob struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	WorkflowID       string
	TriggerEventID   *uuid.UUID
	JobType          string
	Status           string
	InputSnapshotRef map[string]any
	OutputEventIDs   []any
	StartedAt        *time.Time
	FinishedAt       *time.Time
	CreatedAt        time.Time
}

type RouteDecision struct {
	ID                          uuid.UUID
	TenantID                    uuid.UUID
	ProjectID                   uuid.UUID
	CoordinationJobID           uuid.UUID
	DemandID                    *uuid.UUID
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	Reason                      string
	InputRequirements           map[string]any
	ExpectedOutputs             []any
	BudgetEstimate              map[string]any
	RequiresHumanReview         bool
	CreatedEventID              *uuid.UUID
	CreatedAt                   time.Time
}

type ExecutionSummary struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	ProjectID             uuid.UUID
	ProjectTaskID         uuid.UUID
	DigitalEmployeeID     uuid.UUID
	Conclusion            string
	EvidenceRefs          []any
	ArtifactRefs          []any
	ConfidenceFactors     map[string]any
	Uncertainty           *string
	MissingInformation    []any
	RecommendedNextAction *string
	RequiresHumanReview   bool
	TransferRequestID     *uuid.UUID
	CreatedEventID        *uuid.UUID
	CreatedAt             time.Time
}

type TransferRequest struct {
	ID                           uuid.UUID
	TenantID                     uuid.UUID
	ProjectID                    uuid.UUID
	ProjectTaskID                uuid.UUID
	RequestedByDigitalEmployeeID uuid.UUID
	Reason                       string
	SuggestedEmployeeType        *string
	SuggestedDigitalEmployeeIDs  []uuid.UUID
	MissingContextRefs           []any
	Status                       string
	CreatedEventID               *uuid.UUID
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type DecisionRequest struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ApprovalRequestID uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	TargetUserID      uuid.UUID
	DecisionType      string
	TitleSnapshot     string
	SummarySnapshot   *string
	RiskLevelSnapshot *string
	StatusSnapshot    string
	CreatedEventID    *uuid.UUID
	ResolvedEventID   *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
}
```

- [x] **Step 4: Add repository interface methods**

Add to `apps/control-plane/internal/project/repository.go`:

```go
	CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error)
	FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error)
	ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)
	CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error)
	ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)
	CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error)
	UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID) (ProjectTask, error)
	CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error)
	ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)
	CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error)
	ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)
	CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error)
	ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error)
	ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)
```

Define the request structs in the same file, using fields matching the domain structs and sqlc query inputs.

- [x] **Step 5: Implement structured planner**

Create `apps/control-plane/internal/workflow/projectcoordination/planner.go`:

```go
package projectcoordination

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidRouteDecision = errors.New("invalid route decision")

type CoordinationSnapshot struct {
	ProjectID            uuid.UUID
	Demand               DemandSnapshot
	DigitalEmployeePool  []ProjectMemberSnapshot
	CoordinationPolicy   map[string]any
	PreviousRouteContext map[string]any
}

type DemandSnapshot struct {
	ID      uuid.UUID
	Title   string
	Content string
}

type ProjectMemberSnapshot struct {
	PrincipalID uuid.UUID
	ProjectRole string
	Status      string
	DisplayName string
}

type RouteDecisionPlan struct {
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	Reason                      string
	InputRequirements           map[string]any
	ExpectedOutputs             []string
	BudgetEstimate              map[string]any
	RequiresHumanReview         bool
	TaskTitle                   string
	TaskSummary                 string
}

func PlanDemandRoute(snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	candidates := activeExecutorIDs(snapshot.DigitalEmployeePool)
	if len(candidates) == 0 {
		return RouteDecisionPlan{}, ErrInvalidRouteDecision
	}
	selected := []uuid.UUID{candidates[0]}
	title := strings.TrimSpace(snapshot.Demand.Title)
	if title == "" {
		title = "处理项目需求"
	}
	decision := RouteDecisionPlan{
		CandidateDigitalEmployeeIDs: candidates,
		SelectedDigitalEmployeeIDs:  selected,
		Reason:                      "选择项目数字员工池中的 active executor 作为第一执行人",
		InputRequirements: map[string]any{
			"demand_id": snapshot.Demand.ID.String(),
			"title":     title,
			"content":   snapshot.Demand.Content,
		},
		ExpectedOutputs:     []string{"execution_summary", "evidence_refs", "recommended_next_action"},
		BudgetEstimate:      map[string]any{"mode": "policy_default"},
		RequiresHumanReview: highRiskPolicyEnabled(snapshot.CoordinationPolicy),
		TaskTitle:           title,
		TaskSummary:         snapshot.Demand.Content,
	}
	return decision, ValidateRouteDecision(decision, candidates)
}

func ValidateRouteDecision(decision RouteDecisionPlan, poolIDs []uuid.UUID) error {
	if strings.TrimSpace(decision.Reason) == "" || len(decision.SelectedDigitalEmployeeIDs) == 0 || len(decision.ExpectedOutputs) == 0 {
		return ErrInvalidRouteDecision
	}
	pool := map[uuid.UUID]struct{}{}
	for _, id := range poolIDs {
		if id != uuid.Nil {
			pool[id] = struct{}{}
		}
	}
	for _, id := range decision.SelectedDigitalEmployeeIDs {
		if id == uuid.Nil {
			return ErrInvalidRouteDecision
		}
		if _, ok := pool[id]; !ok {
			return ErrInvalidRouteDecision
		}
	}
	return nil
}

func activeExecutorIDs(members []ProjectMemberSnapshot) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalID != uuid.Nil && member.ProjectRole == "executor" && member.Status == "active" {
			ids = append(ids, member.PrincipalID)
		}
	}
	return ids
}

func highRiskPolicyEnabled(policy map[string]any) bool {
	value, ok := policy["require_human_review_for_new_demands"].(bool)
	return ok && value
}
```

- [x] **Step 6: Implement PgRepository V1 methods and mappers**

In `apps/control-plane/internal/project/pg_repository.go`, add methods for each new repository interface method. Follow the existing JSON helper pattern:

```go
func (r *PgRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	rows, err := r.q.ListProjectRouteDecisions(ctx, queries.ListProjectRouteDecisionsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	decisions := make([]RouteDecision, 0, len(rows))
	for _, row := range rows {
		decision, err := routeDecisionFromRecord(row)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
```

Add mapper helpers for UUID arrays encoded as JSON:

```go
func uuidSliceFromJSON(raw []byte) ([]uuid.UUID, error) {
	values := []string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	}
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func jsonbUUIDSlice(values []uuid.UUID, field string) ([]byte, error) {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			encoded = append(encoded, value.String())
		}
	}
	return marshalJSON(encoded, field)
}
```

- [x] **Step 7: Run planner and project package compile tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/project -run 'TestPlanDemandRoute|TestValidateRouteDecision|TestCreateProject' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit domain and planner foundation**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: add project coordination domain model"
```

Expected: commit succeeds.

## Task 4: Implement Temporal Workflow, Activities, And Signal Client

**Files:**
- Modify: `apps/control-plane/go.mod`
- Modify: `apps/control-plane/go.sum`
- Modify: `apps/control-plane/internal/config/config.go`
- Modify: `apps/control-plane/config/config.example.yaml`
- Create: `apps/control-plane/internal/project/coordination_signal.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/client.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/worker.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [x] **Step 1: Add Temporal dependency**

Run:

```bash
cd apps/control-plane && go get go.temporal.io/sdk@latest
```

Expected: `apps/control-plane/go.mod` includes `go.temporal.io/sdk`.

- [x] **Step 2: Add Temporal config**

Modify `apps/control-plane/internal/config/config.go`:

```go
type Config struct {
	HTTP        HTTPConfig        `yaml:"http"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	ObjectStore ObjectStoreConfig `yaml:"objectStore"`
	Temporal    TemporalConfig    `yaml:"temporal"`
}

type TemporalConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Address   string `yaml:"address"`
	Namespace string `yaml:"namespace"`
	TaskQueue string `yaml:"taskQueue"`
}
```

In `defaultConfig()` add:

```go
Temporal: TemporalConfig{
	Enabled:   false,
	Address:   "127.0.0.1:7233",
	Namespace: "default",
	TaskQueue: "superteam-project-coordination",
},
```

In `applyEnv()` add:

```go
if value, ok := os.LookupEnv("TEMPORAL_ENABLED"); ok {
	cfg.Temporal.Enabled = parseBool(value)
}
cfg.Temporal.Address = envOrDefault("TEMPORAL_ADDRESS", cfg.Temporal.Address)
cfg.Temporal.Namespace = envOrDefault("TEMPORAL_NAMESPACE", cfg.Temporal.Namespace)
cfg.Temporal.TaskQueue = envOrDefault("TEMPORAL_TASK_QUEUE", cfg.Temporal.TaskQueue)
```

In `validate()` add:

```go
if cfg.Temporal.Enabled {
	if strings.TrimSpace(cfg.Temporal.Address) == "" {
		return errors.New("TEMPORAL_ADDRESS is required when Temporal is enabled")
	}
	if strings.TrimSpace(cfg.Temporal.Namespace) == "" {
		return errors.New("TEMPORAL_NAMESPACE is required when Temporal is enabled")
	}
	if strings.TrimSpace(cfg.Temporal.TaskQueue) == "" {
		return errors.New("TEMPORAL_TASK_QUEUE is required when Temporal is enabled")
	}
}
```

- [x] **Step 3: Update example config**

Ensure `apps/control-plane/config/config.example.yaml` contains this block. If it is already present from commit `56522da`, leave it unchanged:

```yaml
temporal:
  enabled: false
  address: "127.0.0.1:7233"
  namespace: "default"
  taskQueue: "superteam-project-coordination"
```

- [x] **Step 4: Define project signal interface**

Create `apps/control-plane/internal/project/coordination_signal.go`:

```go
package project

import (
	"context"

	"github.com/google/uuid"
)

type CoordinatorSignalClient interface {
	EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error
	SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error
	SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error
	SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error
	SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error
	SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error
	SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error
	SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error
}

type NoopCoordinatorSignalClient struct{}

func (NoopCoordinatorSignalClient) EnsureProjectCoordinator(context.Context, ProjectCoordinatorSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalDemandSubmitted(context.Context, DemandSubmittedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalProjectPolicyChanged(context.Context, ProjectPolicyChangedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalProjectMemberChanged(context.Context, ProjectMemberChangedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalEmployeeTaskCompleted(context.Context, EmployeeTaskCompletedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalEmployeeTaskFailed(context.Context, EmployeeTaskFailedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalEmployeeTransferRequested(context.Context, EmployeeTransferRequestedSignal) error { return nil }
func (NoopCoordinatorSignalClient) SignalHumanDecisionSubmitted(context.Context, HumanDecisionSubmittedSignal) error { return nil }

type ProjectCoordinatorSignal struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
}

type DemandSubmittedSignal struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	SubmittedByUserID uuid.UUID
	CreatedEventID    uuid.UUID
	WorkflowID        string
}

type ProjectPolicyChangedSignal struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	ConfigRevisionID uuid.UUID
	ChangedEventID   uuid.UUID
	WorkflowID       string
}

type ProjectMemberChangedSignal struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	ChangedMemberIDs []uuid.UUID
	ChangedEventID   uuid.UUID
	WorkflowID       string
}

type EmployeeTaskCompletedSignal struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      uuid.UUID
	ExecutionSummaryID uuid.UUID
	CompletedEventID   uuid.UUID
	WorkflowID         string
}

type EmployeeTaskFailedSignal struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ProjectTaskID   uuid.UUID
	FailureSummary  string
	FailedEventID   uuid.UUID
	WorkflowID      string
}

type EmployeeTransferRequestedSignal struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	TransferRequestID uuid.UUID
	RequestedEventID  uuid.UUID
	WorkflowID        string
}

type HumanDecisionSubmittedSignal struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ApprovalRequestID uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	ResolvedEventID   uuid.UUID
	WorkflowID        string
}
```

- [x] **Step 5: Write failing workflow tests**

Create `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`:

```go
package projectcoordination

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestProjectCoordinatorHandlesDemandSubmitted(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	activities := &Activities{}
	executorID := uuid.New()
	env.RegisterActivity(activities.LoadProjectCoordinationSnapshot)
	env.RegisterActivity(activities.CreateCoordinationJob)
	env.RegisterActivity(activities.PlanDemandRoute)
	env.RegisterActivity(activities.PersistRouteDecision)
	env.RegisterActivity(activities.CreateProjectTasks)
	env.RegisterActivity(activities.AppendProjectEvent)
	env.RegisterActivity(activities.DispatchProjectTask)
	env.OnActivity(activities.LoadProjectCoordinationSnapshot, mockAny(), mockAny()).Return(CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{ID: uuid.New(), Title: "验证 Runtime", Content: "检查心跳"},
		DigitalEmployeePool: []ProjectMemberSnapshot{{PrincipalID: executorID, ProjectRole: "executor", Status: "active"}},
	}, nil)
	env.OnActivity(activities.CreateCoordinationJob, mockAny(), mockAny()).Return(CoordinationJobResult{ID: uuid.New()}, nil)
	env.OnActivity(activities.PlanDemandRoute, mockAny(), mockAny()).Return(RouteDecisionPlan{
		CandidateDigitalEmployeeIDs: []uuid.UUID{executorID},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{executorID},
		Reason:                      "测试路由",
		ExpectedOutputs:             []string{"execution_summary"},
		TaskTitle:                   "验证 Runtime",
	}, nil)
	env.OnActivity(activities.PersistRouteDecision, mockAny(), mockAny()).Return(RouteDecisionResult{ID: uuid.New(), CreatedEventID: uuid.New()}, nil)
	env.OnActivity(activities.CreateProjectTasks, mockAny(), mockAny()).Return([]ProjectTaskResult{{ID: uuid.New()}}, nil)
	env.OnActivity(activities.AppendProjectEvent, mockAny(), mockAny()).Return(ProjectEventResult{ID: uuid.New()}, nil)
	env.OnActivity(activities.DispatchProjectTask, mockAny(), mockAny()).Return(nil)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID: uuid.New(),
			DemandID: uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{ProjectID: uuid.New()})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}
```

Use `mockAny()` helper:

```go
func mockAny() interface{} {
	return mock.Anything
}
```

Import `github.com/stretchr/testify/mock` for the helper.

- [x] **Step 6: Implement workflow types and signal names**

Create `apps/control-plane/internal/workflow/projectcoordination/types.go` with signal constants and compact activity DTOs. Use the exact signal names from the spec:

```go
package projectcoordination

import "github.com/google/uuid"

const (
	SignalDemandSubmitted            = "DemandSubmitted"
	SignalProjectPolicyChanged       = "ProjectPolicyChanged"
	SignalProjectMemberChanged       = "ProjectMemberChanged"
	SignalEmployeeTaskCompleted      = "EmployeeTaskCompleted"
	SignalEmployeeTaskFailed         = "EmployeeTaskFailed"
	SignalEmployeeTransferRequested  = "EmployeeTransferRequested"
	SignalHumanDecisionSubmitted     = "HumanDecisionSubmitted"
	SignalShutdown                   = "Shutdown"
)

type ProjectCoordinatorInput struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
}

type DemandSubmitted struct {
	DemandID          uuid.UUID
	ProjectID         uuid.UUID
	SubmittedByUserID uuid.UUID
	CreatedEventID    uuid.UUID
}

type ProjectPolicyChanged struct {
	ProjectID        uuid.UUID
	ConfigRevisionID uuid.UUID
	ChangedEventID   uuid.UUID
}

type ProjectMemberChanged struct {
	ProjectID        uuid.UUID
	ChangedMemberIDs []uuid.UUID
	ChangedEventID   uuid.UUID
}

type EmployeeTaskCompleted struct {
	ProjectTaskID      uuid.UUID
	ExecutionSummaryID uuid.UUID
	CompletedEventID   uuid.UUID
}

type EmployeeTaskFailed struct {
	ProjectTaskID  uuid.UUID
	FailureSummary string
	FailedEventID  uuid.UUID
}

type EmployeeTransferRequested struct {
	ProjectTaskID     uuid.UUID
	TransferRequestID uuid.UUID
	RequestedEventID  uuid.UUID
}

type HumanDecisionSubmitted struct {
	ApprovalRequestID uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	ResolvedEventID   uuid.UUID
}

type ShutdownSignal struct{}

type CoordinationJobResult struct {
	ID uuid.UUID
}

type RouteDecisionResult struct {
	ID             uuid.UUID
	CreatedEventID uuid.UUID
}

type ProjectTaskResult struct {
	ID uuid.UUID
}

type ProjectEventResult struct {
	ID uuid.UUID
}
```

- [x] **Step 7: Implement workflow loop**

Create `apps/control-plane/internal/workflow/projectcoordination/workflow.go`:

```go
package projectcoordination

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

func ProjectCoordinatorWorkflow(ctx workflow.Context, input ProjectCoordinatorInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:        defaultRetryPolicy(),
	})
	demandCh := workflow.GetSignalChannel(ctx, SignalDemandSubmitted)
	policyCh := workflow.GetSignalChannel(ctx, SignalProjectPolicyChanged)
	memberCh := workflow.GetSignalChannel(ctx, SignalProjectMemberChanged)
	completedCh := workflow.GetSignalChannel(ctx, SignalEmployeeTaskCompleted)
	failedCh := workflow.GetSignalChannel(ctx, SignalEmployeeTaskFailed)
	transferCh := workflow.GetSignalChannel(ctx, SignalEmployeeTransferRequested)
	humanCh := workflow.GetSignalChannel(ctx, SignalHumanDecisionSubmitted)
	shutdownCh := workflow.GetSignalChannel(ctx, SignalShutdown)

	for {
		selector := workflow.NewSelector(ctx)
		var shouldStop bool
		selector.AddReceive(demandCh, func(c workflow.ReceiveChannel, more bool) {
			var signal DemandSubmitted
			c.Receive(ctx, &signal)
			_ = handleDemandSubmitted(ctx, input, signal)
		})
		selector.AddReceive(policyCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ProjectPolicyChanged
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "project policy changed")
		})
		selector.AddReceive(memberCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ProjectMemberChanged
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "project member changed")
		})
		selector.AddReceive(completedCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTaskCompleted
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "employee task completed")
		})
		selector.AddReceive(failedCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTaskFailed
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "employee task failed")
		})
		selector.AddReceive(transferCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTransferRequested
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "employee transfer requested")
		})
		selector.AddReceive(humanCh, func(c workflow.ReceiveChannel, more bool) {
			var signal HumanDecisionSubmitted
			c.Receive(ctx, &signal)
			_ = appendSignalObservedEvent(ctx, input, "human decision submitted")
		})
		selector.AddReceive(shutdownCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ShutdownSignal
			c.Receive(ctx, &signal)
			_ = signal
			shouldStop = true
		})
		selector.Select(ctx)
		if shouldStop {
			return nil
		}
	}
}
```

Implement `handleDemandSubmitted`, `appendSignalObservedEvent`, and `defaultRetryPolicy` in the same file. `handleDemandSubmitted` must call activities in this order: `CreateCoordinationJob`, `LoadProjectCoordinationSnapshot`, `PlanDemandRoute`, `PersistRouteDecision`, `CreateProjectTasks`, `DispatchProjectTask` for each created task, `FinishCoordinationJob`.

- [x] **Step 8: Implement activities shell**

Create `apps/control-plane/internal/workflow/projectcoordination/activities.go`:

```go
package projectcoordination

import "context"

type Activities struct {
	store ActivityStore
}

type ActivityStore interface {
	LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error)
	CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error)
	PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error)
	CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error)
	AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error)
	DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error
	FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error
}

func NewActivities(store ActivityStore) *Activities {
	return &Activities{store: store}
}
```

Each activity method must validate `a.store != nil`, then delegate to the store. `PlanDemandRoute` calls the deterministic planner from Task 3.

- [x] **Step 9: Implement Temporal client and worker registration**

Create `apps/control-plane/internal/workflow/projectcoordination/client.go` with a struct wrapping `go.temporal.io/sdk/client.Client`. It must implement `project.CoordinatorSignalClient`. `EnsureProjectCoordinator` calls `ExecuteWorkflow` with ID reuse policy allowing existing workflow. Signal methods call `SignalWorkflow` with the exact signal names.

Create `apps/control-plane/internal/workflow/projectcoordination/worker.go`:

```go
package projectcoordination

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func NewWorker(c client.Client, taskQueue string, activities *Activities) worker.Worker {
	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflow(ProjectCoordinatorWorkflow)
	w.RegisterActivity(activities)
	return w
}
```

- [x] **Step 10: Run workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit workflow foundation**

Run:

```bash
git add apps/control-plane/go.mod apps/control-plane/go.sum apps/control-plane/internal/config apps/control-plane/config/config.example.yaml apps/control-plane/internal/project/coordination_signal.go apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: add temporal project coordinator workflow"
```

Expected: commit succeeds.

## Task 5: Bridge Project Service To Workflow Signals And Activities

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`

- [x] **Step 1: Write failing project service signal test**

In `apps/control-plane/internal/project/service_test.go`, add:

```go
func TestSubmitDemandSignalsProjectCoordinatorInV1(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected one DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	if coordinator.lastDemand.DemandID != demand.ID || coordinator.lastDemand.CreatedEventID == uuid.Nil {
		t.Fatalf("unexpected demand signal: %#v", coordinator.lastDemand)
	}
}
```

Add `fakeCoordinatorSignalClient` implementing all signal methods and counters.

- [x] **Step 2: Run failing service signal test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestSubmitDemandSignalsProjectCoordinatorInV1 -count=1
```

Expected: FAIL because `NewServiceWithCoordinator` and V1 demand behavior do not exist.

- [x] **Step 3: Add service constructor and signal calls**

Modify `apps/control-plane/internal/project/service.go`:

```go
type Service struct {
	repository  Repository
	coordinator CoordinatorSignalClient
}

func NewService(repository Repository) (*Service, error) {
	return NewServiceWithCoordinator(repository, NoopCoordinatorSignalClient{})
}

func NewServiceWithCoordinator(repository Repository, coordinator CoordinatorSignalClient) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if coordinator == nil {
		coordinator = NoopCoordinatorSignalClient{}
	}
	return &Service{repository: repository, coordinator: coordinator}, nil
}
```

In `CreateProject`, after project creation and initial events:

```go
if err := s.coordinator.EnsureProjectCoordinator(ctx, ProjectCoordinatorSignal{
	TenantID:   req.TenantID,
	ProjectID:  project.ID,
	WorkflowID: project.CoordinationWorkflowID,
}); err != nil {
	return nil, err
}
```

In `SubmitDemand`, change status to `ProjectDemandStatusPlanningPending` and signal:

```go
demand, err := s.repository.CreateProjectDemand(ctx, req, ProjectDemandStatusPlanningPending, &event.ID)
if err != nil {
	return nil, err
}
if err := s.coordinator.SignalDemandSubmitted(ctx, DemandSubmittedSignal{
	TenantID:          req.TenantID,
	ProjectID:         req.ProjectID,
	DemandID:          demand.ID,
	SubmittedByUserID: req.SubmittedByUserID,
	CreatedEventID:    event.ID,
	WorkflowID:        project.CoordinationWorkflowID,
}); err != nil {
	return nil, err
}
return &demand, nil
```

In `UpdateProjectConfig` and `ReplaceProjectMembers`, signal `ProjectPolicyChanged` and `ProjectMemberChanged` after appending the config/member event.

- [x] **Step 4: Implement activity store adapter**

Create `apps/control-plane/internal/workflow/projectcoordination/project_store.go` with a store struct depending on `project.Repository`. It must implement:

- `LoadProjectCoordinationSnapshot`: load project, demand, members; convert only active digital employee executor/reviewer members into snapshot.
- `CreateCoordinationJob`: call project repository `CreateCoordinationJob`.
- `PersistRouteDecision`: append `route_decision.created`, persist route decision with `created_event_id`.
- `CreateProjectTasks`: create one project task per selected employee, append `project_task.created`.
- `DispatchProjectTask`: update task status to `assigned`, append `project_task.dispatched`.
- `AppendProjectEvent`: delegate event append.
- `FinishCoordinationJob`: update job status and output event IDs.

Use actor type `project_coordinator` and actor ID equal to WorkflowID for Workflow-created events.

- [x] **Step 5: Wire services in app container**

Modify `apps/control-plane/internal/app/app.go`:

- Create `approvalRepository := approval.NewPgRepository(q)` and `approvalService`.
- Create Temporal client only when `cfg.Temporal.Enabled`.
- When Temporal disabled, pass `project.NoopCoordinatorSignalClient{}` to project service.
- When Temporal enabled, create project coordination activities and signal client, start worker in `runContainer`, and close Temporal client on shutdown.

Keep app startup valid when Temporal is disabled so local tests and current dev config still run.

- [x] **Step 6: Run service and app tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [x] **Step 7: Commit service bridge**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/app/app.go
git commit -m "feat: bridge project service to coordinator workflow"
```

Expected: commit succeeds.

## Task 6: Add Writeback, Decision Resolve, And V1 Project APIs

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [x] **Step 1: Write failing handler tests for V1 APIs**

Add tests to `apps/control-plane/internal/project/handler_test.go`:

```go
func TestProjectHandlerListsRouteDecisionsAndResolvesDecision(t *testing.T) {
	projectID := uuid.New()
	decisionID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/route-decisions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New()))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.ListRouteDecisions(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected route decisions 200, got %d: %s", resp.Code, resp.Body.String())
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/decisions/"+decisionID.String()+"/resolve", strings.NewReader(`{"decision":"approved","comment":"同意"}`))
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), middleware.TenantIDKey, uuid.New()))
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), middleware.UserIDKey, uuid.New()))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	rctx.URLParams.Add("decisionId", decisionID.String())
	resolveReq = resolveReq.WithContext(context.WithValue(resolveReq.Context(), chi.RouteCtxKey, rctx))
	resolveResp := httptest.NewRecorder()

	handler.ResolveDecision(resolveResp, resolveReq)

	if resolveResp.Code != http.StatusOK {
		t.Fatalf("expected decision resolve 200, got %d: %s", resolveResp.Code, resolveResp.Body.String())
	}
}
```

Extend `handlerTestService` with V1 methods and in-memory return values.

- [x] **Step 2: Run failing handler tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectHandlerListsRouteDecisionsAndResolvesDecision -count=1
```

Expected: FAIL because handler methods and service interface entries are missing.

- [x] **Step 3: Add service methods**

Implement these methods in `apps/control-plane/internal/project/service.go`:

- `ListRouteDecisions(ctx, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error)`
- `ListCoordinationJobs(ctx, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error)`
- `ListDecisionRequests(ctx, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error)`
- `ResolveDecision(ctx, req ResolveDecisionRequest) (*DecisionRequest, error)`
- `ListExecutionSummaries(ctx, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error)`
- `ListTransferRequests(ctx, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error)`
- `CompleteProjectTask(ctx, req CompleteProjectTaskRequest) (*ExecutionSummary, error)`
- `FailProjectTask(ctx, req FailProjectTaskRequest) (*ProjectTask, error)`
- `RequestProjectTaskTransfer(ctx, req RequestProjectTaskTransferRequest) (*TransferRequest, error)`

Validation rules:

- `project_task_id`, `tenant_id`, actor IDs and required text fields must be non-empty.
- Completion and transfer must require `digital_employee_id` to match `ProjectTask.AssignedDigitalEmployeeID`.
- Completion writes `ExecutionSummary`, updates task status to `completed`, appends `project_task.completed`, then signals `EmployeeTaskCompleted`.
- Failure updates task status to `failed`, appends `project_task.failed`, then signals `EmployeeTaskFailed`.
- Transfer writes `TransferRequest` status `requested`, appends `transfer.requested`, then signals `EmployeeTransferRequested`.
- Decision resolve calls approval service, appends `decision.submitted`, updates project decision projection, then signals `HumanDecisionSubmitted`.

- [x] **Step 4: Add handler methods and response mappers**

Add methods in `apps/control-plane/internal/project/handler.go`:

```go
func (h *HTTPHandler) ListRouteDecisions(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListCoordinationJobs(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListDecisionRequests(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ResolveDecision(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListExecutionSummaries(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListTransferRequests(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CompleteProjectTask(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) FailProjectTask(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) RequestProjectTaskTransfer(w http.ResponseWriter, r *http.Request)
```

Console routes use `projectRouteContext`. Runtime writeback routes use `middleware.GetTenantID`, `middleware.GetRuntimeNodeID`, parse `projectTaskId` from URL, and decode JSON body.

Decision resolve body:

```go
type resolveDecisionBody struct {
	Decision string         `json:"decision"`
	Comment  string         `json:"comment"`
	Payload  map[string]any `json:"payload"`
}
```

Completion body:

```go
type completeProjectTaskBody struct {
	DigitalEmployeeID     uuid.UUID      `json:"digital_employee_id"`
	Conclusion            string         `json:"conclusion"`
	EvidenceRefs          []any          `json:"evidence_refs"`
	ArtifactRefs          []any          `json:"artifact_refs"`
	ConfidenceFactors     map[string]any `json:"confidence_factors"`
	Uncertainty           string         `json:"uncertainty"`
	MissingInformation    []any          `json:"missing_information"`
	RecommendedNextAction string         `json:"recommended_next_action"`
	RequiresHumanReview   bool           `json:"requires_human_review"`
}
```

- [x] **Step 5: Register routes**

In `apps/control-plane/internal/api/server.go`, add Console routes:

```go
r.Get("/projects/{projectId}/route-decisions", s.projectHandler.ListRouteDecisions)
r.Get("/projects/{projectId}/coordination-jobs", s.projectHandler.ListCoordinationJobs)
r.Get("/projects/{projectId}/decisions", s.projectHandler.ListDecisionRequests)
r.Post("/projects/{projectId}/decisions/{decisionId}/resolve", s.projectHandler.ResolveDecision)
r.Get("/projects/{projectId}/execution-summaries", s.projectHandler.ListExecutionSummaries)
r.Get("/projects/{projectId}/transfer-requests", s.projectHandler.ListTransferRequests)
```

Add runtime session routes inside the runtime authenticated group:

```go
r.Post("/project-tasks/{projectTaskId}/complete", s.projectHandler.CompleteProjectTask)
r.Post("/project-tasks/{projectTaskId}/fail", s.projectHandler.FailProjectTask)
r.Post("/project-tasks/{projectTaskId}/transfer-requests", s.projectHandler.RequestProjectTaskTransfer)
```

- [x] **Step 6: Update OpenAPI contract**

Modify `contracts/control-plane/openapi.yaml` to include the V1 paths from the spec and the runtime writeback paths above. Use UUID string schemas for all IDs and `additionalProperties: true` for JSON snapshots.

- [x] **Step 7: Run API tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'Project|Runtime.*ProjectTask' -count=1
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 8: Commit APIs**

Run:

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go contracts/control-plane/openapi.yaml scripts/verify-foundation-contracts.mjs
git commit -m "feat: expose project coordination APIs"
```

Expected: commit succeeds.

## Task 7: Add Frontend V1 API Client And Operational UI

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Write failing API client tests**

Add to `apps/web/src/lib/api/projects.test.ts`:

```ts
it("lists route decisions and resolves project decisions", async () => {
  const fetcher = vi.fn(async () =>
    new Response(JSON.stringify([]), {
      headers: { "content-type": "application/json" },
      status: 200,
    }),
  );

  await expect(
    listProjectRouteDecisions(
      { baseUrl: "http://control-plane.local", fetcher },
      "project 1/primary",
      { limit: 10 },
    ),
  ).resolves.toEqual([]);

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/projects/project%201%2Fprimary/route-decisions?limit=10",
    {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    },
  );

  fetcher.mockResolvedValueOnce(
    new Response(JSON.stringify({ id: "decision-1", status_snapshot: "approved" }), {
      headers: { "content-type": "application/json" },
      status: 200,
    }),
  );

  await resolveProjectDecision(
    { baseUrl: "http://control-plane.local", fetcher },
    "project 1/primary",
    "decision 1",
    { decision: "approved", comment: "同意继续" },
  );

  expect(fetcher).toHaveBeenLastCalledWith(
    "http://control-plane.local/api/v1/projects/project%201%2Fprimary/decisions/decision%201/resolve",
    expect.objectContaining({
      body: JSON.stringify({ decision: "approved", comment: "同意继续" }),
      method: "POST",
    }),
  );
});
```

- [ ] **Step 2: Run failing API client test**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts
```

Expected: FAIL because V1 functions are undefined.

- [ ] **Step 3: Add TypeScript V1 API types and functions**

In `apps/web/src/lib/api/projects.ts`, add types:

```ts
export type ProjectRouteDecision = {
  id: string;
  tenant_id: string;
  project_id: string;
  coordination_job_id: string;
  demand_id?: string;
  candidate_digital_employee_ids: string[];
  selected_digital_employee_ids: string[];
  reason: string;
  input_requirements: Record<string, unknown>;
  expected_outputs: unknown[];
  budget_estimate: Record<string, unknown>;
  requires_human_review: boolean;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectCoordinationJob = {
  id: string;
  tenant_id: string;
  project_id: string;
  workflow_id: string;
  trigger_event_id?: string;
  job_type: string;
  status: string;
  input_snapshot_ref: Record<string, unknown>;
  output_event_ids: unknown[];
  started_at?: string;
  finished_at?: string;
  created_at?: string;
};

export type ProjectDecisionRequest = {
  id: string;
  tenant_id: string;
  project_id: string;
  approval_request_id: string;
  coordination_job_id?: string;
  project_task_id?: string;
  target_user_id: string;
  decision_type: string;
  title_snapshot: string;
  summary_snapshot?: string;
  risk_level_snapshot?: string;
  status_snapshot: string;
  created_event_id?: string;
  resolved_event_id?: string;
  created_at?: string;
  resolved_at?: string;
};

export type ProjectExecutionSummary = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id: string;
  digital_employee_id: string;
  conclusion: string;
  evidence_refs: unknown[];
  artifact_refs: unknown[];
  confidence_factors: Record<string, unknown>;
  uncertainty?: string;
  missing_information: unknown[];
  recommended_next_action?: string;
  requires_human_review: boolean;
  transfer_request_id?: string;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectTransferRequest = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id: string;
  requested_by_digital_employee_id: string;
  reason: string;
  suggested_employee_type?: string;
  suggested_digital_employee_ids: string[];
  missing_context_refs: unknown[];
  status: string;
  created_event_id?: string;
  created_at?: string;
};
```

Add functions:

```ts
export function listProjectRouteDecisions(options: ApiClientOptions, projectId: string, filters: PaginationFilters = {}) {
  return getJson<ProjectRouteDecision[]>(options, projectPath(projectId, `/route-decisions${paginationQuery(filters)}`), "project route decisions");
}

export function listProjectCoordinationJobs(options: ApiClientOptions, projectId: string, filters: PaginationFilters = {}) {
  return getJson<ProjectCoordinationJob[]>(options, projectPath(projectId, `/coordination-jobs${paginationQuery(filters)}`), "project coordination jobs");
}

export function listProjectDecisionRequests(options: ApiClientOptions, projectId: string, filters: PaginationFilters = {}) {
  return getJson<ProjectDecisionRequest[]>(options, projectPath(projectId, `/decisions${paginationQuery(filters)}`), "project decisions");
}

export function resolveProjectDecision(options: ApiClientOptions, projectId: string, decisionId: string, input: { decision: string; comment?: string; payload?: Record<string, unknown> }) {
  return postJson<ProjectDecisionRequest>(options, projectPath(projectId, `/decisions/${encodeURIComponent(decisionId)}/resolve`), input, "resolve project decision");
}

export function listProjectExecutionSummaries(options: ApiClientOptions, projectId: string, filters: PaginationFilters = {}) {
  return getJson<ProjectExecutionSummary[]>(options, projectPath(projectId, `/execution-summaries${paginationQuery(filters)}`), "project execution summaries");
}

export function listProjectTransferRequests(options: ApiClientOptions, projectId: string, filters: PaginationFilters = {}) {
  return getJson<ProjectTransferRequest[]>(options, projectPath(projectId, `/transfer-requests${paginationQuery(filters)}`), "project transfer requests");
}
```

- [ ] **Step 4: Wire V1 queries and mutations**

In `apps/web/src/features/projects/index.tsx`, add queries for route decisions, jobs, decisions, execution summaries, and transfer requests. Each query must use `placeholderData: keepPreviousData`. Add `resolveDecisionMutation` and invalidate `project-decisions`, `project-events`, `project-overview`, and `project-tasks` on success.

- [ ] **Step 5: Update operational detail UI**

In `project-operational-detail.tsx`, add props for:

```ts
routeDecisions: ProjectRouteDecision[];
coordinationJobs: ProjectCoordinationJob[];
decisionRequests: ProjectDecisionRequest[];
executionSummaries: ProjectExecutionSummary[];
transferRequests: ProjectTransferRequest[];
onResolveDecision: (decisionId: string, decision: string) => void;
```

Replace the V0 decision placeholder with actionable pending decisions:

```tsx
{decisionRequests.length === 0 ? (
  <EmptyLine label="当前没有待处理的人类决策" />
) : (
  decisionRequests.slice(0, 5).map((decision) => (
    <div className="grid gap-3 p-4" key={decision.id}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{decision.title_snapshot}</p>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
            {decision.summary_snapshot || "等待负责人处理"}
          </p>
        </div>
        <StatusBadge tone={decision.status_snapshot === "pending" ? "warning" : "success"}>
          {decision.status_snapshot}
        </StatusBadge>
      </div>
      {decision.status_snapshot === "pending" ? (
        <div className="flex gap-2">
          <Button size="sm" type="button" onClick={() => onResolveDecision(decision.id, "approved")}>
            批准
          </Button>
          <Button size="sm" type="button" variant="outline" onClick={() => onResolveDecision(decision.id, "needs_more_evidence")}>
            要求补证
          </Button>
        </div>
      ) : null}
    </div>
  ))
)}
```

Add panels for `RouteDecision`, `TransferRequest`, and recent coordination jobs in the existing grid. Keep cards at 8px or current theme radius and avoid nested cards.

- [ ] **Step 6: Add frontend V1 behavior tests**

In `apps/web/src/features/projects/index.test.tsx`, extend `createProjectFetcher` to return V1 endpoints. Add a test:

```ts
it("renders route decisions, transfer requests, and resolves pending human decisions", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher, "project-1");

  await expect.element(screen.getByText("路由决策")).toBeInTheDocument();
  await expect.element(screen.getByText("选择项目数字员工池中的 active executor")).toBeInTheDocument();
  await expect.element(screen.getByText("转派请求")).toBeInTheDocument();
  await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "批准" }));

  await vi.waitFor(() => {
    expect(
      fetchCalls(fetcher).some(([url, init]) => {
        return (
          String(url).endsWith("/api/v1/projects/project-1/decisions/decision-1/resolve") &&
          init?.method === "POST" &&
          JSON.parse(String(init.body)).decision === "approved"
        );
      }),
    ).toBe(true);
  });
});
```

- [ ] **Step 7: Run frontend tests and typecheck**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 8: Commit frontend runtime UI**

Run:

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/features/projects
git commit -m "feat: show project coordination runtime state"
```

Expected: commit succeeds.

## Task 8: Add Config Workflow Impact Prompt And Browser Verification

**Files:**
- Modify: `apps/web/src/features/projects/components/project-config-page.tsx`
- Modify: `apps/web/src/features/projects/config.test.tsx`

- [ ] **Step 1: Write failing config UI test**

Add to `apps/web/src/features/projects/config.test.tsx`:

```ts
it("shows workflow impact notice when coordination policy or members are dirty", async () => {
  const fetcher = createConfigFetcher();
  const screen = await renderConfig(fetcher);

  await userEvent.click(screen.getByRole("tab", { name: "协调策略" }));
  await userEvent.fill(screen.getByLabelText("协调策略 JSON"), '{"cadence":"hourly"}');

  await expect
    .element(screen.getByText("保存后会向当前项目协调 Workflow 发送策略变更 signal"))
    .toBeInTheDocument();

  await userEvent.click(screen.getByRole("tab", { name: "成员" }));
  await userEvent.fill(screen.getByLabelText("项目成员 JSON"), "[]");

  await expect
    .element(screen.getByText("保存成员后会向当前项目协调 Workflow 发送成员变更 signal"))
    .toBeInTheDocument();
});
```

- [ ] **Step 2: Run failing config test**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/config.test.tsx
```

Expected: FAIL because notices are not rendered.

- [ ] **Step 3: Add notices**

In `project-config-page.tsx`, render `Alert` near the save controls:

```tsx
{isConfigDirty ? (
  <Alert className="mt-4 border-[color:var(--superteam-info)]/30 bg-white/70">
    <GitBranch className="text-[color:var(--superteam-info)]" />
    <AlertTitle>协调 Workflow 将收到配置变更</AlertTitle>
    <AlertDescription>
      保存后会向当前项目协调 Workflow 发送策略变更 signal，新的项目任务将使用最新策略。
    </AlertDescription>
  </Alert>
) : null}
```

Near the member editor save control:

```tsx
{isMembersDirty ? (
  <Alert className="mt-4 border-[color:var(--superteam-decision)]/30 bg-white/70">
    <UserRound className="text-[color:var(--superteam-decision)]" />
    <AlertTitle>数字员工池变更将影响新任务</AlertTitle>
    <AlertDescription>
      保存成员后会向当前项目协调 Workflow 发送成员变更 signal，后续分派只能使用最新 active 数字员工池。
    </AlertDescription>
  </Alert>
) : null}
```

- [ ] **Step 4: Run frontend verification**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/config.test.tsx src/features/projects/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 5: Start local Web dev server for browser QA**

Run:

```bash
pnpm dev:web
```

Expected: Vite serves on `http://127.0.0.1:3000`. If port 3000 is occupied, use the alternative port printed by Vite.

- [ ] **Step 6: Browser screenshot verification**

Open `http://127.0.0.1:3000/projects` in the in-app Browser. Verify:

- Project detail first viewport shows current project, task area, decision queue, and workflow status without overlapping text.
- Config page shows Workflow impact notices after editing coordination policy and members.
- Existing data stays visible during list/filter refresh.

Save screenshots only if the Browser tool or test runner is already configured to capture them in the project screenshots directory.

- [ ] **Step 7: Commit config UX**

Run:

```bash
git add apps/web/src/features/projects/components/project-config-page.tsx apps/web/src/features/projects/config.test.tsx
git commit -m "feat: explain workflow impact on project config"
```

Expected: commit succeeds.

## Task 9: Final Verification, Changelog, And Handoff

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run full backend verification**

Run:

```bash
go test ./apps/control-plane/...
```

Expected: PASS.

- [ ] **Step 2: Run full frontend verification**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected: PASS.

- [ ] **Step 3: Run contract verification**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 4: Run Temporal workflow package tests with race-sensitive package focus**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -race -count=1
```

Expected: PASS.

- [ ] **Step 5: Add changelog entry**

Get timestamp:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add under `CHANGELOG.md` → `[Unreleased]` → `### Added`:

```markdown
- YYYY-MM-DD HH:mm：项目管理 V1 接入 Temporal 虚拟协调线程，新增 RouteDecision、CoordinationJob、ExecutionSummary、TransferRequest、人类决策投影和 Workflow signal 可观测能力。
```

Replace `YYYY-MM-DD HH:mm` with the command output.

- [ ] **Step 6: Commit final docs and generated updates**

Run:

```bash
git add CHANGELOG.md contracts/control-plane/openapi.yaml apps/control-plane/internal/storage/queries apps/control-plane/go.mod apps/control-plane/go.sum
git commit -m "docs: record project management v1 coordination"
```

Expected: commit succeeds if files changed. If no files changed, run `git status --short` and proceed without a docs commit.

- [ ] **Step 7: Summarize stacked branch state**

Run:

```bash
git status --short --branch
git log --oneline --decorate -8
```

Expected: worktree clean; recent log shows V1 commits on top of V0 commit.

## Self-Review

Spec coverage:

- 每个项目绑定 Workflow：Task 4 client、Task 5 CreateProject `EnsureProjectCoordinator`。
- 用户提交需求 signal Workflow：Task 5 `SubmitDemand` signal。
- Workflow 生成 RouteDecision 和 ProjectTask：Task 4 workflow loop、Task 5 activity store。
- ProjectTask 限定项目数字员工池：Task 3 planner validation、Task 5 snapshot and task creation。
- 执行完成、失败、转派、人类决策回写事件流：Task 6 service methods and routes。
- approval 事实源优先全局模块：Task 2 approval core，Task 6 project projection only references approval request。
- 前端运行态展示：Task 7 and Task 8。
- 技术验收测试：Task 1 storage tests、Task 2 approval tests、Task 4 workflow tests、Task 6 handler/API tests、Task 7/8 frontend tests、Task 9 full verification。

Placeholder scan:

- 本计划未使用 `TBD`、`TODO`、`implement later`。
- 所有任务包含明确文件、命令、预期结果和核心代码片段。

Type consistency:

- Go domain type names：`CoordinationJob`、`RouteDecision`、`ExecutionSummary`、`TransferRequest`、`DecisionRequest`。
- API client type names：`ProjectRouteDecision`、`ProjectCoordinationJob`、`ProjectDecisionRequest`、`ProjectExecutionSummary`、`ProjectTransferRequest`。
- Signal names match spec exactly: `DemandSubmitted`、`ProjectPolicyChanged`、`ProjectMemberChanged`、`EmployeeTaskCompleted`、`EmployeeTaskFailed`、`EmployeeTransferRequested`、`HumanDecisionSubmitted`。
