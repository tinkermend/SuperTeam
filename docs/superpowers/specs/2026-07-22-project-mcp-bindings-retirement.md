# 项目级 MCP 绑定退役

**状态**: 已落地  
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
