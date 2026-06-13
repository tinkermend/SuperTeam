CREATE TABLE IF NOT EXISTS user_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    name TEXT NOT NULL,
    credential_type VARCHAR(80) NOT NULL,
    encrypted_value TEXT NOT NULL,
    last_four VARCHAR(8) NOT NULL DEFAULT '',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_credentials_owner_name_active
    ON user_credentials(tenant_id, user_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_credentials_owner_type
    ON user_credentials(tenant_id, user_id, credential_type, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS team_mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    credential_id UUID,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_servers_team_name_active
    ON team_mcp_servers(tenant_id, team_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_team_mcp_servers_team_status
    ON team_mcp_servers(tenant_id, team_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    credential_id UUID,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_mcp_bindings_employee_name_active
    ON digital_employee_mcp_bindings(tenant_id, digital_employee_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employee_mcp_bindings_employee_status
    ON digital_employee_mcp_bindings(tenant_id, digital_employee_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_instruction_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_instruction_files_path_active
    ON digital_employee_instruction_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employee_instruction_files_employee_path
    ON digital_employee_instruction_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_user_credentials_updated_at'
    ) THEN
        CREATE TRIGGER update_user_credentials_updated_at
        BEFORE UPDATE ON user_credentials
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_team_mcp_servers_updated_at'
    ) THEN
        CREATE TRIGGER update_team_mcp_servers_updated_at
        BEFORE UPDATE ON team_mcp_servers
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_mcp_bindings_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_mcp_bindings_updated_at
        BEFORE UPDATE ON digital_employee_mcp_bindings
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_instruction_files_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_instruction_files_updated_at
        BEFORE UPDATE ON digital_employee_instruction_files
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE user_credentials IS '个人凭据池，保存用户可复用的外部能力授权令牌密文';
COMMENT ON COLUMN user_credentials.id IS '个人凭据主键 UUID';
COMMENT ON COLUMN user_credentials.tenant_id IS '凭据所属租户 ID';
COMMENT ON COLUMN user_credentials.user_id IS '凭据所属用户 ID';
COMMENT ON COLUMN user_credentials.name IS '凭据显示名称，同一用户下未删除时唯一';
COMMENT ON COLUMN user_credentials.credential_type IS '凭据类型，例如 mcp_token，由服务端注册表校验';
COMMENT ON COLUMN user_credentials.encrypted_value IS '服务端封存后的凭据密文，API 永不返回明文';
COMMENT ON COLUMN user_credentials.last_four IS '凭据明文末尾四位或更短尾标，用于用户识别';
COMMENT ON COLUMN user_credentials.status IS '凭据状态，例如 active 或 disabled';
COMMENT ON COLUMN user_credentials.metadata IS '凭据扩展元数据 JSON';
COMMENT ON COLUMN user_credentials.disabled_at IS '凭据禁用时间';
COMMENT ON COLUMN user_credentials.deleted_at IS '凭据软删除时间';
COMMENT ON COLUMN user_credentials.created_at IS '凭据创建时间';
COMMENT ON COLUMN user_credentials.updated_at IS '凭据更新时间';

COMMENT ON TABLE team_mcp_servers IS '团队公共 MCP 服务器配置，团队下数字员工强制继承';
COMMENT ON COLUMN team_mcp_servers.id IS '团队 MCP 配置主键 UUID';
COMMENT ON COLUMN team_mcp_servers.tenant_id IS '团队 MCP 所属租户 ID';
COMMENT ON COLUMN team_mcp_servers.team_id IS '团队 MCP 所属团队 ID';
COMMENT ON COLUMN team_mcp_servers.name IS '团队 MCP 显示名称，同一团队下未删除时唯一';
COMMENT ON COLUMN team_mcp_servers.url IS '团队 MCP 远程 HTTP 地址';
COMMENT ON COLUMN team_mcp_servers.credential_id IS '引用的个人凭据 ID，由应用层校验归属和类型';
COMMENT ON COLUMN team_mcp_servers.status IS '团队 MCP 状态，例如 active 或 disabled';
COMMENT ON COLUMN team_mcp_servers.metadata IS '团队 MCP 扩展元数据 JSON';
COMMENT ON COLUMN team_mcp_servers.disabled_at IS '团队 MCP 禁用时间';
COMMENT ON COLUMN team_mcp_servers.deleted_at IS '团队 MCP 软删除时间';
COMMENT ON COLUMN team_mcp_servers.created_by IS '创建团队 MCP 的用户 ID';
COMMENT ON COLUMN team_mcp_servers.created_at IS '团队 MCP 创建时间';
COMMENT ON COLUMN team_mcp_servers.updated_at IS '团队 MCP 更新时间';

COMMENT ON TABLE digital_employee_mcp_bindings IS '数字员工个人 MCP 服务器配置';
COMMENT ON COLUMN digital_employee_mcp_bindings.id IS '数字员工 MCP 配置主键 UUID';
COMMENT ON COLUMN digital_employee_mcp_bindings.tenant_id IS '数字员工 MCP 所属租户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.digital_employee_id IS '数字员工 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.name IS '个人 MCP 显示名称，同一数字员工下未删除时唯一';
COMMENT ON COLUMN digital_employee_mcp_bindings.url IS '个人 MCP 远程 HTTP 地址';
COMMENT ON COLUMN digital_employee_mcp_bindings.credential_id IS '引用的个人凭据 ID，由应用层校验归属和类型';
COMMENT ON COLUMN digital_employee_mcp_bindings.status IS '个人 MCP 状态，例如 active 或 disabled';
COMMENT ON COLUMN digital_employee_mcp_bindings.metadata IS '个人 MCP 扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_mcp_bindings.disabled_at IS '个人 MCP 禁用时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.deleted_at IS '个人 MCP 软删除时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.created_by IS '创建个人 MCP 的用户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings.created_at IS '个人 MCP 创建时间';
COMMENT ON COLUMN digital_employee_mcp_bindings.updated_at IS '个人 MCP 更新时间';

COMMENT ON TABLE digital_employee_instruction_files IS '数字员工个人 Instructions 文件内容';
COMMENT ON COLUMN digital_employee_instruction_files.id IS '数字员工 Instructions 文件主键 UUID';
COMMENT ON COLUMN digital_employee_instruction_files.tenant_id IS '文件所属租户 ID';
COMMENT ON COLUMN digital_employee_instruction_files.digital_employee_id IS '数字员工 ID';
COMMENT ON COLUMN digital_employee_instruction_files.path IS '文件相对路径，例如 AGENTS.md 或 SOUL.md';
COMMENT ON COLUMN digital_employee_instruction_files.content IS '文件文本内容';
COMMENT ON COLUMN digital_employee_instruction_files.size_bytes IS '文件内容字节数';
COMMENT ON COLUMN digital_employee_instruction_files.checksum_sha256 IS '文件内容 SHA256 校验值';
COMMENT ON COLUMN digital_employee_instruction_files.metadata IS '文件扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_instruction_files.deleted_at IS '文件软删除时间';
COMMENT ON COLUMN digital_employee_instruction_files.created_at IS '文件创建时间';
COMMENT ON COLUMN digital_employee_instruction_files.updated_at IS '文件更新时间';
