# 数字员工 CLI 技能依赖与运行环境变量设计

日期：2026-06-22
状态：已确认，待实现计划

## 1. 背景

SuperTeam 当前已经有技能管理、团队/数字员工技能绑定、Runtime capability 上报、数字员工长期目录、Runtime `provision_instance` 和 `start_session` 执行链路。现有能力能把技能包下发到 Runtime，但缺少三个关键闭环：

- 技能无法声明自己依赖哪些本机 CLI 工具和环境变量。
- Runtime 节点无法把本机已安装 CLI 工具作为 capability 上报。
- 数字员工无法保存自身运行所需环境变量，也无法在 Provider 进程启动时动态注入。

本设计先落地 CLI-first 的技能运行依赖与员工运行环境变量。项目源码访问方式、项目目录传递策略、MCP 自动合并和 CLI 工具下发部署 UI 均不纳入本阶段。

## 2. 目标

- 技能仍使用统一 Skill 模型，不新增显式“CLI 技能”类别。
- Skill 可以声明运行依赖：必需 CLI 工具和必需环境变量名。
- Runtime Agent 探测配置清单中的 CLI 工具是否存在，并通过现有 capability 上报通道写入 Control Plane。
- 技能绑定到数字员工后，Console 能显示该技能在当前员工 Runtime 节点上是否可加载，以及缺少哪些工具或环境变量。
- 数字员工创建和详情编辑支持配置环境变量。
- 环境变量值数据库只保存密文、key id、指纹和审计字段，不保存长期明文。
- 启动数字员工任务时，Control Plane 解密员工环境变量，通过 Runtime command payload 下发给 Runtime Agent，Runtime Agent 注入 Provider 子进程环境。
- Runtime、Control Plane 和 Web 日志不得输出环境变量明文。

## 3. 非目标

- 不实现项目源码目录访问策略。
- 不实现 CLI 工具在 Runtime 节点上的下发、安装、升级或版本管理。
- 不做 CLI 工具版本约束；第一期只判断工具是否存在。
- 不新增技能类别，不把“CLI 技能”做成独立菜单。
- 不实现 MCP 与项目目录 MCP 配置的自动合并或冲突处理。
- 不让 Runtime Agent 持有环境变量解密密钥。
- 不接入 KMS、Vault 或多租户 per-tenant key 管理；第一期使用 Control Plane 部署 secret。
- 不实现普通 Web 页面查看环境变量明文。

## 4. 方案选择

### 4.1 采用方案：统一 Skill + runtime dependencies + Runtime tool capability

技能继续是统一 Skill。运行依赖是 Skill 的元数据：

```json
{
  "runtime_dependencies": {
    "tools": ["gh", "kubectl"],
    "env": ["GH_TOKEN"]
  }
}
```

Runtime Agent 从配置读取待探测工具清单，按 PATH 查找工具。找到则上报：

```json
{
  "capability_type": "tool",
  "capability_key": "gh",
  "provider_type": "tool",
  "binary_path": "/usr/bin/gh",
  "available": true,
  "status": "available",
  "health_status": "configured"
}
```

Control Plane 在技能绑定展示和任务启动前，根据 Skill 依赖、员工环境变量、Runtime 节点 capability 计算可加载状态。

采用原因：

- 保留现有技能管理和绑定模型，不增加用户心智负担。
- 复用 `runtime_capabilities`，不新增能力体系。
- 复用 `start_session` payload 和 Runtime Provider adapter，能形成真实运行闭环。
- 第一阶段可快速验证 CLI-first 方案是否成立。

### 4.2 不采用：单独新增 CLI Skill 类型

单独分类会让技能管理、绑定、授权、搜索、详情页、任务运行都出现分支。用户在使用时还要先判断某个能力是普通技能还是 CLI 技能。当前需求只需要声明运行依赖，不需要拆模型。

### 4.3 不采用：Runtime UI 负责安装工具

Runtime 工具安装涉及操作系统、包管理器、权限、网络、升级、回滚和节点差异。第一期让运维或节点 bootstrap 管理 CLI 工具安装，平台只探测和展示是否具备能力。

## 5. 数据模型

### 5.1 Skill 运行依赖

第一期优先复用 `skills.metadata` 存储依赖，服务端 domain struct 显式暴露 `RuntimeDependencies` 字段。

逻辑结构：

```text
runtime_dependencies.tools []string
runtime_dependencies.env   []string
```

规范化规则：

- `tools` 只接受非空 CLI 名称，不接受路径。
- `tools` 名称必须匹配 `[A-Za-z0-9._-]+`。
- `env` 只接受环境变量名，不接受值。
- `env` 名称必须匹配 `[A-Za-z_][A-Za-z0-9_]*`。
- 服务端去重、排序、去空白。
- 依赖为空表示该技能无额外 Runtime 依赖。

### 5.2 数字员工环境变量表

新增 `digital_employee_environment_variables` 表，独立保存数字员工运行环境变量。它不进入配置修订 JSON，避免配置响应、配置预览和 revision diff 意外携带密文或明文。

建议字段：

```text
id UUID PRIMARY KEY
tenant_id UUID NOT NULL
team_id UUID NOT NULL
digital_employee_id UUID NOT NULL
name TEXT NOT NULL
encrypted_value TEXT NOT NULL
encryption_key_id TEXT NOT NULL
value_fingerprint TEXT NOT NULL
sensitive BOOLEAN NOT NULL DEFAULT true
status VARCHAR(50) NOT NULL DEFAULT 'active'
created_by UUID
updated_by UUID
created_at TIMESTAMPTZ NOT NULL
updated_at TIMESTAMPTZ NOT NULL
deleted_at TIMESTAMPTZ
metadata JSONB NOT NULL DEFAULT '{}'::jsonb
```

关键约束：

- `UNIQUE (tenant_id, digital_employee_id, name) WHERE deleted_at IS NULL`
- `name` 按环境变量名规则校验。
- `status` 第一版支持 `active`、`disabled`，由服务端注册表校验，不使用数据库 enum。
- `encrypted_value` 不允许为空。
- `sensitive` 第一版默认 true。普通 Console 不返回任何变量明文；后续如需非敏感明文展示，另做显式产品设计。

## 6. 加密与密钥管理

### 6.1 密钥存放

环境变量加密密钥放在数据库之外，由 Control Plane 部署环境提供。

第一期配置：

```text
SUPERTEAM_ENV_ENCRYPTION_KEYS=v1:base64-32-byte-key
SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID=v1
```

部署建议：

- 本地开发：`.env` 或本机私有环境文件，不进 git。
- 服务器：`/etc/superteam/control-plane.env`，文件权限 `0600`。
- Docker：Docker secret 或容器环境变量。
- Kubernetes：Kubernetes Secret；后续可接 SealedSecret 或 ExternalSecret。

密钥不进入：

- 数据库
- Web 前端
- Runtime Agent
- Provider 进程
- 代码仓库
- 日志和审计正文

### 6.2 加密格式

Control Plane 使用服务端密钥进行应用级加密。推荐 AES-256-GCM，每个变量值使用随机 nonce。

存储格式包含：

```json
{
  "encryption_key_id": "v1",
  "encrypted_value": "base64(nonce+ciphertext+tag)",
  "value_fingerprint": "hmac-sha256-prefix"
}
```

`value_fingerprint` 用于 UI 显示和排障确认，不用于解密。指纹由服务端密钥派生出的 fingerprint key 计算，并截断展示，例如前 8 到 12 个 hex 字符。不要直接存 `sha256(value)`，避免低熵值被离线猜测。

### 6.3 密钥轮换

从第一期开始保存 `encryption_key_id`。

- 加密新值时使用 `SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID`。
- 解密旧值时按记录上的 `encryption_key_id` 找对应密钥。
- 轮换时新增 `v2`，把 active key 切到 `v2`。
- 后台任务可逐条解密旧密文并用新 key 重加密。

第一期可以不实现轮换任务，但数据结构必须支持多 key 解密。

## 7. Control Plane 行为

### 7.1 技能上传和编辑

技能上传或编辑时，服务端接收 `runtime_dependencies`：

```json
{
  "tools": ["gh"],
  "env": ["GH_TOKEN"]
}
```

服务端负责校验、规范化并写入 Skill metadata。技能列表和详情响应返回规范化后的依赖。

### 7.2 员工环境变量管理

创建数字员工时可以同时提交环境变量列表：

```json
[
  {
    "name": "GH_TOKEN",
    "value": "plain-token-entered-once",
    "sensitive": true
  }
]
```

服务端保存时立即加密。普通 API 响应只返回：

```json
{
  "name": "GH_TOKEN",
  "configured": true,
  "fingerprint": "a13f09c2",
  "sensitive": true,
  "updated_at": "2026-06-22T10:00:00Z"
}
```

更新语义：

- 新增：写入加密值。
- 替换：用新明文重新加密，更新指纹和审计字段。
- 删除：软删除。
- 禁用：保留密文但 `status=disabled`，运行时不注入。
- 普通详情页不提供明文读取。

### 7.3 管理员解密查看

管理员确需查看明文时，通过后端 CLI 或受限审计接口完成。

流程：

```text
管理员 -> CLI/审计接口 -> Control Plane 权限校验 -> 解密 -> 写审计 -> 返回一次性明文
```

审计记录至少包含：

```text
who
when
tenant_id
team_id
digital_employee_id
env_name
reason
request_id
source_ip 或 execution_context
```

没有管理员权限或没有填写查看原因时拒绝。普通 Web 详情页不接入这个明文接口。

### 7.4 技能依赖状态计算

Control Plane 提供内部依赖评估能力：

```text
EvaluateSkillRuntimeDependencies(skill, digital_employee, runtime_node)
```

输入：

- Skill 的 required tools/env。
- 员工 active 环境变量名集合。
- 员工当前 Runtime 节点 `runtime_capabilities` 中 `capability_type=tool` 且 `available=true` 的工具集合。

输出：

```json
{
  "load_status": "loadable",
  "missing_tools": [],
  "missing_env": []
}
```

状态：

- `loadable`：依赖满足。
- `missing_tools`：缺少 Runtime CLI 工具。
- `missing_env`：缺少员工环境变量。
- `pending_runtime`：员工还没有可判断的 Runtime 节点或 capability 尚未上报。

绑定技能时可以保存绑定关系，但响应和员工详情必须展示依赖状态。启动任务时，如果 active 技能依赖不满足，则拒绝启动并返回明确错误。

### 7.5 启动任务

`start_session` 构建前：

1. 查询员工有效技能集合。
2. 校验技能 runtime dependencies。
3. 如存在不可加载的 active 技能，拒绝创建或派发 run，返回缺失项。
4. 查询员工 active 环境变量，解密后形成 `environment` payload。
5. 将可加载技能填入 `skills` payload。
6. 将环境变量填入 `environment` payload。

失败示例：

```json
{
  "error": "skill_dependencies_not_satisfied",
  "message": "技能 GitHub Issue 处理无法加载：Runtime 节点缺少 gh；员工缺少 GH_TOKEN",
  "missing_tools": ["gh"],
  "missing_env": ["GH_TOKEN"]
}
```

## 8. Runtime Agent 行为

### 8.1 CLI 工具探测

Runtime Agent 增加配置：

```yaml
tools:
  probe_names:
    - git
    - gh
    - kubectl
    - psql
```

环境变量覆盖：

```text
RUNTIME_AGENT_TOOL_PROBE_NAMES=git,gh,kubectl,psql
```

探测规则：

- 对每个 tool name 按 Runtime Agent 进程 PATH 查找。
- 找到则上报 `available=true` 和 `binary_path`。
- 找不到则上报 `available=false`，`status=missing`。
- 第一版不执行 `--version`。
- 不扫描 PATH 中所有可执行文件。

### 8.2 Runtime command payload

`RuntimeSessionCommandPayload` 增加环境变量字段：

```json
{
  "environment": [
    {
      "name": "GH_TOKEN",
      "value": "plain-token",
      "sensitive": true
    }
  ]
}
```

Runtime 解析 payload 后把环境变量复制到 `RunSpec`，再传入 `ProviderRequest`。

### 8.3 Provider 子进程注入

`ProviderRequest` 增加 `environment` map 或列表。Claude、Codex、OpenCode adapter 在构造 `std::process::Command` 后统一调用 env 注入函数：

```text
command.envs(redacted_safe_env_map)
```

注入规则：

- 只注入 payload 中传入的变量。
- 不把变量写入员工 workspace 文件。
- 不把变量写入 Provider prompt。
- 不在 Runtime logs、Provider event、run snapshot、error message 中输出明文。

## 9. Console 体验

### 9.1 技能管理

技能上传/编辑界面增加“运行依赖”区域：

- CLI 工具，多值输入，例如 `gh`、`kubectl`。
- 环境变量名，多值输入，例如 `GH_TOKEN`。

不新增技能类别。技能卡片和详情页展示依赖摘要。

### 9.2 数字员工创建和详情

创建数字员工时增加“环境变量”配置区：

- 名称
- 值
- 敏感值开关，默认开启

详情页展示：

```text
GH_TOKEN    已配置    fp:a13f09c2    2026-06-22 10:00
```

详情页支持：

- 新增变量
- 替换变量值
- 禁用变量
- 删除变量

普通详情页不显示明文，不显示完整密文。

### 9.3 技能绑定状态

员工技能列表展示依赖状态：

- 可加载
- 缺少 Runtime 工具：列出工具名
- 缺少员工环境变量：列出 env 名
- 等待 Runtime 上报

错误提示需要指向可操作路径：

- 缺工具：提示到对应 Runtime 节点安装 CLI 并加入 probe list。
- 缺 env：提示到数字员工环境变量区新增变量。

### 9.4 Runtime 节点详情

Runtime 节点能力页展示 `tool` capability：

```text
gh        available    /usr/bin/gh
kubectl   missing
```

不提供安装按钮。

## 10. API 与权限

新增或扩展 API：

- Skill 上传/编辑请求支持 `runtime_dependencies`。
- Skill 列表/详情响应返回 `runtime_dependencies`。
- 数字员工创建请求支持初始环境变量。
- 数字员工详情响应返回环境变量摘要。
- 数字员工环境变量新增、替换、禁用、删除接口。
- 员工技能绑定响应返回 runtime dependency status。
- 管理员 CLI 或审计接口支持按员工和变量名查看明文。

权限原则：

- 创建/编辑员工环境变量需要员工配置写权限。
- 查看环境变量摘要需要员工详情读权限。
- 查看明文需要管理员级权限和审计原因。
- Runtime Agent 不拥有解密权限。

## 11. 错误处理

- Control Plane 启动时缺少加密密钥：服务启动失败，避免写入不可解密或明文数据。
- `SUPERTEAM_ENV_ENCRYPTION_ACTIVE_KEY_ID` 不在 key 列表中：服务启动失败。
- 保存 env 时加密失败：请求失败，不写入半成品。
- 解密 env 失败：启动任务失败，错误类型为 `employee_env_decryption_failed`，不输出明文或密文。
- Runtime payload env 名不合法：Runtime 拒绝命令，并回写命令失败。
- Provider 启动失败：沿用现有 provider failure path，但错误输出需要经过 env redact。
- 技能依赖缺失：任务启动前失败，不下发 Runtime command。

## 12. 测试设计

Control Plane：

- Skill runtime dependencies 解析、校验、规范化、响应。
- 员工环境变量新增、替换、禁用、删除。
- 环境变量值只以密文落库，普通响应不返回明文。
- key id 不匹配、缺 key、解密失败的错误路径。
- 管理员解密接口或 CLI 写审计记录。
- 技能依赖评估：loadable、missing_tools、missing_env、pending_runtime。
- run preflight 在依赖缺失时拒绝，在满足时填充 skills 和 environment payload。

Runtime Agent：

- tool probe 从 PATH 找到工具并上报 `capability_type=tool`。
- 缺失工具上报 `available=false`。
- `RuntimeSessionCommandPayload` 正确解析 environment。
- `RunSpec` 和 `ProviderRequest` 保留 environment。
- Claude、Codex、OpenCode command 都注入 env。
- Runtime 日志和错误路径不输出敏感值。

Web：

- Skill 上传/编辑可以填写运行依赖。
- 员工创建/详情可以配置环境变量但不显示明文。
- 员工技能绑定展示依赖缺失状态。
- Runtime 节点详情展示 tool capabilities。

端到端 smoke：

- 配置 Runtime probe `gh`。
- 创建需要 `gh` 和 `GH_TOKEN` 的技能。
- 给员工配置 `GH_TOKEN`。
- 绑定技能后显示可加载。
- 启动一次真实 Runtime/Provider smoke，确认 Provider 子进程能读取 `GH_TOKEN`，同时日志不出现该值。

## 13. 实施顺序

1. Control Plane 增加加密配置、env 加解密服务和员工环境变量表。
2. Skill domain/API/Web 增加 runtime dependencies。
3. Runtime Agent 增加 tool probe capability。
4. Control Plane 增加依赖评估和员工技能加载状态响应。
5. Run preflight 接入依赖校验、技能 payload 填充和 env 解密 payload。
6. Runtime payload、RunSpec、ProviderRequest 和三类 Provider adapter 接入 env 注入。
7. Console 补齐技能依赖、员工环境变量和 Runtime tool capability 展示。
8. 补齐测试与真实 smoke 验证。

## 14. 安全边界

本阶段的安全边界是：

- DB 泄露不能单独解密员工环境变量。
- 普通 Web API 永不返回变量明文。
- Runtime 只在单次任务 payload 中接收必要明文，不持久化解密密钥。
- Provider 只通过进程环境变量获得值，不通过 prompt 或文件获得值。
- 管理员查看明文必须通过受控后端路径并写审计。

这个设计不等价于完整企业级 Secret Manager。后续如果进入客户侧生产部署或更高合规要求，应把 `SUPERTEAM_ENV_ENCRYPTION_KEYS` 替换为 KMS/Vault envelope encryption，并增加密钥轮换任务、访问审批和异常访问告警。
