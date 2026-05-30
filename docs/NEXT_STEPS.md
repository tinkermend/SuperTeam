# SuperTeam 下一步工作指引

**文档更新日期**: 2026-05-30

## 当前 Foundation 状态

- Control Plane 编译和统一启动入口已恢复。
- Runtime Agent 默认 daemon 语义已明确。
- Runtime 主链路已具备 fake-provider 端到端验收。

## 下一步建议

### 1. Foundation Readiness 收口

- 保持 `pnpm verify:contracts`、`pnpm verify:foundation`、Rust 测试和 Go 测试入口可用。
- 在进入任务中心、审批中心、数字员工管理等功能前，先确保 README、开发文档、API 文档和实际代码状态一致。
- 将 Web 新页面的数据访问收敛到 `packages/api-client` 和 `packages/core`，避免继续扩展 mock-only 页面结构。

### 2. Web Control Plane UI 与集成

- 基于现有 `packages/api-client` 和 `packages/core` 数据边界，继续补任务列表、任务详情、Runtime 节点等控制台页面。
- 让 Web 主链路直接消费真实 Control Plane API，而不是继续扩展 mock 页面。

### 3. 更丰富的 Provider Adapter

- 在已打通的 fake-provider 验收基础上，继续补 `claude-code`、`opencode` 等真实 provider 的执行细节。
- 完善事件映射、错误分类、会话恢复和工件回传，保持 provider contract 语言无关。

### 4. 生产化加固

- 补强 Runtime Agent 与 Control Plane 的认证、重试、超时、审计和可观测性。
- 继续完善存储、对象上传、故障恢复和多节点运行的边界验证。

### 5. 更完整的联调与回归验证

- 在现有 Control Plane / Runtime foundation 验收之上，补更稳定的本地联调脚本和 CI 验证。
- 对远端数据库、Redis、对象存储和 Provider CLI 等外部依赖缺失场景给出更明确的开发说明。
