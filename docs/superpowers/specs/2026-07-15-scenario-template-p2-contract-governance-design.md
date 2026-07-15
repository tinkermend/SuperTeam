# 场景模板 P2 修订版：契约化、需求级引用与出口剪枝

- 状态：待评审
- 日期：2026-07-15
- 锚定：取代 `2026-07-13-scenario-template-registry-design.md` §7 中原 P2 的范围定义（原 P2 = 选角可行性判定 + 事实性 feasibility），并吸收 2026-07-15 会话对模板模型的五项修正。P1 已落地部分（注册表、种子、planner 注入、template_key 校验、Console 三触点）不动摇，本文在其上升级。
- 破坏性变更授权：本项目为进行中开发项目，允许破坏性变更（项目级模板绑定的语义变更、spec jsonb 结构升级、确认门策略变更均在授权范围内）。
- 核心目标对齐：① 差异化 = 治理契约化（约束服务端强制 + 审计血缘），不是三模式形态本身；② 关键操作人类确认 = 确定性双层模型（模式定发起侧、能力档位定执行侧），不做内容分类放行；③ 复杂度 = 每模式一个人类触点的硬预算。

## 0. 与 07-13 spec 的模型差异（本次修订基线）

| # | 07-13 spec / P1 现状 | 本修订 | 理由 |
|---|---|---|---|
| 1 | 模板在项目创建时绑定，全项目需求共用 | **需求级引用**，项目列降级为默认值 | 规划的真实单位是需求；项目级绑定事实上把模板变成封闭的项目类型枚举，违背宪法 |
| 2 | spec jsonb 六类字段混装一袋，约束与默认值不分 | **constraints（服务端强制）与 defaults（实例化素材）显式分离** | 四眼原则、人类门是治理不变量，必须是契约；骨架、默认判据是素材，planner 可改编 |
| 3 | 范围变体无建模（同形态不同交付深度靠换模板或不管） | **出口 = 交付物剪枝**：合法出口是骨架自身的 produces 名，选定终点后按 DAG 祖先闭包裁剪阶段 | 不发明跨场景通用档位词汇；机制从结构派生，对所有形态自动通用 |
| 4 | 计划人类确认条件式（policy 触发才等人） | **Plan 模式全量强制确认**，废除内容分类放行 | "什么算低风险只读"不可判定；确认语义定死于模式，执行侧硬门交给能力风险档位（独立立项） |
| 5 | 无版本语义，绑 key 不绑版本，模板修改静默影响后续规划 | **版本化快照**：规划时解析 active 版本并钉进计划 payload（key@version） | 审计一等平台的注册表对象必须有变更血缘 |

## 1. 问题（2026-07-15 代码核查结论）

P1 之后模板本质仍是"高级提示词"，不是"平台契约"：

1. **约束零消费。** `independent_from`（四眼）、`collapsible_with`（折叠降级）、`risk_policy.release_requires_human`、`feasibility_thresholds` 在全部 Go 代码中无消费方——种子模板描述里"审查者与开发者必须不同人"无任何机制兑现。
2. **骨架遵循只到 prompt。** 全部注入浓缩在一段 prompt 指令（`openai_compatible_planner.go:287`）；服务端唯一校验是 template_key 相等（`graph_validation.go:60`）。planner 丢骨架步骤、改 produces 名，平台不知情（P1 E2E 首轮即发生丢字段，靠加固 prompt 补救）。
3. **能力词汇无锚。** 模板 `required_capabilities` 与员工 planning_profile 的 capability key 是两处自由字符串，靠 LLM 模糊对齐；07-13 E2E 因此自评置信度 0.3–0.6，用项目级阈值旋钮硬放行——LLM 自评仍是事实上的裁决。
4. **只读注册表。** 仅两个 GET 端点（`server.go:454-455`）；`scenario_template.manage` authz 常量已立未接线；"加一类场景 = 插一行数据"实际是运维直插 DB。
5. **无版本、无变更审计。** 计划确认卡答不了"当时按哪一版模板规划"。
6. **项目级绑定框死需求形态。** 软件项目里的调研需求被迫走软件骨架或全项目 generic；spec §3.6 承诺的需求级覆盖未实现。
7. **模板解析失败只 warn 日志**，无项目事件（P1 修订 #2 遗留）。

## 2. 目标与非目标

**目标**

1. 模板 spec v2：constraints / defaults 分离；约束由服务端在计划校验时强制。
2. 需求级模板解析（人显式 > planner 推断 + 人确认 > 项目默认 > generic），版本快照钉入计划 payload。
3. 出口 = 交付物剪枝：模板标注合法出口，planner 提议、人在确认卡修正，服务端校验剪枝后骨架完整性。
4. 选角可行性判定三档出口（通过 / 折叠降级需人确认 / 拒绝 + 结构化缺口报告），feasibility 由服务端从事实计算，LLM 自评降级为参考信号。
5. 缺口报告的"补员工"出路接通：从标准员工模板（`digital_employee_templates`）一键实例化进项目池（人确认）或走团队借调。
6. 能力词汇表：租户级注册表 + 服务端校验，锚定模板角色要求与员工画像的语义对齐。
7. 版本化 + 管理 API + 变更审计 + `scenario_template.manage` 接线（不设审批门——2026-07-15 已拍板"版本+审计+权限"档）。
8. Plan 模式计划确认全量强制；一张确认卡承载：模板、出口、计划、折叠降级标注、约束触发说明。
9. 差异化可见：确认卡逐条显示"此门由哪条约束触发"；血缘可查（template key@version → 计划 → 约束评估记录）。

**非目标**

- 意图层 criterion 对象化（verification_method 路由、逐条 verdict 绑定）——intent spec 恢复后消费本文的出口与默认判据挂载点。
- Loop 信封的完整实现——本文只保证约束引擎可被"每轮迭代评估"消费、`budget_profile`/constraints 可作为信封默认值来源。
- 能力风险档位执行门（chat 高危拦截）——独立立项，归属 capability 域；本文只在约束事件词汇中预留 `capability_risk_tier` 引用。
- 模板可视化编辑器、从成功项目蒸馏模板（仍是 P3）。
- 命令层实时拦截、模板变更四眼审批门（显式否决/后置）。

## 3. 核心模型

### 3.1 spec v2：constraints 与 defaults 分离

```jsonc
{
  "spec_version": 2,
  // ---- defaults：实例化素材，planner 按需求改编，人确认时可改 ----
  "roles": [
    {"key": "developer", "title": "开发", "required_capabilities": ["code_implementation"]},
    {"key": "reviewer",  "title": "审查", "required_capabilities": ["code_review"]}
  ],
  "skeleton": [
    {"step": "develop", "role": "developer",
     "produces_defaults": [{"name": "branch_ref", "kind": "branch_ref"}, {"name": "head_commit", "kind": "git_commit"}]},
    {"step": "review", "role": "reviewer", "depends_on": ["develop"],
     "required_inputs_defaults": ["head_commit"],
     "produces_defaults": [{"name": "review_verdict", "kind": "conclusion"}]},
    {"step": "release", "role": "developer", "depends_on": ["review"],
     "produces_defaults": [{"name": "release_record", "kind": "evidence_ref"}]}
  ],
  "exits": [
    {"deliverable": "branch_ref",     "label": "交付分支（不合入）"},
    {"deliverable": "review_verdict", "label": "审查通过并合入"},
    {"deliverable": "release_record", "label": "发布上线"}
  ],
  "default_acceptance_criteria": [
    {"statement": "变更以 branch+commit 交付", "applies_from_exit": "branch_ref"},
    {"statement": "通过独立审查", "applies_from_exit": "review_verdict"}
  ],
  "budget_profile": {},          // defaults：Loop 信封与预算估计的出厂值
  "collapse_rules": [            // defaults：折叠是"默认可接受但须显式标注"的降级
    {"roles": ["developer", "tester"]}
  ],
  // ---- constraints：治理不变量，服务端强制，违反即拒绝或强制人类门 ----
  "constraints": [
    {"kind": "role_independence", "roles": ["reviewer", "developer"],
     "when": {"exit_at_or_beyond": "review_verdict"}},
    {"kind": "stage_required", "step": "review",
     "when": {"exit_at_or_beyond": "review_verdict"}},
    {"kind": "human_gate", "target": "release",
     "when": {"exit_at_or_beyond": "release_record"}}
  ],
  "feasibility_thresholds": {"pass": 0.8, "degrade": 0.5}
}
```

要点：

- **约束的判定材料是计划的结构化事实**（选定出口、阶段集合、选角结果），在计划校验时确定性评估——不做任何内容分类，与 2026-07-15 拍板的确定性原则一致。执行期事件（真实 merge/发布动作）的拦截属能力风险档位立项，不在本文。
- `constraint.kind` 与 `when` 条件走**服务端注册表校验，非封闭枚举**（接宪法）。P2 种子三种 kind：`role_independence` / `stage_required` / `human_gate`；条件先只支持 `exit_at_or_beyond`（对骨架 DAG 上出口交付物的拓扑序比较）。
- `collapsible_with` 改名为 defaults 侧的 `collapse_rules`：折叠永远合法但必须在计划上显式标注降级事实（`requires_human_review` 置位由此来，而非 LLM 自评）。
- 旧 spec（v1）迁移：`independent_from` → `role_independence` 约束；`risk_policy.release_requires_human` → `human_gate` 约束；`collapsible_with` → `collapse_rules`；`default_acceptance_criteria` 字符串 → 对象（无 `applies_from_exit` 限定 = 适用于所有出口）；种子五模板随迁移升级为 v2。

### 3.2 需求级引用与解析顺序

- 解析顺序：**需求显式指定 > planner 从需求内容推断候选（人在确认卡改） > 项目默认（`projects.scenario_template_key`，语义降级为默认值） > generic**。
- 需求（demand）增加可空 `scenario_template_key`；规划快照装载时按上述顺序解析出 key，再解析 active 版本，`template_key` + `template_version` 一并钉入 `PlanRevisionPayload`。
- `projects.scenario_template_key` 保留列但更新注释与消费语义：仅作解析顺序第三级的默认值，不再是约束（破坏性语义变更，已授权）。
- 模板/版本解析失败：回落 generic + warn + **项目事件**（补 P1 遗留债）。

### 3.3 出口 = 交付物剪枝

- `exits` 标注骨架 produces 名中的合法出口并给人话标签；**单出口模板不出现选择器**（复杂度预算）。
- planner 依需求内容提议出口；人在计划确认卡上一行"本次交付到：\<label\>"可改选。
- 剪枝 = 选定出口交付物所属步骤在骨架 DAG 上的祖先闭包；剪枝后集合是本次计划的必含骨架。
- **范围修订方向性**：计划执行中把出口向后推（扩张）= 新授权，必须重新过确认卡；向前收缩走轻量确认。复用既有 plan revision 机制，payload 记录出口变更方向。
- 需求本身歧义、连出口都提不出时：planner 提议以"澄清/分析"前置阶段的交付物为出口的第一段计划——出口机制天然覆盖"先搞清楚再定范围"的迭代推进，不强迫发起时定准。

### 3.4 服务端契约化（graph validation 扩展）

计划校验（`graph_validation.go` 家族）新增，全部确定性：

1. **骨架遵循**：计划任务集必须覆盖剪枝后骨架的每个 step；step 对应任务的 produces 必须逐字含该 step 的 produces_defaults 名。违反 → `invalidRouteDecision`（复用需求驳回诊断机制）。
2. **约束评估器**：输入（出口、阶段集、选角映射 role→employee_id），逐条评估 constraints：
   - `role_independence` 违反（同一员工被排进两个须独立的角色）→ 拒绝；
   - `stage_required` 违反 → 拒绝；
   - `human_gate` 命中 → 对应任务强制 `requires_human_approval=true`（planner 不可覆盖）。
3. **折叠标注**：命中 `collapse_rules` 的选角必须携带降级标注，计划确认卡显示；缺标注 → 拒绝。
4. 每条拒绝/强制动作产出结构化理由（约束 ID + 触发条件 + 涉及对象），落审计并供确认卡展示——差异化可见性的数据来源。

### 3.5 选角可行性三档与补员出路

- **事实性 feasibility**：复用 predispatch gate 已有积木（`PlanningProfileScore` 的 matched/missing capabilities、hard failures，`planning_profile.go:127` 起），前移到规划期、按模板角色聚合成覆盖率；信号：角色覆盖、能力匹配、容量（available_slots）、runtime 就绪；历史履约信号本期权重置零（冷启动规则保留）。LLM `selection_confidence` 降级为参考信号，不再是裁决。
- **三档出口**：
  1. 全覆盖 → 正常实例化；
  2. 软缺口 → 按 `collapse_rules` 折叠 + 降级标注 → 确认卡人确认；
  3. 硬缺口（约束不可满足）→ 规划期拒绝 + 结构化缺口报告：缺哪个角色、需要什么能力（词汇表 key）、三条出路。
- **缺口报告三出路接通**：
  a. **从标准员工模板补员**：平台种子若干标准角色员工模板（代码审查员、测试员；能力声明使用词汇表 key），缺口报告提供一键"实例化进本项目员工池"，**必须人确认**——实例化后是正常项目员工（有归属、预算、权限边界、出现在员工列表），出身特殊、治理不特殊；
  b. 团队借调（机制已有）；
  c. 负责人显式豁免：**豁免是一等决策记录**（绑约束 ID + 生效范围 = 本需求 + 决策人 + 时间），不是确认卡上的勾选；落审计，血缘可查。
- 同源模型的独立性诚实声明：数字员工间的 `role_independence` 是上下文/会话/角色隔离意义上的独立，弱于人类独立；高风险真门仍是 `human_gate`。

### 3.6 能力词汇表

- 新表 `capability_vocabulary`（tenant 级：key、title、description、status），种子收录当前种子模板与标准员工模板引用的全部 key。
- 服务端校验：模板写入/升版时 `required_capabilities` 引用的 key 必须存在；员工 planning profile 的 capability key 不在词汇表时进 `selection_warnings`（不硬失败，渐进收敛）。
- casting 的能力匹配以词汇表 key 精确对齐，消除 LLM 模糊对齐。
- 归属：先独立小表落在 scenariotemplate 域相邻位置；与 capability 域（外部能力注册）的合并留待该域演进时评审（待决策项 #1）。

### 3.7 版本化与管理 API

- 版本形态：`scenario_template_versions` 不可变版本表（template_id、version 单调递增、spec jsonb、created_by、created_at）；`scenario_templates` 主表保留当前元数据 + `active_version` 指针。规划时解析 active 版本；进行中计划钉住已解析版本不受后续修改影响。
- 管理 API：`POST /scenario-templates`（建模板 = 建 v1）、`POST /scenario-templates/{key}/versions`（升版）、`PATCH /scenario-templates/{key}`（启停/元数据）；全部挂 `scenario_template.manage` authz（接线既有常量）+ 变更审计事件（谁、何时、diff 摘要）。不设审批门（已拍板）。
- Console：目录页从只读升级为管理页（新建/升版/启停 + 版本历史视图）；沿用 v3 组件与紧凑列表形态，改动前读 `DESIGN.md`。

### 3.8 确认语义与触点预算（2026-07-15 拍板）

- **Plan 模式：计划确认全量强制**——废除 `RequiresHumanReview` 条件式放行的"不满足即自动派发"路径；该标志保留为确认卡上的"降级/风险提示"信息位。不做任何基于内容分类的自动放行。
- **一张确认卡**承载全部规划期人类决策：场景模板（可改选）、出口（可改选）、计划任务、折叠降级标注、约束触发说明（"发布任务已强制人类审批：由 human_gate@software_delivery v3 触发"）。
- **触点预算不变量**（写进本 spec，后续设计遵守）：chat 0 个强制发起触点 / plan 1 张确认卡 / loop 1 次信封定义 + 审批队列。任何设计要新增强制门，必须先从同模式减一道。
- Chat 高危拦截、Loop 信封实现均不在本文；本文交付的约束评估器须可被两者复用（纯函数式：事实进、裁决出，不耦合 Plan 流程）。

### 3.9 边界声明：场景模板 vs Workflow Template

- **场景模板 = 治理契约 + 规划先验**：作用于"计划如何生成与校验"——骨架是给 planner 的素材与服务端的校验基线，约束是计划必须满足的不变量。计划仍由 planner 按需求内容动态生成。
- **Workflow Template = 确定性编排**：作用于"固定流程的执行"，不经 planner 动态分解。
- 一个需求走且只走其一。场景模板不得演进为自由阶段编排器（出口档位是模板作者预定义的合法出口集合，不是任意组合）；Workflow Template 不承载角色身份约束与验收判据。越界需求出现时回本节评审，不得默默扩权。

## 4. 数据与接口

- 迁移（一个迁移文件族，编号顺延，遵循 `DATABASE_DESIGN.md`，更新 `atlas.sum` 并 `make -C apps/control-plane migrate-validate`）：
  1. `scenario_template_versions` 表 + 主表 `active_version` 列 + 存量五种子生成 v1 版本行；
  2. 种子 spec v1 → v2 结构升级（constraints/defaults 分离、exits 标注）作为 v2 版本行，active 指向 v2；
  3. 需求表增加可空 `scenario_template_key`；`projects.scenario_template_key` 注释更新为默认值语义；
  4. `capability_vocabulary` 表 + 种子；
  5. 豁免决策记录复用/扩展既有人类决策记录表（评审时对齐 authzcenter 决策模型，避免新造平行表）。
- sqlc + `generate:control-plane` + OpenAPI（管理端点、需求级模板字段、计划确认卡响应扩展：exits、约束触发说明、缺口报告、降级标注）；契约改动走 `verify:contracts`。
- `PlanRevisionPayload` 增加 `template_version`（omitempty 保历史指纹稳定，沿用 P1 的 template_key 做法）。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| 需求/项目均未绑模板 | generic 兜底，行为与现状一致 |
| 模板 key 或 active 版本解析失败 | 回落 generic + warn + 项目事件 |
| planner 输出计划缺剪枝后骨架步骤 / produces 名不符 | `invalidRouteDecision`，理由带步骤名与期望 produces |
| 选角违反 role_independence / stage_required | 拒绝 + 结构化缺口报告（约束 ID + 涉及对象 + 三出路） |
| human_gate 命中但 planner 未置人类审批 | 服务端强制置位，不视为错误 |
| 折叠发生但无降级标注 | 拒绝 |
| 模板 required_capabilities 引用词汇表外 key | 模板写入/升版被拒 |
| 员工画像 capability 不在词汇表 | casting 记 selection_warnings，不硬失败 |
| 出口扩张修订未过确认卡 | 拒绝修订 |
| 标准员工模板实例化未经人确认 | 无此路径——实例化 API 仅由确认动作触发 |
| 进行中计划遇模板升版 | 不受影响（钉住已解析版本）；下一次规划用新版本 |

## 6. 测试策略

**单元（Go）**：spec v2 反序列化与 v1 兼容迁移；约束评估器逐 kind（含 `exit_at_or_beyond` 拓扑比较）；剪枝闭包计算；三档 casting（全覆盖/折叠/硬缺口）与 feasibility 聚合、冷启动；版本解析与钉住；词汇表校验。

**真实 E2E（完成的必要条件，走真实 Web + Control Plane + DB + Runtime + Provider）**：

1. **需求级引用**：绑 `software_delivery` 的项目发一条调研需求并显式选 `research_report` → 计划按调研骨架生成，payload 记录 key@version。
2. **出口剪枝**：同一软件需求，确认卡选"交付分支（不合入）" → 计划仅含 develop，无审查任务且不触发四眼；改选"审查通过并合入" → 计划含 review，选角 reviewer≠developer 被校验。
3. **硬缺口拒绝 + 补员闭环**：单员工项目选"审查通过并合入" → 规划期拒绝，缺口报告在 Web 可读；一键从标准员工模板实例化审查员（人确认）→ 重提 → 计划通过且审查任务绑定新员工。
4. **豁免血缘**：同场景负责人显式豁免四眼 → 计划通过，豁免决策记录可查（约束 ID + 决策人 + 范围）。
5. **版本钉住**：升版模板（改约束）→ 进行中计划不受影响，新规划按新版本；变更审计事件与版本历史在 Console 可见。
6. **全量确认回归**：低风险计划（原本自动派发路径）也停在确认卡等人。
7. **骨架校验兜底**：构造 planner 丢骨架步骤的输出 → 服务端拒绝且诊断可读（可用测试 planner 注入）。
8. **generic 回归护栏**：未绑模板需求行为与现状一致。

## 7. 分期

- **P2a（契约化地基）**：spec v2 迁移与版本化、需求级解析、出口剪枝、骨架遵循校验、约束评估器（三种 kind）、计划确认全量强制、确认卡扩展（模板/出口/约束触发说明）。
- **P2b（选角与补员）**：事实性 feasibility 三档、缺口报告、词汇表、标准员工模板种子与一键补员、豁免决策记录、管理 API + Console 管理页。
- P2a 不依赖 P2b 可独立交付（约束评估器在无 casting 数据时对 role_independence 按"计划声明的选角"评估）。每期收尾走真实 E2E 与 `$superteam-completion-check`。

## 8. 待决策项

1. `capability_vocabulary` 最终归属（独立表 vs 并入 capability 域注册表）——P2b 实现前定，倾向先独立、后合并。
2. 豁免决策记录的存储对齐（复用 authzcenter 决策记录 vs project 域新表）——迁移设计时与 authzcenter 模型核对后定。
3. 标准员工模板种子范围（首批：代码审查员、测试员；安全审查员是否入首批）。
4. `exit_at_or_beyond` 之外的约束条件词汇（如按能力风险档位触发）——留待能力风险档位立项后扩展，本期不做。

## 9. 跨 spec 关系

| spec | 关系 |
|---|---|
| `2026-07-13-scenario-template-registry-design.md` | P1 基座；本文取代其原 P2 范围，P3（蒸馏）不变 |
| `2026-06-30-intent-acceptance-criteria-design.md`（暂停中） | 消费本文的出口与 `default_acceptance_criteria.applies_from_exit` 挂载点，将文本判据升级为 criterion 对象；恢复评审时以本文为前置 |
| `2026-06-30-autonomous-outer-loop-*.md` | 约束评估器与 `budget_profile` 是 Loop 信封的默认值来源；返工迭代（收敛的迭代）与出口修订（范围的迭代）在本文 §3.3 划清 |
| 能力风险档位执行门（待立项，capability 域） | Chat 高危拦截与执行期事件约束的正解；本文 §3.1 预留条件词汇扩展点 |
| `2026-07-13-handoff-contract-execution-loop-design.md` | 骨架 produces/required_inputs 的执行语义来源；本文的骨架校验建立其上 |
| Workflow Template 体系 | 边界见 §3.9，一个需求走且只走其一 |
