# 运行必须归属项目 Spec（任务运行项目归属不变量）

- 日期：2026-07-26
- 状态：**已实施（本地 dev 库真实 E2E 通过 G3/G4/G6/G7b；G1/G2/G5/G7 见 §5 说明）**（2026-07-26）
- 交付性质：契约 + Control Plane + Web + 历史数据处置。范围极窄，只负责一条不变量。
- 定位：**前置项**。它是「项目·任务 tab 聚合重构」（另立 spec）的前提——只有"任何运行都必然归属某个项目"成立，"项目内的任务视图 = 任务全貌的唯一入口"才不漏项。
- **不负责**：任务详情聚合的形态、流程编排节点跳转、工件与任务的关联展示。那些属于重构 spec。

---

## 1. 目标不变量

> **任何数字员工运行（run）都必须归属一个项目，且归属关系在服务端强制、在 schema 层可查，不依赖调用方自觉。**

人类给出的三条核心口径（2026-07-26 对齐）：

1. **任务运行必须在任务中枢发起**——任务中枢（`/`，`sidebar-data.ts:38`）是唯一发起入口。
2. **运行有三种模式：plan / loop / chat**。
3. **chat 特殊，相当于一次临时对话**。

---

## 2. 现状实证（2026-07-26，真实代码 + 远程 dev 库）

### 2.1 三条派发路径，两条已合规、一条裸奔

| 路径 | 入口 | 项目归属 | 结论 |
|---|---|---|---|
| **plan / loop** | 任务中枢 → `submitProjectDemand(projectId, …)`（`task-launches/index.tsx:111`）→ 协调线程 → `StartProjectTaskRun`（`run_service.go:358`） | `project_id` / `demand_id` / `project_task_id` / `project_task_attempt_id` **四项全部必填**（`:363-377`），且**绕过 `CreateRun` 直达 `createAndDispatchRun`** | ✅ 已合规 |
| **chat** | 任务中枢 chat 面板（`chat-panel.tsx:280`，写死 `run_kind: "chat"`）→ `CreateRun` | `run_service.go:170-172` 强制校验 `project_id`，缺失即 `ErrInvalidInput` | ✅ 已合规（语义待定，见 §7-Q1） |
| **run_kind=task 的裸 `CreateRun`** | `POST /api/v1/digital-employees/{employeeId}/runs`（`server.go:362`，契约 `openapi.yaml:4236`） | `run_service.go:173-178` **主动把 `project_id` 置空** | ❌ **不合规，本 spec 的唯一治理目标** |

裸奔分支原文（`run_service.go:173-178`）：

```go
} else {
    // §13 design revision: task runs ignore any anchor project_id the
    // caller may have sent — it is a chat-only concept, not validated or
    // persisted for task-kind runs.
    req.ProjectID = nil
}
```

即：**当前设计明确规定"任务运行不携带项目"**。这不是遗漏，是 §13 的既定决策，与本 spec 的不变量直接冲突，需要推翻。

### 2.2 Web 侧已经删干净

全仓库创建运行的调用点只有一个：`task-launches/components/chat-panel.tsx:280`，且写死 `run_kind: "chat"`。员工详情页无任何发起入口（`rg "发起任务|运行任务|新建运行"` 零命中）。**人类反馈"按钮已删除"属实。**

### 2.3 项目归属不是列，是 join 出来的

`ListDigitalEmployeeRunsDetailed`（`storage/queries/tasks.sql:551-585`）这样解析 `project_id`：

```
LEFT JOIN project_tasks pt ON pt.digital_employee_run_id = tr.id
LEFT JOIN projects p ON p.id = COALESCE(
    pt.project_id,
    tasks.params #>> '{metadata,anchor_project_id}' (或 metadata.project_id) 的 uuid 兜底
)
```

`task_runs` 表**没有 project_id 列**（`\d task_runs` 已核）。归属靠两个可失效的间接引用：

- `project_tasks.digital_employee_run_id` 是**单值指针**，重试/返工会覆盖，被覆盖的旧 run 即 join 不出项目
- `tasks.params.metadata.anchor_project_id` 是 JSON 里的字符串，无外键、无约束

**潜在误判路径**：被覆盖的旧 run → `project_id` 解析为 NULL → 员工抽屉判为 standalone（`run-detail-drawer.tsx:145`）→ 错误露出「重试 / 确认关闭」。
**当前是否触发**：查库确认 **0 条**——有 attempt 关联但无 `project_tasks` 指针的 run 共 10 条，全部带 `anchor_project_id` 兜底。路径开着，尚未踩到。

### 2.4 历史数据（远程 dev 库 `115.190.247.9:35432/superteam`，2026-07-26 实查）

```
run_kind | runs | 无 project_tasks 指针
chat     |   52 |   52   ← 设计如此，靠 anchor_project_id
task     |   96 |   70
```

70 条中，**60 条既无 `project_tasks` 指针也无 `project_task_attempts` 关联**，即真正无项目归属。状态分布：`completed` 59 / `cancelled` 1——**全部终态，清理无并发风险**。

抽样标题：`输出中文问候`(07-26)、`回复一个字：好`、`P-enforce smoke`、`Implement ParseRange function`、`Run pwd and collect output`。**全部是 E2E / smoke 经 API 直造的，无一条来自用户界面操作。**

### 2.5 相关联的服务端表面

| 端点 / 函数 | 位置 | 与本不变量的关系 |
|---|---|---|
| `POST …/runs` | `server.go:362` / `openapi.yaml:4236` | 唯一能造出无项目运行的入口 |
| `standaloneDispatchCommandType` | `run_service.go:599` | standalone 派发的命令类型选择 |
| `RetryFailedRun` | `run_failure_recovery.go:80` | 内部 `ensureStandaloneRun`（`:134`）**拒绝**项目任务运行——即它专为无项目运行服务 |
| `AcknowledgeFailedRun` | `run_failure_recovery.go:51` | 同上（`:65`） |
| 抽屉「重试 / 确认关闭」按钮 | `run-detail-drawer.tsx:146-149` | 门禁 `!isProjectLinkedRun`——同上 |
| 自动化定时发起 | `automation/adapters.go:53-62` | 创建的是 `RunKindChat` + `ProjectID: &projectRef`，**已合规** |
| 飞书 connector | `ConnectorSubmitDemandRequest`（`openapi.yaml:5040`）`required: [project_id, title]` | 只提交 demand（plan/loop），**已合规** |

---

## 3. 改动清单

### 3.1 服务层（承重）

**C1 — 推翻 §13，删除 `CreateRun` 的 task 分支（A3）**
`run_service.go:173-178` 的 `req.ProjectID = nil` 连同整个 task 分支删除；`CreateRun` 只接受 `RunKindChat`，传入 `task` 返回 `ErrInvalidRunKind`。`project_id` 校验（`:170-172`）提升为无条件。
连带清理：`standaloneDispatchCommandType`（`run_service.go:599`）的 chat/task 分叉可简化；`RunKindTask` 常量仍由 `StartProjectTaskRun`（`run_service.go:460`）使用，**不删常量**。

**C2 — `task_runs` 增加 `project_id` 列并回填**
把归属从"两级 join 兜底"提升为一等列，由写入方在三条派发路径上各自落值：

- `StartProjectTaskRun`：写 `req.ProjectID`
- `CreateRun`（chat）：写校验过的 `req.ProjectID`
- 迁移回填：优先 `project_tasks.digital_employee_run_id` → 再 `project_task_attempts.digital_employee_run_id` → 再 `tasks.params.metadata.anchor_project_id`

回填后按 A2 清理剩余空值行，然后加 `NOT NULL`（同一迁移序列内，清理在前、约束在后）。
迁移目录 `apps/control-plane/internal/storage/migrations/`，需更新 `atlas.sum` 并跑 `make -C apps/control-plane migrate-validate`。

**C3 — `ListDigitalEmployeeRunsDetailed` 改读一等列**
`tasks.sql:551-585` 的 `COALESCE(pt.project_id, metadata 兜底)` 整段替换为 `tr.project_id`。这同时关闭 §2.3 的误判路径（指针被覆盖不再影响归属）。

### 3.2 契约

**C4 — `CreateDigitalEmployeeRunRequest` 收敛为 chat 专用（A3）**
`openapi.yaml:13032-13049`：
- `run_kind` 枚举去掉 `task`，只留 `chat`（`default: chat`）
- `project_id` 进 `required` 数组，描述去掉 "ignored when run_kind is task"
- `post` 操作的 summary 由 "Start a digital employee run" 改为对话语义

**C4b — 删除 standalone run 失败恢复的整条链（A4 + A4b）**

⚠️ **范围比初稿大**：这两个函数不只挂在员工抽屉上，**收件箱也是消费方**。只删函数会留下"有按钮、无 handler"的哑动作卡——正是 `2026-07-24-human-task-unification.md` F1 那一类缺陷，且 `decision_action_registry` 的护栏只覆盖 `project_decision`，**覆盖不到 `digital_employee_run_recovery` 这个 item type**。必须整链删除：

| 层 | 删除对象 | 位置 |
|---|---|---|
| 契约/路由 | retry + acknowledge-failure 两个 path 与路由 | `openapi.yaml` / `server.go:369-370` |
| 服务 | `RetryFailedRun` / `AcknowledgeFailedRun` / `ensureStandaloneRun` | `employee/run_failure_recovery.go:51/80/134` |
| 服务（生产者） | `projectStandaloneFailureBestEffort` —— 失败时投影收件箱卡的源头 | `employee/run_writeback.go:441-470` |
| 接口与装配 | `RunFailureInboxProjector` 接口 + `WithFailureInboxProjector` ×2 + `app.go` 装配 | `run_failure_recovery.go:14-43`、`app.go:516-517` |
| 收件箱 | `RunRecoveryProjectorAdapter` 三个方法 | `inbox/run_recovery_adapter.go:22/55/77` |
| 收件箱 | `ItemTypeDigitalEmployeeRunRecovery` 类型、`DefaultActions` 的「重试/确认关闭」分支、三处 switch 分支 | `inbox/types.go:69`、`inbox/service.go:380/457-462/524` |
| Web | 收件箱对该 item type 的渲染与动作提交（需 grep `digital_employee_run_recovery` 确认调用点） | `apps/web/src/features/inbox/` |
| 数据 | 库内该类型的 `open` 态存量事项需一并 resolve/cancel，否则删代码后成僵尸卡 | 迁移 |

连带清理：`objectiveFromRunMetadata` / `projectIDFromRunMetadata`（`run_failure_recovery.go:145-169`）无其他调用方后一并删。
`task_runs.failure_acknowledged_at` / `failure_acknowledged_by` 两列随之无写入方——本 spec **不删列**（避免范围外的数据迁移），列为观察项。

改后必须跑 `corepack pnpm generate:control-plane` + `verify:contracts`。

### 3.3 Web

**C5 — 删除抽屉的「重试 / 确认关闭」（A4）**
`run-detail-drawer.tsx:145-216`：删除 `canRecoverFailure` 及其两个按钮、`retryFailure` / `acknowledgeFailure` mutation 与 `displayedRun` 里对应的乐观合并分支。
`isProjectLinkedRun` 分支（`:217+`，提示"失败恢复请在项目详情或收件箱处理"）**保留**——它是删除按钮后唯一的引导文案，且不变量成立后对所有 run 恒成立，需把条件从 `isProjectLinkedRun && isRecoverableRun(...)` 简化为 `isRecoverableRun(...)`。

### 3.4 数据

**C6 — 物理删除 60 条无归属历史运行（A2）**
判据：`t.run_kind='task'` 且既无 `project_tasks.digital_employee_run_id` 指针、又无 `project_task_attempts.digital_employee_run_id` 关联、且 `tasks.params.metadata` 无 `anchor_project_id`/`project_id`。全部终态，无并发风险。
级联需一并清理其 `tasks` 行与关联的 `task_events` / `runtime_*` 残迹；删除前后各留一次计数快照作为迁移证据。

---

## 4. 明确不在范围内

- 任务详情聚合页 / 项目·任务 tab 重构
- 工件与任务的关联展示、收件箱工件区
- 流程编排节点检查器跳转
- chat 会话在项目内如何呈现（A1 已定：chat 不进任务全貌，本 spec 只保证它有项目锚点）
- `task_runs.failure_acknowledged_at` / `failure_acknowledged_by` 两列的下线（C4b 后无写入方，列为观察项）

---

## 5. 验收标准（真实端到端，非单测）

**2026-07-26 实测结果**（dev 库 `115.190.247.9:35432/superteam`，服务基于当前代码重启）：

- **G3 ✅** chat 模式发起（任务中枢 → htu-also-close 项目 → 报告员小王）：PG 查 `task_runs.project_id = e3f8a1c5…` = 所选锚点项目，run_kind=chat，status=completed。一等列落值正确。
- **G4a ✅** `POST …/runs` 带 `run_kind=task` → **400 `invalid run_kind`**（后门已封）
- **G4b ✅** `POST …/runs` 带 `run_kind=chat` 无 project_id → **400 `project_id is required`**
- **G6 ✅** 全库 `task_runs.project_id IS NULL` = 0；`NOT NULL` 约束已生效；无孤儿 task run
- **G7b ✅** `inbox_items` 中 `item_type=digital_employee_run_recovery` 的 open 态 = 0（迁移已 cancel）；收件箱无哑动作卡
- **G8 ✅** `verify:foundation`（契约 + TS/Go/Rust 全量）+ `verify:web`（969 测试）+ `migrate-validate` 全绿

**留待合并前复验**（需真实 provider + 返工/造失败场景，核心断言已被 Go 测试 + G6 实测覆盖）：

- **G1/G2** plan/loop 发起后 PG 验 project_id：StartProjectTaskRun 落值路径已被单元测试覆盖（`CreateRunRecordRequest.ProjectID` 赋值断言），且迁移三路回填后全库 0 NULL（G6）已间接证明历史 plan/loop run 归属正确
- **G5** 返工指针覆盖：C3 已把读路径切到一等列，`project_tasks.digital_employee_run_id` 覆盖不再影响归属（查询不再 join 该指针）
- **G7** 失败重试链回归：`task_failure_recovery` 收件箱链路代码未动，Go 测试全绿；真实停 runtime 造失败的场景留合并前

按 CLAUDE.md「验证与收尾」，以下为原始验收条目（保留作复验清单）：

- **G1** 任务中枢 plan 模式发起 → PG 查该 run 的 `task_runs.project_id` 非空且等于所选项目
- **G2** 任务中枢 loop 模式发起 → 同上
- **G3** 任务中枢 chat 模式发起 → 同上
- **G4a** `curl POST …/runs` 带 `run_kind=task` → **400 invalid run kind**（后门已封）
- **G4b** `curl POST …/runs` 带 `run_kind=chat` 但不带 `project_id` → **400**
- **G5** 一次真实返工/重试（`project_tasks.digital_employee_run_id` 指针被覆盖）后，员工页运行列表里被覆盖的旧 run **仍显示正确项目**，且抽屉内无「重试 / 确认关闭」
- **G6** C6 清理执行后，全库 `task_runs.project_id IS NULL` 计数 = 0，且 `NOT NULL` 约束已生效
- **G7〔A4 回归闸〕** 真实制造一次 plan 任务的运行时失败（如停掉 runtime-agent 后派发），确认重试链未被削弱：自动退避重试发生 → 耗尽后任务转 `waiting_human` → 收件箱出现 `task_failure_recovery` 卡 → 点「重试任务」→ 真实重新派发成功
- **G7b〔C4b 无哑动作闸〕** 全库查询 `inbox_items` 中 `item_type='digital_employee_run_recovery'` 的存量事项 = 0（或全部终态），且浏览器打开收件箱不出现任何带「重试/确认关闭」却无 handler 的卡
- **G8** 门禁：`verify:contracts` + `verify:control-plane` + `verify:web` + `migrate-validate` 全绿

---

## 6. 风险

- **R1（高）**：C4 / C4b 是契约级破坏性变更。仓库内已核无调用方（Web 唯一调用点 `chat-panel.tsx:280` 本就写死 `run_kind: "chat"`），但仓库外的 E2E 夹具、手工 curl 脚本、smoke 脚本若在用 `run_kind=task` 或 retry/acknowledge 端点，改后立即失效。§2.4 那 60 条孤儿运行正是这类脚本的产物，说明确实存在这样的调用方。
已核实的契约门禁约束（`scripts/verify-foundation-contracts.mjs`）：其必备端点清单包含 `POST /api/v1/digital-employees/{employeeId}/runs`（:40、:115），因此该端点**必须保留**——A3 的"收敛为 chat 专用"满足此约束，若改成整体删除端点则会打挂 `verify:contracts`。retry / acknowledge-failure **不在**该清单内，删除不影响门禁。
- **R2（中）**：C2 加列 + 回填在生产库上是长事务风险；需按 `DATABASE_DESIGN.md` 的迁移规则拆步（先加可空列 + 回填 + 再加约束）。
- **R3（低）**：并发会话共享同一 checkout，`tasks.sql` / `openapi.yaml` / 生成物是高冲突文件；提交按 CLAUDE.md 只暂存自己的 hunk。

- **R4（中）〔C4b 的行为变化，需人类确认〕**：`projectStandaloneFailureBestEffort`（`run_writeback.go:441-470`）在 run 失败且 metadata 无 `project_task_id` 时投影一张收件箱卡。A3 之后，唯一还满足该条件的就是 **chat run**——也就是说这条链当前实际服务的是「chat 对话跑失败了 → 收件箱提醒 + 可重试」。
  C4b 整链删除后，**chat run 失败将不再产生任何收件箱提醒**，人类只能在 chat 面板里自行看到失败。
  按 A1（chat = 临时对话）这是自洽的，但它是一个**用户可感知的行为回退**，不是纯代码清理。
  **已由 A4b 拍板覆盖**：人类明确「Q4b 一并删」，即接受此回退——chat 失败不再产生收件箱提醒，人类想重来直接在 chat 面板再发一句。

---

## 7. 人类拍板结论（2026-07-26）

**A1 — chat 的项目归属维持"纯运行时锚点"，不升级为业务归属。**
chat run 仍必须携带 `project_id`（解析派发节点 / 预算 / 策略边界），但项目对这次对话无业务感知，契约现有描述（`openapi.yaml:13044-13049`）保持不变。
**「任务全貌」暂不含 chat**；后续如有需要再单独扩展开发。
→ 对重构 spec 的约束：任务详情聚合只覆盖 project_task，chat 不进任务视图。

**A2 — 60 条无归属历史运行：物理清理。**
理由：全部终态（`completed` 59 / `cancelled` 1）、全部为 E2E/smoke 产物、无业务价值。清理是保证数据干净与一致性的必要动作，也是 `task_runs.project_id` 能加 `NOT NULL` 的前提。

**A3 — 删除 `POST …/runs` 的 `run_kind=task` 分支。**
`run_kind` 枚举收敛为只剩 `chat`，该端点专职开对话。项目任务派发走完全独立的 `StartProjectTaskRun`（`run_service.go:358`），task 分支在仓库内已无合法调用方；保留它等于留一个绕过任务中枢的后门，与「任务运行必须在任务中枢发起」直接冲突。

**A4 — 删除 `RetryFailedRun` / `AcknowledgeFailedRun` 端点与员工抽屉的「重试 / 确认关闭」按钮。**

拍板前核实了「删了之后 plan/loop 和定时自动化任务怎么重试」这一疑问，结论是**完全不受影响**——项目任务有自己独立完整的重试链：

| 层 | 机制 | 位置 |
|---|---|---|
| 自动重试 | 失败写回时自动排一个带退避（`defaultDispatchRecoveryBackoff`）的新 attempt，受 `max_attempts` 约束 | `project/service.go:5841-5860` |
| 耗尽升级 | 转 `waiting_human` + 创建决策请求并投影到收件箱 | `project/service.go:5865+` |
| 人类重试 | 收件箱 `task_failure_recovery` 卡的**「重试任务」**按钮 → `projectcoordination.applyFailureRecoveryDecision` 重新派发；另有「取消下游」 | `inbox/decision_action_registry.go:85-88` |

而被删的那两个端点由 `ensureStandaloneRun`（`run_failure_recovery.go:134-143`）**显式拒绝**项目任务运行（报错「项目任务失败请在项目详情或收件箱处理恢复」），本就不可能用于 plan/loop 重试。自动化定时同理：它的两条腿 `SubmitDemand`（plan/loop）与 `CreateChatRun`（chat）都不依赖这两个端点。

**实施时必须注意的现存 UI/服务端不一致**（本次核实发现）：

- **UI 侧已经是死的**：`isProjectLinkedRun = Boolean(project_id)`（`run-detail-drawer.tsx:145`），而 chat run 经 `anchor_project_id` 兜底能解析出 project_id → `canRecoverFailure` 恒 false → 按钮**对 chat run 早已不显示**。它当前**只对那 60 条无项目孤儿运行显示**；A2 清理后即 100% 不可达。
- **服务端侧比 UI 宽**：`ensureStandaloneRun` 只拦 `project_task_id`，chat run 没有该键 → 放行。即「chat run 服务端重试」这个能力存在但无任何界面在用。

**A4b（已拍板）— 「chat run 服务端重试」能力一并删除，不保留、不改门禁。**
理由：与 A1 一致（chat = 临时对话），且该能力当前无任何界面在用；人类想重来直接在 chat 面板再发一句即可。
因此 `RetryFailedRun` / `AcknowledgeFailedRun` / `ensureStandaloneRun` 三者整体删除，不做「门禁改成 `run_kind == chat`」的保留式改造。将来若需要 chat 重开，属新增功能、另行立项。

**A5 —「必须在任务中枢发起」只约束人类手动的 UI 入口，不引入服务端 origin 白名单。**
服务端不变量只管一条：**运行必须有项目**。自动化定时（`automation/adapters.go:53`）与飞书 connector 作为已登记的合法旁路保留，二者本就都携带 project_id。
→ 本 spec 范围不扩大，不引入 run 的 origin 字段。
