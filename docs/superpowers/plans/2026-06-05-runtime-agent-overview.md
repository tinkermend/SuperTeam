# Runtime Agent 总览页实现计划

> 面向子代理执行：本计划按可独立验证的任务拆分执行，优先使用 `superpowers:subagent-driven-development`。每个任务完成后都需要提交小范围 commit，并在进入下一任务前做代码 review。

## 目标

实现 Runtime Agent 总览页的前后端闭环，让 Web 控制台可以查看 Runtime 节点在线状态、接入审批队列、Provider 能力覆盖和 Runtime 事件审计。

本任务只交付总览页，不进入单个 Runtime Agent 详情页，不实现接入密钥创建，不实现诊断包下载或接入，也不新增撤销操作入口。

## 架构原则

- Runtime 管理事实由 Control Plane 持久化和聚合，Web 只消费 Console API。
- Runtime Agent 不承载控制台 UI、业务策略或长期业务状态。
- Provider 能力只作为 Runtime 上报事实展示，不在本任务里进入 Provider 详情或策略编辑。
- 接入审批是人类决策对象，批准和拒绝必须通过后端 API 写入审计相关事实。
- 总览页需要围绕扫描、筛选和操作反馈设计，避免扩展成诊断或详情工作台。

## 任务拆分

### 任务 1：Runtime 事件存储

范围：
- 新增 `runtime_events` 迁移。
- 新增 sqlc 查询，用于事件创建、事件列表、阻断事件计数、Provider 会话计数和能力聚合。
- 新增租户维度 Runtime 节点总数与在线数查询。
- 覆盖 query 层测试或在没有真实数据库环境时至少保证 Go 包编译。

提交：
- `feat(control-plane): add runtime overview event storage`

### 任务 2：Runtime 总览领域服务

范围：
- 新增 Runtime event、overview、capability summary 等领域模型。
- 新增 repository 接口和 PostgreSQL 映射。
- 新增 `CreateRuntimeEvent`、`GetOverview`、`ListRuntimeEvents`、`ListRuntimeCapabilitiesForNode` 等服务方法。
- 对事件 payload 做基础脱敏。
- 覆盖 Runtime service 单元测试。

提交：
- `feat(control-plane): add runtime overview service`

### 任务 3：接入和执行事件投影

范围：
- 在 Runtime 接入申请、批准、拒绝、已有撤销路径和 command writeback 中记录 Runtime 管理事件。
- employee 模块只依赖窄接口，避免反向耦合 Runtime 内部实现。
- 事件写入采用 best-effort，不阻断原主流程。
- 覆盖接入决策事件和 command writeback 事件测试。

提交：
- `feat(control-plane): record runtime overview events`

### 任务 4：Console API 契约和路由

范围：
- 在 OpenAPI 中新增 Runtime overview、event list、node capabilities 读取契约。
- 生成 Control Plane API 代码。
- 在 Console Web session auth 下挂载总览、事件和能力读取路由。
- 对事件过滤参数做服务端校验，非法类型返回 400。
- 不新增详情页 API，不新增诊断包 API。

提交：
- `feat(control-plane): expose runtime overview APIs`

### 任务 5：Web API 客户端

范围：
- 在 Web runtime API client 中新增总览、事件列表、接入拒绝等调用。
- 保留已有节点和接入列表能力。
- 过滤参数为空时不发送，分页参数为 0 时仍保留。
- 覆盖客户端路径、方法、query 和 body 测试。

提交：
- `feat(web): add runtime overview api client`

### 任务 6：Runtime 总览页 UI

范围：
- 将 `/runtime` 页面实现为四个 tab：节点总览、接入审批、能力范围、事件审计。
- 节点总览展示在线指标、待接入摘要、已登记节点、Provider 能力和最近事件。
- 接入审批 tab 使用完整接入列表；pending 记录可批准/拒绝，已决记录只读展示状态和原因。
- 能力范围展示 Provider 类型、节点覆盖、可用数、健康数和最近上报时间。
- 事件审计支持事件类型、严重级别、节点和 Provider 过滤。
- mutation 失败时展示后端返回的错误原因。
- 明确不渲染 Runtime 详情、接入密钥创建、诊断包下载和撤销操作入口。

提交：
- `feat(web): build runtime overview page`

### 任务 7：变更日志和最终验证

范围：
- 按 `YYYY-MM-DD HH:mm` 写入 `CHANGELOG.md`，时间使用 `Asia/Shanghai`。
- 运行前后端和契约验证。
- 如当前 shell 没有 `DATABASE_URL` 或 `TEST_DATABASE_URL`，明确跳过 live schema 验证并报告原因。
- 保持工作区干净，不把无关 `go.work.sum` checksum 噪音纳入提交。

提交：
- `docs: record runtime overview changes`

## 验证命令

```bash
pnpm verify:contracts
go test ./apps/control-plane/internal/runtime ./apps/control-plane/internal/api ./apps/control-plane/internal/employee ./apps/control-plane/internal/app
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
pnpm verify:web
pnpm verify:control-plane
git diff --check
git status --short
```

有真实数据库配置时再执行 live schema 验证；没有配置时不伪造结果。

## 最终验收清单

- Runtime 总览 API 返回 summary、pending enrollment 摘要、nodes、provider capability summary 和 recent events。
- Runtime 事件列表支持 event type、severity、node id、provider type 过滤。
- Web `/runtime` 首屏可查看总览、审批、能力和事件审计。
- 接入审批 tab 不受 overview 摘要数量限制。
- 已批准、已拒绝和已停用接入记录在审批 tab 中只读展示。
- 批准和拒绝失败时显示后端错误原因。
- 页面不包含详情页、接入密钥创建、诊断包下载或撤销操作入口。
- 所有提交保持小范围，最终工作区干净。
