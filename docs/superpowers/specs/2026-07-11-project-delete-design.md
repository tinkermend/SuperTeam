# 项目删除功能设计

日期：2026-07-11  
> 复核状态：基于CHANGELOG证据
状态：已确认，待实现计划

## 1. 背景

项目管理已有归档（`POST /api/v1/projects/{projectId}/archive`），用于生命周期软结束并保留历史。产品还需要独立的**删除**能力：项目从当前管理面消失，并级联清理成员归属、任务、待审批与 Runtime 绑定。

现状要点：

- 无 `DELETE /projects/{projectId}`，无 `projects.deleted_at`。
- 归档 API 与 service 已存在；详情页归档按钮埋在「高级」折叠侧栏，头部不可见。
- 项目写操作鉴权已区分：租户 owner/admin 或项目 `human_owner` 可 `project.update` / `project.archive`；普通成员不可。
- 数字员工删除已建立「preview/阻断 + 软删 + 审计」先例；本设计对齐该模式，但项目删除额外要求终止 Temporal coordinator，并清理项目域投影数据。

## 2. 目标

- 提供与归档分离的项目删除能力（软删除）。
- 删除前硬阻断仍在排队/执行中的工作；对待审批等可清理项在 preview 中警告，确认后一并清理。
- 删除前先终止项目协调 Temporal workflow；终止失败则不改库。
- 解除数字员工与人类成员的项目归属；释放 Runtime 节点绑定与 affinity。
- 写入可审计的 `project.delete` 事件。
- 在项目详情头部补齐可见的「归档」与「删除」入口（仅详情，不进列表）。

## 3. 非目标

- 不替代或合并归档语义；归档仍是独立软结束路径。
- 不物理 purge 证据、工件、报告、预算账本、执行账本或历史审计行。
- 不调用 Runtime Agent 停止本机进程或删除本机目录。
- 不做已删项目回收站 / 恢复 UI。
- 不做列表页或批量删除。
- 不重新设计项目角色体系；权限以当前 `human_owner` + 租户 admin 为准。

## 4. 产品规则

### 4.1 删除 vs 归档

| 能力 | 语义 |
|---|---|
| 归档 | 项目进入 `archived`，历史保留，不再推进；不级联清成员/任务/审批/runtime |
| 删除 | 危险操作：软删项目，级联清理当前投影与待办，列表/详情不可见 |

已归档项目在无活跃执行时可删除。

### 4.2 谁可以删

允许：

1. 租户 `owner` / `admin`
2. 项目 `human_owner_user_id`

不允许：普通项目成员；仅 `leader` / `acceptance`（除非同时满足上列之一）。

动作名：`project.delete`。`delete-preview` 使用同一动作（只有能删的人能看预览）。

### 4.3 阻断与警告

硬阻断（`409 project_delete_blocked`，不改库、不停 workflow）：

- `project_tasks.status IN ('queued', 'running', 'in_progress')`
- 该项目关联仍活跃的 `task_runs.status IN ('queued', 'dispatching', 'running', 'cancelling')`

仅警告（preview 展示，用户确认后清理）：

- 待处理 `project_decision_requests` / `approval_requests`
- `project_tasks` 处于 `waiting_human` / `pending_review`
- 未关闭的项目相关 `inbox_items`

### 4.4 删除成功后的产品语义

- 项目不再出现在项目列表、详情、调度与创建候选中。
- 纳入该项目的数字员工实体仍在；仅失去该项目成员归属。
- 该项目任务不再作为可操作当前任务出现（标记为 `cancelled` 或等价终态）。
- 待审批与对应 inbox 关闭。
- Runtime 绑定释放；Console/Control Plane 不清理 Runtime 本机文件。
- 审计可追溯删除动作与 cascade 计数。

## 5. 后端契约

### 5.1 预览

```http
GET /api/v1/projects/{projectId}/delete-preview
```

鉴权：`project.delete`，资源 `project`。

成功 `200` 示例字段：

```json
{
  "project_id": "...",
  "project_name": "...",
  "can_delete": false,
  "blockers": [
    {
      "type": "project_task",
      "id": "...",
      "status": "running",
      "title": "..."
    }
  ],
  "warnings": {
    "pending_decision_count": 2,
    "waiting_human_task_count": 1,
    "open_inbox_count": 2,
    "active_member_count": 5,
    "digital_employee_member_count": 3,
    "runtime_node_binding_count": 1,
    "affinity_count": 2
  },
  "message": "删除将取消待审批并解除成员与 Runtime 绑定。存在活跃执行时不可删除。"
}
```

`can_delete` 为 `true` 当且仅当 `blockers` 为空（且项目未删除）。Workflow 是否可终止不在 preview 强依赖 Temporal 实时探测；执行删除时再终止。

### 5.2 删除

```http
DELETE /api/v1/projects/{projectId}
```

鉴权：`project.delete`，资源 `project`。

| 结果 | 状态 |
|---|---|
| 成功 | `204 No Content` |
| 不存在或已删 | `404` |
| 活跃执行阻断 | `409` + `code: project_delete_blocked` + `blockers[]` |
| Workflow 终止失败 | `409` 或 `503`，可重试；DB 不变 |

阻断响应体对齐数字员工删除风格：`code`、`message`、`blockers[]`（含 `type`/`id`/`status`/`title`）。

### 5.3 权限与 allowed_actions

- `authz.ActionProjectDelete = "project.delete"`
- `checkProjectAccess`：与 `project.archive` / `project.update` 相同——租户 admin 或 `human_owner`；**不**对普通成员开放
- OpenFGA mapping 同步挂上 `project.delete`
- `Project`（或详情/overview 响应）增加 `allowed_actions`，至少可含 `project.archive`、`project.delete`

## 6. 数据与级联

### 6.1 Schema

- `projects` 增加 `deleted_at TIMESTAMPTZ NULL`
- 所有当前项目列表/详情/调度查询增加 `deleted_at IS NULL`（或等价）
- 迁移落在 `apps/control-plane/internal/storage/migrations/`，更新 `atlas.sum` 并 `migrate-validate`

可选：同步将 `status` 设为应用层认可的终态字符串；若与现有 `ProjectStatus` 枚举冲突，以 `deleted_at` 为权威，不强制扩枚举。

### 6.2 级联（同一 DB 事务）

在 Temporal coordinator 已成功终止（或不存在）之后：

1. 软删 `projects`（写 `deleted_at`）
2. `project_members`：active → `removed`/`inactive`（人类与数字员工均解除归属；不删 `digital_employees` 行）
3. `project_tasks`：非终态与历史当前投影统一标 `cancelled`（或等价），使其不可再调度
4. pending `project_decision_requests` / `approval_requests` → `cancelled`
5. 相关 open `inbox_items` → 关闭/取消
6. 删除投影：`project_runtime_nodes`、`project_employee_node_affinity`；失效活跃 `project_placements`
7. 写 `audit_events`：`action = project.delete`，details 含 cascade 计数与 workflow_id；不含密钥明文

保留（本阶段不物理删）：`project_events`、证据/工件/报告/预算、执行账本、历史审计。Console 因项目不可见而不再入口展示。

### 6.3 幂等

已 `deleted_at` 再删 → `404`。

## 7. Temporal 终止与事务边界

`CoordinatorSignalClient` 新增：

```text
TerminateProjectCoordinator(ctx, {TenantID, ProjectID, WorkflowID, Reason})
```

实现使用 Temporal Client Terminate/Cancel；workflow 已结束或不存在视为成功。

严格顺序：

1. 鉴权
2. 锁项目行（`FOR UPDATE`）；已删 → `404`
3. 查阻断；有则 `409`，不停 workflow、不改库
4. 终止 coordinator；失败则返回可重试错误，DB 不变
5. DB 事务：级联软删与清理 + 审计 → commit
6. 不调用 Runtime Agent

补偿说明：若步骤 4 成功、步骤 5 失败，允许用户重试删除；重试时 workflow 已结束算终止成功，继续事务。需在服务日志/审计候选中记录该窗口，避免静默不一致。

归档路径**不**使用本终止与级联逻辑。

## 8. 前端

### 8.1 入口

仅项目详情头部操作区（与「提交需求 / 配置项目」同级）：

- **归档项目**（outline）：从高级折叠侧栏挪到头部；已归档禁用/隐藏；由 `project.archive` 控制可见性（若暂未注入 allowed_actions，至少与现有可归档行为一致并尽快改为 allowed_actions）
- **删除项目**（destructive）：仅当 `allowed_actions` 含 `project.delete`

列表不加归档/删除。高级区原归档按钮移除，避免双入口。

### 8.2 归档交互

沿用现有 `archiveProject` mutation；成功后刷新详情。

### 8.3 删除交互

对齐数字员工删除：

1. 打开确认弹窗时请求 `delete-preview`
2. 展示 warnings 摘要；若有 blockers，展示明细并禁用确认
3. 要求输入项目名称确认
4. 确认后 `DELETE`；成功导航回项目列表并失效相关 query
5. `409 project_delete_blocked` 在弹窗内展示阻断
6. Workflow 终止失败展示可重试文案

遵循 `DESIGN.md` 与现有 v3 / `ConfirmDialog` 模式；内部跳转用 TanStack Router。

## 9. 测试与验证

- Control Plane：authz 单测；service 阻断/成功/回滚；handler/route；sqlc 查询；迁移校验
- Web：按钮可见性、preview 警告、阻断 UI、成功跳转
- 提交前门禁：受影响层的 `verify:*`（至少 `verify:control-plane`、`verify:web`、契约相关）
- 完成条件：真实 Web → Control Plane → DB → Temporal 路径 smoke；不得仅以单测/构建冒充端到端

## 10. 风险与决策摘要

| 决策 | 选择 |
|---|---|
| 与归档关系 | 独立删除（A）；详情补归档按钮 |
| 活跃执行 | 硬阻断；待审批警告后清理（C） |
| 存储 | 软删（A） |
| Temporal | 先终止成功再改库（C） |
| Preview | 独立 `delete-preview`（B） |
| UI 入口 | 仅详情（A） |
| 实现路径 | 事务型软删 + preview，对齐数字员工删除 |

主要风险：

- Workflow 已停、DB 事务失败的短暂窗口 → 靠可重试删除收敛
- 任务「级联删除」落地为终态标记而非物理删，避免破坏 FK/审计；若产品后续要求物理 purge，另开阶段
- 列表/调度漏过滤 `deleted_at` → 实现时用查询清单回归
