# 自动化任务（定时触发）：调度配置 + 复用任务中枢 / 飞书人类闸门

- 状态：实施中（分支 `feat/automation-tasks`，独立 worktree）
- 日期：2026-07-22
> 复核状态：状态显示"实施中（分支feat/automation-tasks，独立worktree）"
- 决策来源：2026-07-22 与人类负责人对齐——「自动 ≠ 默认无人」；自动化是定时触发的任务中枢发起，不是另开执行栈；完成后人类闸门（含验收）走既有 Console / 飞书投影。
- 前置：
  - 飞书集成 P1（`2026-07-17-feishu-integration-design.md`）——外部服务凭据、on-behalf-of、决策 outbox、any-of-N；**自动化立项排在飞书之后**（该文已写明）。
  - 任务中枢三模式（`2026-07-13-task-hub-tri-mode-design.md`）——plan / loop / chat 入口语义。
  - 计划阶段与验收边界（`2026-07-10-project-plan-phase-refactor-design.md` §4.7）——人类是裁决者；loop 只自动补闭合契约，不自动判定目标达成。
- 现状：Console `/automations` 为占位页（「配置由定时规则驱动的平台任务入口」）；无规则表、无调度、无 fire 记录。

---

## 0. 已拍板决策（不再重议）

1. **自动 ≠ 默认无人。** 自动化解决的是「谁在什么时候点发起」；不默认跳过计划确认、缺口升级、终态验收等人类闸门。
2. **执行路径唯一：任务中枢同一条。** 到点 = 一次 `SubmitDemand`（plan/loop）或一次员工 `chat` run；不进平行执行器、不绕过 Project Coordinator。
3. **人类闸门投影复用飞书。** 自动化跑出来的决策 / 终态与手点发起完全同路径：outbox → connector → 审批卡 / 结果卡；Console inbox 永远可处理。不单做「自动化专用飞书流」。
4. **调度引擎用已有 Temporal Schedule**，不在 Control Plane 内自研 cron，不新建持业务状态的独立调度服务。
5. **配置与事实源在 Control Plane**（规则表、启停、fire 审计）；Temporal 只持时钟与重试。
6. **一期主推 Loop + 终验守门**；Plan 可配但默认半自动（仍可卡 `plan_review`）；Chat 可配但定位为定时对话/巡检，不冒充项目闭环验收。
7. **触发实现：Activity 同进程直调 Service 层**（不经 HTTP / ServiceAuth）。当前阶段规则量小，同进程更简单、少一层故障面；审计用 `source_type=automation` + `actor_user_id` 表达双主体即可。若日后规则量或部署形态需要把触发器拆出进程，再改为 HTTP+ServiceAuth，业务语义不变。
8. **日程：cron + interval 双支持**（创建时二选一，不是两套并行时钟）。
9. **Chat 规则强制绑定项目**（与现网 chat run 项目锚点一致，无孤儿 run）。
10. **连续发起失败 3 次 → 自动 disable 规则**（Pause Temporal Schedule + 事件留痕；可手动重新 enable）。`skipped_overlap` / 人为 disable / **因失权被系统 disable** **不计入**失败次数。
11. **发起人 = 规则创建者，创建后不改挂他人。** 创建时校验当前用户有权选择该项目（与任务中枢可选项目集合一致——至少是该项目人类成员/负责人之一，因而也在 any-of-N 审核收件人集合内）。**创建者被移出项目或账号停用 → 规则立即 disable**，写入 `disabled_reason`（如 `actor_removed_from_project`），Pause Schedule；不靠等到点连败才发现。

---



## 1. 问题

1. 重复性工作（日报巡检、周期回归、固定项目例行需求）仍要人手打开任务中枢点发起。
2. 若把「自动化」理解成「无人值守跑完」，会与现行架构冲突：Plan 强制计划确认、验收判据由人类裁决、模板 `human_gate` / 缺口仍停等人——假装全自动只会堆出无人处理的 inbox。
3. 飞书已把人类决策推到手机侧；自动化若另造通知/验收通道，会双轨分叉。
4. 菜单名「自动化任务」易被理解成 Workflow/RPA；实际产品是**定时任务配置（Automation Rule）**。

---



## 2. 目标与非目标



### 目标（P1）

1. Console「自动化任务」可 CRUD 定时规则：锚点项目、模式、需求模板（或 chat 目标）、cron/间隔、启停；发起人固定为创建者；**前端按 §7 脆数据主从表 + 抽屉落地**。
2. 到点以创建者为 `SubmittedBy` 调用既有发起 API，产出普通 Demand / Chat Run，带 `source=automation` 与 fire 幂等键。
3. 跑完后的 `plan_review` / `planning_gap` / `demand_acceptance` / 终态通知 **零改飞书契约语义**：与手发需求同一 outbox 投影；人类可在飞书卡或 Console 处理（any-of-N）。
4. 规则变更与 fire 全量审计；漏跑/重叠/并发策略可解释、可观测。
5. 产品文案与配置 UI 明确标出「本规则到点后是否仍会等人」——按模式与闸门策略，而非统一「全自动」话术。
6. 创建者失权（移出项目 / 停用）→ 规则立即 disable 并标识原因。



### 非目标（P1 明确不做）

- 不引入新的轻量 cron 框架或独立「调度微服务」持业务状态。
- 不做「自动签署验收判据 / 模型自批通过」（与 §4.7 收窄一致）。
- 不做 Plan 的预授权模板跳过 `plan_review`（列入 P1.5 / P2，见 §8）。
- 不做事件触发（webhook、仓库 push、飞书消息隐式建单）——仅日历/cron 类时间触发。
- 不做跨项目编排、规则 DAG、条件分支（那是流程编排菜单的事）。
- 不改 Runtime / Provider；不改协调线程状态机语义（只多一个发起源）。
- 不在飞书侧为自动化单独做验收协议；判据卡内闭环签署仍遵循飞书 P1「深链 Console」边界（若日后飞书 P2 放开，自动化自动受益）。

---



## 3. 核心命题：自动化分档


| 分档             | 含义                     | 典型模式             | 人类仍做什么                                       |
| -------------- | ---------------------- | ---------------- | -------------------------------------------- |
| A. 定时触发 + 终验守门 | 到点自动开跑，中间尽量自治，**终点等人** | **Loop（一期默认推荐）** | `demand_acceptance` / 升级分歧；可走飞书结果卡 +（P1）深链签署 |
| B. 定时触发 + 过程确认 | 到点生成待确认计划/缺口           | **Plan（默认）**     | 必做或大概率做 `plan_review`；飞书审批卡可点                |
| C. 定时对话        | 到点对某员工发起 chat run      | **Chat**         | 看回复；要进项目闭环须「转为任务」或另配 Demand 规则               |
| D. 预授权无人 Plan  | 配置时锁死信封，运行时跳过计划确认      | Plan + 预授权       | 仅终验 / 异常——**非 P1**                           |


**原则写死：** 自动化的是触发与（在既有模式下）执行延展；**判断仍按项目既有闸门。** 飞书只降低「人必须坐在 Console 前」的成本，不消灭闸门。

---



## 4. 架构



### 4.1 形态

```
Console /automations
  CRUD AutomationRule ──► Control Plane（事实源）
                              │ 同步 Create/Update/Delete
                              ▼
                     Temporal Schedule（时钟）
                              │ 到点
                              ▼
              Activity: FireAutomationRule
                              │ 同进程直调；SubmittedBy = actor_user_id（创建者）
                              ▼
         SubmitDemand | CreateDigitalEmployeeChatRun
                              │ source=automation, fire_id 幂等
                              ▼
              既有 Project Coordinator / Employee Run
                              │
              人类闸门 Decision + 终态 ──► feishu_outbox（既有）
                              ▼
                     feishu-connector 投影（既有）
```

- **AutomationRule**：业务配置（租户/团队/项目、模式、模板字段、日程、时区、enabled、`actor_user_id`=创建者、`disabled_reason`、并发策略）。
- **AutomationFire**：每次触发的审计行（scheduled_at、status、demand_id/run_id、error、idempotency_key）。
- **触发调用方**：Control Plane Worker 内 Activity **同进程直调** `project.Service.SubmitDemand` / 员工 RunService（chat）。不经自调 HTTP。审计：`SubmittedByUserID` = 规则创建者；事件带 `source_type=automation`、`automation_rule_id`、`automation_fire_id`。
- **权限生命周期**：
  1. **创建**：当前用户必须出现在「任务中枢可选项目」集合中（对该项目有发起权；产品语义：至少是该项目人类成员/负责人之一 → 也在 any-of-N 审核收件人内）。
  2. **运行中**：`actor_user_id` 固定为创建者，UI 不提供「改挂他人」。
  3. **失权**：创建者被移出项目或账号停用 → **立即** `enabled=false`、`disabled_reason` 落库（如 `actor_removed_from_project` / `actor_deactivated`）、Pause Schedule、审计事件；列表展示禁用原因。重新加入项目后须**人工**重新 enable（不自动复活）。



### 4.2 为什么是 Temporal Schedule，而不是 CP 内轮询





- 项目协调已依赖 Temporal；Schedule 提供 cron、日历、重叠策略（Skip / Buffer / Cancel）、暂停、手动 Trigger。
- 多副本 CP 不需要选主抢锁跑 cron。
- 业务幂等仍落在 `AutomationFire.idempotency_key = rule_id + scheduled_fire_time`，与 Temporal 至少一次投递对齐。



### 4.3 与飞书的关系（显式）


| 环节                  | 自动化是否新做 | 说明                                                       |
| ------------------- | ------- | -------------------------------------------------------- |
| 到点发起                | 新       | 等价于任务中枢 / connector `SubmitDemand`，多 `source=automation` |
| `plan_review` 卡     | **不新做** | 决策创建 → 既有 outbox                                         |
| `planning_gap` 等    | **不新做** | 同上                                                       |
| `demand_acceptance` | **不新做** | 飞书 P1：富摘要 + **深链 Console 签署**；Console / 飞书结果通知照常         |
| 需求终态结果卡             | **不新做** | 既有终态 outbox                                              |


结论：**接入飞书后，自动化任务「完成后可在飞书侧感知并处理验收相关交互」是复用，不是本立项的新通道。** 人类在飞书点审批卡、点结果卡深链去 Console 签判据，与手发需求体验一致。

---



## 5. 数据与契约（草案）



### 5.1 表（示意）

```sql
-- automation_rules
id uuid PK
tenant_id, team_id, project_id uuid NOT NULL
name text NOT NULL
enabled boolean NOT NULL DEFAULT true
coordination_mode text NOT NULL  -- plan | loop | chat
-- plan/loop:
demand_title_template text
demand_body_template text
scenario_template_key text NULL
-- chat:
digital_employee_id uuid NULL
chat_objective_template text NULL
-- schedule:
schedule_kind text NOT NULL      -- cron | interval
cron_expr text NULL              -- schedule_kind=cron 时必填
interval_seconds int NULL       -- schedule_kind=interval 时必填（下限实施时定，建议 ≥60）
timezone text NOT NULL DEFAULT 'Asia/Shanghai'
overlap_policy text NOT NULL DEFAULT 'skip'  -- skip | buffer_one
actor_user_id uuid NOT NULL      -- 固定=创建者；SubmittedByUserID / chat 行为人
disabled_reason text NULL       -- 系统 disable 时填写；人工 disable 可为 user_disabled
consecutive_failure_count int NOT NULL DEFAULT 0
temporal_schedule_id text NULL  -- 同步句柄
created_at, updated_at, ...
```

```sql
-- automation_fires
id uuid PK
rule_id uuid NOT NULL
tenant_id uuid NOT NULL
scheduled_fire_at timestamptz NOT NULL
idempotency_key text NOT NULL UNIQUE
status text NOT NULL  -- pending | succeeded | failed | skipped_overlap | skipped_disabled
demand_id uuid NULL
run_id uuid NULL
error_code text NULL
error_message text NULL
created_at timestamptz NOT NULL
```

索引：`(tenant_id, rule_id, scheduled_fire_at DESC)`；规则列表 `(tenant_id, enabled)`。

### 5.2 API（Console）

- `GET/POST /api/v1/automations`
- `GET/PATCH/DELETE /api/v1/automations/{id}`
- `POST /api/v1/automations/{id}/enable|disable`
- `POST /api/v1/automations/{id}/trigger`（手动试跑，仍走同一 Fire 路径）
- `GET /api/v1/automations/{id}/fires`

Authz：创建/改规则要求对该 `project_id` 有发起权（与任务中枢选项目一致）；列表可读范围同项目可见性。租户 admin 可只读审计。

### 5.3 发起载荷扩展

- `SubmitProjectDemand` / 内部等价调用增加可选：`source_type=automation`、`automation_rule_id`、`automation_fire_id`。
- Chat run 同样写入 metadata / 审计事件，便于运行总览过滤「来自自动化」。

模板变量（P1 最小集）：`{{date}}`、`{{datetime}}`、`{{rule_name}}`、`{{project_name}}`；不做自由脚本。

---



## 6. 模式行为（配置与文案）


| 模式   | P1 是否支持   | 到点行为                    | UI 必须提示                                 |
| ---- | --------- | ----------------------- | --------------------------------------- |
| loop | **是（推荐）** | SubmitDemand(mode=loop) | 「执行中按项目闸门可能停等；**验收仍需人类**（Console / 飞书）」 |
| plan | 是         | SubmitDemand(mode=plan) | 「通常需**计划确认**后才派发；确认可在飞书审批卡完成」           |
| chat | 是         | Create chat run         | 「不进项目协调与验收；仅员工对话记录」                     |


并发：`overlap_policy=skip`——上一次 fire 对应的 demand 仍非终态则跳过本次并记 `skipped_overlap`（防例行任务堆叠）。`buffer_one` 列为可选，P1 可只做 skip。

失败：发起失败（项目停用、模板渲染失败、竞态下 actor 已失权但钩子尚未跑完等）→ fire=`failed` + 事件；规则上 `consecutive_failure_count` 在 `failed` 时 +1，成功 fire 时归零；**达到 3** → 自动 `enabled=false`、`disabled_reason=consecutive_fire_failures`、Pause Schedule。`skipped_overlap` / `skipped_disabled` / **失权钩子导致的 disable** 不递增计数。重新 enable 时计数归零，且 enable 时再次校验 actor 仍对该项目有发起权，否则拒绝 enable 并提示原因。

**失权钩子（P1）**：项目成员移除 / 用户停用路径上，查询 `actor_user_id = 该用户` 且 `enabled` 的规则 → 批量 disable + `disabled_reason` + Pause Schedule。这是主路径；连败 3 次是兜底。

---



## 7. 前端页面设计（P1）

> 约束来源：`DESIGN.md` 容器选择规则——**自动化属「注册表与配置」高密度面**，内容必须实底脆数据面，**禁止玻璃 / 半透明 / blur**。组件优先 `@/components/superteam`（`WorkSurface` / `V3Table` / `StatusPill` / `V3EmptyState` 等）。内部跳转一律 TanStack `Link` / `navigate`。枚举经 `status-labels.ts`。

### 7.1 页面身份与路由

| 项 | 决定 |
|---|---|
| 路由 | `/automations`（替换现 `UnimplementedPage`） |
| 侧栏 | 保持「自动化任务」；页头副标题用「定时规则」 |
| 宽度 | `Main width="wide"`（配置表 + 可选右栏详情，对齐 MCP / 系统配置类页） |
| 页头 | `ShellPageHeader` 或 `V3PageHeader`：标题「自动化任务」、副标题「按日程自动发起任务中枢需求或对话；人类闸门仍按项目逻辑处理（含飞书）」、主 CTA「新建规则」 |
| 密度 | 脆数据面；顶栏可放 ≤3 个真实计数 `MetricGrid`（规则总数 / 启用中 / 近 24h 失败或跳过），无数据不伪造趋势 |

不设独立 `/automations/$id` 全页（P1）：选中规则用 **主从**（宽屏右栏 / 窄屏 Sheet）承载详情与 fire 历史，降低导航层级。

### 7.2 信息架构（一屏）

```
┌─ 页头：标题 · 副标题 ──────────────────────────────────────┐
├─ 运营说明 + [新建规则] ────────────────────────────────────┤
├─ 事实条：启用中 / 72h待触发 / 需关注 / 待你处理 ─────────────┤
├─ 工具栏：搜索 · 状态筛选 · 模式筛选 ────────────────────────┤
├────────────────────────────┬──────────────────────────────┤
│ 规则表 WorkSurface+V3Table │ 右栏常驻（未选中 = 工作台）      │
│ 列含：下次触发 · 健康       │ · 即将触发 72h                │
│ 整行可选中                  │ · 待你处理（inbox 切片）       │
│ 空态：三场景模板卡          │ · 最近触发动态                │
│                            │ 选中后切：规则详情 + fires     │
└────────────────────────────┴──────────────────────────────┘
         │
         └─ 新建/编辑 → Sheet（分区：锚点 / 模式三卡 / 模板 / 日程预设 / 闸门摘要）
```

布局复用 `MasterDetailLayout` + `narrowDetail="stack"`：**右栏始终有内容**（未选中为工作台轨，选中为规则详情）；禁止半屏空洞。

### 7.3 规则列表（主表）

**列（从左到右，P1 必显）**

| 列 | 内容 | 规则 |
|---|---|---|
| 名称 | 规则名；下方一行次要灰字：项目显示名（禁止裸 UUID） | 两行 clamp |
| 模式 | `StatusPill` 或静音 chip：`plan` / `loop` / `chat` → 中文词表 | 词表键补 `status-labels` |
| 日程 | 人读摘要，如「每天 09:00」「每 6 小时」；tooltip 给 cron/interval 原文 | `tabular-nums` 用于时刻 |
| 状态 | 启用 / 已禁用；系统禁用时同格或下一行展示 `disabled_reason` 中文 | tone：启用=ok 轻量；禁用=mute；失权/连败禁用=warn |
| 最近触发 | 主时间（相对）+ 结果圆点（成功/失败/跳过） | **时间必显**；无 fire 显示「尚未触发」 |
| 创建者 | 当前用户即创建者时显示「我」或显示名 | 只读 |

- 行 `tone`：最近 fire=`failed` 或系统禁用 → `warn` accent bar；其余默认。
- 排序：默认按 `updated_at` 倒序（新近优先）。
- 整行点击 = 选中打开详情轨；行内「启停」为独立开关，不冒泡抢选中（或选中后在详情轨操作——P1 推荐**详情轨操作为主**，表内仅展示状态，减少误触）。

### 7.4 详情轨（选中规则）

区块自上而下，单职责：

1. **摘要头**：名称、项目 `Link`→`/projects/$id`、模式、日程摘要、启用态。
2. **人类闸门提示**（必有，静音 `Alert` / 内嵌说明条，非大红警告）：按模式固定文案（见 §6 表）。一句点明「自动触发 ≠ 无人」。
3. **动作行**：`启用/禁用`、`立即试跑`、`编辑`、`删除`（删除二次确认 `ConfirmDialog`）。
   - 试跑：走同一 Fire 路径；成功 toast + 刷新 fires；详情内高亮最新一条。
   - 重新启用：若服务端因失权拒绝，就地错误条展示原因，不静默。
4. **最近触发**：子表或紧凑列表（时间、结果 pill、demand/run 名称链接、错误摘要一行）。
   - plan/loop 成功 → `Link` 到 `/workflows/$demandId`（或项目内需求深链，与现网任务中枢落地一致）。
   - chat 成功 → `Link` 到员工详情 run（现网 chat 轨迹入口）。
   - `skipped_overlap` / `failed`：展示中文原因，无死链。

空详情（未选中）：不占右栏；主表全宽。

### 7.5 新建 / 编辑抽屉（冻结的任务中枢）

容器：右侧 `Sheet`（或宽 `Dialog`），标题「新建定时规则」/「编辑规则」。**一步表单**，不分多步 wizard（字段量可控）。

**字段顺序（刻意对齐任务中枢认知）**

| 顺序 | 字段 | 交互 |
|---|---|---|
| 1 | 规则名称 | 必填短文本 |
| 2 | 项目 | Select；选项 = 当前用户有发起权的项目（与任务中枢同源 API）；无项目 → 空状态引导去建项目/加成员 |
| 3 | 模式 | 三段控件 `V3Segmented` 或任务中枢同款三卡缩小版：`Plan` / `Loop`（标注推荐）/ `对话`；切换时下方闸门提示与字段区联动 |
| 4a | plan/loop：标题模板、正文模板 | textarea；底部小字「可用变量：`{{date}}` `{{datetime}}` `{{rule_name}}` `{{project_name}}`」；场景模板 key 可选（有则 Select） |
| 4b | chat：数字员工、对话目标模板 | 员工 Select 限该项目可调度员工；目标模板 textarea |
| 5 | 日程种类 | `cron` \| `interval` 二选一 segmented |
| 5a | cron | 预设快捷（每天/每周一/工作日 9:00）+ 高级 cron 原文；时区默认 `Asia/Shanghai` 只读或可选 |
| 5b | interval | 数字 + 单位（小时/天）；下限校验（建议 ≥60s，UI 用分钟起步防误填） |
| 6 | 重叠策略 | P1 固定文案说明「上一次未结束则跳过本次」，不开放改（或只读展示 `skip`） |
| — | 发起人 | **不展示选择器**；脚注「发起人 = 当前账号；离开项目后规则将自动停用」 |

**提交前闸门摘要卡**（实底，非玻璃）：根据所选模式渲染 2–4 条「到点后仍可能需要你处理」的条目（计划确认 / 终验 / 仅对话无验收）。主按钮文案：「创建并启用」；次按钮「仅保存为停用」（可选，P1 可只做创建即启用）。

编辑态：项目、模式是否允许改——**P1 允许改模板与日程；项目与模式锁定**（避免历史 fire 语义错乱）。若需换项目/模式 → 停用旧规则 + 新建。

### 7.6 状态、文案与词表

须在 `status-labels.ts`（及 guard 测试）补齐至少：

- 模式：`plan`→计划、`loop`→循环、`chat`→对话（若与任务中枢已有键则复用，不另造）。
- fire 状态：`succeeded` / `failed` / `skipped_overlap` / `skipped_disabled` / `pending`。
- `disabled_reason`：`actor_removed_from_project`→创建者已离开项目、`actor_deactivated`→创建者账号已停用、`consecutive_fire_failures`→连续发起失败 3 次、`user_disabled`→已手动停用。

列表/详情禁止直渲英文枚举与裸 UUID。

### 7.7 空态 / 加载 / 错误

| 态 | 表现 |
|---|---|
| 首屏加载 | `V3LoadingState` |
| 无规则 | `V3EmptyState`：说明「把例行需求做成定时规则」+ CTA「新建规则」；可附一句「自动触发后验收仍可在 Console / 飞书处理」 |
| 无可用项目 | 抽屉内嵌空态：引导「先加入或创建项目」→ `Link` 项目管理 |
| 列表错误 | `V3ErrorState` + 重试 |
| 试跑/启停失败 | 详情轨或 toast + 可复制错误摘要 |

### 7.8 关键交互流

1. **创建**：新建 → 填表 → 提交 → 关闭抽屉 → 列表插入并选中 → 详情可见「尚未触发」。
2. **试跑**：详情「立即试跑」→ fire 行出现 → 成功则深链打开 demand/run（可选新开，P1 同页刷新 + 链接即可）。
3. **失权回看**：他人打开列表见该行已禁用 + 原因「创建者已离开项目」；启用不成功并提示。
4. **连败停用**：三次失败后行 warn + 原因；详情展示最近错误。

### 7.9 组件与文件落点（实施时）

| 路径 | 职责 |
|---|---|
| `apps/web/src/routes/_authenticated/automations/index.tsx` | 路由挂载页 |
| `apps/web/src/features/automations/index.tsx` | 页面组合 |
| `.../components/automation-rules-table.tsx` | 主表 |
| `.../components/automation-rule-detail.tsx` | 详情轨 |
| `.../components/automation-rule-form-sheet.tsx` | 新建/编辑 |
| `.../components/human-gate-callout.tsx` | 模式闸门提示 |
| `apps/web/src/lib/api/automations.ts` | API 客户端 |
| 测试 | `*.test.tsx` 覆盖空态、模式提示、禁用原因展示；真链路浏览器验列表/创建/试跑 |

### 7.10 明确不做的前端（P1）

- 不做装饰性日历热力图 / 甘特式调度可视化（有内容的「即将触发」清单与下次触发列可以做）。
- 不做规则卡片网格作主列表（身份目录风）——主列表仍是配置审计表；空态可用场景模板卡引导创建。
- 不做玻璃壳创建画布（任务中枢那套沉浸入口不复用到本页）。
- 不做「改发起人」控件、不做 overlap 高级策略编辑器。

---



## 8. 分期



### P1（本计划范围）

1. 规则 CRUD + Temporal Schedule 同步 + Fire 幂等发起。
2. 三模式可选；默认推荐 loop；文案标明人类闸门。
3. 复用飞书投影（零新飞书协议）。
4. 手动试跑、启停、fire 列表、审计。
5. overlap=skip；最小模板变量；日程 **cron + interval**。
6. 连续 failed **3 次**自动 disable（Pause Schedule + 事件）。
7. Chat 强制绑定项目；触发路径 **同进程直调 Service**。
8. **发起人=创建者**；创建校验项目发起权；**失权钩子立即 disable 并写 `disabled_reason`**。
9. **前端 §7 全量**：列表/详情轨/新建编辑抽屉/闸门提示/词表/空态（非占位页）。

### P1.5

1. Plan **预授权信封**（配置时批准一版 plan 形状 / 出口 / 预算，运行时跳过或弱化 `plan_review`）——单独评审，防「配置时橡皮图章」。
2. 更丰富日程（日历排除节假日等）。
3. 失权 disable 后可选通知（飞书/站内）告知原创建者或其他项目人类成员。



### P2

1. 事件触发（webhook / 仓库 / 告警）。
2. 与流程编排对象的显式关联（若需要）。
3. 随飞书 P2：若判据卡内签署放开，自动化验收体验自动变好，本模块无差量。

---



## 9. 风险与不变量

1. **禁止**自动化 fire 绕过项目发起权：创建与每次 enable / fire 均校验 `actor_user_id` 仍可对该项目发起；失权以钩子 disable 为主路径。
2. **禁止**用自动化身份伪造人类 verdict。
3. Temporal Schedule 与 DB 规则必须可对账：禁用规则 = Schedule Pause；删除 = Delete Schedule；启动对账 job 或管理接口「修复漂移」（P1 至少有手动对账或启动自检）。
4. 飞书全员未绑定 → 与今日相同：`skipped_unbound`，Console 可处理；**自动化不额外要求飞书绑定才能跑**（绑定只影响投影，不阻塞执行）。
5. 与自治姿态校准（`2026-07-16-autonomy-posture-calibration-design.md`）的关系：自动化**不**提前兑现「默认无人」；闸门密度仍由项目/模板/模式决定。姿态校准落地后，同一规则会「自然更少卡人」，无需改调度模块。
6. **不把规则「所有权」转给其他成员**：创建者离开后规则停用，由仍有权限的人新建或（在仍有权时）由原创建者重新 enable——避免静默换人顶名发起。

---



## 10. 验收判据（GATE，真链路）

1. 创建一条 loop 规则（短 cron 或手动 trigger）→ DB 有 fire → 项目出现 demand，`source_type=automation` → 协调线程推进。
2. 同一 `idempotency_key` 重放 → 不创建第二个 demand。
3. 上一次 demand 未终态 + overlap=skip → 下次 fire 为 `skipped_overlap`。
4. plan 规则 trigger → 出现 `plan_review` 决策 → 飞书已绑定成员收到审批卡（或 Console inbox）→ resolve 后继续；与手发 plan 需求无行为差。
5. demand 进入验收待签 → 飞书结果/验收投影行为与手发一致（P1 深链 Console 签署仍成立）。
6. 禁用规则 → Schedule 暂停，不再产生 succeeded fire。
6b. 将规则创建者移出项目 → 规则立即 `enabled=false` 且 `disabled_reason` 可见；Schedule 已 Pause。
7. chat 规则 → 仅员工 chat run，无 ProjectTask / 无 demand_acceptance。
8. `verify:contracts` + 控制平面定向测试 + `migrate-validate`；Web 按 §7：空态、禁用原因中文、模式闸门提示、创建抽屉字段与试跑深链——组件测 + 浏览器真链路。

---



## 11. 实施顺序（供后续 plan 文件拆任务）

1. 迁移 + sqlc + OpenAPI（rules/fires）+ generate。
2. Service：规则 CRUD、模板渲染、Fire 幂等、调用 SubmitDemand / chat run。
3. Temporal Schedule 注册/更新/暂停与 Activity `FireAutomationRule`。
4. API handler + authz。
5. Web：按 §7 替换 `/automations`（脆数据主从表 + Sheet 表单 + 词表 + 闸门提示）。
6. Service：成员移除 / 用户停用钩子 → 批量 disable 该 actor 的规则。
7. 真链路 GATE §10（含飞书绑定成员验投影；含「踢出创建者 → 规则禁用带原因」；含浏览器走通新建/试跑/深链）。

---



## 12. 评审确认项

| # | 议题 | 结论 |
|---|---|---|
| 1 | Activity 同进程直调 vs HTTP+ServiceAuth | **同进程直调**；审计靠 source+actor |
| 2 | 日程形态 | **cron + interval**（创建时二选一） |
| 3 | Chat 是否绑项目 | **强制**（与现网 chat 一致） |
| 4 | 发起人（actor）策略 | **已拍板**——见 §12.4 |
| 5 | 连续失败自动 disable | **P1：连续 3 次 failed → disable** |

### §12.4 发起人与权限生命周期（已拍板）

1. **创建**：校验当前用户能否选择该项目（与任务中枢可选项目一致）。能选 ⇒ 至少是该项目人类成员/负责人之一 ⇒ 有权出现在审核收件（any-of-N）集合中。
2. **发起人**：固定为创建者（`actor_user_id`），不提供改挂他人。
3. **失权**：创建者被踢出项目或账号停用 → 规则**立即禁用**，`disabled_reason` 标识原因，Pause 调度；不靠等到点连败才发现。连败 3 次 disable 仅作其他失败（模板/项目停用等）的兜底。
4. **复活**：重新 enable 须人工操作，且再次校验发起权。

---



## 13. 一句话总结

**自动化任务 = Control Plane 上的定时规则 + Temporal 到点触发 + 与任务中枢同一条发起链路；人类闸门（含验收）仍按项目逻辑走，飞书只是同一套决策的投影通道——自动触发，不默认无人。**