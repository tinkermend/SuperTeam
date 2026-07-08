# 数字员工删除功能设计

日期：2026-07-09
状态：已确认，待实现计划

## 1. 背景

数字员工已经成为项目任务调度、团队能力继承、Runtime Provider 投影、技能、MCP、环境变量和工作区上下文的核心业务身份。删除数字员工不能只从列表里移除一行，否则会留下几个风险：

- 如果员工正在排队或执行任务，删除会破坏运行链路和项目任务闭环。
- Runtime 节点上可能已经有 agent home、技能包、MCP 配置、环境变量投影或 workspace 文件，删除后需要留下可审计的清理线索。
- 团队管理、项目管理和项目调度候选不能继续展示已删除员工。
- 历史运行、项目任务、工件和审计不能被破坏，需要继续可追溯。

当前数据库已经有 `digital_employees.deleted_at`、`archived_at` 以及多处 `deleted_at IS NULL` 过滤，适合实现为软删除，而不是物理删除。

## 2. 目标

- 提供 Console 可调用的数字员工删除入口。
- 删除前强阻断仍在排队、调度、运行、取消中的工作。
- 删除成功后，数字员工不再出现在当前数字员工列表、团队数字员工列表、项目创建和项目调度候选中。
- 删除成功后，员工相关配置、执行实例、环境变量、MCP 绑定、技能绑定、workspace 和记忆类文件进入不可再投影的软删除或归档状态。
- 删除操作写入可审计事件，记录后台清理 Runtime 目录和配置残留所需的标识。
- 保留历史运行、项目任务、工件、执行账本和审计记录。

## 3. 非目标

- 不做后台物理清理任务。
- 不让 Control Plane 或 Console 直接删除 Runtime 节点上的本地目录。
- 不自动停止运行中的任务。
- 不自动改派项目任务。
- 不物理删除历史运行、项目任务、工件、执行账本和审计记录。
- 不重新设计项目任务生命周期或团队成员模型。

## 4. 产品规则

删除数字员工是危险操作。只有当员工没有仍在排队、调度、运行或取消中的工作时才允许删除。

删除成功后的产品语义：

- 员工不再作为当前可管理、可选择、可调度的数字员工出现。
- 团队页面和项目创建/调度候选依赖当前员工列表或 readiness 过滤，因此不会出现已删除员工。
- 历史项目任务和历史运行可以继续展示该员工的历史引用或快照；如果无法解析当前员工详情，界面退化显示“已删除数字员工”。
- Runtime 文件和目录不会立即物理删除，只在审计日志中留下清理线索，供后续后台清理任务使用。

## 5. 后端契约

新增接口：

```http
DELETE /api/v1/digital-employees/{employeeId}
```

鉴权：

- 新增权限动作 `employee.delete`。
- 只允许租户 owner/admin 级管理者执行。
- 资源类型为 `digital_employee`。

成功响应：

```http
204 No Content
```

未找到响应：

```http
404 Not Found
```

已经删除的员工再次删除返回 `404`，与当前 `GetDigitalEmployee` 和列表过滤 `deleted_at IS NULL` 的语义一致。

阻断响应：

```http
409 Conflict
```

响应体：

```json
{
  "code": "digital_employee_delete_blocked",
  "message": "该数字员工仍有排队或执行中的工作，停止或完成后再删除。",
  "blockers": [
    {
      "type": "run",
      "id": "run-id",
      "status": "running",
      "title": "运行标题",
      "run_id": "run-id",
      "project_id": "project-id"
    },
    {
      "type": "project_task",
      "id": "project-task-id",
      "status": "queued",
      "title": "项目任务标题",
      "project_id": "project-id"
    }
  ]
}
```

`blockers` 用于前端展示精确提示，字段可以随数据源补充，但必须包含 `type`、`id`、`status` 和可读标题或名称。

## 6. 删除阻断规则

删除前必须在事务内检查以下阻断项。只要存在任意命中，就返回 `409`，并且不修改任何员工或级联配置状态。

### 6.1 运行阻断

阻断状态：

```sql
task_runs.status IN ('queued', 'dispatching', 'running', 'cancelling')
```

匹配条件：

- `tenant_id = 当前租户`
- `digital_employee_id = employeeId`

### 6.2 项目任务阻断

阻断状态：

```sql
project_tasks.status IN ('queued', 'running', 'in_progress')
```

匹配条件：

- `tenant_id = 当前租户`
- `assigned_digital_employee_id = employeeId`

### 6.3 非阻断历史

以下历史不阻断删除：

- 已完成、失败、取消、超时的 `task_runs`
- 已完成、失败、取消、等待人工、阻塞、计划态但未进入 queue 的历史或待处理 `project_tasks`
- 历史审计、工件、事件和执行账本

这些记录继续保留，用于追溯。

## 7. 服务与事务设计

服务层新增 `DeleteDigitalEmployee(ctx, req)`。

事务顺序：

1. 按 `tenant_id + employee_id` 查询并锁定未删除员工。
2. 查询运行阻断项和项目任务阻断项。
3. 如果存在阻断项，返回 `ErrDigitalEmployeeDeleteBlocked`，携带 blocker 明细，事务不做写入。
4. 收集审计清理线索，包括员工名称、team_id、provider_type、execution_instance、runtime_node_id、agent_home_dir、workspace 文件、MCP 绑定、技能绑定和配置修订计数。
5. 软删除员工。
6. 软删除或归档员工关联的当前配置。
7. 写入 `audit_events`。
8. 提交事务。

`DeleteDigitalEmployee` 不调用 Runtime Agent，不发停止命令，不物理删除文件。

## 8. 级联处理

删除成功时保留历史链路，只清理当前可投影状态。

| 数据 | 处理 |
| --- | --- |
| `digital_employees` | 设置 `deleted_at`，同步 `status='disabled'` 和 `disabled_at` |
| `digital_employee_execution_instances` | 设置 `deleted_at`，避免 Runtime 后续选中 |
| `digital_employee_environment_variables` | 设置软删除字段或 `status='disabled'`，后续 Runtime payload 不再投影 |
| `digital_employee_mcp_bindings` | 设置 `deleted_at` |
| `digital_employee_mcp_bindings_v2` | 设置 `deleted_at` |
| `skill_agent_bindings` | 删除绑定或置为 disabled，确保技能不再投影给该员工 |
| `digital_employee_config_revisions` | 设置 `archived_at` |
| `digital_employee_workspace_files` | 设置 `deleted_at`、`archived_at`、`status='deleted'` |
| 记忆类 workspace 配置 | 按 workspace 文件同样软删除或归档 |

实现时优先复用已有表字段。若某个表缺少软删除字段，应选择该表既有的禁用或归档语义，并在审计 details 中记录处理结果。

## 9. 审计事件

删除成功必须写入 `audit_events`。

字段：

- `event_type`: `digital_employee_management`
- `actor_type`: `user`
- `actor_id`: 当前用户 ID
- `resource_type`: `digital_employee`
- `resource_id`: 员工 ID
- `action`: `digital_employee.delete`

`details` 包含：

```json
{
  "digital_employee_id": "employee-id",
  "name": "员工名称",
  "team_id": "team-id",
  "provider_type": "codex",
  "runtime_node_id": "runtime-node-id",
  "execution_instance_id": "execution-instance-id",
  "agent_home_dir": "/path/to/agent/home",
  "cascade_counts": {
    "execution_instances": 1,
    "environment_variables": 3,
    "mcp_bindings": 2,
    "mcp_bindings_v2": 1,
    "skill_bindings": 4,
    "config_revisions": 2,
    "workspace_files": 6
  },
  "cleanup_candidates": {
    "agent_home_dir": "/path/to/agent/home",
    "workspace_file_ids": ["file-id"],
    "mcp_binding_ids": ["binding-id"],
    "skill_binding_ids": ["skill-binding-id"]
  },
  "deleted_at": "2026-07-09T00:00:00Z"
}
```

安全要求：

- 不记录环境变量明文。
- 不记录密钥值、token、credential secret。
- MCP、技能、workspace 只记录清理所需标识。

## 10. 前端设计

### 10.1 删除入口

删除入口放在数字员工详情页头部操作区。

显示条件：

- 后端能力或前端权限判断包含 `employee.delete`。
- 当前员工未处于已删除状态。

不在团队页或项目页的关联列表中直接提供删除按钮，避免误操作。

### 10.2 确认弹窗

确认弹窗使用危险操作样式，并要求输入员工名称后才能确认。

弹窗文案明确：

- 删除后该员工不会再出现在数字员工列表、团队数字员工列表、项目创建和项目调度候选中。
- 历史运行、项目任务和审计记录会保留。
- Runtime 节点上的目录不会由本次请求直接物理删除，只写入审计和清理线索。

### 10.3 成功体验

删除成功后：

- 失效 `digital-employees`、`digital-employees/overview`、`team-digital-employees`、项目创建候选等 query cache。
- 跳转到 `/employees`。
- 展示成功提示：“已删除，历史记录已保留。”

### 10.4 阻断体验

收到 `409` 时不展示通用失败，而是展示 blocker 明细。

提示结构：

- 标题：`暂不能删除`
- 摘要：`该数字员工仍有排队或执行中的工作`
- 明细：按运行任务、项目任务分组，展示状态、标题、项目 ID 或运行 ID。
- 建议：`先停止运行或等待任务完成后再删除`

## 11. 项目和团队展示影响

团队管理：

- 团队数字员工列表通过 `GET /api/v1/digital-employees?team_id=...` 读取当前员工。
- 由于后端过滤 `deleted_at IS NULL`，删除后不再显示。
- 团队统计的数字员工数量必须同样只统计未删除员工。

项目管理：

- 项目创建和项目调度候选只能来自未删除员工。
- 项目历史成员或任务如果保留员工引用，不应再把该员工作为可选执行员工。
- 历史任务详情可显示员工快照或“已删除数字员工”。

## 12. OpenAPI 与生成

需要更新 Control Plane OpenAPI：

- 新增 `DELETE /api/v1/digital-employees/{employeeId}`。
- 新增 `DigitalEmployeeDeleteBlockedError` 或复用统一错误 schema 并扩展 `blockers`。
- 新增 `employee.delete` 权限说明。

更新契约后必须走生成和契约验证流程，避免 Web client、后端 handler 和 OpenAPI 漂移。

## 13. 测试计划

后端测试：

- 成功删除会设置员工 `deleted_at`，并级联处理执行实例、环境变量、MCP 绑定、技能绑定、配置版本、workspace 或记忆文件。
- 成功删除会写 `audit_events`，并包含清理线索和级联数量。
- 存在 `task_runs.status IN ('queued', 'dispatching', 'running', 'cancelling')` 时返回 `409`，且不改任何级联表。
- 存在 `project_tasks.status IN ('queued', 'running', 'in_progress')` 时返回 `409`，且返回可展示 blocker 明细。
- 已完成、失败、取消、超时的运行和历史项目任务不阻断删除。
- 无 `employee.delete` 权限时拒绝删除。
- 已删除员工再次删除返回 `404`。

前端测试：

- 有权限时员工详情页显示删除入口。
- 无权限时不显示删除入口。
- 确认弹窗要求输入员工名称。
- 成功删除后跳转员工列表并刷新相关 query cache。
- `409` 时展示运行和项目任务 blocker 明细。

## 14. 验证计划

局部验证：

```bash
make -C apps/control-plane migrate-validate
corepack pnpm test:go
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
git diff --check
```

如果实现不新增迁移，可以跳过 `migrate-validate`，但需要说明原因。

真实联调验证：

1. 使用 `scripts/dev-services.sh status` 确认 Web、Control Plane、Runtime Agent 和 Temporal 加载当前代码。
2. 创建一个无阻断数字员工。
3. 从详情页删除该员工。
4. 验证员工列表、团队数字员工列表、项目创建候选和项目调度候选不再出现该员工。
5. 通过审计事件接口或数据库验证 `digital_employee.delete` 审计事件包含清理线索。
6. 构造带 `queued/dispatching/running/cancelling` 运行的员工，验证删除返回 `409`，前端展示 blocker 明细。
7. 构造带 `queued/running/in_progress` 项目任务的员工，验证删除返回 `409`，前端展示 blocker 明细。

## 15. 实现边界

本功能涉及：

- Control Plane employee handler/service/repository/sqlc。
- `audit_events` 写入。
- OpenAPI 契约和生成物。
- Web API client。
- 员工详情页删除入口和提示。
- 受影响测试。

本功能不涉及：

- Runtime Agent 物理目录删除。
- Provider 会话强制终止。
- 项目任务自动改派。
- 历史数据物理删除。
- 新建后台清理调度器。

## 16. 验收标准

- 有阻断工作时不能删除，且提示明确到具体运行或项目任务。
- 无阻断工作时可以删除。
- 删除后当前数字员工列表、团队数字员工列表、项目创建候选和项目调度候选均不再展示该员工。
- 员工相关配置不会再投影给 Runtime。
- 审计事件能支持后续后台清理识别该员工的 Runtime 目录、workspace 文件、MCP 绑定和技能绑定。
- 历史运行、项目任务、工件、执行账本和审计记录仍可追溯。
