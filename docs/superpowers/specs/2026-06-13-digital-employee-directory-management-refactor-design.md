# 数字员工目录定义与工作区文件管理重构设计

日期：2026-06-13
状态：已确认，待实现计划

## 1. 背景

数字员工已经具备团队归属、运行实例、Provider 绑定、Provider Session、Run 执行闭环、技能和 MCP 的团队/员工两层治理能力。但当前 Runtime Agent 目录策略仍停留在 `workspace.base_dir/agents/{execution_instance_id}`，并且旧 task executor 中还存在 `instances/{execution_instance_id}/runs/{run_id}/workspace` 这种按 run 创建临时工作目录的路径。

这与业务模型不一致。数字员工是归属于团队的长期业务身份，可以参与多个项目；项目只是任务、证据和验收的管理容器，不应该成为数字员工执行目录的上级。数字员工调试和项目内任务执行都应该在同一个数字员工长期目录中启动 Provider，通过不同 Provider Session、Run、ProjectTask metadata 区分上下文和审计。

同时，数字员工本地目录不能被建模成只有一个 `AGENTS.md` 文件。后续 Console 的 Instructions 页面需要支持文件列表、加号新增文件、编辑、版本化和同步下发。第一阶段只实现 DB 存储文本文件，但模型必须预留对象存储和多文件扩展。

## 2. 核心口径

- 数字员工归属团队，不归属项目。
- `digital_employee_id` 是目录业务身份；`execution_instance_id` 是运行实例记录，不作为目录 key。
- 创建数字员工时选定 `team_id`、`runtime_node_id` 和 `provider_type`。当前阶段这些绑定不可变；如需换团队、Runtime 或 Provider，重新创建数字员工。
- Runtime 本地员工目录是 Provider 执行 `cwd` 和受控文件同步副本，不是平台事实源。
- Control Plane DB 是数字员工文件列表、文件版本、技能/MCP 绑定、Provider Session、Run 状态、上下文和审计的事实源。
- Provider 内部目录由 Provider adapter 管理。平台只定义员工根目录和受控文件 materialization，不定义 `.claude`、`.opencode` 内部结构。

## 3. 目标

- 统一数字员工长期目录为 `{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}`。
- 数字员工调试和项目任务执行使用同一个员工目录作为 Provider `cwd`。
- 新增数字员工工作区文件资产模型，支持 `AGENTS.md` 和后续用户新增文件。
- 第一版文件正文存 DB，预留对象存储字段。
- 创建数字员工时默认生成 `AGENTS.md` 文件资产，并同步到 Runtime 员工目录。
- Runtime 根据绑定 Provider 初始化对应 dot dir，例如 `claude-code` 创建 `.claude`，`opencode` 创建 `.opencode`。
- `CLAUDE.md` 作为兼容产物指向 `AGENTS.md`，不作为用户可编辑主文件。
- 后续文件新增、编辑和激活 revision 后，可以通过同步命令推送到对应数字员工目录。

## 4. 非目标

- 不在本阶段实现 Console Instructions 文件编辑页面。
- 不在本阶段实现对象存储正文落盘；只预留字段。
- 不让 Runtime 扫描本地目录反向更新 Control Plane 文件事实。
- 不定义 `.claude`、`.opencode` 等 Provider dot dir 的内部文件格式。
- 不实现数字员工跨团队、跨 Runtime 或跨 Provider 迁移。
- 不改变项目任务的业务归属模型；项目只进入 metadata 和上下文，不改变员工目录。

## 5. 方案选择

### 5.1 方案 A：团队下的数字员工长期目录 + DB 文件资产

目录 key 使用 `team_id + digital_employee_id`：

```text
{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}
```

文件清单和内容版本在 Control Plane DB 中建模。Runtime 目录只保存当前激活版本的同步副本。

优点：

- 符合“团队拥有数字员工，项目调度数字员工”的业务模型。
- 同一个数字员工参与多个项目时，不会切碎技能、MCP、AGENTS.md、Provider 本地上下文和会话恢复。
- 后续 Instructions 页面可以自然展示文件列表、ENTRY 文件、编辑和同步状态。
- 保留 Runtime 本地目录作为 Provider 工作区，但不让本地文件系统成为长期事实源。

结论：采用。

### 5.2 方案 B：继续使用 `agents/{execution_instance_id}`

优点是改动小。缺点是执行实例变成了目录业务身份，后续读目录、UI 展示和审计都需要额外反查数字员工；如果未来执行实例概念扩展，也容易把运行绑定和员工身份混在一起。

结论：不采用。

### 5.3 方案 C：项目下的数字员工目录

目录形如：

```text
projects/{project_id}/employees/{digital_employee_id}
```

这个方案把项目管理维度误当成员工归属维度。一个数字员工参与多个项目时会产生多个目录，导致技能、MCP、AGENTS.md、Provider Session 和长期上下文被项目切碎。

结论：明确不采用。

## 6. 目录规范

数字员工根目录：

```text
{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}/
```

首版 Runtime materialization 示例：

```text
{employee_home}/
  AGENTS.md
  CLAUDE.md
  .claude/
```

`AGENTS.md` 是受控文件资产中的入口文件。`CLAUDE.md` 是 Runtime 为 Claude Code 兼容生成的软链接：

```text
CLAUDE.md -> AGENTS.md
```

如果目标环境不支持软链接，Runtime 写入一个薄兼容文件，内容只说明主入口文件为 `AGENTS.md`，并包含或引用相同正文。该兼容文件不作为 Console 文件列表中的独立业务文件。

平台不预创建以下通用子目录：

```text
state/
sessions/
runs/
artifacts/
skills/
mcp/
context/
```

这些目录要么是 Provider 自身约定，要么对应 Control Plane DB 中的事实，不进入平台目录规范。

Provider dot dir 由 adapter 按 provider 初始化：

- `claude-code`：创建或校验 `.claude/`。
- `opencode`：创建或校验 `.opencode/`。
- 后续 Provider 由对应 adapter 注册初始化器。

## 7. 数据库设计

新增 `digital_employee_workspace_files` 表，保存数字员工根目录下的受控文件身份。

表字段：

```text
id UUID PRIMARY KEY
tenant_id UUID NOT NULL
team_id UUID NOT NULL
digital_employee_id UUID NOT NULL
path TEXT NOT NULL
file_role VARCHAR(50) NOT NULL
mime_type VARCHAR(100) NOT NULL
sync_policy VARCHAR(50) NOT NULL
current_revision_id UUID
status VARCHAR(50) NOT NULL
metadata JSONB NOT NULL DEFAULT '{}'::jsonb
created_by UUID
created_at TIMESTAMPTZ NOT NULL
updated_at TIMESTAMPTZ NOT NULL
archived_at TIMESTAMPTZ
deleted_at TIMESTAMPTZ
```

关键约束：

- `UNIQUE (tenant_id, digital_employee_id, path) WHERE deleted_at IS NULL`
- `path` 由服务端规范化和校验，不允许绝对路径、空路径、`..`、反斜杠、控制字符、以 `/` 结尾的目录路径。
- `file_role` 首版支持 `entrypoint`、`supporting_doc`、`provider_config`、`generated`，由服务端注册表校验，不使用数据库 enum。
- `sync_policy` 首版支持 `auto`、`manual`、`disabled`，由服务端注册表校验。
- 同一数字员工只能有一个 active `entrypoint` 文件。

新增 `digital_employee_workspace_file_revisions` 表，保存文件内容版本。

表字段：

```text
id UUID PRIMARY KEY
tenant_id UUID NOT NULL
file_id UUID NOT NULL
revision_number INTEGER NOT NULL
content_text TEXT
content_hash VARCHAR(64) NOT NULL
size_bytes INTEGER NOT NULL
storage_backend VARCHAR(50) NOT NULL
object_key TEXT
created_by UUID
created_at TIMESTAMPTZ NOT NULL
change_note TEXT
metadata JSONB NOT NULL DEFAULT '{}'::jsonb
```

关键约束：

- `UNIQUE (file_id, revision_number)`
- `storage_backend = 'db'` 时 `content_text` 必填，`object_key` 为空。
- 预留 `storage_backend = 'object_store'`，用于未来大文件或二进制文件。
- `content_hash` 使用 SHA-256 hex。

第一版 `AGENTS.md` 是默认创建的文件资产：

```text
path = "AGENTS.md"
file_role = "entrypoint"
mime_type = "text/markdown"
sync_policy = "auto"
storage_backend = "db"
```

`digital_employee_execution_instances.agent_home_dir` 继续保留，语义收敛为 Runtime 员工根目录的规范化结果：

```text
{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}
```

## 8. Control Plane 行为

### 8.1 创建数字员工

创建成功前，Control Plane 完成：

1. 创建数字员工业务身份。
2. 创建唯一 execution instance。
3. 合成团队治理、员工配置、技能/MCP 选择和角色画像。
4. 创建默认 `AGENTS.md` workspace file 和 revision。
5. 将 `agent_home_dir` 计算并写入 execution instance。
6. 下发 `provision_instance` 命令，携带员工目录身份、Provider 类型和需要同步的文件 revision。

`AGENTS.md` 内容第一版由 Control Plane 生成，包含：

- 团队宪法和员工角色边界。
- 生效配置摘要。
- 技能/MCP 选择摘要。
- 审批和安全边界。
- 输出契约和工作方式约束。

AGENTS.md 是可编辑文件资产。后续编辑不直接覆盖历史，而是创建新 revision 并激活。

### 8.2 文件新增和编辑

后续 Console Instructions 页面新增文件时：

1. Control Plane 校验路径、文件角色、mime type 和权限。
2. 创建 `digital_employee_workspace_files` 记录。
3. 创建初始 revision。
4. 如 `sync_policy=auto`，下发 `sync_workspace_files`。

编辑文件时：

1. 创建新 revision。
2. 更新 `current_revision_id`。
3. 记录审计事件。
4. 根据同步策略触发 Runtime 同步或标记待同步。

文件删除首版采用软删除；Runtime 同步时可以删除对应本地 materialized 文件，但不能删除 Provider 保留路径。

## 9. Runtime 命令

### 9.1 `provision_instance`

`provision_instance` 语义调整为首次 materialize 数字员工工作目录：

1. 校验 `tenant_id`、`team_id`、`digital_employee_id`、`execution_instance_id`、`provider_type`。
2. 创建员工根目录 `{workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}`。
3. 根据 `provider_type` 调用 provider adapter 初始化 dot dir。
4. 写入 `sync_policy=auto` 的初始文件，例如 `AGENTS.md`。
5. 创建或更新 `CLAUDE.md` 兼容链接。
6. 回写 `agent_home_dir`、已同步文件 hash、Provider 初始化结果和失败列表。

### 9.2 `sync_workspace_files`

新增 Runtime command：

```text
sync_workspace_files
```

用途：将指定文件 revision 同步到数字员工根目录。

payload 至少包含：

```text
command_id
tenant_id
team_id
digital_employee_id
execution_instance_id
provider_type
agent_home_dir
files: [
  {
    file_id
    revision_id
    path
    file_role
    mime_type
    content_hash
    size_bytes
    storage_backend
    content_text
  }
]
delete_paths: []
```

Runtime 处理规则：

- 所有路径必须通过安全校验。
- 写入使用临时文件 + fsync + rename 的原子替换策略。
- 同步完成后重新计算 hash 并回写。
- 单个文件失败时返回失败列表；Control Plane 记录同步失败状态。
- 不允许通过该命令写入 Provider 保留路径。

### 9.3 `start_session`

`start_session` 不再按 `execution_instance_id` 自行计算目录。Runtime 必须使用已 provision 的员工根目录作为 Provider `cwd`：

```text
cwd = {workspace_base_dir}/teams/{team_id}/employees/{digital_employee_id}
```

启动前校验：

- execution instance 处于 `ready` 或 `active`。
- 本地员工目录存在。
- 必要 auto-sync 文件 hash 与 Control Plane 目标 revision 一致。
- 如果 hash 不一致，优先触发同步；同步失败则拒绝启动并回写结构化错误。

## 10. Provider Adapter 边界

Provider adapter 负责 Provider 专属目录和配置 materialization：

- Claude Code adapter 管理 `.claude/`。
- OpenCode adapter 管理 `.opencode/`。
- Provider 内部目录不是平台通用文件资产，不在 Console 的普通文件列表中展示为可编辑文件。

技能、MCP、Provider 配置的事实源仍在 Control Plane：

- 技能/MCP 绑定关系在 DB。
- Provider adapter 可以把这些绑定 materialize 成 Provider 所需配置文件。
- 本地 Provider 配置文件不是反向同步源。

## 11. 项目任务与调试任务

数字员工调试：

```text
cwd = teams/{team_id}/employees/{digital_employee_id}
metadata.source = employee_debug
provider_session_id = 调试会话
```

项目任务：

```text
cwd = teams/{team_id}/employees/{digital_employee_id}
metadata.source = project_task_dispatch
metadata.project_id = ...
metadata.project_task_id = ...
provider_session_id = 项目任务会话
```

项目不改变目录。一个数字员工参与多个项目时，仍复用同一个员工目录，通过 Provider Session 和 Run metadata 隔离审计。

## 12. 路径安全和保留路径

普通 workspace file 不允许写入：

- `.claude/**`
- `.opencode/**`
- `.git/**`
- `.superteam/**`
- 绝对路径
- 包含 `..` 的路径
- 空路径
- 以 `/` 结尾的路径
- 控制字符或平台不支持字符

`AGENTS.md` 是保留入口文件，只能以 `entrypoint` 角色存在。`CLAUDE.md` 是 Runtime 兼容产物，不允许用户创建同名 workspace file。

## 13. 同步状态

Control Plane 需要记录文件同步状态。首版使用独立同步投影表，避免把运行状态塞进文件 metadata：

```text
digital_employee_workspace_file_syncs
```

表字段：

```text
tenant_id
digital_employee_id
execution_instance_id
file_id
revision_id
runtime_node_id
status              # pending / synced / failed
synced_hash
error_message
last_command_id
last_synced_at
updated_at
```

Console 可以基于该表展示：

- 已同步 revision。
- 待同步 revision。
- 同步失败原因。
- 最后同步时间。

## 14. 兼容和迁移

已有 `agents/{execution_instance_id}` 目录不作为新模型继续扩展。迁移策略：

1. 新创建数字员工使用新目录。
2. 旧数字员工首次同步或首次运行时，Control Plane 计算新 `agent_home_dir` 并下发 re-provision/sync。
3. Runtime 不自动搬迁旧目录内容，避免误把 Provider 缓存当平台事实。
4. 如需要保留旧 Provider 会话缓存，后续做显式迁移工具，由人工触发和审计。

DB 迁移不删除 `execution_instance_id`，不删除 `agent_home_dir`，只调整其写入和使用语义。

## 15. 测试策略

Control Plane 单元测试：

- 创建数字员工会创建默认 `AGENTS.md` file 和 revision。
- `AGENTS.md` revision hash、size、current revision 正确。
- 新增文件校验 path、role、mime type 和权限。
- 编辑文件创建新 revision，不覆盖历史。
- `provision_instance` payload 包含 team/digital employee 目录身份和文件 revision。
- `start_session` payload 包含目标文件 revision/hash。

Runtime Agent 单元测试：

- `ensure_instance` 创建 `teams/{team_id}/employees/{digital_employee_id}`。
- 非法路径被拒绝。
- `CLAUDE.md` 软链接或兼容文件创建正确。
- `claude-code` 初始化 `.claude`，`opencode` 初始化 `.opencode`。
- `sync_workspace_files` 原子写入并校验 hash。
- 写入 Provider 保留路径被拒绝。

链路测试：

- 创建数字员工后 Runtime 目录包含 `AGENTS.md` 和 provider dot dir。
- 数字员工调试 run 和项目任务 run 使用同一个 `cwd`。
- 修改 `AGENTS.md` 后触发同步，Runtime 本地 hash 与 DB revision hash 一致。
- 同步失败时 run 不应被表述为可用，Control Plane 返回可排查错误。

## 16. 后续扩展

- Console Instructions 页面：文件列表、ENTRY 标签、加号新增、编辑预览、同步状态。
- 对象存储正文：大文件、二进制附件、导入文件包。
- 文件级审批：高风险员工或生产团队修改 `AGENTS.md` 前需要人类审批。
- Provider 配置可视化：只展示 adapter 暴露的安全摘要，不直接编辑 Provider 私有目录。
- 显式旧目录迁移工具：用于保留 Provider 缓存或人工确认搬迁。

## 17. 实施顺序建议

1. 新增 DB 表、sqlc 查询和领域类型。
2. 调整数字员工创建流程，生成 `AGENTS.md` file/revision，并在 provisioning payload 中携带文件清单。
3. 重构 Runtime `ensure_instance`，使用 `team_id + digital_employee_id` 创建员工根目录。
4. 增加 Provider adapter 初始化接口。
5. 增加 `sync_workspace_files` 命令。
6. 调整 `start_session` 使用员工根目录，并校验同步 hash。
7. 增加测试和最小 API/Console 数据读取能力。
