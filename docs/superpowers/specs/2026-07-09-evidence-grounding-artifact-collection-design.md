# 证据地基：Artifact 采集、去重与可取回

- 状态：待评审
- 日期：2026-07-09
- 范围：让「证据」在 SuperTeam 里真正成立——数字员工的产出物被采集、内容寻址存储、可通过平台 API 取回，并与 evidence 读模型接上。
- 不在范围：acceptance criterion、verification 判据、人类待办自动生成、验收就绪判据。见文末「后续 spec」。

## 1. 问题

真实 E2E 已经跑通「需求 → 协调 → 路由 → 任务 → runtime → claude-code 执行 → 回写」，但项目的证据与验收两段是空的。对生产库（`superteam`，2026-07-09）的实测：

| 项目 | status | tasks | done | evidence | artifacts | acceptance |
|---|---|---|---|---|---|---|
| Plan B Multi-Node E2E 20260709013558 | running | 2 | 1 | 2 | 0 | 0 |
| E2E Clean Project 20260708153057 | acceptance | 8 | 5 | 10 | 0 | 0 |
| 系统健康分析项目-150215-fix | archived | 3 | 3 | 6 | 0 | 1 |

结论与原始判断不同：**证据不是「没写」，而是「写了但不构成证据」；验收链路完整存在，只是到不了。artifacts 才是真正一行都没有的那段。**

### 1.1 证据存在，但不可核验

`project_evidence_refs` 的真实行：

```
evidence_type       | runtime_command
source_ref          | runtime-command://cmd-c4a5f62f-3bbd-4b32-a95d-4b8e5dc2e907
verification_status | (空串)
summary             | (空)
artifact_ref_id     | (NULL)
```

`runtime-command://cmd-xxx` 没有任何解引用路径——没有对象存储中的对象，没有 API 能取回它指向的输出。它证明「有个命令跑过」，不证明「结论成立」。

### 1.2 双写

两条独立路径物化同一份 refs，所以 evidence 行数恰好是完成任务数的两倍：

- `apps/control-plane/internal/project/pg_repository.go:3679` `extractExecutionEvidenceRefsWithQueries`，在 writeback 事务内调用（3771、3855），不设 `verification_status` → 落成空串。
- `apps/control-plane/internal/project/service.go:3336` `materializeTaskCompletionEvidence`，在事务外调用（2532、2772、2801），设 `verification_status='submitted'`。

后者的注释写着「没有它 evidence 就永远空」——添加时没有删除前者。且它的错误被吞成一条 workflow signal event，物化失败静默通过。

### 1.3 artifacts 恒空

`apps/runtime-agent/src/commands/executor.rs:1713` 从 provider 的 JSON 输出里解析 `artifact_refs`。与 `evidence_refs`（1707-1708 有兜底 push）不同，artifact 没有任何兜底。claude-code 从不主动输出该字段，所以恒为空数组；即使输出了也只是一个字符串 ref，背后没有内容。

因此 `/projects/{id}/artifacts` 恒空，`project_evidence_refs.artifact_ref_id` 恒 NULL——证据与产出物之间那条链根本没有接上。

### 1.4 第三方事实源被 parser 丢弃，从未存在于任何一层

**这是本 spec 最根本的发现。**

`apps/runtime-agent/src/providers/claude.rs:29` 以 `--output-format stream-json` 运行 claude-code。它的 stdout 里确实含有 `tool_use` 与 `tool_result`——包括命令的真实退出码。但 `parse_claude_event`（`claude.rs:78`）只有三个 match 分支：

| 输入 | 处理 |
|---|---|
| `"system"` | → `SessionStarted` |
| `"assistant"` | 只取 content 中**第一个含 `text` 的 block**；同消息内的 `tool_use` block 丢弃 |
| `"result"` | → `TurnCompleted` |
| `"user"` | **没有这个分支** → `_ => Ok(None)` 丢弃。claude-code 的 `tool_result` 正是包在 `type:"user"` 消息里 |

原始 stdout 行由 `providers/mod.rs:120` 的 `BufReader::lines()` 逐行喂给 parser，返回 `None` 即就地丢弃，**从不落盘**。

因此：

- `{log_dir}/{run_id}/events.jsonl`（`runs.rs:278`）保存的是**解析后**的 `ProviderEvent` 流，同样不含 tool 事件。
- 控制平面的 `execution_ledger_events` 中 97 条 `provider.event` 只有四种：`session_started`(18) / `text_delta`(43) / `turn_completed`(18) / `run_completed`(18)。**`tool_use`、`tool_result` 零条。**
- `events.rs:24-30` 早已定义 `ProviderEvent::ToolStarted` / `ToolCompleted`，`executor.rs:1518`、`1527` 早已在消费它们——**没有任何 provider adapter 生产过**。下游管道全通，源头没接。

现实后果，可在库中直接看到：某条 `run_completed` 的 `output_summary` 是模型自己写的

> 「证据：命令退出码 `0`；stdout 为 `final-sanity-20260709014431`。」

**那个 `0` 是数字员工声称的。平台从未见过真正的退出码。**

这条决定了本 spec 中「证据」的定义：

| 候选 | 由谁产生 | 是否构成证据 |
|---|---|---|
| result contract 的 `summary` | 数字员工自己 | 否，自证 |
| `text_delta` 事件（模型旁白） | 数字员工自己 | 否，自证 |
| provider 原始 stdout 中的 `tool_result` | claude-code 进程写出的字节 | **是**，数字员工无法伪造 |
| `git diff` | 产出物本身，可独立复核 | 是 |

**证据 = 不由声称者自己产生、且可独立复核的事实。** 一个只有 `conclusion.md` 的任务，即使 artifact 计数非零，证据仍然为零。

推论：**在停止丢弃 tool_result 之前，采集任何东西上传对象存储都不产生证据。**

修复 parser、捕获 tool 事件、以及由此带来的 Web 执行过程显示，构成一个独立且更靠前的问题，拆分至
`docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md`。
本 spec 依赖其产物 `raw.jsonl`（见 4.0），不再重复其设计。

### 1.5 已有但未接线的基础设施

- `apps/control-plane/internal/storage/storage.go`：`S3ObjectStore` 完整（`PutObject` / `GetObject` / `Exists` / `DeleteObject`），当前仅被 skill service 使用。
- `apps/control-plane/config/config.yaml`：`objectStore` 已配置真实的火山 TOS bucket。
- `apps/runtime-agent/src/artifacts.rs`：`ArtifactCollector::upload_directory` 存在，**零调用者**。它接收一个 `S3Client` 参数，而整个 runtime 里没有任何地方构造过 `S3Client`。同理 `skills.rs`、`commands/install_skills.rs`。全部是死代码。
- `task_artifacts` 表：0 行，字段模型（`task_id` / `run_id` / `storage_url`）与 project 侧不匹配。

结论：对象存储这层不需要从零建，且没有兼容包袱。

## 2. 目标与非目标

**目标**

1. 每个完成的项目任务至少产出一条**真证据**——由工具而非数字员工产生的 `execution_transcript`。
2. artifact 内容寻址（sha256），重复内容不重复上传。
3. 人类可通过平台 API 取回 artifact 原始内容。
4. `/projects/{id}/evidence` 每任务每 ref 恰好一行（消除双写），且 `artifact_ref_id` 指向可取回的 artifact。
5. 数字员工的自述（`conclusion`）在数据模型上被明确标记为 `self_report` / `unverified`，不冒充证据。
6. 证据物化与任务完成状态原子——不存在「completed 但无证据」的行。

**非目标**

- 不定义「什么算做对了」（acceptance criterion）。
- 不产生 verification 判据，不自动生成人类待办。
- 不改动验收就绪判据 `AreAllProjectDemandsTerminal`。
- 不做 artifact 版本树、diff 可视化、全文检索。

## 3. 架构决策

### 3.1 Runtime 不持有对象存储凭证

Control Plane 签发 presigned URL，runtime 拿 URL 直传对象存储。

理由：`CLAUDE.md` 明确 Runtime Agent 可部署在**客户侧执行机**。把 TOS 的 `secretAccessKey` 下发到那里不可接受。现有 runtime 侧 S3 死代码（`artifacts.rs` 的直连方案）正好无需兼容。

数据面（对象字节）直接走 runtime ↔ 对象存储，不经过 Control Plane；控制面（授权、元数据、读模型）走 Control Plane。这与「Control Plane 不承载执行数据面」的架构职责边界一致。

### 3.2 内容寻址：key 不承载归属

object key = `artifacts/{tenant_id}/sha256/{hex}`

**key 只回答「这是什么内容」，不回答「这是谁的」。** 归属完全由 `project_artifact_refs` 的行表达：

```
对象存储   artifacts/{tenant}/sha256/a3f9c2…     一份字节，存一次
              ▲
project_artifact_refs 行
   project_id + project_task_id + attempt_id + digital_employee_id → checksum = a3f9c2…
```

由此：

- 同一任务下多个数字员工 → 多行，同 `project_task_id`，不同 `digital_employee_id`。各自 transcript 内容不同、sha256 不同，天然是不同对象，无需文件名约定。
- 同一任务被循环重复调度 → 每次 attempt 一行，`attempt_id` 不同。（实测：`project_task_attempts` 中 task `3e0cf335-a7c3-43c0-9798-659d468d0642` 有 3 次 attempt。）
- 两次执行字节完全一致 → 同一 sha256，对象只存一份，DB 仍是两行两个 `attempt_id`。**去重发生在存储层，血缘保留在关系层。**
- 上传前 `HeadObject` 探测，命中则跳过上传；重传天然幂等。
- 租户前缀隔离；presign 时服务端强制 key 前缀属于调用方 tenant。

被否决的替代方案是层级路径 `runtime/{tenant}/{run_id}/{type}/{filename}`（即现有死代码 `artifacts.rs::upload_directory` 的方案）：相同内容重跑 N 次存 N 份；且 `run_id` 是 runtime 侧概念，控制平面读模型中没有该字段，接不上归属。

### 3.3 复用 `project_artifact_refs`，删除 `task_artifacts`

`project_artifact_refs` 已具备大部分字段：`object_ref`（存 object key）、`checksum`（存 sha256 hex）、`content_type`、`size_bytes`、`artifact_type`、`title`、`project_task_id`、`metadata`。它同时已经是 `GET /projects/{id}/artifacts` 读取的读模型。

**缺两列，本次补上**：`attempt_id`、`digital_employee_id`。没有它们就无法区分同一任务下的多员工与多次循环执行——这正是 3.2 所依赖的血缘。`digital_employee_id` 虽可由 `attempt_id` 关联 `project_task_attempts` 推出，仍做反范式冗余，与 `project_evidence_refs.submitted_by_id` 的既有做法保持一致，避免验收界面按员工过滤时的强制 join。

`task_artifacts`（0 行、遗留、模型不匹配）在本次迁移中删除，避免两个 artifact 概念长期并存。

### 3.4 提交 result 前同步上传

provider 结束 → runtime 采集 → 上传 → 把带 sha256 的 `artifact_refs` 写进 result contract → 提交 result。

Control Plane 收到 result 时内容已在对象存储中，可以在同一个 writeback 事务里直接写 DB 行。代价是大产出物会拖慢 result 提交；这个代价换取的是 3.5 的原子性。

### 3.5 证据与完成原子

证据/工件物化移进 writeback 事务。物化失败则整个 completion 回滚，任务不进 `completed`，attempt 可重试（内容寻址保证重传幂等）。

上传已在 runtime 侧完成并带重试，事务内只写 DB 行，失败概率极低。换取的性质是：**库里永远不会出现 completed 但无证据的任务**。

## 4. 组件设计

### 4.0 前置依赖：Provider Transcript 与 Tool 事件捕获

本 spec **依赖** `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md` 先行落地。

在 tool 事件被 parser 丢弃的前提下（1.4），采集任何东西上传对象存储都不产生证据。该 spec 负责：

- `ProviderParser` 签名改为返回 `Vec<ProviderEvent>`
- `parse_claude_event` 补全 `tool_use` / `tool_result` 解析，`ProviderEvent::ToolStarted` / `ToolCompleted` 扩字段
- provider 原始 stdout 逐行落 `{log_dir}/{run_id}/raw.jsonl`
- tool 事件经既有 `execution_ledger_events` + WS 通路流向 Web

本 spec 只消费其产物：`raw.jsonl` 即 4.1 中 `execution_transcript` 所采集、上传的对象。

### 4.1 Runtime：`artifacts.rs` 重写

`ArtifactCollector` 改为不持有 S3Client，而是持有 Control Plane client：

```rust
pub struct CollectedArtifact {
    pub artifact_type: String,   // execution_transcript | diff | declared | conclusion
    pub name: String,            // raw.jsonl / changes.diff / conclusion.md / 相对路径
    pub sha256: String,          // hex，脱敏后字节的哈希
    pub size_bytes: u64,
    pub content_type: String,
    pub truncated: bool,
    pub is_evidence: bool,       // conclusion 为 false，其余为 true
}
```

职责：

1. `collect(workspace_path, run_id, contract) -> Vec<(CollectedArtifact, Bytes)>`（`run_id` 用于定位 `{log_dir}/{run_id}/raw.jsonl`）
2. 对每个 artifact 调 `POST /api/v1/runtime/artifacts/presign` 换取 PUT URL（服务端返回 `already_exists: true` 时跳过上传）
3. PUT 到对象存储，带指数退避重试
4. 返回 `artifact_refs` JSON 数组供 result contract 使用

采集四类。**保证每个任务至少一条真证据（`execution_transcript`），而非仅仅至少一个 artifact**：

| artifact_type | 来源 | 条件 | 是否构成证据 |
|---|---|---|---|
| `execution_transcript` | `{log_dir}/{run_id}/raw.jsonl`（由前置 spec 产出） | 总是（provider 跑过就有） | 是 |
| `diff` | worktree 内 `git diff` 输出落成 `changes.diff` | 存在 git worktree 且有变更 | 是 |
| `declared` | `handoff_contract.artifact_globs` 按 glob 收集 | 该字段非空时 | 是 |
| `conclusion` | result contract 的 `summary` 落成 `conclusion.md` | 总是 | **否**，自证，仅便于人类阅读 |

`execution_transcript` 是本 spec 的核心采集物。它已经在磁盘上，采集成本近乎为零，而它是唯一能支撑后续 verification 判据的原材料——下一个 spec 里「`pnpm test` 退出码为 0」这类 `automated` verification，其证据指针就落在 transcript 中某条 tool_result 事件上。

`conclusion` 仍然采集并上传，但**不计入证据**：物化时其 `evidence_type` 标为 `self_report`，`verification_status` 标为 `unverified`。它让人类在验收界面能直接读到数字员工的说法，同时在数据模型上明确它不是证据。

`artifact_globs` 本期**只消费不生成**——planner 填充它是后续 spec 的事。字段缺失时该类不产出。

**限额**：单文件 10MB、单任务 50 个文件、单任务总计 200MB。超限时截断，并在该 artifact 的 `metadata.truncated=true` 标记。不静默丢弃：被完全跳过的文件名记入任务级 `metadata.skipped_files`。

transcript 常见为数百 KB 到数 MB，通常在单文件上限内。超过 10MB 时**从尾部保留**（末尾含最终 tool result 与结论），并标记 `truncated`。

### 4.1.1 Transcript 脱敏

`raw.jsonl` 可能包含 tool result 里回显的环境变量、token、密钥。上传前 runtime 按行做正则脱敏：

- 已知敏感 env 变量名的值（从 provider 进程环境快照取键名，替换其值）
- 常见凭证形态：`sk-[A-Za-z0-9]{20,}`、`ghp_*`、AWS AKIA/ASIA 前缀、`Bearer <token>`、JWT 三段式
- 替换为 `[REDACTED:{reason}]`，并在 artifact 的 `metadata.redaction_count` 记录命中次数

脱敏在计算 sha256 **之前**执行——存储的字节即脱敏后的字节，原始 transcript 不出节点。这意味着同一次执行在不同节点上的 sha256 可能不同（env 键名不同），这是可接受的：内容寻址在此处服务于去重与幂等重传，不服务于跨节点内容比对。

同时删除 `executor.rs` 中「artifact_refs 完全依赖 provider 自报」的行为：改为 provider 自报的 refs 与采集到的 refs 合并，采集结果优先（provider 自报的裸字符串 ref 无内容，仅作为 metadata 保留）。

### 4.2 Control Plane：presign 端点

```
POST /api/v1/runtime/artifacts/presign
Auth: runtime node token
Body: { sha256, size_bytes, content_type }
→ 200 { object_key, upload_url, expires_at, already_exists }
```

服务端行为：

1. 校验 `sha256` 是 64 位 hex；`size_bytes` 不超过单文件上限。
2. `object_key = artifacts/{tenant_id}/sha256/{sha256}`。**`tenant_id` 取自 node token，不出现在请求体中**——避免出现一个看起来可以被信任、实际必须被忽略的字段。
3. `Exists(object_key)` 为真 → 返回 `already_exists: true`，不签发 URL。
4. 否则签发有效期 15 分钟的 presigned PUT。

`S3ObjectStore` 需新增 `PresignPut(ctx, key, ttl)` 和 `PresignGet(ctx, key, ttl)`。

**完整性说明**：presigned PUT 无法强制客户端写入的字节确实哈希为 key 中的 sha256。本期接受这一点——runtime node token 已经是可信执行身份。物化时 Control Plane 会 `HeadObject` 确认对象存在且 `size_bytes` 匹配；哈希校验留给后续的 attestation spec（见文末）。

### 4.3 Control Plane：内容取回端点

```
GET /api/v1/artifacts/{artifactId}/content
Auth: console user
→ 302 Location: <presigned GET URL, TTL 5min>
```

按 `artifactId` 查 `project_artifact_refs`，校验调用方对该 project 有读权限，签发 presigned GET 并 302 重定向。不代理字节流，省控制平面带宽。

### 4.4 Control Plane：物化路径合一

删除 `pg_repository.go:3679` `extractExecutionEvidenceRefsWithQueries` 及其两处调用（3771、3855）。

`materializeTaskCompletionEvidence`（`service.go:3336`）改造：

1. 从 service 层移入 writeback 事务，成为 `materializeTaskCompletionEvidenceWithQueries(ctx, q, ...)`。
2. 先物化 artifacts：对每个 `artifact_ref`，`HeadObject` 确认对象存在且 `size_bytes` 匹配，然后 upsert `project_artifact_refs`，冲突键为 4.5 的 `(tenant_id, project_task_id, attempt_id, checksum)`。`attempt_id` 与 `digital_employee_id` 取自当前 attempt 上下文。
3. 再物化 evidence，按 `artifact_type` 映射：

   | artifact_type | evidence_type | verification_status |
   |---|---|---|
   | `execution_transcript` | `execution_transcript` | `submitted` |
   | `diff` | `code_change` | `submitted` |
   | `declared` | `declared_output` | `submitted` |
   | `conclusion` | `self_report` | `unverified` |

   每条 evidence 的 `artifact_ref_id` 填第 2 步写入的 `project_artifact_refs.id`。

4. 若该任务的 artifact 中不存在任何 `is_evidence=true` 的项，视为**零证据完成**，返回 error 触发回滚。数字员工不能只交一份自述就把任务标完成。
5. 任一步失败 → 返回 error → writeback 事务回滚 → 任务不进 completed。

`service.go:2532`、`2772`、`2801` 三处事务外调用点相应移除。

### 4.5 数据库迁移

新增 `apps/control-plane/internal/storage/migrations/0NN_evidence_grounding.sql`：

1. `DROP TABLE task_artifacts;`（0 行）
2. `project_artifact_refs` 加两列：`attempt_id UUID`、`digital_employee_id UUID`（均可空——项目级或人工上传的 artifact 无 attempt）。加索引 `(tenant_id, project_id, attempt_id)`。
3. `project_artifact_refs` 加**部分唯一索引**支撑 upsert 幂等：

   ```sql
   CREATE UNIQUE INDEX uq_project_artifact_refs_attempt_checksum
     ON project_artifact_refs (tenant_id, project_task_id, attempt_id, checksum)
     WHERE attempt_id IS NOT NULL AND project_task_id IS NOT NULL;
   ```

   幂等边界是 **attempt 内**：同一 attempt 的 result 重复提交（现有 `idempotency_key` 机制允许）不产生重复行；不同 attempt 即使内容字节一致也各自保留一行。**不要**把 `attempt_id` 从索引中省略——那会把多次循环执行折叠成一行，丢失血缘。

4. `project_artifact_refs.checksum` 加 `CHECK (checksum IS NULL OR checksum ~ '^[a-f0-9]{64}$')`。
5. `project_evidence_refs.verification_status` 加 `CHECK (verification_status <> '')`。迁移前先把存量空串行回填为 `'submitted'`。

存量数据（2026-07-09 实测）：`project_evidence_refs` 共 30 行，其中 16 行 `verification_status` 为空串——由 1.2 的双写路径产生，需在加 CHECK 前回填。这些历史行的 `source_ref` 保持原样，不做清理也不追溯补 artifact：它们指向的内容从未被存储过，无法重建。迁移只保证新数据正确。

### 4.6 契约变更

`contracts/control-plane/openapi.yaml`：

- 新增 `POST /runtime/artifacts/presign`
- 新增 `GET /artifacts/{artifactId}/content`
- `TaskResultContract.artifact_refs` 的元素 schema 从裸字符串扩展为对象：`{ type, name, sha256, size_bytes, content_type, truncated, is_evidence }`，同时保留字符串形式向后兼容——`executor.rs:2530` `normalized_result_ref` 已经对 `value.as_str()` 和对象两种形态分别处理。裸字符串形态的元素 `is_evidence` 默认为 `false`（无内容可核验）。

  `attempt_id` 与 `digital_employee_id` **不进 contract**：Control Plane 在 writeback 时从 `ProjectTaskAttemptRuntimeRequest` 已有的 attempt 上下文取值，不接受 runtime 自报归属。

## 5. 错误处理

| 场景 | 行为 |
|---|---|
| presign 返回 `already_exists` | 跳过上传，直接使用该 key |
| PUT 失败 | runtime 指数退避重试 3 次；仍失败则整个 result 提交失败，attempt 保留租约由 runtime 重试 |
| `execution_transcript` 超 10MB | **从尾部保留** 10MB（末尾含最终 tool result 与结论），`metadata.truncated=true` |
| 其他单文件超 10MB | 从头部保留 10MB，`metadata.truncated=true` |
| 任务总量超 200MB | 按 `execution_transcript` > `diff` > `declared` > `conclusion` 优先级采集直到触顶，其余文件名记入 `metadata.skipped_files`。**证据优先于自述**：`conclusion` 是唯一可被完全丢弃的类型 |
| `HeadObject` 在物化时发现对象不存在 | 事务回滚，任务不完成 |
| 对象存储整体不可用 | 任务无法完成，attempt 重试；持续失败最终由现有租约超时逻辑处理 |
| `raw.jsonl` 不存在（provider 未产生任何输出） | 零证据完成 → 事务回滚，任务不完成 |
| 脱敏正则命中 | 替换为 `[REDACTED:{reason}]`，`metadata.redaction_count` 递增；不阻断上传 |

## 6. 测试策略

**单元**

- `artifacts.rs`：sha256 计算、限额截断、glob 匹配、`already_exists` 跳过。
- `S3ObjectStore.PresignPut/PresignGet`：key 前缀强制、TTL。
- presign handler：body 中的 `tenant_id` 被 node token 覆盖。
- `materializeTaskCompletionEvidenceWithQueries`：artifact upsert 幂等、`artifact_ref_id` 回填、`HeadObject` 失败导致回滚、零证据完成导致回滚。
- **血缘隔离**（对应 3.2）：同一 `project_task_id` 下，(a) 两个不同 `digital_employee_id` 的 attempt 各写一行；(b) 同一员工的两次 attempt 即使 `checksum` 完全相同也各写一行；(c) 同一 attempt 的 result 重复提交只有一行。

**真实 E2E（完成的必要条件，非单测可替代）**

`scripts/dev-services.sh start` 起全套服务，创建项目，跑一个**真实执行了至少一次 shell 命令**的 claude-code 任务（例如让它运行 `pnpm --filter ./apps/web run test` 或 `git status`），然后断言：

1. `GET /projects/{id}/artifacts` 含一条 `artifact_type=execution_transcript` 的记录。
2. `GET /api/v1/artifacts/{id}/content` 跟随 302 取回该 transcript，解析 JSONL 后能找到至少一条 `tool_result` 事件，**其中包含该 shell 命令的真实退出码**。
3. `GET /projects/{id}/evidence` 对该任务返回的行数等于该任务的 ref 数（不是两倍）。
4. transcript 对应的 evidence 行 `artifact_ref_id` 非空、`verification_status='submitted'`；`conclusion` 对应的行 `evidence_type='self_report'`、`verification_status='unverified'`。
5. 直接查库确认 `project_artifact_refs.checksum` 是合法 sha256 hex，且对象存储中 `artifacts/{tenant}/sha256/{hex}` 确实存在。
6. 构造一个 tool result 中含 `sk-` 前缀假 token 的任务，确认上传后的 transcript 中该 token 已被 `[REDACTED:*]` 替换。

**第 2 条是本 spec 的核心判据**：人类能从平台取回一份数字员工无法伪造的事实。仅仅「artifacts 非空」不构成通过——那用一个 `conclusion.md` 就能骗过去。

## 7. 后续 spec

本 spec 让「产出物可取回」成立，这是下面两件事的前提：

1. **意图与验收判据**（`docs/superpowers/specs/2026-06-30-intent-acceptance-criteria-design.md` 已立项未实现）：planner 拆任务时一并生成 acceptance criterion 与 `verification_method`，提交人类在 plan revision 审批时确认或修改。criterion 绑定 verification，verification 绑定 artifact。

   本 spec 采集的 `execution_transcript` 正是 `automated` 类 verification 的原材料：「`pnpm test` 退出码为 0」这条判据，其证据指针指向 transcript 中某条 `tool_result` 事件的偏移。没有 transcript，`automated` verification 就只能靠数字员工自报，退化成自证。

2. **人类待办的机制反转**：当前人类待办只在「AI 自己声明 `requires_human_review`」或「result contract 校验失败」时产生——等于让 AI 决定要不要被审查。目标是补上第三个来源：任务完成时若某条 criterion 找不到 `automated` 或 `peer_review` 的 passed verification，控制平面**自动**开一条 `verification_method=human` 的待办，把 artifact 直接挂上去。这同时解开 `waiting_human` 导致 demand 永不 terminal、项目永不进验收的死锁——人类待办成为推进 demand 的正常一步，而非卡住它的例外。

3. **哈希完整性 attestation**：4.2 中 presigned PUT 无法强制字节与声明的 sha256 一致。后续引入服务端异步校验或 `x-amz-checksum-sha256` 强校验。

事实依据：`project_task_results` 20 条记录中 `contract_payload.verifications` 一条都没有；52 个 `project_tasks` 的 `handoff_contract.required_outputs` 全为 NULL。`TaskResultVerification` 类型与校验函数在 Go 侧已定义（`task_result_contract.go:140`），从未被填充。
