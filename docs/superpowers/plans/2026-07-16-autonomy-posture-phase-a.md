# 自治姿态校准 Phase A（默认翻转）实现计划

> 复核状态：**部分撤销（2026-08-07）**。Task 1/3 仍是当前默认：兜底人类判据从"每需求强制注入"改为"仅策略 require_human_acceptance 或高风险注入"，安全属性端到端成立，未被 07-17 方向修正影响（修正只针对档2对抗式评判）。
>
> **Task 2（确认密度接出口深度）已于 2026-08-07 撤销**，`planRequiresHumanConfirmation` / `planExitAtOrBeyondConfirmationDepth` 两个函数已从 `project_store.go` 删除。**plan 模式现在无条件停下等人类确认**。撤销理由（人类决策，详见 `docs/superpowers/specs/2026-08-07-task-hub-ux-remediation-design.md` §12.5）：
>
> 1. 该判据读的 `RequiresHumanReview` / `RequiresHumanApproval` / `RiskLevel` 全部由 planner（LLM）自己填写——等于 AI 判断自己的计划要不要给人看。最需要人看的计划，恰恰是模型没识别出风险的那些，而它们必然自报低风险。风险分级是人类职责。
> 2. 自治已有显式开关，就是 loop 模式。plan 模式再偷偷自动派发，等于存在第二条由 AI 决定的隐式自治路径。开关只留一个，握在人类手里。
>
> **连带失效**：Task 4（模板 human_checkpoints 驱动确认）从未实施，且其针对 plan 模式确认闸的部分已无对象——plan 一律停，无需卡点声明来决定停不停（该 Task 若重启，只剩"注入人类判据"一半仍有意义）。**Task 5 Step 1 的判据已反转**：低风险浅出口需求现在**必然**产生 plan_review 确认卡，不再是"自动派发不停确认卡"；照原文复跑该 GATE 会误判为回归。
>
> 实测反证（2026-08-07 真实链路）：`software_delivery` 模板声明 3 出口，需求选中最浅的 `branch_ref`、`review_required=false`、无任何风险触发器点火——撤销前这正是自动派发的形状，撤销后落 `pending_review` 并进人类收件箱。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把已落地意图层 P1 的"每需求强制人类判据"默认翻成**自治默认**——低风险/浅出口需求零人类触点跑完，人类判据只在"策略显式要求 / 高风险不可逆动作 / 模板声明的阶段卡点"注入；收敛闸对执行接地判据自动放行。

**Architecture:** 最小改动、不破坏已落地闭环。核心翻转在 `acceptance_criteria.go` 的兜底注入逻辑（把"除非豁免才不注入"翻成"除非要求才注入"）+ 确认密度接出口深度（`PersistPlanRevision` 状态逻辑）；收敛闸（`demand_acceptance_gate.go`）行为随之自然放行，补测试钉死。场景模板 human_checkpoints 声明是**附加精化**（Task 4），撞并发会话未提交的 scenariotemplate 改动，显式门控——前三任务不依赖它即可交付"默认自治"。

**Tech Stack:** Go (Temporal workflow + project domain) / sqlc（仅 Task 4）/ 真实 E2E。

**Spec:** `docs/superpowers/specs/2026-07-16-autonomy-posture-calibration-design.md` §8 Phase A + §6.5 模式边界。本计划只覆盖 Phase A；B（对抗式 AI 评判）、C（外环闭合+问责翻转）另立。

**关键设计落点（已核当前代码）：**
- 兜底注入：`ensureHumanJudgmentCriterion(plan, policy)`（acceptance_criteria.go:80）现读 `acceptanceHumanJudgmentExempt(policy)`（键 `acceptance_human_judgment_exempt`，默认注入除非豁免）。翻转为默认不注入。
- 高风险信号已在 plan 上：`plan.RequiresHumanReview`、per-task `RequiresHumanApproval`（template_governance 的 human_gate 已置，:455）、`task.RiskLevel`；`highRiskLevel(...)` helper 在 task_result_contract.go 已有。
- 确认门：`PersistPlanRevision`（project_store.go:516-520）——plan 模式恒 `PendingReview`，仅 autonomous mode（loop/chat）+ 无 review 触发才 `Accepted`。要让浅出口 plan 模式也能 Accept。
- 收敛闸：`ResolveUnsatisfiedBlockingCriteria`（demand_acceptance_gate.go:105）——无 blocking human 判据时自然放行；行为随 Task 1 翻转，Task 3 补测钉死。
- 出口深度信号：`plan.ExitDeliverable` + 模板 `exits` 数组下标（越深下标越大，P2a 已有 `exit_at_or_beyond` 拓扑序）。

## Global Constraints

- 根级命令 `corepack pnpm <script>`；Go 定向 `cd apps/control-plane && go test ./internal/...`；禁 npx。
- **workflow.go / project_store.go 协调路径有改动的任务必须跑 `go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 并报结果；改 workflow 决策序列须由新判别事实驱动，禁事后 GetVersion 围栏（除非判别子只能靠新 activity 读取——见意图层 Task 6 先例，那种情形 GetVersion 从新分支诞生即围栏是正解，须在报告论证）。**
- 迁移（仅 Task 4）：编号顺延、atlas hash + migrate-validate、全中文 COMMENT。
- 契约改动走 generate + verify:contracts。
- **共享 checkout 纪律（本次尤其关键，刚踩过事故）**：只在 `feat/autonomy-posture-a` 分支工作，只暂存本任务文件；工作树有并发会话未提交的 `scenariotemplate/service.go`+`service_test.go`+`AGENTS.md`，**绝不 touch/stage**；合并用 ref 手术、切换用 symbolic-ref、删分支前三查+核对 `was` sha（见 memory `shared-checkout-concurrent-session-git-safety`）。
- **Task 4 执行前置**：确认并发会话的 scenariotemplate 改动已落地（`git status` 无 scenariotemplate 未提交）或本任务在隔离 worktree 跑，否则撞车——**Task 4 未解阻塞时不得动 scenariotemplate**。
- 每任务提交，尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: 兜底人类判据默认翻转——自治默认（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/acceptance_criteria.go`（:80 `ensureHumanJudgmentCriterion`、:100 `acceptanceHumanJudgmentExempt`、常量区 :42）
- Test: `acceptance_criteria_test.go`（既有测试文件；`rg -l ensureHumanJudgmentCriterion apps/control-plane/internal/workflow/projectcoordination/*_test.go` 定位）

**Interfaces:**
- Produces（语义翻转，签名扩展）:
```go
// 默认不注入(自治); 仅三种情形注入 blocking human_judgment 兜底:
//   1. 策略显式要求: policy["require_human_acceptance"]==true
//   2. 计划高风险: plan.RequiresHumanReview 或任一 task.RequiresHumanApproval 或 highRiskLevel(task.RiskLevel)
//   3. (Task 4 追加) 模板在选定出口声明了 human_checkpoint —— 本任务先留 TODO 注释, Task 4 接
func ensureHumanJudgmentCriterion(plan *RouteDecisionPlan, policy map[string]any)
const requireHumanAcceptancePolicyKey = "require_human_acceptance"
func requireHumanAcceptance(policy map[string]any) bool  // 读新键, 缺省 false
func planTouchesHighRisk(plan *RouteDecisionPlan) bool    // RequiresHumanReview || 任一 task RequiresHumanApproval || highRiskLevel(RiskLevel)
```
- 向后兼容：保留读旧键 `acceptance_human_judgment_exempt`——若显式 `true` 仍语义为"绝不注入"（即使高风险？否——高风险是宪法级不可豁免，见下）。**决策**：`require_human_acceptance` 与高风险是"注入触发器"，`acceptance_human_judgment_exempt=true` 仅豁免"策略/模板触发的注入"，**不豁免高风险注入**（宪法级不可绕）。即注入 = `(requireHumanAcceptance || 模板卡点) && !exempt` OR `planTouchesHighRisk`（高风险无视 exempt）。
- 已有的注入位置 `applyAcceptanceCriteriaDefaults`（:148）调用不变；只是 `ensureHumanJudgmentCriterion` 内部逻辑翻转。

- [ ] **Step 1: 失败测试**：
  1. `TestFallbackNotInjectedByDefault` — 普通 plan、空 policy、无高风险 → **无** human_judgment 判据注入（现状会注入，先红）。
  2. `TestFallbackInjectedWhenPolicyRequires` — policy `{"require_human_acceptance":true}` → 注入。
  3. `TestFallbackInjectedWhenHighRisk` — plan.RequiresHumanReview=true（或某 task RequiresHumanApproval=true）→ 注入,即使 policy 空。
  4. `TestHighRiskInjectionNotExemptable` — policy `{"acceptance_human_judgment_exempt":true}` 但 plan 高风险 → **仍注入**（高风险不可豁免）。
  5. `TestPolicyInjectionExemptable` — policy `{"require_human_acceptance":true, "acceptance_human_judgment_exempt":true}` 非高风险 → 不注入（豁免压过策略要求）。
  6. `TestPlannerAuthoredHumanCriterionSuppressesFallback` — plan 自带 human_judgment 判据 → 不重复注入（现状行为保留）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay 测试**（acceptance_criteria 在 planner 管线，改注入逻辑；跑 replay 报结果）→ **Step 5: Commit** — `feat(coordination): 兜底人类判据默认翻转——自治默认, 仅策略/高风险注入`

---

### ~~Task 2: 确认密度接出口深度——浅出口 plan 模式自动派发（TDD）~~【已撤销 2026-08-07，见文首】

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`（`PersistPlanRevision` 状态逻辑 :516-520；`isAutonomousCoordinationMode` :587 附近加出口深度判定）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go` 或快照——确认 `plan.ExitDeliverable` 与模板 `exits` 在 PersistPlanRevision 输入可得（`rg -n "ExitDeliverable" apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go`）
- Test: `project_store_test.go`

**Interfaces:**
- Consumes: Task 1 的注入语义（无 human 判据的浅出口计划应可自动派发）；`plan.ExitDeliverable`、快照模板 `exits` 数组（深度=下标）。
- Produces: `PersistPlanRevision` 状态判定扩展——plan 模式不再恒 PendingReview：
  - `PendingReview` 当：`validation.ReviewRequired` || `Decision.RequiresHumanReview` || `planTouchesHighRisk` || **出口深度 ≥ 模板声明的确认阈值**（Task 4 前用保守回退：出口是模板 exits 的**最深一档**时需确认；无模板/无 exits 时按现状 plan 模式 PendingReview——不放开未知深度）。
  - `Accepted`（自动派发）当：非上述任一，即浅出口 + 无高风险 + 无人类判据 + plan 或 autonomous 模式。
  - 新 helper `func planRequiresHumanConfirmation(input PersistPlanRevisionInput, plan/snapshot) bool` 收口这套判定；`isAutonomousCoordinationMode` 保留但不再是唯一放行条件。
- **Temporal 纪律**：本改动改的是 activity（PersistPlanRevision）内的状态派生，非 workflow 命令序列——replay 安全；仍跑 replay 报结果。**保守回退**保证存量（无出口深度信息的计划）行为不变。

- [ ] **Step 1: 失败测试**：
  1. `TestShallowExitPlanModeAutoDispatches` — plan 模式、出口=最浅档（如 branch_ref）、无高风险、无 human 判据 → `Accepted`（现状 plan 模式恒 PendingReview，先红）。
  2. `TestDeepExitPlanModeHoldsForConfirm` — 出口=最深档（如 release_record）→ `PendingReview`。
  3. `TestHighRiskAlwaysHolds` — 浅出口但 planTouchesHighRisk → `PendingReview`。
  4. `TestHumanCriterionPresentHolds` — 有 human_judgment 判据（Task1 因策略/高风险注入的）→ `PendingReview`。
  5. `TestNoTemplateExitsConservativePendingReview` — 无模板/无 exits 的 plan 模式 → `PendingReview`（保守回退,存量不变）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay** → **Step 5: Commit** — `feat(coordination): 确认密度接出口深度——浅出口plan模式自动派发`

---

### Task 3: 收敛闸随默认翻转自然放行——低风险零人类触点闭环（TDD）

**Files:**
- Test-primarily: `apps/control-plane/internal/project/demand_acceptance_gate_test.go` + `pg_repository_test.go`（收敛闸集成）
- Modify（若需）: `demand_acceptance_gate.go` / `recomputeProjectDemandStatusWithQueries`——预期无逻辑改动（Task 1 无 human 判据→无 blocking human→闸自然放行），仅确认并补测；若发现存量有硬编码假设"总有 human 判据"则修正。

**Interfaces:**
- Consumes: Task 1（无兜底 human）、Task 2（浅出口自动派发）。
- Produces: 语义钉死——只有 automated_test（执行接地）判据的需求，任务全完（verdict satisfied+attestation）→ **直接 completed，不 hold acceptance_pending，不开 demand_acceptance 决策**。有 blocking human 判据（策略/高风险/模板注入）时才 hold。

- [ ] **Step 1: 失败测试（或确认现状）**：`TestLowRiskDemandCompletesWithoutHumanHold` — 快照仅 automated_test blocking 判据、executor verdict satisfied+attestation、无 human 判据 → recompute 直达 completed（非 acceptance_pending），无 demand_acceptance 决策创建。`TestHumanCriterionStillHolds` — 有 blocking human 判据 → acceptance_pending（回归护栏，意图层已有行为不破）。
- [ ] **Step 2: 跑测试**（可能直接绿——则说明 Task1 已使其成立，仍保留测试钉死；若红则修 gate）→ **Step 3: 必要修正** → **Step 4: 全包** → **Step 5: Commit** — `feat(project): 收敛闸低风险零人类触点闭环+测试钉死`

---

### Task 4:【撞车门控·执行前必须确认并发 scenariotemplate 已落地或用隔离 worktree】场景模板 human_checkpoints 声明（TDD）

**执行前置检查（硬性）**：`git status --short | rg scenariotemplate` 为空（并发会话改动已落地），否则本任务阻塞——报告 BLOCKED，不得动 scenariotemplate 文件，Phase A 以 Task 1-3 交付"策略/风险驱动的默认自治"，Task 4 待并发落地后补。

**Files（并发落地后）:**
- Migration: scenario_templates spec v2 增 `human_checkpoints`（或走 spec jsonb，无需迁移——评审时定；倾向 jsonb 内字段，零迁移）
- Modify: `apps/control-plane/internal/scenariotemplate/spec.go`（SpecV2 加 `HumanCheckpoints []SpecHumanCheckpoint{AfterStage/Exit string; Kind string}`）
- Modify: `acceptance_criteria.go`（Task 1 留的 TODO：模板在选定出口声明 checkpoint → 注入 human 判据）
- Modify: `project_store.go`（Task 2 的 `planRequiresHumanConfirmation` 接模板 checkpoint 而非仅"最深档"保守回退）
- Test: spec 解析 + 注入/确认的模板驱动路径

**Interfaces:**
- Consumes: Task 1/2 的注入与确认判定挂点、场景模板 P2 的 spec 解析。
- Produces: 模板可声明"分析后确认 / 上线前验收"——注入与确认从"仅策略/风险驱动"升级为"模板阶段卡点驱动"；无声明的阶段边界 agent 自过。

- [ ] **Step 0: 前置检查**（BLOCKED 则停）→ **Step 1: 失败测试**（模板声明 checkpoint@exit → 该出口计划注入 human 判据 + PendingReview；未声明 → 自治）→ **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay + verify:contracts（若动契约）** → **Step 5: Commit** — `feat(scenariotemplate): human_checkpoints阶段卡点声明驱动人类确认`

---

### Task 5:【GATE】真实 E2E —— 默认自治 + 高风险仍拦（判决点）

**前置：** Task 1-3（及 Task 4 若已解阻塞）合入分支；control-plane 重启加载分支码。

- [ ] **Step 1 低风险零人类触点**：单/双员工项目提**浅出口需求**（"只分析磁盘用量给结论"，出口=分析结论，无删除写入）→ 计划**自动派发不停确认卡**（psql plan revision 状态 Accepted）→ 任务真实执行 → verdict 投影 → 需求**直达 completed,从不 acceptance_pending,无 demand_acceptance 决策**，全程零人类触点。
- [ ] **Step 2 高风险仍拦**：提**触及高风险的需求**（含"合入主干"或诱导 human_gate 的出口）→ 计划 PendingReview（拦人）；或任务 RequiresHumanApproval → 注入 human 判据 → 需求 hold acceptance_pending 等签署。**判决点**：低风险真的零触点、高风险真的拦——两者都成立才 PASS；若低风险仍被拦（默认没翻转成功）或高风险被放过（拦不住)→ 停止回评审。
- [ ] **Step 3 模板卡点**（Task 4 已做才验）：绑声明了 checkpoint 的模板 → 对应出口拦人。
- [ ] **Step 4 记录 + Commit** — `docs(plan): Phase A 默认自治闸门记录`

---

### Task 6: 收尾 —— 门禁 + 完成检查

- [ ] **Step 1**: `corepack pnpm verify:control-plane`（+ verify:contracts 若 Task 4 动契约）。
- [ ] **Step 2**: 存量回归 E2E：既有含 human 判据的项目（策略要求/高风险）行为不变——需求仍 hold + 签署闭环（意图层 P1 闭环不破）。
- [ ] **Step 3**: `$superteam-completion-check`；CHANGELOG（`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`）；memory 更新（姿态校准 Phase A 落地）。
- [ ] **Step 4**: Commit + 汇报（含 Phase B/C 衔接、Task 4 若阻塞的遗留状态）。

---

## 实施记录

（实施时追加。）
