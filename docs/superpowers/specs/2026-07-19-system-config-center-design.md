# 系统配置中心设计（System Config Center）

日期：2026-07-19
状态：P1 已实施并通过真实 E2E（分支 worktree-system-config-center，合并待并发会话让路，见 §12 实施记录）

## 1. 背景与结论

平台目前有 40+ 处平台级运维参数硬编码在代码常量里（工件大小上限、presign TTL、心跳超时、会话有效期、上传上限等），既无 DB 承载也无运行时修改通道；同时不存在任何"系统配置表/模块"（`internal/config` 与 runtime `config.rs` 均为进程启动加载器，改值需改文件+重启）。

菜单结构结论（已与人类对齐）：**一个"系统配置"菜单**，放入侧边栏"治理平台"分组；内部按**领域分 tab**（文件与工件 / 执行与调度 / 安全与会话），不按"系统 vs 功能项"拆分。真正的拆分轴是**作用域**：

- **全局平台参数** → 本配置中心。
- **资源级参数**（每节点并发槽位、每员工预算、每项目配置）→ 跟随资源详情页，不进配置中心。runtime 节点并发（`RUNTIME_AGENT_MAX_CONCURRENT_TASKS`，默认 3）是节点级 env 配置，本 spec 不动它；将来若做"平台侧下发节点默认值"，另行立项。

## 2. 三层配置边界（本 spec 的硬边界）

| 层 | 载体 | 举例 | 是否进配置中心 |
|---|---|---|---|
| 部署态基础设施配置 | env / config.yaml，改后重启 | DATABASE_URL、S3、Temporal、加密密钥、CORS | **否**，永不。密钥类尤其禁止入 DB |
| 平台运行态参数 | **本配置中心**（DB 覆盖 + 代码默认值），热生效 | 工件大小上限、presign TTL、心跳超时、会话 TTL | 是 |
| 资源级业务配置 | 各业务表（budget_policy、节点 env、项目配置修订） | 节点槽位、员工预算、任务墙钟上限 | 否，跟随资源页 |

判定标准：一个参数进配置中心，当且仅当（a）平台级全局语义（b）管理员运维时有真实调整动机（c）改后无需重启即可安全生效。

## 3. 数据模型（Registry-first：定义在代码，DB 只存覆盖）

配置项的**定义**（key、类型、默认值、边界、领域、文案）是服务端 Go 注册表，随代码演进；DB 只存**显式覆盖值**。"恢复默认"= 删除覆盖行。好处：加配置项零迁移、默认值有代码评审、DB 无 schema 漂移。

迁移 `084_create_system_config_overrides.sql`（遵循 DATABASE_DESIGN.md：UUID-first、tenant-first、TIMESTAMPTZ、中文 COMMENT、VARCHAR 不用 DB 枚举）：

```sql
CREATE TABLE system_config_overrides (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    config_key  VARCHAR(128) NOT NULL,
    value       JSONB NOT NULL,
    updated_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_system_config_overrides_tenant_key
    ON system_config_overrides (tenant_id, config_key);
COMMENT ON TABLE system_config_overrides IS '平台级系统配置覆盖值；配置定义与默认值在服务端注册表，本表只存管理员显式修改的覆盖';
-- 各字段 COMMENT 略（实施时补全）
```

不加跨模块 FK（遵循应用层完整性立场）；`updated_by` 存用户 UUID 仅作展示与审计冗余。

## 4. 服务端注册表

新模块 `apps/control-plane/internal/systemconfig/`，按 capability 模块分层：`types.go` / `service.go` / `pg_repository.go` / `handler.go`。

```go
type Definition struct {
    Key          string        // 点分命名：artifact.max_file_size_bytes
    Domain       string        // 领域标签，前端按此分 tab：artifact | execution | security
    Label        string        // 中文名
    Description  string        // 中文说明（含生效语义与影响面）
    ValueType    string        // int | duration_seconds | bytes | bool | string
    DefaultValue any
    MinValue     *int64        // 数值型防呆下界（防止管理员把心跳超时改成 0 弄瘫平台）
    MaxValue     *int64        // 上界（含"P1 不得高于 runtime 硬编码值"类护栏）
}
```

- 注册表是包内静态切片 + `init` 校验（key 唯一、默认值在界内），**不是封闭枚举**：新增配置项 = 注册表加一条 + 使用点接入，无迁移无契约变更（列表接口按注册表动态返回）。
- Service 是唯一写闸门：`Set(ctx, tenantID, key, value, actor)` 校验 key 存在、类型匹配、边界合规后 upsert 覆盖行并写审计；`Reset` 删除覆盖行并写审计。未知 key 一律 404，越界 422。
- 读取接口：`Reader` 窄接口注入各业务模块：

```go
type Reader interface {
    Int64(ctx context.Context, tenantID uuid.UUID, key string) int64
    Duration(ctx context.Context, tenantID uuid.UUID, key string) time.Duration
}
```

  实现带进程内缓存（TTL 15s，本进程写后即时失效）；DB 不可达时回退注册表默认值并记 warn，**读路径永不因配置中心故障而失败**。现有使用点的包级常量改为经 `Reader` 取值，常量保留为注册表 `DefaultValue` 的单一定义处。

## 5. API 契约（contracts/control-plane/openapi.yaml）

```
GET    /api/v1/system-configs            listSystemConfigs
PUT    /api/v1/system-configs/{key}      updateSystemConfig      body: { value }
DELETE /api/v1/system-configs/{key}      resetSystemConfig
```

`listSystemConfigs` 返回注册表全量投影（管理页一次拉全，无分页）：

```json
{ "items": [{
    "key": "artifact.max_file_size_bytes",
    "domain": "artifact",
    "label": "单个工件大小上限",
    "description": "...",
    "value_type": "bytes",
    "default_value": 10485760,
    "effective_value": 10485760,
    "is_overridden": false,
    "min_value": 1048576, "max_value": 10485760,
    "updated_at": null, "updated_by_name": null
}]}
```

生成走 `generate:control-plane`，路由按 capability 模式在 `server.go` 手动分组挂接（handler nil 判断 + `r.Group`）。

## 6. 鉴权与审计

- `internal/authz/types.go` 新增 `ActionSystemConfigRead` / `ActionSystemConfigManage`；`authorizer.go` switch 新增 case：资源为本租户 + `checkTenantAdminAccess`（admin/owner）。handler 复用 capability 的 `authorize` 辅助并带 `AuditReason`。
- 每次 `Set`/`Reset` 写 `audit.Event`：`EventType: "system_config"`、`ResourceType: "system_config"`、`ResourceID: key`、`Action: "update"|"reset"`、`Details: {old_value, new_value, default_value}`。沿用"审计失败不阻断主流程"惯例。

## 7. P1 首批收编清单

选取标准：control-plane 单侧读取、热生效无一致性副作用、有真实运维动机。共 7 项（实施修订：duration 键名统一 `*_seconds`；`runtime.heartbeat_timeout_seconds` 移出 P1，见下）：

| key | domain | 默认值 | 原址 |
|---|---|---|---|
| `artifact.max_file_size_bytes` | artifact | 10MiB | project/artifact_storage.go |
| `artifact.presign_upload_ttl_seconds` | artifact | 15min | project/artifact_storage.go |
| `artifact.content_get_ttl_seconds` | artifact | 5min | project/artifact_storage.go |
| `skill.upload_max_bytes` | artifact | 50MiB（上限 200MiB=runtime 解包限额） | skill/handler.go |
| `skill.archive_presign_ttl_seconds` | artifact | 15min | skill/service.go |
| `runtime.session_ttl_seconds` | security | 12h | runtime/service.go |
| `auth.session_ttl_seconds` | security | 12h | auth/service.go |

**心跳超时移出 P1（实施时发现）**：`HeartbeatTimeout` 使用点横跨 3 个包——`runtime/models.go` 的无 ctx 方法 `IsOnline()`、`runtime/scheduler.go`、以及 `authz/pg_repository.go`（授权层直接引用）。做成租户级可配需要全局可变状态或大范围重构，与 P1 范围克制冲突，移入 P2 随 runtime 配置下发一并考虑。当前 execution domain 因此暂无配置项，页面只显示两个 tab。

**`artifact.max_file_size_bytes` 的 P1 护栏**：runtime 侧 `MAX_ARTIFACT_FILE_BYTES` 是独立硬编码 10MiB（artifacts.rs:29），P1 没有配置下发通道，因此该项 `MaxValue = 10MiB`——**只允许调低不允许调高**，避免管理员调到 20MiB 后 presign 放行而 runtime 静默丢文件。调高能力属 P2。页面 description 明示此限制。

**明确不进 P1 的**（防止范围膨胀）：
- 分页默认/上限（各模块 20/50/100 各写一份）：收益低、改造面横跨 10+ 模块，列为 P3 候选，届时先统一为共享 helper 再谈可配。
- Temporal 超时/重试、退避间隔、轮询间隔：调错即伤调度正确性，无运维动机，保持代码常量。
- 飞书/planner 的 env 配置：属部署态层。
- 审批超时/自动过期：现无此机制，属新功能不属配置收编。

## 8. P2：runtime 侧配置下发

目标：消除 CP/runtime 双份硬编码（工件 10MiB、附件 5MiB/20 个/50MiB 总量、技能解包 200MiB 等），并解锁 `artifact.max_file_size_bytes` 调高。

方案方向（P2 开工时细化）：在 runtime session 建立/续期响应中附带 `platform_limits` 快照，runtime 以其覆盖本地默认值；契约改动涉及 `contracts/runtime/openapi.yaml`（注意：该契约无自动化验证，需人工核对生成物，见 CLAUDE.md 已知债）。P2 完成后放开 P1 的 MaxValue 护栏。

## 9. Web 设计

- 侧边栏：`sidebar-data.ts` "治理平台"分组新增 `{ title: "系统配置", url: "/system-config", icon: Settings2 }`（与个人 `/settings/account` 账户设置无关，互不合并）。
- 路由 `routes/_authenticated/system-config/index.tsx` → `features/system-config/index.tsx`。
- 页面结构：`@/components/ui/tabs`，**tab 由接口返回的 `domain` 字段动态分组**（domain → tab 标题映射表 + 未知 domain 落"其他"tab），后端加配置项前端零改动。
- 每项渲染为一行：Label + description + 当前值编辑控件（按 `value_type`：bytes 用 MiB 输入、duration 用秒/分钟、bool 用 Switch）+ 覆盖态标识（"已修改"badge + "恢复默认"按钮，默认态显示"默认值"）。
- 交互：行内编辑 → 保存调 `PUT`，越界错误就地展示 `min/max`；恢复默认走确认弹窗调 `DELETE`。React Query `queryKey: ["system-configs"]`，写后 invalidate。
- API client：`lib/api/system-config.ts`，沿用 `getJson/putJson/deleteJson`。
- 布局遵循 DESIGN.md 与布局宪法（Main 默认 contained），实施前按约定重读 DESIGN.md。

## 10. 验证（真实 E2E 为完成条件）

1. 管理员登录 → 系统配置页可见三 tab、8 项配置、默认值正确；非 admin 角色访问接口 403、菜单不可见（前端按权限隐藏可后置，接口 403 必须）。
2. 改 `skill.upload_max_bytes` 调低到 1MiB → 真实上传 2MiB 技能包被 413/422 拒绝 → 恢复默认 → 上传成功。走真实 Web + CP + DB 链路。
3. 改 `artifact.presign_upload_ttl_seconds` → 发起真实任务产出工件，presign URL 过期参数为新值（查 URL X-Amz-Expires）。
4. 越界写（心跳超时设 0）→ 422 且 DB 无覆盖行。
5. 每次 update/reset 在审计中心可见事件（old/new value）。
6. 缓存生效性：改值后 ≤15s 新值生效（可在测试中直接二次请求确认，不苛求即时）。
7. `verify:control-plane`、`verify:web`、`verify:contracts`、`make -C apps/control-plane migrate-validate` 全过。

## 11. 自审记录（找茬结论）

- **双份硬编码陷阱**：CP 与 runtime 各持 10MiB，若 P1 直接放开调高会造成"presign 放行、runtime 静默丢文件"的隐性数据丢失 → 已用 MaxValue 护栏封死，P2 才解锁。这是本设计最重要的防呆。
- **管理员自伤防护**：所有数值配置带服务端 min/max（如心跳超时下界 10s、会话 TTL 下界 1h），防止一次误操作弄瘫平台。
- **配置中心自身故障不放大**：读路径回退代码默认值，绝不让"配置系统挂了"演变成"业务读不到配置全挂"。
- **范围克制**：分页统一、Temporal 参数、审批超时明确拒收，避免 P1 变成横切 10+ 模块的重构。
- **密钥红线**：加密密钥、凭据类 env 永不入 DB 配置，边界写死在 §2。
- **单租户现实 vs tenant-first 规范**：表带 tenant_id 遵循规范，Reader 接口带 tenantID 参数，避免将来多租户时全量返工；当前实现按调用方现有 tenant 上下文取值。
- **auth.session_ttl_seconds 生效语义**：只影响新签发会话，存量会话按签发时 TTL 到期，description 中写明，不做存量回收（避免误改导致全员掉线）。

## 12. 实施记录（2026-07-19，P1 全量落地）

与 spec 原文的偏差（均为实施时发现的事实修正）：

- **迁移编号 085**（084 已被并发工作占用）：`085_create_system_config_overrides.sql`。
- **P1 收编 7 项而非 8 项**：心跳超时移出（见 §7 说明）。
- **import 环约束**：`api/middleware` 反向依赖 `auth` 与 `runtime` 两包，故这两个包不能 import `systemconfig`（systemconfig/handler → middleware → auth/runtime 成环）。auth/runtime 的会话 TTL 改由 app 装配层以闭包注入（`SetSessionTTLResolver`），包内保留兜底默认常量；project/skill 无环，直接注入 `systemconfig.Reader`。
- **登录会话租户口径**：登录先于租户上下文，`auth.session_ttl_seconds` 固定读 `platform.DefaultTenantID` 的配置（闭包内写死）。
- **默认值单一定义**：新增 `systemconfig.DefaultFor/DefaultDurationFor`，消费方 Reader 未注入（测试场景）时统一回退注册表默认值。

落地清单：迁移 085 + sqlc `system_config.sql`；`internal/systemconfig/`（registry/types/service/pg_repository/handler + 单测 6 组）；authz `ActionSystemConfigRead/Manage` 并入 mcp_registry/scenario_template 同一 case（OpenFGA 映射按既有先例不覆盖，走 DB authorizer）；OpenAPI `/api/v1/system-configs` 三端点 + 三 schema + regen；server.go/app.go 装配；7 个使用点接入；Web `features/system-config/`（domain 动态 tab + 单位感知编辑弹窗 + 恢复默认确认 + 单测）+ 路由 + 侧栏"治理平台/系统配置"。

门禁：`migrate-validate`（podman 一次性库 exit=0）、`verify:control-plane`、`verify:web`、`verify:contracts` 全绿（顺手修 `capabilities.test.ts` 两个 base 上既有的失效 type import——typecheck 在 main 上本已红）。

真实 E2E（因并发会话占着主 checkout 的 server.go/app.go 未提交改动，无法安全合并后在 main 验证，改为 worktree 独立全链：worktree 构建 CP :8082 + Web :3001 + podman 隔离 PG 全量迁移 + 真实 S3/Redis，07-11 有先例）：
1. 管理员登录 → 页面两 tab、7 项、格式化值/边界/默认提示正确（浏览器 Playwright 全程）。
2. 越界（UI 500MiB / API 20MiB）→ 前端就地报错 + 服务端 422 且 DB 无覆盖行；未知 key 404；非数字 400。
3. **配置真实生效**：`skill.upload_max_bytes` 调至 1MiB → 真实 multipart 上传 2MiB 技能包 400（错误信息带动态限额）→ 恢复默认 → 同包 201 真实入 S3。
4. **presign TTL 真实生效**：`artifact.content_get_ttl_seconds` 300→120→300，`/artifacts/{id}/content` 302 Location 的 `X-Amz-Expires` 逐次跟随（直插 artifact ref 行造数）。
5. **登录会话 TTL 真实生效**：改 2h → 新登录 `expires_at-created_at`≈2h，恢复 → ≈12h（±0.2h 为 podman 容器时钟漂移）。
6. 审计：6 条 `system_config` 事件（update/reset 各带 old/new/default）。
7. 权限：新建普通成员用户 GET/PUT 均 403。
8. UI 闭环：编辑保存 → 行内即时刷新为 1 MiB + "已修改" pill + 修改人/时间 + 恢复默认按钮；确认恢复 → 回默认态。

遗留/交接：分支 `worktree-system-config-center` 待合并 main（合并后需按分支收尾在 main 上做一轮 E2E 复验：至少页面可达 + 改值生效一例）；P2（runtime 配置下发 + 心跳超时 + 解锁工件上限调高）未立项。
