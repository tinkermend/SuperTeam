# 意图与验收标准（完整版）：让"收敛"有人类锚定的定义

- 状态：待评审
- 日期：2026-07-16
- 取代：`2026-06-30-intent-acceptance-criteria-design.md`（提纲）。提纲的核心论点不变——attestation 保证"真的跑了"，acceptance criteria 才保证"跑的是对的"；本文按 2026-07-15/16 已落地的现实（场景模板 P2a/P2b、attestation、计划确认全量强制）重写落地路径。
- 前置已全部就位：模板 v2 `default_acceptance_criteria`（对象形，`applies_from_exit` 挂出口）；出口=交付物剪枝；plan 模式计划确认全量强制（人类锚的现成挂点）；`project_task_attestations` + "通过 verification 必须引用 attestation" 契约（`task_result_contract.go:501` 家族）；planning_gap/豁免决策通道。

## 0. 与提纲的关键差异（现实修正）

1. **挂载单位 = 需求（demand），不是 Work Item**。Work Item 是外环提纲对象、未实现；P2b 之后 demand 已是事实上的收敛单位（重开/重规划/缺口决策都以 demand 为轴）。将来外环引入 Work Item 时挂载点上移。
2. **不新造"Acceptance Spec 确认门"**。`PlanAcceptanceCriterion{ID, Statement, SatisfiedBy}` 已存在于计划修订 payload（`plan_revision_payload.go:43-53`），随计划版本化、指纹化、经 pending_review 人类确认——**人类确认判据 = 批准计划**，一张卡，零新增触点（守 P2a 触点预算不变量）。本文做的是给这个既有对象补语义（verification_method/severity）、补判定通道、补持久化与血缘。
3. **不新造 Attestation**。判据的证据要求直接复用既有 attestation 契约与 `TaskResultAcceptanceResult{CriterionID, Status, EvidenceRefs}`（契约已强制 completed 结果逐条提交，`task_result_contract.go:111-120, 416-422`）。
4. **verification_method Phase 1 只有两种**：`automated_test`（机器可判，verdict 证据必须含 attestation 引用）与 `human_judgment`（人类必判，签署即证据）。`review`/`external_check` 及判定者独立路由是 Phase 2（07-15 已拍板）。

## 1. 问题（现状精确缺口）

1. **判据无语义**：`PlanAcceptanceCriterion` 只有 statement 文本——没有 verification_method（谁来判、凭什么判）、没有 severity（阻断/非阻断）。"通过"的含义仍由执行员工自述。
2. **verdict 不可查询**：逐条判据结果埋在 `project_task_results` 的 JSON blob 里，无法回答"需求 X 的判据 #2 是谁在何时凭什么证据判过的"。
3. **没有人类判定通道**：所有 acceptance_results 由执行任务的员工自报。`human_judgment` 类断言（"这个方案业务上可接受"）无处安放——要么塞给 agent 自判（自己出卷自己判），要么散落在计划外。
4. **没有收敛判定**：任务完成 ≠ 需求做对。当前 demand 的"完成"由任务图终态推导，从不核对"所有阻断级判据是否全部有满足的 verdict"。
5. **验收记录不引用判据**：`project_acceptance_records` 只有裸 evidence_ref_ids 数组，验收结论无法回溯到逐条标准。
6. **歧义不拦截**：目标写不出可判定断言时，planner 照样产出模糊判据（07-13 E2E 曾出现"本计划未声明验收判据"），跑到最后才发现没有标准。

## 2. 目标与非目标

**目标（Phase 1）**

1. `PlanAcceptanceCriterion` 补语义字段：`verification_method`（注册表校验，Phase 1 注册 automated_test/human_judgment）、`severity`（blocking/non_blocking）、`evidence_hint`（给判定者的提示，可空）。
2. 判据草拟规则：模板默认判据（按选定出口过滤 `applies_from_exit`）⊗ planner 按需求内容增补；**每需求至少一条 blocking `human_judgment` 判据**（默认注入"人类负责人确认交付符合需求意图"，Policy 可豁免——07-15 拍板 #2）。
3. 人类确认 = 计划确认（既有卡，判据区变为可读语义展示：method 徽标 + severity + satisfied_by）；判据修改走既有 request_changes 重规划通路（07-15 拍板 #3 的版本化语义由计划修订天然承载：新修订 = 新判据版本，旧 verdict 对旧修订）。
4. **逐条 verdict 持久化**：新表 `demand_criterion_verdicts`——任务结果落库时，从 acceptance_results 抽取 criterion 维度写入（judge=执行员工，evidence_refs，attestation 引用校验对 automated_test 收紧）；human_judgment 判据的 verdict 由人类在需求完成评审时签署。
5. **收敛判定**：demand 进入完成态前核对所有 blocking 判据均有 satisfied verdict；human_judgment 未签 → 需求停在人类评审（复用既有 waiting_human/完成评审通道），评审界面逐条呈现签署。
6. **血缘展示**：workflows 需求详情增加判据面板——逐条 statement/method/severity/verdict 状态/判定者/证据链接（attestation/transcript 深链）。
7. **歧义拦截（轻量）**：planner 产出的判据 statement 经启发式判定性检查（空/过短/含"尽量、适当"类模糊词）→ 计划确认卡该条标黄提示人工改写；不阻断（人类确认本就全量强制，拦截=让人看见）。

**非目标（Phase 1 明确不做）**

- `review`/`external_check` 方法与审查员/连接器判定路由（Phase 2）。
- 判据复用沉淀/复利（Phase 3，接外环支柱 D）。
- Work Item 对象、返工迭代状态机（外环 spec 范围；本文的"未过判据带证据回灌"输出为其预留接口）。
- `project_acceptance_records`（项目级验收）的改造——Phase 1 收敛判定在 demand 级；项目级验收引用 demand 判据汇总留 Phase 2。
- LLM 语义判定歧义（启发式够 Phase 1；语义判定属过度设计）。

## 3. 核心模型

### 3.1 判据对象（扩展既有 PlanAcceptanceCriterion）

```go
type PlanAcceptanceCriterion struct {
    ID                 string   `json:"id"`
    Statement          string   `json:"statement"`
    SatisfiedBy        []string `json:"satisfied_by"`                  // 任务 key（automated_test 必填；human_judgment 可空=需求级）
    VerificationMethod string   `json:"verification_method,omitempty"` // automated_test | human_judgment（注册表校验）
    Severity           string   `json:"severity,omitempty"`            // blocking | non_blocking（缺省 blocking）
    EvidenceHint       string   `json:"evidence_hint,omitempty"`
    AmbiguityFlag      bool     `json:"ambiguity_flag,omitempty"`      // 服务端启发式标注，卡上提示
}
```

- 向后兼容：旧 payload 缺新字段 → method 缺省 `automated_test`、severity 缺省 `blocking`（读取端缺省，不迁移存量）。
- 注册表：`knownVerificationMethods`（Go 注册表，同 constraint kind 模式：新增方法 = 加判定通道实现）。
- 指纹：新字段全部纳入计划指纹（判据语义变更 = 计划实质变更，须重新确认）。

### 3.2 草拟与人类锚

```
规划时（openai planner prompt 扩展 + 服务端补全）：
  模板 default_acceptance_criteria 按选定出口过滤（applies_from_exit ≤ exit）
    → 逐条实例化为 criterion（模板判据缺省 automated_test/blocking；statement 原样）
  ⊗ planner 按需求内容增补判据（prompt 指令：断言必须可判定，声明 method 与 satisfied_by）
  ⊗ 服务端注入兜底 human_judgment 判据（若 planner 未产出任何 human_judgment 且 Policy 未豁免）：
      {statement: "人类负责人确认交付符合需求意图", method: human_judgment, severity: blocking}
服务端校验（graph_validation 扩展）：
  method 在注册表内；automated_test 判据的 satisfied_by 非空且引用真实任务 key（既有校验收紧）；
  歧义启发式打 AmbiguityFlag。
人类确认 = 批准计划（确认卡判据区展示语义 + 黄标歧义项；改判据 = request_changes 重规划）。
```

### 3.3 verdict 持久化与判定通道

新表 `demand_criterion_verdicts`：

```
id, tenant_id, project_id, demand_id, plan_revision_id, criterion_id (payload 内 ID),
verdict (satisfied | unsatisfied), judge_type (executor | human), judge_id,
evidence_refs JSONB, project_task_id*, created_at
唯一性（注意 NULL 语义）：执行侧 UNIQUE(tenant_id, demand_id, plan_revision_id, criterion_id, project_task_id) WHERE project_task_id IS NOT NULL；
人类签署侧 partial UNIQUE(tenant_id, demand_id, plan_revision_id, criterion_id) WHERE project_task_id IS NULL —— 两条 partial index，防 NULL 使唯一约束失效。
```

- **automated_test 通道（执行侧）**：任务结果落库（既有 `acceptance_results` 强校验通过后），服务端把每条 result 投影为 verdict 行（judge=executor+员工 id）。收紧：criterion.method==automated_test 时，该条 evidence_refs 必须含 attestation 引用（复用 `verificationHasAttestationRef` 判别逻辑），否则任务结果校验失败（走既有 rejected+waitHuman）——把"绿灯必须挂真实执行记录"从任务级 opt-in（requires_runtime_attestation）落到判据级强制。
- **human_judgment 通道（人类侧）**：需求所有任务终态后、demand 完成前，若存在未签署的 blocking human_judgment 判据 → 创建 `demand_acceptance` DecisionRequest（复用 P2b 三件套模式，TargetUserID=human_owner，进 inbox），完成评审界面逐条呈现（statement + 关联任务产出/证据汇总），人类逐条 签署满足/不满足（+理由）→ verdict 行（judge=human）。全部满足 → demand 完成；有不满足 → demand 转 rejected 终态 + 结构化"哪条判据未过+理由"事件（外环返工的预留输入）。
- **收敛判定**：demand 完成态推进逻辑增加判据闸——所有 blocking 判据存在 satisfied verdict（对当前生效 plan_revision）。non_blocking 未过仅记录不阻断。

### 3.4 血缘展示

- workflows 需求详情：判据面板（method/severity 徽标、verdict 状态、判定者、证据深链——attestation → 既有 transcript/attempt 证据面）。
- inbox `demand_acceptance` 决策项：动作 = 打开评审界面（深链），签署在界面内完成（非 inbox 一键——逐条判定需要看证据，防橡皮图章）。
- 审计链闭合：`需求 → 计划修订(人类确认, 含判据版本) → criterion → verdict(judge+evidence) → attestation/签署 → demand 完成/驳回`。

## 4. 数据与接口

- 迁移（编号顺延）：`demand_criterion_verdicts` 表（全中文 COMMENT，规则同 DATABASE_DESIGN.md）。
- 契约：任务结果契约不变（acceptance_results 已含所需字段）；新增 demand 判据面板读端点（criteria+verdicts 汇总）与签署端点（`POST /project-demands/{demandId}/criterion-verdicts`，仅 human_judgment、仅 pending `demand_acceptance` 决策存在时、authz 校验签署人）；openapi + codegen + verify:contracts。
- planner prompt：判据产出指令扩展（可判定断言/method/satisfied_by/至少考虑人类判据）。
- Console：确认卡判据区语义化；需求详情判据面板；完成评审界面。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| planner 判据 method 不在注册表 / automated_test 缺 satisfied_by | 计划校验拒绝（invalidRouteDecision，回灌重规划） |
| automated_test 判据 verdict 缺 attestation 引用 | 任务结果校验失败 → 既有 rejected+waitHuman 通路 |
| 员工对不存在的 criterion_id 报 result | 既有契约校验已拒（acceptance_result_missing 家族反向） |
| human_judgment 判据被员工自报 satisfied | 投影时忽略（该 method 的 verdict 只收 judge=human），事件留痕 |
| 人类签署不满足 | demand → rejected 终态 + 结构化未过判据事件（返工预留） |
| 签署接口越权/重复签署 | authz 403 / UNIQUE 幂等（重复签同值幂等，改判需先撤销——Phase 1 不支持撤销，签错走需求重提） |
| 存量计划（无新字段） | 读取端缺省 automated_test/blocking，行为与现状一致（无 human_judgment 判据 → 不触发签署门，收敛判定按 executor verdict——迁移期护栏） |

## 6. 测试策略

**单元（Go）**：判据草拟补全（模板过滤/兜底注入/Policy 豁免）；校验扩展（method 注册表/satisfied_by/歧义启发式）；verdict 投影（attestation 收紧/幂等/human_judgment 拒自报）；收敛闸（blocking 全过/有未签/non_blocking 不阻断）。

**真实 E2E（完成的必要条件）**：

1. software_delivery 需求（合入出口）→ 确认卡判据区显示模板判据（method/severity 徽标）+ 兜底人类判据 → 批准 → 开发/审查任务真实执行，acceptance_results 投影为 verdict 行（psql）。
2. 全部任务完成 → inbox 出现 demand_acceptance 决策 → 评审界面逐条签署（一条满足）→ demand 完成；判据面板全链血缘可点（verdict → attestation → transcript）。
3. 签署"不满足" → demand rejected + 结构化未过判据事件落库。
4. automated_test 判据无 attestation 证据的结果被拒（构造 fixture）。
5. 歧义 statement（"尽量优化性能"）→ 确认卡黄标。
6. 存量回归：模板未带判据 + planner 未产人类判据 + Policy 豁免 → 行为与现状一致。

## 7. 分期

- **P1a（判据语义+人类锚）**：字段扩展/草拟补全/校验/确认卡语义区/歧义标注。
- **P1b（verdict 通道+收敛+血缘）**：verdict 表与投影/attestation 收紧/demand_acceptance 决策与签署界面/收敛闸/判据面板。
- P1a 独立可交付（判据有语义且人类确认），P1b 让它闭环。Phase 2/3 见非目标。

## 8. 待决策项

1. 兜底 human_judgment 的 Policy 豁免粒度（租户/项目/场景模板）——倾向项目 coordination_policy 键，实现期定。
2. `demand_acceptance` 签署界面的形态（独立页 vs 需求详情内嵌区）——DESIGN.md 评审时定，倾向内嵌。
3. loop/chat 模式需求是否走判据闸（倾向：机制 mode 无关，但 loop 的签署节奏属 Loop 信封 spec，Phase 1 loop 需求如产生 human_judgment 判据照常进 inbox）。

## 9. 跨 spec 关系

| spec | 关系 |
|---|---|
| 场景模板 P2（已落地） | 消费其出口挂载（applies_from_exit 过滤）、确认卡、模板判据对象、决策三件套模式 |
| 外环提纲（06-30） | 本文的"未过判据+证据"结构化事件是支柱 A 返工的输入；attestation 契约（支柱 B）被判据级收紧 |
| Loop 信封（未立项） | 判据闸 mode 无关；loop 签署节奏归其管 |
| 交接闭环（07-13，已落地） | acceptance_results/deliverables 契约是 verdict 投影的数据源 |

> 一句话：P2 让计划"按对的方式生成"，本文让完成"有人类锚定的定义"——判据经人批、verdict 可查询、人类判的归人类、收敛有闸、血缘可点。

## 10. P1 实现记录与 Phase 2 跟进（2026-07-16 落地）

P1 全量实现并合入（10 任务，全链血缘 E2E 闸门 PASS：需求真实 HOLD 在 acceptance_pending，automated 判据绿灯挂服务端核实的真实 attestation，签署完成/驳回双径全通）。E2E 与终审揪出并修的关键设计缺陷：
- **attestation 必须服务端核实**：guard 原设计要求员工把 attestation 引用自报进 acceptance_results 不可行（runtime 在 writeback 时才铸造、员工不可知）；改为服务端核实该 attempt 确有 succeeded 的 project_task_attestations 行并自动附引用。
- **not_applicable 破除死锁**：执行员工对注入的 automated 判据返回 not_applicable（合法契约值）曾致需求永卡 acceptance_pending（无 verdict、任务不失败、人类无法签自动判据）；改为投影成非阻断 `not_applicable` verdict，闸门放行，强制兜底人类判据仍是背板。

**Phase 2 跟进（本次记录，未做）：**
1. **豁免项目 N/A 硬化**：`acceptance_human_judgment_exempt` 项目若自撰 automated 判据，执行员工可全部 N/A 化 → 无 attestation 无人审自动接受（等价意图层前基线，豁免即弃权；但自撰判据的期望被绕过）。硬化方向：无人类背板时 N/A automated 判据仍要求 attestation，或显式声明豁免=全信执行侧自报。
2. **N/A 理由透传背板**：非豁免下人类签兜底判据时，面板未展示执行员工 N/A 某自动判据的理由（存于 verdict.reason，未进读模型）——弱化（不破坏）背板知情。
3. verdict 枚举 `not_applicable` 补进 openapi（本次因 openapi 被并发会话占用未改，手写路径不 gate 运行时无影响）；迁移 065 待 migrate-validate（仅注释零风险）。
4. 判据复用沉淀（Phase 3）；review/external_check 判定者路由（Phase 2 原定）；项目级验收引用 demand 判据汇总（Phase 2 原定）。
