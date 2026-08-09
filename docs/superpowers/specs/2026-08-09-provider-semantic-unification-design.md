# Provider 语义统一层（事件 / 结果 / 错误 / 能力）

- 日期：2026-08-09（**2026-08-10 按代码复核修订**，见 §17 修订记录）
- 状态：**待人类拍板（设计稿已过一轮代码复核，未实施）**——阻塞项集中在 §13
- 系列：
  - 承接已落地：`2026-07-09-provider-transcript-tool-event-capture-design.md`（transcript / tool 事件 / raw 双轨）
  - 承接立项未实施：`2026-07-19-runtime-provider-contract-verification.md`（本 spec **吸收并细化其 P2 Provider 侧**；P1 runtime openapi 仍可独立推进）
  - 对齐架构约定：`AGENTS.md`「Provider 协议必须语言无关」与已知债 `contracts/provider/`
- 交付性质：**架构与契约设计**。分 Phase 实施；**不**要求一次 PR 做完。拍板后再开实施 plan。
- 目标读者：架构评审 / 实施会话（本文自包含；实施前对照文末「现状锚点」）
- **迁移**：Phase 1/2 零迁移（只扩展 `additionalProperties: true` 的 writeback `payload`/`metadata` 与既有列）。**但 §15「跨 Provider 可比较与告警」需要 `error_code` 落到 `project_task_attempts` 一列**——attempt 行现有列只有 `n`(failure_family)/`retryable`/`failure_message`，`execution_context_packet` 是入参不是出参，code 只存在事件 payload 里意味着告警与统计要扫 JSON。**该列加不加是 §13 议题 10，未拍板前 §15 只兑现到「协调决策可比较」，不兑现「可告警统计」。**

---

## 0. 一句话方案

> **在现有 Runtime Adapter 映射之上，把「过程事件、终态结果、结构化错误、适配器能力」升为 `contracts/provider/` 的可验证契约；Control Plane 与协调线程只消费平台语义；三家 CLI 方言永远关在 Runtime 适配器 + golden 样例里。**

不推倒重做 `ProviderEvent`，不引入 protobuf/新主栈，不在 CP 做二次方言解析。

---

## 1. 背景与问题

### 1.1 平台定位对「统一」的约束

SuperTeam 是企业数字员工**控制平面**，不是聊天聚合器：

| 平台目标 | 对 Provider 层的要求 |
|---|---|
| 项目协调可决策 | 可靠知道会话/回合/失败/证据 |
| 人类可审批与复盘 | 工具过程是**进程事实**，不是模型自述 |
| 多 Provider 可替换 | CP / 协调 / Web **零依赖** Claude·OpenCode·Codex 字段名 |
| 审计可追责 | 归一事件 + raw 双轨 |
| 路由与预算 | 用量、失败族、可重试性可跨 Provider 比较 |

因此「完整」= **语义覆盖 + 可验证契约 + 能力诚实 + 业务只认平台语义**，不是「三家 stdout 像素级同构」。

### 1.2 已经对的部分（不要重做）

| 能力 | 位置 | 说明 |
|---|---|---|
| 统一事件枚举 | `apps/runtime-agent/src/events.rs` `ProviderEvent` | session / text / tool / turn（**error 变体在真实链路上是死的，见 1.3.6**） |
| 三家 parser | `providers/claude.rs` · `opencode.rs` · `codex.rs` | 原生 JSON 行 → `ProviderEvent` |
| 共享流管道 | `providers/mod.rs` `stream_child_events` | raw 先落、再 parse、退出合成错误 |
| 用量 best-effort | `providers/usage.rs` | usage / token_usage 字段归一；已被预算心跳与 result contract 消费（`executor.rs` `usage_tokens`），**不是死字段** |
| writeback | `commands/executor.rs` `runtime_event_writeback` | → CP `event_type` + payload |
| 失败族（粗糙） | `project_task_failure_classification` | 字符串 contains → `failure_family` |
| raw 证据轨 | 2026-07-09 已落地 | 原始流不因 parse 丢弃 |
| 零终态兜底 | `executor.rs` `drain_provider_events` 尾部 | 「流尽却无 TurnCompleted/TurnError」已会 fail，不再永滞 dispatching |

### 1.3 仍存在的结构性缺口（每条附现场证据，实施时可直接复现）

1. **契约债**：`contracts/provider/` 仅散文 README；事实源在 Rust 类型（AGENTS 已知债 + 2026-07-19 P2）。
2. **错误以字符串穿透，且是自伤式往返**：熔断点 `executor.rs` 已经知道原因是 `wall_clock_exceeded`（`let reason = "wall_clock_exceeded"`），把它拼成字符串交给 fail writeback，`project_task_failure_classification` 再用 `contains("wall_clock_exceeded")` 从自己刚生成的字符串里把 family 猜回来。结构化错误不是洁癖，是消除这段往返。
3. **能力不诚实**：三家 tool / usage / 结构化错误覆盖不均，但产品面常假设同构；未知原生行多 `Ok(Vec::new())` 静默丢（raw 仍在，业务不可观测）。
4. **终态判据漏读原生错误位**：`claude.rs` 的 `"result"` 分支只取 `summary` + `usage`，**完全不读顶层 `is_error` / `subtype`**（`is_error` 只在 tool_result 块被读，见 `parse_user_blocks`）。claude 的报错回合若以 0 退出，会被写成 `completed`，错误文本还成了 summary。这是「终态隐含」的具体实例，Phase 1 必测。
5. **终态不是一条路径而是四条，且 attestation 覆盖不齐**：
   - in-loop `TurnError` → 有 attestation（`provider_terminal/failed`）
   - 尾部成功 → 有 attestation
   - 尾部「无终态事件」→ 有 attestation
   - **caller 捕获 stream `Err`（`spawn` 后 `if let Err(error) = result`）→ 只有 `writeback.fail(message)`，没有 attestation**

   而第四条恰恰是最常见的真实失败路径（见下条）。
6. **`turn_error` 是死事件类型**：没有任何 parser 产 `ProviderEvent::TurnError`——codex 的 error/turn.failed 走 `anyhow::bail!`，非零退出走 `provider_exit_result` 返回 `Err`，`drain_provider_events` 的 `let event = event?` 直接早退。`TurnError` 只由 `runs.rs` `finish_failed` 与 `main.rs` CLI 路径产生，前者只进本地 run store，**不上行 CP**。后果：CP 侧从未收到过 `turn_error`，失败只经 fail writeback 到达，**L2 时间线在失败时没有任何终止标记，戛然而止**。
7. **`failure_family` 有两个生产者，词表不重合**：runtime 产 6 个族（`project_task_failure_classification`），CP 自产 16 个族常量（`project/types.go` `FailureFamily*`）。runtime 的 `budget_fuse` **全仓只有 runtime 认识**——CP 无此常量，落到 `projectTaskFailureAction` 走 `default → failed`，`humanReadableFailureSummary` 也走兜底文案「任务执行失败」，预算熔断在人类侧看不出是预算问题。
8. **同一语义三种字段名**：`provider_type = "claude-code"`（`catalog.rs`）、`provider_kind = "claude"`（同文件 descriptor）、`server.rs` 只认 `claude|opencode|codex`、现网 attestation metadata 落的是 `provider_kind`（即 `"claude"`）、CP 侧还兼容 `claude-code|claude_code|claude` 三写法（`employee/pg_repository.go`）。这本身就是本 spec 要解决的那类语义不统一。
9. **family 没进中文词表**：Web 直接渲染裸英文枚举 `失败族：{attempt.failure_family}`（`project-execution-trace-panel.tsx`），`status-labels.ts` 无任何 family 词条，护栏测试只拦 `.status` / `.risk_level`。这违反 CLAUDE.md 的中文优先约定，也是 `TODO.md` 2026-08-07 那条积压。
10. **验证缺失**：无 golden「原生样例 → 期望事件」；无 schema 校验 writeback；改 adapter 易回归。**且仓库当前没有任何 JSON Schema 校验器**（根 `package.json` `devDependencies` 为空、无 ajv；`runtime-agent/Cargo.toml` 无 jsonschema/schemars），见 §13 议题 8。

### 1.4 目标

1. 定义 **L0–L4 语义分层**与禁止泄漏规则。
2. 在 `contracts/provider/` 落 **机器可读 schema**（事件 envelope、错误、结果、能力、用量）。
3. 规定 **Adapter 职责、Capability Matrix、Native→Platform 映射资产、golden 门禁**。
4. 用 **ErrorEnvelope + 稳定 code→family 表** 替换字符串 contains 主路径。
5. 规定 **ProviderResult 与业务验收契约的边界**，并把现有四条终态路径并到一处。
6. 收敛 **Provider 标识字段命名**（1.3.8 的 `provider_type` / `provider_kind` 三写法）。
7. 补齐 **failure_family 的跨层词表**：两个生产者共用一份族清单 + 中文词条 + 护栏（1.3.7 / 1.3.9）。
8. 给出 **分 Phase 实施与验收**，拍板后可直接开 plan。

### 1.5 非目标

- 不引入 protobuf / gRPC / 替代主栈 IDL。
- 不强制三家 100% 同构 tool/thinking 流（用 capability 表达差异）。
- 不把业务验收（demand 判据、casting 策略）写进 parser。
- 不在 Control Plane 再做一层「Provider 方言映射」。
- 不用 LLM 把日志「总结成统一事件」当主路径。
- 不把 `failure_family` 收成闭集 enum（CP 自产族与 runtime 产族同写一列，闭集必打架；见 §5.3）。
- 不本批改 Temporal 协调状态机语义、不改派发 payload 全量 schema（派发 payload 可在后续并入 2026-07-19 的 start_session 部分）。
- 不本批做 Desktop；不本批 SSO。
- 历史 attempt **不强制回填**结构化 error（只保证上线后新执行）。

---

## 2. 语义分层（架构核心）

```text
L0  Raw Stream          证据：stdout/stderr 原样；永不因 parser 失败丢
L1  Native Parse        适配器私有：只认识本 Provider 方言
L2  Platform Events     统一过程事件：唯一上行过程形态
L3  Attempt Outcome     统一终态：ProviderResult + ErrorEnvelope
L4  Control Plane Facts 业务事实：attempt / ledger / narrative / 验收
```

| 层 | 写入方 | 读取方 | 禁止 |
|---|---|---|---|
| L0 | Runtime | 审计、排障、离线重放 | 当业务状态机唯一源 |
| L1 | 各 adapter 内部 | 仅 adapter | 泄漏到 CP OpenAPI / 协调逻辑 |
| L2 | adapter → executor | ledger、执行时间线、Web | 塞业务策略 / 验收 |
| L3 | Runtime 在 attempt 结束时合成 | 协调线程、重试、预算、收件箱 | 依赖原生错误字符串 |
| L4 | Control Plane | 产品与编排 | 再解析 Claude/OpenCode/Codex 字段 |

**过程（L2）与结论（L3）必须分离。**  
协调线程优先信 L3；Web 执行时间线信 L2；争议回 L0。

### 2.1 数据流（目标态）

```text
Provider CLI JSON lines
  → [L0] raw_sink 逐行落盘/上传
  → [L1] parse_*_event
  → [L2] ProviderEvent envelope (schema_versioned)
  → executor: redaction + seq + writeback
  → [L4] CP ledger / project task events

attempt 结束:
  → [L3] ProviderResult { status, usage, error?, artifacts, diagnostics }
  → complete / fail / wait-human writeback
  → 协调 / 重试只看 family + retryable + status
```

---

## 3. 契约包布局（事实源）

路径：`contracts/provider/`（扩展现有目录，不新开并行契约根）。

```text
contracts/provider/
  README.md                          # 边界、演进、与本 spec 链接（实施时更新）
  schemas/
    provider-event.schema.json       # L2 envelope + type 判别
    provider-event-payloads.schema.json  # 各 type 的 payload（或 oneOf 内联）
    provider-error.schema.json       # ErrorEnvelope
    provider-result.schema.json      # L3
    provider-usage.schema.json
    provider-capability.schema.json  # 适配器能力声明
    failure-family.json              # 跨层 family 词表（known_values，两个生产者共用，见 §5.3）
  golden/
    claude-code/                     # 原生 stdout 行样例 + expected events（**含 ≥1 条失败样例**）
    opencode/
    codex/
    README.md                        # 样例采集约定（真实 smoke 截取、脱敏、**语义推导法见 §7.5**）
  fixtures/
    result-succeeded.json
    result-failed-budget-fuse.json
    result-failed-transient.json
    error-rate-limit.json
```

### 3.1 演进规则

| 规则 | 说明 |
|---|---|
| 事实源 | **语义**契约以 schema 为准，Rust/Go 类型是实现；**wire 传输**契约仍在 `contracts/control-plane/openapi.yaml`（payload/metadata 为不透明对象），两者分工见 §4.5.1，不得互为副本 |
| 破坏性变更 | 升 `schema_version`（如 `provider.event.v2`），旧版读路径保留至少一个发布周期 |
| 新增可选字段 | 同 major 内允许；写入方不得依赖对端必填 |
| 验证 | golden（零依赖，必进门禁）+ schema validate（依赖 §13 议题 8）；见 §11 |
| 禁止 | 业务核心 `switch provider_type` 解析原生 payload |
| 反腐烂 | schema 必须被至少一个**会红**的检查消费；只存在不校验 = 未交付（先例：`contracts/provider/README.md`） |

### 3.2 与 2026-07-19 立项关系

| 2026-07-19 条目 | 本 spec |
|---|---|
| P1 runtime openapi 纳入 verify | **不吞并**；仍可独立做 |
| P2 派发 payload schema | **后续切片**（本 spec Phase 4）；本 spec Phase 1/2 先做 **event / error / result / capability / usage** |
| P2 事件回写 / 结果 / 错误分类 | **由本 spec 定义字段与分层**，实施时销账 AGENTS Provider 债的「事件结果错误」部分 |

---

## 4. L2：平台事件契约

### 4.1 Envelope（对外 JSON 形状）

> **命名前置条件（§13 议题 7，未拍板前不要照抄本例）**：仓库现状是 `provider_type = "claude-code"`（注册表口径，CP 也用它）与 `provider_kind = "claude"`（runtime 内部短名）并存，且现网 attestation metadata 写的是后者。本 spec **推荐统一到 `provider_type` + 注册表取值（`claude-code` / `opencode` / `codex`），`provider_kind` 退役**；下例按推荐方案书写，字段名一旦拍板需全文替换并同步 `catalog.rs` / `server.rs` 校验 / `employee/pg_repository.go` 的三写法归一。

```json
{
  "schema_version": "provider.event.v1",
  "type": "tool_started",
  "ts": "2026-08-09T04:00:00.123Z",
  "seq": 17,
  "provider_type": "claude-code",
  "provider_session_id": "optional-session-id",
  "attempt_ref": {
    "command_id": "…",
    "attempt_id": "…"
  },
  "payload": { },
  "provenance": {
    "native_type": "assistant",
    "raw_line_ref": "optional-opaque-ref"
  }
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `schema_version` | 是 | 固定前缀 `provider.event.` + 版本 |
| `type` | 是 | 见 §4.2 闭集（平台级；扩展需升版或显式 registry） |
| `ts` | 是 | RFC3339；Runtime 生成（主机时钟） |
| `seq` | 是 | **单 runtime run 内单调从 1 起**（现状：`runs.rs` `next_sequence` 按 run 计数），与 writeback `sequence_number` 对齐。项目任务链路上 run ↔ attempt 为 1:1，故等价于「attempt 内单调」；chat 线程一条 thread 有多个 run，seq 会重新从 1 起，**不可当作线程内全序** |
| `provider_type` | 是 | 注册表字符串（`claude-code` / `opencode` / `codex` / 未来类型）；命名见 §4.1 前置条件 |
| `provider_session_id` | 否 | 已知则填；`session_started` 之后应尽量带上 |
| `attempt_ref` | 否* | 有 command/attempt 上下文时必填 |
| `payload` | 是 | 按 type 约束；可 `{}` |
| `provenance` | 否 | 排障用；**CP 业务逻辑不得依赖** |

\* CLI 单机调试无 attempt 时可空。

### 4.2 `type` 闭集（v1）

与现有 `ProviderEvent` 对齐，**v1 不删既有语义**，只加契约字段与可选扩展：

| type | payload 要点 | 现有 Rust 变体 |
|---|---|---|
| `session_started` | `session_id`；可选 `session_state` | `SessionStarted` |
| `turn_started` | `{}` | `TurnStarted` |
| `text_delta` | `text`（出站已脱敏） | `TextDelta` |
| `tool_started` | `tool_id`, `name`, `input_excerpt`, `input_truncated` | `ToolStarted` |
| `tool_completed` | `tool_id`, `is_error`, `output_excerpt`, `output_truncated` | `ToolCompleted` |
| `turn_completed` | 可选 `summary`；可选 `usage`（**v1 必须允许并建议必填若 adapter 能解析**） | `TurnCompleted` |
| `turn_error` | payload 携带 `error`（ErrorEnvelope，见 §5）+ 兼容期保留扁平 `message` | `TurnError`（**枚举存在，但真实链路从不产出——见下方「行为变更」**） |
| `native_unmapped` | `native_type?`, `reason: "unrecognized_type" \| "parse_skipped"`（**可选开关**，见 §4.4） | 新增 |

**v1 不引入** `thinking` / `diff` / `stdout` 作为平台 type（需要时升 v2 或继续只留 L0）。

#### 4.2.1 `turn_error` 是**行为变更**，不是兼容改造（修订新增）

现状（1.3.6）：`turn_error` 从未上行到 CP，provider 失败一律以 stream `Err` 早退 + fail writeback 表达。因此本 spec 让 adapter/executor 在 stream `Err` 时**合成一条 `turn_error` 上行**属于新增能力，必须连带定义：

| 事项 | 规定 |
|---|---|
| 触发点 | `stream_child_events` 产出 `Err` 时，由 executor 在早退前合成（不在各 parser 里各写一遍） |
| 与 fail writeback 的关系 | `turn_error` 是**过程事件（L2）**，fail writeback 是**终态（L3）**；两者都发，携带**同一个** ErrorEnvelope 实例 |
| 去重口径 | 二者同源同 code，消费侧按层各取所需：时间线/叙事只认 `turn_error`，协调/重试只认终态；**叙事层不得把同一次失败记两条**（`event_narrative` 需按 §8.3 显式排除终态重复） |
| 收益 | 失败时 L2 时间线有终止标记（现状戛然而止）；`turn_error` 与 attestation 补齐后，四条终态路径证据一致 |
| 风险 | 若 §13 议题 1 拍板不做，则 `turn_error` 应从 v1 type 闭集中**删除**，不要留一个契约上存在、链路上永不出现的类型 |

### 4.3 截断与脱敏

| 规则 | 值 |
|---|---|
| excerpt 上限 | 与现码一致：`EXCERPT_LIMIT_BYTES = 4096`；契约写死同值 |
| 截断标记 | `…[truncated]` + `*_truncated: true` |
| 脱敏边界 | **L2/L3 出站**（writeback / 对外事件）；L0 raw 保持原样（访问受权限与对象存储策略约束） |
| 实现 | 复用 `redaction::redact_with_environment`；契约注明「message/text/excerpt 均为脱敏后」 |

### 4.4 未知原生行策略

| 策略 | 行为 | 默认 |
|---|---|---|
| A. 静默 | 仅 L0；L2 不发（现状多数路径） | 兼容旧行为 |
| B. 可观测 | L0 + 可选 L2 `native_unmapped`（rate-limit：同 attempt 最多 N 条，默认 20） | **Phase 3 实现开关，默认关；诊断环境可开**（放 Phase 3 是因为 codex 需先收敛贪婪兜底，否则计数无意义，见 4.4.1） |
| C. 失败 | 直接 fail attempt | **禁止**作为默认（噪声会拖垮任务） |

诊断指标：`diagnostics.unmapped_native_count` 进入 L3（§6），默认始终累计。

#### 4.4.1 实现成本（修订新增，实施前必读）

「数未映射的行」在现有代码形态下**不是加个计数器那么简单**：

1. **parser 签名要改**。现在是 `fn(&str) -> anyhow::Result<Vec<ProviderEvent>>`，空 `Vec` 同时表示两种完全不同的事：
   - 「已知类型但本行无事件」——`opencode.rs` 的 `step_start` 无 `sessionID`、`text` 为空串都返回空 `Vec`；
   - 「未知类型」——各 parser 的 `_ =>` 兜底分支。

   要区分，必须改成返回结构体（如 `ParseOutcome { events, unmapped: Option<&str> }`），或让每个 parser 在 `_ =>` 分支自己产 `native_unmapped` 事件。**推荐后者**：签名不变，goldens 直接把 `native_unmapped` 作为期望事件写进去，语义可测。
2. **codex 的计数天然失真**。`codex.rs` 的 `extract_session_id` / `extract_text` 是**跨类型贪婪兜底**（任何带 `session_id`/`text`/`delta`/`content` 键的行都会命中，与 `type` 无关），因此 codex 几乎不存在「未知类型」，`unmapped_native_count` 恒接近 0，却**不代表映射正确**。codex 的 capability 与诊断指标不可与另两家横向比较，Phase 3 处理 codex 映射时应先把贪婪兜底收敛到显式 type 分支。
3. **噪声行与未知类型分开计数**：`parse_line_json` 跳过的非 JSON 行计入 `unparseable_line_count`，未知 type 计入 `unmapped_native_count`，两者混在一起会让「provider 换了输出格式」和「provider 打印了一行日志」看起来一样。

### 4.5 与现有 writeback 的兼容

当前 CP 消费：`event_type` 字符串 + 扁平 `payload` map（`employee/run_writeback.go` `RecordEvent`）。

**契约事实（已核）**：`contracts/control-plane/openapi.yaml` 的 `RuntimeCommandEventWritebackRequest` 里 `payload` 与 `metadata` 都是 `additionalProperties: true` 的不透明 object。**因此新增字段不需要改 openapi、不需要 `generate:control-plane`，零契约变更**——代价是**没有任何一侧在运行期校验它们**，见下方 4.5.1。

**过渡策略（Phase 1）：**

1. writeback **继续**发 `event_type` = 上表 type（snake_case，与现网一致）。
2. payload **保持**现有键（`text`, `tool_id`, …），避免一次打断 Web。
3. 新增可选顶层或 metadata：
   - `schema_version`
   - `provider_type`（命名见 §4.1 前置条件；注意现网 attestation metadata 里已有一个取值不同的 `provider_kind`，两者并存期必须在 §14 锚点里写明谁是谁）
   - `usage`（挂在 `turn_completed`；现状 `runtime_event_writeback` 的 `TurnCompleted` 分支**丢弃了 usage**，只透传 summary。usage 目前经 `usage_tokens` 累加走预算心跳与 result contract，事件层补 usage 属于新增可观测性，不影响预算口径）
   - `error` 对象（挂在 `turn_error` / fail writeback）
4. Phase 2 起 CP 读路径优先 `error.code` / `error.family`；无则回退旧 `message` + 旧 classification（双读一期）。

#### 4.5.1 谁校验 schema（修订新增，§13 议题 9）

`contracts/provider/schemas/*.json` 若**只**被单测与 fixture 使用，它就是第三份会腐烂的文档——`contracts/provider/README.md` 挂着散文契约腐烂数月正是先例。三个可选强度，必须显式选一个：

| 强度 | 做法 | 成本 | 失效模式 |
|---|---|---|---|
| S1 仅测试 | Rust golden + Go fixture 校验，生产不校验 | 低（但仍需校验器，见 §13 议题 8） | 生产漂移只能靠事后排障发现 |
| S2 ingest 打标 | CP `RecordEvent` 校验失败时**不拒绝**，落 `metadata.schema_violation` + 计数告警 | 中 | 需要 Go 侧 JSON Schema 依赖 |
| S3 ingest 拒绝 | 校验失败返 400 | 中 | runtime 版本落后于 CP 时会批量掉事件，**不推荐** |

**推荐 S1 起步 + Phase 3 评估 S2**；无论选哪个，都要在 README 写死「wire envelope 的传输契约在 control-plane openapi，语义契约在 contracts/provider」，避免两份事实源。

---

## 5. 结构化错误（ErrorEnvelope）

### 5.1 形状

```json
{
  "schema_version": "provider.error.v1",
  "code": "RATE_LIMIT",
  "family": "transient_provider",
  "retryable": true,
  "message": "claude exited with status 1: rate limit exceeded",
  "provider_type": "claude-code",
  "native": {
    "type": "result",
    "exit_code": 1,
    "excerpt": "…"
  },
  "evidence_refs": [
    { "type": "runtime_command", "ref": "runtime-command://…" }
  ]
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `code` | 是 | **稳定机器码**，UPPER_SNAKE；闭集见 §5.2 |
| `family` | 是 | 写进 `project_task_attempts.n` 的业务族；**开放字符串**，词表见 §5.3 |
| `retryable` | 是 | 协调/重试的布尔输入；**注意 CP 侧并非只看它**——`projectTaskFailureAction` 的路由（重试/等人/取消/失败）仍由 `family` 决定，`retryable=false` 只是短路。二者不一致时以 §5.3 的 code→(family, retryable) 单表为准，两个值必须同源产出 |
| `message` | 是 | **技术原文（英文亦可）+ 已脱敏**。**不在这里产中文**：中文归 CP/词表（§5.5），Rust adapter 产中文会与 `humanReadableFailureSummary`、`status-labels.ts` 三处打架 |
| `provider_type` | 是 | 命名见 §4.1 前置条件 |
| `native` | 否 | 截断后的排障信息；**禁止**作为 family 决策输入（决策只用 code） |
| `evidence_refs` | 否 | 指向 L0 / command |

### 5.2 `code` 闭集（v1，宜少）

**闭集来源规则**：code 集必须从 **runtime 现在真的会产出的失败点**反推，不是先验想象。已核的现存发射点（`executor.rs` / `mod.rs`）——`provider_exit_result` 的「非 0 退出」与「wait 失败」、`spawn` 失败（`failed to spawn claude/opencode/codex`）、`wall_clock_exceeded`、`operator cancelled`、`provider exited without a terminal event`、codex `bail!` 的原生 error 行、workspace/content_hash 类错误。下表已覆盖全部，新增发射点必须同批加 code。

| code | 典型来源 | 默认 family | 默认 retryable |
|---|---|---|---|
| `PROVIDER_EXIT_NON_ZERO` | 进程非 0 退出 | `transient_provider` 或 `non_retryable_execution`* | *见映射表 |
| `PROVIDER_SPAWN_FAILED` | 二进制不存在/无权限 | `provider_configuration` | false |
| `PROVIDER_PROTOCOL_ERROR` | 流协议致命错误 / codex 原生 error 行 | `non_retryable_execution` | false |
| `PROVIDER_NO_TERMINAL_EVENT` | **exit 0 但全程无 TurnCompleted/TurnError**（现状 `provider exited without a terminal event`，即输出格式漂移被解析层全量丢弃的典型形态） | 现状落 `non_retryable_execution`（无 contains 命中）→ **建议改 `transient_provider`**，见 §13 议题 11 | 现状 false → 建议 true |
| `RATE_LIMIT` | 限流 | `transient_provider` | true |
| `AUTH_FAILED` | 鉴权/配额账号 | `provider_configuration`（CP 已有中文 lead「执行器配置有误」，比 `non_retryable_execution` 信息量高） | false |
| `TIMEOUT` | 超时 | `timeout` | true |
| `BUDGET_FUSE` | 墙钟/token 熔断 | `budget_fuse`（**CP 目前不认识此族**，落 default「失败」，见 §13 议题 12） | false |
| `CANCELLED` | 操作者取消 | `business_cancelled` | false |
| `WORKSPACE_INVALID` | 工作区/同步/hash | `invalid_contract` | false |
| `WAITING_HUMAN` | 需人类（若走错误通道） | （通常走 wait-human 状态，不是 fail） | false |
| `UNKNOWN` | 无法归类 | `non_retryable_execution` | false |

\* `PROVIDER_EXIT_NON_ZERO`：adapter 若能从 stderr/原生事件识别 rate limit / auth，应先映射到更具体 code；否则 Runtime 统一表可按 exit + 关键字做**最后兜底**（兜底逻辑集中在一处，禁止散落 contains）。

### 5.3 `family`：两个生产者，一份词表（修订重写）

**先纠正原稿的事实错误**：原表列的 6 个族不是「现网 family 全集」，只是 **runtime 生产子集**。现网 family 的权威清单在 `apps/control-plane/internal/project/types.go` 的 `FailureFamily*` 常量（16 个），由 **CP 自己**在派发失败、预检闸、结果契约等路径产出。同一列 `project_task_attempts.n` 有两个生产者。

| 生产者 | 产出族 |
|---|---|
| Runtime（`project_task_failure_classification`） | `budget_fuse`、`business_cancelled`、`invalid_contract`、`timeout`、`transient_provider`、`non_retryable_execution` |
| Control Plane（`types.go` 常量 + 协调线程/预检闸） | 另有 `transient_runtime`、`dispatch_transient`、`runtime_start_timeout`、`runtime_lease_lost`、`transient_provider_start`、`provider_configuration`、`approval_required`、`permission_required`、`plan_invalid`、`requirement_changed`、`acceptance_required` 等 |

**已发现的实缺陷（本 spec 负责修）**：`budget_fuse` 是 runtime 单方面发明的族，**CP 无对应常量** → `projectTaskFailureAction` 落 `default → failed`，`humanReadableFailureSummary` 落兜底文案「任务执行失败」。结果是**墙钟预算熔断在人类侧看不出是预算问题，也不会进等人**（而 `HumanWaitReasonBudgetApproval` 与 `project_task_budget_approval` 决策动作是已存在的）。处置见 §13 议题 12。

**规则**：

| # | 规则 |
|---|---|
| 1 | `family` 在 schema 里是 **开放字符串**（`type: string`，非 enum）。闭集会让 CP 自产族违反自己的契约 |
| 2 | 但**取值必须来自单一词表**：新建 `FailureFamily` 词表作为跨层事实源（建议随 schema 落 `contracts/provider/schemas/failure-family.json` 的 `known_values` 清单，Rust 与 Go 各自单测断言自己的常量集 ⊆ 清单） |
| 3 | Runtime 新增族 → 同批加 CP 常量 + `projectTaskFailureAction` 路由分支 + `humanReadableFailureSummary` 中文 lead + `status-labels.ts` 词条（§8.3）。**四处缺一即视为未完成**，`budget_fuse` 就是缺三处的现例 |
| 4 | code → (family, retryable) 是**一张表、一处实现**（Runtime `providers/error_map.rs`），单测覆盖；`retryable` 与 `family` 必须同源产出，不得分别计算 |
| 5 | 优先复用 CP 已有族而不是造新族：如 spawn 失败用已存在的 `provider_configuration`（CP 有中文 lead「执行器配置有误」），比塞进 `non_retryable_execution` 信息量高且零新增 |

**禁止** CP 再 `strings.Contains(msg, "claude exited")` 做 family 决策。

### 5.4 替换 `project_task_failure_classification`

| 阶段 | 行为 |
|---|---|
| Phase 1 | 新增 `classify_provider_error(ErrorEnvelope) -> (family, retryable)`；无 envelope 时调用旧函数（deprecated） |
| Phase 1 末 | fail writeback **必带** ErrorEnvelope；旧函数仅测兼容，主路径删除 contains 分支 |
| 完成标准 | Runtime 侧无 `contains("claude exited")` 式 **family 决策** |

### 5.5 中文文案归属（修订新增）

现状有**三处**在把技术错误变成人类可读文本，实施时不要再加第四处：

| 位置 | 现状 | 本 spec 后 |
|---|---|---|
| Runtime adapter | 无（只产英文原文） | **保持**：`ErrorEnvelope.message` 是技术原文 + 脱敏 |
| CP `humanReadableFailureSummary` / `humanizeTechnicalFailureDetail`（`project/service.go`） | 按 family 出中文 lead + 对 raw 文本做 `contains` 归一 | **保留**（§8.2 允许展示层 contains）；但 `contains` 链应随 code 落地逐步改为 `switch code`，因为 code 比英文子串稳定 |
| Web `status-labels.ts` | **没有任何 family 词条**，直接渲染裸英文枚举 | 补 family 词表 + 扩护栏（§8.3） |

判据：**决策只看 code/family/retryable，中文只在 CP 与 Web 产出，Rust 不产中文。**

---

## 6. L3：ProviderResult（attempt 终态）

### 6.1 形状

```json
{
  "schema_version": "provider.result.v1",
  "status": "succeeded",
  "summary": "可选摘要",
  "usage": {
    "total_tokens": 10848,
    "input_tokens": 10717,
    "output_tokens": 3
  },
  "error": null,
  "artifacts": [],
  "session": {
    "provider_session_id": "…",
    "resumable": true
  },
  "diagnostics": {
    "provider_type": "opencode",
    "exit_code": 0,
    "duration_ms": 12345,
    "event_counts": {
      "text_delta": 3,
      "tool_started": 0,
      "tool_completed": 0,
      "turn_completed": 1,
      "turn_error": 0,
      "native_unmapped": 2
    },
    "unmapped_native_count": 2,
    "unparseable_line_count": 0
  }
}
```

### 6.2 `status` 闭集

| status | 对应 writeback | 说明 |
|---|---|---|
| `succeeded` | complete | 正常结束 |
| `failed` | fail | 执行失败 |
| `cancelled` | cancelled / fail+family | 取消 |
| `timed_out` | fail + timeout | 可与 failed+code=TIMEOUT 二选一；**v1 推荐 status=`failed` + code=`TIMEOUT`**，status 枚举保留 `timed_out` 供显式 |
| `waiting_human` | wait-human | 非失败 |

**v1 拍板建议：** `status` 使用  
`succeeded | failed | cancelled | waiting_human`  
超时与熔断用 `failed` + ErrorEnvelope.code 表达，减少双重语义。  
`timed_out` 若已有外部依赖再保留。

### 6.3 与业务验收的边界

| 对象 | 层 | 含义 |
|---|---|---|
| `ProviderResult` | L3 执行引擎 | 这轮 Provider **跑完了没有**、用量、错误 |
| Task result contract / 验收判据 | L4 业务 | 交付是否合格、是否过 adversarial review |

**禁止**在 adapter 内判断「需求是否验收通过」。  
协调线程：先消费 ProviderResult，再跑业务 gate。

### 6.4 合成时机

每个 attempt **恰好一个** ProviderResult，在：

- 收到 `turn_completed` 且进程成功退出，或  
- 流错误 / 非 0 退出 / 取消 / 预算熔断 / wait-human  

由 executor 统一 `build_provider_result(...)`，再派生 complete/fail writeback。  
禁止多处手写 fail 字符串而不经 Result 构造。

**现状是四条路径，不是两条**（`executor.rs`，实施时必须全部并入 `build_provider_result`）：

| # | 路径 | 现状终态回写 | 现状 attestation |
|---|---|---|---|
| 1 | 事件循环内 `TurnError` | fail | ✅ `provider_terminal/failed` |
| 2 | 流正常结束 + 有 `TurnCompleted` | complete | ✅ `provider_terminal/succeeded` |
| 3 | 流正常结束但**从无终态事件** | fail（`provider exited without a terminal event`） | ✅ `provider_terminal/failed` |
| 4 | **`drain_provider_events` 早退（stream `Err`：非 0 退出 / spawn / io / codex `bail!`）**，由 spawn 处 `if let Err(error) = result` 兜底 | fail | ❌ **无 attestation** |

第 4 条是最常见的真实失败路径，却是唯一没有执行证明的——**这是本 spec 顺带修的既有缺陷**，也是 §6.4「恰好一个 Result」能否成立的前提。路径 1 在合成 `turn_error`（§4.2.1）落地前实际不会被触发。

验收判据：四条路径都必须产出 ProviderResult + attestation + 终态 writeback 三件套，缺一即红。

---

## 7. 适配器与能力矩阵

### 7.1 Adapter 接口（语义）

保持现有 `ProviderAdapter`，硬化约定：

```text
ProviderAdapter
  provider_type() -> &str              // 现状无此方法，provider 名是 stream_child_events 的 &'static str 入参
  capability() -> ProviderCapability   // 新增，可静态
  start(request, raw_sink) -> ProviderRun
    events: Stream<Result<ProviderEvent, ProviderError>>   // 现状是 anyhow::Result，Phase 1 换成 ProviderError
    handle: cancel()
```

- 流上 `Err(ProviderError)`：**现状**由 `drain_provider_events` 的 `?` 早退 + caller 兜底 fail（§6.4 路径 4）；**目标**是合成 `turn_error` 事件 + 终态 failed（§4.2.1，需 §13 议题 1' 拍板）。
- 非 JSON 噪声行：`parse_line_json` 跳过（现状），噪声与未知 type **必须分开计数**（`unparseable_line_count` / `unmapped_native_count`），混计会让「provider 换了输出格式」和「provider 多打了一行日志」看起来一样（§4.4.1）。

### 7.2 Capability（v1）

```json
{
  "schema_version": "provider.capability.v1",
  "provider_type": "claude-code",
  "session_resume": true,
  "stream_text": true,
  "stream_tools": true,
  "stream_usage": true,
  "structured_error": false,
  "mcp_native": true
}
```

| 字段 | 含义 |
|---|---|
| `session_resume` | 支持续会话 |
| `stream_text` | 文本增量 |
| `stream_tools` | tool_started/completed |
| `stream_usage` | 可靠 usage |
| `structured_error` | 原生错误对象可解析到 code（非仅 exit） |
| `mcp_native` | 原生 MCP 配置路径 |

### 7.3 三家基线声明（实施时以实测修正）

| capability | claude-code | opencode | codex | 依据（2026-08-10 读码核实） |
|---|---|---|---|---|
| `session_resume` | true | true | true | 三家 `build_command` 均有 session/continue 参数 |
| `stream_text` | true | true | true | 三家均产 `TextDelta` |
| `stream_tools` | true | **false** | **false** | 只有 `claude.rs` `parse_assistant_blocks`/`parse_user_blocks` 产 tool 事件；另两家 parser 无任何 tool 分支 |
| `stream_usage` | true | true | true（结构待 golden 确认） | 三家 `TurnCompleted` 都接 `extract_usage`；opencode 走 `part.tokens`，另两家走通用 `usage`/`token_usage` |
| `structured_error` | **false** | **false** | **partial** | claude 顶层 `result.is_error`/`subtype` **当前根本没被读**（1.3.4）——修完才可能置 true；codex 有独立 error/turn.failed 类型但走 `bail!` 丢结构 |
| `mcp_native` | true | true | true | **但机制不同**：只有 claude 走 CLI 参数（`--mcp-config` + `--strict-mcp-config`，会话隔离）；opencode/codex 走**家目录配置文件合并**（`opencode.json` / `.codex/config.toml`，见 `mcp_config.rs`），需注入 + 回滚，残留清单要靠下次会话防御性回滚。建议 v1 拆成 `mcp_native` + `mcp_isolation: "argv" \| "home_file"`，否则产品面会误以为三家隔离性相同 |

\* 若后续补映射，先改 capability 再改 UI/预检假设。  
\*\* `structured_error` 的三家现值全部不高于 partial，因此 §7.4「强审计任务可拒绝无能力 Provider」在 Phase 3 前**无可用判据**，不要提前接进预检闸。

### 7.4 能力驱动行为（产品与调度）

| 场景 | 行为 |
|---|---|
| UI 执行时间线 | `stream_tools=false` 时显示「本提供方仅摘要模式」，不假装有工具轨迹 |
| 强审计任务（可选后续） | predispatch 可拒绝无 `stream_tools` 的 Provider |
| 预算面板 | `stream_usage=false` 时标注「用量可能不完整」 |
| 新 Provider 接入 | 必须提交 capability + golden，否则不许标 production |

### 7.5 Native → Platform 映射表（资产）

每个 adapter 目录或 `docs` 旁路维护映射说明；**以 golden 为可执行真相**。

> **⚠ golden 必须按语义重新推导，不得照抄当前 parser 输出。** 现有 parser 已知至少一处错映射（claude `result.is_error`，见 1.3.4）；若 golden 用「跑一遍现 parser 把输出存下来」的方式生成，等于把缺陷升格为契约，之后修复反而会被门禁判红。生成方式规定为：**取真实脱敏原生行 → 人工按 provider 文档/字段语义写期望事件 → 跑 parser 对齐**，两者不一致时**先判定谁错**。

| 原生（示例） | 平台 type |
|---|---|
| claude `system` + session_id | `session_started` |
| claude `assistant` text / tool_use | `text_delta` / `tool_started` |
| claude `user` tool_result | `tool_completed` |
| claude `result`（`is_error != true`） | `turn_completed` + usage |
| claude `result`（**`is_error == true`**，可带 `subtype`） | **失败终态 + ErrorEnvelope**（现状错映射成 `turn_completed`，Phase 1 修复；golden 必须含此样例）。落地形态随 §13 议题 1'：合成 `turn_error` 事件，或与 codex 一致以 `Err` 早退——两者都必须让 attempt 判失败，不得再判成功 |
| opencode `step_start` | `session_started` |
| opencode `text` | `text_delta` |
| opencode `step_finish` | `turn_completed` + usage |
| codex session/thread id 字段 | `session_started` |
| codex text/delta | `text_delta` |
| codex `turn.completed` 等 | `turn_completed` |
| codex `error` / `turn.failed` 等 | ErrorEnvelope + `turn_error` |

**codex 的现状是「跨类型贪婪兜底」而非映射表**：`extract_session_id` / `extract_text` 对**任何**带 `session_id`/`thread_id`/`text`/`delta`/`content` 键的行都命中，与 `type` 无关（只靠 `drain_provider_events` 里的 session_id 去重兜住重复 `session_started`）。写 codex golden 前须先把这两个函数收敛到显式 type 分支，否则 golden 锁住的是「碰巧能跑」的行为，且 §4.4 的 unmapped 计数对 codex 永远接近 0。

映射变更 = 改 parser + 更新 golden；CP 零改。

---

## 8. Control Plane 消费规则

### 8.1 允许依赖

- `event_type`（平台闭集）
- payload 平台字段（schema 内）
- `ErrorEnvelope.code` / `family` / `retryable`
- `ProviderResult.status` / `usage` / `diagnostics.event_counts`
- `provider_type`（注册表字符串，用于展示与路由，**不是**解析方言的许可证）

### 8.2 禁止依赖

- Claude/OpenCode/Codex 原生 `type` 字符串出现在业务分支
- `failure_summary` 全文 contains 分类（展示可以，决策不行）
- 模型 prose 中的「退出码 0」替代 `tool_completed.is_error`

### 8.3 叙事层（修订：词表现在并不存在，需本批建）

`event_narrative` / `status-labels` **只翻译平台 type 与 family**。

**现状核实**：
- 事件 type → 中文：已有，在 CP 侧 `employee/activity.go` `ActivityEventPresentation`（含 `session_started`/`text_delta`/`tool_started`/`tool_completed`/`turn_completed`）。新增 `turn_error` / `native_unmapped` 需在此补词条 —— 否则会落到 `strings.Contains(lower, "fail")` 的启发式兜底。
- family → 中文：**不存在**。Web 直接渲染 `失败族：{attempt.failure_family}`（`project-execution-trace-panel.tsx`），`status-labels.ts` 无 family 词表，`status-labels.guard.test.ts` 只拦 `.status` / `.risk_level`，因此这处裸英文枚举**至今没有被护栏拦下**。

**本 spec 范围内必须补齐**：
1. `status-labels.ts` 新增 `FAILURE_FAMILY_LABELS` + `failureFamilyLabel()`，覆盖 §5.3 两个生产者的全部族；
2. `project-execution-trace-panel.tsx` 等展示点改走词表；
3. 护栏正则扩到 `failure_family`（与 `.status` 同形），缺词条即红；
4. 新增 family 的四处同步规则见 §5.3 规则 3。

### 8.4 协调线程

| 决策 | 输入 | 现状注意 |
|---|---|---|
| 是否重试 | `error.family` + `retryable` | `projectTaskFailureAction`：`retryable=false` 只是短路，**真正的路由分支由 family 决定**（transient_*/timeout → 重试；invalid_contract/approval_required/... → 等人；business_cancelled/plan_invalid/requirement_changed → 取消；其余 → 失败）。ErrorEnvelope 必须让两者同源，否则 code 对了路由仍错 |
| 是否失败关单 | `ProviderResult.status` + family | |
| 是否等人 | wait-human 状态 / 业务 gate，非扫 text | |
| 预算 | Runtime 已上报的 `BUDGET_FUSE` 等 | **现状断链**：`budget_fuse` CP 不认识 → 走 default「失败」，不进等人也不显示为预算问题（§5.3 / §13 议题 12） |

---

## 9. 双轨证据（重申并硬化）

沿用 2026-07-09，增加可执行规则：

1. **L0 先于 L1**：先 `raw_sink.write_line`，再 parse（现有 `stream_child_events` 已满足，回归测试锁住）。
2. **L2 可截断，L0 不截断**（对象存储 + 指针）。
3. **工具错误以 `tool_completed.is_error` 与进程事实为准**，不以模型 summary 为准。
4. Web：默认 L2 时间线；「原始日志」入口 L0；L0 不作状态机。

---

## 10. 分 Phase 实施

> **修订：Phase 顺序已对调。** 原稿把「五个 schema 文件」放在最前，但仓库没有任何 JSON Schema 校验器（§1.3.10），且 writeback 的 payload/metadata 在 openapi 里是不透明对象（§4.5）——先落 schema 等于先产出一份**没有任何东西在校验它**的文档，与已腐烂的 `contracts/provider/README.md` 同构。改为：**先用错误与终态硬化把字段打出来（同时修 4 个既有缺陷），再把已经跑起来的形状固化成 schema。**

### Phase 1 — 错误与终态硬化（原 Phase B，提前）

**范围**

1. `ProviderError` / `ErrorEnvelope` 贯穿 stream 失败与 exit；`providers/error_map.rs` 落 code →(family, retryable) 单表。  
2. `build_provider_result`：**四条终态路径**（§6.4）合一；补第 4 条缺失的 attestation。  
3. `code → family` 表替换 contains 主路径；`project_task_failure_classification` deprecated。  
4. CP 双读一期：有 `error.family` 用新，否则回退旧 classification。  
5. **顺带修既有缺陷**：claude `result.is_error/subtype` 漏读（1.3.4）；`budget_fuse` 跨层断链（§13 议题 12 拍板后执行）。  
6. 单测：限流 / 取消 / 预算 / 超时 / 非 0 退出 / spawn 失败 / **exit 0 无终态事件**。

**非范围**：schema 文件；三家 tool 对齐；native_unmapped。

**验收**

- [ ] 四条终态路径各产 Result + attestation + 终态 writeback，缺一即红（单测逐条）  
- [ ] 新 fail 路径 100% 带 `failure_family` 且与 envelope 同源  
- [ ] Runtime 主路径无方言 contains 分类  
- [ ] claude `is_error=true` 的 `result` 不再判成功（golden/单测各一条）  
- [ ] **真实 E2E**：至少 1 次真实 provider 失败（建议改坏 binary path 触发 spawn 失败 + 1 次真实非 0 退出），核对 attempt 行的 family/retryable 与 Web 展示  

### Phase 2 — 契约固化与可验证映射（原 Phase A，后置）

**范围**

1. 落 schema 文件（event / error / result / usage / capability / failure-family 词表），README 更新边界与演进规则。  
2. 冻结 v1 type / code 表（本文 §4–§6，字段已在 Phase 1 跑通）。  
3. Golden：每家 Provider ≥ 5 条真实脱敏 stdout 行 → 期望 `ProviderEvent[]`，**按 §7.5 的语义推导法生成**；含 1 条失败样例。  
4. Rust：parser 单测绑定 golden；catalog 暴露静态 `capability()`。  
5. writeback metadata 增加 `schema_version` + `provider_type`；`turn_completed` 带上 `usage`。  
6. `status-labels.ts` family 词表 + 护栏扩展（§8.3）。  
7. 校验器接线（§13 议题 8 定案后）：Node 侧 ajv 进 `verify:contracts`，或 Rust 侧 fixture 反序列化断言。

**验收**

- [ ] `contracts/provider/schemas/*.json` 存在且被**至少一个会红的门禁**消费（不是只存在）  
- [ ] 三家 golden 测试绿，且含各自 ≥1 条失败样例  
- [ ] 改某一家原生字段：只动 adapter + golden 即可让门禁复绿，CP 与 openapi 不被迫改（作为一次真实演练记录在 plan 里）  
- [ ] Web 无裸 `failure_family` 英文枚举，护栏能拦住新增裸渲染  

### Phase 3 — 能力对齐与产品诚实

**范围**

1. codex 贪婪兜底收敛为显式 type 分支（§7.5），之后 `native_unmapped` 计数才有意义。  
2. OpenCode/Codex：有原生 tool 事件则映射；否则 capability 保持 false + UI 文案。  
3. `native_unmapped` / `unparseable_line_count` 开关与 runtime overview 指标；**漂移告警**（§12）。  
4. 可选：L0 离线重放工具（升级映射表不重跑 Provider）。  
5. 预检/文档：强工具审计场景的 Provider 要求（若产品要；注意 §7.3 的 `structured_error` 现值不支持提前接闸）。

**验收**

- [ ] 同任务换 Provider：协调状态机行为一致（成功/失败/重试）  
- [ ] 时间线丰富度可不同，但 UI 不谎报 tool 轨迹  
- [ ] 新 Provider 接入清单（capability + golden + catalog）成文  

### Phase 4 — 生成与扩展（后置，可选）

- schema → Rust/Go 类型生成（typify / oapi 等）评估  
- 派发 payload schema 并入（接 2026-07-19 剩余）  
- `error_code` 持久化列（§13 议题 10 若拍板要做统计告警）  
- 非目标仍成立：不引入新主栈  

---

## 11. 验证与门禁

| 层级 | 手段 | 前置依赖 |
|---|---|---|
| 契约 | JSON Schema 文件 + validate writeback fixtures | **需要校验器，仓库现在没有**：根 `package.json` `devDependencies` 为空、无 ajv；`runtime-agent/Cargo.toml` 无 jsonschema/schemars。见 §13 议题 8 |
| Adapter | golden：原生行 → 事件数组 deep equal | 纯 Rust，**零新依赖**（serde + assert_eq），这是 Phase 2 唯一无依赖的强门禁 |
| Runtime | 单测 ErrorEnvelope 映射；executor 构造 Result（四条终态路径逐条） | 无 |
| CP | 双读测试；禁止新增方言 contains 做 family 决策（`rg` 护栏） | 展示层 contains（`humanizeTechnicalFailureDetail`）不在拦截范围，规则要写清 |
| Web | family 词表护栏（`status-labels.guard.test.ts` 扩 `failure_family`） | 无 |
| 集成 | 既有 runtime smoke / project task 路径：成功 + 一类失败 | 无 |
| 门禁脚本 | 扩展 `verify:contracts`（Node）或 `verify:runtime-agent`（Rust）子检查；**不**手拼未登记命令 | `verify-foundation-contracts.mjs` 现在是纯路径集合比对，加 schema 校验即引入第一个运行时依赖 |

真 E2E：**Phase 1 起**至少一次真实 Provider 失败分类抽检（本 spec 的核心变更就在失败路径，不允许「单测绿=完成」）；Phase 2 以单测 + golden + 契约为主。

**「schema 文件存在」不算门禁**：Phase 2 验收要求 schema 至少被一个会红的检查消费，否则按 §4.5.1 的 S1 强度也未达成。

---

## 12. 风险与缓解

| 风险 | 缓解 |
|---|---|
| writeback 加字段打断旧 Web | 只增可选字段；旧键保留（openapi 侧 payload/metadata 本就是 `additionalProperties: true`） |
| **三家协议漂移（本 spec 最容易自欺的一条）** | **golden 挡不住这个**——它挡的是我们自己的回归。`852ff0d6`（opencode 1.17 事件改名）的真实形态是：上游改名 → parser 静默产 0 事件 → **旧 golden 一直绿** → 只有真实 smoke 才暴露。真正的漂移探针是**运行期信号**：① 已有的「exit 0 无终态事件」兜底（升为 `PROVIDER_NO_TERMINAL_EVENT`，Phase 1）；② `unmapped_native_count` / `unparseable_line_count` 超阈值告警（Phase 3）；③ `event_counts` 全零或只有 text 的 attempt 计数。catalog 记 min version 只是备注，不是探针 |
| family 改名破坏协调 | 不改现有 family 字符串；新增族按 §5.3 规则 3 四处同步 |
| **两个生产者写同一列** | family 保持开放字符串 + 共享词表 + 双侧「常量集 ⊆ 词表」单测（§5.3） |
| 过度统一导致虚假同构 | capability 强制诚实；`structured_error` 三家现值不高于 partial，不得提前接预检闸 |
| 范围膨胀 | Phase 门禁；派发 payload / codegen / `error_code` 列后置到 Phase 4 |
| 双读长期残留 | Phase 1 完成标准含删除 Runtime 侧旧 contains；CP 侧双读设 TODO 日期 |
| **schema 沦为第三份腐烂文档** | §4.5.1 强度必须选定；Phase 2 验收要求 schema 被会红的门禁消费。先例：`contracts/provider/README.md` 挂散文契约腐烂数月无人发现 |

---

## 13. 待人类拍板项

实施前必须确认（默认推荐已标）。**议题 7–12 是 2026-08-10 代码复核新增，7/8/9 未定则 Phase 1/2 无法开工。**

| # | 议题 | 推荐默认 |
|---|---|---|
| ~~1~~ | ~~`turn_error` payload 形状~~ **作废**：`turn_error` 在真实链路上从不出现（1.3.6），讨论其 payload 是伪问题 | — |
| 1'（替代） | stream `Err` 是否合成 `turn_error` 事件上行 CP | **合成**（失败时 L2 时间线才有终止标记）；与 fail writeback 同源同 code，叙事层按 §4.2.1 去重。若不做，则从 v1 type 闭集删除 `turn_error` |
| 2 | `native_unmapped` 默认开还是关 | **默认关**，diagnostics 仍计数；实现方式取「parser 在 `_ =>` 分支自产事件」，不改 parser 签名（§4.4.1） |
| 3 | 超时用 `status=failed+TIMEOUT` 还是独立 `timed_out` | **failed + TIMEOUT** |
| 4 | Phase 2 是否强制 `verify:foundation` 红灯 | **golden 进 `verify:runtime-agent`（零依赖，强制红）；schema 校验待议题 8** |
| 5 | OpenCode/Codex tool 映射进哪个 Phase | **Phase 3**（不挡错误硬化） |
| 6 | 是否改 AGENTS 已知债表述为「部分由本 spec 跟踪」 | **拍板后改一句指针** |
| **7** | **Provider 标识字段收敛到哪个**：`provider_type`(`claude-code`) / `provider_kind`(`claude`) 现同时存在，现网 attestation metadata 用后者，CP 还兼容 `claude_code` 第三写法 | **统一 `provider_type` + 注册表取值，`provider_kind` 退役**；改动面 = `catalog.rs` descriptor、`server.rs` 校验、attestation metadata 键（**旧键需保留一版**，已有 attestation 行不回填）、CP `pg_repository.go` 归一函数 |
| **8** | **JSON Schema 校验器加不加依赖**：仓库当前一个都没有 | **加 ajv 到根 `devDependencies`，只在 `verify:contracts` 用**（Node 侧一处，Rust 侧仍靠 golden deep-equal）。备选：schema 由 Rust `schemars` 生成——省依赖但**与「schema 优先、Rust 是实现」的宪法口径相反**，需人类明确接受才可选 |
| **9** | **schema 校验强度**（§4.5.1）：S1 仅测试 / S2 ingest 打标 / S3 ingest 拒绝 | **S1 起步，Phase 3 评估 S2**；S3 不推荐（runtime 版本落后会批量掉事件） |
| **10** | **`error_code` 是否加 `project_task_attempts` 列** | **Phase 4 再定**；未加之前 §15「可跨 Provider 告警统计」只兑现到「协调可比较」，不承诺统计面 |
| **11** | **`PROVIDER_NO_TERMINAL_EVENT` 是否改判可重试**（现状 `non_retryable_execution`/false → 直接失败） | **改 `transient_provider`/true**：这一类多是上游 schema 漂移或空跑，重试耗尽后由 `max_attempts` 收敛到等人，比静默判死可观测。反对理由是系统性失败会烧 3 次预算——若人类更看重成本则维持现状 |
| **12** | **`budget_fuse` 断链怎么修**（CP 无此常量 → 走 default「失败」，人类看不出是预算问题） | **加 CP 常量 + 路由到 `waiting_human`/`budget_approval` + 中文 lead + 词条**（复用已存在的 `HumanWaitReasonBudgetApproval` 与 `project_task_budget_approval` 决策动作）。备选：runtime 改产已有族——信息量更低，不推荐 |

---

## 14. 现状锚点（实施时省勘察）

路径为 2026-08-10 复核时核实；**行号会漂移，按符号名定位**。

| 项 | 路径 / 符号 |
|---|---|
| 统一事件 | `apps/runtime-agent/src/events.rs`（`ProviderEvent`、`EXCERPT_LIMIT_BYTES=4096`、`truncate_excerpt`） |
| 流管道 | `apps/runtime-agent/src/providers/mod.rs`（`stream_child_events`、`parse_line_json`、`provider_exit_result`） |
| Claude/OpenCode/Codex parser | `apps/runtime-agent/src/providers/{claude,opencode,codex}.rs` |
| **claude 漏读 is_error** | `claude.rs` `parse_claude_event` 的 `"result"` 分支（对照 `parse_user_blocks` 里读了 tool_result 的 `is_error`） |
| **codex 贪婪兜底** | `codex.rs` `extract_session_id` / `extract_text`（与 `type` 无关地命中） |
| 用量 | `apps/runtime-agent/src/providers/usage.rs`；消费点 `executor.rs` `usage_tokens`（预算心跳 + result contract） |
| Catalog / 标识三写法 | `apps/runtime-agent/src/providers/catalog.rs`（`provider_type` vs `provider_kind`）、`server.rs` `validate_run_spec`、CP `employee/pg_repository.go` 的 `claude-code\|claude_code\|claude` 归一 |
| writeback 映射 | `apps/runtime-agent/src/commands/executor.rs` `runtime_event_writeback`（注意 `TurnCompleted` 分支丢弃 usage） |
| 旧失败分类 | 同文件 `project_task_failure_classification` + `project_task_fail_writeback` |
| **四条终态路径** | 同文件 `drain_provider_events`（尾部 match）+ spawn 处 `if let Err(error) = result`（第 4 条，无 attestation） |
| attestation 构造 | 同文件 `record_attestation` 调用点（3 处有、1 处缺） |
| **CP family 词表** | `apps/control-plane/internal/project/types.go` `FailureFamily*`（16 个常量） |
| **CP family 路由** | `project/service.go` `projectTaskFailureAction` / `projectTaskAttemptFailureStatus` / `humanWaitReasonForFailureFamily` |
| **CP 中文兜底** | `project/service.go` `humanReadableFailureSummary` / `humanizeTechnicalFailureDetail` |
| CP 事件 type → 中文 | `apps/control-plane/internal/employee/activity.go` `ActivityEventPresentation` |
| CP writeback | `apps/control-plane/internal/employee/run_writeback.go` `RecordEvent` |
| **wire 契约（payload/metadata 不透明）** | `contracts/control-plane/openapi.yaml` `RuntimeCommandEventWritebackRequest` / `FailProjectTaskAttemptRequest`（`failure_family` 是无 enum 的 string） |
| **Web 裸枚举现场** | `apps/web/src/features/projects/components/project-execution-trace-panel.tsx`（`失败族：{attempt.failure_family}`）；词表 `apps/web/src/lib/status-labels.ts`（**无 family 条目**）；护栏 `status-labels.guard.test.ts`（只拦 `.status`/`.risk_level`） |
| 校验器现状 | 根 `package.json`（`devDependencies` 为空）、`apps/runtime-agent/Cargo.toml`（无 schema 库）、`scripts/verify-foundation-contracts.mjs`（纯路径集合比对） |
| Provider 散文契约 | `contracts/provider/README.md`（已指向本 spec） |
| 契约验证立项 | `docs/superpowers/specs/2026-07-19-runtime-provider-contract-verification.md` |
| Transcript 已落地 | `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md` |
| 架构债声明 | `AGENTS.md` 已知债段 |

---

## 15. 完成态画像（验收北极星）

> **任意新 Provider 接入后：协调线程、重试策略、预算、审批、审计、Web 时间线无需改业务代码；差异只体现在 capability 与时间线丰富度；所有终态错误可跨 Provider 比较。**

（原稿写的是「可比较**与告警**」。告警需要 `error_code` 可查询，而 Phase 1/2 的零迁移约束把 code 只放在事件 payload 里；**告警统计面在 §13 议题 10 拍板加列之前不承诺**。）

| 维度 | 现在 | 完成态 |
|---|---|---|
| 统一事件 | Rust enum；`turn_error` 死类型 | schema 版本化 envelope + 兼容 writeback；失败有终止事件 |
| 映射 | 三家 parser，至少 1 处已知错映射 | golden 守护（语义推导，非行为抄录） |
| 错误 | 字符串 + contains（且是自伤式往返） | ErrorEnvelope + code→family 单表 |
| 终态 | 四条路径、attestation 缺一条 | 单一 `build_provider_result`，四路径三件套齐全 |
| family | 两个生产者、词表不重合、`budget_fuse` 断链 | 一份词表 + 四处同步规则 + 中文词条 |
| 契约 | 散文 | JSON Schema + 会红的门禁（强度见 §4.5.1） |
| 能力 | 隐式 | Capability matrix（含 MCP 隔离机制差异） |
| CP | 吃 string event_type | 只吃平台语义 |
| 标识 | `provider_type`/`provider_kind`/`claude_code` 三写法 | 单一 `provider_type` |

---

## 16. 建议审查关注点

1. L0–L4 分层是否与 AGENTS 控制平面边界一致。  
2. v1 type/code 闭集是否过粗/过细（family 已定为开放字符串 + 词表，见 §5.3）。  
3. Phase 切分是否可独立交付与回滚——**注意顺序已对调**，理由见 §10 引言。  
4. 事实源边界：wire 契约在 control-plane openapi、语义契约在 `contracts/provider`（§4.5.1），与 2026-07-19 P2 的分工是否清晰。  
5. 默认「不强制三家 tool 同构」是否符合产品预期。  
6. **本 spec 顺带修 4 个既有缺陷**（claude `is_error` 漏读 / 第 4 条终态路径无 attestation / `budget_fuse` 跨层断链 / Web 裸 family 枚举）——是否接受它们进入本批范围，还是拆出独立修复。

审查意见请直接批注本节与 §13 拍板项；拍板后另开 `docs/superpowers/plans/2026-…-provider-semantic-unification.md` 实施。

---

## 17. 修订记录

### 2026-08-10 — 对照代码复核后修订（未实施，仅改设计稿）

**改正的事实错误（原稿与代码不符）：**

1. `turn_error` 被当成活事件讨论（原 §13 议题 1）→ 实为死类型，真实链路从不产出（新 1.3.6 / §4.2.1）；原议题作废，替换为「stream `Err` 是否合成上行」。
2. `provider_kind` 例值写作 `claude-code` → 仓库里 `provider_kind` 是 `claude`、`provider_type` 才是 `claude-code`，原稿等于制造第四种写法（新 1.3.8 / §4.1 前置条件 / §13 议题 7）。
3. §5.3「与现网对齐的 family 表」只有 6 个 → 实为 runtime 生产子集，CP 侧另有 16 个常量，且 `budget_fuse` 跨层断链（重写 §5.3 / §13 议题 12）。

**补入的遗漏：**

4. 仓库没有任何 JSON Schema 校验器（§1.3.10 / §11 / §13 议题 8）。
5. schema 与 control-plane openapi 的事实源边界、校验强度三档（新 §4.5.1 / §13 议题 9）。
6. golden 挡不住上游协议漂移，需运行期探针（重写 §12 对应行 + Phase 3 范围）。
7. `native_unmapped` 的真实实现成本：parser 空 `Vec` 语义二义、codex 贪婪兜底致计数失真（新 §4.4.1）。
8. 终态是四条路径且第 4 条无 attestation（新 §6.4 表）。
9. 中文文案归属：Rust 不产中文，避免与 CP 两处 + Web 词表打架（新 §5.5）。
10. family 中文词表**根本不存在**，Web 在渲染裸英文枚举且护栏拦不住（重写 §8.3）。
11. §15「可告警」与零迁移互斥，需 `error_code` 列（头部说明 + §13 议题 10 + §15 括注）。

**结构调整：** Phase A/B 对调为 Phase 1（错误与终态硬化）/ Phase 2（契约固化），理由见 §10 引言；Phase 编号由字母改数字，全文引用同步。

**未改动：** §2 分层、§3 契约包布局、§9 双轨证据、§0 一句话方案（复核认为均成立）。
