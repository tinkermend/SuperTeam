# 场景模板 P2b（选角与补员）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 P2a 的"规划期拒绝"升级为"规划期判定 + 可操作的补救闭环"：事实性可行性三档取代 LLM 自评裁决、结构化缺口报告进 inbox、标准员工模板一键补员（人确认且**真正可派发**）、豁免一等决策记录、场景模板管理 API + Console 管理页。

**Architecture:** 全部构建在已合并的 P2a（main `9f2e1cf9`）之上：可行性判定复用 `ApplyPlanningProfileScores` 已写入任务的服务端 `SelectionScore`（LLM `SelectionConfidence` 降级为参考）；缺口报告 ride 既有 `DecisionRequest` 机制（新 decision_type=`planning_gap`，自动获得 inbox 投影与 web 动态 actions）；补员复用 `CreateDigitalEmployee`（模板 `capability_bindings` 合并链已存在）+ `PUT /projects/{id}/members` 前端组合；管理 API 照抄 capability 模块 MCP registry 模式（authz 常量 `scenario_template.manage` 已存在未接线）。

**Tech Stack:** Go (chi + sqlc + Temporal) / Atlas / OpenAPI (oapi-codegen Go server, TS 客户端手写) / React + TanStack Router + vitest browser。

**Spec:** `docs/superpowers/specs/2026-07-15-scenario-template-p2-contract-governance-design.md` §7 P2b。§8 待决策项在此拍板：#1 词汇表独立表 `capability_vocabulary`（后续与 capability 域合并另议）；#2 豁免走 project 域 DecisionRequest + 专表（authzcenter 的 DecisionRecord 是 operation_log 投影、无生命周期，仅作事后审计，不承载豁免）；#3 标准员工种子首批 = 代码审查员 + 测试员（安全审查员缓）。

**显式缓做（非遗漏）：** spec §3.6 的"员工画像 capability key 不在词汇表 → selection_warnings"软校验缓做——它要求 planning profile adapter 反向依赖词汇表，跨模块成本高于当前收益（能力匹配本就 advisory）；待词汇表与 capability 域合并评审时一并做。

**P2a 实测教训的设计修正（相对 spec §3.5 原文）：**
- **不为瞬态条件设终局拒绝**：HardFailures（runtime 未就绪等）保持"翻转人类审批"的降级语义（现状），终局拒绝只保留给结构性缺口（role_independence 池不足，P2a 已有）。三档 = 通过（无注记）/ 降级（低分注记 + 人类确认，plan 模式本就全量确认，注记让确认"知情"）/ 拒绝（结构性，带缺口报告）。
- **移除 LLM 置信度硬门**（graph_validation.go 的 `SelectionConfidence < threshold → ErrNoSuitableEmployee`）——它是 07-13/07-15 两轮 E2E 的假阴性来源（LLM 自评 0.3-0.6 杀死可行计划）。`selection_confidence` 保留进 payload 作参考。
- **需求重开是显式例外**：demand 状态机 forward-only（failed=终态 rank）；补员/豁免后重规划需要专用 `ReopenProjectDemandForReplanning`（带审计事件），不是放宽 rank 守卫。
- **Temporal 纪律**（P2a 缺陷③教训）：本计划两处改 workflow 决策序列（Task 5/6 的 planning_gap case 与 gap 结构透传）。新增分支必须由**新的判别事实**天然区分（新 decision_type / 新错误载荷字段只在新执行中产生），禁止事后 GetVersion 围栏；每个改 workflow.go 的任务必须跑 `replay_test.go`（真实历史回放）并在报告中附结果。

## Global Constraints

- 根级命令 `corepack pnpm <script>`；Go 测试 `cd apps/control-plane && go test ./internal/... `（定向包）；Web 测试 `corepack pnpm --filter @superteam/web test -- --no-file-parallelism`；禁止 `npx playwright` / `npx vitest`。
- 迁移目录唯一；**编号以当前最大为准顺延**（并发会话可能已用 062，Task 1 先 `ls apps/control-plane/internal/storage/migrations/ | tail -3` 确认）；写完 `atlas migrate hash` + `make -C apps/control-plane migrate-validate`；全部中文 COMMENT；UUID-first、tenant_id 开头索引、TIMESTAMPTZ、JSONB、status VARCHAR+应用层校验。
- 契约改动：openapi.yaml → `corepack pnpm generate:control-plane` + `corepack pnpm verify:contracts`；TS 客户端手写在 `apps/web/src/lib/api/`。
- 前端改动前读 `DESIGN.md`；内部跳转仅 TanStack Router `Link`/`navigate`；注册表管理页照 `features/mcp/index.tsx` 形态。
- 服务启停 `scripts/dev-services.sh`（主 checkout）；`restart control-plane` 自动跑迁移。
- **workflow.go 有改动的任务**：必须运行 `go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 并在报告附结果；禁止 GetVersion 事后围栏。
- 每任务收尾提交，commit 尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: 迁移 —— 词汇表、豁免表、标准员工模板种子

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/0NN_capability_vocabulary_exemptions_standard_employees.sql`（NN = 当前最大编号+1，先确认）
- Modify: `atlas.sum`（atlas hash）

**Interfaces:**
- Produces: 表 `capability_vocabulary(id, tenant_id, vocab_key, title, description, status, created_at, updated_at)`（`UNIQUE(tenant_id, vocab_key) WHERE deleted_at IS NULL` 风格随 058）；表 `project_demand_constraint_exemptions(id, tenant_id, project_id, demand_id, constraint_kind, roles JSONB, granted_by_user_id, decision_request_id, created_at)`（`UNIQUE(tenant_id, demand_id, constraint_kind)`）；`digital_employee_templates` 两行系统种子。

- [ ] **Step 1: 确认编号**：`ls apps/control-plane/internal/storage/migrations/ | tail -3`，取最大编号+1。
- [ ] **Step 2: 写迁移**。三段：

```sql
-- 能力词汇注册表：模板角色要求与员工能力声明共享的键词汇；场景差异走注册表不走代码枚举。
CREATE TABLE IF NOT EXISTS capability_vocabulary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    vocab_key TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_capability_vocabulary_key_not_blank CHECK (btrim(vocab_key) <> ''),
    CONSTRAINT ck_capability_vocabulary_status_supported CHECK (status IN ('active', 'disabled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_capability_vocabulary_tenant_key_active
    ON capability_vocabulary(tenant_id, vocab_key) WHERE deleted_at IS NULL;
```

（全部中文 COMMENT 逐列；触发器仿 058 的 `update_updated_at_column`。）种子：收录五个种子场景模板 spec v2 里出现的全部 `required_capabilities` 键（`code_implementation/code_review/test_execution/log.analysis/incident.triage`），tenant `00000000-0000-0000-0000-000000000001`，title 用中文（如 `code_review` → `代码审查`），`ON CONFLICT DO NOTHING`。

```sql
-- 治理约束豁免记录：人类负责人对单需求豁免某条模板约束的一等决策留痕；重规划时治理评估器消费。
CREATE TABLE IF NOT EXISTS project_demand_constraint_exemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    constraint_kind VARCHAR(64) NOT NULL,
    roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    granted_by_user_id UUID NOT NULL,
    decision_request_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_demand_constraint_exemption UNIQUE (tenant_id, demand_id, constraint_kind)
);
CREATE INDEX IF NOT EXISTS idx_demand_constraint_exemptions_tenant_demand
    ON project_demand_constraint_exemptions(tenant_id, demand_id);
```

标准员工模板种子（`digital_employee_templates`，`is_system=true`，`ON CONFLICT DO NOTHING`，固定 UUID 仿 058 种子风格）：
- type=`standard_code_reviewer`、label=`标准代码审查员`、default_role=`代码审查`、`capability_bindings='{"skills":[],"mcp_servers":[],"external_capabilities":["code_review"],"environment_variable_refs":[]}'`、`recommended_provider_types='["claude-code"]'`、persona_memory_markdown 一段简短审查员人格（独立审查、只审不改、结论须附证据指针）。
- type=`standard_tester`、label=`标准测试员`、default_role=`测试`、external_capabilities=`["test_execution"]`、其余同构。

- [ ] **Step 3: hash + validate + 应用**：`cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations && make migrate-validate`（无 docker 用 scratch-schema DEV_URL，P2a Task 1 报告有先例）→ `make migrate-up` → psql 抽查三表/两种子行存在。
- [ ] **Step 4: Commit** — `feat(db): 能力词汇表+约束豁免记录+标准员工模板种子`

---

### Task 2: 词汇表 Go 模块（TDD）

**Files:**
- Create: `apps/control-plane/internal/scenariotemplate/vocabulary.go`
- Create: `apps/control-plane/internal/storage/queries/capability_vocabulary.sql`（`ListCapabilityVocabulary :many`、`GetCapabilityVocabularyByKeys :many`——`WHERE tenant_id=$1 AND vocab_key = ANY($2::text[]) AND deleted_at IS NULL AND status='active'`）+ sqlc 再生成
- Test: `apps/control-plane/internal/scenariotemplate/vocabulary_test.go`

**Interfaces:**
- Produces:
```go
package scenariotemplate
type VocabularyEntry struct { Key, Title, Description, Status string }
type VocabularyRepository interface {
    ListVocabulary(ctx context.Context, tenantID uuid.UUID) ([]VocabularyEntry, error)
    ActiveKeys(ctx context.Context, tenantID uuid.UUID, keys []string) (map[string]bool, error)
}
// ValidateCapabilityKeys 返回 keys 中不在词汇表（active）里的键，全部存在返回 nil。
func (s *Service) ValidateCapabilityKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error)
```
`Service` 为既有 scenariotemplate.Service，增持 VocabularyRepository（app.go 装配处注入 pg 实现；nil 时 ValidateCapabilityKeys 直接放行并返回 nil——与既有 resolver-nil 模式一致）。

- [ ] **Step 1: 失败测试**：`TestValidateCapabilityKeysReportsUnknown`（fake repo：`code_review` active、`ghost` 不在 → 返回 `["ghost"]`）；`TestValidateCapabilityKeysNilRepoPasses`；`TestValidateCapabilityKeysTrimsAndDedupes`（`[" code_review ", "code_review"]` → nil）。
- [ ] **Step 2: RED** → **Step 3: 实现**（sqlc query + pg 实现 + service 方法 + app.go 注入）→ **Step 4: GREEN**（`go test ./internal/scenariotemplate/ -count=1`）→ **Step 5: Commit** — `feat(scenariotemplate): 能力词汇表注册与键校验`

---

### Task 3: 事实性可行性三档 —— 服务端分取代 LLM 自评裁决（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`（~:82-84 的置信度门整段替换；`selectionConfidenceThreshold` 保留但新增 `selectionScoreThreshold`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/template_governance.go`（低分降级注记）
- Test: `graph_validation_test.go` + `template_governance_test.go`

**Interfaces:**
- Consumes: `ApplyPlanningProfileScores` 已在校验前写入 `task.SelectionScore`（openai_compatible_planner.go:132/158 先于 Validate 调用——事实已核）。
- Produces: 语义——**删除** `SelectionConfidence` 硬门（LLM 值仅留 payload 参考）；新增 `selectionScoreThreshold(policy)`（读 `coordination_policy["selection_score_threshold"]`，默认 `40`，语义注释：能力维恒 40 分，低于阈值意味着角色/运行时/负载全线偏弱）；低于阈值不拒绝，由 governance 追加 `PlanConstraintNote{Kind:"low_feasibility", Message:"任务 <key> 选角事实性评分 <score> 低于阈值 <threshold>：缺失 <missing…>"}` 且 `plan.RequiresHumanReview=true`。HardFailures 语义不变（既有 ApplyPlanningProfileScores 已翻转审批）。

- [ ] **Step 1: 失败测试**：
  1. `TestValidatePlanNoLongerGatesOnLLMConfidence` — `SelectionConfidence:0.2`、`SelectionScore:85` → `ValidateRouteDecisionPlan` NoError（现状会 ErrNoSuitableEmployee——先红）。
  2. `TestGovernanceLowFeasibilityScoreAddsDegradeNote` — `SelectionScore:25` → note kind `low_feasibility` 含分数与阈值、`RequiresHumanReview=true`。
  3. `TestGovernanceHighScoreNoNote` — `SelectionScore:85` → 无 low_feasibility note。
  4. `TestSelectionScoreThresholdPolicyOverride` — policy `{"selection_score_threshold": 60}` → 55 分产生 note。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN + 全包**（`go test ./internal/workflow/projectcoordination/ -count=1`——预计有既有测试断言置信度门，按新语义更新其意图：改断言"不再拒绝"或改用 HardFailure fixture）→ **Step 5: replay 测试**（Global Constraints）→ **Step 6: Commit** — `feat(validation): 事实性可行性三档——服务端分裁决, LLM 置信度降级为参考`

---

### Task 4: 结构化缺口对象贯通（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/template_governance.go`（结构缺口错误携带 gap 结构）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`（`RejectDemandPlanningInput` 加 `Gap *PlanningGap`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`（`noSuitableEmployeeDiagnosis` 家族透传 gap；`rejectDemandPlanningByID` 带 gap）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:3234-3239`（coordination.blocked payload 加 `"gap"`）
- Test: 对应 *_test.go

**Interfaces:**
- Produces:
```go
type PlanningGap struct {
    ConstraintKind       string   `json:"constraint_kind"`            // role_independence
    Roles                []string `json:"roles,omitempty"`            // [reviewer developer]
    RequiredCapabilities []string `json:"required_capabilities,omitempty"` // 缺角色的 required_capabilities（词汇键）
    ActiveExecutorCount  int      `json:"active_executor_count"`
    Options              []string `json:"options"`                    // [restaff exempt lending]
}
```
- 结构缺口错误：`enforceRoleIndependence` 的 `ErrNoSuitableEmployee` 家族错误改为经 `temporal` 可序列化的 ApplicationError **details** 携带 PlanningGap（`temporal.NewNonRetryableApplicationError(msg, errTypeNoSuitableEmployee, err, gap)`——details 随错误跨 activity 边界序列化；workflow 侧 `appErr.Details(&gap)` 解出）。旧执行/无 details 的错误 → gap 为 nil，行为与现状一致（**新判别事实天然区分新旧，无需围栏**）。
- coordination.blocked payload 增 `"gap": gapMap`（nil 时缺省）；`RejectDemandPlanningInput.Gap *PlanningGap` 透传。

- [ ] **Step 1: 失败测试**：`TestStructuralGapErrorCarriesDetails`（enforce 后 `errors.As` + `Details(&gap)` 解出 kind/roles/count）；`TestRejectDemandPlanningPersistsGapPayload`（store 层：input 带 gap → 事件 payload["gap"] 含 constraint_kind）；`TestRejectDemandPlanningNilGapOmitsField`。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN + 全包 + replay 测试** → **Step 5: Commit** — `feat(coordination): 结构化缺口对象贯通拒绝通道`

---

### Task 5: planning_gap 决策请求 —— 缺口进 inbox、补员后重开重规划（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（`RejectDemandPlanning` 追加创建 DecisionRequest + inbox 投影；新方法 `ReopenProjectDemandForReplanning`）
- Modify: `apps/control-plane/internal/project/`（`validHumanDecision` 白名单加 `restaffed`；demand 重开 repository 方法 + 事件 `ProjectEventDemandReplanningReopened = "demand.replanning_reopened"`）
- Modify: `apps/control-plane/internal/inbox/service.go`（`DefaultActions` 按 item/decision type 支持自定义——`UpsertItemRequest.Actions` 非空时优先，`planning_gap` 传 `[{key:"restaffed",label:"已补员，重新规划"},{key:"rejected",label:"关闭"}]`；豁免 action 由 Task 6 追加）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`（`handleHumanDecisionSubmitted` 的 decisionType switch 新增 `case "planning_gap"`：decision==restaffed → 经 activity 重开 demand（failed→planning_pending，审计事件）并在**同一 workflow 内**直接复用 `handleDemandSubmitted` 的规划路径重新规划）
- Modify: `contracts/control-plane/openapi.yaml`（决策 enum 加 `restaffed`）+ codegen
- Test: 各层 *_test.go

**Interfaces:**
- Consumes: `RequestPlanRevisionReview` 的 DecisionRequest 创建三件套模式（project_store.go:1820-1880：`approvals.CreateRequest` + `AppendProjectEvent(ProjectEventDecisionRequested)` + `CreateDecisionRequest` + `inbox.UpsertProjectDecisionRequest`）；`ResolveDecision` 通用回收通道。
- Produces: `RejectDemandPlanning` 在置 failed 之后创建 `DecisionRequest{DecisionType:"planning_gap", TargetUserID:项目 human_owner, TitleSnapshot:"规划缺口：<诊断摘要>", ContextPayload 含 gap 结构}`（幂等：同 demand 已有 pending planning_gap 决策则跳过）；resolve `restaffed` → workflow 重开+重规划；resolve `rejected` → 决策关闭，demand 保持 failed。
- **Temporal 纪律**：planning_gap 的 DecisionRequest 只由新代码创建，旧历史不含此 decisionType 的信号 → switch 新 case 对旧历史不可达，天然安全；仍跑 replay 测试为证。

- [ ] **Step 1: 失败测试**：store 层 `TestRejectDemandPlanningCreatesPlanningGapDecision`（approval/decision/inbox 三件套 + ContextPayload 带 gap + 幂等）；project 层 `TestReopenProjectDemandForReplanning`（failed→planning_pending + 事件，非 failed 状态拒绝重开）；inbox 层 `TestUpsertPlanningGapDecisionUsesCustomActions`；workflow 层 `TestPlanningGapRestaffedTriggersReplan`（restaffed 信号 → 重开 activity 调用 + 第二次 planner 调用发生 + 无 signal_failed）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN + 全包 + replay + verify:contracts** → **Step 5: Commit** — `feat(coordination): 规划缺口决策请求进 inbox+补员后重开重规划`

---

### Task 6: 豁免一等决策记录 + 治理消费（TDD）

**Files:**
- Modify: `apps/control-plane/internal/project/`（豁免 repository：`CreateDemandConstraintExemption`/`ListDemandConstraintExemptions` + sqlc query 文件 `demand_constraint_exemptions.sql`）
- Modify: `apps/control-plane/internal/project/service.go`（`ResolveDecision` 对 planning_gap + decision==`exempted` 落豁免记录——constraint_kind/roles 取自 DecisionRequest 的 ContextPayload gap，granted_by 取决策人，decision_request_id 关联，然后走与 restaffed 相同的重开+重规划信号路径）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go`（`CoordinationSnapshot` 加 `DemandConstraintExemptions []DemandConstraintExemption`——`{ConstraintKind string; Roles []string}`）+ `project_store.go` 快照装载处按 demand 读取
- Modify: `template_governance.go`（`enforceRoleIndependence` 前查豁免：命中 kind+roles 匹配 → 跳过该约束并追加 `PlanConstraintNote{Kind:"exemption", Message:"约束 role_independence 已由人类负责人豁免（决策记录 <id>）"}`）
- Modify: `validHumanDecision` 白名单 + openapi enum 加 `exempted`；inbox planning_gap actions 追加 `{key:"exempted",label:"豁免约束并重规划"}`
- Test: 各层

**Interfaces:**
- Consumes: Task 1 豁免表、Task 4 gap 结构（ContextPayload 里的 constraint_kind/roles）、Task 5 的重开重规划通路与 planning_gap actions。
- Produces: 豁免记录持久化且重规划时 role_independence 被跳过（带 note 可见）；豁免作用域 = 单需求（表 UNIQUE 约束保证幂等）。

- [ ] **Step 1: 失败测试**：`TestResolveExemptedCreatesExemptionRecord`（记录字段 + 关联 decision_request_id + 触发重规划信号）；`TestGovernanceSkipsExemptedRoleIndependence`（snapshot 带豁免 → 单员工同角色 NoError + exemption note）；`TestGovernanceExemptionScopedByKind`（豁免 kind 不匹配 → 照常拒绝）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN + 全包 + replay + verify:contracts** → **Step 5: Commit** — `feat(project): 约束豁免一等决策记录+治理消费`

---

### Task 7: Web —— workflows 直链可达性最小修复（预存在缺陷，缺口 UX 的地基）

**Files:**
- Modify: `apps/web/src/features/workflows/index.tsx:35-71`
- Test: 就近组件测试

**Interfaces:**
- Consumes: `getProjectDemandLaunchDetail(options, demandId)`（`projects.ts:1327`，by-id 端点已存在）。
- Produces: `/workflows/{demandId}` 直链不再依赖首页 50 条列表命中：`selectedDemandId` 直接取路由参数；detail/graph 查询按 id 拉取；**仅当 by-id detail 真 404**（`detailQuery.isError`）才回退重定向到列表第一条。列表命中的 instance 仅用于头部展示（optional）。

- [ ] **Step 1: 失败测试**：fixture——列表返回 50 条不含目标 demandId、by-id detail 返回正常 → 断言渲染 detail（现状会重定向，先红）；by-id 404 → 重定向行为保留。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN**（targeted + workflows 套件）→ **Step 5: Commit** — `fix(web): workflows 直链按 id 拉取, 不再被首页列表命中挟持`

---

### Task 8: Web —— 缺口面板 + 一键补员对话框 + 豁免动作

**改动前必读 `DESIGN.md`。**

**Files:**
- Modify: `apps/web/src/features/workflows/index.tsx`（BlockingBanner 下方渲染缺口操作区——当 blocking fact 携带 gap 时）
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`（「当前需求」卡对 failed demand 增诊断摘要行 + 跳转缺口处理入口——不再只有裸「失败」badge）
- Create: `apps/web/src/features/projects/components/staff-gap-dialog.tsx`（补员对话框）
- Modify: `apps/web/src/lib/api/projects.ts`（blocking fact 类型加 `gap?`；resolve 决策 body 支持 `restaffed`/`exempted`）
- Modify: 服务端 blocking fact 透传：`apps/control-plane/internal/project/service.go:2001-2026`（`projectTaskGraphBlockingFactFromEvent` 提取 payload["gap"]）+ `handler.go:2243-2250` response struct + openapi
- Test: 组件测试（vitest browser）

**Interfaces:**
- Consumes: Task 4 的 payload.gap、Task 5/6 的 planning_gap 决策与 actions、探查确认的组合链——`createDigitalEmployee`（`lib/api/employees.ts`，body 带 `employee_type: "standard_code_reviewer"`、`team_id`、`provider_type`）+ `PUT /api/v1/projects/{projectId}/members`（成员追加，读改写现有成员列表）。
- Produces: 缺口面板三动作：**从标准模板补员**（对话框：列 `is_system` 员工模板 → 选择 + 命名 + provider 默认 claude-code → 创建 → 追加进项目成员 → 自动 resolve planning_gap 决策为 `restaffed` → 提示重规划已触发）；**豁免并重规划**（确认弹层说明后果 → resolve `exempted`）；**发起借调**（`Link` 到项目借调入口，不实现新流程）。inbox 里同名 actions 由 Task 5/6 已通（inbox-shell 动态渲染 `item.actions`）。

- [ ] **Step 1: 失败组件测试**：blocking fact 带 gap fixture → 面板可见三动作；补员对话框选模板提交 → 断言 createDigitalEmployee 与 members PUT 与 resolve 依次调用（mock 层）；无 gap → 无面板。
- [ ] **Step 2: RED** → **Step 3: 实现**（后端透传先行，再前端）→ **Step 4: GREEN**（targeted + 全量串行）+ `verify:contracts` → **Step 5: Commit** — `feat(web): 规划缺口面板——一键补员/豁免重规划/借调入口`

---

### Task 9:【GATE·生死判据】补员闭环真实 E2E —— 实例化必须真正可派发

**Files:** 无代码；证据追加到本文件"实施记录"。

**前置：** Task 1–8 合入分支；dev-services 全绿（control-plane restart 加载分支码）。

- [ ] **Step 1: 制造缺口**：单员工 software_delivery 项目提"合入"需求 → 一轮终局 failed + **inbox 出现 planning_gap 决策项**（actions 含 补员/豁免）+ workflows 直链（Task 7 修复后）显示缺口面板。
- [ ] **Step 2: 一键补员**：面板补员对话框 → 从 `standard_code_reviewer` 实例化（人确认）→ 验证：员工出现在项目成员池；**调度就绪**——`GET /digital-employees/{id}/scheduling-readiness` ready，且规划画像 `provider_status` 就绪（新员工在 dev 环境需真实 runtime/provider 绑定：local-dev-node + claude-code；若创建流不产生可执行 execution instance，按 07-15 已知事实 runtime 现绑项目，走真实项目派发路径验证而非直插 DB）。
- [ ] **Step 3: 判决点**：resolve `restaffed` → demand 重开（failed→planning_pending + 事件）→ 真实 planner 重规划 → 计划含独立审查任务且 reviewer 绑定**新员工** → pending_review → 批准 → 审查任务真实派发到新员工并进入 running。**PASS = 新员工真实接到任务开始执行**；FAIL（如新员工 dispatch not_ready 卡死）→ 停止，回评审补员就绪链路缺口，不得带病推进。
- [ ] **Step 4: 豁免路径**：另一个单员工项目同样制造缺口 → resolve `exempted` → 豁免记录落库可查 → 重规划通过（同员工身兼开发审查 + exemption note 在确认卡约束说明可见）→ 批准派发。
- [ ] **Step 5: 记录 + Commit** — `docs(plan): P2b 补员闭环收敛闸门记录`

---

### Task 10: 场景模板管理 API（TDD）

**Files:**
- Modify: `apps/control-plane/internal/scenariotemplate/{types,service,pg_repository,handler}.go`（新增写方法族）
- Modify: `apps/control-plane/internal/storage/queries/scenario_template.sql`（`CreateScenarioTemplate :one`、`CreateScenarioTemplateVersion :one`、`UpdateScenarioTemplateActiveSpec :one`、`UpdateScenarioTemplateStatus :one`）+ sqlc
- Modify: `apps/control-plane/internal/api/server.go:454-457`（追加 `r.Post("/scenario-templates", …)`、`r.Post("/scenario-templates/{templateKey}/versions", …)`、`r.Patch("/scenario-templates/{templateKey}", …)`）
- Modify: `contracts/control-plane/openapi.yaml` + codegen；`apps/web/src/lib/api/scenario-templates.ts`（create/version/patch 手写客户端）
- Modify: app.go 装配（audit service 注入 scenariotemplate.Service，仿 employee run_service 模式）
- Test: handler/service 层

**Interfaces:**
- Consumes: `ActionScenarioTemplateManage`（authz/types.go:60 已存在已授权未接线）；`ParseSpec`（写入前校验）；Task 2 `ValidateCapabilityKeys`（spec.roles[].required_capabilities 必须在词汇表）；`audit.Service.RecordEvent`（resourceType `scenario_template`）。
- Produces: `POST /api/v1/scenario-templates`（建模板=建 v1，spec 经 ParseSpec+词汇校验，主表 spec 镜像+active_version=1+版本行）；`POST /{key}/versions`（升版：校验→版本行 version+1→主表镜像+active_version 更新）；`GET /{key}/versions`（只读版本历史列表，Task 11 展开区消费）；`PATCH /{key}`（status active/disabled 与 name/description）。每个写操作发 domain 审计事件（action=create/version/status，Details 带 diff 摘要）。错误映射仿 capability `writeHandlerError`（无效 spec/词汇缺键→400 带缺键列表、key 冲突→409）。
- 进行中计划不受升版影响的语义由 P2a 的 payload 钉版已保证（本任务不动规划侧）。

- [ ] **Step 1: 失败测试**：service 层 `TestCreateScenarioTemplateValidatesSpecAndVocabulary`（坏 spec→err；含 `ghost` 能力→err 带键名；好 spec→主表+版本行+审计事件）；`TestCreateVersionBumpsActiveAndMirrorsSpec`；`TestPatchStatusDisabled`（disabled 后规划解析回落 generic——复用既有 adapter 的 `Status != "active"` 行为断言）；handler 层 authz manage 403 用例。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN + verify:contracts** → **Step 5: Commit** — `feat(scenariotemplate): 管理 API——建模板/升版/状态, 词汇校验+审计`

---

### Task 11: Console 管理页升级

**改动前必读 `DESIGN.md`。照抄 `features/mcp/index.tsx` 形态（内联 WorkSurface 表单 + ConfirmDialog + V3Table）。**

**Files:**
- Modify: `apps/web/src/features/scenario-templates/index.tsx`（只读目录页升级：新建模板表单（key/name/description/spec JSON textarea + 服务端校验错误展示 `ApiRequestError.detail`）、行内「升版」动作（预填当前 spec 的 JSON 编辑）、状态 toggle（ConfirmDialog）、展开区加版本历史（消费 Task 10 的 `GET /{key}/versions`））
- Test: `apps/web/src/features/scenario-templates/index.test.tsx` 扩展

**Interfaces:**
- Consumes: Task 10 的三个写端点 + 手写客户端。

- [ ] **Step 1: 失败测试**：新建表单提交调用 create 客户端；升版动作预填并提交；disabled toggle 经确认弹层；服务端 400 的 detail 文本渲染。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: GREEN**（targeted + 全量串行）→ **Step 5: Commit** — `feat(web): 场景模板管理页——新建/升版/状态/版本历史`

---

### Task 12: 收尾 —— 门禁 + 真实端到端 + 完成检查

- [ ] **Step 1**: `corepack pnpm verify:control-plane` + `corepack pnpm --filter @superteam/web test -- --no-file-parallelism` + typecheck + build。
- [ ] **Step 2**: 真实端到端回归（dev-services 分支码）：① 管理页真实新建一个模板（含一个故意的词汇缺键先看到 400 detail，再改对提交成功）→ 项目绑定它提需求 → 按其骨架规划；② 升版该模板（改一条约束）→ 存量 pending_review 计划 payload 版本不变、新需求用新版本；③ Task 9 两条闭环抽一条快速复跑（补员或豁免）；④ generic/无模板回归。逐条记录到"实施记录"。
- [ ] **Step 3**: `$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）；CHANGELOG 条目（`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`）。
- [ ] **Step 4**: Commit + 向人类汇报（含遗留与 P3/intent 层衔接点）。

---

## 实施记录

（实施时追加。）
