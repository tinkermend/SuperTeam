# #2 同单接续（Demand Continuation）

- 日期：2026-08-01
- 状态：**已实施（P0a–P0d 2026-08-03；P1 resume 预检降级 2026-08-04）**
- 系列：对齐基线 `2026-07-27-workspace-and-playbook-alignment-baseline.md` §8 第 2 项
- 交付性质：CP 写路径（1 条 Atlas 迁移 + OpenAPI 契约变更）+ 派发期会话血缘传播 + Web 卷宗接续入口
- 目标读者：实施会话（本文自包含；实施前必读基线 §3/§4/§6/§7 与 `2026-07-29-demand-workbench-design.md`）
- 已拍板（2026-08-01）：
  1. **接续 = 新 demand + 血缘链**，原单终态永不回退（基线 §4.3）
  2. **会话按「员工 × 任务血缘根」续**，不是「一单一会话」——多员工单里各员工各续各的
  3. chat 不在本 spec 范围（chat 有自己的续聊链，且不产生 demand）

---

## 1. 背景与目标

### 1.1 问题

闭环走到「验收完成」就断了。今天**没有任何人发起的接续通路**：

- Web 全仓无「继续此任务」入口（`rg` 零命中）
- 控制平面从不下发 `send_input`（runtime 早已实现该命令，无调用点）
- plan/loop 无「基于原会话追加指令」的 API

用户要在已完成的一单基础上继续，只能重新提一条需求：丢血缘、丢上下文、丢 provider 会话，从零复述背景。而迭代恰恰是真实工作的常态。

### 1.2 已经对的部分（不要重做）

会话恢复机制本身**已经是正确语义**，本 spec 是复用它而不是重建：

- `FindProviderSessionForTaskRoot(tenant_id, digital_employee_id, project_task_root_id)` —— 键里带员工，多员工天然各续各的，不存在「一单复用一个会话」
- 血缘根解析 `ResolveProjectTaskLineageRoot`：`planner_metadata.revision_root_task_id` > `revision_of_task_id`（一跳）> 任务自身 id
- 会话身份在 `StartProjectTaskRun` 派发时决定，runtime 只消费
- 现网已有三类**系统发起**的接续且都能正确 resume：同任务 retry、对抗评审返工、上游补做

缺的只是：**人发起的接续**，以及**跨 demand 时血缘根断掉**。

### 1.3 目标

1. 卷宗上给出「继续这一单」入口，产出**新 demand + 血缘链**，不重走向导、不重选剧本。
2. 派发时按**员工**把新任务接回它在祖先链里的会话；接续换人则正常开新会话。
3. 修掉「人工 recovery 丢血缘根」缺陷。
4. 会话不可用时**降级而非失败**，且降级留痕。
5. 卷宗按链聚合，让「一单」的用户身份是链而不是行。

### 1.4 一句话

> **接续是新开一单并接上血缘链；谁被派到活儿，谁回自己上次那条会话。**

---

## 2. 非目标

| 不做 | 理由 / 归属 |
|---|---|
| `send_input`（往仍在跑的原任务里追加指令） | 用户要的「在同一个会话里继续」由 **resume** 实现，不是由 send_input 实现。往运行中的会话插指令是另一套并发语义（打断、排队、幂等），独立立项 |
| 同 demand 上追加任务 | 基线 §5 已否决（demand 状态单调、验收闸已消费） |
| 放开 `ProjectDemandStatusCanAdvance` 单调性 | 承重不变量，不动 |
| chat 接续 / chat 转任务继承 session | chat 有独立续聊链；基线 §7 已列非目标 |
| 变更范围可见（diff / numstat / 右轨 git 槽接真数据） | #3 |
| 轻发起、剧本归属、计划确认卡定收口 | #4 |
| 改 provider 协议或 provider 进程行为 | 基线 §4.8。降级决策放在**控制平面**，见 §6.2 |
| 接续时重选剧本 / 重选收口向导 | 基线 §4.3「接续继承，不重选」 |

---

## 3. 地基核对（2026-08-01 经代码与真实库核实，勿重复勘察）

| 事实 | 锚点 |
|---|---|
| 会话查找键含员工 | `queries/provider_session.sql` `FindProviderSessionForTaskRoot`，条件 `tenant_id + digital_employee_id + project_task_root_id`，状态 `active/idle/completed` 且 `recoverable`，按 `last_active_at` 取最新 |
| 血缘根解析 | `employee/pg_run_repository.go` `ResolveProjectTaskLineageRoot`；镜像 `projectcoordination.revisionRootTaskID` |
| 根是 metadata 不是列 | `planner_metadata["revision_root_task_id"]`；`project_tasks` **没有** root/parent/lineage 列（已查 information_schema） |
| 派发期决定会话 | `employee/run_service.go` `StartProjectTaskRun`：算 root → `metadata["revision_root_task_id"]` → `shouldAttemptSessionResume` → 查到则 `metadata["provider_session_id"]` |
| 返工写根 | `revisionPlannerMetadata` 显式写 `revision_root_task_id` |
| **缺陷：recovery 不写根** | `recoveryPlannerMetadata` 只 `cloneAnyMap(source.PlannerMetadata)`，既不写 `revision_root_task_id` 也不设 `revision_of_task_id` → 替换**原始任务**时 root 落回自身 id → 丢会话。现网 recovery 任务数为 0，属代码级确认 |
| demand 表无血缘列 | `project_demands` 列：id/tenant/project/submitted_by/title/content/source_type/source_refs/attachments/priority/risk_level/status/created_event_id/created_at/updated_at/coordination_mode/scenario_template_key |
| `source_type` 是渠道枚举 | `manual/github/ticket/document/log/automation` —— 语义是「需求从哪来」，**不是血缘**，不得挪用 |
| 按需求取任务已有查询 | `ListDemandLaunchProjectTasks(tenant, project, demand, limit)` |
| resume 现网无降级 | `providers/claude.rs` `--resume <id>`；仓库内无 resume 失败处理分支，会话文件不在则进程失败 → 任务失败 |
| 迁移用 Atlas 时间戳命名 | 最新 `20260729071624_clear_terminal_task_waiting_pointer.sql` |
| 卷宗容器已就绪 | `2026-07-29-demand-workbench-design.md` 已实施；单头刻意未放禁用接续按钮，挂点留在契约与组件边界 |

---

## 4. 数据模型

### 4.1 血缘用真列，不用 JSONB

```sql
ALTER TABLE project_demands
    ADD COLUMN continues_demand_id UUID REFERENCES project_demands (id);

CREATE INDEX idx_project_demands_tenant_continues
    ON project_demands (tenant_id, continues_demand_id)
    WHERE continues_demand_id IS NOT NULL;
```

**为什么不塞 `source_refs` JSONB**：血缘是本 spec 的承重关系，链遍历要走递归 CTE 并且要能走索引；塞 JSONB 既无索引也无外键完整性。`source_type` 更不能挪用——接续单同样可能来自 manual / 飞书 / automation，渠道与血缘是正交的两件事。

**链的形状约束（实施必须保证）**：

- 单亲：一个 demand 至多接续一个前序（不做合并式接续）
- 同项目内：`continues_demand_id` 指向的 demand 必须同租户同项目，服务端校验
- 无环：写入时校验目标不在自己的后代里；读取时递归 CTE 必须带 `depth` 上限（见 §6.1）

### 4.2 继承什么

新 demand 从父单继承（人不重选）：

| 字段 | 规则 |
|---|---|
| `project_id` | 必须同项目 |
| `scenario_template_key` | 直接继承父单**有效剧本 key**（父单为空则继承项目默认时留空，保持"有效剧本"解析规则不变） |
| `coordination_mode` | 继承 |
| `source_type` / `source_refs` | **不继承**：接续单的渠道是本次发起的渠道；`source_refs` 可带 `continues_from_demand_id` 冗余便于排查，但**权威是列** |
| `priority` / `risk_level` | 继承，允许发起时覆盖 |
| 收口 exit | **不继承**：新一单在同一剧本内重新规划自己的收口（接续的目的往往就是走更深） |

---

## 5. 会话血缘传播（本 spec 的核心，最容易做歪）

### 5.1 规则

在 `ResolveProjectTaskLineageRoot` 现有三条之后、兜底之前，插入第四条：

```text
1. planner_metadata["revision_root_task_id"] 非空        → 用它
2. revision_of_task_id 非空                              → 用它
3. ★ 任务所属 demand 有 continues_demand_id              → 沿祖先链上溯，
     找 assigned_digital_employee_id 相同 的最近任务，用该任务的 root
4. 都没有                                                → 任务自身 id（新会话）
```

**为什么在派发期做，而不是规划期**：

- 员工绑定由 route decision 决定，规划期 planner 输出不保证已定人；`assigned_digital_employee_id` 到派发时才可靠
- 派发期按员工对齐是天然正确的，**不需要 planner 做「新任务 ↔ 前序任务」的映射**
- 因此**不改 planner、不改 Temporal workflow 输入**，风险面只在一个仓储方法内

### 5.2 必须写死的判定细则（缺一条就会做歪）

| # | 细则 | 不写死的后果 |
|---|---|---|
| D1 | 「最近任务」排序钉死为：先按祖先链**距离**近的一代优先，同一代内按 `created_at DESC, id DESC` | 同一员工在父单里有多个任务（如开发做了 develop 与 release）时，取哪条不确定 → 会话随机 |
| D2 | 上溯**逐代**进行：父单没有该员工的任务，才继续找祖父单 | 一步跳到链根会跨过中间单的上下文 |
| D3 | 上溯必须有 `depth` 上限（建议 10）与环检测 | 数据被手工改出环时死循环 |
| D4 | 命中的前序任务本身若带 root（它是 revision 链的一员），继承的是**它的 root**，不是它的 id | 会话是按 root 存的，取 id 会查不到 |
| D5 | 只匹配 `assigned_digital_employee_id` **完全相同**；换人一律落回第 4 条 | 会把 A 的会话续给 B —— 用户明确反对的那种错 |
| D6 | 整条上溯必须是**一条递归 CTE 查询**，不得在 Go 里循环 N 次往返 | 派发路径是热路径，N 层链 = N 次远程往返 |
| D7 | 无 `assigned_digital_employee_id` 的任务（未定人）直接落第 4 条 | 空值参与匹配会误命中 |

### 5.3 举例（验收判据的语言）

父单有三个员工各自一条会话：开发 E1、审查 E2、测试 E3。

| 接续单派发 | 期望 |
|---|---|
| 任务给 E1 | 回到 E1 在父单的会话（同一个 `provider_session_id`） |
| 任务给 E3 | 回到 E3 自己的会话，**与 E1/E2 无关** |
| 任务给新员工 E4 | 开新会话，不得续任何人的 |
| 三层链 A→B→C，C 的任务给 E1，B 中无 E1 任务 | 逐代上溯到 A，回到 E1 在 A 的会话 |

### 5.4 修掉 recovery 丢根

`recoveryPlannerMetadata` 补写：

```go
metadata["revision_root_task_id"] = revisionRootTaskID(source)
```

与 `revisionPlannerMetadata` 对齐。**这条独立于接续功能，是既有缺陷**，可先落地。

---

## 6. 降级与边界

### 6.1 会话不可用

现状：`--resume <id>` 找不到会话 → provider 进程失败 → 任务失败。人主动接续却因为一条过期会话而整单失败，是最坏的失败模式。

**降级决策放在控制平面**（不动 provider 管道，守基线 §4.8）：

派发前在 `StartProjectTaskRun` 增加 resume 预检，命中任一条则**不下发 `provider_session_id`**（主动放弃 resume，正常开新会话）：

- 会话 `last_runtime_seen_at` 早于阈值（建议取系统配置，默认 7 天）
- 会话绑定的 runtime 节点与本次派发目标节点不同（会话文件在原机器上）
- 会话 `recoverable = false`

放弃 resume 时必须**留痕**：任务事件 + attestation metadata 写 `session_resume_skipped` 与原因码。不留痕的降级等于静默丢上下文。

runtime 侧本轮**只加留痕、不改行为**（真正的 provider 侧 resume 失败兜底属越界，若实施中发现必须改，按基线 §4.8 停下来评估）。

### 6.2 接续与治理

- 接续单照常走规划 → 计划确认闸；**不因为「是接续」而跳过人类确认**
- 父单的验收结论不自动继承给接续单；接续单有自己的收口与验收
- 缺员照常走 `planning_gap`（基线 §4.6）

### 6.3 父单状态

父单保持终态不变（completed 仍 completed）。接续**不修改父单任何字段**，只有子单持有指针。这是 F1 拍板的直接推论，实施中若发现需要回写父单，说明设计走偏了。

---

## 7. API

### 7.1 发起接续

```http
POST /api/v1/project-demands/{demandId}/continuations
```

- **operationId**：`createProjectDemandContinuation`
- 请求体：`{ title?: string, content: string, priority?: string, attachments?: [] }`
  - `content` 必填：人要说明"接着做什么"
  - `title` 缺省时服务端派生（如父单标题 + 序号），**不得留空**
- 响应 201：新建的 `ProjectDemand`（含 `continues_demand_id`）
- 鉴权：与 `submitProjectDemand` 同级（项目内写权限）
- 校验与错误：
  - 父单不存在/跨租户 → 404
  - 父单**未到终态** → 409（`demand_not_settled`）：还在跑的单应该等它跑完或纠偏，不是接续
  - 链深超上限 → 409（`continuation_chain_too_deep`）
  - 成环 → 400
- 行为：创建新 demand（§4.2 继承规则）→ 与 `SubmitDemand` 走**同一条**协调信号通路（不新开 signal 类型）

### 7.2 卷宗补链信息

`GET /api/v1/project-demands/{demandId}/dossier` 的响应增补：

```yaml
lineage:
  type: object
  required: [chain_position, chain_length, chain]
  properties:
    continues_demand_id: { type: string, format: uuid, nullable: true }
    chain_position: { type: integer }   # 本单是链上第几单，从 1 起
    chain_length:   { type: integer }
    chain:                              # 全链摘要，时间正序
      type: array
      items:
        type: object
        required: [demand_id, title, status, created_at]
        properties:
          demand_id:  { type: string, format: uuid }
          title:      { type: string }
          status:     { type: string }
          created_at: { type: string, format: date-time }
          is_current: { type: boolean }
actions:
  type: object
  properties:
    continue_demand:
      type: object
      required: [available, reason_code]
      properties:
        available:      { type: boolean }
        reason_code:    { type: string }   # ok / demand_not_settled / chain_too_deep
        reason_message: { type: string }   # 中文
```

`actions.continue_demand` 让前端不必自己判"能不能接续"——判据是服务端的。

---

## 8. Web

### 8.1 入口

卷宗单头（`demand-dossier-header.tsx`）主操作区加「继续这一单」：

- `actions.continue_demand.available = false` 时按钮**不渲染**（不放禁用按钮——#1 已经定过这个调子），改为在链条区显示原因中文
- 点击 → 轻量弹层：一个多行输入（"接着要做什么"）+ 可选优先级；**没有向导、没有剧本选择、没有收口选择**（基线 §4.3）
- 提交成功 → `navigate` 到新单的 canonical 深链 `?tab=demands&demand={newId}`（TanStack Router，禁止整页跳转）

### 8.2 链的呈现

- **单头**加一行链条：`本单为第 k / n 次接续`，左右可跳前一单/后一单；父单标题可点
- **左轨需求列表按链折叠**：一条链只占一行，显示**链上最新一单**的状态与标题，副行标 `接续 n 次`；展开可切换链内各单
  - 这是"一单的身份是链"的直接落地；不做折叠就会出现"每接续一次多一个单"，正是 F1 要避免的
- 链内切换**不改 URL 形态**，仍是 `?tab=demands&demand=<id>`

### 8.3 文案

- 中文枚举经 `status-labels.ts`；`reason_code` 需补词表
- 不得出现"接续/血缘"以外的新黑话；面向用户统一说「继续这一单」

---

## 9. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0a** | recovery 丢根修复（§5.4） | **已实施**：三态单测，经临时移除修复实证会挂 |
| **P0b** | 迁移 + `continues_demand_id` + 接续 API + 链继承规则 | **已实施**（迁移落为 `20260803180000_*`） |
| **P0c** | 派发期血缘传播（§5.1/§5.2，含递归 CTE 与 D1–D7） | **已实施**：D1–D5/D7 + 成环停机由真库测试锁住 |
| **P0d** | 卷宗 lineage 字段 + Web 入口 + 左轨按链折叠 | **已实施**：浏览器真链路走通 |
| **P1** | resume 预检降级 + 留痕（§6.1） | **已实施**：真库造过期会话，派发照常且留痕 `session_stale` |

**默认交付 = P0a–P0d 全做完**。P1 可紧随。若排期砍刀：**不得只做 API 不做传播**（那就是"接续了但会话没接上"，等于没做）。

---

## 10. 验收 GATE（真实 E2E，非 mock）

环境：当前代码的 Web + CP + DB + runtime + 真实 provider。

| ID | 步骤 | 期望 |
|---|---|---|
| G1 | 单员工单完成后接续，派发同一员工 | 新任务 run 的 `provider_session_id` **等于**父单该员工的会话 id（DB 直查比对） |
| G2 | 多员工单（≥2 个 DE 各有会话）接续，只派发其中一个 | 该员工回自己会话；另一员工会话 `last_active_at` 不变 |
| G3 | 接续换成从未参与的员工 | 新会话（`provider_session_id` 与链上任何既有会话都不同） |
| G4 | 三层链 A→B→C，B 中无该员工任务 | C 的任务逐代上溯回到 A 的会话 |
| G5 | 接续后检查父单 | 状态、验收结论、终态通知**均未变化** |
| G6 | 对未到终态的单调接续 API | 409 `demand_not_settled` |
| G7 | 人工 recovery 重试一个**原始任务** | 替换任务仍回到同一会话（修复前会开新会话） |
| G8 ✅ | 造一条过期/跨节点会话后派发 | **已验证**：任务照常 running、另起新会话，派发 metadata 带 `session_resume_skipped=session_stale` 与被放弃的会话 id |
| G9 | 浏览器：完成单 → 点「继续这一单」→ 填写 → 提交 | 落到新单卷宗；单头链条显示第 2/2；左轨该链只占一行 |
| G10 | 链上任一单的深链 | 均可直达，左轨高亮该链，能切到链内其他单 |
| G11 | `verify:contracts` + `verify:control-plane` + `verify:web` + `make migrate-validate` | 全过 |

**完成定义**：G1–G11 全过；基线 §4 不变量无破坏；§2 非目标未偷做。

---

## 10.1 实施期新增的两条判据（原 spec 未覆盖，E2E 揪出）

1. **只有链尾可接续**（`already_continued`）。原设计只说单亲，没禁止一个父单接出多个子单；实测发现链中间的单仍可接续，会把线性链分叉，而「第 k / n 次」与链条展示都建立在线性之上。写路径 409、读路径判据同步。
2. **归档项目不可接续**（`project_archived`）。`SubmitDemand` 早有归档门禁，但卷宗判据没表达它——真实点击时出现「界面说能接、点了报 409」。判据是唯一真相源，必须与写路径同源。

**另修一处集成缺陷**：接续成功后未失效父页需求列表，导致跳到新单时左轨里没有它（链也折不出来）。不能只依赖 SSE——那是尽力而为的，而这里是刚发生的确定事实。

---

## 11. 风险

| 风险 | 缓解 |
|---|---|
| **会话续错人**（最严重） | D5 钉死完全匹配；G2/G3 是必过判据；单测覆盖"换人不得命中" |
| 上溯排序不确定 → 会话随机 | D1 钉死排序；单测造"同员工父单两条任务"验证取哪条 |
| 递归 CTE 无上限 → 环导致挂死 | D3 深度上限 + 写入期环校验 + 单测造环 |
| 派发热路径变慢 | D6 单条查询；只在 `continues_demand_id` 非空时触发（普通单零额外开销） |
| 左轨按链折叠改动大 | 折叠是读路径纯投影；实在超期可 P0d 先出链条 + 入口，折叠留 P1（但必须同期，否则"每接续一次多一个单"立刻可见） |
| resume 预检误判导致该续的没续 | 阈值走系统配置可调；留痕必须带原因码，便于事后区分"该续没续"与"本就不该续" |
| 接续绕过人类确认 | §6.2 明确：接续单照常走计划确认闸 |
| 共享工作树 git 踩踏 | 显式路径 `git add`，禁 `-A` |

---

## 12. 开放细节（不阻塞立项；实施默认值）

1. 链深上限默认 10，走系统配置 key 可调。
2. 接续单标题派生规则：`{父单标题}（接续 {k}）`，人可在弹层覆盖。
3. `chain` 返回全链还是最近 N 单：默认全链（链深有上限，量可控）。
4. 父单为 `failed`/`cancelled` 时是否允许接续：**允许**（"跑挂了接着修"是核心场景），G6 只拦未到终态的。
5. 是否把父单结论注入接续单的规划上下文：**P1 观察项**。默认不注入——人在接续输入里自己说清楚更可靠；若实测规划质量差再评估（注入点在快照构建的活动内，不触及 workflow 输入）。

---

## 13. 文档关系

| 文档 | 关系 |
|---|---|
| `2026-07-27-workspace-and-playbook-alignment-baseline.md` | **基线**；本 spec 是其 §8 #2，F1/F2 拍板见其 §4.3/§4.4 |
| `2026-07-29-demand-workbench-design.md` | 本 spec 的容器（已实施）；接续入口与链条挂在其单头与左轨 |
| #3 变更范围可见 / #4 轻发起与剧本归属 | 均只依赖 #1，与本 spec 无先后强约束 |

实施完成后：将本文状态改为「已实施」，并在基线 §8 第 2 项链到本文路径。

---

## 14. 审阅检查清单（给人）

- [ ] 同意接续产出新 demand、父单完全不动
- [ ] 同意血缘用真列 `continues_demand_id`（含一条迁移），不塞 `source_refs`
- [ ] 同意会话传播放在**派发期**按员工对齐（不改 planner、不改 Temporal）
- [ ] 同意 D1–D7 七条细则作为实施必须项
- [ ] 同意左轨按链折叠（否则每接续一次多一个"单"）
- [ ] 同意 resume 降级判据放控制平面、runtime 只留痕
- [ ] 同意 failed/cancelled 的单可以接续

---

## 15. 一句话方案

> **卷宗上「继续这一单」产出接血缘链的新 demand；派发时按员工逐代上溯祖先链取回它自己的会话根，换人则开新会话；父单终态不动，链在卷宗里折叠成一单。**
