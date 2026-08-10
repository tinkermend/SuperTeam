# Runtime Provider 原生配置管理

日期：2026-08-10 15:22（Asia/Shanghai）
修订：2026-08-10（设计复审推翻决策 #2 / #6，管理单元从「文件」降到「键」，见 §16）
状态：已定稿（决策已拍板，待实现）

## 1. 问题与目标

Runtime Agent 已纳管 Claude Code / OpenCode / Codex 的进程执行，但不管理其**框架原生配置**（供应商、模型、API 端点、认证等）。运维仍需登录节点手改 `settings.json` / `config.toml` / `opencode.json` 等，多节点时不可扩展。

本方案在平台侧提供：

1. 探测节点 PATH/binary 上是否存在上述框架；
2. 在 Console 查看、编辑**受管键**（模型、供应商、端点、认证定位）；
3. 以 read-modify-write 下发到节点，保留文件内其余内容；
4. Control Plane 落库受管键快照与整文件指纹，供离线查看与审计。

**参考来源：** 「模型/供应商配置如何组织与切换」这一点参考 [cc-switch](https://github.com/farion1231/cc-switch) 的做法，仅借鉴该点，其余产品范围不在本方案内。

本方案不改变任务派发与执行语义——CLI 仍读自己的原生文件。

**边界（承重）：** 三家框架的原生配置文件是**混合关切**的——codex 的 `config.toml` 和 opencode 的 `opencode.json` 把模型/供应商配置与 MCP server 定义放在同一份文件里，codex 还在同一文件里存 `[projects.*]` 的 sandbox 信任决策。平台已经通过能力注册表（`internal/capability/`）、项目 MCP 绑定与会话投影治理 MCP，通过 workspace 机制治理项目信任。因此本方案**只管模型/供应商/端点/认证这一组键**，不以文件为单位读写，避免开出第二条绕过既有治理的写入通道。

## 2. 已拍板决策

| # | 议题 | 决定 |
|---|------|------|
| 1 | 配置 home 范围 | **v1 仅 host**：Runtime 进程用户环境下各框架解析出的原生路径；不做 employee `agent_home` |
| 2 | 管理粒度 | **每框架键级白名单**（allowlist，仅模型/供应商/端点/认证定位）；不做文件级整份读写，不做任意路径浏览 |
| 3 | 密钥 | 详情 API **有权限可读明文**；列表/概览不含敏感键值；全量审计 |
| 4 | 写冲突 | **乐观锁：整文件 `sha256`**（节点侧 actual 为准）；不做强制备份 / atomic write |
| 5 | 离线 | 读可看快照（标非实时）；**写必须节点在线** |
| 6 | v1 形态 | **结构化字段编辑 + 只读全文预览**；不做原文编辑器、不做多供应商档案库 / 一键切换 UI |
| 7 | 与会话 overlay | 只管 **host 静态原生配置**；不管会话 MCP 物化、`CODEX_HOME` overlay 等临时路径 |
| 8 | 权限 | 与 Runtime 节点管理同级（或子集）；写操作审计 |
| 9 | 排除项 | MCP server 定义、项目信任/状态、hooks/plugin/command 等可执行面 **一律不在白名单内**（见 §5.3） |

明确**不做**：

- 数字员工 / 任务级 model 字段或执行时 `--model` 注入（既有通道可保留，本方案不依赖）；
- 平台自研「模型策略」替代原生配置语义；
- 改写 employee home、会话临时配置；
- 通过本通道读写 MCP server 定义（走能力注册表与项目 MCP 绑定）；
- 强制写前备份与 atomic rename（实现可选用简单 `write`，风险接受）。

## 3. 架构与职责

```text
Console（Runtime 节点详情）
    │  读快照 / 触发拉取 / 提交受管键变更
    ▼
Control Plane
    │  鉴权 · 审计 · 快照落库 · 在线校验
    │  下发 RuntimeCommand
    ▼
Runtime Agent
    │  detect · 解析路径 · 读受管键 · read-modify-write · 算整文件 hash · 回报
    ▼
框架原生文件（由各框架自身的解析顺序决定，见 §5.2）
```

| 层 | 职责 | 禁止 |
|----|------|------|
| Console | 框架有无、受管键表单、拉取/保存、非实时标识 | 本机直连节点、存长期密钥逻辑、原文编辑 |
| Control Plane | 权限、快照、command 编排、审计、乐观锁协调 | 解释框架业务语义、拼接节点路径、直接改客户磁盘 |
| Runtime Agent | 探测 binary、按各框架规则解析路径、按白名单读写键、算 hash、回报 | 业务策略、多 profile 切换产品逻辑、读写白名单外的键 |
| Provider CLI | 按自身逻辑加载原生配置 | — |

**真相源：** 节点磁盘上的原生文件。
**快照：** CP 中最近一次成功读/写得到的**受管键子集**与整文件指纹，可陈旧；写成功后以节点返回的新值更新；主动「从节点拉取」覆盖。CP **不保存文件全文**。

## 4. 概念模型

### 4.1 Provider 框架实例（节点维度，观测）

每个 `(runtime_node_id, provider_type)`：

- `present`：binary 可解析且（可选）health 可用
- `version`、`binary_path`
- `configs[]`：各受管配置面的元数据（见下）

可挂在现有 `runtime_capabilities`（`capability_type=provider`）的 `metadata` 扩展，或独立表；**推荐独立表存配置快照**，capability 仅保留 present/version 摘要，以免受管键值进 capability 列表路径。

### 4.2 受管配置面（读写与快照单元）

```text
RuntimeProviderNativeConfig
  tenant_id
  runtime_node_id          -- 内部 UUID
  node_id                  -- 对外 node 字符串，冗余便于查
  provider_type            -- claude-code | opencode | codex
  config_key               -- 逻辑配置面：model_profile | auth
  resolved_path            -- 节点侧解析出的实际绝对路径（展示与审计用；由节点回报，CP 不拼接）
  format                   -- json | toml
  managed_values           -- JSONB，仅白名单内的键值对；敏感键的值加密存储（见 §9）
  file_content_hash        -- sha256(整文件 utf8)；乐观锁与漂移比对
  exists_on_node           -- 上次探测/读写时文件是否存在
  manageable               -- false 表示该平台该面不可经文件管理（如 macOS Keychain / codex keyring）
  unmanageable_reason      -- manageable=false 时的原因码
  source                   -- pulled | pushed
  node_mtime               -- 可选，节点侧 mtime
  snapshot_at
  last_pulled_at
  last_pushed_at
  last_pushed_by           -- user id
  updated_at
```

唯一键：`(tenant_id, runtime_node_id, provider_type, config_key)`。

**不存全文。** `file_content_hash` 只用于乐观锁与「节点已漂移」提示，不用于还原内容。

### 4.3 审计

每次 pull（可选采样）与 **每次 push** 写 `runtime_events`（或专用 audit）：

- actor、node、provider_type、config_key
- 操作 `provider_native_config_pull` / `provider_native_config_push`
- 变更的**键名列表**与前后 `file_content_hash`
- 结果 success/failure、错误摘要
- **不**把任何受管键的值写入事件 payload（敏感与否一律不写；只记键名、值长度、hash）

> `runtime_events.event_type` 与 `source` 目前是封闭 CHECK 约束（`migrations/007_runtime_events_overview.sql`，13 个 event_type / 5 个 source），新增事件类型需配套迁移放宽约束。

## 5. 受管键白名单（v1）

实现集中在 Runtime 的 `ProviderNativeConfigAdapter`，版本漂移只改 adapter。

### 5.1 白名单（allowlist 语义）

**只有下表列出的键可读可写。** 框架新版本引入的新键默认**不可管理**，需显式加入白名单后才开放——不采用 denylist，避免上游新增键静默进入可写面。

| provider_type | config_key | 文件 | 受管键 | format |
|---------------|-----------|------|--------|--------|
| `claude-code` | `model_profile` | `settings.json` | `model`、`fallbackModel`、`apiKeyHelper`、`env` 的受限子键（见 §5.4） | json |
| `claude-code` | `auth` | — | 见 §5.5，v1 不可写 | — |
| `codex` | `model_profile` | `config.toml` | `model`、`model_provider`、`model_providers.*`（`name`/`base_url`/`wire_api`/`env_key`/`requires_openai_auth`/`experimental_bearer_token`/`query_params`/`http_headers`） | toml |
| `codex` | `auth` | `auth.json` | 全键（该文件语义单一），受 §5.5 可管理性判定约束 | json |
| `opencode` | `model_profile` | `opencode.json` | `model`、`small_model`、`provider.*` | json |
| `opencode` | `auth` | `auth.json` | 全键（该文件语义单一） | json |

### 5.2 路径解析：一律由节点求值，CP 不拼接

各框架的配置根由自身环境变量决定，写死 `$HOME` 相对路径在设置了 XDG / 框架专有变量的节点上会读错文件。adapter 必须按下列顺序在节点侧求值，并把结果作为 `resolved_path` 回报：

| provider_type | 解析顺序 |
|---------------|----------|
| `claude-code` | `~/.claude/settings.json`（`~` = Runtime 进程用户 home） |
| `codex` | `$CODEX_HOME`（默认 `~/.codex`）→ `config.toml` / `auth.json` |
| `opencode` config | `$OPENCODE_CONFIG`（**直接指向配置文件**）→ `$OPENCODE_CONFIG_DIR/opencode.json` → `$XDG_CONFIG_HOME/opencode/opencode.json`（默认 `~/.config/opencode/opencode.json`） |
| `opencode` auth | `$XDG_DATA_HOME/opencode/auth.json`（默认 `~/.local/share/opencode/auth.json`） |

规则：

- 解析结果必须落在该框架的配置根内；任何 path traversal、绝对路径注入拒绝。
- 文件不存在：pull 返回 `exists_on_node=false`、`managed_values={}`、`file_content_hash` 为空内容 hash；push 可 **创建** 父目录与文件后写入（创建行为记审计）。
- 不做目录列举、不做全文回传。

### 5.3 显式排除（不在白名单内，且为何）

| 键 | 所在 | 排除理由 |
|----|------|----------|
| `[mcp_servers.*]` | codex `config.toml` | MCP 由能力注册表 + 项目 MCP 绑定治理（凭据加密、授权、审计）。经本通道手写会绕过全部治理，且 `merge_codex_config` 会把 host 值合进每次会话 overlay，直接进入真实执行。 |
| `mcp` | opencode `opencode.json` | 同上 |
| `[projects."<path>"]` | codex `config.toml` | sandbox 信任决策。可写 = 在任意 runtime 节点把任意路径标 trusted，属权限提升面。 |
| hooks / plugin / command / formatter / lsp / permission | 各框架 | 均可导致节点上执行代码或放宽权限，超出「模型配置管理」范围。 |
| `~/.claude.json` 全文 | claude-code | 官方明示由 Claude Code 自身管理、不供手改；含 `mcpServers`、`oauthAccount`、逐项目状态与大量缓存（实测约 99KB）。MCP 见上，账号身份不由本方案接管。 |

### 5.4 `claude-code` 的 `env` 子键必须再收窄

`settings.json.env` 会注入到 Claude Code 进程环境。允许写任意键 = 向该节点上每一次 provider 执行注入任意环境变量。因此 `env` 只开放供应商切换所需的具名子键：

```
ANTHROPIC_BASE_URL
ANTHROPIC_AUTH_TOKEN
ANTHROPIC_API_KEY
ANTHROPIC_MODEL
ANTHROPIC_SMALL_FAST_MODEL
```

其余 `env` 子键读时不返回、写时拒绝（`validation_error`），且**不得删除**白名单外的既有子键。

### 5.5 认证面的可管理性判定（平台/配置相关，非恒定文件）

认证不是恒定的「一个文件」，adapter 必须先判定该节点上是否可经文件管理，并回报 `manageable` / `unmanageable_reason`：

| provider_type | 判定 |
|---------------|------|
| `claude-code` | **macOS：凭据在系统钥匙串**（generic password，service `Claude Code-credentials`），无 `.credentials.json` → `manageable=false`，原因 `platform_keychain`。**Linux：** `~/.claude/.credentials.json`。**v1 两平台均不可写**：该文件承载 OAuth 会话且有刷新语义，覆盖会破坏登录态。供应商/端点切换走 `model_profile` 的 `env` 子键。 |
| `codex` | 先读 `config.toml` 的 `cli_auth_credentials_store`：`file`（默认）→ `auth.json` 可管理；`keyring` / `auto` 命中钥匙串 → `manageable=false`，原因 `credentials_store_keyring`。 |
| `opencode` | `auth.json`（§5.2 路径）可管理。 |

`manageable=false` 时 Console 显示原因并禁用编辑，不得回退到「创建文件」——那会写出一个框架不读的文件，产生「已配置」的假象。

## 6. Runtime 行为

### 6.1 探测（已有能力延伸）

心跳 / enroll 时已有 provider capability。扩展：

- `present` + version（已有）
- `native_configs`: `[{ config_key, resolved_path, format, exists, manageable, file_content_hash? }]`
  - 心跳只报 **exists + manageable + hash**（不报任何键值），避免配置值进高频通道；
  - 键值仅走显式 pull command。

### 6.2 Command（经现有 RuntimeCommand / WS 通道）

新增两类 command type（名称在实现时落入契约枚举，并同步 Rust 侧 `RuntimeCommandType`）。

**`read_provider_native_config`**

```json
{
  "provider_type": "codex",
  "config_key": "model_profile"
}
```

成功 receipt：

```json
{
  "provider_type": "codex",
  "config_key": "model_profile",
  "resolved_path": "/Users/agent/.codex/config.toml",
  "format": "toml",
  "exists": true,
  "manageable": true,
  "managed_values": {
    "model": "gpt-5.6",
    "model_provider": "custom",
    "model_providers.custom.base_url": "https://example.internal/v1",
    "model_providers.custom.wire_api": "responses"
  },
  "file_content_hash": "sha256:...",
  "node_mtime": "RFC3339-optional"
}
```

**不回传文件全文。** `managed_values` 只含 §5.1 白名单内实际存在的键。

**`write_provider_native_config`**

```json
{
  "provider_type": "codex",
  "config_key": "model_profile",
  "values": {
    "model": "gpt-5.6",
    "model_providers.custom.base_url": "https://new.internal/v1",
    "model_providers.custom.query_params": null
  },
  "expected_file_content_hash": "sha256:..."
}
```

语义：

1. 解析路径（§5.2）；越权、白名单外键、`env` 子键越界 → 失败（`validation_error`）。
2. 判定 `manageable`（§5.5）；不可管理 → 失败（`unmanageable`），不创建文件。
3. 读当前文件全文（不存在视为 empty），算 `actual_hash`。
4. `expected_file_content_hash` 与 `actual_hash` 不一致 → **冲突失败**（映射 `409` / `conflict`），不写。
5. parse 文件；parse 失败 → 拒绝写入（`validation_error`）。
6. **read-modify-write**：只替换 `values` 中列出的键（值为 `null` 表示删除该键），**保留文件内其余全部内容**。TOML 用保留格式的编辑器（如 `toml_edit`）以尽量保住注释与键序；JSON 保留既有键序。
7. 写入（简单覆盖即可）；返回新 `file_content_hash` 与重新读出的 `managed_values`。
8. **不**要求备份文件；**不**要求 atomic rename（实现自愿优化，非契约）。

节点离线：CP 不派发或派发超时 → API 返回「节点不可用」。

**版本偏斜：** 现有 Rust 侧 `RuntimeCommandType::Unsupported(_)` 只返回 `accepted:false` 且**不写失败 receipt**（`executor.rs:273`），CP 的 `WaitForRuntimeCommandCompletion` 会轮询到调用方超时。实现时必须二选一：pull/push 前用 capability 上报的 `native_configs` 存在性做前置判断，或让 `Unsupported` 写 failed receipt。

**Receipt 泄露面：** 终态 writeback 的 `Result` 会被 `terminalReceiptResult` 持久化进 `runtime_command_receipts.result`（`run_writeback.go:260,269 / :364,369`），而现有脱敏 `redactRuntimeEventSecrets` 是**按 key 名**屏蔽（`pg_run_repository.go:992-1004`）。因此 read/write receipt 的 `Result` **只允许携带** `file_content_hash`、`exists`、`manageable`、`resolved_path` 与变更键名；`managed_values` 必须走独立的加密写入路径，不得进 receipt result。另注：run-less 命令的 writeback 分支目前硬编码 `receipt.ResourceType == "project_workspace"`（`run_writeback.go:253-283`），新增 resource type 需同步扩此分支。

### 6.3 与执行链路隔离

- `start_session` / MCP 物化 / project overlay **不**读取本方案的 CP 快照。
- 本方案写的是 host 静态文件；会话期 overlay 仍按现有逻辑。
- **已知不成立的场景（须在 UI 标注）：** opencode 在存在 MCP server 时，runtime 会把 `OPENCODE_CONFIG` 指向 session 投影文件（`project_session.rs:125-131`），而该文件由 `render_opencode_task_mcp_config` 生成，是**只含 `mcp` 键的全新根对象**，不 merge host `opencode.json`（`mcp_config.rs:582-586`）。因此这类任务不消费 host 的 `model`/`provider`。codex 侧走的是 `merge_codex_config`，host 值会被合入，不受此影响。落地前需二选一：把 opencode 投影也改成 merge，或在 Console 明确标注「带 MCP 的 opencode 任务不使用本页配置」。

## 7. Control Plane API（Console）

均需 Runtime 管理权限；写操作记审计。路径归入现有 `/api/v1/runtime/...`（现有约定为 `/api/v1/runtime/nodes/{nodeId}`，camelCase 参数）。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/runtime/nodes/{nodeId}/provider-native-configs` | 列表：各 provider + 配置面元数据；**不含 `managed_values`**；含 snapshot 时间、hash、exists、manageable、在线状态 |
| POST | `/api/v1/runtime/nodes/{nodeId}/provider-native-configs/pull` | body: `provider_type`, `config_key`；在线则 command 读节点并 **upsert 快照**；返回含 `managed_values` 的详情 |
| GET | `/api/v1/runtime/nodes/{nodeId}/provider-native-configs/{providerType}/{configKey}` | 读**快照**（可陈旧）；标 `stale_hint` / `snapshot_at`；含 `managed_values`（敏感键解密后返回） |
| PUT | `/api/v1/runtime/nodes/{nodeId}/provider-native-configs/{providerType}/{configKey}` | body: `values`, `expected_file_content_hash`；在线 write command；成功更新快照 `source=pushed` |

错误：

- `404` 节点不存在
- `409` hash 冲突（节点内容已变）
- `422` 白名单外键 / `env` 子键越界 / 文件 parse 失败 / 非法 `config_key`
- `409` `manageable=false`（该平台该面不可经文件管理，带 `unmanageable_reason`）
- `503` / `409` 节点离线不可写（与现有 runtime 在线判定一致）
- `403` 无权限

列表与详情分离，避免概览接口泄露敏感键值。

## 8. 乐观锁流程

```text
用户打开配置面
  → 优先 pull 最新（UI 默认）或打开快照
  → 前端持有 file_content_hash_H
用户保存
  → PUT values + expected_file_content_hash=H
  → Runtime: 若 actual ≠ H → 409
  → UI: 提示「节点配置已变更，请重新拉取」
```

乐观锁用**整文件** hash 而非受管键子集的 hash：文件里的非受管内容（含 MCP、projects）被别人改过时也必须让本次写落空后重拉，否则 read-modify-write 会基于过期底本重写整份文件。

CP 在派发 write 前可用快照 hash 做一次快检，但仍以 **节点 actual hash** 为最终裁决。

## 9. 存储与安全

- `managed_values` 中标记为敏感的键（`env.ANTHROPIC_AUTH_TOKEN`、`env.ANTHROPIC_API_KEY`、`model_providers.*.experimental_bearer_token`、`auth` 面全部键）**必须用仓库既有的 `capability.AESGCMCredentialSealer`（`internal/capability/crypto.go`）加密列存**——不留「否则收敛 DB 权限」的退路。
- API 访问日志、`runtime_events`、`runtime_command_receipts`：**禁止**出现任何受管键的值（见 §6.2 receipt 约束）。
- 传输：既有 Console session + Runtime session token。
- 不向第三方同步配置内容。

## 10. Console UX

入口：**Runtime 节点详情**（`apps/web/src/features/runtime/index.tsx` 已有 `MasterDetailLayout`，节点详情面板可直接扩展）。

1. **框架卡片**：claude-code / opencode / codex — 有 / 无、version。
2. 展开 **配置面列表**（`model_profile` / `auth`）：exists、manageable、hash 短显、快照时间、来源 pulled/pushed。
3. 操作：
   - **从节点拉取** → 打开字段表单（最新）
   - **编辑受管字段** → 保存下发
4. 表单：按 §5.1 白名单渲染具名字段（模型、供应商、base_url、wire_api、token 等），敏感字段掩码显示 + 显式「显示」动作；**不提供原文编辑器**。可提供只读的「当前受管键 JSON 预览」。
5. 明确提示「本页只管模型与供应商配置；MCP server 请到能力注册表 / 项目 MCP 绑定配置」。
6. `manageable=false`：显示原因（如「该节点凭据存于系统钥匙串，平台不接管」）并禁用编辑。
7. 离线：保存禁用；可浏览快照并角标「非实时」。
8. 409：提示重新拉取后再改（受管字段表单可自动合并，冲突键高亮）。

文案中文；新增状态词进 `apps/web/src/lib/status-labels.ts`。

## 11. 权限

- 读列表/快照、pull、push：具备该租户 **Runtime 节点管理** 权限的人类用户（与节点审批/总览管理对齐；OpenFGA 关系名实现时复用或新增 `can_manage_runtime_config` 一类，避免全员可读敏感键）。
- Runtime agent 自身 token：仅能执行针对本 `node_id` 的 read/write command，不能读他节点快照 API。

## 12. 实现切分建议

| 阶段 | 交付 | 验证 |
|------|------|------|
| **P0** | Runtime adapter：路径解析（含 env 变量）、键级白名单、`manageable` 判定、read/write（read-modify-write + 格式保留）、整文件 hash、parse 与键校验；两 command；`Unsupported` 写失败 receipt；单测 | `verify:runtime-agent` + 本地假 HOME/CODEX_HOME/XDG 文件 |
| **P1** | CP：表迁移（含放宽 `runtime_events` CHECK）、快照与敏感键加密、Console API、command 编排、审计事件、在线/409/422、run-less writeback 分支扩展 | `verify:contracts` + `verify:control-plane` + `make -C apps/control-plane migrate-validate` + 真库 |
| **P2** | Web 节点详情 UI：配置面列表 / 拉取 / 字段表单 / 保存 / 离线与冲突 / manageable 提示 | `verify:web` + 浏览器真链路 |
| **P3**（可选） | 心跳附带 exists+hash 摘要；UI 显示「与节点 hash 不一致」 | 不阻塞 P0–P2 |

契约：扩展 `contracts/control-plane/openapi.yaml`（Console API）及 Runtime command 类型；改后走 `generate:control-plane` 与 `verify:contracts`。

> **契约门禁只覆盖一半：** `verify:contracts` 目前只覆盖 `contracts/control-plane/openapi.yaml`（`scripts/verify-foundation-contracts.mjs`），新增的两个 command type 事实上活在 `apps/runtime-agent/src/controlplane/models.rs` 的 Rust 枚举里，**无自动化可依**，需人工双边核对 CP 侧常量与 Rust 侧 `RuntimeCommandType` 一致。

## 13. 风险与缓解

| 风险 | 缓解 |
|------|------|
| CLI 升级导致配置路径或键名变更 | adapter 集中常量 + 按 env 解析 + 版本探测；allowlist 语义使新键默认不可写 |
| 混合关切文件被误改（MCP / projects / hooks） | 键级 allowlist + read-modify-write 保留非受管内容 + 整文件 hash 乐观锁 |
| 敏感键进库/进日志/进 receipt | 列表不含值；日志与事件禁值；receipt 只带 hash 与键名；敏感键 AES-GCM 列存 |
| host 配置与 `provider_auth_mode=employee` 任务无关 | 文档与 UI 标明「节点宿主用户配置」；employee home 为后续 epic |
| opencode 带 MCP 的任务不消费 host 配置 | §6.3：改投影为 merge，或 UI 明确标注；落地前必须二选一 |
| 认证不在文件里（Keychain / keyring） | `manageable` 判定 + UI 禁用编辑；禁止回退创建文件 |
| 无 backup 写坏文件 | parse 校验 + 键级替换 + 409 防盲写；产品接受运维自负（后续可加可选备份） |
| employee `environment` 可注入 `HOME`/`CODEX_HOME`/`XDG_*` 改变实际读取路径 | `apply_environment`（`providers/mod.rs:57-60`）无 denylist；本方案的 host 前提在该情况下失效，需在员工环境变量层加 denylist 或至少在文档记录 |

## 14. 后续非 v1

- employee `agent_home` 下同构管理
- 多 provider 档案 + 一键切换
- 写前可选备份 / atomic write
- 配置模板 / 租户级预设下发到多节点
- 白名单扩容（新键需显式评审后加入）

## 15. 验收标准（实现完成时）

1. 节点安装并 PATH 可见的框架在详情页显示「有」；未安装显示「无」，无编辑入口或禁用。
2. 对受管键：拉取值与节点磁盘一致；编辑下发后节点文件对应键与提交值一致；快照更新。
3. **非受管内容不被改动**：在含 `[mcp_servers.*]`、`[projects.*]`、注释的 `config.toml` 上下发一次 `model` 变更，事后 diff 证明仅目标键变化，MCP 段、projects 段与注释原样保留。
4. 白名单外键（如 `mcp_servers.foo`、`projects."/x"`、`env.PATH`）在 API 与 Runtime 两侧均被拒（422），磁盘不变。
5. 使用过期 `expected_file_content_hash` 保存 → 409，磁盘不被覆盖；他人改动文件的**非受管部分**同样触发 409。
6. 非法 JSON/TOML → 422，磁盘不变。
7. macOS 节点的 `claude-code` / `auth` 面显示 `manageable=false` 且原因为钥匙串；尝试写入返回 409，**不产生** `~/.claude/.credentials.json`。
8. codex 节点 `cli_auth_credentials_store=keyring` 时 `auth` 面同样不可管理且不创建文件。
9. 节点停机 → 可打开快照只读，保存失败。
10. 无权限用户无法读敏感键 / 写。
11. 审计可见 push 操作者、对象与变更键名；事件与 `runtime_command_receipts.result` 中无任何键值。
12. 执行一次真实 provider 任务确认仍依赖节点原生配置（claude-code 与 codex 两条腿；opencode 按 §6.3 结论标注或验证）。

---

## 16. 修订记录

**2026-08-10 设计复审：推翻决策 #2 与 #6。**

原方案以「文件」为管理单元、v1 做单文件原文编辑下发。复审核对三家框架官方定义与实测节点后推翻：

- `~/.claude/settings.json` 只含 `model`/`env`/`permissions`/`hooks` 等，**不含 MCP**；Claude Code 的 MCP 在 `~/.claude.json`（user scope）与项目根 `.mcp.json`。
- `~/.codex/config.toml` **同时**含 `model`/`model_provider`/`[model_providers.*]`、`[mcp_servers.*]` 与 `[projects."<path>"].trust_level`。
- `~/.config/opencode/opencode.json` **同时**含 `provider`/`model`/`small_model` 与 `mcp`。

即三家里只有 Claude Code 把模型配置与 MCP 分了文件。以文件为单位读写会给运维开出一条绕过能力注册表 / 项目 MCP 绑定 / 凭据加密 / 授权的 MCP 第二写入通道，并在 codex 上额外暴露 sandbox 信任的提升面。故管理单元降到「键」，并新增决策 #9。

同批修正的事实错误：

- 原 §5 的 `claude-code` / `credentials` 行（`~/.claude/.credentials.json`）在 macOS 不成立——凭据在系统钥匙串，该路径不存在；原「不存在则 `exists_on_node=false`」的兜底会导致 push 创建一个 Claude Code 不读的文件。改为 §5.5 的 `manageable` 判定。
- 原「路径均相对于 `$HOME`」对 XDG / 框架专有变量不成立，改为 §5.2 由节点按各框架解析顺序求值。
- opencode auth 路径补齐为 `$XDG_DATA_HOME/opencode/auth.json`（默认 `~/.local/share/opencode/auth.json`）。

## 附录 A：与现有对象关系

| 现有 | 关系 |
|------|------|
| `runtime_capabilities` | 继续表达 provider 有无/健康；可附 hash 与 manageable 摘要；**不**存受管键值 |
| `RuntimeCommand` | 承载 read/write 下发；同步形状可复用 `RuntimeWorkspaceCommanderAdapter`（dispatch + `WaitForRuntimeCommandCompletion` + 有界 timeout） |
| `internal/capability/` 能力注册表 | MCP server 的唯一治理入口；本方案显式不碰 MCP 键 |
| 项目 MCP 绑定 / 会话 MCP 投影 | 会话期 overlay，与本方案并存；本方案不写会话路径 |
| `provider_auth_mode` / `agent_home_dir` | 本方案 v1 不读写；执行路径不变 |
| 数字员工 `provider_type` | 不变；仍只表达用哪类框架，不表达模型配置 |
| Project placement | 不变；任务落到节点后使用该节点 host 上已写好的原生配置 |

## 附录 B：决策记录摘要

- 范围：模型/供应商原生配置的多节点远程管理；其中配置的组织与切换方式参考 cc-switch 的同名做法，不引入其余产品范围。
- 用户确认（初版）：快照落库；写回不强制 backup/atomic；host-only；明文详情+审计；乐观 hash；离线禁写；overlay 不管；权限按 Runtime 管理。
- 用户确认（2026-08-10 复审）：管理单元改为键级白名单；MCP 与项目信任键排除；认证按平台判可管理性。
