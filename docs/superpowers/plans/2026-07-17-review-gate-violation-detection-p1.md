# 违反检测门 P1 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 把审核/门控从"证明正确性"重构为"检测违反明确条件"——检出违反才 hold（按档位），检不出（证不了伪也证不了真）默认放行，人类只在最终验收确认。落地 spec `2026-07-17-review-gate-violation-detection-design.md` P1。

**Architecture:** 引入**条件检测器**抽象（规则型 + LLM-prompt 型），指向 attestation 真工件（被审任务 Contract 的 diff/deliverables），输出 `{detected, severity, finding, evidence_refs}`。收敛闸对**检测门判据**默认反转（检出违反才 hold，否则放行）。`adversarial_review` 降格为该框架下的一个 LLM-prompt 检测器且默认放行；C1 自动返工默认关。

**Tech Stack:** Go（Temporal + 直连 LLM chatCompletionClient）/ 真实 deepseek E2E。

**Spec:** `docs/superpowers/specs/2026-07-17-review-gate-violation-detection-design.md`。**已拍板**：配置粒度 a（预置标准条件 + 项目开关调档 + 受约束自定义槽，自定义槽本 P1 不做）；默认放行翻转;C1 自动返工降默认关。

**关键锚点（已核，main 7f8ee255）：**
- 收敛闸 `ResolveUnsatisfiedBlockingCriteria`（project/demand_acceptance_gate.go:158）：现 `!hasVerdict` 即 held（held-by-default，:165 adversarial / :182 其余）。`criterionEffectiveVerdict`（:84）。常量 :49-62（method/judge_type 镜像）。
- 判官读工件 `adversarialEvidenceFromResult`（projectcoordination/adversarial_trigger.go:377，取被审任务 latestTaskResult 的 Contract summary/deliverables/evidence_refs）；触发 `AdversarialReviewForTask`（:171 起）；引擎 `runAdversarialReview`（adversarial_review.go）；直连 LLM `chatCompletionClient.CreateChatCompletion`（openai_compatible_planner.go:62）。
- policy 读取：`adversarialJudgeCount`（adversarial_trigger.go:368）、`requireHumanAcceptance`（acceptance_criteria.go:138），源 `coordination_policy` map。
- verification_method 注册 `knownVerificationMethods`（acceptance_criteria.go:19）。
- C1 自动返工触发 workflow.go GetVersion `adversarial-review-trigger` v3（held+预算→返工）。

## Global Constraints
- 根级 `corepack pnpm`；Go 定向 `cd apps/control-plane && go test ./internal/...`；禁 npx。
- 改 workflow.go 命令序列必跑 replay 并 GetVersion 从诞生围栏；无则 Activity 层改动跑 replay 为证。
- **隔离 worktree 基于 main 开 feat/review-gate-p1**（并发会话可能活跃）；只在 worktree 干活、只 add 本任务文件、禁 -A/. 禁 git stash；合并 ref 手术/ff、删前三查核对 was sha。
- 每任务提交尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- **边界纪律（spec §1/§10）**：detector 只输出"检出违反/未检出 + 证据"，**不出综合分、不判正确性**；LLM-prompt detector 的 system 必须框成"检测特定违反类"，未检出/不确定→detected=false。代码注释引用 spec §1 边界表。

---

### Task 1: 条件检测器抽象 + 规则型实现（TDD）
**Files:** Create `apps/control-plane/internal/workflow/projectcoordination/detector.go` + test。
**Interfaces:** Produces
```go
type DetectionArtifact struct { Summary string; Deliverables []string; EvidenceRefs []string; DiffText string } // 被审任务真工件
type DetectionResult struct { Detected bool; Severity string; ConditionKey string; Finding string; EvidenceRefs []string } // Severity: block|major|minor
type ConditionDetector interface { Key() string; Detect(ctx context.Context, art DetectionArtifact) DetectionResult }
```
- 规则型 `RuleDetector{key, severity, match func(DetectionArtifact) (bool, finding)}`。首个规则条件：**密钥泄漏**（`secret_leak`）——正则扫 DiffText/Deliverables 命中常见密钥形态（`sk-`、`AKIA`、`-----BEGIN.*PRIVATE KEY`、`password\s*=\s*["'][^"']+`）→ detected + severity=block + finding 指出命中片段（脱敏）。
- 未检出→`DetectionResult{Detected:false}`。
- [ ] Step1 失败测试：`TestSecretLeakDetectorFires`（diff 含 `sk-xxxx`→detected block）；`TestSecretLeakDetectorCleanReleases`（无密钥→detected=false）。
- [ ] Step2 RED→Step3 实现→Step4 全包+build→Step5 Commit `feat(coordination): 条件检测器抽象+规则型(密钥泄漏门)`

---

### Task 2: LLM-prompt 型检测器（TDD）
**Files:** Modify detector.go（加 `LLMPromptDetector`）+ test。
**Interfaces:** Consumes `chatCompletionClient`。Produces `LLMPromptDetector{key, severity, systemPrompt, client}`——system 框成"**检测这份 diff 是否违反 <条件>；只输出 JSON {detected, finding}；不确定或未检出→detected=false；不要评价整体好坏/打分**"；user=真工件（diff/deliverables/evidence_summary）。解析 JSON；解析失败→detected=false（默认放行方向，与安全 detector 相反：检测门宁放不误拦，安全门另说见 Task 3 档位）。
- 首个 LLM 条件：**代码缺陷/安全 review**（`code_review`）——扫 diff 有无明显缺陷/漏洞类，检出 major。
- 诚实：注释标注这是检测特定违反类、非正确性证明（spec §1）。
- [ ] Step1 失败测试（fake client）：`TestLLMDetectorParsesDetected`（返回 detected=true→DetectionResult detected）；`TestLLMDetectorCleanOrParseFailReleases`（detected=false 或非法 JSON→detected=false）；`TestLLMDetectorPromptFramedAsViolation`（断言 system 含"检测/违反"不含"打分/正确"）。
- [ ] Step2 RED→…→Commit `feat(coordination): LLM-prompt型条件检测器(检测违反非证明正确)`

---

### Task 3: 预置条件注册表 + 配置层（TDD）
**Files:** Create `detector_registry.go` + test。
**Interfaces:** Produces
```go
type ConditionSpec struct { Key string; Kind string /*rule|llm*/; DefaultAction string /*block|need_human|record_only*/; Detector ConditionDetector }
func standardConditions(client chatCompletionClient) []ConditionSpec // 预置: secret_leak(rule,block) + code_review(llm,need_human)
func enabledConditions(policy map[string]any, all []ConditionSpec) []ConditionSpec // 读 coordination_policy.review_gate_conditions: {key: action}
```
- policy 形态 `coordination_policy.review_gate_conditions = {"secret_leak":"block","code_review":"need_human"}`（缺省=用 DefaultAction；值="off"=禁用该条件）；`review_gate_minor_tolerance`(int, 容忍几个 minor 不 hold)。仿 `adversarialJudgeCount` 读法。
- [ ] Step1 失败测试：`TestEnabledConditionsReadsPolicyAndAction`（policy 开关+调档生效、off 禁用、缺省用 DefaultAction）；`TestStandardConditionsPresent`（secret_leak/code_review 在册）。
- [ ] Step2 RED→…→Commit `feat(coordination): 预置条件注册表+coordination_policy配置层`

---

### Task 4: 检测门执行 + verdict 投影（TDD）
**Files:** Modify adversarial_trigger.go（把 `AdversarialReviewForTask` 泛化为 `RunReviewGateForTask`，或加平行 `RunReviewGateForTask`），project 侧 verdict 写入。
**Interfaces:** 被审任务完成→组装 DetectionArtifact（复用 `adversarialEvidenceFromResult` + 若 evidence_refs 有 code_change/diff 则取 DiffText）→对 enabledConditions 逐个 Detect→聚合：任一 detected 且 action∈{block,need_human}（超 minor 容忍）→写一条 **violation verdict**（criterion unsatisfied + finding/evidence 入 reason/evidence_refs + action 档记入）；全未检出→写 **clean verdict**（satisfied）或不写（由 Task5 闸门按"检测门无 violation=放行"处理）。
- 复用 Phase B 的 verdict 落库（CreateAdversarialVerdict/明细）或新建轻量 review_gate verdict——倾向复用 demand_criterion_verdicts（judge_type 记 `review_gate`，verdict=satisfied/unsatisfied）。
- [ ] Step1 失败测试：`TestReviewGateDetectsViolationWritesUnsatisfied`（密钥命中→unsatisfied+finding）；`TestReviewGateCleanWritesSatisfied`（无检出→satisfied）；`TestReviewGateMinorToleranceReleases`（仅 minor 且在容忍内→satisfied）。
- [ ] Step2 RED→…→Commit `feat(coordination): 检测门执行——真工件跑detector+violation/clean verdict投影`

---

### Task 5: 收敛闸默认反转（检测门判据检出才 hold，否则放行）（TDD，承重）
**Files:** Modify project/demand_acceptance_gate.go（`ResolveUnsatisfiedBlockingCriteria` :158）。
**Interfaces:** 新 verification_method `review_gate`（注册 knownVerificationMethods + 镜像常量）。闸门语义**反转**：对 `review_gate` 判据——**仅当存在 violation verdict(unsatisfied) 才 held**；`!hasVerdict`（检测门还没跑/未检出）→**不 held、放行**（与现有"无 verdict=pending"相反）。human 覆盖仍优先；`adversarial_review` 判据本 P1 一并改为 review_gate 语义（默认放行，Task6 处理迁移）。**automated_test/human_judgment 语义不变**（执行接地/阶段最终验收仍 held-by-default）。
- [ ] Step1 失败测试：`TestReviewGateNoVerdictReleases`（review_gate 判据无 verdict→不在 pending）；`TestReviewGateViolationHolds`（unsatisfied→pending）；`TestHumanJudgmentStillHeldByDefault`（回归：human_judgment 无 verdict 仍 pending）；`TestAutomatedTestUnchanged`（回归）。
- [ ] Step2 RED→…→Step4 全包+replay→Step5 Commit `fix(project): 收敛闸对review_gate判据默认反转——检出违反才hold否则放行`

---

### Task 6: adversarial_review 降格 + C1 自动返工默认关（TDD）
**Files:** adversarial_trigger.go / workflow.go / acceptance_criteria.go。
**Interfaces:**
- `adversarial_review` verification_method → 作为 review_gate 的一个 LLM-prompt 检测条件接入（或标注等价语义走 Task5 反转）；其收敛语义即 Task5 的"检出才 hold、默认放行"。多判官保留为该条件的可选实现（降检测方差），语义是检测违反非证明正确。
- **C1 自动返工默认关**：workflow.go v3 held→返工 分支加 policy 门 `coordination_policy.auto_rework_on_violation`（默认 false）；false 时 held（检出违反）→不自动返工、走人类最终验收（放行下游至 acceptance_pending 由检出违反 inform 人类，复用 4.6/4.5）；true 时才走 C1 自动返工。GetVersion 是否升 v4 视是否改命令序列而定（加 policy 分支若改 ExecuteActivity 走向需围栏+replay）。
- [ ] Step1 失败测试：`TestAutoReworkDefaultOffReleasesToHuman`（auto_rework 缺省→held 不返工、放行至验收）；`TestAutoReworkOnWhenEnabled`（policy true→走 C1 返工）；replay 绿。
- [ ] Step2 RED→…→Step4 全包+replay(MANDATORY 报)→Step5 Commit `feat(coordination): adversarial_review接入检测门+C1自动返工降默认关(policy开)`

---

### Task 7:【GATE】真实违反检测门 E2E（判决点）
**前置:** Task1-6 合入分支；control-plane 重启加载；主 checkout 窗口。
- [ ] Step1 **检出违反→拦**：项目开 secret_leak(block)+code_review(need_human)；真实任务产出含密钥的 diff→检测门 secret_leak 命中→demand held+finding 可查（psql verdict judge_type=review_gate unsatisfied + reason 指出密钥）。
- [ ] Step2 **无检出→默认放行零人类**（判决点）：真实任务产出干净 diff（无密钥、code_review 未检出违反）→检测门全 clean→**demand 默认放行、不 held、零人类触点**(对比旧 Phase B 会 held 等判官/人类)。**PASS=干净产出真默认放行 + 脏产出真被拦**。
- [ ] Step3 **人类最终验收 inform**：被拦的 demand 到 acceptance_pending，人类看到检出违反、签署放行或驳回（复用 4.5）。
- [ ] Step4 记录+Commit `docs(plan): 违反检测门P1闸门记录`

---

### Task 8: 收尾
- [ ] `corepack pnpm verify:control-plane`；存量回归（automated_test/human_judgment 语义不变、无 review_gate 判据的需求行为不变）。
- [ ] `$superteam-completion-check`；CHANGELOG（TZ=Asia/Shanghai）；memory 更新；spec 提交入 git。
- [ ] 合并 main（ref 手术/ff，三查）+ 删分支/worktree（E2E 过后）。

## 实施记录
（实施时追加。）
