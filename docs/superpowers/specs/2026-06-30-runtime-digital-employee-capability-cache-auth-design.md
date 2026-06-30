# Runtime 数字员工能力缓存与 Provider 认证边界 Spec

> 日期：2026-06-30
> 状态：待评审
> 锚定：本文展开内环 spec（`2026-06-29-project-code-workspace-runtime-affinity-design.md`）§10 待决策项 #3，并修正 §4 中把员工能力目录误当 Provider 认证 home 的倾向。
> 决策：Provider 认证默认复用服务器/宿主统一配置；`digital_employee_id` 只作为 Runtime 本地能力缓存命名空间；员工能力缓存承载 skills/MCP/context，不承载默认 Provider auth。

## 0. 为什么需要这份

当前外环实现已经把项目 worktree、Runtime attestation、预算熔断串到真实 Provider 路径上。真实 smoke 暴露出一个边界问题：如果 Runtime 为 Codex/OpenCode 默认设置员工级 `CODEX_HOME` / `OPENCODE_CONFIG_DIR`，就会把 Provider 登录态也切到员工缓存目录。服务器统一配置里的认证不会被复用，最终可能出现上游 `401 Unauthorized`，并且 Claude Code、OpenCode、Codex 的支持方式还不一致。

用户期望的模型是：数字员工的能力来自平台数据库和对象存储，Runtime 在执行节点按需缓存和物化；Provider 认证则复用这台服务器统一配置好的 Provider 登录态。员工能力缓存不是认证隔离机制，不能被误解释成 Provider auth home。

## 1. 核心决策

1. **Provider auth 默认是宿主级**：Runtime 进程所在服务器已经配置好的 Codex / Claude Code / OpenCode 认证是默认来源。Runtime 不为每个数字员工默认改写认证 home。
2. **员工能力缓存是能力素材缓存**：缓存 skills、MCP 配置模板、上下文切片、工具说明、策略快照等可从控制平面重建的数据，不缓存默认 Provider 登录态。
3. **`digital_employee_id` 是全局唯一业务键，也是本地缓存 namespace**：Runtime 本地目录以 `employees/{digital_employee_id}` 分区；平台必须保证 ID 全局唯一。当前数据库基础表已使用 `digital_employees.id UUID PRIMARY KEY DEFAULT gen_random_uuid()`，天然给出全局唯一主键约束。
4. **正确性由版本与哈希保证，不由 TTL 保证**：TTL 只是性能策略；是否需要刷新以 `manifest_version` 以及 item 级 `revision_id` / `content_hash` 为准。
5. **外环和未来内环都只消费能力 manifest 引用**：派工单、attestation、verdict 可以记录使用了哪个 `capability_manifest_version`，但不能把 `agent_home_dir` 当成 Provider auth home。

## 2. 非目标

- 不在本 spec 内设计完整沙箱、密钥防外泄、进程隔离或不可篡改 attestations；这些仍属于内外环共同 gap #2。
- 不把能力缓存变成项目代码 worktree。项目代码仍由内环的 `repos/` 与 `workspaces/` 管理。
- 不要求第一阶段实现员工级 Provider 独立账号。未来可以扩展，但默认不是它。
- 不把 MCP 服务放到 Runtime 本地托管。MCP 仍按既有产品方向作为控制平面管理的 HTTP 能力、绑定与投影。

## 3. Runtime 磁盘布局

目标布局：

```text
{data_root}/
├── employees/{digital_employee_id}/
│   ├── manifest.version.json
│   ├── manifest.{manifest_version}.json
│   ├── skills/{skill_key}/{revision_id}/...
│   ├── mcp/{server_key}/{revision_id}/...
│   ├── context/{content_hash}/...
│   ├── overlays/{provider}/{manifest_version}/...
│   └── locks/
├── repos/{project_id}/...
└── workspaces/{project_id}/{task_id}/{attempt_id}/...
```

目录含义：

- `employees/{digital_employee_id}`：Runtime 对数字员工能力素材的本地缓存，来源是控制平面 DB 与对象存储。
- `manifest.version.json`：只存当前本地版本指针、过期时间、校验状态和最近刷新时间，用于快速判断是否命中。
- `manifest.{manifest_version}.json`：完整能力清单，用于执行物化和 attestation 记录。
- `overlays/{provider}/...`：按 Provider 生成的只读或半只读投影文件，如 MCP config、skills symlink 入口、上下文包索引。
- `workspaces/...`：任务工作区，Provider 的 CWD 或 `--dir` 指向这里，而不是员工缓存目录。

## 4. Capability Manifest

控制平面给 Runtime 的能力 manifest 是员工能力的版本化快照。最小结构：

```json
{
  "digital_employee_id": "uuid",
  "manifest_version": "2026-06-30T10:22:31Z-00042",
  "generated_at": "2026-06-30T10:22:31Z",
  "expires_at": "2026-06-30T10:32:31Z",
  "items": [
    {
      "kind": "skill",
      "key": "code-review",
      "revision_id": "rev-17",
      "content_hash": "sha256:...",
      "object_ref": "s3://bucket/skills/code-review/rev-17.tgz",
      "materialization_mode": "symlink",
      "target_provider": ["codex", "claude-code", "opencode"]
    },
    {
      "kind": "mcp",
      "key": "github-http",
      "revision_id": "rev-4",
      "content_hash": "sha256:...",
      "object_ref": "db:mcp_servers/github-http/rev-4",
      "materialization_mode": "render_config",
      "target_provider": ["codex", "claude-code", "opencode"]
    },
    {
      "kind": "context",
      "key": "project-policy-slice",
      "revision_id": "rev-9",
      "content_hash": "sha256:...",
      "object_ref": "s3://bucket/context/policy/rev-9.json",
      "materialization_mode": "copy"
    }
  ]
}
```

约束：

- `manifest_version` 是员工能力快照的全局版本，变更任何 item 后必须变化。
- `revision_id` 表示平台业务版本；`content_hash` 表示下载和本地缓存校验值。两者都要记录。
- `object_ref` 只允许引用 DB 或对象存储中的能力素材，不直接引用 Provider 本地 auth 文件。
- MCP 配置文件中只写 endpoint、工具声明、策略和环境变量名；真实 secret 由 Runtime 在执行时从授权边界取值，不能落到可被长期复用的缓存文件里。

## 5. 刷新算法

Runtime 执行前按如下顺序处理员工能力缓存：

1. 读取 `{data_root}/employees/{digital_employee_id}/manifest.version.json`。
2. 如果文件存在、未过 TTL，且本地 `manifest_version` 与控制平面快速版本查询一致，直接命中缓存。
3. 如果 TTL 到期或版本不一致，向控制平面获取完整 manifest。
4. 对每个 item 按 `(kind, key, revision_id, content_hash)` 做 diff，只下载缺失或 hash 不匹配的 item。
5. 下载到临时目录，校验 `content_hash` 后再原子 rename 到目标目录。
6. 在员工级 lock 下更新 `manifest.{version}.json` 和 `manifest.version.json`，避免并发任务重复下载或看到半写入状态。
7. 后台按 LRU / 版本保留策略清理旧 item；仍被运行中 attempt 引用的版本不得删除。

TTL 语义：

- TTL 命中只说明可以跳过远端完整 manifest 拉取，不说明内容永远正确。
- TTL 到期但控制平面版本仍一致时，只更新本地 `checked_at` / `expires_at`，不重新下载 item。
- 版本不一致时必须刷新；不能因为 TTL 未到就继续使用旧 manifest，除非控制平面不可达且任务策略允许离线执行。

## 6. 任务物化

派工单应包含：

```json
{
  "digital_employee_id": "uuid",
  "capability_manifest_version": "2026-06-30T10:22:31Z-00042",
  "workspace": {
    "project_id": "uuid",
    "task_id": "uuid",
    "attempt_id": "uuid",
    "workspace_mode": "branch"
  }
}
```

Runtime 的执行物化：

- 先准备项目任务工作区：`workspaces/{project_id}/{task_id}/{attempt_id}`。
- 再把员工能力 manifest 投影进任务工作区或 Provider 配置入口。
- 对不可变大素材优先用 symlink / hardlink；对 Provider 可能写入的配置使用 copy-on-write。
- 技能目录可以从员工缓存软链到任务工作区；MCP config 可以在任务工作区生成 provider-local config，内容引用已授权的 HTTP 能力和环境变量名。
- Provider 的 CWD 必须是任务工作区；员工缓存目录不能成为默认 CWD。

## 7. Provider Adapter 认证规则

默认规则：

| Provider | 默认认证来源 | Runtime 默认行为 |
|---|---|---|
| Codex | 宿主服务器现有 Codex 配置 | 不默认设置 `CODEX_HOME` 到员工缓存目录；只设置工作目录和必要的能力投影参数 |
| Claude Code | 宿主服务器现有 Claude Code 登录态 | 可显式传入生成的 `--mcp-config`；不假设员工目录是 Claude home |
| OpenCode | 宿主服务器现有 OpenCode 配置 | 不默认设置 `OPENCODE_CONFIG_DIR` / `OPENCODE_CONFIG` 为员工认证目录；能力投影需避免覆盖宿主认证 |

未来扩展：

```text
provider_auth_mode = host | employee | explicit_credential
```

- `host`：默认。复用 Runtime 所在服务器统一 Provider auth。
- `employee`：显式启用后，才允许把员工专属 Provider auth materialize 到隔离 home；需要单独授权、审计和密钥生命周期设计。
- `explicit_credential`：由控制平面下发短期凭据引用，Runtime 注入进程环境或 provider-specific secret store；不得写入长期缓存。

当前阶段只要求实现和文档以 `host` 为默认假设。任何代码、spec、测试若写成“员工 home = Provider auth home”，都与本 spec 冲突。

## 8. 安全边界

本设计不把 Provider 登录态作为数字员工权限边界。权限边界应来自：

- 控制平面上的团队/员工/项目授权关系。
- 能力 manifest 中的 MCP allowlist、上下文切片和策略快照。
- Runtime 对任务工作区、命令、网络、环境变量、工件的执行控制。
- 外环 attestation、预算熔断、人类审批和审计。

这意味着：宿主级 Provider auth 是服务器/运营信任边界；数字员工权限是平台业务边界。两者不能混为一谈。若未来需要“每个数字员工独立 Provider 账号”，必须走 `provider_auth_mode=employee` 的显式设计，而不是偷偷通过 `CODEX_HOME` 之类环境变量实现。

## 9. 与内环、外环、意图层的一致性

- **内环**：项目代码工作区和员工能力缓存继续分离；内环只负责把 `capability_manifest_version` 对应的能力投影到任务工作区，不负责把员工缓存目录变成 Provider 认证 home。
- **外环**：attestation 应记录 `digital_employee_id`、`capability_manifest_version`、workspace commit/ref、命令与日志哈希。预算熔断与返工循环以这些证据判断，不关心 Provider auth home 的物理位置。
- **意图层**：Acceptance criteria 定义“做对了什么”；它可以要求证据来自特定 manifest 版本，但不能依赖 `CODEX_HOME`、`OPENCODE_CONFIG_DIR`、`agent_home_dir` 等物理路径。
- **未来内环实现**：下一步实现内环时，派工单和 Provider adapter 只能依赖 task workspace + capability manifest/version。`agent_home_dir` 若仍作为字段存在，语义应收窄为能力缓存或投影来源，不得暗含认证 home。

## 10. 落地分期

### Phase 1：文档与契约语义对齐

- 新增本文。
- 更新内环 spec：把“员工家目录”改成“员工能力缓存”，去掉默认 Provider auth home 假设。
- 更新外环/意图 spec：明确 attestation/verdict 记录 capability manifest version，但不编码物理 home。

### Phase 2：Runtime 能力缓存

- 增加控制平面快速 manifest version 查询与完整 manifest 获取接口。
- Runtime 实现员工级缓存目录、锁、hash 校验、原子更新和 LRU 清理。
- 派工单携带 `capability_manifest_version`，attestation 回写实际使用版本。

### Phase 3：Provider adapter auth 默认修正

- Codex adapter 不再默认把 `CODEX_HOME` 指到员工目录。
- OpenCode adapter 不再默认把 `OPENCODE_CONFIG_DIR` / `OPENCODE_CONFIG` 指到员工认证目录。
- Claude Code 保持宿主 auth，MCP config 走显式文件参数。
- 增加 `provider_auth_mode`，默认 `host`，仅显式配置时进入 `employee` 或 `explicit_credential`。

### Phase 4：员工级 Provider auth（可选）

只有当业务明确要求“每个数字员工单独 Provider 账号”时才进入此阶段。该阶段必须补齐密钥生命周期、撤销、审计、泄漏防护与沙箱策略；不能作为 Phase 2/3 的隐式副产物。

## 11. 待决策项

1. `capability_manifest_version` 的版本格式：单调序号、时间戳+序号、还是内容寻址 hash。
2. 控制平面快速版本查询是否批量化，以支持同一项目多员工并发派工。
3. Runtime 缓存清理保留几个历史 manifest 版本，以及运行中 attempt 如何 pin 版本。
4. MCP config 中 secret 的注入机制：环境变量、短期 token 文件、还是 provider-specific secret store。
5. `agent_home_dir` 字段是否重命名为 `employee_cache_dir` / `capability_cache_dir`，以避免继续误解。

## 12. 判断准则

后续实现或文档如果满足下面条件，说明与本 spec 一致：

- Provider 真实执行可以在不设置 `OPENAI_API_KEY` 环境变量、且不改写员工级 `CODEX_HOME` 的情况下复用宿主 Codex 认证。
- 同一个 `digital_employee_id` 在任意 Runtime 节点上都映射到同一能力 manifest 语义，但物理绝对路径可不同。
- 缓存命中时不访问对象存储；版本不一致时只更新变化的 item。
- Attestation 能回答“本 attempt 使用了哪个员工、哪个能力 manifest、哪个 workspace commit/ref、跑了哪些命令”。
- 外环和未来内环都不需要知道 Provider auth 文件位于哪里。
