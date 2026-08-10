# L2 事件 envelope 做实

- 日期：2026-08-10
- 状态：**批一已实施（E2E 取证通过），批二待做**
- 来源：`2026-08-09-provider-semantic-unification-design.md` 复查（§18-3）
- 迁移：**零迁移**（`task_events.payload` 是 JSONB；历史行永久保持扁平，靠双读兼容）

---

## 0. 已拍板

| # | 议题 | 结论 |
|---|---|---|
| 1 | 正式降级为扁平事件，还是做实 §4.1 信封 | **做实** |
| 2 | `seq` 以信封为真相，还是外层 `sequence_number` 为真相 | **外层 `sequence_number` 是唯一真相**；信封内的 `seq` 只是冗余投影，二者不一致时**以外层为准**，并在 schema 里写死这句话 |

议题 2 的理由：外层那个号是 CP 用来**去重（`HasRunEventSequence`）、排序、落 `task_events.sequence_number` 列**的实际依据，所有现存消费者读的都是它；信封内那份的唯一用途是让一条事件**脱离请求上下文时仍能自解释**（离线重放、导出分析）。换真相要动去重与排序，收益为零。

配套必须写进 schema 的两条事实：

- **单调但不连续**：被抑制的 `native_unmapped` 事件会消耗号但不上行，实测时间线为 1,3,4,…,8,10。消费者不得假设连续。
- **终态事件不在此号段**：CP 自己写的 `run_completed` / `run_failed` 用保留号 `2147483600+`，与 runtime 的计数器天然不同源，信封语义不覆盖它们。

## 1. 现状（复查实测）

spec §4.1 定义的信封（`schema_version`/`type`/`ts`/`seq`/`provider_type`/`provider_session_id`/`attempt_ref`/`payload`/`provenance`）**没有实现**。实际走的是 §4.5 过渡策略：

- 请求体外层：`event_type` + `sequence_number` + `payload`（扁平业务键）；
- `metadata`：`source` / `schema_version=provider.event.v1` / `provider_type`；
- `ts` / `attempt_ref` / `provenance` **不存在**。

`contracts/provider/schemas/provider-event.schema.json` 已被写成**扁平并集**（`required` 只有 `type`，所有 payload 键平铺），描述的是过渡态实现；golden 的 `expected_events` 是 Rust 枚举序列化形状，因此 `verify:contracts` 至今**只校验 `native_lines` 非空**，没拿 schema 校验过 `expected_events`。

## 2. 承重约束（决定了必须分两批）

1. **历史行永远扁平**。库里已有事件行没有 `ts`/`attempt_ref`，没有任何迁移能回填。所有信封读路径必须**双读**，且这个"过渡期"实际是永久的。
2. **不能同时保留扁平键与嵌套副本**。tool 事件的 `input_excerpt`/`output_excerpt` 上限各 4096 字节；若过渡期在同一 payload 里既留扁平键又塞一份嵌套 `envelope.payload`，**每条 tool 事件体积翻倍**，而 `task_events` 是热表并经 WS 广播。
3. **`provenance.native_type` 不是免费的**。`ProviderEvent` 枚举当前不携带原生类型（只有 `native_unmapped` 变体带），要填 `provenance` 得改 parser 让原生 type 透出来——单独一件事，不进第一批。
4. **S2 校验范围会扩大**。现在 CP 的 jsonschema 只校验 ErrorEnvelope（`schema_violation_count=0` 正是因为事件基本没进校验）；信封做实后每条事件都可能进校验，噪声与开销要先想清楚。

## 3. 分两批

### 批一：写入侧补齐信封字段（读路径零改动）—— ✅ 已实施 2026-08-10

落地位置：`executor.rs` `runtime_event_writeback` 与业务键同层写入 `schema_version` / `type` / `ts` / `seq` / `provider_type` / `provider_session_id` / `attempt_ref{command_id, attempt_id}`；`ts` 由 `record.recorded_at_ms` 经 `time` crate 转 RFC3339（越界返回 None 而不是伪造时间戳）；`provider_type` 走 `canonical_provider_type` 归一（短名 `claude` → `claude-code`）。metadata 原样保留，未删任何旧键。

**实施期发现的一个契约错配**（原方案没预见）：golden 的 `expected_events` 在 Rust 侧是**子集断言**（只校验列出的键），而 schema 校验是**完整实例**校验，两者口径不同。处置：

- `provider-event.schema.json` 定为**解析形态**（parser 产出；信封字段可选），golden 用它校验；
- 新增 `provider-event-envelope.schema.json` 定为**写回形态**（`allOf` 解析形态 + `required: schema_version/type/seq/ts`），fixture `event-envelope-tool-started.json` 用它校验；
- 含 `error` 的 golden 需把期望写成完整 envelope（已改 `claude-code/02-result-is-error.json`），Rust 的子集断言不受影响。

`ts` 用 `pattern` 而非 `format: date-time`——仓库的 ajv 没装 ajv-formats，`format` 只会被忽略并打警告，`pattern` 才是真会红的那一个。

**保持扁平**，在 payload 同层补上缺的信封字段：

| 字段 | 来源 | 备注 |
|---|---|---|
| `schema_version` | 已有（现在在 metadata） | 移到/同时放 payload 同层 |
| `type` | 已有（外层 `event_type`） | 冗余投影，同 `seq` 规则：**外层为准** |
| `ts` | Runtime 主机时钟，RFC3339 | 新增 |
| `seq` | run store 计数器 | **冗余投影，外层 `sequence_number` 为准** |
| `provider_type` | 已有（metadata） | 注册表口径（`claude-code`/…） |
| `provider_session_id` | 已有上下文 | 已知则填 |
| `attempt_ref` | `{command_id, attempt_id}`，sink 已持有 | 新增 |
| `provenance` | — | **不做**，见约束 3 |

批一结束时事件已能自解释（离线拿到一条就知道它属于哪个 attempt、第几条、什么时候、哪个 provider），但**仍未达成 §4.1 的嵌套形状**——这一点必须在 spec §4.1 显式标注，避免有人提前宣布完成。

同批把门禁补上：

- [ ] `verify:contracts` 用事件 schema 校验三家 golden 的 `expected_events`（现在完全没校验）；
- [ ] golden 强制每家 ≥5 例（现在只要求 ≥1）；
- [ ] schema 里写死「`seq`/`type` 为冗余投影，以外层为准」「单调但不连续」两句。

### 批二：切成嵌套并迁移读路径

`payload` 变为 `{...信封字段, payload: {业务键}}`，同时改读路径并对历史扁平行双读：

- CP：`employee/activity.go` `ActivityEventPresentation`、执行轨迹投影、卷宗时间线；
- Web：执行时间线、运行脉搏、任务弹层里读事件 payload 的位置；
- S2：决定事件是否纳入 ingest 校验，以及违规是打标还是只计数。

读路径改动清单在实施前需按 `rg` 实测补全——本 spec 不预先冻结，避免漏项被当成"不在范围内"。

## 4. 验收

- [x] 批一：新事件带 `ts`/`attempt_ref`/`seq`，旧读路径与 Web 时间线**零改动且行为不变**  
  取证 2026-08-10：需求 `0f8a2ea9…`（真 claude，5 轮工具）任务 `已完成`；库里逐条事件的 7 个信封键齐全（`schema_version,type,ts,seq,provider_type,provider_session_id,attempt_ref`），业务键原样；浏览器复核任务弹层「已完成 / 运行与结论 / 执行尝试 1/3 / 交付物」渲染正常
- [x] 批一：改任一家 parser 的映射后，golden 校验能红  
  反向验证：把 `opencode/01` 的 `expected_events[0].type` 改成 `bogus_type` → `verify:contracts` 红（`data/type must be equal to one of the allowed values`）；把 envelope fixture 的 `ts` 改成非法值 → 红。两次均已还原
- [ ] 批二：历史扁平行与新嵌套行在同一条时间线里都能正确渲染（造混合数据验证）
- [x] `sequence_number` 的去重与排序行为不变（本批未动外层字段；`seq` 只是同值投影，单测锁死二者相等）
- [ ] 批二：CP/Web 读路径切嵌套后，去重与排序仍以外层 `sequence_number` 为准（重发同号不产生第二条）

## 5. 现状锚点

| 项 | 路径 / 符号 |
|---|---|
| 写回构造 | `apps/runtime-agent/src/commands/executor.rs` `runtime_event_writeback`（metadata 里的 `schema_version`） |
| 号的来源 | `apps/runtime-agent/src/runs.rs` `next_sequence`（每 run 从 1 起） |
| 外层契约 | `contracts/control-plane/openapi.yaml` `RuntimeCommandEventWritebackRequest`（`payload`/`metadata` 均 `additionalProperties: true`） |
| CP 去重/排序 | `apps/control-plane/internal/employee/run_writeback.go` `RecordEvent` / `HasRunEventSequence`；终态保留号 `terminalCompletedSequence` 等 |
| 事件 type→中文 | `apps/control-plane/internal/employee/activity.go` `ActivityEventPresentation` |
| 扁平并集 schema | `contracts/provider/schemas/provider-event.schema.json` |
| golden 与门禁 | `contracts/provider/golden/**`、`scripts/verify-foundation-contracts.mjs` provider 段、`apps/runtime-agent/tests/provider_golden_test.rs` |
