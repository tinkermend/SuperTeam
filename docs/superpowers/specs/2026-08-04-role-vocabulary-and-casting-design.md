# 批二：角色词表与编制（Role Vocabulary & Casting）

- 日期：2026-08-04
> 复核状态：状态行已过时（写"已立项、数据已预置，可直接开工"），实际已在commit 7b0369de+1f36c944全量实施入main（角色词表/员工角色多值/项目×剧本编制/扩编写路径），并被08-05-role-governance-console-design.md确认承接（"系列：批二 7b0369de+1f36c944"）。已知缺口§7.2判官生产者缺失，由08-05-semantic-casting-expansion-design.md补齐。建议将文件状态行更新为"已实施"并注明commit。
- 状态：**已立项、数据已预置，可直接开工**（交接给实施会话）
- 系列：剧本可落地化第二批（批一「能力词表两侧对齐 + 剧本卫生」已入 main `7a7064b5`）
- 交付性质：新增角色注册表 + 员工角色多值化（schema）+ 编制表（schema）+ 剧本选择器读路径 + 发起期编制界面 + 执行期扩编决策
- 目标读者：实施会话（本文自包含；实施前必读基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` §1/§4）
- **剧本本身一行不改**

---

## 0.0 开工须知（实施会话先读这一节）

**开工第一件事，不是写代码，是勘察一个未知：**

> 重规划时**已完成任务如何处理**——会不会重复创建、已消费的验收闸怎么算、已完成产物是否被新计划继承。
> 入口：`request_changes` 重规划的现有行为（`service.go` 的 `PlanReviewDecisionRequestChanges` → 协调线程重规划）。
> **必须先读懂再动 §7 的扩编**，不得凭猜实现。若发现现有行为本身就有缺陷，停下来报告，不要在其上叠加。

**环境**（服务由 `./scripts/dev-services.sh start|status|restart <service>` 管理，OpenFGA 需单独管）：

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100`（**不是 3000**） |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `POST /api/auth/login`，`admin` / `admin` |
| 数据库 | `apps/control-plane/config/config.yaml` 的 `postgres.url`（远程 dev 库） |
| 迁移校验 | 无 docker，需自建干净 PG16：`make -C apps/control-plane migrate-validate DEV_URL=...`；改迁移后必须 `atlas migrate hash` |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web` / `verify:runtime-agent`（**最后一个有既有并发 flake，单独跑可复现通过**） |

**踩过的坑，别重复**：

- `capability_vocabulary.status` 的 CHECK 只允许 `active|disabled`（写 `inactive` 会被拒）——新建 `role_vocabulary` 沿用同一取值
- 建员工时 `employee_type` 必须是已注册模板 type 或哨兵 `custom_agent`；`avatar_asset_id` 必填（如 `engineer-m-01`）
- 创建项目要求当前用户对该团队有 scope：`PUT /api/auth/users/{id}/project-team-scopes`
- Web 测试若新增了对 `@tanstack/react-router` 的 hook 依赖，**多个测试文件的 mock 都要同步补**，否则整文件导入失败

---

## 0. 已拍板（2026-08-04 讨论结论，不重议）

| # | 决定 |
|---|---|
| 1 | 剧本用 **exit 分档**表达深度，**不拆成多个剧本**（否则组合爆炸 + 约束重复 + 剧本承担两个正交含义） |
| 2 | **角色词表为主**：先注册角色，剧本 `roles[].key` 必须引用已注册角色 |
| 3 | 员工角色**多值**（一人可兼多角色） |
| 4 | 编制**一角色一人**；需要更细分就拆角色。同一人可占多角色（= 兼任，触发既有 collapse 注解升人工复核） |
| 5 | 编制是**项目 × 剧本**级的可复用配置，不是每次发起都要做的一步 |
| 6 | 编制选人**自动加入项目成员池**，且**必须由人操作** |
| 7 | 候选范围 = **租户内权限可见全体**；团队是**展示维度**，不是过滤门禁 |
| 8 | 剧本选择器**不置灰**，显示**可达最深收口 + 缺什么角色** |
| 9 | `required_capabilities` **降为提示与排序**，不做门禁 |
| 10 | 扩编：**任一任务完成后即可**提请（不必等验收），判官/协调线程提请、**人批准** |
| 11 | 扩编 = **带新编制的重规划**，不是给任务图打补丁；约束全部重新校验 |
| 12 | 人批准扩编**即确认计划**；但重规划结果**越界时退回人工确认一次**（§7.4） |
| 13 | 编制因运行期资源变化失效时 automation **允许失败但必须通知**（基线 §4.7 已按此修订） |

---

## 1. 背景与目标

### 1.1 问题

剧本定义了角色，但**角色在平台上没有归属**：

- 员工的 `role` 是**自由文本**（`varchar NOT NULL`，无词表）。清库前 19 个员工的取值是 `代码审查`/`developer`/`reviewer`/`开发`/`前端开发工程师`/`E2E smoke` 这种中英混杂、粒度不一的值。
- 剧本的 `roles[].key` 是**另一套词汇**（`developer`/`reviewer`/`tester`/`collector`/`analyst`/`diagnostician`/`operator`/`verifier`/`researcher`/`writer`）。
- **两者之间没有任何关联关系**——剧本匹配实际上从不读员工的 `role`。

于是产生两个后果：

1. **剧本要什么角色、项目里有没有，平台不检查。** 缺员拦截只在 `role_independence` 且活跃执行者 < 2 时触发；「剧本要 `log_analysis`、池子里没人有」完全不拦，照跑。
2. **执行中途发现需要剧本外的角色时，没有任何通路。** 上游补做只派给已有产出物的原 owner，不引入新员工；对抗返工只在原任务内打转。故障分诊做完发现"还要查网络"——平台接不住。

### 1.2 目标

1. 角色成为**一等注册表**，剧本与员工引用同一份词表。
2. 发起时**人为每个角色定人**（编制），可复用、可预填。
3. 剧本选择器诚实显示**这个剧本在这个项目能走到哪一档**。
4. 执行中途可**扩编**：判官/协调线程提请 → 人选人 → 带新编制重规划。
5. 能力从门禁降为**提示与排序**。

### 1.3 一句话

> **角色进注册表，编制由人定；剧本能走多深由编制决定，走不下去时可以中途扩编。**

---

## 2. 非目标

| 不做 | 理由 / 归属 |
|---|---|
| 改剧本 spec 结构（roles/skeleton/constraints/exits 语义） | 本批只改"谁来演"，不改"怎么演" |
| 让剧本表达分支/返工 | 岔路的本质是规划时信息不足，对应机制是**重规划**，不是更大的剧本 |
| 编制定一组人由 planner 挑 | 已拍板否决（责任边界会松、实现复杂度不成比例） |
| planner 提示注入**能力**词表 | 本批注入**角色**词表即可；能力已降为提示 |
| 剧本管理页面 UI 大改 | 已记 `TODO.md`，后期单独立项 |
| 让判官自行创建角色 | 词表外的需求降级为自然语言提请，由人翻译 |
| 硬性要求员工能力匹配才能被选 | 已拍板：人选，系统提示 |

---

## 3. 地基核对（2026-08-04 经代码与真实库核实，勿重复勘察）

| 事实 | 锚点 |
|---|---|
| 能力词表已存在且两侧校验 | `capability_vocabulary`（列 `vocab_key/title/description/status`，status 仅 `active|disabled`）；模板侧 `scenariotemplate.Service.validateSpecVocabulary`，员工侧 `employee.Service.validateDeclaredCapabilities`（批一新增） |
| 员工 role 是自由文本 | `digital_employees.role varchar NOT NULL`，无词表、无外键 |
| 剧本角色 key 与员工 role 无关联 | 匹配链是 `roles[].required_capabilities` → 员工 `capability_bindings.external_capabilities`；`role` 从不参与 |
| 能力只记录不扣分 | `planning_profile.go` `scoreCapabilities`——差异记录进 `matched/missing_capabilities`，对分数贡献常量 |
| 缺员拦截面很窄 | `structuralGapForPlan`/`enforceRoleIndependence`：仅 `role_independence` 且 `activeExecutorCount < 2` |
| 选角唯一硬校验 | `graph_validation.go`：`selected_employee_id` 必须在活跃执行池内 |
| 收口裁剪已实现 | `pruneSkeletonForExit(spec, exit)` 返回该 exit 的依赖闭包步骤；`exitCondMet` 判定约束是否在该档生效 |
| 兼任已被服务端记录 | `EnforceScenarioTemplateGovernance` 按**实际派工**无条件生成 collapse 注解 + `RequiresHumanReview=true` |
| 缺员决策已有 restaff 通路 | `DecisionTypePlanningGap` + `PlanningGapDecisionRestaffed/Exempted` → 协调线程 reopen + replan |
| **reopen 仅对 failed 生效** | `ReopenProjectDemandForReplanning`：「Only a currently-failed demand reopens」。**扩编发生在 executing 中途，不能复用**（见 §7.3） |
| demand 状态单调 | `ProjectDemandStatusCanAdvance` 只升不降；`executing → planning_pending` 是回退，会被静默拒绝 |
| 上游补做不引入新人 | `CreateUpstreamSupplementTasks`：「appends a task for the owner of each missing input」 |
| 现有 decision types | `plan_review` / `planning_gap` / `planning_failed` / `demand_acceptance` / `task_failure_recovery` / `upstream_supplement_review` / `project_task_approval` / 各 recovery 类 |
| 数据库已清空 | 2026-08-04 B 档清理：业务数据归零，保留用户/租户/团队/剧本/能力词表/技能/MCP/员工模板/runtime 注册 |
| **清库误伤已修复** | `user_project_team_scopes` 表名含 "project" 但实为 **user × team 授权**（无 `project_id` 列），清库时被误清，已通过 `PUT /api/auth/users/{id}/project-team-scopes` 重新授予 admin 对「默认团队」的 scope。**若遇到「当前用户无权使用该团队创建项目」，就是这条**——按同一 API 补授权即可 |

---

## 4. 数据模型

### 4.1 角色注册表（新）

参照 `capability_vocabulary` 的形状，租户级：

```sql
CREATE TABLE role_vocabulary (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants (id),
    role_key    TEXT NOT NULL,           -- 与剧本 roles[].key 同一命名空间
    title       TEXT NOT NULL,           -- 中文显示名
    description TEXT,
    status      VARCHAR NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'disabled')),
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, role_key)
);
```

**命名规范**：下划线小写（`code_reviewer`），与批一统一后的能力词表一致。

**词表为主的落地**：`scenariotemplate.Service` 新增 `validateSpecRoles`——`Create`/`CreateVersion`/`Patch` 时校验 `spec.roles[].key` 全部已注册且 active，未注册则 400 并点名。与既有 `validateSpecVocabulary` 同一位置、同一风格。

### 4.2 员工角色多值（schema 变更）

```sql
CREATE TABLE digital_employee_roles (
    tenant_id           UUID NOT NULL,
    digital_employee_id UUID NOT NULL REFERENCES digital_employees (id) ON DELETE CASCADE,
    role_key            TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (digital_employee_id, role_key)
);
CREATE INDEX idx_digital_employee_roles_tenant_role
    ON digital_employee_roles (tenant_id, role_key);
```

**关联表而非数组列**：候选查询是「按角色找人」（`WHERE role_key = ?`），关联表能走索引；数组列要 GIN 且写起来更绕。

**旧 `digital_employees.role` 列的处置**：保留但**降级为显示用自述标签**，读路径不再参与任何匹配。列注释要写明这一点，防止后人再拿它做匹配。（不删列：`NOT NULL` 且历史数据/审计快照可能引用。）

### 4.3 编制表（新）

```sql
CREATE TABLE project_playbook_casting (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID NOT NULL,
    project_id          UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    scenario_template_key TEXT NOT NULL,
    role_key            TEXT NOT NULL,
    digital_employee_id UUID NOT NULL REFERENCES digital_employees (id),
    cast_by_user_id     UUID NOT NULL,      -- 必须由人操作，留痕谁定的
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, scenario_template_key, role_key)
);
```

`UNIQUE` 直接落实「一角色一人」；同一 `digital_employee_id` 可出现在多行 = 兼任。

`cast_by_user_id` 是承重字段：**编制必须能回答"谁批准这个人上这一仗"**，这是整批工作的责任边界所在。

### 4.4 编制与项目成员池的关系

**编制选人 = 自动加入 `project_members`（principal_type=digital_employee, project_role=executor）。**

成员池 = 编制的并集 + 人工额外添加的人。两者不是独立真相：

- 编制写入时若该员工不在池内 → 同事务插入成员行
- 从池内移除某员工时 → 若他仍被某条编制引用，**拒绝移除**并提示先改编制（否则编制会指向池外的人）

---

## 5. 剧本选择器：显示可达最深收口

### 5.1 判定算法

对某项目 × 某剧本：

```text
for exit in spec.exits（浅 → 深）:
    steps = pruneSkeletonForExit(spec, exit)          # 已实现
    roles = distinct(steps[].role)
    满足 = 每个 role 在编制里有人，或池内存在具备该 role 的员工
    并且 该 exit 下生效的 role_independence 约束能被满足   # exitCondMet 已实现
    if 满足: deepest = exit
返回 deepest（可能为 nil = 一档都走不了）+ 下一档缺什么角色
```

**必须复用 `pruneSkeletonForExit` 与 `exitCondMet`**，不得在读路径另写一套裁剪逻辑——两套一定会漂。

### 5.2 呈现

```
软件开发    可走到「交付分支」      再往深需要：独立的审查角色
故障排查    可走到「仅诊断根因」    再往深需要：修复、验证角色
运维分析    ⚠ 暂不可跑             缺：采集、分析角色
调研报告    可走到「成稿」          ✓ 角色齐备
```

- **一律可选**（含"暂不可跑"的），选中后进编制界面就地补齐——缺是可执行动作，不是墙
- 「暂不可跑」也要显示，而不是隐藏：隐藏会让人以为平台没有这个剧本

---

## 6. 编制

### 6.1 时机

| 场景 | 编制来源 |
|---|---|
| 首次在某项目用某剧本 | 要求编制一次（重） |
| 之后同项目同剧本 | 带出已保存的编制（轻） |
| 同单接续 | 完全继承父单的剧本与编制，不重选 |
| automation / 飞书旁路 | **必须用已保存的编制**；失效则失败 + 通知（基线 §4.7 修订版） |

### 6.2 候选列表

- **范围**：租户内**权限可见**的全部数字员工（不限项目所属团队）
- **过滤**：具备该 `role_key`（来自 `digital_employee_roles`）
- **分组**：按团队分组展示（团队是展示维度）
- **排序与标注**：按是否满足该角色的 `required_capabilities` 排序并标注

  ```
  ✓ 张三（开发一组）      具备 code_implementation
  ✓ 李四（平台组）        具备 code_implementation
  ⚠ 王五（外包组）        缺 code_implementation
  ```

  **可以选 ⚠ 的**——人有权知情地选一个系统认为不匹配的人。能力是信息，不是门禁。

### 6.3 automation 规则保存期校验

基线 §4.7 修订版要求「可预防的缺失挡在有人在场的时刻」：automation 规则保存时，若其 `scenario_template_key` 对应的编制不完整 → **保存即失败并点名缺哪个角色**。

---

## 7. 扩编（执行期）

### 7.1 触发

任一任务完成后，判官或协调线程可提请扩编。**不必等整单验收**——越早扩编返工越小。

### 7.2 判官如何知道缺什么角色

判官**不需要知道员工库，只需要知道角色词表**。分工：

| 环节 | 谁做 | 输出 |
|---|---|---|
| 发现缺口 | 判官/协调线程 | 自然语言（"还需核查网络链路"） |
| 映射到角色 | **约束输出**：把租户角色词表注入提示，只能从词表里选 | `suggested_role_key: network_diagnostics` |
| 找候选 | 系统按 `digital_employee_roles` 查 | 候选员工列表 |
| 决定 | **人** | 批准（选人）/ 换人 / 驳回 |

**约束输出是本设计对"符合当前 AI 发展现状"的兑现**：让模型从几十项枚举里选一项是可靠的；让它自由判断"这个组织该有什么角色"是不可靠的。

**词表外的需求**：判官只能标记「需要词表外的角色」并附自然语言说明，由**人**翻译（新建角色+招人，或判断现有角色够用）。**绝不允许判官自行创建角色**。

### 7.3 扩编 = 带新编制的重规划（关键约束）

扩编**不在既有任务图上打补丁**，而是触发一次重规划——这样 `EnforceScenarioTemplateGovernance` 会把职责分离、必经阶段、人类闸**全部重新校验一遍**，剧本结构不被破坏反而被再验证一次。

**三条必须写死的边界**：

| 剧本成分 | 扩编能否改变 |
|---|---|
| `constraints`（职责分离/必经阶段/人类闸） | **绝对不能**。扩编进来的人若恰好是原开发，照样触发职责分离拦截 |
| `exits` 收口 | 不能新增；改选深浅要走既有 `target_exit_deliverable` 链路 |
| `skeleton` 步骤 / `roles` 编制 | 可增补 |

**实现约束（地基核对发现，实施必须处理）**：

- `ReopenProjectDemandForReplanning` **只对 failed 的 demand 生效**，扩编发生在 `executing`，**不能直接复用**
- `executing → planning_pending` 是**状态回退**，会被 `ProjectDemandStatusCanAdvance` 静默拒绝
- 因此扩编重规划**不应回退 demand 状态**：保持 `executing`，新增一个 coordination job 产出新的 plan revision（新修订被接受后经 `CurrentEffectivePlanRevisionID` 自然成为生效版本）

**⚠ 实施前必须勘察（本 spec 最大未知）**：重规划时**已完成任务如何处理**——会不会重复创建、已消费的验收闸怎么算、已完成任务的产物是否被新计划继承。`request_changes` 重规划应已面对过这个问题，实施会话必须先读懂现有行为再动手，**不得凭猜实现**。

### 7.4 批准即确认计划 + 越界拦截

人批准扩编时新计划尚不存在，所以这一次点击是**预确认**。为防止重规划跑偏：

- **正常情况零打扰**：新计划相对旧计划的差异若**仅包含扩编角色相关的新任务**，直接生效开跑
- **越界则退回人工确认一次**：动了收口、新增了与扩编无关的任务、或改变了既有任务的归属

越界判据必须是**服务端**计算的结构化 diff，不能靠人眼看。

### 7.5 载体

新开 `DecisionTypeCastingExpansion = "casting_expansion"`，不复用 `planning_gap`——后者语义是"规划期缺员"，混用会把两种时机搅浑，且收件箱呈现与人的心智会乱。

决策动作：`approved`（带选定的 employee_id）/ `rejected`。

---

## 8. API

```http
GET  /api/v1/projects/{projectId}/playbook-readiness        # §5 可达收口 + 缺什么
GET  /api/v1/projects/{projectId}/castings?template_key=    # 读编制
PUT  /api/v1/projects/{projectId}/castings                  # 写编制（整套替换，含自动入池）
GET  /api/v1/role-vocabulary                                # 角色词表 CRUD
POST /api/v1/role-vocabulary
PATCH /api/v1/role-vocabulary/{roleKey}
GET  /api/v1/projects/{projectId}/role-candidates?role_key= # 候选（按角色过滤、能力标注、团队分组）
```

扩编决策走既有 `POST /api/v1/projects/{projectId}/decisions/{decisionId}/resolve`，新增 `casting_expansion` 类型的动作词汇。

---

## 9. Web

| 落点 | 改动 |
|---|---|
| 提交需求对话框 | 剧本选择器改为「可达最深收口 + 缺什么」；选中后进编制区 |
| 编制区（新组件） | 每个角色一行，候选按团队分组、能力标注排序；缺角色高亮 |
| 项目配置页 | 新增「剧本编制」分区，可预先做好 |
| 收件箱 / 卷宗 | 扩编决策卡：判官理由 + 建议角色 + 候选人选择 |
| `status-labels.ts` | 补 `casting_expansion` 等新枚举中文 |

---

## 10. 最小可用数据（**已于 2026-08-04 预置完成**）

数据库已于 2026-08-04 做过 B 档清理（业务数据归零，保留用户/租户/团队/剧本/能力词表/技能/MCP/员工模板/runtime 注册）。**下列数据已用真实 API 建好，实施会话直接用，不必重建。**

### 10.1 已预置（真实 ID，可直接引用）

**数字员工 5 个**（租户 `00000000-0000-0000-0000-000000000001`，团队「默认团队」`00000000-0000-0000-0000-000000000101`，provider 均为 `claude-code`）：

| 名称 | `digital_employees.id` | 当前 `role`（自由文本，待 P0b 迁词表） | `external_capabilities` |
|---|---|---|---|
| 开发-A | `0be393bb-9dfd-48c8-b010-4b5abb114f23` | developer | `code_implementation` |
| 审查-B | `7a16f593-9a99-490e-bcab-77bb8b326afa` | reviewer | `code_review` |
| 测试-C | `157b1a2c-b2af-4a08-99f3-f16abe291ed1` | tester | `test_execution` |
| 运维-D | `9a623b40-c9ec-4d7d-99a4-17b1f569b52e` | collector | `log_analysis` |
| 诊断-E | `3683f032-2e24-43da-af06-5af1b8ce71a4` | diagnostician | `incident_triage` |

**项目 1 个**：`批二基线项目 P1` = `ca82b054-de2d-4810-9a2b-dd41f5e50a2c`，目录名 `batch2-baseline-p1`，负责人 admin，**数字员工成员池初始为空**（这是 §5/§6 的验证前提，不要预先加人）。

**剧本**：5 个 active（`generic` / `software_delivery` / `ops_analysis` / `incident_response` / `research_report`）。**能力词表**：9 个 active（批一已对齐为下划线命名）。

### 10.2 P0a/P0b 落地后需要补的（现在建不了）

角色词表表与员工角色关联表尚不存在，因此以下两项**必须在 P0a/P0b 实现后补建**：

```text
1) 角色词表 8 项（覆盖内置 4 个剧本的全部角色）
   developer / reviewer / tester
   collector / analyst
   diagnostician / operator / verifier

2) 员工角色绑定（注意两处兼任，用于验 collapse）
   开发-A  → [developer]
   审查-B  → [reviewer]
   测试-C  → [tester]
   运维-D  → [collector, analyst]        # 兼任
   诊断-E  → [diagnostician, verifier]   # 兼任
```

**故意不建 `operator` 角色的员工** —— 这是 §5 可达收口计算的关键验证条件：故障排查剧本因此只能走到「仅诊断根因」，走不到「实施修复」。**不要"顺手"补一个 operator 员工**，那会让 G2 失去判别力。

### 10.3 预置数据对应的期望（G2 的判据来源）

项目 P1 成员池为空时：

| 剧本 | 期望可达最深收口 | 缺什么 |
|---|---|---|
| 软件开发 | 「交付分支」（只需 developer） | 再往深缺独立的 reviewer |
| 故障排查 | 「仅诊断根因」（只需 diagnostician） | 再往深缺 operator |
| 运维分析 | 角色齐备（collector + analyst 由运维-D 兼任） | — |
| 调研报告 | 无对应角色员工 | 缺 researcher / writer |

编制后重新计算，验证可达档位随编制变化（G3）。

### 10.4 automation 规则（G7 用，实施时建）

一条绑定 `software_delivery` 的规则；期望编制不完整时**保存即失败并点名缺角色**（§6.3）。

## 11. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | 角色词表表 + CRUD + 剧本 `roles[].key` 校验 | curl 造词表，未注册角色建剧本被拒 |
| **P0b** | 员工角色多值表 + 员工配置读写接线 | 造员工带多角色 |
| **P0c** | 编制表 + 读写 API + 自动入池 + 移除保护 | curl 造编制，验证成员池同步 |
| **P0d** | 可达收口计算 + `playbook-readiness` 端点 | 按 §10 数据验证三个剧本的可达档位 |
| **P0e** | Web：剧本选择器 + 编制区 | 浏览器真链路 |
| **P1a** | 扩编决策类型 + 判官提示注入角色词表 + 提请通路 | 真实判官提请 |
| **P1b** | 扩编重规划 + 越界拦截 | 真实扩编闭环 |

**P0 与 P1 可分批交付**：P0 让"发起时人定编制"可用；P1 补执行期扩编。但 **P0a–P0e 必须整体交付**——只有词表没有编制，等于加了一层不产生价值的登记。

---

## 12. 验收 GATE（真实 E2E）

| ID | 步骤 | 期望 |
|---|---|---|
| G1 | 建剧本引用未注册角色 | 400 并点名该角色 |
| G2 | 按 §10 建数据后看项目 P1 的剧本列表 | 软件开发「交付分支」、故障排查「仅诊断根因」、运维分析齐备、缺 operator 的提示正确 |
| G3 | 为软件开发编制 developer+reviewer+tester | 可达档位升到「发布上线」；三人自动进成员池 |
| G4 | 同一人同时编制为 developer 与 tester | 规划后出现 collapse 注解且 `RequiresHumanReview=true` |
| G5 | 从成员池移除仍被编制引用的员工 | 拒绝并提示先改编制 |
| G6 | 候选列表 | 按角色过滤正确、能力标注正确、⚠ 项**可选中** |
| G7 | automation 规则绑定编制不全的剧本 | 保存即失败并点名缺角色 |
| G8 | 编制中的员工被停用后触发 automation | **失败 + 通知写明原因**（基线 §4.7 修订版） |
| G9 | 真实判官提请扩编 | `suggested_role_key` **来自词表**，不是自由文本 |
| G10 | 人批准扩编并选人 | 新编制生效 → 重规划 → 新任务派给新人；**已完成任务未被重复创建** |
| G11 | 扩编后重规划越界（构造改收口的情况） | 退回人工确认，不直接开跑 |
| G12 | 扩编进来的人恰好是原开发（触发职责分离） | 照样被拦，扩编不豁免任何 constraint |
| G13 | `verify:contracts` + `verify:control-plane` + `verify:web` + `migrate-validate` | 全过 |

**完成定义**：G1–G13 全过；§0 拍板项无破坏；§2 非目标未偷做。

---

## 13. 风险

| 风险 | 缓解 |
|---|---|
| **重规划重复创建已完成任务**（最大未知） | §7.3 标为实施前必须勘察；G10 是硬判据 |
| 扩编绕过治理约束 | §7.3 三条边界 + G12 |
| 可达收口算法与规划期裁剪漂移 | 强制复用 `pruneSkeletonForExit`/`exitCondMet`，不另写 |
| 编制失效导致 automation 静默换人 | 基线 §4.7 已修订为「允许失败但必须通知」；G8 |
| 编制与成员池分裂 | §4.4 双向约束 + G3/G5 |
| 角色词表与能力词表概念混淆 | 角色 = 编制单位（who）；能力 = 提示（what he can do）。UI 文案必须区分 |
| 判官编造角色 | 约束输出（§7.2）+ G9 |
| 共享工作树 git 踩踏 | 显式路径 `git add`，禁 `-A` |

---

## 14. 开放细节（不阻塞立项；实施默认值）

1. 角色词表是否需要"建议能力"字段（`suggested_capabilities`）用于编制候选排序？默认：不加，排序直接读剧本 `roles[].required_capabilities`。
2. 扩编决策的审批人：沿用项目人类负责人 any-of-N。
3. 编制变更是否要审计事件？默认：要（`project.casting.changed`），承重字段 `cast_by_user_id` 已在表上。
4. 剧本改版新增角色导致既有编制不完整：读路径显示"编制不完整 + 缺哪个角色"，不自动修改编制。

---

## 15. 文档关系

| 文档 | 关系 |
|---|---|
| `2026-07-27-workspace-and-playbook-alignment-baseline.md` | 基线；§4.7 已于 2026-08-04 按本批讨论修订 |
| 批一（CHANGELOG 2026-08-04 16:30） | 能力词表两侧对齐是本批的前置 |
| 基线 §8 #4「轻发起与剧本归属」 | **被本批吞掉大部分**：编制前移到发起期且可复用；#4 剩余仅「Prompt 模板降权改名」「来源字段补齐」等收敛项 |
| `TODO.md` 剧本管理页面 UI | 后期单独立项，不在本批 |

---

## 16. 一句话方案

> **角色进注册表、剧本引用已注册角色、员工可兼多角色；发起时人为每个角色定一人（自动入池、可复用），剧本选择器诚实显示能走到哪一档；执行中途判官从词表里指出缺什么角色、人选人、带新编制重规划，越界才打断。**
