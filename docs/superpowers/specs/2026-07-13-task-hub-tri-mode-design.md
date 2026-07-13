# 任务中枢三模式:Plan / Loop / Chat 设计

- 日期:2026-07-13
- 状态:已评审定稿,待实现
- 前置:`2026-07-10-project-plan-phase-refactor-design.md`(§4.7 定义 plan/loop 行为差异,§6 声明 chat 隔离原则)

## 1. 背景与问题

最初的产品愿景是任务提出时可选 plan / chat / loop 三种模式。现状盘点(2026-07-13):

- **UI 无入口**:任务中枢(`/` → `TaskLaunchForm`)只有需求描述、项目、优先级、风险级别,没有任何模式选择;且优先级/风险级别是纯本地状态,提交时被丢弃(`SubmitProjectDemandInput` 无对应字段),属既有装饰性缺陷。
- **plan/loop 无字段**:上游补链机制已实现(`workflow.go` `handleEmployeeTaskCompleted` 的 `blocked_resolvable_upstream` 分支 → `CreateUpstreamSupplementTasks`,迁移 054 `plan_iteration` + `max_plan_iterations` 上限),但全仓没有 mode 字段,**等于所有项目硬编码跑 loop 行为**,plan 模式(阻塞时停下报人类)不存在。
- **chat 零实现**:2026-07-10 spec 将其列为非目标并留给独立立项,即本 spec。

本 spec 一次闭环三模式:任务中枢成为三模式入口,plan/loop 落到协调链路,chat 作为数字员工的直接对话通道落地。

## 2. 目标

1. 任务中枢提供三态切换 `[Plan 任务 | Loop 任务 | 对话]`。
2. chat:用户与指定数字员工单次对话(一问一答 + 可追问),结果就地呈现,产出默认隔离(quarantined),唯一正式出口是"转为任务"。
3. plan/loop:模式随需求提交、冻结进 plan revision,workflow 在上游阻塞分支按模式分流(报人类 / 自动补链)。
4. Runtime 与 Provider 层**零改动**。

## 3. 非目标

- 不做多轮会话中心、会话列表、独立对话页面(线程视图为页面本地状态)。
- 不做"提升为证据"(chat 回答内容写入项目证据对象),留下一版;quarantined→promoted 的完整语义届时再设计。
- 不给 chat run 定义只读/受限执行档位,不改任何 provider spawn 参数(见 §11 取舍)。
- 不实现优先级/风险级别的真实接线(本次从表单移除,接线另行处理)。
- 不改 `max_plan_iterations` / `revision_max_attempts` 既有上限机制。

## 4. 决策总览(评审中逐项确认)

| 决策点 | 结论 |
|---|---|
| 对话对象 | 指定数字员工(非协调线程、非通用助手) |
| 结果呈现 | 任务中枢就地流式呈现;持久留痕 = 员工 run 历史 + transcript |
| 审批/编排 | chat 不进协调链路、不触发审批;授权人 = 打字的人类本人;产出隔离兜底 |
| 会话连续性 | 一问一答 + 可追问(provider session resume);首问强制新会话 |
| 首版出口 | 问答 + 转为任务;"提升为证据"下一版 |
| 实现载体 | 复用 `POST /api/v1/digital-employees/{employeeId}/runs` 独立 run 路径(方案 A) |
| plan/loop 范围 | 并入本 spec 一次闭环 |
| 模式缺省 | 新需求缺省 `plan`;存量 plan revision 无 mode 按 `loop` 解释(保持现行为) |

## 5. Chat:架构与数据流

**定位一句话:chat = 数字员工的一次普通 standalone run,平台侧归类为对话,不触碰项目协调链路。**

```
任务中枢 [对话模式]
  │ POST /api/v1/digital-employees/{id}/runs
  │   { objective: 问题, run_kind: "chat", resume_of_run_id?: 上次runId }
  ▼
Control Plane RunService(归类 + 会话延续裁决)
  ▼
Runtime Agent → Provider(spawn 参数与今天完全一致)
  ▼
事件流 / transcript 照常回传(053 raw log 基建)
  ▼
任务中枢轮询 GET runs/{runId} + /events → 就地渲染回答
  ├─ [追问] → 新 run,resume_of_run_id 指向上次
  └─ [转为任务] → 前端切任务模式预填 → 正常 demand 链路
```

四条不变量:

1. chat run 永远不出现在项目协调线程视野里——无 signal、无 ProjectTask、无 RouteDecision。
2. chat 产出默认无业务效力:不进证据链、不触发任何业务流转。
3. 进入正式链路的唯一通道是人类编辑并提交的需求文本(转为任务)。
4. 审计照常:run 记录、事件流、transcript 与任务 run 同等落盘,可追溯不可豁免。

**就地流式呈现首版用轮询**(1–2s 轮询 run + events),不新建 WebSocket/SSE 通道;体感不足再升级。

## 6. Chat:契约与控制平面改动

### 6.1 契约(`contracts/control-plane/openapi.yaml`,改后走 `generate:control-plane`)

- `CreateDigitalEmployeeRunRequest` 新增可选字段:
  - `run_kind`: `task | chat`,缺省 `task`(存量调用全兼容)。纯分类标签,封闭枚举,值域服务端校验。
  - `resume_of_run_id`: uuid,追问时指向上一个 chat run。
- `DigitalEmployeeRun` 及列表项回传 `run_kind`;`listDigitalEmployeeRuns` 新增 `run_kind` query 过滤。

### 6.2 数据库(一个迁移,编号以实现时为准)

勘误(2026-07-13 计划期核实):没有 `digital_employee_runs` 表,standalone run 实际存 `tasks` + `task_runs` 两表(sqlc CTE `CreateDigitalEmployeeTaskRun` 一次插入)。归类字段落 `tasks`:

```sql
ALTER TABLE tasks
    ADD COLUMN run_kind VARCHAR(20) NOT NULL DEFAULT 'task',
    ADD COLUMN resume_of_run_id UUID;
-- CHECK (run_kind IN ('task','chat'));追问血缘仅审计用,不做 FK 级联
```

照例更新 `atlas.sum` + `make -C apps/control-plane migrate-validate`。

### 6.3 RunService(`internal/employee/run_service.go`)

- `run_kind` 落库并透传到 run 响应;能力、凭据、工作区注入**与现状 standalone run 完全一致**,不做任何裁剪。
- **首问**:强制新会话(payload `continue_session=false`)。显式设置,防止员工 session policy `resume:true` 时聊天误吸此前任务 run 的上下文。
- **追问**:`resume_of_run_id` → 从上次 run metadata 取 `provider_session_id`(现有锚点 `run_service.go:224`)→ payload 带 `session_id` + `continue_session=true`(Claude 侧即普通 `--resume`)。
- **resume 校验矩阵**(任一不满足 → 400):同租户、同员工、上次 run `run_kind=chat`、上次 run 已终态、session id 存在。
- 员工执行槽位被任务 run 占用时,沿用现有单活跃 run 约束报"员工忙",不排队不静默。

### 6.4 Runtime / Provider

**零改动。** `ProviderRequest` 不加字段,`claude.rs` / `codex.rs` / `opencode.rs` spawn 参数原样,三 Provider 天然全支持 chat。

## 7. Chat:转为任务

首版零新后端,"转"发生在提交需求之前,靠人类之手完成:

1. 回答渲染完成后,前端持有 run_id、员工、提问、回答文本。
2. 点"转为任务" → 任务中枢切回任务模式,需求框预填草稿(目标占位 + 结论摘录 + 来源行"源自与 @员工 的单次对话"),标题沿用 `deriveTitle` 取首行。纯前端状态传递,无 API 调用。
3. 用户编辑、选项目、提交 → 现有 `POST /projects/{id}/demands`;`source_refs`(现成自由结构字段)填 `{ chat_run_id, digital_employee_id }` 留审计血缘;`source_type` 首版仍用 `manual`(契约枚举加 `chat` 值为可选优化)。
4. 之后与 chat 再无关系:demand → `DemandSubmitted` signal → 协调 → planner → 人类审批 → 派发,全部既有链路。

**关键语义:这不是"提升产出",是"人类背书的需求草稿"。** 进入正式链路的是用户编辑确认的文本,责任主体是提交的人;transcript 本身留在隔离区,`source_refs` 只提供溯源不赋予证据效力。回答超长时预填做摘录截断(需求框 5000 字上限);run 未终态或失败时按钮禁用。

## 8. Plan / Loop:模式落地

### 8.1 存储:随需求走,冻结进计划

- `SubmitProjectDemandInput` / demand 契约新增 `coordination_mode: plan | loop`,新需求缺省 `plan`,服务端校验。
- `project_demands` 落库;planner 出计划时复制冻结到 `project_plan_revisions`;workflow 运行期只读已批准 plan revision 上的值。
- 效果:人类批准计划时看到并背书的内容包含模式本身;改模式 = 新 plan revision = 重新审批,不存在运行中偷换档位。

### 8.2 行为分支(唯一 workflow 改动点)

位置:`handleEmployeeTaskCompleted` 的 `blocked_resolvable_upstream` 分支(`workflow.go:536` 附近),包在 `workflow.GetVersion("coordination-mode-branch", ...)` 内:

- **loop**:现行为原样——`CreateUpstreamSupplementTasks` 自动追加 owner 任务 + 下游重跑;`max_plan_iterations` 兜底,耗尽转人类(`requestProjectTaskIterationExhaustedReview`)。
- **plan**:不自动追加。将补链**提案**(缺失输入清单 `Blocker.MissingInputs`、拟追加的 owner 任务集、拟重跑的下游集)包成项目决策请求送人类,复用 `pendingTaskFailureRecovery` 等待机制与收件箱呈现。批准 → 执行同一个 `CreateUpstreamSupplementTasks` + 派发;驳回 → 任务停在阻塞态,落审计事件,不自动重试。

同一套补链机制,差别只是谁按下扳机。

### 8.3 缺省与存量兼容

- 新提交需求缺省 `plan`。理由:与"人类决策一等对象"一致;plan 缺省误报最坏多问一次,loop 缺省误判最坏烧预算跑歪图。**这是对新需求的行为变更,已确认接受。**
- 存量 plan revision 无 mode 值 → 按 `loop` 解释,存量在跑的计划保持现行为;叠加 GetVersion,存量 workflow 不受影响。

## 9. UI:任务中枢

- 头部三态切换 `[Plan 任务 | Loop 任务 | 对话]`,缺省 Plan。切换器下方一行说明文案点出差异(遇阻塞:报你决策 / 自动补做上游 / 与员工直接对话)。
- Plan/Loop 共用现有任务表单,仅提交的 `coordination_mode` 值不同。
- 对话态同一张 GlassCard 内容置换:参数区三 chip 换成一个必选"员工"chip(现有员工列表 API,名字+角色);placeholder 换"向这位员工提问……";保留模板库;按钮"发送"。
- 发送 → `createDigitalEmployeeRun({ objective, run_kind: "chat" })`;输入框下方对话流区域,问题即时上屏,轮询渲染回答(Markdown),终态停止轮询。
- 追问:输入框保留,再发自动带 `resume_of_run_id`;问答对堆叠成线程视图。**线程视图是页面本地状态**,刷新即散;runs 持久可查。换员工或刷新即开新线程。
- 每条完成回答旁:[转为任务] [复制]。
- 员工详情页 run 历史加 `run_kind` 徽章与筛选;点开走现有 run-detail-drawer 看 transcript——这就是对话的持久呈现,不另做页面。
- 既有装饰性缺陷处理:优先级/风险级别 chip 本次从表单移除。
- 实现期:先读 `DESIGN.md`;内部跳转用 TanStack Router;测试 `corepack pnpm --filter @superteam/web test`。

## 10. 错误处理

### 10.1 对话侧

| 场景 | 行为 |
|---|---|
| Runtime 离线 / 员工无可用 Provider 执行实例 | 创建 run 即报错,前端呈现原因并禁用发送 |
| run 失败/超时 | 回答区错误卡片 + 重试(全新 run,不带 resume) |
| `resume_of_run_id` 非法(§6.3 校验矩阵) | 服务端 400;前端降级为新会话并明示"上下文未延续" |
| resume 时 provider 侧 session 失效 | run 失败呈现 + 引导重新开问 |
| 员工执行槽位被占 | 报"员工忙",不排队不静默 |
| 转为任务回答超长 | 摘录截断 + 提示补编辑 |

### 10.2 plan/loop 侧

- 模式值非法 → demand 提交 400(服务端校验)。
- plan 模式决策请求创建失败 → workflow 既有错误路径(审计事件 + 循环存活),不吞。
- 人类驳回补链提案 → 任务停留阻塞态,落审计事件。
- 存量 plan revision 无 mode → 按 loop 解释。

## 11. 已知取舍与风险

1. **chat run 以员工完整常备权限执行**(与任务 run 同为全速档,无只读沙箱)。依据:任务 run 的权限门本就在计划审批层而非执行层;chat 是人类本人直接下的指令,打字者即授权人,与"直接指令的审批就是下指令这个动作"同构。兜底是产出隔离(不变量 2/3)+ 全量审计。曾评审过四层只读强制方案(plan mode / read-only sandbox / 能力凭据不注入 / 无工作区),因过度设计被否,记录在案备将来重估。
2. **轮询而非推送**:单次问答场景 1–2s 延迟可接受,省实时通道基建。
3. **线程不持久**:刷新丢线程视图是"单次对话"定位的刻意选择,run 级留痕仍完整。
4. **`contracts/runtime` 无自动契约验证**(已知债):本 spec Runtime 零改动,不触发该债,但 demand/runs 契约改动仍须走 `generate:control-plane` + `verify:contracts`。
5. **Temporal 版本化**:workflow 分支必须走 `GetVersion`,实现时以回放测试证明存量兼容。

## 12. 测试与验证

1. **契约层**:`generate:control-plane` + `verify:contracts`。
2. **控制平面 Go**:resume 校验矩阵逐项拒绝;首问强制新会话;`run_kind` 落库与过滤;demand mode 冻结进 plan revision;workflow blocked 分支两模式 + GetVersion 兼容 + 提案批准/驳回两条路(用 `workflow_test.go` 既有设施)。
3. **Web**:三态切换;chat 提交与轮询渲染;追问携带 resume;转为任务预填与 `source_refs`;plan/loop 提交值;优先级/风险移除后无残留引用。
4. **DB**:`atlas.sum` + `migrate-validate` + `verify:db`。
5. **真实端到端(完成条件,不可用单测/构建替代)**:
   - chat:真实 Web→控制平面→Runtime→Claude Provider 首问 + 追问,确认追问延续上下文(答案引用前文事实)。
   - 转为任务:对话结论预填提交 → demand → planner → 收件箱审批,全链真实。
   - plan 模式:构造真实上游阻塞,确认收件箱出现补链提案决策,批准后补链任务真实派发执行。
   - loop 模式:同场景自动补链(复用既有 E2E 先例)。
   - 环境不满足任一条件时按仓库规矩标记阻塞,不降级交付。

## 13. 关联

- `2026-07-10-project-plan-phase-refactor-design.md`:plan/loop 行为定义(§4.7)、chat 隔离原则(§6)、补链机制与上限(§4.8)。
- `2026-06-30-autonomous-outer-loop-iteration-attestation-budget-design.md`:预算熔断细则(loop 模式延展前核算的调用点归属)。
- `2026-06-30-intent-acceptance-criteria-design.md`:验收判据语义(与本 spec 的模式无耦合,判据裁决两模式下都归人类)。
