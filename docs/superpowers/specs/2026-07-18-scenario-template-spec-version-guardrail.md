# 场景模板 spec_version 缺失护栏 Spec

> 复核状态：已合并 main e021fce8

> 日期：2026-07-18
> 状态：已立项（用户拍板），待实施
> 性质：注册写入侧校验护栏，单模块小改（scenariotemplate），无迁移、无契约新端点。
> 来源：团队归属参与门禁 G4 补员 E2E 实测踩坑（CHANGELOG 2026-07-18 22:15 条目发现②）。

---

## 1. 缺陷（2026-07-18 真实踩坑复盘）

`scenariotemplate.ParseSpec`（spec.go:103）按 `spec_version < 2` 把 spec 交给 `normalizeV1` 归一化。v1 归一化**只认 v1 形态字段**（`roles[].independent_from`、`roles[].collapsible_with`、`risk_policy.release_requires_human`、字符串形态的 `default_acceptance_criteria`），对 v2 专属的顶层字段**静默丢弃**：

- `constraints`（role_independence / stage_required / human_gate 及其 `when.exit_at_or_beyond` 条件）
- `exits`（出口声明——丢掉后 `exit_at_or_beyond` 永不可判）
- `collapse_rules`（顶层形态）
- 对象形态的 `default_acceptance_criteria`（normalizeV1 只收 string 条目）

**实测事故链**：作者按 v2 形态写模板但忘写 `"spec_version": 2` → `POST /scenario-templates` 结构校验（ParseSpec）与词汇校验（validateSpecVocabulary，roles 在两条路径都保留所以照常通过）**全部绿灯 201** → 协调器治理（`EnforceScenarioTemplateGovernance` / `structuralGapForPlan`）看到的 spec 零约束 → 单员工池身兼申请人+审批人的计划**直接通过规划**，职责分离约束静默失效。作者全程无任何信号可察觉。

危害定性：这不是易用性瑕疵，是**治理约束静默失效**——模板作者以为立了 SoD/人闸/独立性约束，运行时根本不存在。与 CLAUDE.md"场景差异通过服务端注册校验表达"的定位直接冲突：注册侧放进了一个与作者意图相反的对象。

## 2. 存量审计（2026-07-18 实测）

- `scenario_templates` 主表 8 行全部 `spec_version: 2` 且带 constraints/exits——**存量无违规**。
- `scenario_template_versions` 历史行中的 v1 行（software_delivery/ops_analysis/incident_response/research_report/generic 各自的 version 1）是**真 v1 形态**（不含任何 v2 专属键），属合法 legacy，归一化语义正确，不在打击面。
- 结论：护栏只需管住**新写入**；不需要数据迁移，不需要动 ParseSpec 的运行时归一化行为（存量 v1 读路径保持不变）。

## 3. 方案：写入侧拒绝"v2 形态但未声明版本"

**判定函数**（scenariotemplate 包内，纯函数）：raw spec 满足 `specVersion(raw) < 2` 且命中以下任一，即判为"v2 形态但未声明版本"：

1. 顶层存在 `constraints` / `exits` / `collapse_rules` 任一键；
2. `default_acceptance_criteria` 存在非 string 条目（对象形态判据）。

**拦截点**（两处，均为已有校验链的前置一步）：

- `Service.Create`（service.go:87 ParseSpec 之前）
- `Service.CreateVersion`（service.go:156 ParseSpec 之前）

命中即返回 `ErrInvalidInput` 包装的 400，错误消息中文、可执行：
`spec 包含 v2 字段（constraints/exits/…）但未声明 "spec_version": 2；v1 归一化会静默丢弃这些字段导致治理约束失效。请补 "spec_version": 2 后重试。`

**明确不做**：

- 不做自动推断升级（"检测到 v2 键就按 v2 解析"）——写入侧静默改写作者声明与本缺陷同构，且会改变 ParseSpec 对既有存量的读语义，风险大于收益；拒绝+明说是唯一无歧义姿态。
- 不动 `ParseSpec` / `normalizeV1` 运行时行为——真 v1 存量的读路径零变化；护栏只挡新写入。
- `PatchScenarioTemplate` 若现状不接收 spec 变更（实测 PATCH 未更新 spec）则不加拦截；实施时核实，若它接收 spec 则同样前置判定。

## 4. 验证要求

单测（scenariotemplate 包）：

- v2 形态无版本号（带 constraints）→ Create/CreateVersion 均 400，消息含 `spec_version`；
- 真 v1 形态（只有 independent_from 等 v1 字段，无 v2 键）→ 照常 201，归一化行为不变（含 independent_from → role_independence 合成回归断言）；
- `spec_version: 2` 完整 v2 → 照常 201；
- 对象形态 default_acceptance_criteria 无版本号 → 400。

真实 E2E（轻量，单接口层）：dev 环境 curl 三连——无版本号 v2 形态 400 / 补版本号 201 / 真 v1 形态 201。本护栏为纯写入侧校验，不涉运行链路，按 CLAUDE.md 轻量验证例外之上限执行（定向测试 + 真实 API 三连即可，不需协调器全链）。

## 5. 关联与后续（非本项范围）

- template_governance.go:247 的 TODO(governance)：review 独立性迁移判别子的能力键子串启发，未来以显式 `review_of` / `review_independence` 约束字段取代——与本护栏同属"模板作者意图必须显式声明"方向，彼时可一并考虑 `spec_version` 的强制化（所有新写入必须 ≥2，v1 只读）。
- 飞书 connector 联调进程以 admin 自动回写决策干扰 dev E2E（同日发现③）——独立环境治理问题，另行处理。
