# runtime/provider 契约自动验证立项

> 复核状态：未实施（立项状态）

> 日期:2026-07-19
> 状态:立项(用户批准),未实施
> 动因:CLAUDE.md"已知债"挂账已久;2026-07-18/19 能力绑定统一工作再次实证——派发 payload 新增
> `metadata.capability_manifest_version`、runtime 反序列化扩展 `RuntimeSessionCommandPayload`,
> 全程只能人工核对 Go 构造侧与 Rust 消费侧一致性,无任何自动化护栏。

---

## 现状债务(两笔,口径不同)

1. **`contracts/runtime/openapi.yaml` 未入验证流程**。`scripts/verify-foundation-contracts.mjs`
   只覆盖 `contracts/control-plane/openapi.yaml`(路径三向核对:openapi ⊇ Go 路由注册、
   openapi ⊇ Rust CP client、openapi ⊇ TS api-client)。runtime openapi 改了没人管,
   与 `apps/runtime-agent/src/server.rs` 的真实路由可以任意漂移。
2. **`contracts/provider/` 只有散文 README,无机器可读 schema**。事实上的 Provider 协议
   (派发 payload 的 skills/mcp_servers/metadata 形态、事件流、结果、工件、错误)活在
   `apps/runtime-agent/src/commands/payload.rs` 与 `src/providers/` 的 Rust 类型里,
   与"协议必须语言无关"的架构约定相悖。CP 侧 `buildStartSessionPayload`(Go, map[string]any
   手工拼装)和 Rust 反序列化类型之间没有共同事实源。

## 目标

- 改 runtime 契约或派发 payload 形态时,CI/门禁能自动发现 Go 构造侧、Rust 消费侧、契约文件
  三者不一致,替代人工核对。
- Provider 协议获得机器可读 schema 作为单一事实源,Rust 类型退化为其中一种实现。

## 建议方案(分两期,可独立实施)

### P1 runtime openapi 纳入 verify-foundation-contracts(小,先做)

- 扩展 `scripts/verify-foundation-contracts.mjs`:解析 `contracts/runtime/openapi.yaml` 路径集,
  与 `apps/runtime-agent/src/server.rs`(axum Router 注册)及 CP 侧调用 runtime 的 client
  (若有)做包含关系断言,套用现有 control-plane 三向核对的实现模式。
- 产出:改 runtime 契约不生成/不核对会在 `verify:contracts`(foundation 门禁一环)直接红。

### P2 派发 payload / Provider 协议 schema 化(中,是本债的本体)

- 在 `contracts/provider/` 落 JSON Schema(或复用 openapi components):至少覆盖
  start_session 命令 payload(含 skills[]、mcp_servers[]、metadata 已知键——
  `capability_manifest_version`、`chat_thread_id`、project workspace 五元组等)、
  事件回写(RecordEvent/terminal writeback)、结果与工件形态、错误分类。
- 验证接线(择一,按成本):
  a. **golden 样例双向校验**(推荐起步):仓库放一组 payload 样例 JSON;Go 侧测试断言
     `buildStartSessionPayload` 产物 validate 过 schema;Rust 侧测试断言样例能反序列化进
     `RuntimeSessionCommandPayload` 且关键字段无损。schema 改动没跟上任一侧即红。
  b. 代码生成:schema → Rust serde 类型(typify 之类)替换手写 payload.rs 类型。改动大,
     P2 后半程再评估。
- 明确非目标:不引入替代主栈的 IDL 框架(protobuf 等);沿用 JSON Schema/openapi 生态。

## 验收口径

- P1:改 `contracts/runtime/openapi.yaml` 增删路径而不同步实现 → `verify:contracts` 失败;
  正常同步 → 绿。
- P2:任改 schema/Go 构造/Rust 类型三者之一而不同步其余 → 对应侧单测红;
  CLAUDE.md"已知债"两条随 P1/P2 分别销账。

## 参考事实(实施时省勘察)

- 现验证脚本断言逻辑:`scripts/verify-foundation-contracts.mjs` `assertSetContainsAll`
  (client 路径 ⊆ openapi 双向各一次)。
- 派发 payload 构造:`apps/control-plane/internal/employee/run_service.go` `buildStartSessionPayload`;
  Rust 消费:`apps/runtime-agent/src/commands/payload.rs`(`RuntimeSessionCommandPayload`、
  `project_workspace()` metadata 提取)。
- 能力指纹字段:`metadata.capability_manifest_version`(cmv1:sha256:…,CP 只在 skillLister
  就绪时写入;runtime 以其存在与否作为 prune 保险丝)——schema 化时注意"可缺省"语义即契约。
