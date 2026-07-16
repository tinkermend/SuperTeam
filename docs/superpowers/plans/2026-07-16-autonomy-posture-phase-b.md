# 自治姿态校准 Phase B（对抗式 AI 评判）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把信任三档锚定的**档 2（多样对抗 AI 评判）**建起来——落不到执行事实的判据由 N 个独立判官 agent **证伪（非确认）**、多数证伪即杀，产出可复利血缘的 verdict，接进收敛闸。这是 HiClaw/paperclip 共缺、命门所在的那一层。

**Architecture:** 判官不需要 worktree——一个 judge = 一次直连 LLM 调用（复用 planner 的 `CreateChatCompletion` 范式）+ 被评审任务的工件（复用 `collectUpstreamResults`）。默认 3 判官（正确性/安全/可复现视角，策略可调 N），多数证伪（≥⌈N/2⌉ refute）即判 unsatisfied。判官组由协调线程在**被评审任务完成后**异步派一个新 Activity `RunAdversarialReview` 执行（该 Activity 只对 adversarial_review 判据派发，旧历史无此判据→天然不触发→Temporal 重放安全）；聚合结果写一条 adversarial verdict（judge_type=`adversarial`）+ 逐判官明细行（血缘）。收敛闸把 adversarial verdict 当作 satisfied/unsatisfied 消费（多数逻辑在 Activity 内算，闸门不引入 quorum）。

**Tech Stack:** Go (Temporal + 直连 LLM chatCompletionClient) / Atlas / 真实 LLM 判官 E2E。

**Spec:** `docs/superpowers/specs/2026-07-16-autonomy-posture-calibration-design.md` §2 档2 + §8 Phase B。本计划只覆盖 Phase B；C（外环闭合+问责翻转）另立。**已拍板决策**：3 固定视角（correctness/security/reproducibility）+ 项目 `coordination_policy.adversarial_review_judges` 可调 N；多数证伪即杀；触发范围=机制先行（planner/策略驱动 adversarial_review 判据，零撞车），模板 reviewer 角色迁入 + exit_evidence 校验延后（Task 5 撞车门控）。

**关键设计落点（已核当前代码，来自探查）：**
- verification_method 注册表 `knownVerificationMethods`（acceptance_criteria.go:19）：现 automated_test/human_judgment，加 adversarial_review。校验 `validateAcceptanceCriteriaSemantics`（:233）+ graph_validation.go:272（satisfied_by 豁免）——adversarial_review **需 satisfied_by**（它评审具体任务产出，像 automated_test）。
- 直连 LLM 范式：`chatCompletionClient.CreateChatCompletion`（openai_compatible_planner.go:62/251），调用范例 planner :99-105（system/user/MaxTokens/Temperature + requestContext 超时 + MaxAttempts 重试）。**当前包内未导出**——judge 引擎与 planner 同包（projectcoordination）或把 client 做成可复用。
- verdict 表 `demand_criterion_verdicts`（迁移 064）：`judge_type VARCHAR(32)` 无 CHECK，DDL-free 加 `adversarial`；但两唯一索引（uq_demand_verdicts_task/human）容不下"一 criterion 一 adversarial 聚合行"——**需新增 partial unique**（judge_type='adversarial' 时 one-per-criterion，project_task_id NULL）。
- 证据注入 `collectUpstreamResults`（project_store.go:3188）：直接 blocker 的 summary(4KB)/deliverables/evidence_refs/artifact_refs——judge 的工件载荷。
- 收敛闸聚合 `criterionEffectiveVerdict`（demand_acceptance_gate.go:73）现为乐观 OR，human 覆盖 executor——adversarial verdict 作为第三 judge_type，按其 satisfied/unsatisfied 参与（多数已在 Activity 算完，闸门不加 quorum）。
- 任务完成触发点：executor 投影在 `recordProjectTaskAttemptResult`（service.go:3240）/`projectDemandCriterionVerdicts`（:3515），HTTP 回调同步——但 judge 跑 N 次 LLM（N×25-60s）不能同步阻塞回调，必须**异步经协调线程 Activity**（见 Architecture）。

## Global Constraints

- 根级命令 `corepack pnpm <script>`；Go 定向 `cd apps/control-plane && go test ./internal/...`；禁 npx。
- **workflow.go 有改动的任务必须跑 `go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 并报结果**；`RunAdversarialReview` 派发是新 Activity、仅对 adversarial_review 判据（旧历史无此判据）触发——旧历史天然不派发此 Activity，重放无新命令，Temporal 安全；仍跑 replay 为证。禁事后 GetVersion 围栏除非判别子只能靠新 activity 读取（那种情形从新分支诞生即围栏是正解，须论证）。
- 迁移：编号顺延（先 `ls migrations/*.sql | tail -1` 确认，并发会话可能已用 067+）、atlas hash + migrate-validate、全中文 COMMENT、DATABASE_DESIGN.md。
- **成本护栏**：每条 adversarial_review 判据跑 N 次 LLM（默认 3）。Activity 必须接项目预算——触及预算上限时降级（跳过 judge、记未评审事件、升级人类档3）而非静默烧钱；`RunAdversarialReview` 有 max 判官数硬上限（如 7）防策略配置爆炸。
- **共享 checkout 纪律（本会话踩过两次事故）**：只在 `feat/autonomy-posture-b` 分支工作，只 `git add <本任务具体文件>`（禁 -A/.）；工作树有并发会话大批未提交改动（scenariotemplate/service.go、employee/run_*、chat-panel 等），**绝不 touch/stage**；合并用隔离 worktree + ref 手术，不切/删共享分支；commit 前 `git symbolic-ref HEAD` 核对当前分支（快照会过期）。见 memory `shared-checkout-concurrent-session-git-safety`。
- **Task 5 执行前置**：`git status --short | rg scenariotemplate` 为空才可动 scenariotemplate，否则 BLOCKED、Phase B 以 Task 1-4 交付机制。
- 每任务提交，尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: adversarial_review 判据方法注册 + 校验（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/acceptance_criteria.go`（:11-22 常量+注册表、:233 校验）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`（:272 satisfied_by 豁免逻辑——adversarial_review 不豁免）
- Modify: `apps/control-plane/internal/project/demand_acceptance_gate.go`（:49-56 持久化侧字符串镜像加 adversarial）
- Test: acceptance_criteria_test.go

**Interfaces:**
- Produces: `const VerificationMethodAdversarialReview = "adversarial_review"`；加入 `knownVerificationMethods`；`demandCriterionVerificationMethodAdversarialReview = "adversarial_review"`、`demandCriterionJudgeTypeAdversarial = "adversarial"`（project 包镜像）。
- 校验语义：adversarial_review **必须有 satisfied_by**（评审具体任务产出）——`validateAcceptanceCriteriaSemantics` 对 adversarial_review 走与 automated_test 相同的 satisfied_by 非空校验；graph_validation.go:272 的 human_judgment 豁免**不**扩到 adversarial_review。
- normalize：`normalizeCriterionDefaults` 不改（空方法仍默认 automated_test）；adversarial_review 由 planner 显式声明或策略升级（Task 4 触发逻辑），本任务只注册+校验。

- [ ] **Step 1: 失败测试**：`TestAdversarialReviewMethodRegistered`（方法在注册表、校验通过）；`TestAdversarialReviewRequiresSatisfiedBy`（无 satisfied_by → 校验拒绝，与 automated_test 同）；`TestUnknownMethodStillRejected`（回归）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay** → **Step 5: Commit** — `feat(coordination): adversarial_review 判据方法注册+satisfied_by校验`

---

### Task 2: 迁移 —— 对抗判官明细表 + adversarial verdict 唯一索引（TDD 抽查）

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/0NN_adversarial_review_judgements.sql`（NN=当前最大+1，先确认）
- Modify: atlas.sum

**Interfaces（后续任务消费）:**
```sql
-- 对抗判官逐判官明细：一条 adversarial_review 判据的 N 个独立判官各出一行（血缘/可复利/审计）。
CREATE TABLE IF NOT EXISTS demand_adversarial_judgements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL, project_id UUID NOT NULL, demand_id UUID NOT NULL,
    plan_revision_id UUID NOT NULL,
    criterion_id TEXT NOT NULL,
    reviewed_task_id UUID NOT NULL,        -- 被评审任务
    lens VARCHAR(64) NOT NULL,             -- correctness | security | reproducibility | ...(策略扩展)
    verdict VARCHAR(16) NOT NULL,          -- refuted | accepted（判官视角:证伪 or 未能证伪）
    reason TEXT NOT NULL DEFAULT '',
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_adversarial_judgement UNIQUE (tenant_id, demand_id, plan_revision_id, criterion_id, lens)
);
CREATE INDEX IF NOT EXISTS idx_adversarial_judgements_tenant_demand
    ON demand_adversarial_judgements(tenant_id, demand_id, plan_revision_id);

-- demand_criterion_verdicts 新增 adversarial 聚合行的唯一约束(一 criterion 一聚合 verdict, project_task_id NULL, judge_type='adversarial')
CREATE UNIQUE INDEX IF NOT EXISTS uq_demand_verdicts_adversarial
    ON demand_criterion_verdicts(tenant_id, demand_id, plan_revision_id, criterion_id)
    WHERE project_task_id IS NULL AND judge_type = 'adversarial';
```

全中文 COMMENT 逐表逐列。注意：新 partial index 与既有 `uq_demand_verdicts_human`（project_task_id IS NULL 无 judge_type 条件）**可能重叠**——human 行 judge_type='human'，adversarial 行 judge_type='adversarial'，两 partial index 的 WHERE 谓词需互斥（human 索引须加 `AND judge_type='human'`，否则同 criterion 的 human+adversarial 两 NULL 行撞车）。**迁移必须同时把 `uq_demand_verdicts_human` 收紧为含 `judge_type='human'`**（drop+recreate，向后兼容——存量 human 行 judge_type 均为 'human'）。

- [ ] **Step 1: 确认编号写迁移**（含 human 索引收紧）→ **Step 2: atlas hash + migrate-validate（scratch-schema DEV_URL）+ migrate-up + psql 抽查两对象 + human 索引新谓词** → **Step 3: Commit** — `feat(db): 对抗判官明细表+adversarial verdict唯一索引+收紧human索引 (迁移0NN)`

---

### Task 3: 对抗判官引擎 —— 直连 LLM 证伪 + 多数聚合（TDD，核心）

**Files:**
- Create: `apps/control-plane/internal/workflow/projectcoordination/adversarial_review.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`（新 Activity `RunAdversarialReview`）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`（input/output 类型）
- Test: `adversarial_review_test.go`

**Interfaces:**
- Consumes: `chatCompletionClient.CreateChatCompletion`（现有直连范式）；`collectUpstreamResults` 组装的证据（被评审任务产出）；project `coordination_policy.adversarial_review_judges`（默认 3，硬上限 7）。
- Produces:
```go
type AdversarialLens struct { Key string; SystemPrompt string } // correctness/security/reproducibility
type AdversarialJudgement struct { Lens string; Verdict string; Reason string } // Verdict: "refuted"|"accepted"
type AdversarialReviewResult struct {
    CriterionID string; ReviewedTaskID uuid.UUID
    Judgements  []AdversarialJudgement
    Aggregate   string // "satisfied"(多数未证伪) | "unsatisfied"(多数证伪)
    RefutedCount, JudgeCount int
}
// RunAdversarialReview 对一条 adversarial_review 判据跑 N 判官(默认3), 每判官一次证伪式 LLM 调用, 多数证伪→unsatisfied。
func (a *Activities) RunAdversarialReview(ctx context.Context, input RunAdversarialReviewInput) (AdversarialReviewResult, error)
```
- 判官 prompt（对抗式，写死默认三视角，system 明确"你的任务是**证伪**——找出这份产出不满足判据的理由；默认判 refuted 除非你被工件说服它确实满足；不要客气地确认"）：
  - correctness：产出是否真正满足判据断言（逻辑/边界/正确性）。
  - security：是否引入安全/权限/数据泄漏问题。
  - reproducibility：结论是否有可复现证据支撑（非空口断言）。
- 每判官返回结构化 `{verdict: refuted|accepted, reason}`（解析 JSON；解析失败→保守判 refuted + reason=parse_failed，宁严勿漏）。
- 聚合：`RefutedCount >= ceil(JudgeCount/2)` → Aggregate=unsatisfied，否则 satisfied。N=2 时 ceil(2/2)=1 → 任一 refute 即杀（保守，与用户"2 判官偏保守"一致）。
- **成本护栏**：Activity 开头查预算（复用既有 budget 检查路径——`rg -n "budget|Budget" activities.go project_store.go | head`）；触顶 → 返回特殊 result（Aggregate=escalate_human，RefutedCount/JudgeCount=0）供 Task 4 转档3；`JudgeCount = min(policy值, 硬上限7)`。
- 独立性：N 个判官是同一 LLM 的 N 次独立调用（不同视角 system prompt）——诚实声明这是"视角多样"非"模型多样"（同源模型的独立弱于人类独立，档2 的已知局限，spec §7 已记）。

- [ ] **Step 1: 失败测试**（fake chatCompletionClient 注入固定响应）：
  1. `TestAdversarialMajorityRefuteKills` — 3 判官 2 refute → Aggregate=unsatisfied，RefutedCount=2。
  2. `TestAdversarialMinorityRefutePasses` — 3 判官 1 refute → satisfied。
  3. `TestAdversarialParseFailureConservativeRefute` — 判官返回非法 JSON → 该判官记 refuted+parse_failed。
  4. `TestAdversarialTwoJudgeAnyRefuteKills` — N=2 1 refute → unsatisfied（保守）。
  5. `TestAdversarialJudgeCountCappedAndPolicyRead` — policy=10 → JudgeCount=7（硬上限）；policy 缺省=3。
  6. `TestAdversarialEachJudgeGetsRefutePrompt` — 断言每次 CreateChatCompletion 的 system 含证伪指令且视角不同。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包（judge 引擎不派 workflow 命令，但 Activity 在协调包——跑 replay 报结果）** → **Step 5: Commit** — `feat(coordination): 对抗判官引擎——直连LLM证伪+多数聚合`

---

### Task 4: 投影路径 C + 触发 + 收敛闸消费（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`（被评审任务完成后派 `RunAdversarialReview`——见触发设计）
- Modify: `apps/control-plane/internal/project/`（repository：`CreateAdversarialJudgements`(批量) + `CreateAdversarialVerdict`(聚合行)；判据快照按 method 查 adversarial 判据 + 其 satisfied_by）
- Modify: `apps/control-plane/internal/project/demand_acceptance_gate.go`（`criterionEffectiveVerdict` 消费 adversarial verdict）
- Test: 各层

**Interfaces:**
- Consumes: Task 2 表、Task 3 引擎。
- **触发设计**：协调线程在某任务完成解锁下游时（`resolveReadyDownstream`/任务完成处理，workflow.go），查该完成任务是否是某条 adversarial_review 判据的 satisfied_by（经一个查快照的 Activity）；是→`ExecuteActivity(RunAdversarialReview, {criterion, reviewedTaskID, ...})`→拿 result→经 Activity 写 adversarial verdict（聚合行 judge_type=adversarial verdict=result.Aggregate project_task_id=NULL）+ N 条明细行（`CreateAdversarialJudgements`）。**Temporal 安全**：该派发分支只在存在 adversarial_review 判据时进入——旧历史无此判据（旧计划无此 method）→分支不可达→无新命令→重放安全（跑 replay 为证）。escalate_human 结果（预算触顶）→ 注入/升级人类档3（复用 human 判据 hold 机制）。
- 收敛闸：`criterionEffectiveVerdict` 加 adversarial 分支——一条 adversarial_review 判据的 effective verdict = 其 adversarial 聚合行的 verdict（satisfied/unsatisfied）；human 仍可覆盖（人类档3 兜底）；无 adversarial verdict（未跑/进行中）的 blocking adversarial 判据按未决 hold（同 human 未签）。
- 幂等：同 criterion 重跑（任务重试）→ adversarial 聚合行 upsert（uq_demand_verdicts_adversarial），明细 upsert（uq_adversarial_judgement per lens）。

- [ ] **Step 1: 失败测试**：`TestAdversarialVerdictProjectedAndConsumed`（引擎 result unsatisfied → 聚合行+明细行落库 → 收敛闸算该 criterion unsatisfied → demand hold）；`TestAdversarialSatisfiedReleasesGate`（多数未证伪 → satisfied → 放行）；`TestAdversarialTriggerOnlyForReviewedTask`（非 satisfied_by 任务完成不触发）；`TestAdversarialEscalateHumanOnBudget`（escalate_human → 转人类 hold）；`TestHumanOverridesAdversarial`（人类档3 覆盖）。
- [ ] **Step 2: RED** → **Step 3: 实现** → **Step 4: 全包 + replay（改 workflow.go 触发——MANDATORY 报结果）** → **Step 5: Commit** — `feat(coordination): 对抗verdict投影+任务完成触发+收敛闸消费`

---

### Task 5:【撞车门控·执行前查 scenariotemplate 无并发未提交或用隔离 worktree】模板 reviewer 角色迁入 adversarial_review + exit_evidence 校验（TDD）

**执行前置（硬性）**：`git status --short | rg scenariotemplate` 为空，否则 BLOCKED——Phase B 以 Task 1-4 交付对抗判官机制（planner/策略驱动 adversarial_review），Task 5 待并发落地补。

**Files（并发落地后）:** scenariotemplate spec.go（reviewer 角色可声明 adversarial_review + skeleton step 的 exit_evidence）、template_governance.go（reviewer 独立性表达）、planner prompt（引导 review 型判据用 adversarial_review、无人自治阶段 exit_evidence 不得全意见类）。
- exit_evidence 校验（spec §3 写死推论）：模板声明为无人自治的阶段，其出口判据不得全是 human_judgment/纯意见——至少一条 execution-grounded(automated_test) 或 adversarial_review，否则规划期拒绝或降级需人确认。

- [ ] **Step 0: 前置检查**（BLOCKED 则停）→ **Step 1-5**: TDD 模板 reviewer→adversarial_review 迁移 + exit_evidence 校验 + replay + verify:contracts → Commit — `feat(scenariotemplate): reviewer角色迁入adversarial_review+exit_evidence校验`

---

### Task 6:【GATE】真实对抗评判 E2E（判决点）

**前置：** Task 1-4（及 Task 5 若解阻塞）合入分支；control-plane 重启加载分支码。

- [ ] **Step 1 判据带对抗评审**：造一条 adversarial_review 判据（planner 产出或策略/手工注入到某软件交付需求的 review 出口），satisfied_by=某开发任务。
- [ ] **Step 2 好产出通过**：开发任务真实产出合格结果 → 协调线程真实派 RunAdversarialReview → 3 个真实 LLM 判官跑证伪 → 多数未证伪 → adversarial verdict satisfied（psql demand_criterion_verdicts judge_type=adversarial + demand_adversarial_judgements 3 行含各视角 reason）→ 收敛闸放行。
- [ ] **Step 3 烂产出被证伪（判决点）**：造一条明显不满足判据的产出（如结论无证据支撑/引入明显缺陷）→ 3 判官多数 refuted → adversarial verdict unsatisfied → demand hold/未过 → **judge 血缘可查**（psql 三条明细 reason 指出缺陷）。**PASS = 好产出真放行 + 烂产出真被多数证伪拦住**；若烂产出被放行（判官没抓住）或好产出被误杀 → 记录判官原始输出，评估是否 prompt 需加固,不硬改绕过。
- [ ] **Step 4 成本/预算**：观察 N 次 LLM 调用真实发生、预算计入；构造预算触顶 → escalate_human 转档3。
- [ ] **Step 5 记录 + Commit** — `docs(plan): Phase B 对抗评判闸门记录`

---

### Task 7: 收尾 —— 门禁 + 完成检查

- [ ] **Step 1**: `corepack pnpm verify:control-plane`（+ verify:contracts 若动契约）。
- [ ] **Step 2**: 存量回归：无 adversarial_review 判据的需求行为不变（Phase A 默认自治 + 意图层闭环不破）。
- [ ] **Step 3**: `$superteam-completion-check`；CHANGELOG（`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`，注记成本=每条 adversarial 判据 N 次 LLM、同源模型独立性局限、Task 5 若阻塞的遗留）；memory 更新。
- [ ] **Step 4**: Commit + 汇报（含 Phase C 衔接：外环自迭代消费 adversarial verdict 作返工输入；问责层 pull 记录仪；loop 高风险收口）。

---

## 实施记录

（实施时追加。）
