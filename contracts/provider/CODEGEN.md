# Provider schema → 类型生成评估（Phase 4）

## 结论（2026-08-10）

**本批不接代码生成。** 继续以 `contracts/provider/schemas/*.json` 为语义事实源，Rust/Go 手写类型为实现；用 golden + ajv fixture 门禁防漂移。

## 评估

| 方案 | 优点 | 缺点 | 判定 |
|---|---|---|---|
| **typify / schemars (Rust)** | 从 schema 生成 serde 类型 | 与现 `ProviderEvent` 标签枚举、`anyhow` 流类型难无损对齐；生成物进仓会与手写测试交织 | 不采纳 |
| **oapi-codegen 扩展** | 已有 CP openapi 生成流水线 | Provider 语义契约刻意不进 control-plane openapi（opaque payload）；硬塞会混淆 wire/语义两事实源 | 不采纳 |
| **手写类型 + schema 校验（现状+本批）** | 零新主栈；ajv 校验 fixture；golden deep-equal | schema 变更需人工同步类型 | **采用** |

## 何时再评估

- 第三家以上 Provider 接入导致 payload 字段爆炸；或  
- 多语言 adapter（非 Rust）要消费同一 schema 且无法靠 fixture 约束时。

届时优先 **schema → 校验类型（只生成校验用 DTOs）**，不替换 `ProviderEvent` 热路径枚举。
