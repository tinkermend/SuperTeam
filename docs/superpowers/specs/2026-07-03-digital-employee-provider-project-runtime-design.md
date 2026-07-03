# 数字员工 Provider 固定与项目 Runtime 调度设计

日期：2026-07-03
状态：已确认，待实现计划

## 1. 背景

当前项目已经把 Project 建模为业务闭环容器，并且任务发起入口也是基于 Project 提交 Demand。ProjectTask 派发链路会在 metadata 和 execution packet 中携带 `project_id`、`project_task_id`、`project_task_attempt_id`、`digital_employee_id`、`runtime_node_id` 等事实。

但数字员工创建和运行模型仍有一个关键分层问题：

- `digital_employees` 身份表本身没有 Runtime 字段。
- `POST /api/v1/digital-employees` 创建时要求 `runtime_node_id` 和 `provider_type`。
- 创建服务会立刻预检 Runtime 在线状态，并创建唯一活跃 `digital_employee_execution_instances`。
- `digital_employee_execution_instances` 同时持有 `digital_employee_id`、`runtime_node_id`、`provider_type`、`agent_home_dir`。
- 项目任务派发通过数字员工 run preflight 读取这个 execution instance，再确定真实下发的 Runtime。

结果是：数字员工身份虽然没有直接绑定 Runtime Agent，但产品和服务语义上已经形成“数字员工必须先绑定某个 Runtime 才能存在或可用”。这与目标模型冲突。

目标模型应区分三类事实：

- 数字员工身份事实：角色、权限、上下文、输出契约、固定 Provider。
- 项目调度事实：项目成员池、项目资源、项目 Runtime placement、ProjectTask 和 attempt。
- Runtime 执行事实：某次任务 attempt 落在哪个 Runtime、使用哪个 Provider binary、工作区和写回状态。

## 2. 目标

- 数字员工不绑定任何一个 Runtime Agent。
- 数字员工创建时必须确定且只能确定一个 Provider：`claude-code`、`opencode`、`codex`。
- Runtime Agent 可以承载多个 Project。
- 一个 Project 可以绑定多个数字员工作为项目数字员工池。
- 任务发起以 Project 为基本维度，Demand 必须属于 Project。
- 数字员工执行的每个 ProjectTask attempt 都必须明确记录执行所属 Project。
- ProjectTask dispatch 时才选择 Runtime，并把选择结果记录在 attempt、runtime command、execution ledger 或 attestation 中。
- execution instance 不再作为数字员工身份绑定 Runtime 的事实源；若保留，应收窄为运行态缓存、工作区投影或兼容读模型。

## 3. 非目标

- 不引入新的主栈、消息队列或独立调度框架。
- 不把 Control Plane 改成本地命令执行器。
- 不让 Runtime Agent 负责业务策略、项目成员选择、人类审批或长期平台状态。
- 不删除 `tasks`、`task_runs`、`project_tasks` 或 `project_task_attempts`。
- 不在本阶段实现多 Provider fallback 或每个任务临时改 Provider。
- 不把数字员工重新归入团队专属模型；团队和项目仍是不同边界。
- 不把普通非项目的手动数字员工 run 声明为完整项目任务执行链路；这类入口需要独立决策是否保留。

## 4. 当前问题判断

### 4.1 已满足的部分

Project 已经是任务发起入口：

- Web 任务发起页先选择 Project，再调用 `submitProjectDemand(projectId, input)`。
- 后端 `SubmitDemand` 必须带 `ProjectID`，并 signal `project-coordinator:{project_id}`。
- `project_tasks.project_id` 是项目任务必填事实。
- ProjectTask dispatch metadata 和 execution packet 已携带 `project_id`。

Project 也已经支持多数字员工池：

- `project_members` 用 `project_id + principal_type + principal_id + project_role` 表达项目成员。
- `principal_type = digital_employee` 可以表示项目数字员工池成员。

Runtime 多项目承载也有 schema 基础：

- `project_placements` 约束一个 Project 只有一个 active placement。
- 该表没有限制一个 Runtime 只能绑定一个 Project，因此一个 Runtime 可承载多个 Project。

### 4.2 不满足的部分

数字员工创建仍强制 Runtime：

- 创建请求包含 `RuntimeNodeID` 和 `ProviderType`。
- `normalizeCreateDigitalEmployeeRequest` 要求 `runtime_node_id` 非空。
- `CreateDigitalEmployee` 会执行 Runtime provisioning preflight 并要求 command channel connected。
- 创建事务会写入 `digital_employee_execution_instances`，其中 `runtime_node_id` 必填。

Provider 固定位置不正确：

- 目标是 Provider 属于数字员工身份事实。
- 当前 Provider 存在于 execution instance 上，可以通过 `BindExecutionInstance` 后续替换。
- 这导致“改 Runtime 绑定”也可能顺手改 Provider，破坏“创建时固定一个 Provider”的业务约束。

ProjectTask attempt 已有项目上下文，但普通数字员工 run 没有硬项目字段：

- ProjectTask 链路通过 metadata 和 packet 携带项目事实。
- `task_runs` 和 `DigitalEmployeeRun` 本身没有 `project_id` 字段。
- 如果保留普通手动 run，它不应被表述为“项目任务执行”；如果要成为项目任务执行，就必须通过 ProjectTask/ProjectDemand 入口创建。

## 5. 方案选择

### 5.1 推荐方案：数字员工固定 Provider，Runtime 在 ProjectTask attempt 选择

把 `provider_type` 提升为数字员工身份或当前有效配置事实，创建数字员工时必须选择一个 Provider。创建时不再要求选择 Runtime，也不做 Runtime provisioning。

ProjectTask dispatch 时，Control Plane 根据以下事实选择 Runtime：

- Project 的 active placement 或候选 Runtime 集。
- 数字员工固定 Provider。
- Runtime capability 是否支持该 Provider。
- Runtime 在线、command channel connected、容量和项目策略。
- 项目 repo/resource 绑定和工作区要求。

选择结果写入 `project_task_attempts.runtime_node_id`、`task_runs.runtime_node_id`、runtime command payload、execution ledger 和 attestation。Runtime Agent 只执行下发的 attempt，不反向决定项目策略。

优点：

- 符合数字员工不绑定 Runtime 的核心业务逻辑。
- Provider 决策稳定，避免同一数字员工跨任务人格或工具栈漂移。
- Runtime 离线不会污染数字员工身份，只影响调度可用性。
- ProjectTask attempt 成为“某个项目、某个员工、某个 Runtime、某次执行”的完整事实。

缺点：

- 需要迁移现有创建流程和 execution instance 语义。
- 需要补 Runtime placement/capability 的派发选择逻辑。
- 员工详情、技能安装、运行按钮等 UI 需要从“绑定 Runtime”改成“Provider 固定 + 当前可调度状态”。

### 5.2 备选方案：保留 execution instance，但允许 Runtime 为空

继续保留 `digital_employee_execution_instances`，但允许 `runtime_node_id` 为空。Provider 仍迁到数字员工身份层，execution instance 只作为可选缓存记录。

优点：

- 对现有表和 API 改动较小。
- 迁移路径平滑，旧读模型可以逐步兼容。

缺点：

- 概念容易继续混淆：execution instance 仍看起来像员工运行归属。
- 需要长期维护大量 nullable 和兼容逻辑。
- 容易在后续功能里再次把 Runtime 误绑定回员工。

### 5.3 不推荐：维持当前创建期 Runtime 绑定，只补文档

保留创建数字员工时选择 Runtime Provider 的行为，只在文档中说明这是“默认执行实例”。

不推荐原因：

- 直接违背“数字员工不绑定任何 Runtime Agent”。
- 旧 binding 失效时仍会让员工级功能报 `runtime_not_connected`。
- Project 维度调度无法真正落地，Runtime 仍被员工实例间接决定。
- Provider 固定和 Runtime 可调度资源的边界继续混在一起。

## 6. 目标架构

```text
DigitalEmployee
  - id
  - provider_type
  - role / policies / output contract
  - no runtime_node_id

Project
  - id
  - human owner
  - members: human_user / digital_employee / team
  - optional repo/resource binding
  - active runtime placement or candidate runtime policy

ProjectDemand
  - project_id
  - submitted_by_user_id
  - title / content

ProjectTask
  - project_id
  - demand_id
  - assigned_digital_employee_id
  - expected outputs / handoff contract

ProjectTaskAttempt
  - project_task_id
  - project_id through task
  - digital_employee_id
  - provider_type
  - runtime_node_id
  - lease / status / terminal facts
```

调度链路：

```text
ProjectDemand submitted
  -> project coordinator plans ProjectTasks
  -> selected digital employee must be active project member
  -> dispatch computes runtime placement for (project, employee.provider_type)
  -> creates ProjectTaskAttempt
  -> creates DigitalEmployeeRun and Runtime command
  -> Runtime Agent executes Provider
  -> attempt started/complete/fail/wait-human writeback
  -> ProjectTask and ProjectDemand status advance
```

## 7. 数据模型设计

### 7.1 数字员工身份层

新增或迁移字段：

- `digital_employees.provider_type VARCHAR(100) NOT NULL`

约束：

- `provider_type IN ('claude-code', 'opencode', 'codex')`，实际枚举可通过服务端注册表校验表达，数据库 check 只在团队确认稳定命名后加入。
- 创建后不可通过普通 update 修改。
- 如未来必须迁移 Provider，应走显式“重建数字员工”或“Provider 迁移审批”流程，不是 execution binding update。

兼容：

- 现有员工的 Provider 可从当前 active `digital_employee_execution_instances.provider_type` 回填。
- 存在多个历史 execution instance 时，取未删除 active/ready 记录；冲突记录进入迁移报告，由人工确认。

### 7.2 execution instance 语义收窄

推荐逐步废弃员工级必选 execution instance。

短期兼容可保留表，但语义改为：

- 运行缓存或历史读模型，不是数字员工身份。
- `runtime_node_id` 可以为空，或只保留历史记录。
- 不再由创建数字员工 API 必写。
- 不再作为 ProjectTask dispatch 选择 Runtime 的权威输入。
- `BindExecutionInstance` 改为兼容/运维接口，不能修改员工固定 Provider。

如果保留 `agent_home_dir`：

- 它表示员工能力缓存或历史路径，不代表 Provider auth home。
- ProjectTask 的工作目录由项目和 attempt 派生，不由员工 execution instance 决定。

### 7.3 Project Runtime placement

`project_placements` 保留项目到 Runtime 的动态亲和状态：

- 一个 Project 默认一个 active placement。
- 一个 Runtime 可以有多个 active project placements。
- Project placement 是调度输入，不是员工绑定。

后续可扩展为多 Runtime 候选：

- 单 active placement：简单、适合本地 repo/worktree。
- 多 candidate placement：适合 HA 或容量溢出，但必须依赖中央 git remote 和可重建能力缓存。

### 7.4 ProjectTaskAttempt 执行事实

`project_task_attempts` 应持有或可稳定关联以下事实：

- `project_task_id`
- `project_id`，可冗余存储以便索引和审计，也可通过 task 强一致关联。
- `digital_employee_id`
- `provider_type`
- `runtime_node_id`
- `lease_token`
- `execution_context_packet`

当前表已有 `project_task_id`、`runtime_node_id`、lease 和 packet，但缺少显式 `digital_employee_id`、`provider_type`。可以通过 `project_tasks.assigned_digital_employee_id` 和 run 关联推导，但不够直接。实现阶段应优先让 attempt 成为完整审计事实。

## 8. API 与服务设计

### 8.1 创建数字员工

`POST /api/v1/digital-employees` 目标请求：

```json
{
  "team_id": "optional",
  "employee_type": "requirements_analyst",
  "name": "需求分析员",
  "provider_type": "codex",
  "role": "...",
  "capability_selection": {},
  "context_policy_override": {},
  "approval_policy_override": {},
  "output_contract_addendum": {}
}
```

移除创建必填：

- `runtime_node_id`
- `runtime_binding`
- `agent_home_dir`

服务行为：

- 校验 Provider 被当前租户/团队策略允许。
- 校验数字员工配置和有效配置。
- 写入数字员工、配置版本、effective config、环境变量。
- 不要求 Runtime 在线。
- 不下发 `provision_instance` Runtime command。

创建结果：

- 员工状态可以为 `ready`，表示身份与配置可用。
- 调度可用性由派发时 Runtime placement/capability 决定，不写成员工状态。

### 8.2 create-options

`GET /api/v1/digital-employees/create-options` 应返回：

- 可选 Provider 列表。
- Provider 是否被团队策略允许。
- 可用 Runtime Provider 能力摘要仅作为“当前可调度预览”，不作为创建必选项。

前端创建向导：

- 运行步骤改为 Provider 选择步骤。
- 不再让用户选择 Runtime。
- 如果某 Provider 当前没有任何在线 Runtime，可显示风险提示，但不阻塞创建，除非团队策略要求创建时必须可调度。

### 8.3 ProjectTask dispatch

派发服务输入：

- `ProjectID`
- `ProjectTaskID`
- `AssignedDigitalEmployeeID`

派发服务内部读取：

- Project active placement 或候选 Runtime。
- 数字员工固定 Provider。
- Runtime capability 和 command channel connected。
- 项目 repo/resource 绑定。

派发失败分类：

- `no_project_runtime_placement`
- `runtime_offline`
- `provider_unavailable`
- `employee_not_project_member`
- `employee_provider_not_allowed`
- `capacity_unavailable`

失败处理：

- 可恢复条件进入 retry 或 waiting human。
- 策略/成员错误进入 waiting human 或 failed，按现有 pre-dispatch gate 和 recovery 逻辑处理。

### 8.4 普通数字员工 run

普通 `POST /api/v1/digital-employees/{id}/runs` 有两种选择，需要实现前确认：

推荐收敛选择：

- 普通 run 仅作为员工调试或工作台动作，不声明为 ProjectTask。
- 它仍可没有 `project_id`，但 UI 和文档不能把它叫作项目任务执行。

更严格选择：

- 所有业务执行都必须选择 Project。
- 员工详情页“开始任务”也必须先选 Project，并创建 ProjectDemand/ProjectTask。

本 spec 推荐第二个作为长期目标，但实现可先保留普通 run，避免一次性破坏工作台。

## 9. Runtime Agent 与 Provider 边界

Runtime Agent 接收 command payload 时应只信任 Control Plane 下发的执行事实：

- `project_id`
- `project_task_id`
- `project_task_attempt_id`
- `digital_employee_id`
- `provider_type`
- `runtime_node_id`
- `workspace_mode`
- `capability_manifest_version`

Runtime Agent 不做：

- 选择数字员工。
- 判断项目成员资格。
- 选择 Provider。
- 修改 ProjectTask 业务策略。
- 推断项目归属。

Provider adapter 只负责把固定 Provider 类型映射到本机执行器：

- `claude-code`
- `opencode`
- `codex`

如果 Runtime 缺少对应 Provider capability，Control Plane 不应派发到该 Runtime。

## 10. Web 设计影响

### 10.1 员工创建

创建页从“运行绑定”改成“Provider 选择”：

- 只展示 Provider 候选。
- 明确 Provider 创建后固定。
- Runtime 在线情况展示为调度预览，不作为必选绑定。
- 提交体不再含 `runtime_node_id`。

### 10.2 员工列表与详情

员工卡片展示：

- 固定 Provider。
- 当前可调度状态：由 Provider + Runtime capability + project placement 推导。
- 不再展示“绑定 Runtime”作为身份事实。

如果需要提示不可调度：

- 文案应指向“当前没有支持该 Provider 的在线 Runtime”或“项目未绑定 Runtime placement”，而不是“员工未绑定 Runtime”。

### 10.3 项目配置

项目数字员工池继续从 `project_members` 管理。

项目配置或项目运行页应能显示：

- 项目 active Runtime placement。
- 该 Project 下每个数字员工固定 Provider 是否被 placement 支持。
- 不支持时给出成员级调度风险提示。

## 11. 迁移策略

### Phase 1：Provider 提升到数字员工身份

- 增加 `digital_employees.provider_type`。
- 从 active execution instance 回填。
- 更新创建 API：仍兼容旧字段，但新字段以 `provider_type` 为准。
- 前端创建页不再要求 Runtime。
- 保留 execution instance 读模型，避免一次性破坏详情页和技能安装页。

### Phase 2：ProjectTask dispatch 使用 Project placement 选择 Runtime

- Dispatch 前读取 Project placement 和 Runtime capabilities。
- attempt 写入 `digital_employee_id`、`provider_type`、`runtime_node_id`。
- run preflight 不再依赖员工 execution instance 的 Runtime。
- 旧 execution instance Runtime 只作为历史兼容或 fallback，不作为默认选择。

### Phase 3：UI 与能力安装改语义

- 员工详情和技能安装从“绑定 Runtime”改为“按 Provider 投影到目标 Runtime/Project”。
- 若技能必须物理安装到 Runtime，应由目标 Project placement 或显式安装目标决定，而不是员工身份绑定决定。

### Phase 4：清理旧 execution binding

- `BindExecutionInstance` 标记 deprecated 或限制为运维迁移接口。
- 移除创建期 `provision_instance` 必选链路。
- 删除或归档不再使用的 `runtime_node_id` 必填约束。

## 12. 验收标准

- 创建数字员工时只需要选择 Provider，不需要选择 Runtime。
- 创建后的数字员工记录有且只有一个固定 Provider。
- 修改 execution instance 或 Runtime placement 不能改变数字员工 Provider。
- 一个 Runtime 可以同时作为多个 Project 的 active placement。
- 一个 Project 可以包含多个 `digital_employee` project members。
- 从任务发起页提交 Demand 时必须选择 Project。
- ProjectTask dispatch 时只允许选择项目数字员工池内 active 数字员工。
- 每个 ProjectTask attempt 都能审计出：Project、ProjectTask、DigitalEmployee、Provider、RuntimeNode。
- Runtime 离线时，数字员工身份仍存在且配置可编辑；只有相关 ProjectTask dispatch 被阻塞或等待。
- 旧的 `runtime_not_connected` 类错误不再以“员工绑定失效”为第一诊断，而应定位到 Project placement 或 Runtime capability/connection。

## 13. 测试计划

Control Plane：

- 创建数字员工缺少 `runtime_node_id` 仍成功，缺少 `provider_type` 失败。
- 创建后 `digital_employees.provider_type` 不可被普通配置更新改变。
- 从旧 execution instance 回填 Provider 的迁移测试。
- ProjectTask dispatch 在无 Project placement 时进入明确失败或 waiting human。
- ProjectTask dispatch 选择 Runtime 时要求该 Runtime 支持员工 Provider。
- attempt 写入或返回完整 Project + employee + provider + runtime 事实。

Web：

- 员工创建页不再要求 Runtime 候选。
- Provider 多选一后创建请求不含 `runtime_node_id`。
- 员工列表/详情不再把 Runtime 作为身份绑定展示。
- 项目配置页能看到成员 Provider 与项目 Runtime placement 的兼容风险。

Runtime Agent：

- command payload 中 Provider 与本机 capability 不匹配时拒绝执行并返回结构化错误。
- ProjectTaskAttempt writeback 仍带 lease token、attempt id、runtime node id。

真实链路验证：

- 启动 Web、Control Plane、Runtime Agent。
- 创建一个只固定 Provider、不选择 Runtime 的数字员工。
- 创建或配置一个 Project，绑定该数字员工到项目池。
- 为 Project 设置可用 Runtime placement。
- 从任务发起页提交 Demand。
- 让 ProjectTask dispatch 到 Runtime，确认 attempt/run/事件能审计出 Project、员工、Provider、Runtime。

## 14. 风险与待决策

1. 是否允许普通员工 run 无 Project：短期可保留为工作台调试，长期建议业务执行都项目化。
2. Provider 命名是否统一为 `claude-code`，还是继续兼容 `claude_code`。实现前需要统一 contract 和 UI label。
3. `digital_employee_execution_instances` 是废弃、保留兼容读模型，还是改造成能力缓存 projection 表。
4. 技能安装目标从员工绑定 Runtime 改为 Project placement 后，是否需要支持“预安装到所有候选 Runtime”。
5. 多 Runtime project placement 是否进入本轮，还是先保持一个 Project 一个 active Runtime。

## 15. 后续计划边界

本设计确认后，下一步应进入实现计划编写。实现计划需要按 Phase 1 到 Phase 4 拆分，不能在第一阶段同时重写 Runtime Agent、技能安装、项目工作区和员工创建全部链路。

第一阶段最小闭环应只证明：

- Provider 成为数字员工身份事实。
- 创建数字员工不再需要 Runtime。
- 旧数据可迁移。
- 现有 ProjectTask dispatch 尚未完全切换前，不误称目标架构已全部完成。
