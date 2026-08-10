# 新 Provider 接入清单

本清单与 `docs/superpowers/specs/2026-08-09-provider-semantic-unification-design.md` Phase 3 验收对齐。

## 必做

1. **注册类型字符串**  
   - Runtime：`apps/runtime-agent/src/providers/catalog.rs` 增加 `ProviderDescriptor`  
   - 配置：`RuntimeConfig` provider section + `config.example.yaml`  
   - Control Plane：服务端校验允许该 `provider_type`（无封闭业务枚举，走注册/校验）

2. **`capability()` 诚实声明**  
   - `session_resume` / `stream_text` / `stream_tools` / `stream_usage` / `structured_error` / `mcp_native`  
   - `mcp_isolation`: `"argv"` | `"home_file"`  
   - **禁止**在 capability 为 false 时让 UI 假装有工具轨迹

3. **Parser + golden**  
   - 实现 `parse_*_event`：原生 JSON 行 → `ProviderEvent`  
   - 未知 type 走 `ProviderEvent::native_unmapped`（不要静默吞掉可观测性）  
   - `contracts/provider/golden/<provider>/` ≥5 条真实脱敏样例，**含 ≥1 条失败**  
   - 生成方式：语义推导，禁止「跑现 parser 存盘」把缺陷锁成契约

4. **错误**  
   - 进程失败 / 协议错误经 `error_map` 产 `ErrorEnvelope`  
   - 不得在 adapter 内用业务方言 `contains` 做 family 决策主路径

5. **门禁**  
   - `cargo test --test provider_golden_test`  
   - `node scripts/verify-foundation-contracts.mjs`  
   - 至少一次真实 smoke：成功 + 一类失败（spawn 或非 0 退出）

## 禁止

- Control Plane / 协调逻辑 `switch provider_type` 解析原生 payload  
- 把业务验收写进 parser  
- 默认把未知行 fail attempt（噪声会拖垮任务）

## 可选（后续）

- L0 离线重放工具  
- 强审计任务 predispatch 拒绝 `stream_tools=false`（需产品拍板；structured_error 现值不支持提前接闸）  
- `error_code` 列（Phase 4）用于跨 Provider 统计告警
