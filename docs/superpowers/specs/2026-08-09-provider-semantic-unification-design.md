# Provider 语义统一层（事件 / 结果 / 错误 / 能力）

- 日期：2026-08-09（**2026-08-10 按代码复核修订**，见 §17 修订记录）
- 状态：**Phase 1–4 已落地**（`main`：`5278718b` / `aa2c478c` / `c23391aa` + 后续 follow-up）。本文保留为架构事实源；实施细节以代码与 `contracts/provider/` 为准。
- 系列：
  - 承接已落地：`2026-07-09-provider-transcript-tool-event-capture-design.md`（transcript / tool 事件 / raw 双轨）
  - 承接：`2026-07-19-runtime-provider-contract-verification.md`（本 spec 吸收并细化其 P2 Provider 侧；P1 runtime openapi 仍可独立推进）
  - 对齐架构约定：`AGENTS.md`「Provider 协议必须语言无关」与已知债 `contracts/provider/`
- 交付性质：原为架构设计；现 **Phase 1–4 已实施**。剩余见下方「未决 / 跟进」。
- 目标读者：架构评审 / 实施与复盘会话
- **迁移**：Phase 1/2 零迁移。**Phase 4 已加** `project_task_attempts.error_code`（可空，历史不回填），§15 告警统计面可对上线后新 attempt 查询。

### 未决 / 跟进（实施后仍开）

| 项 | 说明 |
|---|---|
| §13 议题 7 退役 | **完成**：公开 Run API 仅 `provider_type`；HTTP create 仍兼容 deprecated `provider_kind` 入参 |
| golden 数量 | **已达 ≥5/家**（硬门槛） |
| 校验强度 | **S1 + S2 jsonschema v6**（不可用时结构回退）；`/health.provider_contract` 暴露计数与引擎 |
| 漂移告警 | **`ALERT provider_stream_drift`** + Runtime health `provider_stream` 计数 |
| 跨 Provider E2E | **脚本门禁化**：`pnpm e2e:provider-semantic`（opt-in，需真服务+假 binary） |
| Runtime family ⊆ 词表 | **完成**：`error_map` 单测断言 `family::*` ⊆ `failure-family.json`（对齐 CP） |
| L3 diagnostics | **完成**：终态 `ProviderResult` + runtime command terminal `diagnostic` 写入 `unmapped_native_count` |
| spawn 失败 E2E / daemon health 探针 / L0 离线重放 | **延后**，见根 `TODO.md` |

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
| 统一事件枚举 | `apps/runtime-agent/src/events.rs` `ProviderEvent` | session / text / tool / turn / `turn_error`（stream Err 合成上行）/ `native_unmapped` |
| 三家 parser | `providers/claude.rs` · `opencode.rs` · `codex.rs` | 原生 JSON 行 → `ProviderEvent`；未知 type → `native_unmapped` |
| 共享流管道 | `providers/mod.rs` `stream_child_events` | raw 先落、再 parse、退出合成错误；unmapped/unparseable 分计 |
| 用量 best-effort | `providers/usage.rs` | usage / token_usage 字段归一；预算心跳与 result contract 消费 |
| writeback | `commands/executor.rs` `runtime_event_writeback` | → CP `event_type` + payload；`schema_version` + `provider_type` 元数据 |
| 结构化错误 | `providers/error_map.rs` + `ErrorEnvelope` | code→(family, retryable) 单表；fail writeback 同源 |
| L3 终态 | 终态 writeback 的 `status` / `error_code` / `error_family` / `diagnostic` + `events.rs` `attempt_stream_diagnostics` | 四条终态路径 + attestation；diagnostics 含 `unmapped_native_count`。**不传独立 ProviderResult 对象**，见 §6.4 |
| 契约包 | `contracts/provider/schemas/*` + golden + fixtures | ajv S1（`verify:contracts`）+ CP S2 打标 |
| family 词表 | `failure-family.json` + CP/Runtime ⊆ 单测 + Web `failureFamilyLabel` | 跨层共享；`budget_fuse` 路由 waiting_human |
| raw 证据轨 | 2026-07-09 已落地 | 原始流不因 parse 丢弃 |
| 零终态兜底 | `executor.rs` `drain_provider_events` 尾部 | `PROVIDER_NO_TERMINAL_EVENT` → `transient_provider` / **可重试**（§18-1 临时不可重试已随 2026-08-10 派发修复翻回） |

### 1.3 实施前的结构性缺口（历史；Phase 1–4 后状态）

> 以下为 2026-08-10 复核时的现场证据。实施后多数已关；保留以便理解设计动机，**不得当作当前待办**。

1. **契约债** → **已关**：`contracts/provider/schemas/*` + golden + `verify:contracts`（ajv）消费；AGENTS 已知债已改为 runtime openapi 门禁仍债。
2. **错误以字符串穿透 / 自伤式往返** → **已关**：`error_map::map_code` 主路径；legacy string 仅 fallback。
3. **能力不诚实 / 未知行静默丢** → **基本关**：`native_unmapped` + 漂移告警 + capability / 摘要模式 UI；tool 全量对齐仍非目标。
4. **claude `result.is_error` 漏读** → **已关**（golden + 单测）。
5. **四条终态 attestation 缺 Path4** → **已关**：stream `Err` 合成 `turn_error` + attestation + fail。
6. **`turn_error` 死类型** → **已关**（合成上行，与 fail 同源 code）。
7. **family 两生产者不重合 / `budget_fuse` 断链** → **已关**：共享词表 + CP 常量/路由/中文 + 双侧 ⊆ 单测。
8. **`provider_type` / `provider_kind` 三写法** → **基本关**：公开面注册表 `provider_type`；create 仍兼容 deprecated `provider_kind`。
9. **family 裸英文** → **已关**：`failureFamilyLabel` + 护栏扩 `failure_family`。
10. **验证缺失 / 无 ajv** → **已关**：golden≥5/家、ajv fixtures、S2 打标、opt-in E2E 脚本。

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

> **⚠ 本节的嵌套信封形状仍未达成**（2026-08-10）：**批一已实施**——`schema_version`/`type`/`ts`/`seq`/`provider_type`/`provider_session_id`/`attempt_ref` 已随每条事件写回，但**与业务键同层（扁平）**，不是本节画的 `payload` 嵌套形状；`provenance` 未做（需 parser 透出原生 type）。`seq`/`type` 是**冗余投影**，外层 `sequence_number`/`event_type` 才是真相。嵌套化与读路径迁移是批二，见 `docs/superpowers/specs/2026-08-10-l2-event-envelope-decision.md`。**不得据批一宣布本节完成。**

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
| `PROVIDER_SPAWN_FAILED` | 二进制不存在/无权限 | `provider_configuration` | false（**生产近乎不可达，见 §5.2.1**） |
| `PROVIDER_PROTOCOL_ERROR` | 流协议致命错误 / codex 原生 error 行 | `non_retryable_execution` | false |
| `PROVIDER_NO_TERMINAL_EVENT` | **exit 0 但全程无 TurnCompleted/TurnError**（输出格式漂移被解析层全量丢弃的典型形态） | `transient_provider`（保留诊断信息量） | **true**——派发/归因已修好（2026-08-10），恢复 §13 议题 11 原判；临时 `false` 见 §18-1 已销账 |
| `RATE_LIMIT` | 限流 | `transient_provider` | true |
| `AUTH_FAILED` | 鉴权/配额账号 | `provider_configuration`（CP 已有中文 lead「执行器配置有误」，比 `non_retryable_execution` 信息量高） | false |
| `TIMEOUT` | 超时 | `timeout` | true |
| `BUDGET_FUSE` | 墙钟/token 熔断 | `budget_fuse`（**CP 目前不认识此族**，落 default「失败」，见 §13 议题 12） | false |
| `CANCELLED` | 操作者取消 | `business_cancelled` | false |
| `WORKSPACE_INVALID` | 工作区/同步/hash | `invalid_contract` | false |
| `WAITING_HUMAN` | 需人类（若走错误通道） | （通常走 wait-human 状态，不是 fail） | false |
| `UNKNOWN` | 无法归类 | `non_retryable_execution` | false |

#### 5.2.1 `PROVIDER_SPAWN_FAILED` 被健康闸前置遮蔽（2026-08-10 实测结论）

派发前的预检闸会先看提供方健康：`health.rs` `probe_provider_health` 跑 `<bin> --version`（3 秒超时），探不通就以 `reason_code=provider_unavailable` **拦下派发**，任务停在 `waiting_human` / `attempts=0`，**根本产生不了 attempt**。

而 `PROVIDER_SPAWN_FAILED` 的触发条件（文件不存在 / 无执行权限 / shebang 无效）**同时会让 `--version` 失败**。因此：

- 它只在**健康快照与实际派发之间发生漂移**时可达（binary 在两者之间被删或被改权限），是防御性兜底而非常规路径；
- 想为它造稳定的 E2E，需要同一个 binary「`--version` 成功但 `spawn()` 失败」，在正常文件系统语义下自相矛盾（卡在探测与派发之间改权限是竞态，不是用例）。

**结论：不为该 code 立 E2E**，保留 `error_map` 单测即可。曾挂在 `TODO.md` 的「spawn 失败专项 E2E」据此撤销。附带判别器：任何"批准了却不派发 / 任务卡在 attempts=0"的现场，先看 `project_events` 里 `project_task.dispatch_blocked` 的 `reason_code`，别先怀疑审批链路。

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

每个 attempt **恰好一个终态**，在：

- 收到 `turn_completed` 且进程成功退出，或  
- 流错误 / 非 0 退出 / 取消 / 预算熔断 / wait-human  

**L3 的落地形态（2026-08-10 修订）**：终态语义投影到终态 writeback 的 `status` / `error_code` / `error_family` / `diagnostic` 四个字段，**Runtime 不再额外传一个 `ProviderResult` 对象**。原实现构造了 `ProviderResult` 却在每条路径上 `let _ =` 丢弃（纯装饰、无编译警告、会静默腐烂），已删除该结构体。`provider-result.schema.json` 与 fixtures 保留为对外契约描述，是否让它真正上行（并定下消费者）留待后续立项。

**终态路径共五条，全部必须产出「终态 writeback + attestation」**：

| # | 路径 | 终态回写 | attestation | L2 终止标记（`turn_error`） |
|---|---|---|---|---|
| 1 | 事件循环内 `TurnError` | fail | ✅ | ✅ 事件本身 |
| 2 | 流正常结束 + `TurnCompleted` | complete | ✅ succeeded | 不适用 |
| 3 | 流正常结束但**从无终态事件** | fail（`PROVIDER_NO_TERMINAL_EVENT`） | ✅ | ✅ 2026-08-10 补 |
| 4 | `drain_provider_events` 早退（stream `Err`：非 0 退出 / io / codex `bail!`） | fail | ✅ | ✅ |
| 5 | 预算墙钟熔断（心跳线程） | fail（`BUDGET_FUSE`） | ✅ 2026-08-10 补（此前**只有** `provider_start`，终态无证明，真实 E2E 才发现——本表初版曾错标为已有） | ✅ 2026-08-10 补 |
| — | `provider.start()` 失败（spawn） | fail（`PROVIDER_SPAWN_FAILED`） | ✅ `provider_start/failed` | 不适用（provider 从未启动） |

**顺序是承重的**：CP 在 run 进入终态后拒收 run 事件，所以 `turn_error` 必须**先于**终态 writeback 发出，否则被静默丢弃、时间线在流中间戛然而止。实现收敛在 `executor.rs` `emit_turn_error_marker`，标记失败只记日志、不影响终态（终态自带 code/family）。

验收判据：五条路径都必须产出 attestation + 终态 writeback；1/3/4/5 另须有 `turn_error` 上行。

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

> **修订：Phase 顺序已对调（历史决策）。** 实施时先错误/终态硬化（Phase 1），再固化 schema（Phase 2）并接 ajv。writeback 的 payload/metadata 在 openapi 里仍是不透明对象（§4.5）——语义契约在 `contracts/provider`，wire 在 control-plane openapi。

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

- [x] 四条终态路径各产 Result + attestation + 终态 writeback，缺一即红（单测逐条）  
- [x] 新 fail 路径 100% 带 `failure_family` 且与 envelope 同源  
- [x] Runtime 主路径无方言 contains 分类  
- [x] claude `is_error=true` 的 `result` 不再判成功（golden/单测各一条）  
- [x] **真实 E2E**：假 binary `RATE_LIMIT` → attempt `error_code`/`failure_family=transient_provider`（spawn 失败专项 E2E 见 TODO 延后）  

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

- [x] `contracts/provider/schemas/*.json` 存在且被**至少一个会红的门禁**消费（不是只存在）  
- [x] 三家 golden 测试绿，且含各自 ≥1 条失败样例  
- [x] 改某一家原生字段：只动 adapter + golden 即可让门禁复绿，CP 与 openapi 不被迫改（作为一次真实演练记录在 plan 里）  
- [x] Web 无裸 `failure_family` 英文枚举，护栏能拦住新增裸渲染  

### Phase 3 — 能力对齐与产品诚实

**范围**

1. codex 贪婪兜底收敛为显式 type 分支（§7.5），之后 `native_unmapped` 计数才有意义。  
2. OpenCode/Codex：有原生 tool 事件则映射；否则 capability 保持 false + UI 文案。  
3. `native_unmapped` / `unparseable_line_count` 开关与 runtime overview 指标；**漂移告警**（§12）。  
4. 可选：L0 离线重放工具（升级映射表不重跑 Provider）。  
5. 预检/文档：强工具审计场景的 Provider 要求（若产品要；注意 §7.3 的 `structured_error` 现值不支持提前接闸）。

**验收**

- [x] 同任务换 Provider：协调状态机行为一致（成功/失败/重试）——`e2e:provider-semantic` 对 claude-code + opencode 假 binary 校验  
- [x] 时间线丰富度可不同，但 UI 不谎报 tool 轨迹（摘要模式文案）  
- [x] 新 Provider 接入清单（capability + golden + catalog）成文（`contracts/provider/ONBOARDING.md`）  
- [ ] L0 离线重放工具（**延后**，TODO）  

### Phase 4 — 生成与扩展（后置，可选）

- [x] `error_code` 持久化列（议题 10 已做）  
- [x] 派发 payload schema 起步（`start-session-payload.schema.json` + fixture）  
- [ ] schema → Rust/Go 类型生成（typify / oapi 等）评估——本批不接，见 `CODEGEN.md`  
- 非目标仍成立：不引入新主栈  

---

## 11. 验证与门禁

| 层级 | 手段 | 前置依赖 / 现状 |
|---|---|---|
| 契约 | JSON Schema + ajv 校验 fixtures（S1） | **已落地**：根 `devDependencies.ajv`；`scripts/verify-foundation-contracts.mjs` 校验 `contracts/provider` |
| Adapter | golden：原生行 → 事件数组 deep equal（≥5/家） | 纯 Rust，进 `verify:runtime-agent` |
| Runtime | ErrorEnvelope 映射单测；`family::*` ⊆ 词表；executor 四终态 + diagnostics | 无额外依赖 |
| CP | `FailureFamily*` ⊆ 词表；S2 ErrorEnvelope 打标；health `provider_contract` | 展示层 contains 不在 family 决策护栏范围 |
| Web | `failureFamilyLabel` + `status-labels.guard` 扩 `failure_family` | 无 |
| 集成 | opt-in `pnpm e2e:provider-semantic`（真服务 + 假 binary） | 需 dev services |
| 门禁脚本 | `verify:contracts` / `verify:runtime-agent` / `verify:control-plane` 子检查 | 不手拼未登记命令 |

真 E2E：假 binary `RATE_LIMIT` 路径已抽检；spawn 失败专项与 daemon 健康探针见 TODO 延后。

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

## 13. 拍板项（实施期已按推荐默认落地）

| # | 议题 | 决议 / 落地 |
|---|---|---|
| ~~1~~ | `turn_error` payload 伪问题 | 作废 |
| 1' | stream `Err` 合成 `turn_error` | **已落地** |
| 2 | `native_unmapped` 默认关；diagnostics 仍计数 | **已落地**（L3 + terminal `diagnostic`） |
| 3 | 超时 `failed + TIMEOUT` | **已落地** |
| 4 | golden 进 `verify:runtime-agent` | **已落地** |
| 5 | tool 映射 Phase 3 | **已落地**（诚实 capability / 摘要模式） |
| 6 | AGENTS 已知债 | **已落地**（精简宪法 + 债表述更新） |
| 7 | 统一 `provider_type` | **已落地**（create 仍兼容 deprecated `provider_kind`） |
| 8 | 加 ajv | **已落地** |
| 9 | S1 + S2 打标 | **已落地**（S3 仍不做） |
| 10 | `error_code` 列 | **已落地**（Phase 4） |
| 11 | `PROVIDER_NO_TERMINAL_EVENT` → transient 可重试 | **已翻回可重试**（2026-08-10 派发/归因修复落地后）：临时不可重试见 §18-1；再派发与任务级首因归因见 `2026-08-10-retry-redispatch-and-failure-attribution.md` |
| 12 | `budget_fuse` → waiting_human + 中文 | **已落地** |

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
| **Web family 词表** | `apps/web/src/lib/status-labels.ts` `failureFamilyLabel`；护栏 `status-labels.guard.test.ts` 含 `failure_family` |
| 校验器现状 | 根 `ajv`；`verify-foundation-contracts.mjs` 校验 provider fixtures；CP S2 `jsonschema` 打标 |
| Provider 契约 | `contracts/provider/{schemas,golden,fixtures,README,ONBOARDING}.md` |
| 契约验证立项 | `docs/superpowers/specs/2026-07-19-runtime-provider-contract-verification.md` |
| Transcript 已落地 | `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md` |
| 架构债声明 | `AGENTS.md`（schemas 已落地；runtime openapi 门禁仍债） |

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

### 2026-08-10 — Phase 1–4 落地后收口（P0 文档 + P1 护栏）

1. §1.2 / §1.3 改为「现状能力」与「历史缺口+已关状态」，去掉「仓库无 ajv」等过时断言。  
2. §10 验收勾选与 Phase 4 已做项对齐；L0 重放 / spawn E2E 记入 TODO。  
3. §11 / §13 / §14 同步实施态。  
4. Runtime：`family::*` ⊆ `failure-family.json` 单测；终态 `ProviderResult` / terminal `diagnostic` 写入 `unmapped_native_count`。

---

## 18. 独立复查与回归（2026-08-10，另一会话实施后）

复查方式：读码对照 + 提交态门禁（独立 detached worktree，避开工作树在途改动）+ 真实链路 E2E（假 provider + 真 claude + 浏览器）。

**门禁修复**：`a3e4d43a` 在 `RunSpec` 字段收敛时于测试块留下重复字段，main 上 `cargo test` 直接编译失败（E0062），`verify:runtime-agent` / `verify:foundation` 一直是红的（非测试构建不受影响，所以运行中的服务看不出来）。已修（`9bf01fa2`）。

### 18-1 议题 11 翻转：`PROVIDER_NO_TERMINAL_EVENT` 曾临时不可重试（已销账）

原判据「重试耗尽后由 max_attempts 收敛到等人，比静默判死可观测」被真链路证伪：**重试根本没重跑**。实测（需求 `6665b38e`）：attempt #1 正确分类后 requeue，attempt #2/#3 从未产生 runtime 命令（`runtime_events` 在 #1 结束后再无记录），双双空转到看门狗 `lost`，12 分钟后任务落 `waiting_human` / `runtime_recovery`——**真因被最后一次尝试的族盖掉**，人类被指向运行环境而非 provider 输出漂移。同形态在另一批 8 条 `RATE_LIMIT` 任务上复现，属既有派发缺陷，被本 spec 的改判放大到必经路径。

人类 2026-08-10 曾确认临时翻回 `retryable=false`。**派发再送 + 任务级首因归因**已在 `2026-08-10-retry-redispatch-and-failure-attribution.md` 落地，议题 11 现已翻回 `transient_provider` / `retryable=true`。

### 18-2 本次一并修的三项

| 项 | 问题 | 处置 |
|---|---|---|
| L3 空壳 | `ProviderResult` 在全部终态路径上构造后 `let _ =` 丢弃（`let _` 抑制警告，会静默腐烂），注释却称其为唯一终态事实 | 删除结构体与构造函数；§6.4 改写为「L3 投影到终态 writeback 四字段」 |
| L2 终止标记缺失 | `turn_error` 只在「stream `Err`」一条路径上行；尾部无终态、预算熔断两条先写终态再发事件，CP 因 run 已终态拒收且错误被 `let _` 吞掉 → 时间线戛然而止（两次真实 E2E 各证一次） | 抽出 `emit_turn_error_marker`，统一在终态写回**之前**发；标记失败只记日志 |
| 脏 provider_type | 预算心跳与 legacy `fail()` 硬编码 `"unknown"`，而 sink 自己就有该字段 | 改用 `writeback.provider_type` |

另：宪法（`AGENTS.md`）在 `6ad60929` 被顺手压缩，丢了两条硬教训，已补回——worktree 未共享 PID_DIR 时 `restart` 是退出码 0 的静默空操作；禁止 `npx playwright install` / `npx vitest run`。

### 18-3 仍未关闭

- L2 envelope 未实现、schema 描述过渡态、golden `expected_events` 无 schema 校验 → 立项 `docs/superpowers/specs/2026-08-10-l2-event-envelope-decision.md`（**已拍板做实**，分两批；`seq` 外层为唯一真相）。
- 重试再派发与失败归因 → 立项见 18-1。
- ~~预算熔断路径的两项修复未独立取证~~ **已取证 2026-08-10**（attempt `305788f7…`）：`turn_error` 标记上行、envelope `provider_type=claude-code`（不再是 `unknown`）、路由 `waiting_human`/`budget_approval`、attestation `provider_start/succeeded,provider_terminal/failed` 齐全。
  - **此前判定"被审批闸卡住"是误诊**：真实阻塞原因是 `project_task.dispatch_blocked / reason_code=provider_unavailable`——健康探测用 `<bin> --version`（3 秒超时），而当时的假 provider 一律长睡/不应答，被判提供方不可用；"补建等待人工决策卡"是系统对被阻塞派发的正常反应，不是僵尸卡。假 provider 已加 `--version` 应答（`scripts/e2e/fake-providers/`），并把这个坑写进其 README。
  - 该次 E2E 顺带发现并修掉：预算熔断路径**缺终态 attestation**（见 §6.4 路径 5）。
