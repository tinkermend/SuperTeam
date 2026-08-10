# L2 事件 envelope 定档（做实还是正式降级）

- 日期：2026-08-10
- 状态：**立项（人类已批），未实施——先拍板再动代码**
- 来源：`2026-08-09-provider-semantic-unification-design.md` 复查（§18-2）
- 规模：小；但**必须先决定，否则 golden 门禁不知道该校验什么**

---

## 1. 事实：envelope 没有实现，schema 描述的是过渡态

spec §4.1 定义的 L2 信封是：

```json
{ "schema_version", "type", "ts", "seq", "provider_type",
  "provider_session_id", "attempt_ref", "payload", "provenance" }
```

实际实现走的是 §4.5「过渡策略」：`event_type` + **扁平 payload**，`schema_version` / `provider_type` 挂在 writeback 的 `metadata` 里；`ts` / `attempt_ref` / `provenance` 不存在，`seq` 借用 writeback 的 `sequence_number`。

而 `contracts/provider/schemas/provider-event.schema.json` 被写成了**扁平并集**（`required` 只有 `type`，把 `session_id` / `text` / `tool_id` / `usage` / `error` 等所有 payload 键平铺在顶层）——它描述的是过渡态实现，不是 §4.1 的目标态，且 §4.1 没有被同步标注为"未实现"。这是**契约向实现让步且没留痕**。

连带后果：`contracts/provider/golden/**` 的 `expected_events` 是 Rust `ProviderEvent` 枚举的序列化形状，与 §4.1 信封对不上，因此 `verify:contracts` **只校验 `native_lines` 非空**，没有拿 schema 校验过 `expected_events`——「golden 由 schema 守护」目前只兑现了一半。

## 2. 需要拍板的选项

| 选项 | 内容 | 成本 | 代价 |
|---|---|---|---|
| **A. 正式降级（推荐）** | 承认 v1 = 扁平事件：§4.1 改为「v2 目标态」，schema 更名 `provider-flat-event.schema.json` 并把 `type` 判别子写成 `oneOf`（每种 type 约束自己的必填键），golden `expected_events` 纳入该 schema 校验 | 低 | 事件缺 `ts`/`attempt_ref`，跨 attempt 排序仍依赖 writeback 的 `sequence_number` |
| B. 做实 envelope | 按 §4.1 补全字段，CP 读路径与 Web 时间线同步改 | 中 | 动 CP 读路径与既有事件消费者；收益暂无人索取 |

推荐 A：现在没有任何消费者需要 `ts`/`provenance`，而"schema 与实现对不上"是当下真实的腐烂风险。选 A 时必须在 §4.1 标注清楚，避免下一个会话按信封形状写代码。

## 3. 拍板后要做的（无论 A/B）

- [ ] `verify:contracts` 用事件 schema 校验三家 golden 的 `expected_events`（现在完全没校验）
- [ ] golden 门禁强制每家 ≥5 例（现在只要求 ≥1，实际有 5）
- [ ] provider spec §4.1 / §4.5 同步标注最终形态，`contracts/provider/README.md` 跟进

## 4. 现状锚点

| 项 | 路径 |
|---|---|
| 实际写回形状 | `apps/runtime-agent/src/commands/executor.rs` `runtime_event_writeback`（metadata 里的 `schema_version` = `provider.event.v1`） |
| 扁平并集 schema | `contracts/provider/schemas/provider-event.schema.json` |
| golden 与门禁 | `contracts/provider/golden/**`、`scripts/verify-foundation-contracts.mjs` 的 provider 段 |
| Rust golden 断言 | `apps/runtime-agent/tests/provider_golden_test.rs` |
