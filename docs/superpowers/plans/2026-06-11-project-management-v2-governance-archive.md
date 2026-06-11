# 项目管理 V2 治理证据归档 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于已合并到 `main` 的 V1 项目协调能力，为项目管理补齐证据链、工件与报告引用、预算流水、验收结论、归档快照、配置修订历史，以及审计中心和成本中心的 `project_id` 联动。

**Architecture:** V2 不重写 V0 项目事实源，也不重做 V1 Temporal 协调 Workflow；它在 `project` 聚合旁边新增治理事实表和服务方法，并通过 `artifact` 模块提供归档保留锁。Control Plane 暴露项目治理 API，Web 在现有项目运行态和配置治理页中增量加入证据、工件、成本、验收、归档和修订历史视图；审计/成本中心只做 `project_id` 查询与跳转，不扩展为完整 BI 平台。

**Tech Stack:** Go + chi/net/http + pgx/sqlc + Atlas + PostgreSQL + S3 object reference；React + TanStack Query + TanStack Router + shadcn/ui + lucide-react + Vitest browser + Playwright screenshot verification。

---

## 参考规格

- 主规格：`docs/superpowers/specs/2026-06-10-project-management-v2-governance-archive-design.md`
- 阶段总索引：`docs/superpowers/specs/2026-06-10-project-management-menu-frontend-backend-design.md`
- V0 规格：`docs/superpowers/specs/2026-06-10-project-management-v0-foundation-design.md`
- V1 规格：`docs/superpowers/specs/2026-06-10-project-management-v1-temporal-coordination-design.md`
- V0 计划：`docs/superpowers/plans/2026-06-11-project-management-v0-foundation.md`
- V1 计划：`docs/superpowers/plans/2026-06-11-project-management-v1-temporal-coordination.md`
- 数据库规则：`DATABASE_DESIGN.md`
- 前端设计规则：`DESIGN.md`

## 执行基线

V1 是 V2 的前置基线。本计划假设执行者只会在 V1 已完成并合并到 `main` 后开始 V2，因此 Task 0 只做 `main` 同步、V1 基线校验和 V2 分支创建，不把 V1 视为待开发工作。

执行 V2 前必须满足：

- 执行分支从包含 V1 的最新 `main` 创建，且 `apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql` 已存在。
- V1 的 `project_coordination_jobs`、`project_route_decisions`、`project_execution_summaries`、`project_transfer_requests`、`project_decision_requests` 表和 API 已通过测试。
- Web 项目运行态已经能展示 RouteDecision、ExecutionSummary、DecisionRequest 和 TransferRequest。
- V2 从最新 `main` 新建分支：`codex/project-management-v2-governance-archive`。

## 范围边界

包含：

- `project_evidence_refs`、`project_artifact_refs`、`project_report_refs`、`project_budget_ledger`、`project_acceptance_records`、`project_archive_snapshots`。
- `project_config_revisions` 查询和轻量增强字段，用于配置修订历史和策略版本对比。
- `artifact_retention_holds`，用于项目归档保留锁。
- 项目治理 API、Web API client、项目详情治理视图、配置修订历史视图。
- 审计中心按 `resource_type=project` 和 `resource_id=project_id` 查询关键动作。
- 成本中心按 `project_id` 查询项目预算流水和成本汇总。

不包含：

- 外部文档管理系统。
- 完整 BI 报表平台。
- 自动事实真伪判定。
- 跨租户归档迁移。
- 复杂合规保留策略引擎。
- 重写或重构 V1 已交付的 Temporal 协调 Workflow。

## 文件结构

### Control Plane 存储层

- Create: `apps/control-plane/internal/storage/migrations/015_project_management_v2_governance_archive.sql`
  - V2 项目治理表、项目配置修订增强字段、artifact 保留锁表。
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
  - 通过 Atlas hash 更新。
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
  - 增加 V2 migration 守卫。
- Create: `apps/control-plane/internal/storage/queries/project_governance.sql`
  - V2 project governance sqlc queries。
- Create: `apps/control-plane/internal/storage/queries/artifact_retention.sql`
  - artifact 保留锁 sqlc queries。
- Modify generated: `apps/control-plane/internal/storage/queries/*.sql.go`
  - 通过 `sqlc generate` 生成。

### Control Plane project 模块

- Modify: `apps/control-plane/internal/project/types.go`
  - 新增 EvidenceRef、ArtifactRef、ReportRef、BudgetLedgerEntry、BudgetSummary、AcceptanceRecord、ArchivePreview、ArchiveSnapshot、ConfigRevisionDiff 类型。
- Modify: `apps/control-plane/internal/project/repository.go`
  - 新增 V2 repository 方法和 request 类型。
- Modify: `apps/control-plane/internal/project/pg_repository.go`
  - 新增 V2 mapper 和 sqlc-backed 实现。
- Modify: `apps/control-plane/internal/project/service.go`
  - 新增证据、工件、报告、预算、验收、归档和配置修订服务逻辑。
- Modify: `apps/control-plane/internal/project/handler.go`
  - 新增 V2 HTTP handlers 和 response DTO。
- Modify: `apps/control-plane/internal/project/service_test.go`
  - 覆盖 V2 服务规则。
- Modify: `apps/control-plane/internal/project/handler_test.go`
  - 覆盖 V2 handler 行为。
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
  - 覆盖 V2 repository mapper 和查询。
- Modify: `apps/control-plane/internal/api/server.go`
  - 注册 V2 routes。
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
  - 覆盖 route auth、tenant scoping、path/body 覆盖。

### Control Plane artifact / audit

- Modify: `apps/control-plane/internal/artifact/service.go`
  - 从空壳扩展为归档保留锁服务。
- Create: `apps/control-plane/internal/artifact/repository.go`
  - artifact retention repository interface。
- Create: `apps/control-plane/internal/artifact/pg_repository.go`
  - sqlc-backed retention repository。
- Modify: `apps/control-plane/internal/artifact/service_test.go`
  - 覆盖归档保留锁和 GC 保护语义。
- Modify: `apps/control-plane/internal/audit/service.go`
  - 支持按 project resource 查询 audit events。
- Create: `apps/control-plane/internal/audit/handler.go`
  - 最小审计中心查询 API。
- Modify: `apps/control-plane/internal/audit/service_test.go`
  - 覆盖 `project_id` 查询。

### 契约与应用装配

- Modify: `contracts/control-plane/openapi.yaml`
  - 增加 V2 project governance、audit project query、cost project query schema。
- Modify: `scripts/verify-foundation-contracts.mjs`
  - 增加 V2 route contract guard。
- Modify: `apps/control-plane/internal/app/app.go`
  - wire project service 的 artifact locker 与 audit logger。
- Modify: `CHANGELOG.md`
  - V2 实施完成时追加 Asia/Shanghai 时间戳变更记录。

### Web

- Modify: `apps/web/src/lib/api/projects.ts`
  - 新增 V2 types 和 client functions。
- Modify: `apps/web/src/lib/api/projects.test.ts`
  - 覆盖 V2 request path、method、body、credentials。
- Create: `apps/web/src/features/projects/components/project-governance-tabs.tsx`
  - 项目详情治理 Tab 容器。
- Create: `apps/web/src/features/projects/components/project-evidence-panel.tsx`
  - 证据链视图和证据状态更新。
- Create: `apps/web/src/features/projects/components/project-artifact-report-panel.tsx`
  - 工件与报告列表。
- Create: `apps/web/src/features/projects/components/project-budget-panel.tsx`
  - 预算流水和汇总。
- Create: `apps/web/src/features/projects/components/project-acceptance-panel.tsx`
  - 验收结论提交和历史展示。
- Create: `apps/web/src/features/projects/components/project-archive-panel.tsx`
  - 归档预览、风险提示、归档快照。
- Create: `apps/web/src/features/projects/components/project-config-revision-history.tsx`
  - 配置修订历史和策略版本对比。
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
  - 接入治理 Tabs、任务证据状态、决策卡片证据提示。
- Modify: `apps/web/src/features/projects/components/project-config-page.tsx`
  - 接入配置修订历史。
- Modify: `apps/web/src/features/projects/index.tsx`
  - 新增 V2 queries/mutations，所有 query 使用 `placeholderData: keepPreviousData`。
- Modify: `apps/web/src/features/projects/index.test.tsx`
  - 覆盖项目详情治理主路径。
- Modify: `apps/web/src/features/projects/config.test.tsx`
  - 覆盖配置修订历史。
- Modify: `apps/web/src/routes/_authenticated/audit/index.tsx`
  - 最小 project-aware audit 查询。
- Modify: `apps/web/src/routes/_authenticated/costs/index.tsx`
  - 最小 project-aware budget 查询。

---

## Task 0: 确认 main 基线并创建 V2 分支

**Files:**
- Read: `docs/superpowers/plans/2026-06-11-project-management-v1-temporal-coordination.md`
- Read: `apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql`
- Read: `apps/control-plane/internal/project/types.go`
- Read: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: 确认 main 已包含 V1 基线**

Run:

```bash
git switch main
git pull --ff-only
git status --short --branch
test -f apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql
rg -n "project_route_decisions|project_execution_summaries|project_decision_requests" apps/control-plane/internal/storage/migrations/014_project_management_v1_temporal_coordination.sql
rg -n "ListRouteDecisions|CompleteProjectTask|ResolveDecision" apps/control-plane/internal/project
```

Expected:

- `git status --short --branch` 显示执行者位于最新 `main`。
- `test -f` exit 0。
- `rg` 能找到 V1 表和 V1 project service / handler 方法。

- [ ] **Step 2: 跑 V1 基线回归，确认 V2 可以开始**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -run 'Project|Coordination|Migration|Migrations' -count=1
pnpm --filter @superteam/web test -- src/features/projects src/lib/api/projects.test.ts src/routes/_authenticated/projects/-project-route.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: all commands exit 0.

- [ ] **Step 3: 创建 V2 分支**

Run:

```bash
git switch -c codex/project-management-v2-governance-archive
git status --short --branch
```

Expected: branch is `codex/project-management-v2-governance-archive` and worktree has no unrelated dirty files.

- [ ] **Step 4: Commit**

This task has no source changes. Do not commit.

## Task 1: 增加 V2 数据库 schema 与 sqlc 查询

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/015_project_management_v2_governance_archive.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Create: `apps/control-plane/internal/storage/queries/project_governance.sql`
- Create: `apps/control-plane/internal/storage/queries/artifact_retention.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.sql.go`

- [ ] **Step 1: 写 migration 失败测试**

Append to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestProjectManagementV2GovernanceArchiveMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/015_project_management_v2_governance_archive.sql")
	if err != nil {
		t.Fatalf("read v2 migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE project_evidence_refs",
		"CREATE TABLE project_artifact_refs",
		"CREATE TABLE project_report_refs",
		"CREATE TABLE project_budget_ledger",
		"CREATE TABLE project_acceptance_records",
		"CREATE TABLE project_archive_snapshots",
		"CREATE TABLE artifact_retention_holds",
		"ALTER TABLE project_config_revisions",
		"tenant_id UUID NOT NULL",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"CREATE INDEX idx_project_evidence_refs_tenant_project_created",
		"CREATE INDEX idx_project_budget_ledger_tenant_project_created",
		"CREATE INDEX idx_project_archive_snapshots_tenant_project_created",
		"CREATE INDEX idx_artifact_retention_holds_tenant_artifact_active",
		"COMMENT ON TABLE project_evidence_refs IS",
		"COMMENT ON COLUMN project_archive_snapshots.retained_artifact_ids IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected V2 migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE project_evidence_status",
		"CREATE TYPE project_acceptance_status",
		"BIGSERIAL PRIMARY KEY",
		"ON DELETE CASCADE",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("V2 migration must not contain %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
cd apps/control-plane && go test ./internal/storage -run TestProjectManagementV2GovernanceArchiveMigration -count=1
```

Expected: FAIL with `open migrations/015_project_management_v2_governance_archive.sql: no such file or directory`.

- [ ] **Step 3: 新增 V2 migration**

Create `apps/control-plane/internal/storage/migrations/015_project_management_v2_governance_archive.sql` with these tables and indexes:

```sql
-- 015_project_management_v2_governance_archive.sql
-- 项目管理 V2：治理、证据、预算、验收和归档快照

CREATE TABLE project_evidence_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID,
    route_decision_id UUID,
    execution_summary_id UUID,
    evidence_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    source_type VARCHAR(100) NOT NULL,
    source_ref TEXT NOT NULL,
    artifact_ref_id UUID,
    submitted_by_type VARCHAR(50) NOT NULL,
    submitted_by_id UUID,
    verification_status VARCHAR(50) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_artifact_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID,
    artifact_id UUID,
    artifact_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    object_ref TEXT NOT NULL,
    content_type VARCHAR(255),
    size_bytes BIGINT,
    checksum VARCHAR(255),
    retention_status VARCHAR(100) NOT NULL DEFAULT 'unheld',
    retention_hold_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_report_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    report_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    object_ref TEXT NOT NULL,
    format VARCHAR(50) NOT NULL,
    generated_by_type VARCHAR(50) NOT NULL,
    generated_by_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_budget_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID,
    project_task_id UUID,
    digital_employee_id UUID,
    cost_type VARCHAR(100) NOT NULL,
    estimated_tokens BIGINT,
    actual_tokens BIGINT,
    estimated_cost NUMERIC(18,6),
    actual_cost NUMERIC(18,6),
    source VARCHAR(100) NOT NULL,
    reason TEXT,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_acceptance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    accepted_by_user_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    conclusion TEXT NOT NULL,
    summary TEXT,
    evidence_ref_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    report_ref_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    unresolved_risks JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_archive_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    snapshot_type VARCHAR(100) NOT NULL,
    status VARCHAR(100) NOT NULL,
    object_ref TEXT,
    summary TEXT,
    included_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    retained_artifact_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    retention_lock_event_id UUID,
    created_by_user_id UUID NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE artifact_retention_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    hold_type VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    reason TEXT,
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ
);

ALTER TABLE project_config_revisions
    ADD COLUMN changed_sections JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN previous_revision_id UUID,
    ADD COLUMN policy_fingerprint VARCHAR(128),
    ADD COLUMN diff_summary JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX idx_project_evidence_refs_tenant_project_created ON project_evidence_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_evidence_refs_tenant_task ON project_evidence_refs(tenant_id, project_task_id);
CREATE INDEX idx_project_evidence_refs_tenant_execution_summary ON project_evidence_refs(tenant_id, execution_summary_id);
CREATE INDEX idx_project_evidence_refs_tenant_status ON project_evidence_refs(tenant_id, project_id, verification_status);

CREATE INDEX idx_project_artifact_refs_tenant_project_created ON project_artifact_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_artifact_refs_tenant_task ON project_artifact_refs(tenant_id, project_task_id);
CREATE INDEX idx_project_artifact_refs_tenant_artifact ON project_artifact_refs(tenant_id, artifact_id);

CREATE INDEX idx_project_report_refs_tenant_project_created ON project_report_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_report_refs_tenant_type_created ON project_report_refs(tenant_id, project_id, report_type, created_at DESC);

CREATE INDEX idx_project_budget_ledger_tenant_project_created ON project_budget_ledger(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_budget_ledger_tenant_task ON project_budget_ledger(tenant_id, project_task_id);
CREATE INDEX idx_project_budget_ledger_tenant_employee ON project_budget_ledger(tenant_id, digital_employee_id, created_at DESC);
CREATE INDEX idx_project_budget_ledger_tenant_cost_type ON project_budget_ledger(tenant_id, project_id, cost_type);

CREATE INDEX idx_project_acceptance_records_tenant_project_created ON project_acceptance_records(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_archive_snapshots_tenant_project_created ON project_archive_snapshots(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_artifact_retention_holds_tenant_artifact_active ON artifact_retention_holds(tenant_id, artifact_id) WHERE released_at IS NULL AND status = 'active';
CREATE INDEX idx_artifact_retention_holds_tenant_resource ON artifact_retention_holds(tenant_id, resource_type, resource_id, created_at DESC);

COMMENT ON TABLE project_evidence_refs IS '项目证据引用表，保存验收、复盘和审计可追踪的结构化证据链接';
COMMENT ON TABLE project_artifact_refs IS '项目工件引用表，保存项目侧的工件审计快照';
COMMENT ON TABLE project_report_refs IS '项目报告引用表，保存最终报告、阶段报告和复盘报告引用';
COMMENT ON TABLE project_budget_ledger IS '项目预算流水表，保存估算和实际成本明细';
COMMENT ON TABLE project_acceptance_records IS '项目验收记录表，保存人类负责人或验收人的最终判断';
COMMENT ON TABLE project_archive_snapshots IS '项目归档快照表，保存归档时的计数、报告和保留锁结果';
COMMENT ON TABLE artifact_retention_holds IS '工件保留锁表，阻止归档项目引用的工件被全局清理';

COMMENT ON COLUMN project_evidence_refs.verification_status IS '证据校验状态：submitted、linked、verified、rejected、superseded';
COMMENT ON COLUMN project_artifact_refs.retention_status IS '项目侧工件保留状态，例如 unheld、project_archive_hold、retention_failed';
COMMENT ON COLUMN project_budget_ledger.actual_cost IS '实际成本金额，使用 NUMERIC 保存精确值';
COMMENT ON COLUMN project_acceptance_records.unresolved_risks IS '验收时仍未关闭的风险列表';
COMMENT ON COLUMN project_archive_snapshots.retained_artifact_ids IS '归档时成功设置保留锁的工件ID数组';
COMMENT ON COLUMN project_config_revisions.changed_sections IS '本次配置修订涉及的配置路径列表';
COMMENT ON COLUMN project_config_revisions.policy_fingerprint IS '配置快照的稳定指纹，用于历史对比';
```

- [ ] **Step 4: 新增 project governance sqlc queries**

Create `apps/control-plane/internal/storage/queries/project_governance.sql`:

```sql
-- name: CreateProjectEvidenceRef :one
INSERT INTO project_evidence_refs (
    tenant_id, project_id, project_task_id, route_decision_id, execution_summary_id,
    evidence_type, title, summary, source_type, source_ref, artifact_ref_id,
    submitted_by_type, submitted_by_id, verification_status, metadata, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid,
    sqlc.narg('project_task_id')::uuid, sqlc.narg('route_decision_id')::uuid,
    sqlc.narg('execution_summary_id')::uuid, sqlc.arg('evidence_type')::varchar,
    sqlc.arg('title')::varchar, sqlc.narg('summary')::text, sqlc.arg('source_type')::varchar,
    sqlc.arg('source_ref')::text, sqlc.narg('artifact_ref_id')::uuid,
    sqlc.arg('submitted_by_type')::varchar, sqlc.narg('submitted_by_id')::uuid,
    sqlc.arg('verification_status')::varchar, COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectEvidenceRefs :many
SELECT * FROM project_evidence_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('verification_status')::varchar IS NULL OR verification_status = sqlc.narg('verification_status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectEvidenceVerificationStatus :one
UPDATE project_evidence_refs
SET verification_status = sqlc.arg('verification_status')::varchar,
    metadata = COALESCE(sqlc.narg('metadata')::jsonb, metadata)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectArtifactRef :one
INSERT INTO project_artifact_refs (
    tenant_id, project_id, project_task_id, artifact_id, artifact_type, title, object_ref,
    content_type, size_bytes, checksum, retention_status, retention_hold_id, metadata, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.narg('project_task_id')::uuid,
    sqlc.narg('artifact_id')::uuid, sqlc.arg('artifact_type')::varchar, sqlc.arg('title')::varchar,
    sqlc.arg('object_ref')::text, sqlc.narg('content_type')::varchar, sqlc.narg('size_bytes')::bigint,
    sqlc.narg('checksum')::varchar, COALESCE(sqlc.narg('retention_status')::varchar, 'unheld'),
    sqlc.narg('retention_hold_id')::uuid, COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectArtifactRefs :many
SELECT * FROM project_artifact_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectArtifactRetention :one
UPDATE project_artifact_refs
SET retention_status = sqlc.arg('retention_status')::varchar,
    retention_hold_id = sqlc.narg('retention_hold_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectReportRef :one
INSERT INTO project_report_refs (
    tenant_id, project_id, report_type, title, summary, object_ref, format,
    generated_by_type, generated_by_id, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('report_type')::varchar,
    sqlc.arg('title')::varchar, sqlc.narg('summary')::text, sqlc.arg('object_ref')::text,
    sqlc.arg('format')::varchar, sqlc.arg('generated_by_type')::varchar,
    sqlc.narg('generated_by_id')::uuid, sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectReportRefs :many
SELECT * FROM project_report_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectBudgetLedgerEntry :one
INSERT INTO project_budget_ledger (
    tenant_id, project_id, coordination_job_id, project_task_id, digital_employee_id,
    cost_type, estimated_tokens, actual_tokens, estimated_cost, actual_cost, source, reason, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('project_task_id')::uuid, sqlc.narg('digital_employee_id')::uuid,
    sqlc.arg('cost_type')::varchar, sqlc.narg('estimated_tokens')::bigint,
    sqlc.narg('actual_tokens')::bigint, sqlc.narg('estimated_cost')::numeric,
    sqlc.narg('actual_cost')::numeric, sqlc.arg('source')::varchar,
    sqlc.narg('reason')::text, sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectBudgetLedger :many
SELECT * FROM project_budget_ledger
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProjectBudgetSummary :one
SELECT
    COALESCE(SUM(estimated_tokens), 0)::bigint AS estimated_tokens,
    COALESCE(SUM(actual_tokens), 0)::bigint AS actual_tokens,
    COALESCE(SUM(estimated_cost), 0)::numeric AS estimated_cost,
    COALESCE(SUM(actual_cost), 0)::numeric AS actual_cost,
    COUNT(*)::integer AS ledger_count
FROM project_budget_ledger
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: CreateProjectAcceptanceRecord :one
INSERT INTO project_acceptance_records (
    tenant_id, project_id, accepted_by_user_id, status, conclusion, summary,
    evidence_ref_ids, report_ref_ids, unresolved_risks, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('accepted_by_user_id')::uuid,
    sqlc.arg('status')::varchar, sqlc.arg('conclusion')::text, sqlc.narg('summary')::text,
    COALESCE(sqlc.narg('evidence_ref_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('report_ref_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('unresolved_risks')::jsonb, '[]'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetLatestProjectAcceptanceRecord :one
SELECT * FROM project_acceptance_records
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateProjectArchiveSnapshot :one
INSERT INTO project_archive_snapshots (
    tenant_id, project_id, snapshot_type, status, object_ref, summary,
    included_counts, retained_artifact_ids, retention_lock_event_id,
    created_by_user_id, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid, sqlc.arg('project_id')::uuid, sqlc.arg('snapshot_type')::varchar,
    sqlc.arg('status')::varchar, sqlc.narg('object_ref')::text, sqlc.narg('summary')::text,
    COALESCE(sqlc.narg('included_counts')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('retained_artifact_ids')::jsonb, '[]'::jsonb),
    sqlc.narg('retention_lock_event_id')::uuid, sqlc.arg('created_by_user_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectArchiveSnapshots :many
SELECT * FROM project_archive_snapshots
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListProjectConfigRevisions :many
SELECT * FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProjectConfigRevision :one
SELECT * FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;
```

- [ ] **Step 5: 新增 artifact retention sqlc queries**

Create `apps/control-plane/internal/storage/queries/artifact_retention.sql`:

```sql
-- name: CreateArtifactRetentionHold :one
INSERT INTO artifact_retention_holds (
    tenant_id, artifact_id, hold_type, resource_type, resource_id, reason, status, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('artifact_id')::uuid,
    sqlc.arg('hold_type')::varchar,
    sqlc.arg('resource_type')::varchar,
    sqlc.arg('resource_id')::uuid,
    sqlc.narg('reason')::text,
    sqlc.arg('status')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListArtifactRetentionHolds :many
SELECT * FROM artifact_retention_holds
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND artifact_id = sqlc.arg('artifact_id')::uuid
  AND released_at IS NULL
ORDER BY created_at DESC;

-- name: CountActiveArtifactRetentionHolds :one
SELECT COUNT(*)::integer AS active_hold_count
FROM artifact_retention_holds
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND artifact_id = sqlc.arg('artifact_id')::uuid
  AND status = 'active'
  AND released_at IS NULL;
```

- [ ] **Step 6: 生成 sqlc 和 Atlas checksum**

Run:

```bash
cd apps/control-plane && sqlc generate
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

Expected: generated Go files compile and `atlas.sum` changes.

- [ ] **Step 7: 运行存储层测试**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries -run 'ProjectManagementV2|Queries' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/015_project_management_v2_governance_archive.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries/project_governance.sql apps/control-plane/internal/storage/queries/artifact_retention.sql apps/control-plane/internal/storage/queries
git commit -m "feat: add project governance archive schema"
```

## Task 2: 补齐 project V2 领域类型与 repository 边界

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`

- [ ] **Step 1: 写 repository mapper 失败测试**

Append to `apps/control-plane/internal/project/pg_repository_test.go`:

```go
func TestProjectGovernanceRepositoryMapsEvidenceBudgetAndArchive(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	eventID := uuid.New()
	now := time.Now()

	evidence, err := evidenceRefFromRecord(queries.ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "execution_log",
		Title:              "测试日志",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/log.txt",
		SubmittedByType:    "digital_employee",
		VerificationStatus: "submitted",
		Metadata:           []byte(`{"suite":"regression"}`),
		CreatedEventID:     uuid.NullUUID{UUID: eventID, Valid: true},
		CreatedAt:          pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map evidence: %v", err)
	}
	if evidence.Metadata["suite"] != "regression" || *evidence.CreatedEventID != eventID {
		t.Fatalf("unexpected evidence mapping: %#v", evidence)
	}

	summary := budgetSummaryFromRecord(queries.GetProjectBudgetSummaryRow{
		EstimatedTokens: 1000,
		ActualTokens:    800,
		EstimatedCost:   numericFromString(t, "0.120000"),
		ActualCost:      numericFromString(t, "0.096000"),
		LedgerCount:     2,
	})
	if summary.ActualTokens != 800 || summary.LedgerCount != 2 {
		t.Fatalf("unexpected budget summary: %#v", summary)
	}
}
```

- [ ] **Step 2: 运行失败测试**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectGovernanceRepositoryMapsEvidenceBudgetAndArchive -count=1
```

Expected: FAIL with undefined mapper/types.

- [ ] **Step 3: 增加 V2 domain types**

Add to `apps/control-plane/internal/project/types.go`:

```go
var (
	ErrInvalidProjectEvidence   = errors.New("invalid project evidence")
	ErrInvalidProjectAcceptance = errors.New("invalid project acceptance")
	ErrProjectArchiveBlocked    = errors.New("project archive blocked")
)

const (
	ProjectEventEvidenceLinked     ProjectEventType = "project.evidence.linked"
	ProjectEventEvidenceVerified   ProjectEventType = "project.evidence.verified"
	ProjectEventArtifactLinked     ProjectEventType = "project.artifact.linked"
	ProjectEventReportLinked       ProjectEventType = "project.report.linked"
	ProjectEventBudgetRecorded     ProjectEventType = "project.budget.recorded"
	ProjectEventAcceptanceSubmitted ProjectEventType = "project.acceptance.submitted"
	ProjectEventArchiveSnapshotCreated ProjectEventType = "project.archive_snapshot.created"
	ProjectEventArchiveRetentionPending ProjectEventType = "project.archive.retention_pending"
)

type EvidenceVerificationStatus string

const (
	EvidenceStatusSubmitted  EvidenceVerificationStatus = "submitted"
	EvidenceStatusLinked     EvidenceVerificationStatus = "linked"
	EvidenceStatusVerified   EvidenceVerificationStatus = "verified"
	EvidenceStatusRejected   EvidenceVerificationStatus = "rejected"
	EvidenceStatusSuperseded EvidenceVerificationStatus = "superseded"
)

type ProjectEvidenceRef struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      *uuid.UUID
	RouteDecisionID    *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	EvidenceType       string
	Title              string
	Summary            *string
	SourceType         string
	SourceRef          string
	ArtifactRefID      *uuid.UUID
	SubmittedByType    string
	SubmittedByID      *uuid.UUID
	VerificationStatus EvidenceVerificationStatus
	Metadata           map[string]any
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
}

type ProjectArtifactRef struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ProjectTaskID   *uuid.UUID
	ArtifactID      *uuid.UUID
	ArtifactType    string
	Title           string
	ObjectRef       string
	ContentType     *string
	SizeBytes       *int64
	Checksum        *string
	RetentionStatus string
	RetentionHoldID *uuid.UUID
	Metadata        map[string]any
	CreatedEventID  *uuid.UUID
	CreatedAt       time.Time
}

type ProjectReportRef struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	ReportType      string
	Title           string
	Summary         *string
	ObjectRef       string
	Format          string
	GeneratedByType string
	GeneratedByID   *uuid.UUID
	CreatedEventID  *uuid.UUID
	CreatedAt       time.Time
}

type ProjectBudgetLedgerEntry struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	DigitalEmployeeID *uuid.UUID
	CostType          string
	EstimatedTokens   *int64
	ActualTokens      *int64
	EstimatedCost     *string
	ActualCost        *string
	Source            string
	Reason            *string
	CreatedEventID    *uuid.UUID
	CreatedAt         time.Time
}

type ProjectBudgetSummary struct {
	EstimatedTokens int64
	ActualTokens    int64
	EstimatedCost   string
	ActualCost      string
	LedgerCount     int32
}

type ProjectAcceptanceRecord struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	AcceptedByUserID uuid.UUID
	Status           string
	Conclusion       string
	Summary          *string
	EvidenceRefIDs   []uuid.UUID
	ReportRefIDs     []uuid.UUID
	UnresolvedRisks  []any
	CreatedEventID   *uuid.UUID
	CreatedAt        time.Time
}

type ProjectArchivePreview struct {
	ProjectID           uuid.UUID         `json:"project_id"`
	CanArchive          bool              `json:"can_archive"`
	IncludedCounts      map[string]int64  `json:"included_counts"`
	RetainedArtifactIDs []uuid.UUID       `json:"retained_artifact_ids"`
	RiskWarnings        []string          `json:"risk_warnings"`
}

type ProjectArchiveSnapshot struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	SnapshotType         string
	Status               string
	ObjectRef            *string
	Summary              *string
	IncludedCounts       map[string]int64
	RetainedArtifactIDs  []uuid.UUID
	RetentionLockEventID *uuid.UUID
	CreatedByUserID      uuid.UUID
	CreatedEventID       *uuid.UUID
	CreatedAt            time.Time
}
```

- [ ] **Step 4: 扩展 repository interface**

Add to `apps/control-plane/internal/project/repository.go`:

```go
	CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error)
	ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error)
	UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceStatusRequest) (ProjectEvidenceRef, error)
	CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error)
	ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error)
	UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error)
	CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error)
	ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error)
	CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error)
	ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error)
	GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error)
	CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error)
	GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error)
	CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error)
	ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error)
	ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error)
	GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error)
```

- [ ] **Step 5: 实现 pg repository mapper 与方法**

In `apps/control-plane/internal/project/pg_repository.go`, add mapping helpers for every V2 generated row:

```go
func evidenceRefFromRecord(row queries.ProjectEvidenceRef) (ProjectEvidenceRef, error) {
	metadata, err := mapFromJSON(row.Metadata)
	if err != nil {
		return ProjectEvidenceRef{}, fmt.Errorf("metadata: %w", err)
	}
	return ProjectEvidenceRef{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		ProjectID:          row.ProjectID,
		ProjectTaskID:      ptrUUID(row.ProjectTaskID),
		RouteDecisionID:    ptrUUID(row.RouteDecisionID),
		ExecutionSummaryID: ptrUUID(row.ExecutionSummaryID),
		EvidenceType:       row.EvidenceType,
		Title:              row.Title,
		Summary:            ptrText(row.Summary),
		SourceType:         row.SourceType,
		SourceRef:          row.SourceRef,
		ArtifactRefID:      ptrUUID(row.ArtifactRefID),
		SubmittedByType:    row.SubmittedByType,
		SubmittedByID:      ptrUUID(row.SubmittedByID),
		VerificationStatus: EvidenceVerificationStatus(row.VerificationStatus),
		Metadata:           metadata,
		CreatedEventID:     ptrUUID(row.CreatedEventID),
		CreatedAt:          row.CreatedAt.Time,
	}, nil
}

func budgetSummaryFromRecord(row queries.GetProjectBudgetSummaryRow) ProjectBudgetSummary {
	return ProjectBudgetSummary{
		EstimatedTokens: row.EstimatedTokens,
		ActualTokens:    row.ActualTokens,
		EstimatedCost:   numericToString(row.EstimatedCost),
		ActualCost:      numericToString(row.ActualCost),
		LedgerCount:     row.LedgerCount,
	}
}
```

Add repository methods that call the generated queries from Task 1 and reuse existing JSON helper functions (`jsonbObject`, `jsonbArray`, `jsonbUUIDSlice`, `uuidSliceFromJSON`, `anySliceFromJSON`).

- [ ] **Step 6: Run repository tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'ProjectGovernanceRepository|PgRepository' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat: add project governance domain repository"
```

## Task 3: 增加 artifact 归档保留锁服务

**Files:**
- Modify: `apps/control-plane/internal/artifact/service.go`
- Create: `apps/control-plane/internal/artifact/repository.go`
- Create: `apps/control-plane/internal/artifact/pg_repository.go`
- Modify: `apps/control-plane/internal/artifact/service_test.go`

- [ ] **Step 1: 写 artifact retention 失败测试**

Replace `apps/control-plane/internal/artifact/service_test.go` with:

```go
package artifact

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestProjectArchiveHoldCreatesOneActiveHoldPerArtifact(t *testing.T) {
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	artifactA := uuid.New()
	artifactB := uuid.New()

	result, err := service.HoldProjectArchiveArtifacts(context.Background(), HoldProjectArchiveArtifactsRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ArtifactIDs: []uuid.UUID{artifactA, artifactB},
		Reason:      "项目归档保留证据工件",
	})
	if err != nil {
		t.Fatalf("hold artifacts: %v", err)
	}
	if len(result.HoldIDs) != 2 || len(result.ArtifactIDs) != 2 {
		t.Fatalf("unexpected hold result: %#v", result)
	}
	if repo.activeCounts[artifactA] != 1 || repo.activeCounts[artifactB] != 1 {
		t.Fatalf("expected active holds for both artifacts, got %#v", repo.activeCounts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/artifact -run TestProjectArchiveHoldCreatesOneActiveHoldPerArtifact -count=1
```

Expected: FAIL with undefined `HoldProjectArchiveArtifactsRequest`.

- [ ] **Step 3: Add artifact retention interfaces and service method**

Create `apps/control-plane/internal/artifact/repository.go`:

```go
package artifact

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateRetentionHold(ctx context.Context, req CreateRetentionHoldRequest) (RetentionHold, error)
	CountActiveRetentionHolds(ctx context.Context, tenantID, artifactID uuid.UUID) (int32, error)
}

type CreateRetentionHoldRequest struct {
	TenantID       uuid.UUID
	ArtifactID     uuid.UUID
	HoldType       string
	ResourceType   string
	ResourceID     uuid.UUID
	Reason         string
	CreatedEventID *uuid.UUID
}

type RetentionHold struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	ArtifactID uuid.UUID
	Status     string
}
```

Replace `apps/control-plane/internal/artifact/service.go` with:

```go
package artifact

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidRetentionHold = errors.New("invalid artifact retention hold")

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("artifact repository is required")
	}
	return &Service{repository: repository}, nil
}

type HoldProjectArchiveArtifactsRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ArtifactIDs []uuid.UUID
	Reason      string
}

type HoldProjectArchiveArtifactsResult struct {
	HoldIDs     []uuid.UUID
	ArtifactIDs []uuid.UUID
}

func (s *Service) HoldProjectArchiveArtifacts(ctx context.Context, req HoldProjectArchiveArtifactsRequest) (*HoldProjectArchiveArtifactsResult, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || len(req.ArtifactIDs) == 0 {
		return nil, ErrInvalidRetentionHold
	}
	result := &HoldProjectArchiveArtifactsResult{
		HoldIDs:     make([]uuid.UUID, 0, len(req.ArtifactIDs)),
		ArtifactIDs: make([]uuid.UUID, 0, len(req.ArtifactIDs)),
	}
	seen := map[uuid.UUID]struct{}{}
	for _, artifactID := range req.ArtifactIDs {
		if artifactID == uuid.Nil {
			return nil, ErrInvalidRetentionHold
		}
		if _, ok := seen[artifactID]; ok {
			continue
		}
		seen[artifactID] = struct{}{}
		hold, err := s.repository.CreateRetentionHold(ctx, CreateRetentionHoldRequest{
			TenantID:     req.TenantID,
			ArtifactID:   artifactID,
			HoldType:     "project_archive_hold",
			ResourceType: "project",
			ResourceID:   req.ProjectID,
			Reason:       req.Reason,
		})
		if err != nil {
			return nil, err
		}
		result.HoldIDs = append(result.HoldIDs, hold.ID)
		result.ArtifactIDs = append(result.ArtifactIDs, artifactID)
	}
	return result, nil
}

func (s *Service) CanDeleteArtifact(ctx context.Context, tenantID, artifactID uuid.UUID) (bool, error) {
	if tenantID == uuid.Nil || artifactID == uuid.Nil {
		return false, ErrInvalidRetentionHold
	}
	count, err := s.repository.CountActiveRetentionHolds(ctx, tenantID, artifactID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
```

- [ ] **Step 4: Implement pg repository**

Create `apps/control-plane/internal/artifact/pg_repository.go`:

```go
package artifact

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateRetentionHold(ctx context.Context, req CreateRetentionHoldRequest) (RetentionHold, error) {
	row, err := r.q.CreateArtifactRetentionHold(ctx, queries.CreateArtifactRetentionHoldParams{
		TenantID:       req.TenantID,
		ArtifactID:     req.ArtifactID,
		HoldType:       req.HoldType,
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		Reason:         textOrNull(req.Reason),
		Status:         "active",
		CreatedEventID: nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return RetentionHold{}, err
	}
	return RetentionHold{ID: row.ID, TenantID: row.TenantID, ArtifactID: row.ArtifactID, Status: row.Status}, nil
}

func (r *PgRepository) CountActiveRetentionHolds(ctx context.Context, tenantID, artifactID uuid.UUID) (int32, error) {
	return r.q.CountActiveArtifactRetentionHolds(ctx, queries.CountActiveArtifactRetentionHoldsParams{
		TenantID:   tenantID,
		ArtifactID: artifactID,
	})
}
```

Add local nullable helpers in the same file if the artifact package does not already have them.

- [ ] **Step 5: Run artifact tests**

Run:

```bash
go test ./apps/control-plane/internal/artifact -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/artifact apps/control-plane/internal/storage/queries/artifact_retention.sql apps/control-plane/internal/storage/queries
git commit -m "feat: add artifact archive retention holds"
```

## Task 4: 实现 project 治理服务逻辑

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/repository.go`

- [ ] **Step 1: 写服务层失败测试**

Append to `apps/control-plane/internal/project/service_test.go`:

```go
func TestProjectGovernanceCreatesEvidenceAndProjectEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	evidence, err := service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ActorType:       "human_user",
		ActorID:         actorID,
		EvidenceType:    "test_result",
		Title:           "回归测试结果",
		SourceType:      "artifact",
		SourceRef:       "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user",
		SubmittedByID:   &actorID,
	})
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if evidence.VerificationStatus != EvidenceStatusSubmitted {
		t.Fatalf("expected submitted evidence, got %s", evidence.VerificationStatus)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventEvidenceLinked {
		t.Fatalf("expected evidence event, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceRequiresHumanOwnerAndFinalReport(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		AcceptedByUserID: otherUserID,
		Status:           "accepted",
		Conclusion:       "通过",
		EvidenceRefIDs:   []uuid.UUID{uuid.New()},
		ReportRefIDs:     []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance actor, got %v", err)
	}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'ProjectGovernanceCreatesEvidence|ProjectAcceptanceRequiresHumanOwner' -count=1
```

Expected: FAIL with undefined service request types and methods.

- [ ] **Step 3: Add service request types**

Add to `apps/control-plane/internal/project/types.go`:

```go
type CreateEvidenceRefServiceRequest struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ActorType          string
	ActorID            uuid.UUID
	ProjectTaskID      *uuid.UUID
	RouteDecisionID    *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	EvidenceType       string
	Title              string
	Summary            string
	SourceType         string
	SourceRef          string
	ArtifactRefID      *uuid.UUID
	SubmittedByType    string
	SubmittedByID      *uuid.UUID
	Metadata           map[string]any
}

type CreateAcceptanceServiceRequest struct {
	TenantID         uuid.UUID
	ProjectID        uuid.UUID
	AcceptedByUserID uuid.UUID
	Status           string
	Conclusion       string
	Summary          string
	EvidenceRefIDs   []uuid.UUID
	ReportRefIDs     []uuid.UUID
	UnresolvedRisks  []any
}

type CreateArchiveSnapshotServiceRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CreatedByUserID uuid.UUID
	SnapshotType     string
	Summary          string
	ObjectRef        string
}
```

- [ ] **Step 4: Add service methods**

Add to `apps/control-plane/internal/project/service.go`:

```go
func (s *Service) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefServiceRequest) (*ProjectEvidenceRef, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.EvidenceType = strings.TrimSpace(req.EvidenceType)
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.SubmittedByType = strings.TrimSpace(req.SubmittedByType)
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorID == uuid.Nil || req.Title == "" || req.EvidenceType == "" || req.SourceType == "" || req.SourceRef == "" || req.SubmittedByType == "" {
		return nil, ErrInvalidProjectEvidence
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if projectRecord.Status == ProjectStatusArchived || projectRecord.ArchivedAt != nil {
		return nil, ErrProjectArchived
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventEvidenceLinked,
		ActorType:    req.ActorType,
		ActorID:      req.ActorID.String(),
		ResourceType: strPtr("project_evidence_ref"),
		Summary:      "项目证据已登记",
		Payload: map[string]any{
			"evidence_type": req.EvidenceType,
			"title":         req.Title,
			"source_type":   req.SourceType,
		},
	})
	if err != nil {
		return nil, err
	}
	evidence, err := s.repository.CreateEvidenceRef(ctx, CreateEvidenceRefRequest{
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		RouteDecisionID:    req.RouteDecisionID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		EvidenceType:       req.EvidenceType,
		Title:              req.Title,
		Summary:            req.Summary,
		SourceType:         req.SourceType,
		SourceRef:          req.SourceRef,
		ArtifactRefID:      req.ArtifactRefID,
		SubmittedByType:    req.SubmittedByType,
		SubmittedByID:      req.SubmittedByID,
		VerificationStatus: EvidenceStatusSubmitted,
		Metadata:           req.Metadata,
		CreatedEventID:     &event.ID,
	})
	if err != nil {
		return nil, err
	}
	return &evidence, nil
}

func validAcceptanceStatus(status string) bool {
	switch status {
	case "accepted", "rejected", "needs_more_evidence", "partially_accepted":
		return true
	default:
		return false
	}
}
```

Add `CreateAcceptanceRecord`, `BuildArchivePreview`, `CreateArchiveSnapshot`, `ListEvidenceRefs`, `ListArtifactRefs`, `ListReportRefs`, `ListBudgetLedger`, `GetBudgetSummary`, `ListArchiveSnapshots`, `ListConfigRevisions`, and `GetConfigRevision` in the same file. Each method must validate tenant/project UUIDs, refuse project mutations after archive, and append a `ProjectEvent` for write operations.

- [ ] **Step 5: Run project service tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'ProjectGovernance|ProjectAcceptance|Archive|Budget|ConfigRevision' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go
git commit -m "feat: add project governance service"
```

## Task 5: 实现项目归档预览、保留锁和归档快照

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/app/app.go`

Task 5 允许在项目 repository 中新增归档成功路径的事务方法，例如 `CreateArchiveSnapshotWithEventAndArchiveProject`，用于保证归档事件、归档快照和项目状态更新原子提交。该范围只服务于归档最终化一致性，不引入 HTTP/OpenAPI/Web 或数据库迁移改动。

- [ ] **Step 1: 写归档保留锁失败测试**

Append to `apps/control-plane/internal/project/service_test.go`:

```go
func TestArchiveSnapshotLocksReferencedArtifactsBeforeArchiving(t *testing.T) {
	repo := newMemoryRepository()
	locker := &fakeArchiveArtifactLocker{}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID:         uuid.New(),
		TenantID:   tenantID,
		ProjectID:  projectID,
		ArtifactID: &artifactID,
		ObjectRef:  "s3://bucket/report.md",
		Title:      "最终报告",
	})

	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CreatedByUserID: ownerID,
		SnapshotType:     "final_archive",
		Summary:          "验收通过后归档",
		ObjectRef:        "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if snapshot.Status != "archived" {
		t.Fatalf("expected archived snapshot, got %s", snapshot.Status)
	}
	if len(locker.artifactIDs) != 1 || locker.artifactIDs[0] != artifactID {
		t.Fatalf("expected artifact lock, got %#v", locker.artifactIDs)
	}
	if repo.projects[projectID].Status != ProjectStatusArchived {
		t.Fatalf("expected project archived after retention lock, got %s", repo.projects[projectID].Status)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestArchiveSnapshotLocksReferencedArtifactsBeforeArchiving -count=1
```

Expected: FAIL with undefined `NewServiceWithArchiveArtifactLocker`.

- [ ] **Step 3: Add archive locker interface**

Add to `apps/control-plane/internal/project/types.go`:

```go
type ArchiveArtifactLocker interface {
	LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (ArchiveArtifactLockResult, error)
}

type ArchiveArtifactLockResult struct {
	HoldIDs     []uuid.UUID
	ArtifactIDs []uuid.UUID
	EventID     *uuid.UUID
}
```

Add constructor to `apps/control-plane/internal/project/service.go`:

```go
func NewServiceWithArchiveArtifactLocker(repository Repository, locker ArchiveArtifactLocker) (*Service, error) {
	service, err := NewService(repository)
	if err != nil {
		return nil, err
	}
	service.archiveArtifactLocker = locker
	return service, nil
}
```

- [ ] **Step 4: Implement archive snapshot flow**

In `CreateArchiveSnapshot`, use this order:

1. Load project and reject archived projects with `ErrProjectArchived`.
2. Build preview counts from repository.
3. Collect artifact IDs from `ProjectArtifactRef.ArtifactID`.
4. Call `archiveArtifactLocker.LockProjectArtifacts`.
5. If lock fails, create snapshot with `status = "archive_pending_retention"` and do not set project status to archived.
6. If lock succeeds, create snapshot with `status = "archived"`, append `project.archive_snapshot.created`, and call repository archive update.

The service method must write `retained_artifact_ids` and `retention_lock_event_id` into `project_archive_snapshots`.

- [ ] **Step 5: Wire app adapter**

In `apps/control-plane/internal/app/app.go`, wrap `artifact.Service` as project `ArchiveArtifactLocker`:

```go
type projectArtifactLocker struct {
	artifactService *artifact.Service
}

func (l projectArtifactLocker) LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (project.ArchiveArtifactLockResult, error) {
	result, err := l.artifactService.HoldProjectArchiveArtifacts(ctx, artifact.HoldProjectArchiveArtifactsRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ArtifactIDs: artifactIDs,
		Reason:      "project archive hold",
	})
	if err != nil {
		return project.ArchiveArtifactLockResult{}, err
	}
	return project.ArchiveArtifactLockResult{HoldIDs: result.HoldIDs, ArtifactIDs: result.ArtifactIDs}, nil
}
```

- [ ] **Step 6: Run archive tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/app -run 'Archive|App' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/app/app.go
git commit -m "feat: lock project artifacts during archive"
```

## Task 6: 增加 V2 HTTP API、routes 与 OpenAPI

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `scripts/verify-foundation-contracts.mjs`

Task 6 允许为 HTTP 契约补齐必要的 service/repository 一致性边界：PATCH 证据校验状态必须与项目审计事件原子提交，PATCH metadata 省略时必须保留原值；被 V2 HTTP API 暴露的单条读取和聚合读取必须把不存在的项目或对象映射为领域级 `ErrProjectNotFound`，避免 OpenAPI 404 契约在实现中退化为 500 或空摘要。

- [ ] **Step 1: 写 handler 失败测试**

Append to `apps/control-plane/internal/project/handler_test.go`:

```go
func TestProjectHandlerCreatesEvidenceFromConsoleContext(t *testing.T) {
	projectID := uuid.New()
	tenantID := uuid.New()
	actorID := uuid.New()
	service := &handlerTestService{}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/evidence", strings.NewReader(`{
		"evidence_type":"test_result",
		"title":"回归测试",
		"source_type":"artifact",
		"source_ref":"s3://bucket/report.json"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, actorID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	resp := httptest.NewRecorder()

	handler.CreateEvidence(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected created evidence, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.createEvidenceReq.TenantID != tenantID || service.createEvidenceReq.ProjectID != projectID || service.createEvidenceReq.ActorID != actorID {
		t.Fatalf("unexpected evidence request: %#v", service.createEvidenceReq)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectHandlerCreatesEvidenceFromConsoleContext -count=1
```

Expected: FAIL with undefined `CreateEvidence`.

- [ ] **Step 3: Add handler methods and DTOs**

Add handlers:

```go
func (h *HTTPHandler) ListEvidence(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CreateEvidence(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) PatchEvidence(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListArtifacts(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListReports(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListBudgetLedger(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetBudgetSummary(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CreateAcceptance(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetAcceptance(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetArchivePreview(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) CreateArchiveSnapshot(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListArchiveSnapshots(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ListConfigRevisions(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetConfigRevision(w http.ResponseWriter, r *http.Request)
```

All write handlers must ignore client-supplied tenant/user/project IDs and use middleware identity plus path IDs.

- [ ] **Step 4: Register routes**

In `apps/control-plane/internal/api/server.go`, add under the existing project route group:

```go
r.Get("/projects/{projectId}/evidence", s.projectHandler.ListEvidence)
r.Post("/projects/{projectId}/evidence", s.projectHandler.CreateEvidence)
r.Patch("/projects/{projectId}/evidence/{evidenceId}", s.projectHandler.PatchEvidence)
r.Get("/projects/{projectId}/artifacts", s.projectHandler.ListArtifacts)
r.Get("/projects/{projectId}/reports", s.projectHandler.ListReports)
r.Get("/projects/{projectId}/budget-ledger", s.projectHandler.ListBudgetLedger)
r.Get("/projects/{projectId}/budget-summary", s.projectHandler.GetBudgetSummary)
r.Post("/projects/{projectId}/acceptance", s.projectHandler.CreateAcceptance)
r.Get("/projects/{projectId}/acceptance", s.projectHandler.GetAcceptance)
r.Get("/projects/{projectId}/archive-preview", s.projectHandler.GetArchivePreview)
r.Post("/projects/{projectId}/archive-snapshot", s.projectHandler.CreateArchiveSnapshot)
r.Get("/projects/{projectId}/archive-snapshots", s.projectHandler.ListArchiveSnapshots)
r.Get("/projects/{projectId}/config-revisions", s.projectHandler.ListConfigRevisions)
r.Get("/projects/{projectId}/config-revisions/{revisionId}", s.projectHandler.GetConfigRevision)
```

- [ ] **Step 5: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`, add paths matching the V2 spec:

```yaml
  /api/v1/projects/{projectId}/evidence:
    get:
      operationId: listProjectEvidence
      summary: List project evidence references
    post:
      operationId: createProjectEvidence
      summary: Create a project evidence reference
  /api/v1/projects/{projectId}/archive-preview:
    get:
      operationId: getProjectArchivePreview
      summary: Preview project archive content and retention needs
  /api/v1/projects/{projectId}/archive-snapshot:
    post:
      operationId: createProjectArchiveSnapshot
      summary: Create a project archive snapshot after artifact retention
```

Add schemas named `ProjectEvidenceRef`, `CreateProjectEvidenceRequest`, `ProjectBudgetSummary`, `ProjectAcceptanceRecord`, `ProjectArchivePreview`, and `ProjectArchiveSnapshot`.

- [ ] **Step 6: Run route and contract tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'Evidence|Archive|Budget|Acceptance|ConfigRevision|ProjectRoutes' -count=1
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go contracts/control-plane/openapi.yaml scripts/verify-foundation-contracts.mjs
git commit -m "feat: expose project governance APIs"
```

## Task 7: 增加 Web API client 和 query wiring

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

Task 7 允许为 V2 query wiring 补充 `ProjectOperationalDetail` 的可选 props 和测试 fetcher fixture；组件在 Task 7 可先接收数据但不必渲染完整治理 UI，完整治理面板仍由 Task 8 实现。

- [ ] **Step 1: 写 API client 失败测试**

Append to `apps/web/src/lib/api/projects.test.ts`:

```ts
it("creates project evidence with credentials and project path", async () => {
  const fetcher = vi.fn(async () => jsonResponse({
    id: "evidence-1",
    tenant_id: "tenant-1",
    project_id: "project-1",
    evidence_type: "test_result",
    title: "回归测试",
    source_type: "artifact",
    source_ref: "s3://bucket/report.json",
    submitted_by_type: "human_user",
    verification_status: "submitted",
    metadata: {},
  }));

  await createProjectEvidence(
    { baseUrl: "http://control-plane.test", fetcher },
    "project-1",
    {
      evidence_type: "test_result",
      title: "回归测试",
      source_type: "artifact",
      source_ref: "s3://bucket/report.json",
    },
  );

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.test/api/v1/projects/project-1/evidence",
    expect.objectContaining({ method: "POST", credentials: "include" }),
  );
});
```

- [ ] **Step 2: Run failing API test**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts
```

Expected: FAIL with undefined `createProjectEvidence`.

- [ ] **Step 3: Add V2 API types and functions**

Add to `apps/web/src/lib/api/projects.ts`:

```ts
export type ProjectEvidenceRef = {
  id: string;
  tenant_id: string;
  project_id: string;
  project_task_id?: string;
  route_decision_id?: string;
  execution_summary_id?: string;
  evidence_type: string;
  title: string;
  summary?: string;
  source_type: string;
  source_ref: string;
  artifact_ref_id?: string;
  submitted_by_type: string;
  submitted_by_id?: string;
  verification_status: "submitted" | "linked" | "verified" | "rejected" | "superseded";
  metadata: Record<string, unknown>;
  created_event_id?: string;
  created_at?: string;
};

export type ProjectBudgetSummary = {
  estimated_tokens: number;
  actual_tokens: number;
  estimated_cost: string;
  actual_cost: string;
  ledger_count: number;
};

export type ProjectArchivePreview = {
  project_id: string;
  can_archive: boolean;
  included_counts: Record<string, number>;
  retained_artifact_ids: string[];
  risk_warnings: string[];
};

export function createProjectEvidence(
  options: ApiClientOptions,
  projectId: string,
  input: {
    evidence_type: string;
    title: string;
    summary?: string;
    source_type: string;
    source_ref: string;
    metadata?: Record<string, unknown>;
  },
): Promise<ProjectEvidenceRef> {
  return postJson<ProjectEvidenceRef>(
    options,
    projectPath(projectId, "/evidence"),
    input,
    "create project evidence",
  );
}

export function listProjectEvidence(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters = {},
): Promise<ProjectEvidenceRef[]> {
  return getJson<ProjectEvidenceRef[]>(
    options,
    projectPath(projectId, `/evidence${paginationQuery(filters)}`),
    "project evidence",
  );
}
```

Add functions for artifacts, reports, budget ledger, budget summary, acceptance, archive preview, archive snapshot, archive snapshots, config revisions, and config revision detail using the V2 endpoint names from Task 6.

- [ ] **Step 4: Add React Query wiring**

In `apps/web/src/features/projects/index.tsx`, add V2 queries:

```ts
const evidenceQuery = useQuery({
  enabled: Boolean(effectiveProjectId),
  queryKey: ["project-evidence", effectiveProjectId],
  queryFn: () => listProjectEvidence(apiOptions, effectiveProjectId as string, { limit: 50 }),
  placeholderData: keepPreviousData,
});

const budgetSummaryQuery = useQuery({
  enabled: Boolean(effectiveProjectId),
  queryKey: ["project-budget-summary", effectiveProjectId],
  queryFn: () => getProjectBudgetSummary(apiOptions, effectiveProjectId as string),
  placeholderData: keepPreviousData,
});

const archivePreviewQuery = useQuery({
  enabled: Boolean(effectiveProjectId),
  queryKey: ["project-archive-preview", effectiveProjectId],
  queryFn: () => getProjectArchivePreview(apiOptions, effectiveProjectId as string),
  placeholderData: keepPreviousData,
});
```

Pass V2 query data into `ProjectOperationalDetail`.

- [ ] **Step 5: Run Web API tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/features/projects/index.tsx
git commit -m "feat: add project governance web client"
```

## Task 8: 增加项目详情治理视图

**Files:**
- Create: `apps/web/src/features/projects/components/project-governance-tabs.tsx`
- Create: `apps/web/src/features/projects/components/project-evidence-panel.tsx`
- Create: `apps/web/src/features/projects/components/project-artifact-report-panel.tsx`
- Create: `apps/web/src/features/projects/components/project-budget-panel.tsx`
- Create: `apps/web/src/features/projects/components/project-acceptance-panel.tsx`
- Create: `apps/web/src/features/projects/components/project-archive-panel.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: 写治理视图失败测试**

Append to `apps/web/src/features/projects/index.test.tsx`:

```tsx
it("renders evidence, budget, acceptance, and archive governance panels without unloading project detail", async () => {
  render(<ProjectsView apiBaseUrl="http://control-plane.test" fetcher={projectGovernanceFetcher} />);

  expect(await screen.findByText("证据链")).toBeInTheDocument();
  expect(screen.getByText("预算流水")).toBeInTheDocument();
  expect(screen.getByText("验收结论")).toBeInTheDocument();
  expect(screen.getByText("归档预览")).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "归档预览" }));

  expect(screen.getByText("需求数")).toBeInTheDocument();
  expect(screen.getByText("保留工件")).toBeInTheDocument();
  expect(screen.getByText("当前项目")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run failing UI test**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/index.test.tsx
```

Expected: FAIL with missing governance panel text.

- [ ] **Step 3: Create governance tabs**

Create `apps/web/src/features/projects/components/project-governance-tabs.tsx`:

```tsx
import { Archive, BadgeCheck, Coins, FileArchive, FileText } from "lucide-react";
import { LiquidTabsList, LiquidTabsTrigger } from "@/components/superteam";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import type {
  ProjectArchivePreview,
  ProjectBudgetSummary,
  ProjectEvidenceRef,
} from "@/lib/api/projects";
import { ProjectArchivePanel } from "./project-archive-panel";
import { ProjectArtifactReportPanel } from "./project-artifact-report-panel";
import { ProjectBudgetPanel } from "./project-budget-panel";
import { ProjectEvidencePanel } from "./project-evidence-panel";
import { ProjectAcceptancePanel } from "./project-acceptance-panel";

type ProjectGovernanceTabsProps = {
  archivePreview?: ProjectArchivePreview;
  budgetSummary?: ProjectBudgetSummary;
  evidence: ProjectEvidenceRef[];
  projectId: string;
};

export function ProjectGovernanceTabs({
  archivePreview,
  budgetSummary,
  evidence,
  projectId,
}: ProjectGovernanceTabsProps) {
  return (
    <Tabs defaultValue="evidence" className="grid gap-3">
      <LiquidTabsList className="w-full justify-start overflow-x-auto">
        <LiquidTabsTrigger value="evidence"><FileText data-icon="inline-start" />证据链</LiquidTabsTrigger>
        <LiquidTabsTrigger value="artifacts"><FileArchive data-icon="inline-start" />工件报告</LiquidTabsTrigger>
        <LiquidTabsTrigger value="budget"><Coins data-icon="inline-start" />预算流水</LiquidTabsTrigger>
        <LiquidTabsTrigger value="acceptance"><BadgeCheck data-icon="inline-start" />验收结论</LiquidTabsTrigger>
        <LiquidTabsTrigger value="archive"><Archive data-icon="inline-start" />归档预览</LiquidTabsTrigger>
      </LiquidTabsList>
      <TabsContent value="evidence"><ProjectEvidencePanel evidence={evidence} /></TabsContent>
      <TabsContent value="artifacts"><ProjectArtifactReportPanel projectId={projectId} /></TabsContent>
      <TabsContent value="budget"><ProjectBudgetPanel summary={budgetSummary} /></TabsContent>
      <TabsContent value="acceptance"><ProjectAcceptancePanel projectId={projectId} /></TabsContent>
      <TabsContent value="archive"><ProjectArchivePanel preview={archivePreview} /></TabsContent>
    </Tabs>
  );
}
```

- [ ] **Step 4: Add focused panels**

Each panel must use existing `LiquidCard`, `StatusBadge`, `SemanticIconTile`, shadcn `Table`, and lucide icons. The panels must render existing data while background refresh happens; no panel may return a full-page loading state when previous data exists.

`ProjectArchivePanel` must show:

- `需求数`
- `任务数`
- `RouteDecision 数`
- `ExecutionSummary 数`
- `DecisionRequest 数`
- `EvidenceRef 数`
- `ArtifactRef 数`
- `ReportRef 数`
- `预算流水数`
- `未关闭风险`
- `保留工件`

- [ ] **Step 5: Wire into operational detail**

Modify `apps/web/src/features/projects/components/project-operational-detail.tsx` props:

```ts
  archivePreview?: ProjectArchivePreview;
  budgetSummary?: ProjectBudgetSummary;
  evidence: ProjectEvidenceRef[];
```

Render `<ProjectGovernanceTabs />` below the V1 runtime panels and above the event stream.

- [ ] **Step 6: Run UI tests and typecheck**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/projects/components/project-governance-tabs.tsx apps/web/src/features/projects/components/project-evidence-panel.tsx apps/web/src/features/projects/components/project-artifact-report-panel.tsx apps/web/src/features/projects/components/project-budget-panel.tsx apps/web/src/features/projects/components/project-acceptance-panel.tsx apps/web/src/features/projects/components/project-archive-panel.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat: show project governance panels"
```

## Task 9: 增加配置修订历史和策略对比

**Files:**
- Create: `apps/web/src/features/projects/components/project-config-revision-history.tsx`
- Modify: `apps/web/src/features/projects/components/project-config-page.tsx`
- Modify: `apps/web/src/features/projects/config.test.tsx`

- [ ] **Step 1: 写配置修订历史失败测试**

Append to `apps/web/src/features/projects/config.test.tsx`:

```tsx
it("shows project config revision history and selected revision diff", async () => {
  render(<ProjectConfigView apiBaseUrl="http://control-plane.test" fetcher={projectConfigRevisionFetcher} projectId="project-1" />);

  expect(await screen.findByText("配置修订历史")).toBeInTheDocument();
  expect(screen.getByText("revision #3")).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "查看 revision #2" }));

  expect(await screen.findByText("协调策略")).toBeInTheDocument();
  expect(screen.getByText("审批策略")).toBeInTheDocument();
  expect(screen.getByText("证据归档规则")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run failing test**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/config.test.tsx
```

Expected: FAIL with missing revision history text.

- [ ] **Step 3: Create revision history component**

Create `apps/web/src/features/projects/components/project-config-revision-history.tsx`:

```tsx
import { History } from "lucide-react";
import { LiquidCard, StatusBadge } from "@/components/superteam";
import { Button } from "@/components/ui/button";
import type { ProjectConfigRevision } from "@/lib/api/projects";

type ProjectConfigRevisionHistoryProps = {
  revisions: ProjectConfigRevision[];
  selectedRevision?: ProjectConfigRevision;
  onSelectRevision: (revisionId: string) => void;
};

export function ProjectConfigRevisionHistory({
  revisions,
  selectedRevision,
  onSelectRevision,
}: ProjectConfigRevisionHistoryProps) {
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex items-center gap-2">
          <History className="size-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">配置修订历史</h3>
        </div>
        <StatusBadge tone="neutral">{revisions.length} 个版本</StatusBadge>
      </div>
      <div className="grid gap-3 p-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <div className="grid gap-2">
          {revisions.map((revision) => (
            <Button
              key={revision.id}
              type="button"
              variant={selectedRevision?.id === revision.id ? "secondary" : "outline"}
              onClick={() => onSelectRevision(revision.id)}
            >
              查看 revision #{revision.revision_number}
            </Button>
          ))}
        </div>
        <pre className="min-h-[220px] overflow-auto rounded-md border bg-muted/30 p-3 text-xs">
          {JSON.stringify(selectedRevision?.config_snapshot ?? {}, null, 2)}
        </pre>
      </div>
    </LiquidCard>
  );
}
```

- [ ] **Step 4: Wire config page queries**

In `apps/web/src/features/projects/components/project-config-page.tsx`, query:

```ts
const revisionsQuery = useQuery({
  queryKey: ["project-config-revisions", projectId],
  queryFn: () => listProjectConfigRevisions(apiOptions, projectId, { limit: 20 }),
  placeholderData: keepPreviousData,
});
```

Keep selected revision state stable across refetch. Only reset the selected revision if the new revision list no longer contains the selected ID.

- [ ] **Step 5: Run config tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/projects/config.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/features/projects/components/project-config-revision-history.tsx apps/web/src/features/projects/components/project-config-page.tsx apps/web/src/features/projects/config.test.tsx
git commit -m "feat: show project config revision history"
```

## Task 10: 增加审计中心和成本中心 project_id 联动

**Files:**
- Modify: `apps/control-plane/internal/audit/service.go`
- Modify: `apps/control-plane/internal/audit/service_test.go`
- Create: `apps/control-plane/internal/audit/handler.go`
- Modify: `apps/control-plane/internal/storage/queries/audit.sql`
- Modify: `apps/control-plane/internal/storage/queries/audit.sql.go`
- Modify: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/storage/queries/queries_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/app/app_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `scripts/verify-foundation-contracts.mjs`
- Modify: `apps/web/src/features/projects/components/project-budget-panel.tsx`
- Modify: `apps/web/src/routes/_authenticated/audit/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/costs/index.tsx`

Task 10 允许补充 audit service 和 API route 测试文件，用于验证 project_id/resource query 只使用当前 console tenant，并避免审计/成本入口变成未测试占位。
Task 10 也允许补充 app wiring 和项目预算面板导出：新建的 audit handler 必须接入真实容器；成本中心路由复用项目预算查询与面板时，可以在 `project-budget-panel.tsx` 中导出 `CostsProjectView`。
Task 10 的后续 reviewer 修复还必须把 project audit 查询下沉到 SQL tenant/resource 过滤后再分页，补齐 OpenAPI/contract guard，并确保前端 `keepPreviousData` 不会在项目切换时短暂展示旧项目数据。

- [ ] **Step 1: 写 audit project query 失败测试与 storage tenant 分页测试**

Append to `apps/control-plane/internal/audit/service_test.go`:

```go
func TestAuditServiceListsProjectEventsByResource(t *testing.T) {
	repo := &memoryRepository{
		events: []*Event{{
			ID:           uuid.New(),
			TenantID:     uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			EventType:    "project.archive_snapshot.created",
			ActorType:    "human_user",
			ActorID:      uuid.NewString(),
			ResourceType: "project",
			ResourceID:   "11111111-1111-1111-1111-111111111111",
			Action:       "project.archive",
		}},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	events, err := service.ListProjectEvents(context.Background(), uuid.MustParse("00000000-0000-0000-0000-000000000001"), uuid.MustParse("11111111-1111-1111-1111-111111111111"), 20, 0)
	if err != nil {
		t.Fatalf("list project events: %v", err)
	}
	if len(events) != 1 || events[0].ResourceType != "project" {
		t.Fatalf("unexpected project events: %#v", events)
	}
}
```

同时在 `apps/control-plane/internal/storage/queries/queries_test.go` 的 audit 测试附近增加 `TestListAuditEventsByResourceFiltersTenantBeforePagination`：

- 插入当前租户 project audit event。
- 插入同 `resource_id` 的其他租户 project audit event，并让它的 `created_at` 更新。
- `limit=1 offset=0` 调用新 query 时必须返回当前租户记录，证明 tenant/resource 过滤发生在 SQL 分页之前。

- [ ] **Step 2: Run failing audit/storage tests**

Run:

```bash
go test ./apps/control-plane/internal/audit -run TestAuditServiceListsProjectEventsByResource -count=1
go test ./apps/control-plane/internal/storage -run TestListAuditEventsByResourceFiltersTenantBeforePagination -count=1
```

Expected: FAIL with undefined service/query implementation.

- [ ] **Step 3: Add SQL-level audit project query**

Extend `apps/control-plane/internal/storage/queries/audit.sql`:

```sql
-- name: ListAuditEventsByResource :many
SELECT *
FROM audit_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND resource_type = sqlc.arg('resource_type')::varchar
  AND resource_id = sqlc.arg('resource_id')::varchar
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

Run `cd apps/control-plane && sqlc generate` and commit generated updates in `audit.sql.go` and `querier.go`.
`PgRepository.ListResourceEvents` must call `ListAuditEventsByResource`; do not filter tenant in Go after `LIMIT/OFFSET`.

- [ ] **Step 4: Add audit service, handler and app wiring**

Extend audit repository/service:

- `Repository.ListResourceEvents(ctx, tenantID, resourceType, resourceID, limit, offset)`。
- `Service.ListProjectEvents(ctx, tenantID, projectID, limit, offset)`，校验 non-nil UUID。
- memory repo 测试必须真实按 tenant/resource 过滤。

Create `apps/control-plane/internal/audit/handler.go` with `GET /api/v1/audit/events?resource_type=project&resource_id={uuid}` and console auth scoping.
在 `api.Server` 中增加 `SetAuditHandler` 并挂到 `/api/v1` console auth group；在 `app.Container` 中创建并保存 `AuditHandler`。

- [ ] **Step 5: Update audit/cost center routes**

In `apps/web/src/routes/_authenticated/audit/index.tsx`, replace the unimplemented page with a compact project-aware audit view:

- 使用 `validateSearch` 安全解析 `project_id`。
- 无 `project_id` 时显示紧凑空状态。
- 有 `project_id` 时请求 `/api/v1/audit/events?resource_type=project&resource_id=${project_id}&limit=50`。
- 查询结果包装为 `{ projectId, events }`；渲染前只使用 `data.projectId === 当前 projectId` 的数据，避免 `keepPreviousData` 切换项目时展示旧项目审计。

In `apps/web/src/routes/_authenticated/costs/index.tsx`, replace the unimplemented page with a project-aware view:

- 使用 `validateSearch` 安全解析 `project_id`。
- 无 `project_id` 时显示紧凑空状态。
- 有 `project_id` 时渲染 `CostsProjectView`。

`CostsProjectView` must call `getProjectBudgetSummary` plus `listProjectBudgetLedger` when `project_id` exists. Ledger 和 summary 查询结果都必须包装为 `{ projectId, ... }`，且只把匹配当前 `projectId` 的数据传给 `ProjectBudgetPanel`。

- [ ] **Step 6: Update OpenAPI and contract guard**

Update `contracts/control-plane/openapi.yaml`:

- Add `GET /api/v1/audit/events`。
- Query 参数：`resource_type`（当前 enum 只支持 `project`）、`resource_id`（uuid）、`limit`、`offset`。
- Response schema 使用 `AuditEvent`，字段与 handler JSON 一致：`id, tenant_id, event_type, actor_type, actor_id, resource_type, resource_id, action, details, ip_address, created_at`。

Update `scripts/verify-foundation-contracts.mjs` required operations with `GET /api/v1/audit/events`。

- [ ] **Step 7: Run audit/cost/contract tests**

Run:

```bash
cd apps/control-plane && sqlc generate
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/audit ./apps/control-plane/internal/api ./apps/control-plane/internal/app -run 'Audit|ProjectEvents|NewContainer' -count=1
go test ./apps/control-plane/internal/audit ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1
pnpm verify:contracts
pnpm --filter @superteam/web test -- src/features/projects/index.test.tsx src/features/projects/config.test.tsx
pnpm --filter @superteam/web typecheck
git diff --check
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/audit apps/control-plane/internal/storage/queries/audit.sql apps/control-plane/internal/storage/queries/audit.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/storage/queries/queries_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go apps/control-plane/internal/app/app.go apps/control-plane/internal/app/app_test.go contracts/control-plane/openapi.yaml scripts/verify-foundation-contracts.mjs apps/web/src/features/projects/components/project-budget-panel.tsx apps/web/src/routes/_authenticated/audit/index.tsx apps/web/src/routes/_authenticated/costs/index.tsx docs/superpowers/plans/2026-06-11-project-management-v2-governance-archive.md
git commit -m "fix: scope project audit and cost views"
```

## Task 11: 浏览器验证、全量测试和变更日志

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run backend verification**

Run:

```bash
go test ./apps/control-plane/...
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected: PASS.

- [ ] **Step 3: Start local Web server**

Run:

```bash
pnpm dev:web
```

Expected: Vite prints a local URL such as `http://127.0.0.1:3000/`.

- [ ] **Step 4: Browser screenshot verification**

Use Browser plugin after the dev server starts:

1. Open `http://127.0.0.1:3000/projects`.
2. Select a project with V2 fixtures or seeded data.
3. Capture screenshot of project detail with governance tabs.
4. Open `/projects/$projectId/config`.
5. Capture screenshot of config revision history.
6. Confirm no text overlaps, no empty full-page loading replacement occurs during refetch, and archive panel clearly shows preview counts plus artifact retention status.

- [ ] **Step 5: Add CHANGELOG entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add the returned timestamp to `CHANGELOG.md` under `### Added`:

```markdown
#### 项目管理 V2 治理证据归档闭环 (2026-06-11 16:29)

- 增加项目证据链、工件与报告引用、预算流水、验收结论和归档快照。
- 增加项目归档工件保留锁，归档成功前阻止证据工件被清理。
- 增加项目详情治理视图、配置修订历史、审计中心和成本中心 project_id 联动。
```

执行 Task 11 时先运行上面的 `date` 命令，并在 `CHANGELOG.md` 使用该命令实际输出的时间。

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: record project management v2 governance archive"
```

## 自检结果

Spec coverage:

- 证据链：Task 1、Task 2、Task 4、Task 6、Task 8。
- 工件与报告引用：Task 1、Task 2、Task 4、Task 6、Task 8。
- 预算流水和成本汇总：Task 1、Task 2、Task 4、Task 7、Task 8、Task 10。
- 验收结论：Task 1、Task 4、Task 6、Task 8。
- 归档预览、归档快照、artifact 保留锁：Task 1、Task 3、Task 5、Task 6、Task 8。
- 配置修订历史和策略版本对比：Task 1、Task 6、Task 7、Task 9。
- 审计中心 project_id 联动：Task 10。
- 成本中心 project_id 联动：Task 10。
- V2 不重做 V0/V1：Task 0 执行基线和范围边界明确。

Placeholder scan:

- 已通过占位词扫描，未发现未展开的实现步骤。
- 每个写代码任务都有失败测试、执行命令、实现片段、通过测试和 commit 步骤。

Type consistency:

- 后端类型统一使用 `ProjectEvidenceRef`、`ProjectArtifactRef`、`ProjectReportRef`、`ProjectBudgetLedgerEntry`、`ProjectBudgetSummary`、`ProjectAcceptanceRecord`、`ProjectArchivePreview`、`ProjectArchiveSnapshot`。
- 前端类型与后端 JSON response 使用 snake_case。
- Archive artifact lock 边界统一命名为 `ArchiveArtifactLocker`。

## Execution Handoff

V2 基于已包含 V1 的 `main` 执行。执行方式：

1. **Subagent-Driven (recommended)** - 每个 Task 一个新 subagent，任务之间做 review，适合这个跨后端、前端、数据库的阶段。
2. **Inline Execution** - 在同一会话使用 executing-plans 按任务批量执行，每个 Task 完成后 checkpoint。
