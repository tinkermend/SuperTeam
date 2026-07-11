# Plan 1–8 手动验证方案（Console 页面）

面向人类在 Web Console 上手动验证计划期重构 Plan 1–8。  
**没有 Plan 7**（文档跳号 6→8）。

- 环境：仓库根目录 `./scripts/dev-services.sh start all`（Control Plane `:8081`、Web `:3000`、Runtime Agent、Temporal）
- 账号：`admin` / `admin`
- 入口：http://127.0.0.1:3000
- 建议准备：至少 2 名 `claude-code` 数字员工（可复用「压测员小李」「报告员小王」），项目绑定 `local-dev-node`

---

## 0. 通用操作路径（后面各 Plan 都复用）

### 0.1 新建可调度项目

1. 打开 **项目管理** → **新建项目**
2. 填写名称 / 目标；负责人选自己
3. **绑定 Runtime 节点**（如 `runtime-local-dev-node`），确认「命令通道已连接」
4. 把 2 名数字员工加入项目成员（执行者）
5. 创建成功后进入项目详情

### 0.2 提交需求并批准计划

1. 项目详情 → **提交需求**（或「当前需求」相关入口）
2. 需求文案要明确「几步、谁产出什么、谁依赖什么」（各 Plan 用例里给了推荐文案）
3. 等待系统出计划；若出现计划评审 / 审批：
   - 打开 **审批** Tab，或侧栏 **收件箱 / 审批中心**
   - 批准计划（低风险任务一般可直接批）
4. 批准后看 **概览 / 任务** Tab：应出现任务节点与状态变化

### 0.3 观察任务执行

1. **任务** Tab 或概览里的「当前执行」图
2. 状态常见：`planned` → `queued/running` → `completed` / `blocked` / `waiting_human`
3. 需要人类处理时：收件箱会出现决策；在项目 **审批** Tab 也能处理

### 0.4 辅助核对（页面不够时）

部分 Plan 的核心是「后台不再误拦 / 会话复用」，页面只能看到结果。可用：

- 浏览器 DevTools → Network：看 `/api/v1/projects/...` 响应
- 或问开发用 DB 查（文末附录）

---

## 总览：每个 Plan 在页面上能验什么

| Plan | 页面上主要验什么 | 页面够不够 |
|---|---|---|
| 1 拆假闸门 | 空能力员工仍能派发执行，不被「缺能力」卡死 | 够（看任务能否 running） |
| 2 能力控制流退役 | 计划不因虚构能力强制审批；低风险可直接跑 | 够 |
| 2b 权限/runtime 词表 | 虚构 permission 不强制审批；真 provider 不匹配仍应拦 | 够 |
| 3 produces/required_inputs | 两步依赖计划能批准并按依赖阻塞下游 | 够（看依赖与 blocked） |
| 4 计划级验收判据 | 计划确认区有「调度顺序」「验收判据」 | **强 UI** |
| 5 上游补做 | 下游缺输入后自动出现上游补做任务 | 够（看新任务/血缘） |
| 6 会话血缘 | 补做续同一会话（页面几乎看不到 session id） | **弱 UI，需 DB/日志** |
| 8 删 tool 死闸门 | 派发不再出现 tool.* 检查 | **弱 UI，需 DB/Network** |

建议顺序：**先做「一条黄金路径」覆盖 3+4+1/2**，再单独做 **5（+6）**，最后用附录核对 **8**。

---

## 黄金路径 A：两步依赖计划（覆盖 Plan 3、4，并顺带看 1/2）

**目的：** 一次走通「有 produces/required_inputs + 验收判据展示 + 能派发」。

### 准备

- 项目内 2 名员工：A=压测/执行，B=报告/评估
- 需求文案示例：

```text
请拆成恰好两个任务：
1）由「压测员小李」执行一次简短负载测试，产出 load_test_report（produces 必须含 load_test_report）；
2）由「报告员小王」基于该报告写性能评估，必须依赖任务1，required_inputs 必须含 load_test_report。
请给出 plan_acceptance_criteria，每条 satisfied_by 指向真实任务 key。
不要发明其它 artifact key。
```

### 步骤与期望

| # | 你在页面上做什么 | 期望看到 |
|---|---|---|
| A1 | 新建项目、绑 Runtime、加两名员工 | 运行落点「已就绪」、命令通道已连接 |
| A2 | 提交上述需求 | 出现计划评审 / 或直接进入执行（视风险） |
| A3 | 打开计划确认区域（项目详情概览里「计划确认」） | 能看到 **「调度顺序」** 与 **「验收判据」**（Plan 4） |
| A4 | 展开某条验收判据 | 能看到判据说明，以及它归属哪些任务（satisfied_by） |
| A5 | 批准计划（若需要） | 任务图出现 2 个节点；上游可执行，下游因依赖常为 **已阻塞** |
| A6 | 等上游进入 **运行中** | 证明派发闸门未因「虚构能力缺失」卡死（Plan 1/2） |
| A7 | （可选）等上游完成 | 下游应从阻塞变为可调度/运行 |

### 判定

- **Pass：** 有调度顺序 + 验收判据；两任务依赖关系正确；上游能 running。
- **Fail：** 计划区没有「验收判据」标题；或空能力/虚构能力导致任务永远无法派发且强制无意义审批。

---

## Plan 1：拆除假闸门（capability.match）

**页面可观察结论：** 「员工 external_capabilities 为空 / 计划里 missing_capabilities 非空」时，任务仍能越过派发进入执行。

### 用例 P1

1. 使用 **external_capabilities 为空** 的 claude-code 员工（或临时建一个空白能力员工）加入项目
2. 提交简单单任务需求，例如：「写一份 100 字主机健康摘要」
3. 批准计划后观察任务

**期望**

- 任务能进入 `queued/running`，**不会**因为「缺某某能力」长期停在无法派发
- 若进入 `waiting_human`，原因应是验收/风险等真实原因，而不是能力词表闸门

**如何确认不是假闸门**

- 任务详情 / 执行轨迹里不应把「能力不匹配」当作派发硬失败主因
- 开发辅助：派发闸门结果里不应再有 `capability.match` / `capability.hard_missing`（附录）

---

## Plan 2：能力控制流退役 + selection_confidence

**页面可观察结论：** 虚构能力名不再强制「必须人工批才能跑」；真正低置信选人仍会回到人类。

### 用例 P2-a（正向：虚构能力不挡路）

1. 同黄金路径 A，或任意会让 planner 写出 `missing_capabilities` 的需求
2. 若风险为低：观察是否**不必**仅为「缺能力」而强制审批
3. 任务仍应能派发

**期望：** 低风险计划可执行；审批若出现，应是风险/策略等真实原因。

### 用例 P2-b（负向：真的没人可用）

1. 项目里只放 **明显不匹配** 的员工（例如需求要 claude-code，项目里只有不可用员工），或把选人阈值调高（若 UI 暴露 `coordination_policy`）
2. 提交需求

**期望：** 可能出现「无合适员工 / 需人类处理」类决策，而不是硬派一个明显错误的执行者后靠能力闸门拦。

---

## Plan 2b：permission / runtime 词表收敛

**页面可观察结论：**

- 虚构 `permission_requirements` 不再单独逼出强制审批
- **真实** provider 不匹配仍应拦（例如把只要 claude 的任务派给错误 provider 员工）

### 用例 P2b-a

1. 正常两员工项目，提交普通写作/分析需求
2. 即使计划元数据里出现奇怪 permission 文案，低风险任务仍应能跑

### 用例 P2b-b（真事实硬失败）

1. 项目只放 **codex** 员工，需求明确要求 Claude Code 专属能力（或绑定只支持另一 provider 的节点）
2. 观察是否无法顺利派发 / 出现真实 runtime/provider 相关阻塞

**期望：** 拦的是「节点/Provider 事实」，不是散文 permission。

---

## Plan 3：produces / required_inputs

**页面可观察结论：** 上游产出、下游依赖在任务图上成立；下游在上游完成前保持阻塞。

### 用例 P3（可用黄金路径 A）

1. 用黄金路径需求
2. 批准后看任务图：

**期望**

- 恰好（或主要为）两步
- 下游状态为 **已阻塞**，且依赖指向上游
- 上游完成后下游才推进

**补充观察（Network）**

- 打开任务列表 API 或任务详情：上游 `planner_metadata.produces` 含 `load_test_report…`
- 下游 `input_requirements.required_inputs` 引用同一 key
- `input_requirements` 里不应再塞 `repository` / `demand_content` 等自由字段（应在 planner_notes）

---

## Plan 4：计划级验收判据（重点 UI）

**页面可观察结论：** 计划确认面板两块内容。

### 用例 P4

1. 提交会产出多步计划的需求（黄金路径 A 文案）
2. 在计划生成后、或批准前后，打开项目详情 **概览** → **计划确认**

**必须看到**

1. **调度顺序**：任务 key / 标题按依赖排列
2. **验收判据**：至少一条；可展开；能看到归属任务

**期望**

- 判据的「由谁满足」指向真实存在的任务，而不是空/未知 key
- 若模型没给出判据：可能显示「本计划未声明验收判据」——可再提交一轮更强调判据的需求重试

**Fail：** 完全没有「调度顺序」「验收判据」区块（说明前端未加载 Plan 4）。

---

## Plan 5：上游补做与图延展

**页面可观察结论：** 下游申报缺输入后，自动多出一个**上游员工**的补做任务，而不是只让下游空转或干等人类。

### 准备

- 项目 `coordination_policy.max_plan_iterations` 建议设为 `2`（若创建项目 UI 可填；否则用已支持该策略的项目）
- 需求仍用两步 produces/required_inputs

### 用例 P5-a（补做出现）

1. 走完计划批准，让**上游先完成**（真实跑完，或等 Claude 完成）
2. 等下游开始执行
3. 制造「缺上游产出」——手动测试时通常有两种做法：
   - **推荐（真实）：** 让上游故意产出不合格/空报告，使下游自然 blocked
   - **协助（开发）：** 对下游 attempt 注入 `blocked + missing_inputs=[load_test_report…]`（需 API；见附录）
4. 刷新项目 **任务** Tab

**期望**

- 新增补做任务，负责人是**上游员工**（不是下游）
- 任务标题/摘要带补做语义；图上多一个节点
- 原下游仍在，等待补做完成后再继续

### 用例 P5-b（迭代熔断转人类）

1. 同一缺失输入反复触发补做，直到超过 `max_plan_iterations`
2. 观察收件箱 / 审批

**期望**

- 出现类似 **迭代耗尽 / 需人类决策** 的事项（`project_task_iteration_exhausted` 一类）
- 平台不再无限自动补做

---

## Plan 6：会话血缘（页面弱，建议配合附录）

**页面可观察结论（间接）：**

- 同一上游血缘的补做任务，执行体验上像「接着上次做」（少重复自我介绍）——主观
- **客观验证必须看 session id**（附录 SQL）

### 用例 P6（接在 P5-a 后）

1. 完成一次上游任务（产生 provider session）
2. 触发上游补做（P5）
3. 用附录 SQL 比对：
   - 原上游任务的 `provider_sessions.project_task_root_id`
   - 补做任务派发 metadata 里的 `provider_session_id`
4. **期望：** 二者相同（续接）；同员工的**另一无关任务**应是不同 session

**页面-only Pass 标准（弱）：** 补做确实派给原上游员工且能 running。  
**严格 Pass：** session id 一致（附录）。

---

## Plan 8：删除 tool 死闸门

**页面可观察结论（间接）：** 任务照常派发。  
**严格验证：** 派发闸门 checks 里**没有** `tool.binding` / `tool.authorization` / `tool.available`。

### 用例 P8

1. 任意能派发的任务（黄金路径即可）
2. 任务进入 running 后，用附录查最新 `project_task_dispatch_gate_results.checks`

**期望**

- `status=passed`（或因真实原因 blocked，但原因不是 tool.*）
- checks 的 Key 列表不含任何 `tool.*`

---

## 推荐测试日程（约 60–90 分钟）

| 时段 | 做什么 | 覆盖 |
|---|---|---|
| 0–10 min | 确认服务、登录、Runtime 在线 | 环境 |
| 10–35 min | 黄金路径 A（含计划区截图） | Plan 3、4、1/2 |
| 35–55 min | 等上游完成 → 触发下游缺输入 → 看补做 | Plan 5 |
| 55–70 min | （可选）查 session / 闸门 SQL | Plan 6、8 |
| 70–90 min | P2b 真 provider 不匹配负向用例 | Plan 2b |

每条用例建议记录：项目 URL、截图（计划确认、任务图、补做前后）、Pass/Fail、异常现象。

---

## 附录：开发协助核对（非必须，但 Plan 6/8 建议）

把 `PROJECT_ID` / `TASK_ID` 换成页面上的 UUID。

### A. 派发闸门是否还有 tool.* / capability.match（Plan 1、8）

```sql
select status, checks
from superteam.project_task_dispatch_gate_results
where project_id = 'PROJECT_ID'
order by created_at desc
limit 3;
```

期望：`checks` 的 Key 无 `tool.*`，也无 `capability.match`。

### B. produces / required_inputs（Plan 3）

```sql
select planned_task_key, status,
       planner_metadata->'produces' as produces,
       input_requirements->'required_inputs' as required_inputs
from superteam.project_tasks
where project_id = 'PROJECT_ID'
order by created_at;
```

### C. 上游补做（Plan 5）

```sql
select id, planned_task_key, status,
       assigned_digital_employee_id, revision_of_task_id, plan_iteration
from superteam.project_tasks
where project_id = 'PROJECT_ID'
order by created_at;
```

期望：存在 `revision_of_task_id` 指向上游、`plan_iteration >= 1` 的补做行。

### D. 会话续接（Plan 6）

```sql
-- 上游根任务的会话
select provider_session_id, status, project_task_root_id
from superteam.provider_sessions
where project_task_root_id = 'OWNER_TASK_ID'
order by last_active_at desc;

-- 补做派发是否注入同一 session
select params->'metadata'->>'provider_session_id' as session_id,
       params->'metadata'->>'revision_root_task_id' as root,
       params->'metadata'->>'project_task_id' as task_id
from superteam.tasks
where params->'metadata'->>'project_task_id' = 'SUPPLEMENT_TASK_ID'
order by created_at desc
limit 1;
```

期望：两处 `provider_session_id` 相同。

---

## 已知限制（避免误判）

1. **Planner 非确定性：** 同样文案可能拆成 1/2/3 步；若结构不对，改文案强调「恰好两个任务 + 指定 produces key」再试。
2. **Claude session limit / Provider 慢：** 上游长时间 running 不等于失败；可等或换时段。
3. **Plan 6/8 几乎无独立 UI 控件：** 页面只能看结果，严格验收靠附录。
4. **指纹熔断已删：** 不要再按「相同失败指纹两次就熔断」测；熔断看 `max_plan_iterations` 计数。
5. **高风险任务仍会要人类批：** 这是保留的真实审批，不是 Plan 2 回退。
