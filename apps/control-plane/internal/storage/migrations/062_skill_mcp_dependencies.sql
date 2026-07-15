-- 062_skill_mcp_dependencies.sql
-- 技能对注册表 MCP 能力的依赖声明（只校验不授权，见 spec 2026-07-15）。
-- 两侧实体均为软删除，FK 仅保护硬删路径；应用层负责删除保护。

CREATE TABLE IF NOT EXISTS skill_mcp_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    mcp_server_id UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE RESTRICT,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_mcp_dependencies_tenant_skill_server
    ON skill_mcp_dependencies(tenant_id, skill_id, mcp_server_id);

CREATE INDEX IF NOT EXISTS idx_skill_mcp_dependencies_tenant_server
    ON skill_mcp_dependencies(tenant_id, mcp_server_id);
