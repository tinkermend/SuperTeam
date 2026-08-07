# 意图与验收标准 Phase 1 实现计划

> 复核状态：已实施，现状与配对spec一致（10任务全完成，main merge 77293480；计划文件内"实施记录"未回填但CHANGELOG 07-16 16:04条目为完整口径）。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 判据获得语义（谁判/凭什么/阻断与否）并经人类确认；逐条 verdict 可查询持久化（automated_test 判据强制 attestation 证据）；人类判据经 demand_acceptance 决策通道签署；需求完成前过收敛闸；血缘全链可点。

**Architecture:** 全部挂在已 E2E 验证的钩子上：`PlanAcceptanceCriterion`（plan_revision_payload.go:49，已版本化+指纹+人类确认）补语义字段；planner prompt L311/L315 判据指令扩展；分解时判据快照落表 + 注入任务 handoff_contract（既有 `TaskResultAcceptanceResult.CriterionID` 契约直接可用）；结果落库处投影 verdict 行；收敛闸插 `recomputeProjectDemandStatusWithQueries`（pg_repository.go:4676 completed 分支前）经新状态 `acceptance_pending` 停靠；`demand_acceptance` 决策仿 P2b planning_gap 三件套；签署端点全部满足→completed / 有不满足→failed+结构化事件（外环返工预留输入）。

**Tech Stack:** Go (chi + sqlc + Temporal) / Atlas / OpenAPI (Go server 生成, TS 手写) / React + TanStack Router + vitest browser。

**Spec:** `docs/superpowers/specs/2026-07-16-intent-acceptance-criteria-full-design.md`。待决策项在此拍板：#1 兜底人类判据豁免走项目 `coordination_policy["acceptance_human_judgment_exempt"]`（bool）；#2 签署界面内嵌 workflows 需求详情（acceptance_pending 态显示签署区）；#3 判据闸 mode 无关（loop 需求产生人类判据照常进 inbox，节奏归 Loop 信封 spec）。

**关键设计落点（探查确定）：**
- 需求完成态由 `recomputeProjectDemandStatusWithQueries`（pg_repository.go:4676）从任务计数派生（L4690-4697）——收敛闸插在 completed 判定前，未签阻断人判 → 新状态 `acceptance_pending`（rank 介于 executing 与终态之间；status 是 VARCHAR+应用层注册，加值合法）。
- 判据快照落表（分解时）：收敛闸/血缘面板/签署端点全部查表，不解析 payload JSONB。
- `DecisionRequest` 无 DemandID 字段——沿用 P2b 模式：approval `GetRequestByResource(resourceType="project_demand_acceptance", resourceID=demandID)` 幂等 + ContextPayload 带 demand_id。
- attestation 字符串引用约定 `"attestation:"` 前缀已存在（executor.rs L1998 生成、service.go:3180 结构化匹配）——对 `AcceptanceResult.EvidenceRefs []string` 需新写纯字符串前缀判别 helper。
- 迁移编号 **064**（当前最大 063；并发会话可能占用——Task 3 先确认）。

## Global Constraints

- 根级命令 `corepack pnpm <script>`；Go 定向 `cd apps/control-plane && go test ./internal/...`；Web `corepack pnpm --filter @superteam/web test -- --no-file-parallelism`；禁止 npx。
- 迁移：编号以当前最大顺延；atlas hash + `make -C apps/control-plane migrate-validate`；全中文 COMMENT；DATABASE_DESIGN.md 规则。
- 契约改动：openapi → `corepack pnpm generate:control-plane` + `verify:contracts`；TS 客户端手写。
- 前端改动前读 DESIGN.md；内部跳转仅 TanStack Link/navigate。
- **workflow.go 有变更的任务必须跑 `go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 并报结果；禁止事后 GetVersion 围栏（新判别事实驱动新分支）。**
- 每任务提交，尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 共享 checkout 纪律：只在 feat/intent-acceptance-p1 分支工作；发现分支异位即 STOP 报 BLOCKED。

---

### Task 1: 判据语义字段贯通（planner→校验→payload，TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go:49-53`（结构体扩展）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（:311 判据指令行扩展、:585-589 decode 结构体、:598-602 映射）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`（判据校验扩展）
- Create: `apps/control-plane/internal/workflow/projectcoordination/acceptance_criteria.go`（注册表、兜底注入、歧义启发式）
- Test: 对应 *_test.go

**Interfaces:**
- Produces（后续任务按此消费）:
```go
type PlanAcceptanceCriterion struct {
    ID                 string   `json:"id"`
    Statement          string   `json:"statement"`
    SatisfiedBy        []string `json:"satisfied_by"`
    VerificationMethod string   `json:"verification_method,omitempty"` // 缺省读取端补 automated_test
    Severity           string   `json:"severity,omitempty"`            // 缺省 blocking
    EvidenceHint       string   `json:"evidence_hint,omitempty"`
    AmbiguityFlag      bool     `json:"ambiguity_flag,omitempty"`
}
// acceptance_criteria.go
var knownVerificationMethods = map[string]bool{"automated_test": true, "human_judgment": true} // 注册表：新增方法=加判定通道
const (VerificationMethodAutomatedTest = "automated_test"; VerificationMethodHumanJudgment = "human_judgment"
       CriterionSeverityBlocking = "blocking"; CriterionSeverityNonBlocking = "non_blocking")
func normalizeCriterionDefaults(c *PlanAcceptanceCriterion)                  // method/severity 缺省
func ensureHumanJudgmentCriterion(plan *RouteDecisionPlan, policy map[string]any) // 兜底注入（policy["acceptance_human_judgment_exempt"]==true 豁免）
func markAmbiguousCriteria(plan *RouteDecisionPlan)                           // 启发式置 AmbiguityFlag
func validateAcceptanceCriteriaSemantics(plan RouteDecisionPlan) error        // method 注册表内; automated_test 的 SatisfiedBy 非空; severity 合法
```
- 兜底判据：`{ID:"human_final_confirmation", Statement:"人类负责人确认交付符合需求意图", VerificationMethod:human_judgment, Severity:blocking, SatisfiedBy:nil}`；仅当计划无任何 human_judgment 判据且未豁免时注入。
- 歧义启发式：statement TrimSpace 后 <8 字符，或含模糊词（尽量/适当/合理/优化一下/等等/大概）→ AmbiguityFlag=true（只标注不拒绝）。
- prompt 扩展（:311 行后追加一行，风格一致的英文指令）：每条判据须声明 `verification_method`（automated_test：可由命令/测试证据判定，satisfied_by 必填；human_judgment：业务/意图判断，satisfied_by 可空）与 `severity`（blocking 缺省）；statement 必须是可判定断言（不用"尽量、适当"）。
- 接线：planner decode 后（映射 :598-602 处）依次 normalize → ensureHumanJudgment → markAmbiguous；`validateAcceptanceCriteriaSemantics` 挂进 `ValidateRouteDecisionPlan`（既有 PlanAcceptanceCriteria satisfied_by 校验旁）。
- 指纹：新字段随 payload 自然入指纹（canonicalPlanRevisionPayload 复制整个 criteria 切片——确认后在测试断言）。

- [ ] **Step 1: 失败测试**：`TestNormalizeCriterionDefaults`（空 method→automated_test、空 severity→blocking）；`TestEnsureHumanJudgmentInjectsFallback`（无人判→注入一条；已有→不注入；policy 豁免→不注入）；`TestMarkAmbiguousCriteria`（"尽量优化性能"→flag；"登录失败返回 401"→无 flag）；`TestValidateCriteriaSemanticsRejectsUnknownMethod`、`TestValidateCriteriaSemanticsAutomatedRequiresSatisfiedBy`；`TestCanonicalPlanFingerprintSensitiveToCriterionMethod`（同判据仅 method 不同→指纹不同）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay 测试**（planner 管线变更）→ **Step 5: Commit** — `feat(planner): 验收判据语义化——verification_method/severity/兜底人判/歧义标注`

---

### Task 2: 确认卡判据语义化（web）

**改动前必读 DESIGN.md。**

**Files:**
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`（"验收判据" panel :590-622、`PlanAcceptanceCriterionStatement` :1677、字段提取 :2027/:2042 区域）
- Test: `project-operational-detail.test.tsx`

**Interfaces:**
- Consumes: payload criteria 新字段（Task 1 json tags）。
- Produces: 每条判据显示 method 徽标（自动验证/人类判定）+ severity 徽标（阻断为默认不显、non_blocking 显"非阻断"）+ AmbiguityFlag 黄标（文案"断言可能不可判定，请改写后再批准"）；human_judgment 且 SatisfiedBy 空 → "满足任务"行显示"需求级人类判定"。存量 payload 无新字段 → 视觉与现状一致（缺省徽标 自动验证）。

- [ ] **Step 1: 失败组件测试**（fixture 带四种判据：automated/human/non_blocking/ambiguous → 断言徽标与黄标文本；legacy fixture 无字段 → 无黄标、默认徽标）→ **Step 2: RED** → **Step 3: 实现** → **Step 4: targeted+全量串行+typecheck** → **Step 5: Commit** — `feat(web): 确认卡判据语义徽标+歧义黄标`

---

### Task 3: 迁移 064 —— 判据快照表 + verdict 表

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/0NN_demand_acceptance_criteria_verdicts.sql`（NN=当前最大+1，先 `ls migrations/*.sql | tail -1` 确认）
- Modify: atlas.sum

**Interfaces（后续任务消费的表结构）:**

```sql
-- 需求判据快照：计划批准分解时从 payload 固化，收敛闸/血缘/签署全部查表不解析 JSONB。
CREATE TABLE IF NOT EXISTS demand_acceptance_criteria (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL, project_id UUID NOT NULL, demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,          -- payload 内 ID
    statement TEXT NOT NULL,
    verification_method VARCHAR(64) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    satisfied_by JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_demand_criteria_revision UNIQUE (tenant_id, demand_id, plan_revision_id, criterion_id)
);
CREATE INDEX IF NOT EXISTS idx_demand_acceptance_criteria_tenant_demand
    ON demand_acceptance_criteria(tenant_id, demand_id, plan_revision_id);

-- 逐条判据判定记录：executor 投影 + 人类签署两来源。
CREATE TABLE IF NOT EXISTS demand_criterion_verdicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL, project_id UUID NOT NULL, demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,
    verdict VARCHAR(32) NOT NULL,        -- satisfied | unsatisfied
    judge_type VARCHAR(32) NOT NULL,     -- executor | human
    judge_id UUID NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    project_task_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_task
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id, project_task_id)
    WHERE project_task_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_human
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL;
```

全中文 COMMENT 逐表逐列；`project_demands.status` 列 COMMENT 追加 `acceptance_pending` 值说明。

- [ ] **Step 1: 确认编号写迁移** → **Step 2: atlas hash + migrate-validate（scratch-schema DEV_URL 先例见 .superpowers/sdd 报告）+ migrate-up + psql 抽查两表** → **Step 3: Commit** — `feat(db): 需求判据快照表+逐条verdict表 (迁移0NN)`

---

### Task 4: 分解时快照落表 + 判据注入任务契约（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（`DecomposeAcceptedPlanRevision` 路径——定位 `rg -n "DecomposeAcceptedPlanRevision" --glob '!*_test.go'`）
- Modify: `apps/control-plane/internal/project/`（repository：`CreateDemandAcceptanceCriteria`(批量)、`ListDemandAcceptanceCriteria(ctx, tenantID, demandID, planRevisionID)`；sqlc 新查询文件 `demand_acceptance.sql`）
- Test: store + repository 层

**Interfaces:**
- Consumes: Task 1 语义字段、Task 3 表。
- Produces:
  1. 分解已批准计划时：payload criteria（normalize 缺省后）批量写入 `demand_acceptance_criteria`（幂等：UNIQUE + ON CONFLICT DO NOTHING）。
  2. **判据 ID 注入任务契约**：对每条 automated_test 判据，其 `SatisfiedBy` 中每个任务的 `handoff_contract["acceptance_criteria"]` 追加对象 `{"criterion_id": <ID>, "criterion": <Statement>}`（既有契约 `requiredAcceptanceCriteria`/`matchesCriterion` 按 criterion/criterion_id/id/name 匹配——task_result_contract.go:607/623——对象形带 required 键语义已兼容；追加不覆盖 planner 自带的任务级判据）。human_judgment 判据不注入任务（人判归人）。
  3. domain type `project.DemandAcceptanceCriterion{CriterionID, Statement, VerificationMethod, Severity, SatisfiedBy []string, PlanRevisionID}`。

- [ ] **Step 1: 失败测试**：`TestDecomposePersistsCriteriaSnapshot`（分解后快照行=payload 条数、字段忠实）；`TestDecomposeInjectsCriterionIDsIntoHandoffContracts`（satisfied_by 任务的 handoff_contract 含 criterion_id 对象；human_judgment 不注入；非 satisfied_by 任务不注入）；`TestDecomposeSnapshotIdempotent`（重复分解不重复行）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay 测试** → **Step 5: Commit** — `feat(coordination): 分解时判据快照落表+criterion_id注入任务契约`

---

### Task 5: executor verdict 投影 + automated_test attestation 收紧（TDD）

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`（`recordProjectTaskAttemptResult` :3068 路径投影；校验收紧）
- Modify: `apps/control-plane/internal/project/task_result_contract.go`（纯字符串 attestation 判别 helper）
- Modify: repository（`CreateDemandCriterionVerdict`、`ListDemandCriterionVerdicts(ctx, tenantID, demandID, planRevisionID)`；sqlc）
- Test: service/contract 层

**Interfaces:**
- Consumes: Task 4 快照（按 demand+revision 取 criteria 定 method）；`TaskResultAcceptanceResult{CriterionID, Status, EvidenceRefs []string}`（既有）。
- Produces:
```go
func stringRefIsAttestation(ref string) bool // strings.HasPrefix(TrimSpace, "attestation:")
```
  - 投影规则（结果契约校验通过、结果落库同路径内）：对 contract.AcceptanceResults 中能按 criterion_id 匹配到快照 automated_test 判据的每条 → verdict 行（verdict=passed 家族映射 satisfied / failed 映射 unsatisfied；judge_type=executor，judge_id=员工 id，evidence_refs 原样，task_id=本任务）；ON CONFLICT（partial unique）幂等覆盖跳过。匹配不到快照的（任务级 planner 自带判据）不投影——现状行为不变。
  - **收紧**：匹配到快照 automated_test 判据且 Status∈{passed, human_overridden} 的 result，其 EvidenceRefs 必须至少一条 `stringRefIsAttestation` → 否则任务结果校验失败（错误码 `acceptance_result_attestation_required:<criterion_id>`，走既有 rejected+waitHuman）。human_judgment 判据若被员工自报 → 投影时忽略 + 事件留痕（`AppendProjectEvent` 轻量 warn 事件或日志，选日志即可 Phase 1）。
  - 存量护栏：demand 无快照行（旧计划/未走新分解）→ 投影与收紧整体跳过，行为与现状逐字节一致。

- [ ] **Step 1: 失败测试**：`TestRecordResultProjectsCriterionVerdicts`（匹配快照→行字段正确）；`TestAutomatedCriterionRequiresAttestationEvidence`（无 attestation: 前缀→校验失败带错误码；有→过）；`TestHumanJudgmentSelfReportIgnored`；`TestProjectionSkippedWithoutSnapshot`（存量护栏）；`TestStringRefIsAttestation`。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包**（project + projectcoordination）→ **Step 5: Commit** — `feat(project): 判据verdict投影+automated_test强制attestation证据`

---

### Task 6: acceptance_pending 状态 + 收敛闸 + demand_acceptance 决策（TDD）

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`（状态常量 `ProjectDemandStatusAcceptancePending = "acceptance_pending"` + rank 插入 executing 与终态之间 :267-283；web 状态标签由 Task 7 一并）
- Modify: `apps/control-plane/internal/project/pg_repository.go:4676-4697`（`recomputeProjectDemandStatusWithQueries`：counts 判 completed 前查判据闸——存在快照且有 blocking 判据缺 satisfied verdict → `acceptance_pending`；全部满足 → completed 照旧；无快照 → 现状路径）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（`ensureDemandAcceptanceDecision`：仿 planning_gap 三件套 + `GetRequestByResource("project_demand_acceptance", demandID)` 幂等；`ensureFinalDemandSummary` 触发扩展到 acceptance_pending）
- Modify: 触发点：结果写回后 demand 状态为 acceptance_pending → coordinator/service 侧确保决策创建（探查定位 recompute 调用点所在写回链的上层——service 或 store 收到新状态后调用；与 P2b RejectDemandPlanning 同层）
- Test: repository + store 层

**Interfaces:**
- Consumes: Task 3/4/5 表与投影。
- Produces: 收敛闸语义（blocking 判据按当前生效 plan_revision 查 verdict：`CountUnsatisfiedBlockingCriteria(ctx, tenantID, demandID, planRevisionID) (missing int, err)`——快照 LEFT JOIN verdict WHERE severity=blocking AND 无 satisfied 行）；`demand_acceptance` DecisionRequest（TargetUserID=human_owner，Title="需求验收：<需求标题>"，ContextPayload{demand_id, plan_revision_id, pending_criteria[]}），inbox item 无快捷动作仅深链（DecisionActions 对 demand_acceptance 返回空动作数组——inbox 渲染纯深链项；确认深链路由 `/workflows/{demandId}`）。
- 注意：non_blocking 判据不入闸；`acceptance_pending` 的需求不算 `AreAllProjectDemandsTerminal`（项目级验收等它——rank 设计已保证非终态）。

- [ ] **Step 1: 失败测试**：`TestRecomputeHoldsAtAcceptancePendingWhenBlockingUnsigned`（任务全完+人判未签→acceptance_pending）；`TestRecomputeCompletesWhenAllBlockingSatisfied`；`TestRecomputeLegacyDemandWithoutSnapshotCompletes`（护栏）；`TestEnsureDemandAcceptanceDecisionIdempotent`（三件套+幂等+ContextPayload 字段）；rank 前向守卫用例（executing→acceptance_pending→completed 合法；acceptance_pending→executing 非法）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay 测试**（store/coordinator 变更）→ **Step 5: Commit** — `feat(project): acceptance_pending收敛闸+demand_acceptance决策三件套`

---

### Task 7: 签署端点 + 完成/驳回语义（TDD）

**Files:**
- Modify: `apps/control-plane/internal/project/service.go` + `handler.go`（`POST /api/v1/project-demands/{demandId}/criterion-verdicts`）
- Modify: `contracts/control-plane/openapi.yaml` + codegen + TS 客户端（`apps/web/src/lib/api/projects.ts` 新函数 `signDemandCriterionVerdict`）
- Modify: web 状态标签（demandStatusLabel 增 acceptance_pending→待验收）
- Test: service/handler 层

**Interfaces:**
- Consumes: Task 6 决策与闸；Task 3 verdict 表 human partial unique。
- Produces: 请求体 `{criterion_id, verdict: "satisfied"|"unsatisfied", reason}`；前置校验：demand 状态 acceptance_pending、存在 pending demand_acceptance 决策、签署人=决策 TargetUserID（或项目 human_owner——取决于既有 decision resolve 的授权模式，照抄）、criterion 是本 revision 的 human_judgment 判据且未签。写 human verdict 行后：
  - 仍有未签 blocking human_judgment → 返回进度（signed/total）。
  - 全部 blocking 满足 → resolve 决策 approved + demand advance completed + 事件 `demand.acceptance_completed`。
  - 本次 verdict=unsatisfied（blocking）→ resolve 决策 rejected + demand → failed + 事件 `demand.acceptance_rejected` payload 含 `{criterion_id, statement, reason}`（结构化，外环返工预留）+ 剩余未签判据不再要求。
  - 幂等：同判据重复同值签 → 200 幂等；改值 → 409（Phase 1 不支持改判）。

- [ ] **Step 1: 失败测试**：签署推进/完成/驳回三主径 + 越权 403 + 非 human_judgment 判据 400 + 重复签幂等/改值 409 + 非 acceptance_pending 状态 409。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + verify:contracts** → **Step 5: Commit** — `feat(project): 判据签署端点——全签完成/驳回落结构化事件`

---

### Task 8: Web —— 判据血缘面板 + 签署界面

**改动前必读 DESIGN.md。**

**Files:**
- Modify: `apps/web/src/features/workflows/index.tsx` + 新组件 `apps/web/src/features/workflows/components/criteria-panel.tsx`
- Modify: `apps/web/src/lib/api/projects.ts`（读端点：criteria+verdicts 汇总——Task 7 已建 TS 签署函数；读端点在本任务补 openapi+handler `GET /api/v1/project-demands/{demandId}/acceptance-criteria`，返回快照+verdict 汇总）
- Test: 组件测试

**Interfaces:**
- Consumes: Task 3-7 全部。
- Produces: 需求详情判据面板——逐条 statement/method/severity 徽标/verdict 状态（satisfied 绿/unsatisfied 红/待判灰）/判定者/证据链接（attestation ref → 既有 transcript/attempt 证据深链，复用现有证据展示路由——探查任务内确认最近的深链形态并复用）；demand 状态 acceptance_pending 时人判判据行内显示签署控件（满足/不满足+理由输入 → signDemandCriterionVerdict → 失效查询刷新；全签完成后面板转只读+demand 徽标变已完成）。评审防橡皮图章：签署控件紧邻该判据的证据汇总（satisfied_by 任务的产出摘要——快照 satisfied_by + 任务结果 summary，读端点一并返回）。

- [ ] **Step 1: 失败组件测试**（fixture：三判据两 verdict → 面板渲染/徽标/签署控件仅 acceptance_pending+human_judgment 未签行显示；签署提交调用客户端）→ **Step 2: RED** → **Step 3: 实现**（后端读端点先行+Go 测试，再前端）→ **Step 4: targeted+全量串行+typecheck+verify:contracts** → **Step 5: Commit** — `feat(web): 判据血缘面板+人类签署界面`

---

### Task 9:【GATE】全链血缘真实 E2E

**Files:** 无代码；证据入本文件实施记录。

前置：Task 1-8 合入分支，control-plane 重启加载分支码。已知环境事实：deliverables.value 为 object 的写回会 400（韧性家族缺陷未修）——E2E 需求措辞避免诱导 object 交付物。

- [ ] **Step 1 判据生成与确认**：software_delivery 双员工项目提"合入"需求 → 确认卡判据区显示模板判据（automated_test 徽标）+ 兜底人类判据（human_judgment/blocking）→ 批准。
- [ ] **Step 2 executor verdict**：任务真实执行完成 → psql `demand_criterion_verdicts` 出现 executor 行且 evidence_refs 含 `attestation:` 引用；判据面板显示绿。
- [ ] **Step 3 收敛闸+签署**：任务全完 → demand `acceptance_pending`（不是 completed）→ inbox 出现需求验收深链项 → 判据面板签署控件：签"满足" → demand completed + `demand.acceptance_completed` 事件；血缘全链点击验证：判据 → verdict → attestation 证据深链可达。
- [ ] **Step 4 驳回径**：第二条需求同流程签"不满足"+理由 → demand failed + `demand.acceptance_rejected` 事件 payload 含 criterion_id/reason（psql）。
- [ ] **Step 5 判决点**：以上全链无 mock、无手工 DB 干预（除查证）。FAIL → 停止回评审。记录 → 实施记录 + Commit `docs(plan): intent P1 血缘闸门记录`。

---

### Task 10: 收尾 —— 门禁 + 残余场景 + 完成检查

- [ ] **Step 1**: `corepack pnpm verify:control-plane` + web 全量串行 + typecheck + build。
- [ ] **Step 2**: 残余 E2E：attestation 缺失拒绝（构造无 attestation 证据的结果 fixture 或真实诱导）；歧义黄标（"尽量优化性能"需求）；存量回归（Policy 豁免 + 模板无判据 → 行为与现状一致，demand 直接 completed）。
- [ ] **Step 3**: `$superteam-completion-check`；CHANGELOG（`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`）；实施记录；memory 更新。
- [ ] **Step 4**: Commit + 汇报（含 Phase 2/3 与外环衔接点、遗留）。

---

## 实施记录

（实施时追加。）
