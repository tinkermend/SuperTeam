# 场景模板注册表（角色选角、可行性判定与事实性置信度）

- 状态：待评审（设计决策已于 2026-07-13 会话对齐，未实现）
- 日期：2026-07-13
- 范围：把"这类场景该怎么干"从 planner 的硬编码万能 prompt 中抽出，沉淀为租户内可注册的场景模板映射；模板以**角色契约集合**驱动规划，规划期做**选角可行性判定**——员工体系不满足硬约束的项目在规划期被拒绝或升级人类，而不是派下去饿死。
- 前置：`2026-07-13-handoff-contract-execution-loop-design.md` **必须先行**。契约执行闭环不存在时，模板沉淀的默认契约同样无人执行——只是把"每次现编的想象"升级成"预制的想象"。
- 宪法约束：核心模型**不定义封闭的项目类型枚举**；场景差异通过场景模板、Workflow Template、项目画像、标签、Policy 和服务端注册校验表达。本 spec 是这句话的第一个实现。

## 1. 问题

1. **`template_key` 是无人消费的标签。** planner 输出 schema 要求它（`openai_compatible_planner.go:276`），deepseek/heuristic 都会填（如 `heuristic.multi_executor_fanout`），但全仓无任何代码读取它——不解析到注册表、不驱动任何行为。
2. **真正驱动 plan 的是一段硬编码万能 prompt**（`openai_compatible_planner.go:275-283`）。所有需求——运维分析、软件开发、调研——共用同一副骨架现场分解、现场发明契约。这是全平台唯一的、烧死在代码里的"隐形模板"。
3. **声明没有沉淀层。** 交接契约、验收判据每次由 LLM 凭空生成，质量不稳定（07-13 E2E 第三次运行的计划"本计划未声明验收判据"），跑通过的场景经验无处沉淀。
4. **可行性失败暴露太晚。** 项目员工体系撑不起需求形态（如只有 1 名员工却需要独立审查）今天要到执行期才以"员工饿死等上游"的形式暴露，靠补做循环兜底——最贵的失败点。平台没有机制在规划期判定"这个项目干不了这类活"。

## 2. 目标与非目标

**目标**

1. `scenario_templates` 租户内注册表：**内容有限（企业内就几类），机制开放（加一类 = 插一行数据，不改核心代码）**。
2. 模板 spec 注入 planner prompt，planner 从"凭空发明"变为"按模板实例化填参"；`template_key` 输出被服务端解析校验，第一次有消费方。
3. 规划期**选角可行性判定**：三档出口（通过 / 降级需人确认 / 拒绝 + 结构化缺口报告）。
4. **事实性置信度**：feasibility 由服务端从事实计算；LLM 的 `selection_confidence` 降级为参考信号，不再是裁决。

**非目标**

- 不做模板可视化编辑器/市场（先 seed + API）。
- 不做验收判据与 verdict 绑定（intent spec；本 spec 只让模板携带默认判据文本进入 plan_acceptance_criteria）。
- 不做跨租户模板共享。
- 不做执行期动态换模板。

## 3. 架构决策

### 3.1 映射表，不是代码枚举

企业内模板分类事实上就几种（软件开发、运维分析、故障排查、调研报告…），但实现形态必须是数据：

```
scenario_templates: id, tenant_id, key, name, description, spec(jsonb), status, created_at, updated_at
  UNIQUE(tenant_id, key)
```

种子四个（`software_delivery` / `ops_analysis` / `incident_response` / `research_report`），另保留一个 `generic` 兜底模板——其 spec 近似今天的万能 prompt 行为，保证未选模板/未匹配场景的需求不退化。

### 3.2 模板 = 角色契约集合，对应机制 = 选角

模板骨架声明的不是"要几个人"，而是**角色**。spec jsonb 结构：

```json
{
  "roles": [
    {
      "key": "developer", "title": "开发",
      "required_capabilities": ["code_implementation"],
      "task_kinds": ["develop"],
      "collapsible_with": ["tester"],
      "independent_from": []
    },
    {
      "key": "reviewer", "title": "审查",
      "required_capabilities": ["code_review"],
      "task_kinds": ["review"],
      "collapsible_with": [],
      "independent_from": ["developer"]
    }
  ],
  "skeleton": [
    {"task_kind": "develop", "role": "developer", "produces_defaults": [{"name": "head_commit", "kind": "git_commit"}, {"name": "branch_ref", "kind": "branch_ref"}]},
    {"task_kind": "review",  "role": "reviewer", "depends_on": ["develop"], "required_outputs_defaults": [{"name": "head_commit", "kind": "git_commit"}], "produces_defaults": [{"name": "review_verdict", "kind": "conclusion"}]}
  ],
  "default_acceptance_criteria": ["…"],
  "risk_policy": {"release_requires_human": true},
  "budget_profile": {"max_tokens_per_task": null},
  "resource_requires": [],
  "feasibility_thresholds": {"pass": 0.8, "degrade": 0.5}
}
```

规划 = 把角色绑定到项目员工池里的真实员工（casting）。默认交接契约按 task_kind 从模板取，planner 只做实例化与增补——上一份 spec 的 required_outputs/produces schema 在此获得沉淀层。

### 3.3 可折叠与不可折叠约束：一个员工 ≠ 直接拒绝

真实世界一人小作坊也能交付软件——只是没有独立审查。规则不是"人数 ≥ 阶段数"，而是模板声明降级规则：

- `collapsible_with`（软约束）：开发与测试可同员工兼任；折叠后**降级事实写进 plan 给人看**。
- `independent_from`（硬约束）：审查者 ≠ 开发者（四眼原则）、`release_requires_human`。硬约束缺口**不允许静默降级**。

### 3.4 三档出口：拒绝要发生在规划期

选角可行性判定的出口，与真实世界组织一致：

1. **全覆盖** → 正常实例化派发。
2. **缺口但可折叠** → 按声明折叠角色，plan 中显式标注降级（`requires_human_review=true`），人确认后执行——符合宪法"人类决策一等对象"。
3. **硬约束缺口** → **规划期直接拒绝**，产出结构化缺口报告：缺哪个角色、需要什么能力、三条出路（补员工 / 换模板 / 负责人显式豁免）。复用 `invalidRouteDecision(reason…)` 家族与需求驳回可诊断机制。

这把"员工体系不满足"的暴露点从执行期（饿死 → 补做兜底）前移到规划期（一次任务都不派）。

### 3.5 事实性置信度：自评不是证据

planner 已输出 `selection_score` / `selection_confidence` / `requires_human_review`——LLM 自己给自己打分，属自述。可信的 feasibility 由服务端从事实计算：

```
feasibility = f(
  角色覆盖率     模板角色中能绑定到 ≥1 名合格员工的比例（硬约束角色权重更高）
  能力匹配度     employee planning_profile vs 角色 required_capabilities
  容量          available_slots / 并发上限
  runtime 就绪   provider 在线、节点可达（复用既有 readiness 检查）
  历史履约       该员工同 task_kind 的完成率 / 驳回率（结论复利的第一个消费方）
)
```

阈值三档写进模板 `feasibility_thresholds`（可被租户 Policy 覆盖）。LLM 的 `selection_confidence` 保留为其中一路参考信号。挂载点：`LoadProjectCoordinationSnapshot` 已在挑人前做池过滤（readiness、借调闸门），可行性判定是同类机制前移一步、变成 template-aware。

### 3.6 绑定点与选择权

- 模板在**项目创建时**选定，落到项目画像；需求级可覆盖（一个软件项目也会有调研型需求）。
- 谁选：人选；或 planner 依据需求内容推断候选、**人确认**——AI 不自定场景。
- 未选定时用 `generic` 兜底模板，行为与今天一致（渐进迁移）。

### 3.7 planner 注入与 template_key 闭环

选定模板的 spec 注入 planner prompt，**替换**硬编码万能 prompt 中骨架/契约/判据对应段落（选人规则、输出格式等平台不变量保留硬编码）。planner 输出的 `template_key` 必须等于注入模板的 key——不一致或无法解析 → 拒绝。`template_key` 这个字符串第一次有了消费方与校验闭环。

## 4. 数据与接口

- 迁移：`scenario_templates` 表 + 种子数据（遵循 DATABASE_DESIGN.md，UUID-first、租户列、atlas.sum 更新）。
- API：`GET/POST /api/v1/scenario-templates`、`GET/PUT/DELETE /api/v1/scenario-templates/{key}`（ConsoleUserAuth + authz 动作 `scenario_template.read/manage`）；项目创建请求增加可空 `scenario_template_key`；契约改动走生成与验证流程。
- 项目画像：`projects` 增加可空 `scenario_template_key` 列（或落 coordination_policy jsonb，评审时定）。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| 需求未绑模板 | `generic` 兜底，行为与今天一致 |
| planner 输出 template_key ≠ 注入模板 | 拒绝，reason 带两个 key |
| 模板被禁用/删除但项目仍引用 | 项目沿用快照语义：规划时解析失败 → 回落 generic + warn + 事件 |
| 硬约束缺口 | 拒绝 + 缺口报告（不派任何任务） |
| 历史履约数据不足（冷启动） | 该信号权重置零，不惩罚新员工 |
| feasibility 阈值缺失 | 平台默认阈值兜底 |

## 6. 测试策略

**单元（Go）**：模板 spec 反序列化与校验；选角（全覆盖/可折叠/硬约束缺口三档）；feasibility 计算各信号与冷启动；template_key 校验闭环。

**真实 E2E（完成的必要条件）**：

1. 种子 `ops_analysis` 模板 + 单员工项目提运维分析需求 → 计划按模板骨架分解，任务 handoff_contract 含模板默认 required_outputs，`template_key` 落库且可解析。
2. `software_delivery` 模板 + 单员工项目 → 独立审查硬约束缺口 → 计划被拒，缺口报告在 Web 需求详情可读（复用需求驳回诊断展示）。
3. 同项目补入第二名具备 review 能力的员工 → 同需求重提 → 计划通过，审查任务绑定第二名员工。
4. 未绑模板项目 → generic 兜底，行为与现状一致（回归护栏）。

## 7. 分期

- **P1**：注册表 + 种子模板 + planner 注入 + template_key 校验闭环 +（依赖前置 spec 的）默认契约实例化。
- **P2**：选角可行性判定三档出口 + 事实性 feasibility（先四个事实信号，历史履约可后补）。
- **P3**：从真实跑通的闭环里**蒸馏**新模板（模板管理 UI、从成功项目导出骨架）——模板应从真实闭环里蒸馏，而不是先验设计；种子模板本身也应在 P1/P2 跑通后按真实运行修订。

## 8. 后续

- intent spec：模板 `default_acceptance_criteria` 升级为 criterion 对象（verification_method 路由、人类确认生效）。
- 外环 spec：`risk_policy` 接审批队列与预算熔断。
- 内环 spec Phase 2：`resource_requires` 接项目资源池（repo 绑定一般化）。
- 结论复利：历史履约信号是其第一个消费方，反向定义了"任务收敛后要蒸馏哪些结构化结论"。
