-- MCP HTTP capability registry: tenant-level MCP definitions plus team/employee bindings.
-- Control Plane is the source of truth; Runtime only materializes provider config from the
-- effective employee payload. Only HTTP / streamable HTTP transports are modeled.

CREATE TABLE IF NOT EXISTS mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name TEXT NOT NULL,
    server_key TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    transport VARCHAR(40) NOT NULL DEFAULT 'streamable_http',
    url TEXT NOT NULL,
    auth_strategy VARCHAR(40) NOT NULL DEFAULT 'none',
    required_env_vars TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    optional_env_vars TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    provider_visibility JSONB NOT NULL DEFAULT '{"codex":true,"claude-code":true,"opencode":true}'::jsonb,
    tool_allowlist TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    risk_level VARCHAR(40) NOT NULL DEFAULT 'medium',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_mcp_servers_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT ck_mcp_servers_server_key_not_blank CHECK (btrim(server_key) <> ''),
    CONSTRAINT ck_mcp_servers_url_not_blank CHECK (btrim(url) <> ''),
    CONSTRAINT ck_mcp_servers_transport_http_only
        CHECK (transport IN ('streamable_http', 'http')),
    CONSTRAINT ck_mcp_servers_auth_strategy
        CHECK (auth_strategy IN ('none', 'bearer_env', 'headers_env')),
    CONSTRAINT ck_mcp_servers_status_supported
        CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_mcp_servers_tenant_key_active
    ON mcp_servers(tenant_id, server_key)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mcp_servers_tenant_status
    ON mcp_servers(tenant_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS team_mcp_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_team_mcp_bindings_status_supported
        CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_mcp_bindings_active
    ON team_mcp_bindings(tenant_id, team_id, mcp_server_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_team_mcp_bindings_server
    ON team_mcp_bindings(tenant_id, mcp_server_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_mcp_bindings_v2 (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    mcp_server_id UUID NOT NULL,
    credential_env_var TEXT,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_employee_mcp_bindings_v2_status_supported
        CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_employee_mcp_bindings_v2_active
    ON digital_employee_mcp_bindings_v2(tenant_id, digital_employee_id, mcp_server_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_employee_mcp_bindings_v2_server
    ON digital_employee_mcp_bindings_v2(tenant_id, mcp_server_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_mcp_servers_updated_at'
    ) THEN
        CREATE TRIGGER update_mcp_servers_updated_at
        BEFORE UPDATE ON mcp_servers
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_team_mcp_bindings_updated_at'
    ) THEN
        CREATE TRIGGER update_team_mcp_bindings_updated_at
        BEFORE UPDATE ON team_mcp_bindings
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_employee_mcp_bindings_v2_updated_at'
    ) THEN
        CREATE TRIGGER update_employee_mcp_bindings_v2_updated_at
        BEFORE UPDATE ON digital_employee_mcp_bindings_v2
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

-- Backfill legacy team MCP rows into the registry. server_key is derived from a normalized
-- name plus a short hash of the URL so distinct URLs never collide. For bearer_env rows we
-- populate required_env_vars with the same placeholder credential env var so the env-var
-- preflight stays consistent (an empty required list would silently bypass the gate).
INSERT INTO mcp_servers (
    tenant_id,
    name,
    server_key,
    url,
    auth_strategy,
    required_env_vars,
    status,
    metadata,
    created_by,
    created_at,
    updated_at
)
SELECT DISTINCT ON (tenant_id, name, url)
    tenant_id,
    name,
    lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g')) || '-' || substr(md5(url), 1, 8),
    url,
    CASE WHEN credential_id IS NULL THEN 'none' ELSE 'bearer_env' END,
    CASE
        WHEN credential_id IS NULL THEN ARRAY[]::TEXT[]
        ELSE ARRAY['MCP_TOKEN_' || upper(regexp_replace(name, '[^a-zA-Z0-9]+', '_', 'g'))]
    END,
    CASE WHEN status IN ('active', 'disabled') THEN status ELSE 'active' END,
    metadata || jsonb_build_object('legacy_source', 'team_mcp_servers'),
    created_by,
    created_at,
    updated_at
FROM team_mcp_servers
WHERE deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO mcp_servers (
    tenant_id,
    name,
    server_key,
    url,
    auth_strategy,
    required_env_vars,
    status,
    metadata,
    created_by,
    created_at,
    updated_at
)
SELECT DISTINCT ON (tenant_id, name, url)
    tenant_id,
    name,
    lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g')) || '-' || substr(md5(url), 1, 8),
    url,
    CASE WHEN credential_id IS NULL THEN 'none' ELSE 'bearer_env' END,
    CASE
        WHEN credential_id IS NULL THEN ARRAY[]::TEXT[]
        ELSE ARRAY['MCP_TOKEN_' || upper(regexp_replace(name, '[^a-zA-Z0-9]+', '_', 'g'))]
    END,
    CASE WHEN status IN ('active', 'disabled') THEN status ELSE 'active' END,
    metadata || jsonb_build_object('legacy_source', 'digital_employee_mcp_bindings'),
    created_by,
    created_at,
    updated_at
FROM digital_employee_mcp_bindings
WHERE deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- Backfill team bindings against the freshly registered servers, matching on (tenant_id, url).
INSERT INTO team_mcp_bindings (
    tenant_id,
    team_id,
    mcp_server_id,
    credential_env_var,
    status,
    metadata,
    created_by,
    created_at,
    updated_at
)
SELECT
    legacy.tenant_id,
    legacy.team_id,
    reg.id,
    CASE WHEN legacy.credential_id IS NULL THEN NULL
         ELSE 'MCP_TOKEN_' || upper(regexp_replace(legacy.name, '[^a-zA-Z0-9]+', '_', 'g'))
    END,
    CASE WHEN legacy.status IN ('active', 'disabled') THEN legacy.status ELSE 'active' END,
    jsonb_build_object('legacy_source', 'team_mcp_servers'),
    legacy.created_by,
    legacy.created_at,
    legacy.updated_at
FROM team_mcp_servers legacy
JOIN mcp_servers reg
    ON reg.tenant_id = legacy.tenant_id
   AND reg.url = legacy.url
   AND reg.deleted_at IS NULL
WHERE legacy.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- Backfill employee bindings against the freshly registered servers.
INSERT INTO digital_employee_mcp_bindings_v2 (
    tenant_id,
    digital_employee_id,
    mcp_server_id,
    credential_env_var,
    status,
    metadata,
    created_by,
    created_at,
    updated_at
)
SELECT
    legacy.tenant_id,
    legacy.digital_employee_id,
    reg.id,
    CASE WHEN legacy.credential_id IS NULL THEN NULL
         ELSE 'MCP_TOKEN_' || upper(regexp_replace(legacy.name, '[^a-zA-Z0-9]+', '_', 'g'))
    END,
    CASE WHEN legacy.status IN ('active', 'disabled') THEN legacy.status ELSE 'active' END,
    jsonb_build_object('legacy_source', 'digital_employee_mcp_bindings'),
    legacy.created_by,
    legacy.created_at,
    legacy.updated_at
FROM digital_employee_mcp_bindings legacy
JOIN mcp_servers reg
    ON reg.tenant_id = legacy.tenant_id
   AND reg.url = legacy.url
   AND reg.deleted_at IS NULL
WHERE legacy.deleted_at IS NULL
ON CONFLICT DO NOTHING;

COMMENT ON TABLE mcp_servers IS '租户级 MCP HTTP 能力注册表，定义一次、被团队与数字员工绑定复用';
COMMENT ON COLUMN mcp_servers.id IS 'MCP 定义主键 UUID';
COMMENT ON COLUMN mcp_servers.tenant_id IS 'MCP 定义所属租户 ID';
COMMENT ON COLUMN mcp_servers.name IS 'MCP 显示名称';
COMMENT ON COLUMN mcp_servers.server_key IS 'MCP 稳定标识，渲染到 provider 配置的 server 键，租户内未删除时唯一';
COMMENT ON COLUMN mcp_servers.description IS 'MCP 描述';
COMMENT ON COLUMN mcp_servers.transport IS 'MCP 传输方式，仅支持 streamable_http 或 http';
COMMENT ON COLUMN mcp_servers.url IS 'MCP 远程 HTTP 地址';
COMMENT ON COLUMN mcp_servers.auth_strategy IS 'MCP 鉴权方式：none、bearer_env 或 headers_env';
COMMENT ON COLUMN mcp_servers.required_env_vars IS 'MCP 必需环境变量名列表，数字员工缺失则绑定被阻断';
COMMENT ON COLUMN mcp_servers.optional_env_vars IS 'MCP 可选环境变量名列表';
COMMENT ON COLUMN mcp_servers.provider_visibility IS '各 provider 是否投射此 MCP 的可见性开关 JSON';
COMMENT ON COLUMN mcp_servers.tool_allowlist IS 'MCP 工具白名单';
COMMENT ON COLUMN mcp_servers.risk_level IS 'MCP 风险等级';
COMMENT ON COLUMN mcp_servers.status IS 'MCP 状态，例如 active 或 disabled';
COMMENT ON COLUMN mcp_servers.metadata IS 'MCP 扩展元数据 JSON';
COMMENT ON COLUMN mcp_servers.disabled_at IS 'MCP 禁用时间';
COMMENT ON COLUMN mcp_servers.deleted_at IS 'MCP 软删除时间';
COMMENT ON COLUMN mcp_servers.created_by IS '创建 MCP 定义的用户 ID';
COMMENT ON COLUMN mcp_servers.created_at IS 'MCP 创建时间';
COMMENT ON COLUMN mcp_servers.updated_at IS 'MCP 更新时间';

COMMENT ON TABLE team_mcp_bindings IS '团队对注册表 MCP 的绑定，团队下数字员工继承';
COMMENT ON COLUMN team_mcp_bindings.id IS '团队 MCP 绑定主键 UUID';
COMMENT ON COLUMN team_mcp_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN team_mcp_bindings.team_id IS '绑定所属团队 ID';
COMMENT ON COLUMN team_mcp_bindings.mcp_server_id IS '引用的注册表 MCP 定义 ID';
COMMENT ON COLUMN team_mcp_bindings.credential_env_var IS '该绑定使用的凭据环境变量名，值由数字员工环境变量提供';
COMMENT ON COLUMN team_mcp_bindings.status IS '绑定状态，例如 active 或 disabled';
COMMENT ON COLUMN team_mcp_bindings.metadata IS '绑定扩展元数据 JSON';
COMMENT ON COLUMN team_mcp_bindings.disabled_at IS '绑定禁用时间';
COMMENT ON COLUMN team_mcp_bindings.deleted_at IS '绑定软删除时间';
COMMENT ON COLUMN team_mcp_bindings.created_by IS '创建绑定的用户 ID';
COMMENT ON COLUMN team_mcp_bindings.created_at IS '绑定创建时间';
COMMENT ON COLUMN team_mcp_bindings.updated_at IS '绑定更新时间';

COMMENT ON TABLE digital_employee_mcp_bindings_v2 IS '数字员工对注册表 MCP 的个人绑定';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.id IS '员工 MCP 绑定主键 UUID';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.digital_employee_id IS '数字员工 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.mcp_server_id IS '引用的注册表 MCP 定义 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.credential_env_var IS '该绑定使用的凭据环境变量名，值由数字员工环境变量提供';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.status IS '绑定状态，例如 active 或 disabled';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.metadata IS '绑定扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.disabled_at IS '绑定禁用时间';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.deleted_at IS '绑定软删除时间';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.created_by IS '创建绑定的用户 ID';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.created_at IS '绑定创建时间';
COMMENT ON COLUMN digital_employee_mcp_bindings_v2.updated_at IS '绑定更新时间';
