# 自治姿态 Phase C1（对抗 verdict → 自动返工自迭代）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 闭合 spec §4"开发↔判官无人自迭代"——判官 held（对抗 unsatisfied）且返工预算剩余时，把判官逐 lens 驳斥理由合成返工输入 → 走既有返工机件重跑被审任务 → 完成再判官 → 收敛（satisfied 放行）或耗尽（→ acceptance_pending 人类兜，4.6 行为）。E2E 暴露的"held 死等"开口就此闭合。

**Architecture:** 接线为主 + 两处小修。既有返工循环骨架（`createRevisionTaskForResult` + `revisionInputRequirements` 理由回灌 + `revisionBudgetExhausted` 有界）完备但硬依赖 agent 自报 RevisionRequest；对抗层已持久化 verdict 但 fall-through 不返工。C1 = 把对抗 held 翻译成合成 RevisionRequest 走返工路径 + 修两个断点（返工任务 key 不匹配致判官不重点火、逐 lens 理由无 Go 读取器）。

**Tech Stack:** Go（Temporal + sqlc）/ 真实 deepseek 判官 E2E。

**Spec:** posture `2026-07-16-autonomy-posture-calibration-design.md` §4/§8 Phase C + outer-loop `2026-06-30-autonomous-outer-loop-iteration-attestation-budget-design.md` 支柱 A。**已拍板**：C1 先行；held+预算剩余→返工（暂不解锁下游，主动收敛中）、held+预算耗尽→解锁至 acceptance_pending（4.6）。C3 问责翻转保守版另阶段。

**关键锚点（探查已核，main）：**
- 返工构造 `CreateRevisionTaskForResult`（project_store.go ~:944-994）：复用 DemandID/AssignedDigitalEmployeeID/RiskLevel/CoordinationJobID/RouteDecisionID/TaskKind/StageIndex/BlockedByTaskIDs/RevisionOfTaskID；**`PlannedTaskKey: revisionTaskKey(source,result)` 生成新 key**（断点 1）。硬守卫 `Decision==RevisionAttempt && Contract.RevisionRequest!=nil`（:959）。
- 理由回灌 `revisionInputRequirements`（project_store.go:4029）：注入 revision_reason/requested_changes/source_task_id/source_result_id/source_result_summary。
- `revisionPlannerMetadata`（:4042）：revision_root_task_id/revision_attempt_count(+1)/revision_max_attempts。
- 预算 `revisionBudgetExhausted`（方法版 project_store.go:4211，跨 job 扫 priorRevisionTasks 取 max attempt 比 revisionMaxAttempts 默认 3）。
- 对抗触发 `handleEmployeeTaskCompleted`（workflow.go:672-745，GetVersion `adversarial-review-trigger` 现 v2）；held 分支 :699-708 fall-through；`AdversarialReviewForTask`（adversarial_trigger.go）。
- 触发匹配 `listAdversarialCriteriaForTask`（adversarial_trigger.go:225）：`task.PlannedTaskKey` 比 criterion.SatisfiedBy（断点 1——返工 task 新 key 不匹配→不重点火）。
- 逐 lens 明细 `demand_adversarial_judgements`：sqlc 查询**名被误写成 `n`**（`-- name: n :many`，storage/queries/demand_adversarial_judgements.sql）+ 无 Go 仓储包装（断点 2）。聚合 verdict Reason 经 `ListDemandCriterionVerdicts`→`DemandCriterionVerdict.Reason` 可读。
- 对抗结果类型 `AdversarialReviewForTaskResult`（Reviewed/AllSatisfied/AnyEscalated）；持久化 `PersistAdversarialOutcome`→`CreateAdversarialVerdict`/`CreateAdversarialJudgements`。

## Global Constraints
- 根级 `corepack pnpm`；Go 定向 `cd apps/control-plane && go test ./internal/...`；禁 npx。
- **改 workflow.go 命令序列必须 replay**：`go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 报结果。触发行为改动=GetVersion `adversarial-review-trigger` 升 **v3**（v1/v2 保留旧行为，minSupported=DefaultVersion）；不可原地改 v2。
- sqlc 改动跑仓库 sqlc 生成（查 Makefile 的 sqlc/generate target），不手改生成物。
- **共享 checkout + 活跃并发会话铁律**（现有 dircap/feishu 会话）：本 C1 用**隔离 worktree 基于 main 开 feat/autonomy-posture-c1**；只在 worktree 干活、只 add 本任务文件、禁 -A/. 禁 git stash；主 checkout 现被并发切到 detached，绝不碰；合并用 ref 手术/ff、删前三查核对 was sha。
- 每任务提交尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: 修 `n` 查询名 + Go 仓储读取器（ListAdversarialJudgements）（TDD）
**Files:** storage/queries/demand_adversarial_judgements.sql（`-- name: n` → `-- name: ListAdversarialJudgements`）；重生 .sql.go；project/repository.go（接口）+ project/pg_repository.go（实现，照 pg_repository.go:5628 CreateAdversarialJudgements 约定）。
**Interfaces:** Produces `ListAdversarialJudgements(ctx, tenantID, demandID, planRevisionID uuid.UUID) ([]DemandAdversarialJudgement, error)`（或按 criterion 过滤——按 sql 现有参数）。返回逐行 {CriterionID, ReviewedTaskID, Lens, Verdict, Reason}。
- [ ] Step1 失败测试：`TestListAdversarialJudgementsReadsBack`（写几条 judgements 经 CreateAdversarialJudgements → 读回逐 lens reason 一致）。
- [ ] Step2 RED → Step3 改 sql 名+重生 sqlc+加 Go 包装 → Step4 `go test ./internal/project/` + build → Step5 Commit `fix(project): 修demand_adversarial_judgements查询名n→ListAdversarialJudgements+Go读取器`

---

### Task 2: 返工任务重触发判官——按修订根 key 匹配判据（TDD）
**Files:** adversarial_trigger.go（`listAdversarialCriteriaForTask` :225）。
**问题:** 返工任务 PlannedTaskKey=revisionTaskKey(新)，criterion.SatisfiedBy 存原 key → 不匹配 → 返工完成不重点火判官，自迭代断。
**Interfaces:** 修 `listAdversarialCriteriaForTask`：若 task 是修订（RevisionOfTaskID 非空 / 有 revision_root_task_id planner_metadata），解析**修订根的 PlannedTaskKey**（沿 RevisionOfTaskID 链到根，或读 planner_metadata revision_root_task_id 对应 task 的 key），用根 key 比 SatisfiedBy。非修订任务行为不变。
- [ ] Step1 失败测试：`TestAdversarialCriteriaMatchRevisionByRootKey`（修订任务其根 key 在某 adversarial_review 判据 satisfied_by → 匹配命中）；`TestNonRevisionTaskUnchanged`（回归）。
- [ ] Step2 RED → Step3 实现（可能需 store 读根 task key 的 helper）→ Step4 全包 + replay → Step5 Commit `feat(coordination): 对抗判据匹配沿修订链到根key——返工任务重触发判官`

---

### Task 3: 判官驳斥 → 合成返工（TDD，核心）
**Files:** 新 adversarial_rework.go 或扩 adversarial_trigger.go；project_store.go（重构 createRevisionTaskForResult 或加 sibling `createReworkTaskFromAdversarial`）。
**Interfaces:**
- Consumes: Task1 `ListAdversarialJudgements`（逐 lens 驳斥）、`AdversarialReviewForTaskResult`、`revisionBudgetExhausted`。
- Produces: `synthesizeAdversarialRevision(criterion, judgements) project.RevisionRequest`（Reason=判官多数证伪摘要、RequestedChanges=逐 lens 驳斥理由列表）；一条返工构造路径接受合成 RevisionRequest（重构 createRevisionTaskForResult 抽出构造核心不再硬依赖 Contract.RevisionRequest，或 sibling 复用核心 + revisionInputRequirements 注入 revision_reason/requested_changes=判官理由 + 标记 source=adversarial_review）。
- 返工任务须复用同员工/同 demand/同 route + `RevisionOfTaskID=被审task.ID` + revision planner_metadata（attempt_count+1）；PlannedTaskKey 仍走 revisionTaskKey（Task2 已让匹配沿根解析，故新 key 不破 re-judge）。
- [ ] Step1 失败测试：`TestSynthesizeAdversarialRevisionFromJudgements`（3 lens refuted → RevisionRequest.Reason 含证伪摘要、RequestedChanges 含各 lens 理由）；`TestReworkTaskCarriesAdversarialReasonInInput`（返工任务 InputRequirements 含判官理由 + source=adversarial_review + RevisionOfTaskID 血缘）。
- [ ] Step2 RED → Step3 实现 → Step4 全包 + replay → Step5 Commit `feat(coordination): 判官驳斥合成RevisionRequest→既有返工路径重跑被审任务`

---

### Task 4: 触发接线——held+预算→返工 / held+耗尽→放行（TDD，replay v3）
**Files:** workflow.go（handleEmployeeTaskCompleted 对抗 held 分支 :699-708）。
**Interfaces:**
- GetVersion `adversarial-review-trigger` 升 **v3**（v1 阻塞下游 / v2 放行下游 / v3=新）。v3 逻辑：held（!AllSatisfied || AnyEscalated）时——查该被审 task 的 `revisionBudgetExhausted`（经新 Activity 或 AdversarialReviewForTaskResult 带回预算剩余标记）：
  - **预算剩余** → 派合成返工任务（Task3）+ **不** resolveReadyDownstream（返工收敛中，下游等）；
  - **预算耗尽** → **不**返工 → resolveReadyDownstream（v2/4.6 行为，demand 到 acceptance_pending 人类兜）。
  - AnyEscalated（budget escalate_human）/ reviewErr → 同耗尽路径放行至人类（不无限返工）。
- 建议让 `AdversarialReviewForTaskResult` 带回 `ReviewedTaskID` + `ReworkBudgetRemaining bool`（在 Activity 内查 revisionBudgetExhausted），workflow 据此决策，避免 workflow 层查库。
- **replay**：v3 从诞生即围栏；v1/v2 历史（dev 有 v2 workflow）行为不变；minSupported=DefaultVersion。旧 DefaultVersion 历史直连不变。
- [ ] Step1 失败测试：`TestAdversarialHeldWithBudgetTriggersRework`（held+预算剩余→建返工任务、不解锁下游）；`TestAdversarialHeldExhaustedReleasesToAcceptance`（held+耗尽→不返工、解锁下游至 acceptance_pending）；`TestAdversarialEscalateOrErrorReleasesToHuman`（escalate/err→放行人类不返工）。
- [ ] Step2 RED → Step3 实现 → Step4 全包 + **replay MANDATORY 报结果** → Step5 Commit `feat(coordination): 对抗held自动返工闭环——预算剩余返工/耗尽转人类(GetVersion v3)`

---

### Task 5:【GATE】真实自迭代 E2E（判决点）
**前置:** Task1-4 合入分支；control-plane 重启加载分支码（主 checkout 净化窗口，避并发）。
- [ ] Step1 全新 v3 项目 + adversarial_review 判据 satisfied_by 开发任务 → 真实弱产出完成 → 判官证伪 held。
- [ ] Step2 **自迭代（判决点）**：观测协调线程**自动建返工任务**（同员工，InputRequirements 含判官逐 lens 理由）→ 真实 agent 重跑 → 返工完成**重触发判官**（Task2 根 key 匹配）→ 再判。**PASS=无人参与下 held→自动返工带理由→重判**（control-plane.log 见返工派发 + 二次 AdversarialReviewForTask + DB 返工任务 RevisionOfTaskID 血缘 + InputRequirements 判官理由）。
- [ ] Step3 收敛/耗尽两支：产出改好→判官 satisfied→放行下游；或重复失败到预算耗尽→转 acceptance_pending 人类可覆盖（4.5）。
- [ ] Step4 记录 + Commit `docs(plan): Phase C1 自迭代闸门记录`

---

### Task 6: 收尾
- [ ] `corepack pnpm verify:control-plane`（+ sqlc 生成物一致）。
- [ ] 存量回归：无 adversarial 判据的返工/需求行为不变；4.6 held 行为在预算耗尽支保留。
- [ ] `$superteam-completion-check`；CHANGELOG（TZ=Asia/Shanghai）；memory 更新。
- [ ] 合并 main（ref 手术/ff，三查）+ 删分支/worktree（E2E 过后）。

## 实施记录
（实施时追加。）
