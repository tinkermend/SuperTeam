# SuperTeam Foundation Readiness Design

日期：2026-05-30
状态：已确认设计方向
范围：基础骨架与底座能力收口，不进入具体业务功能开发

## 1. 目标

本设计用于在开始具体前后端功能开发前，收口 SuperTeam 当前已经存在的工程骨架。目标不是重新脚手架，也不是继续扩展未来功能，而是让现有 Web、Control Plane、Runtime Agent、contracts 和共享 packages 形成一套可维护、可扩展、可复用的开发基线。

完成后，开发者应能在这套底座之上开始实现任务中心、审批中心、数字员工管理、能力网关、审计等具体功能，而不需要先猜测启动方式、契约事实源、测试入口、数据边界或 Runtime 调用路径。

## 2. 当前状态

当前仓库已经具备较多基础资产：

- `apps/web` 已有 Next.js App Router 入口、控制台壳、shadcn/ui 基础组件和页面测试。
- `apps/control-plane` 已有 Go 服务入口、API server、任务、Runtime、认证、审计、存储和配置包。
- `apps/runtime-agent` 已有 Rust daemon、Control Plane client、Provider adapter、本地诊断 HTTP API 和测试。
- `packages/ui`、`packages/views`、`packages/core`、`packages/api-client` 已经形成前端共享边界。
- `contracts/control-plane`、`contracts/runtime`、`contracts/provider` 已经有契约文件。

最近一次基线验证显示：

- `pnpm -r --if-present test` 通过。
- `pnpm -r --if-present typecheck` 通过。
- `cargo test --manifest-path apps/runtime-agent/Cargo.toml` 通过。
- `go test ./apps/control-plane/...` 只在 `internal/storage/queries` 的 testcontainers 测试处失败，错误原因是本机缺少 rootless Docker provider；其它 Go 包通过。

因此，当前问题不是“没有骨架”，而是需要把已有骨架收口成稳定、可验证、可继续开发的基础基线。

## 3. 设计选择

采用方案 C：均衡收口。

该方案只补 Web、Control Plane、Runtime Agent 和 contracts 共同需要的底座能力，包括启动、契约一致性、测试、文档、最小真实数据边界和最小执行闭环。不继续深入任何单一业务模块，也不为了未来假设提前设计复杂抽象。

选择理由：

- Web 优先会过早进入页面功能，容易把 mock 数据和页面结构绑死。
- 后端执行链路优先会继续拉大 Runtime 和 Provider 的实现边界，可能延迟 Web 主链路验证。
- 均衡收口能让下一阶段具体功能开发有稳定入口，同时符合当前“不扩展边界、不过度设计”的目标。

## 4. 范围

### 4.1 工程启动底座

收口一条标准本地启动路径：

1. 启动 PostgreSQL、Redis 和 S3 兼容对象存储。
2. 运行数据库迁移。
3. 生成 Runtime token。
4. 启动 Control Plane。
5. 启动 Runtime Agent daemon。
6. 启动 Web 控制台。

相关文档必须说明每一步依赖什么、使用哪个配置文件、如何验证服务已经可用，以及常见失败如何判断是代码问题还是环境问题。

### 4.2 契约一致性底座

Control Plane API、Runtime Agent client 和前端 api-client 必须围绕同一组路径和 payload shape 工作。底座阶段需要建立守护机制，防止后续功能开发时再次出现以下问题：

- Go route 已注册但 OpenAPI 未记录。
- OpenAPI 有路径但 Go handler 不存在。
- Rust Control Plane client 调用旧路径。
- TypeScript api-client 调用路径和真实 Control Plane 不一致。
- 文档示例字段和真实 JSON response shape 不一致。

当前阶段不要求引入完整生成式客户端体系；可以先采用小而明确的 smoke test 或脚本，锁住关键路径。

### 4.3 最小后端执行闭环

底座只证明任务执行主链路可用：

1. 创建任务。
2. Runtime 节点注册。
3. Runtime 心跳。
4. Runtime claim 任务。
5. Runtime 回传结构化事件。
6. Runtime 标记任务完成或失败。
7. Control Plane 可读回任务状态和事件。

该闭环用于证明架构边界成立，不扩展复杂调度、审批策略、工件管理、Provider 会话恢复或多节点容灾。

### 4.4 前端真实数据边界

前端底座只定义后续页面接真实数据所需的共享边界：

- `packages/api-client` 暴露 health、task、runtime node 等最小 API 方法。
- `packages/core` 提供任务、Runtime 节点和健康状态的轻量 summary/helper。
- `packages/views` 保持框架无关，不直接依赖 Next.js、Tauri 或具体 router。
- `apps/web` 可以继续保留首页 mock 内容，但不能让新的业务页面继续绕过 api-client/core 边界。

当前阶段不实现完整任务中心、审批中心、数字员工管理或审计页面。

### 4.5 验证与文档底座

必须明确哪些命令代表基础骨架健康：

```bash
pnpm -r --if-present test
pnpm -r --if-present typecheck
cargo test --manifest-path apps/runtime-agent/Cargo.toml
go test ./apps/control-plane/...
```

如果 `go test ./apps/control-plane/...` 因 Docker/testcontainers 环境失败，文档和最终结果必须明确标注：

- 哪个包失败。
- 失败是否由环境前置条件导致。
- 不依赖 Docker 的 Go 包验证是否通过。
- 如何启动或修复本地 Docker/testcontainers 环境后重跑完整验证。

不能把环境失败伪装成全量测试通过。

## 5. 非目标

本轮底座收口不做以下工作：

- 完整登录认证、session、当前用户和退出登录流程。
- OpenFGA 或复杂企业授权模型。
- Temporal 工作流集成。
- 多租户完整模型。
- 完整任务中心 UI。
- 审批中心 UI 或审批策略引擎。
- 数字员工管理 UI。
- 能力网关完整注册、授权和调用编排。
- 生产级 Provider 会话恢复、取消、重放和错误治理。
- 完整工件管理 UI 或对象存储浏览器。
- CI/CD、部署编排或生产运维体系。

这些内容应跟随后续具体功能开发逐步进入，而不是在底座阶段提前实现。

## 6. 维护性标准

底座完成后，维护性由以下标准判断：

- 新开发者能通过 README 和 `docs/development.md` 启动本地开发链路。
- API 路径变化必须同步更新 OpenAPI、handler、Rust client、TypeScript api-client 和文档。
- 数据库迁移、sqlc 生成和 Go 编译有明确命令。
- Runtime Agent 产品路径是受 Control Plane 管理的 daemon；本地 HTTP API 和 `run` 子命令只作为诊断能力。
- Web 页面不直接散落 fetch 逻辑，优先通过 `packages/api-client` 和 `packages/core` 组织数据。
- 共享 packages 不依赖具体 App runtime，保持可复用。

## 7. 可扩展性标准

底座完成后，可扩展性由以下标准判断：

- 新增 Control Plane endpoint 时，有明确位置更新 OpenAPI、handler、测试和 api-client。
- 新增 Runtime Agent 与 Control Plane 交互时，有明确位置更新 Rust client、OpenAPI 和路由测试。
- 新增 Web 页面时，可复用 `ConsoleShell`、`packages/ui`、`packages/views` 和 `packages/core`。
- 新增 Provider 时，优先通过语言无关 provider contract 和 Runtime adapter 接入，不把 Provider 特例写进 Control Plane 核心业务流程。
- 新增外部能力时，走 Capability Integration Layer，不直接侵入任务或流程核心。

这些标准只定义边界，不要求当前阶段实现所有未来扩展点。

## 8. 可复用性标准

底座完成后，可复用性由以下标准判断：

- UI 基础组件继续放在 `packages/ui`，不携带业务状态。
- 共享业务视图继续放在 `packages/views`，不直接绑定 Next.js 或 Tauri。
- 领域状态、summary 和轻量组合逻辑放在 `packages/core`。
- API 请求、路径和 payload 类型放在 `packages/api-client`。
- Go Control Plane 的领域服务、repository、handler、middleware 保持职责分离。
- Rust Runtime Agent 的 Control Plane client、executor、provider adapter、workspace、health 等模块保持边界清晰。

## 9. 实施方向

后续 implementation plan 应按以下顺序推进：

1. 盘点并修正 README、`docs/development.md`、`docs/api.md`、`docs/NEXT_STEPS.md` 中与当前代码不一致的内容。
2. 增加契约一致性 smoke test 或脚本，覆盖 Go routes、Control Plane OpenAPI、Rust client 和 TypeScript api-client 的关键路径。
3. 收口本地开发验证命令，明确 Docker/testcontainers 环境失败时的替代验证与完整验证方式。
4. 补齐前端真实数据边界中仍缺少的最小 client/core helper，但不创建完整业务页面。
5. 确认最小执行闭环测试覆盖任务创建、Runtime 注册、claim、事件回传和终态读回。
6. 更新 `CHANGELOG.md` 记录底座收口变更。

每一步都应该能独立验证，不把大型重构和业务功能混入同一个变更。

## 10. 验收标准

本轮底座收口完成的验收标准如下：

- README、开发文档、API 文档和下一步指引与当前代码状态一致。
- 本地启动路径清楚描述依赖服务、配置、迁移、Control Plane、Runtime Agent 和 Web。
- 契约一致性检查能证明关键路径没有漂移。
- TypeScript 测试和 typecheck 通过。
- Runtime Agent Rust 测试通过。
- Go 非 Docker 依赖包测试通过。
- Go storage/query 集成测试要么通过，要么明确记录 Docker/testcontainers 前置条件和重跑方式。
- 最小任务执行闭环有自动化测试或等价可复现验证。
- 后续功能开发可以从 `packages/api-client`、`packages/core`、`packages/views` 和 Control Plane API 边界继续推进，而不需要先补基础启动、契约和验证缺口。

只有这些标准都被当前证据证明后，才能认为“基础骨架以及底座能力构建”完成。
