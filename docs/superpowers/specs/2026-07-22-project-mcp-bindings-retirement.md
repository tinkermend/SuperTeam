# 项目级 MCP 绑定退役

> ⚠️ **已被 [`2026-08-06-capability-supply-three-layer-design.md`](./2026-08-06-capability-supply-three-layer-design.md) 取代（2026-08-06 人类拍板）。本文两条理由均不成立：**
> 1. 「该表从未有运行时消费者」与 2026-07-17 的 GATE 证据矛盾——当时真实验证过「员工零 MCP 绑定时工作区 `.superteam/mcp/claude.mcp.json` 为纯项目投影」。
> 2. 「统一走 `.mcp.json` 文件管理」这条替代路径**至今未兑现**：`providers/claude.rs` 仍带 `--strict-mcp-config`，`project_workspace.rs` 的 `shield_repo_configs` 仍屏蔽 opencode 原生配置。净结果是项目级 MCP 两条路都不通。
>
> **不要据本文再次拆除项目级绑定。** 背景与替代方案见新 spec §1.1 / §9.1。

**状态**: **已作废（被 2026-08-06 spec 取代）**  
**日期**: 2026-07-22  

## 背景

项目级 MCP 绑定（`project_mcp_bindings` 表，迁移 072）原设计为给项目注入公共 MCP 能力，但实践中平台侧项目 MCP 配置统一走 `.mcp.json` 文件管理，该表从未有运行时消费者。保留它只会引入无用的 API surface 和维护负担。

## 变更内容

1. **数据库**：`DROP TABLE IF EXISTS project_mcp_bindings`（迁移 `20260722003600`）。
2. **Control Plane**：删除 `capability` 包中 `PutProjectMCPBindings`、`ListProjectMCPBindings`、`ListEffectiveProjectMCPServers` 方法及相关 SQL 查询；`ListEffectiveMCPConfigForRuntime` 保留 `projectID *uuid.UUID` 签名但忽略该参数，仅返回员工侧集合。
3. **API 路由**：删除 `GET/PUT /api/v1/projects/{projectId}/mcp-bindings`。
4. **OpenAPI**：删除 `/projects/{projectId}/mcp-bindings` 路径及 `PutProjectMCPBindingsRequest` schema。
5. **Web**：删除 `listProjectMcpBindings`/`putProjectMcpBindings` API 函数。

## 保留不动

- 团队级 MCP 绑定（`team_mcp_bindings`）
- 员工级 MCP 绑定（`employee_mcp_bindings_v2`）
- Skill MCP 依赖声明
- `ListEffectiveMCPConfigForRuntime` 函数签名（向后兼容 runtime 调用方）
