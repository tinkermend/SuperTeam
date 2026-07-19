# 系统配置中心 P2：runtime 平台限额下发 + 心跳超时可配（设计立项）

日期：2026-07-19
状态：已实施入 main（00437cc6 / merge daa17d24，§8 六项真实 E2E 全 PASS，见 CHANGELOG 2026-07-19 14:14）
前置：P1 已入 main（df52809c，spec `2026-07-19-system-config-center-design.md`）

## 1. 背景与问题

P1 落地后遗留三件事，全部根源于"runtime 侧限额是独立硬编码，控制平面改了它不知道"：

1. **CP/runtime 双份硬编码**：工件单文件 10MiB 在 `project/artifact_storage.go`（已可配、上限被锁）与 `runtime-agent/src/artifacts.rs`（`MAX_ARTIFACT_FILE_BYTES`，硬编码）各持一份，靠注释对齐。附件限额（单 5MiB/20 个/总 50MiB）与技能解包限额（200MiB/10k 文件）只存在于 runtime 侧，平台完全不可配。
2. **`artifact.max_file_size_bytes` 只准调低**：P1 给它 `MaxValue=10MiB` 护栏，因为调高后 presign 放行而 runtime 静默丢文件。解锁前提=runtime 拿得到平台生效值。
3. **心跳超时不可配**：`runtime.HeartbeatTimeout`（60s）横跨三个包（`runtime/models.go` 无 ctx 的 `IsOnline()`、`runtime/scheduler.go`、`authz/pg_repository.go`），P1 按范围克制移出。

## 2. 目标 / 非目标

**目标**：
- runtime 每台节点持有一份**平台限额快照**，来源=控制平面系统配置中心，变更后分钟级收敛，无需重启 agent。
- 解锁 `artifact.max_file_size_bytes` 调高（新上限 100MiB，见 §5 版本偏斜护栏）。
- 附件三限额、技能解包两限额进注册表可配（domain=artifact）。
- `runtime.heartbeat_timeout_seconds` 进注册表可配（domain=execution，P1 空 tab 就位）。

**非目标**：
- 不做租户级差异化下发（节点属租户，快照按节点租户解析即可，多租户细化留待真实需求）；不做 provider 超时/LRU 等**节点级** env 配置的平台接管（作用域不同，维持节点 env）；不做配置推送通道（复用心跳轮询，不新增 WS 指令）。

## 3. 下发通道：心跳响应携带 limits 快照

**决策：走心跳响应，不走 session 签发/续期响应。** 会话续期周期≈12h，配置收敛太慢；心跳 30s 一次，天然是新鲜度合适的轮询通道。`HeartbeatResponse` 是 serde 反序列化，加字段对旧 agent 向后兼容（未知字段忽略）。

CP 侧：heartbeat handler 组装响应时经 systemconfig Reader（带节点租户）附加：

```json
"platform_limits": {
  "version": "plv1:sha256:<对限额有序序列化的指纹>",
  "artifact_max_file_size_bytes": 10485760,
  "attachment_max_file_size_bytes": 5242880,
  "attachment_max_count": 20,
  "attachment_total_max_bytes": 52428800,
  "skill_archive_max_bytes": 209715200,
  "skill_archive_max_file_count": 10000
}
```

- `version` 指纹供 runtime 判断"没变就不动"，避免每 30s 写日志/换快照的噪音；变更时 info 级留痕一条（old→new）。
- 字段全部可选（`#[serde(default)]` + `Option`）：CP 老版本不发→runtime 用本地硬编码默认值，**任何一侧缺失都不破坏现状**。

runtime 侧：`controlplane/models.rs` 加 `PlatformLimits` 结构；daemon 心跳循环把快照写入 `ArcSwap`（或 `RwLock`）全局；`artifacts.rs`/`skills.rs` 的常量降级为"快照缺失时的默认值"，读取点改走快照。执行中任务用启动时取到的值即可，不要求任务中途换限额（收敛粒度=下一次任务）。

## 4. 注册表扩展（六个新 key）

| key | domain | 默认值 | 边界 |
|---|---|---|---|
| `artifact.attachment_max_file_size_bytes` | artifact | 5MiB | 1–10MiB |
| `artifact.attachment_max_count` | artifact | 20 | 1–100 |
| `artifact.attachment_total_max_bytes` | artifact | 50MiB | 10–200MiB |
| `skill.archive_unpack_max_bytes` | artifact | 200MiB | 50–500MiB |
| `skill.archive_unpack_max_file_count` | artifact | 10000 | 100–50000 |
| `runtime.heartbeat_timeout_seconds` | execution | 60 | 10–600 |

注册表加条目即可（P1 架构保证零迁移零契约变更；`ValueType` 需新增 `int` 纯计数型——P1 只有 bytes/duration_seconds，前端编辑弹窗对 `int` 免单位换算）。

## 5. 解锁工件上限调高：版本偏斜护栏

风险：限额调到 100MiB 时，尚未升级（或断连缓存旧快照）的 runtime 仍按 10MiB 采集——presign 放行、runtime 跳过，回到静默丢文件。

**护栏：能力自报 + 服务端 clamp。** runtime 心跳请求加 `supports_platform_limits: true`（新 agent 才有）；CP 侧 presign 校验时，若租户内存在**在线且不带该标志**的节点，工件上限按 `min(生效值, 10MiB)` clamp 并在 warn 日志留痕。全部在线节点具备能力后 clamp 自动消失。这样调高动作永不产生静默丢文件，只可能"暂时没生效"，且原因可从日志定位。`MaxValue` 放宽到 100MiB 与该护栏同一提交落地。

## 6. 心跳超时可配：显式穿参，不做全局可变状态

三个使用点分别处理（拒绝 process-global atomic 的捷径，保持可测性）：

1. `runtime/models.go` `IsOnline()`：改为 `IsOnlineAt(threshold time.Duration)`（或调用方直接比较），无 ctx 的 model 方法不做 IO——由**调用方**解析阈值后传入。保留 `IsOnline()` 为默认值便捷形态，逐调用点迁移。
2. `runtime/scheduler.go` 与 `runtime/service.go` 内部：service 已有 systemconfig 闭包注入先例（P1 的 `SetSessionTTLResolver`），同法加 `SetHeartbeatTimeoutResolver`。
3. `authz/pg_repository.go`：authz 不 import systemconfig（P1 已证成环），在 authz 仓储构造时由 app 装配层注入 `heartbeatTimeout func(ctx) time.Duration`，缺省回退 `runtimepkg.HeartbeatTimeout` 常量。

生效语义写进 description：影响"节点在线"判定与 runtime scope 授权的活性窗口，调大意味着离线节点更晚被判离线——边界上限 600s 防止把僵尸节点长期当在线。

## 7. 契约与已知债

- heartbeat 目前**不在** `contracts/runtime/openapi.yaml`（事实协议活在 Go handler 与 Rust models 里，属既有已知债）。本 spec 不顺手补全整个 runtime 契约，但新增的 `platform_limits`/`supports_platform_limits` 字段**必须**在该 yaml 有对应描述（哪怕先以 schema 片段形式），并人工核对 Go/Rust 两侧一致——该契约无自动化验证，实施 PR 里附一份两侧字段对照清单。

## 8. 验证（真实 E2E 为完成条件）

1. 改 `artifact.attachment_max_file_size_bytes` 调低 → 真实任务产出超限附件 → runtime 按新值跳过并留 `execution_output_skipped` 痕（快照收敛≤1 个心跳周期+缓存 15s）。
2. 调高 `artifact.max_file_size_bytes` 至 20MiB：新 agent 在线 → 真传 15MiB 工件全链成功；混布场景（起一个不带能力标志的旧协议 agent 或伪造心跳）→ presign 被 clamp 回 10MiB + warn 日志。
3. 改 `runtime.heartbeat_timeout_seconds`=10 → 停 agent 10s 后节点判离线（Runtime 页 + overview KPI）；恢复默认 60s 行为回归。
4. runtime 断连期间改配置 → 重连后下一次心跳收敛，日志留 old→new 一条。
5. 旧 agent（不识别 platform_limits）+ 新 CP：行为与 P1 完全一致（回退硬编码默认值），零报错。
6. `verify:control-plane` / `verify:runtime-agent` / `verify:web` / `verify:contracts` 全绿；Go/Rust 字段对照清单人工核对留档。

## 9. 自审记录（找茬结论）

- **最大风险=版本偏斜静默丢文件**，已用能力自报+服务端 clamp 封死（§5）；调高动作的最坏结果从"丢数据"降级为"暂未生效+日志可查"。
- **心跳通道的代价**：limits 挂在心跳上意味着"节点不心跳就不收敛"——这正确：不在线的节点本来就不执行任务，不需要新限额。
- **拒绝全局可变状态**：心跳超时三个使用点全部显式穿参/闭包注入，多花的代码量换来单测不需要进程级 reset。
- **int 型新增**：注册表要加纯计数 ValueType，前端弹窗跳过单位换算——实施时别忘了 P1 的 `unitFor` 对未知类型已兜底返回无单位（factor=1），前端实际零改动也能工作，但要补一条单测钉住。
- **不做的事**：不把 provider timeout/LRU 收进平台配置（节点级作用域）；不做配置推送 WS 指令（30s 轮询足够，复杂度不换收益）。
