-- 能力注册表项目级 MCP 绑定（目录与能力投影修订 spec §3.2）。
-- 项目公共 MCP 只走注册表正门：仓库原生 MCP 配置维持屏蔽，项目绑定与既有
-- team/employee 绑定并列为第三个绑定维度。运行时投影 = 员工侧集合 ∪ 项目绑定
-- 集合，同 server_key 时项目绑定优先；凭据仍只下发 env 变量名，不落值。
-- 结构逐字段镜像 037 的 team_mcp_bindings（team_id -> project_id）。

CREATE TABLE IF NOT EXISTS project_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_project_mcp_bindings_status_supported
        CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_project_mcp_bindings_active
    ON project_mcp_bindings(tenant_id, project_id, mcp_server_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_project_mcp_bindings_server
    ON project_mcp_bindings(tenant_id, mcp_server_id, status, created_at DESC)
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

COMMENT ON TABLE project_mcp_bindings IS '项目对注册表 MCP 的绑定，任务运行时与员工侧集合合并投影，同 server_key 项目侧优先';
COMMENT ON COLUMN project_mcp_bindings.id IS '项目 MCP 绑定主键 UUID';
COMMENT ON COLUMN project_mcp_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN project_mcp_bindings.project_id IS '绑定所属项目 ID';
COMMENT ON COLUMN project_mcp_bindings.mcp_server_id IS '引用的注册表 MCP 定义 ID';
COMMENT ON COLUMN project_mcp_bindings.credential_env_var IS '该绑定使用的凭据环境变量名，值由数字员工环境变量提供';
COMMENT ON COLUMN project_mcp_bindings.status IS '绑定状态，例如 active 或 disabled';
COMMENT ON COLUMN project_mcp_bindings.metadata IS '绑定扩展元数据 JSON';
COMMENT ON COLUMN project_mcp_bindings.disabled_at IS '绑定禁用时间';
COMMENT ON COLUMN project_mcp_bindings.deleted_at IS '绑定软删除时间';
COMMENT ON COLUMN project_mcp_bindings.created_by IS '创建绑定的用户 ID';
COMMENT ON COLUMN project_mcp_bindings.created_at IS '绑定创建时间';
COMMENT ON COLUMN project_mcp_bindings.updated_at IS '绑定更新时间';
