# 意图规格与验收标准 Spec 提纲：让"收敛"有锚

> **已被取代（2026-07-16）**：完整版见 `2026-07-16-intent-acceptance-criteria-full-design.md`——按场景模板 P2a/P2b 落地后的现实重写（挂载单位改 demand、复用既有 PlanAcceptanceCriterion 与计划确认门、attestation 已存在无需新造）。本提纲保留作论证背景。

> 日期：2026-06-30
> 复核状态：部分实现（P1完成，P2/P3未启动）
> 状态：提纲（待展开评审）
> 锚定：本文填补内环 spec（`2026-06-29-project-code-workspace-runtime-affinity-design.md` §12）与外环 spec（`2026-06-30-autonomous-outer-loop-iteration-attestation-budget-design.md` §9）共同标记的 **gap #1（最大语义空洞）**。内环让系统能跑、外环让它闭合可信，但两者都在对一个**未被定义的目标**收敛。本文定义"什么算做对了"。

## 0. 为什么需要这份

外环讲"全 verdict pass → 人类验收 → 收敛"，但没人定义 **acceptance criteria 从哪来、怎么流到审查/测试员、人类凭什么验收**。后果：你能有完美的执行证明（测试退出码 0），却**不知道跑的是不是对的东西**。

> attestation 保证"真的跑了"；**acceptance criteria 才保证"跑的是对的"**。没有这一层，attestation 与"收敛"全部悬空。

本文把"意图"变成一组**可判定的验收标准**，作为内外环共同的锚。

## 1. 核心对象：Acceptance Spec / Criterion

- **Acceptance Spec**：挂在 **Work Item**（外环的收敛单位）上的一组验收标准，等价于该 Work Item 的 **Definition of Done**。收敛 = 所有 criterion 满足。
- **Criterion（验收标准项）**：一条离散、可判定的断言。字段提纲：
  ```
  criterion:
    id
    statement            # 人读的断言："登录失败不再 500，返回 401 且有审计日志"
    verification_method  # 验证方式（来自注册表，非封闭枚举）：automated_test | review | human_judgment | external_check
    check_ref            # 自动类：测什么（测试选择器/命令）；审查类：审什么角度；人判类：判什么
    evidence_requirement # 需要哪种 attestation 支撑（退出码/覆盖率/diff/外部回执）
    severity             # 阻断 / 非阻断
    owner_role           # 由哪类判定者负责（测试员/审查员/人类负责人）
  ```
- **不引入封闭类型枚举**：`verification_method` 走注册表 + 服务端校验（接宪法「以注册表为准、不依赖封闭枚举」）。

## 2. 意图 → 标准的分解与授权（关键：人类确认，非 AI 独断）

- **意图源**：Project 目标 + 人类负责人。Work Item 承接目标的一个切片。
- **草拟**：plan / 协调线程从目标 + 场景模板**生成草稿 criteria**。
- **授权（一等人类决策）**：**criteria 必须经人类负责人确认/编辑后才生效**，尤其 `human_judgment` 类与"上线/删除/迁移"相关项。AI 可建议，不可自定验收门槛——否则等于自己给自己出卷又自己判卷。
- 存储：控制平面，挂 Work Item；下发时切片进每个判定者任务的派工单。

## 3. 验证方法分类（决定派给谁、verdict 怎么来）

| verification_method | 判定者 | verdict 来源 | 闭合证据 |
|---|---|---|---|
| automated_test | 测试员 | 测试命令退出码/覆盖率 | 外环支柱 B 的 attestation |
| review | 审查员 | 审查意见（按 check_ref 角度） | diff + 引用的 attestation |
| human_judgment | 人类负责人 | 验收签署（业务判断） | 汇总证据包 |
| external_check | 连接器/能力 | 外部回执 | 外部调用 attestation |

## 4. 与外环的闭合：criterion → verdict → attestation

把外环支柱 A/B 接上锚：

```
Work Item.acceptance_spec = [criterion...]
  每轮 Iteration:
    判定者任务 verdict 必须声明：{criterion_id, satisfied?, evidence=attestation_ref}
    收敛条件 = 所有「阻断级」criterion 的 verdict.satisfied 且证据有效
  全满足 → 人类验收（仅 human_judgment 项需人签）→ converged
  未满足 → 把"哪条 criterion 未过 + 证据"作为返工输入 → 下一轮
```

- **verdict 必须绑定 criterion_id 与 attestation**——这把外环"verdict 必须引用 attestation"再收紧一层：不仅要有证据，还要对应到**哪条标准**。
- 返工不再是模糊的"再改改"，而是**"criterion #3 未过、证据为 X"** 的精确回灌。

## 5. 与场景模板 / 项目目标的关系（默认标准）

- **场景模板可带默认 criteria**：如"代码变更"场景默认 `[测试通过, 审查批准, 无 lint 错误]`；"故障闭环"场景默认 `[根因有证据, 修复可复现, 回归不再现]`。接内环 Phase 2 的 Workflow Template `output_contract`。
- 具体 Work Item = 模板默认 criteria ⊗ 本次目标的特定 criteria，人类负责人确认。

## 6. 歧义处理（接宪法"需求歧义必须暂停"）

- 草拟 criteria 时若目标**无法判定**（statement 写不出可检验断言）→ **标记歧义、暂停、向人类澄清**，在执行**之前**解决，而非跑到验收才发现没标准。
- 即：**acceptance-criteria 授权环节，也是歧义的拦截点。**

## 7. 可追溯 / 审计血缘

```
Project 目标 → Work Item.acceptance_spec → criterion
   → 判定者 verdict(criterion_id, evidence) → attestation
   → 人类验收签署 → converged
```
全链可审计：任何"通过"都能回溯到"哪条标准、什么证据、谁签的"。直接强化你们的审计一等地位。

## 8. 落地分期

- **Phase 1**：Acceptance Spec 最小结构（criterion: statement + verification_method + severity）；verdict 绑定 criterion_id + attestation_ref；人类负责人确认 criteria 才生效。直接让外环的"收敛"有定义。
- **Phase 2**：场景模板默认 criteria；automated/review/human/external 四类判定者路由；歧义拦截。
- **Phase 3**：criteria 复用与沉淀（接外环支柱 D：好的验收标准本身是可复利的知识）。

## 9. 待决策项

1. Acceptance Spec 挂 Work Item 还是也允许挂单个 ProjectTask（倾向 Work Item 级，task 级继承）。
2. `human_judgment` 与自动项的最小配比强制：是否要求每个 Work Item 至少一条人类签署项（防全自动绕过人类）。
3. criteria 变更治理：执行中途人类改 criteria 如何影响已收集 verdict（重判哪些）。
4. `verification_method` 注册表归属（Capability / Policy）。

## 10. 跨 spec 关系

| spec | 职责 | 与本文的关系 |
|---|---|---|
| 内环（06-29） | 执行基座：员工/工作区/调度 | 提供 worktree 与 attestation 的物理来源 |
| 外环（06-30） | 闭合：返工/证明/预算/复利 | 消费本文的 criteria 作为收敛目标 |
| 能力缓存/Auth 边界（06-30） | 员工能力 manifest 与 Provider 认证边界 | 本文可要求 evidence 记录 `capability_manifest_version`，但不编码 `agent_home_dir`、`CODEX_HOME`、`OPENCODE_CONFIG_DIR` 等物理路径 |
| **本文（意图层）** | **定义"做对了"** | **锚**：无此层，内外环对未定义目标收敛 |

> 一句话：内环让它**能跑**，外环让它**闭合可信**，意图层让它**朝对的目标闭合**。三者缺一，自治跃迁不成立。

> 边界补充：Acceptance criteria 只定义要满足的业务/技术断言与所需证据类型；它可以约束“证据必须来自某次 attestation 且记录所用能力 manifest 版本”，但不能依赖 Runtime 节点上的 Provider auth home 物理位置。这样外环当前实现与未来内环实现都只对齐到 task workspace + capability manifest/version，不会把员工能力缓存误当 Provider 认证目录。
