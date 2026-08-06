-- 能力供给三层模型（2026-08-06）：复原 project_mcp_bindings + 新建 project_skill_bindings。
-- project_mcp_bindings 照 072 语义复原，形状取 087 之后的最终态（无 status/disabled_at）。
-- project_skill_bindings 同时表达场地限定与场地供给，投影规则见三层模型 spec §4.2。

CREATE TABLE IF NOT EXISTS project_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_project_mcp_bindings_active
    ON project_mcp_bindings(tenant_id, project_id, mcp_server_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_project_mcp_bindings_server
    ON project_mcp_bindings(tenant_id, mcp_server_id, created_at DESC)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_project_mcp_bindings_updated_at'
    ) THEN
        CREATE TRIGGER update_project_mcp_bindings_updated_at
        BEFORE UPDATE ON project_mcp_bindings
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE project_mcp_bindings IS '项目对注册表 MCP 的绑定：任务运行时与员工侧集合并集投影，同 server_key 项目侧优先；依赖闭包补全的 MCP 另标 source=dependency_closure';
COMMENT ON COLUMN project_mcp_bindings.id IS '项目 MCP 绑定主键 UUID';
COMMENT ON COLUMN project_mcp_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN project_mcp_bindings.project_id IS '绑定所属项目 ID';
COMMENT ON COLUMN project_mcp_bindings.mcp_server_id IS '引用的注册表 MCP 定义 ID';
COMMENT ON COLUMN project_mcp_bindings.credential_env_var IS '该绑定使用的凭据环境变量名，值由数字员工环境变量提供';
COMMENT ON COLUMN project_mcp_bindings.metadata IS '绑定扩展元数据 JSON';
COMMENT ON COLUMN project_mcp_bindings.deleted_at IS '绑定软删除时间';
COMMENT ON COLUMN project_mcp_bindings.created_by IS '创建绑定的用户 ID';
COMMENT ON COLUMN project_mcp_bindings.created_at IS '绑定创建时间';
COMMENT ON COLUMN project_mcp_bindings.updated_at IS '绑定更新时间';

CREATE TABLE IF NOT EXISTS project_skill_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    created_by_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_project_skill_bindings_project_skill UNIQUE (tenant_id, project_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_project_skill_bindings_skill
    ON project_skill_bindings(tenant_id, skill_id);

CREATE INDEX IF NOT EXISTS idx_project_skill_bindings_project
    ON project_skill_bindings(tenant_id, project_id, created_at DESC);

COMMENT ON TABLE project_skill_bindings IS '项目技能绑定：同时表达场地限定与场地供给（见能力供给三层模型 §4.2）';
COMMENT ON COLUMN project_skill_bindings.id IS '项目技能绑定主键 UUID';
COMMENT ON COLUMN project_skill_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN project_skill_bindings.project_id IS '绑定所属项目 ID';
COMMENT ON COLUMN project_skill_bindings.skill_id IS '绑定的技能 ID';
COMMENT ON COLUMN project_skill_bindings.created_by_user_id IS '创建绑定的用户 ID';
COMMENT ON COLUMN project_skill_bindings.created_at IS '绑定创建时间';
